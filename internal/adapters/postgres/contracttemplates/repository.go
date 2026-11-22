package contracttemplatespg

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) begin(ctx context.Context, actor string, spaces ...string) (pgx.Tx, error) {
	if !contracttemplate.ValidIDs(append([]string{actor}, spaces...)...) {
		return nil, contracttemplate.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// The common space lock serializes owner writes, publication, and copying.
	// Sorting also makes concurrent A→B/B→A copy operations deadlock-free.
	spaces = append([]string(nil), spaces...)
	for i := range spaces {
		spaces[i] = uuid.MustParse(spaces[i]).String()
	}
	sort.Strings(spaces)
	for i, id := range spaces {
		if i > 0 && id == spaces[i-1] {
			continue
		}
		var found string
		err = tx.QueryRow(ctx, `SELECT id::text FROM public.espacio WHERE id=$1 AND propietario_id=$2 AND estado <> 'eliminada' FOR UPDATE`, id, actor).Scan(&found)
		if err != nil {
			_ = tx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, contracttemplate.ErrNotFound
			}
			return nil, err
		}
	}
	return tx, nil
}

func readDraft(ctx context.Context, tx pgx.Tx, space string) (contracttemplate.Draft, error) {
	d := contracttemplate.Draft{SpaceID: space, Sections: []contracttemplate.Section{}}
	var blob []byte
	var updated time.Time
	err := tx.QueryRow(ctx, `SELECT revision,secciones,version_publicada,actualizada_en FROM public.espacio_contrato_borrador WHERE espacio_id=$1`, space).Scan(&d.Revision, &blob, &d.PublishedVersion, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	d.UpdatedAt = &updated
	err = json.Unmarshal(blob, &d.Sections)
	return d, err
}

func (r *Repository) Get(ctx context.Context, actor, space string) (contracttemplate.Draft, error) {
	tx, err := r.begin(ctx, actor, space)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	defer tx.Rollback(ctx)
	d, err := readDraft(ctx, tx, space)
	if err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}

func save(ctx context.Context, tx pgx.Tx, space string, expected int64, sections []contracttemplate.Section) (contracttemplate.Draft, error) {
	blob, _, err := contracttemplate.CanonicalSections(sections)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	d, err := readDraft(ctx, tx, space)
	if err != nil {
		return d, err
	}
	if expected < 0 || d.Revision != expected {
		return d, contracttemplate.ErrPrecondition
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.espacio_contrato_borrador(espacio_id,revision,secciones) VALUES($1,1,$2) ON CONFLICT(espacio_id) DO UPDATE SET revision=espacio_contrato_borrador.revision+1,secciones=EXCLUDED.secciones,actualizada_en=clock_timestamp()`, space, blob)
	if err != nil {
		return d, err
	}
	return readDraft(ctx, tx, space)
}

func (r *Repository) Save(ctx context.Context, actor, space string, expected int64, sections []contracttemplate.Section) (contracttemplate.Draft, error) {
	if _, _, err := contracttemplate.CanonicalSections(sections); err != nil {
		return contracttemplate.Draft{}, err
	}
	tx, err := r.begin(ctx, actor, space)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	defer tx.Rollback(ctx)
	d, err := save(ctx, tx, space, expected, sections)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	return d, tx.Commit(ctx)
}

const versionColumns = `id::text,espacio_id::text,version,secciones,encode(sha256,'hex'),publicada_en`

func scanVersion(row pgx.Row) (contracttemplate.Version, error) {
	var v contracttemplate.Version
	var blob []byte
	err := row.Scan(&v.ID, &v.SpaceID, &v.Version, &blob, &v.SHA256, &v.PublishedAt)
	if err == nil {
		err = json.Unmarshal(blob, &v.Sections)
	}
	return v, err
}

func (r *Repository) Publish(ctx context.Context, actor, space string, expected int64) (contracttemplate.Version, int64, error) {
	tx, err := r.begin(ctx, actor, space)
	if err != nil {
		return contracttemplate.Version{}, 0, err
	}
	defer tx.Rollback(ctx)
	d, err := readDraft(ctx, tx, space)
	if err != nil {
		return contracttemplate.Version{}, 0, err
	}
	if expected > 0 && d.Revision == expected+1 {
		v, readErr := scanVersion(tx.QueryRow(ctx, `SELECT `+versionColumns+` FROM public.espacio_contrato_version WHERE espacio_id=$1 AND revision_origen=$2 AND version=$3`, space, expected, d.PublishedVersion))
		if readErr == nil {
			return v, d.Revision, tx.Commit(ctx)
		}
		if !errors.Is(readErr, pgx.ErrNoRows) {
			return v, 0, readErr
		}
	}
	if d.Revision != expected {
		return contracttemplate.Version{}, 0, contracttemplate.ErrPrecondition
	}
	if expected < 1 {
		return contracttemplate.Version{}, 0, contracttemplate.ErrConflict
	}
	blob, digest, err := contracttemplate.CanonicalSections(d.Sections)
	if err != nil {
		return contracttemplate.Version{}, 0, err
	}
	v, err := scanVersion(tx.QueryRow(ctx, `INSERT INTO public.espacio_contrato_version(id,espacio_id,version,revision_origen,texto_canonico,secciones,sha256,publicada_por) VALUES($1,$2,$3,$4,$5,$6,decode($7,'hex'),$8) RETURNING `+versionColumns, uuid.NewString(), space, d.PublishedVersion+1, d.Revision, string(blob), blob, digest, actor))
	if err != nil {
		return v, 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE public.espacio_contrato_borrador SET version_publicada=$2,revision=revision+1,actualizada_en=clock_timestamp() WHERE espacio_id=$1`, space, v.Version)
	if err != nil {
		return v, 0, err
	}
	return v, d.Revision + 1, tx.Commit(ctx)
}

func (r *Repository) Versions(ctx context.Context, actor, space string) ([]contracttemplate.Version, error) {
	tx, err := r.begin(ctx, actor, space)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+versionColumns+` FROM public.espacio_contrato_version WHERE espacio_id=$1 ORDER BY version DESC LIMIT 100`, space)
	if err != nil {
		return nil, err
	}
	versions := []contracttemplate.Version{}
	for rows.Next() {
		v, e := scanVersion(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		versions = append(versions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return versions, tx.Commit(ctx)
}

func (r *Repository) Copy(ctx context.Context, actor, space string, expected int64, source string, sourceVersion int64) (contracttemplate.Draft, error) {
	if sourceVersion < 1 {
		return contracttemplate.Draft{}, contracttemplate.ErrInvalid
	}
	tx, err := r.begin(ctx, actor, space, source)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	defer tx.Rollback(ctx)
	v, err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionColumns+` FROM public.espacio_contrato_version WHERE espacio_id=$1 AND version=$2`, source, sourceVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return contracttemplate.Draft{}, contracttemplate.ErrNotFound
	}
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	d, err := save(ctx, tx, space, expected, v.Sections)
	if err != nil {
		return contracttemplate.Draft{}, err
	}
	return d, tx.Commit(ctx)
}

var _ contracttemplate.Repository = (*Repository)(nil)
