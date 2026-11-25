package commercialauthorizationpg

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialauthorization"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

const (
	otpTTL         = 5 * time.Minute
	maxOTPAttempts = 5
)

var revocationReasonPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func computeOTPMAC(challengeID, actorID, spaceID string, templateVersion int64, code string) []byte {
	mac := hmac.New(sha256.New, []byte(challengeID+":"+actorID))
	mac.Write([]byte(fmt.Sprintf("%s:%d:%s", spaceID, templateVersion, code)))
	return mac.Sum(nil)
}

func maskEmail(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:2] + "***@" + parts[1]
}

const grantCols = `a.id::text,a.espacio_id::text,a.propietario_id::text,a.plantilla_version,a.monto_maximo_clp,a.vigente_desde,a.vence_en,a.creada_en,a.revocada_en,COALESCE(a.revocada_por::text,''),COALESCE(a.motivo_revocacion,'')`

func scanGrant(row pgx.Row) (commercialauthorization.Grant, error) {
	var g commercialauthorization.Grant
	var revoked *time.Time
	err := row.Scan(&g.ID, &g.SpaceID, &g.OwnerID, &g.TemplateVersion, &g.MaxAmountCLP, &g.ValidFrom, &g.ExpiresAt, &g.CreatedAt, &revoked, &g.RevokedBy, &g.RevocationReason)
	if revoked != nil {
		v := revoked.UTC()
		g.RevokedAt = &v
	}
	return g, err
}

func (r *Repository) Current(ctx context.Context, owner, space string, at time.Time) (commercialauthorization.Grant, error) {
	if r == nil || r.pool == nil || ctx == nil || !commercialauthorization.ValidIDs(owner, space) || at.IsZero() {
		return commercialauthorization.Grant{}, commercialauthorization.ErrInvalid
	}
	g, err := scanGrant(r.pool.QueryRow(ctx, `SELECT `+grantCols+` FROM public.autorizacion_arriendo_comercial a JOIN public.espacio e ON e.id=a.espacio_id AND e.propietario_id=a.propietario_id JOIN public.espacio_contrato_borrador d ON d.espacio_id=a.espacio_id AND d.version_publicada=a.plantilla_version WHERE a.propietario_id=$1 AND a.espacio_id=$2 AND a.revocada_en IS NULL ORDER BY a.creada_en DESC,a.id DESC LIMIT 1`, uuid.MustParse(owner).String(), uuid.MustParse(space).String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, commercialauthorization.ErrNotFound
	}
	return g, err
}

func (r *Repository) History(ctx context.Context, owner, space string) ([]commercialauthorization.Event, error) {
	if r == nil || r.pool == nil || ctx == nil || !commercialauthorization.ValidIDs(owner, space) {
		return nil, commercialauthorization.ErrInvalid
	}
	rows, err := r.pool.Query(ctx, `SELECT v.autorizacion_id::text,v.espacio_id::text,v.actor_id::text,v.accion,v.motivo,v.ocurrida_en FROM public.autorizacion_arriendo_comercial_evento v JOIN public.espacio e ON e.id=v.espacio_id WHERE e.id=$1 AND e.propietario_id=$2 ORDER BY v.ocurrida_en DESC,v.id DESC LIMIT 100`, uuid.MustParse(space).String(), uuid.MustParse(owner).String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []commercialauthorization.Event{}
	for rows.Next() {
		var e commercialauthorization.Event
		if err = rows.Scan(&e.GrantID, &e.SpaceID, &e.ActorID, &e.Action, &e.Reason, &e.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Repository) lockOwnerSpace(ctx context.Context, tx pgx.Tx, owner, space string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT e.id::text FROM public.espacio e WHERE e.id=$1 AND e.propietario_id=$2 AND e.estado<>'eliminada' FOR UPDATE`, space, owner).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return commercialauthorization.ErrNotFound
	}
	return err
}

func (r *Repository) CreateOTPChallenge(ctx context.Context, owner, space string, in commercialauthorization.OTPChallengeInput, now time.Time) (commercialauthorization.OTPChallengeResult, string, string, error) {
	if r == nil || r.pool == nil || ctx == nil || !commercialauthorization.ValidIDs(owner, space) || in.TemplateVersion < 1 || now.IsZero() {
		return commercialauthorization.OTPChallengeResult{}, "", "", commercialauthorization.ErrInvalid
	}
	owner, space = uuid.MustParse(owner).String(), uuid.MustParse(space).String()
	var spaceCheck string
	err := r.pool.QueryRow(ctx, `SELECT e.id::text FROM public.espacio e WHERE e.id=$1 AND e.propietario_id=$2 AND e.estado<>'eliminada'`, space, owner).Scan(&spaceCheck)
	if errors.Is(err, pgx.ErrNoRows) {
		return commercialauthorization.OTPChallengeResult{}, "", "", commercialauthorization.ErrNotFound
	}
	if err != nil {
		return commercialauthorization.OTPChallengeResult{}, "", "", err
	}
	var published int64
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(version_publicada, 0) FROM public.espacio_contrato_borrador WHERE espacio_id=$1`, space).Scan(&published)
	if errors.Is(err, pgx.ErrNoRows) || published != in.TemplateVersion {
		return commercialauthorization.OTPChallengeResult{}, "", "", commercialauthorization.ErrConflict
	}
	if err != nil {
		return commercialauthorization.OTPChallengeResult{}, "", "", err
	}
	var email string
	err = r.pool.QueryRow(ctx, `SELECT correo_original FROM public.usuario WHERE id=$1 AND estado='activo'`, owner).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) || strings.TrimSpace(email) == "" {
		return commercialauthorization.OTPChallengeResult{}, "", "", commercialauthorization.ErrNotFound
	}
	if err != nil {
		return commercialauthorization.OTPChallengeResult{}, "", "", err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return commercialauthorization.OTPChallengeResult{}, "", "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	challengeID := uuid.NewString()
	at := now.UTC()
	expiresAt := at.Add(otpTTL)
	digest := computeOTPMAC(challengeID, owner, space, in.TemplateVersion, code)
	_, err = r.pool.Exec(ctx, `INSERT INTO public.autorizacion_firma_desafio_otp(id,espacio_id,actor_id,plantilla_version,canal,codigo_hmac,intentos,expira_en,creada_en) VALUES($1,$2,$3,$4,'email_otp',$5,0,$6,$7)`, challengeID, space, owner, in.TemplateVersion, digest, expiresAt, at)
	if err != nil {
		return commercialauthorization.OTPChallengeResult{}, "", "", err
	}
	return commercialauthorization.OTPChallengeResult{
		ChallengeID: challengeID,
		Channel:     "email_otp",
		ExpiresAt:   expiresAt,
		MaskedEmail: maskEmail(email),
	}, email, code, nil
}

func (r *Repository) verifyOTPInTx(ctx context.Context, tx pgx.Tx, owner, space string, templateVersion int64, challengeID, code string, now time.Time) error {
	var dbSpaceID, dbActorID string
	var dbTemplateVersion int64
	var storedMAC []byte
	var attempts int
	var expiresAt time.Time
	var verifiedAt *time.Time
	err := tx.QueryRow(ctx, `SELECT espacio_id::text, actor_id::text, plantilla_version, codigo_hmac, intentos, expira_en, verificado_en FROM public.autorizacion_firma_desafio_otp WHERE id=$1 FOR UPDATE`, challengeID).Scan(&dbSpaceID, &dbActorID, &dbTemplateVersion, &storedMAC, &attempts, &expiresAt, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return commercialauthorization.ErrOTPInvalid
	}
	if err != nil {
		return err
	}
	if dbActorID != owner || dbSpaceID != space || dbTemplateVersion != templateVersion || verifiedAt != nil {
		return commercialauthorization.ErrOTPInvalid
	}
	at := now.UTC()
	if !expiresAt.After(at) {
		return commercialauthorization.ErrOTPExpired
	}
	if attempts >= maxOTPAttempts {
		return commercialauthorization.ErrOTPInvalid
	}
	expectedMAC := computeOTPMAC(challengeID, owner, space, templateVersion, code)
	if subtle.ConstantTimeCompare(storedMAC, expectedMAC) != 1 {
		_ = tx.Rollback(ctx)
		_, _ = r.pool.Exec(ctx, `UPDATE public.autorizacion_firma_desafio_otp SET intentos = LEAST(intentos + 1, 5) WHERE id=$1 AND verificado_en IS NULL`, challengeID)
		return commercialauthorization.ErrOTPInvalid
	}
	_, err = tx.Exec(ctx, `UPDATE public.autorizacion_firma_desafio_otp SET verificado_en=$2 WHERE id=$1`, challengeID, at)
	return err
}

func (r *Repository) Activate(ctx context.Context, owner, space string, a commercialauthorization.Activation, now time.Time) (commercialauthorization.Grant, error) {
	if r == nil || r.pool == nil || ctx == nil || !commercialauthorization.ValidIDs(owner, space) || !commercialauthorization.ValidateActivation(a, now) || !a.Consent {
		return commercialauthorization.Grant{}, commercialauthorization.ErrInvalid
	}
	if !commercialauthorization.ValidIDs(a.OTPChallengeID) || !commercialauthorization.ValidOTPCode(a.OTPCode) {
		return commercialauthorization.Grant{}, commercialauthorization.ErrOTPRequired
	}
	owner, space = uuid.MustParse(owner).String(), uuid.MustParse(space).String()
	challengeID := uuid.MustParse(a.OTPChallengeID).String()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercialauthorization.Grant{}, err
	}
	defer tx.Rollback(ctx)
	if err = r.lockOwnerSpace(ctx, tx, owner, space); err != nil {
		return commercialauthorization.Grant{}, err
	}
	var published int64
	err = tx.QueryRow(ctx, `SELECT COALESCE(version_publicada, 0) FROM public.espacio_contrato_borrador WHERE espacio_id=$1 FOR UPDATE`, space).Scan(&published)
	if errors.Is(err, pgx.ErrNoRows) || published != a.TemplateVersion {
		return commercialauthorization.Grant{}, commercialauthorization.ErrConflict
	}
	if err != nil {
		return commercialauthorization.Grant{}, err
	}
	if err = r.verifyOTPInTx(ctx, tx, owner, space, a.TemplateVersion, challengeID, a.OTPCode, now); err != nil {
		return commercialauthorization.Grant{}, err
	}
	// Expired authorizations remain visible until renewed or explicitly revoked.
	// Closing them here frees the single-active slot and records the transition.
	_, err = tx.Exec(ctx, `WITH stale AS (UPDATE public.autorizacion_arriendo_comercial SET revocada_en=$3,revocada_por=propietario_id,motivo_revocacion=CASE WHEN propietario_id<>$2 THEN 'ownership_changed' ELSE 'authorization_expired' END WHERE espacio_id=$1 AND revocada_en IS NULL AND (propietario_id<>$2 OR vence_en<=$3) RETURNING id,espacio_id,propietario_id,motivo_revocacion) INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) SELECT gen_random_uuid(),id,espacio_id,propietario_id,'revoked',motivo_revocacion,$3 FROM stale`, space, owner, now.UTC())
	if err != nil {
		return commercialauthorization.Grant{}, err
	}
	id := uuid.NewString()
	g, err := scanGrant(tx.QueryRow(ctx, `INSERT INTO public.autorizacion_arriendo_comercial AS a(id,espacio_id,propietario_id,plantilla_version,monto_maximo_clp,vigente_desde,vence_en,creada_en) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+grantCols, id, space, owner, a.TemplateVersion, a.MaxAmountCLP, a.ValidFrom.UTC(), a.ExpiresAt.UTC(), now.UTC()))
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return commercialauthorization.Grant{}, commercialauthorization.ErrConflict
		}
		return g, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) VALUES($1,$2,$3,$4,'activated','owner_opt_in',$5)`, uuid.NewString(), id, space, owner, now.UTC())
	if err != nil {
		return g, err
	}
	return g, tx.Commit(ctx)
}

func (r *Repository) Revoke(ctx context.Context, owner, space, id, reason string, now time.Time) (commercialauthorization.Grant, error) {
	if r == nil || r.pool == nil || ctx == nil || !commercialauthorization.ValidIDs(owner, space, id) || now.IsZero() || !revocationReasonPattern.MatchString(reason) {
		return commercialauthorization.Grant{}, commercialauthorization.ErrInvalid
	}
	owner, space, id = uuid.MustParse(owner).String(), uuid.MustParse(space).String(), uuid.MustParse(id).String()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return commercialauthorization.Grant{}, err
	}
	defer tx.Rollback(ctx)
	if err = r.lockOwnerSpace(ctx, tx, owner, space); err != nil {
		return commercialauthorization.Grant{}, err
	}
	g, err := scanGrant(tx.QueryRow(ctx, `UPDATE public.autorizacion_arriendo_comercial a SET revocada_en=$4,revocada_por=$1,motivo_revocacion=$5 WHERE a.id=$3 AND a.espacio_id=$2 AND a.propietario_id=$1 AND a.revocada_en IS NULL RETURNING `+grantCols, owner, space, id, now.UTC(), reason))
	if errors.Is(err, pgx.ErrNoRows) {
		return commercialauthorization.Grant{}, commercialauthorization.ErrNotFound
	}
	if err != nil {
		return g, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) VALUES($1,$2,$3,$4,'revoked',$5,$6)`, uuid.NewString(), id, space, owner, reason, now.UTC())
	if err != nil {
		return g, err
	}
	return g, tx.Commit(ctx)
}

var _ commercialauthorization.Repository = (*Repository)(nil)
