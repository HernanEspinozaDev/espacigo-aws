package kybpg

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) GetOrCreateOrganization(ctx context.Context, ownerID, candidateID string) (string, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(ownerID) || !kyb.ValidUUID(candidateID) {
		return "", kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = lockActiveOwner(ctx, tx, ownerID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.organizacion(id,usuario_id) VALUES($1,$2) ON CONFLICT(usuario_id) DO NOTHING`, candidateID, ownerID); err != nil {
		return "", mapError(err)
	}
	var organizationID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM public.organizacion WHERE usuario_id=$1`, ownerID).Scan(&organizationID); err != nil {
		return "", mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", mapError(err)
	}
	return organizationID, nil
}

func (r *Repository) Create(ctx context.Context, item kyb.Verification, documents []kyb.Document) (kyb.Verification, bool, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(item.OwnerID) || !kyb.ValidUUID(item.OrganizationID) || !kyb.ValidUUID(item.ID) || len(documents) != 3 {
		return kyb.Verification{}, false, kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = lockActiveOwner(ctx, tx, item.OwnerID); err != nil {
		return kyb.Verification{}, false, err
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT usuario_id::text FROM public.organizacion WHERE id=$1 FOR UPDATE`, item.OrganizationID).Scan(&owner); err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	if owner != item.OwnerID {
		return kyb.Verification{}, false, kyb.ErrForbidden
	}
	prior, err := scanVerification(tx.QueryRow(ctx, verificationSelect+` WHERE v.submitted_by=$1 AND v.idempotency_key=$2 ORDER BY v.created_at DESC LIMIT 1`, item.OwnerID, item.IdempotencyKey))
	if err == nil {
		prior.Documents, err = listDocuments(ctx, tx, prior.ID)
		if err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
		if err = tx.Commit(ctx); err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
		return prior, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return kyb.Verification{}, false, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.kyb_verifications(
		id,organization_id,submitted_by,idempotency_key,legal_name,company_rut_fingerprint,
		company_rut_key_version,company_rut_ciphertext,representative_name,
		representative_rut_fingerprint,representative_rut_key_version,
		representative_rut_ciphertext,sii_giro_code,status,submitted_at,created_at,updated_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		item.ID, item.OrganizationID, item.SubmittedBy, item.IdempotencyKey, item.LegalName,
		item.CompanyRUTFingerprint, item.CompanyRUTKeyVersion, item.CompanyRUTCiphertext,
		item.RepresentativeName, item.RepresentativeRUTFprint, item.RepresentativeRUTKey,
		item.RepresentativeRUTCipher, item.SIIGiroCode, kyb.StatusPending,
		item.SubmittedAt, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	for _, document := range documents {
		if !kyb.ValidUUID(document.ID) || !kyb.ValidUUID(document.StorageRef) || document.VerificationID != item.ID {
			return kyb.Verification{}, false, kyb.ErrInvalid
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.kyb_verification_document(
			id,verification_id,document_kind,storage_ref,mime_type,size_bytes,content_sha256
		) VALUES($1,$2,$3,$4,$5,$6,$7)`, document.ID, item.ID, document.Kind, document.StorageRef, document.MIME, document.Size, document.SHA256); err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
	}
	for _, fingerprint := range item.CompanyRUTFingerprints {
		if _, err = tx.Exec(ctx, `INSERT INTO public.kyb_rut_claim(rut_kind,fingerprint,verification_id,organization_id) VALUES('company',$1,$2,$3)`, fingerprint, item.ID, item.OrganizationID); err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
	}
	for _, fingerprint := range item.RepresentativeRUTFprints {
		if _, err = tx.Exec(ctx, `INSERT INTO public.kyb_rut_claim(rut_kind,fingerprint,verification_id,organization_id) VALUES('representative',$1,$2,$3)`, fingerprint, item.ID, item.OrganizationID); err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.kyb_verification_event(verification_id,organization_id,actor_id,action,to_status)
		VALUES($1,$2,$3,'submitted','PENDING')`, item.ID, item.OrganizationID, item.SubmittedBy); err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	created, err := scanVerification(tx.QueryRow(ctx, verificationSelect+` WHERE v.id=$1`, item.ID))
	if err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	created.Documents = append([]kyb.Document(nil), documents...)
	if err = tx.Commit(ctx); err != nil {
		return kyb.Verification{}, false, mapError(err)
	}
	return created, false, nil
}

func (r *Repository) GetOwn(ctx context.Context, ownerID string) (kyb.Verification, error) {
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+` WHERE v.submitted_by=$1 ORDER BY v.created_at DESC,v.id DESC LIMIT 1`, ownerID))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	item.Documents, err = listDocuments(ctx, r.pool, item.ID)
	return item, mapError(err)
}

func (r *Repository) ListPending(ctx context.Context) ([]kyb.Verification, error) {
	rows, err := r.pool.Query(ctx, verificationSelect+` WHERE v.status IN ('PENDING','IN_REVIEW') ORDER BY v.submitted_at,v.id`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	items := make([]kyb.Verification, 0)
	for rows.Next() {
		item, scanErr := scanVerification(rows)
		if scanErr != nil {
			return nil, mapError(scanErr)
		}
		item.Documents, err = listDocuments(ctx, r.pool, item.ID)
		if err != nil {
			return nil, mapError(err)
		}
		items = append(items, item)
	}
	return items, mapError(rows.Err())
}

func (r *Repository) GetForReview(ctx context.Context, id string) (kyb.Verification, error) {
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+` WHERE v.id=$1`, id))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	item.Documents, err = listDocuments(ctx, r.pool, item.ID)
	return item, mapError(err)
}

func (r *Repository) GetDocument(ctx context.Context, verificationID, documentID string) (kyb.Document, kyb.Verification, error) {
	return r.getDocument(ctx, `d.verification_id=$1 AND d.id=$2`, verificationID, documentID)
}

func (r *Repository) GetDocumentByStorageRef(ctx context.Context, storageRef string) (kyb.Document, kyb.Verification, error) {
	return r.getDocument(ctx, `d.storage_ref=$1`, storageRef)
}

func (r *Repository) getDocument(ctx context.Context, predicate string, args ...any) (kyb.Document, kyb.Verification, error) {
	query := `SELECT d.id::text,d.verification_id::text,d.document_kind,d.mime_type,d.size_bytes,d.content_sha256,d.storage_ref::text,
		v.id::text,v.organization_id::text,o.usuario_id::text,v.submitted_by::text,v.legal_name,v.company_rut_fingerprint,
		v.company_rut_key_version,v.company_rut_ciphertext,v.representative_name,v.representative_rut_fingerprint,
		v.representative_rut_key_version,v.representative_rut_ciphertext,v.sii_giro_code,v.status,v.rejection_reason,
		v.submitted_at,v.reviewed_at,COALESCE(v.reviewer_id::text,''),v.created_at,v.updated_at,v.idempotency_key
		FROM public.kyb_verification_document d
		JOIN public.kyb_verifications v ON v.id=d.verification_id
		JOIN public.organizacion o ON o.id=v.organization_id
		WHERE ` + predicate
	var document kyb.Document
	var item kyb.Verification
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&document.ID, &document.VerificationID, &document.Kind, &document.MIME, &document.Size, &document.SHA256, &document.StorageRef,
		&item.ID, &item.OrganizationID, &item.OwnerID, &item.SubmittedBy, &item.LegalName, &item.CompanyRUTFingerprint,
		&item.CompanyRUTKeyVersion, &item.CompanyRUTCiphertext, &item.RepresentativeName, &item.RepresentativeRUTFprint,
		&item.RepresentativeRUTKey, &item.RepresentativeRUTCipher, &item.SIIGiroCode, &item.Status, &item.RejectionReason,
		&item.SubmittedAt, &item.ReviewedAt, &item.ReviewerID, &item.CreatedAt, &item.UpdatedAt, &item.IdempotencyKey)
	return document, item, mapError(err)
}

func (r *Repository) StartReview(ctx context.Context, id, reviewer string, now time.Time) (kyb.Verification, error) {
	return r.transition(ctx, id, reviewer, now, func(tx pgx.Tx, item kyb.Verification) (kyb.Status, string, string, error) {
		if item.Status != kyb.StatusPending {
			return "", "", "", kyb.ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE public.kyb_verifications SET status='IN_REVIEW',updated_at=$2 WHERE id=$1`, id, now); err != nil {
			return "", "", "", mapError(err)
		}
		return kyb.StatusInReview, "review_started", "", nil
	})
}

func (r *Repository) Decide(ctx context.Context, id, reviewer string, status kyb.Status, reason string, now time.Time) (kyb.Verification, error) {
	if status != kyb.StatusApproved && status != kyb.StatusRejected {
		return kyb.Verification{}, kyb.ErrInvalid
	}
	return r.transition(ctx, id, reviewer, now, func(tx pgx.Tx, item kyb.Verification) (kyb.Status, string, string, error) {
		if item.Status != kyb.StatusInReview {
			return "", "", "", kyb.ErrConflict
		}
		var rejection any
		if status == kyb.StatusRejected {
			rejection = reason
		}
		if _, err := tx.Exec(ctx, `UPDATE public.kyb_verifications SET status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5,updated_at=$5 WHERE id=$1`, id, status, rejection, reviewer, now); err != nil {
			return "", "", "", mapError(err)
		}
		return status, strings.ToLower(string(status)), reason, nil
	})
}

type transitionFunc func(pgx.Tx, kyb.Verification) (kyb.Status, string, string, error)

func (r *Repository) transition(ctx context.Context, id, reviewer string, now time.Time, apply transitionFunc) (kyb.Verification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var ownerID string
	if err = tx.QueryRow(ctx, `SELECT o.usuario_id::text FROM public.kyb_verifications v JOIN public.organizacion o ON o.id=v.organization_id WHERE v.id=$1`, id).Scan(&ownerID); err != nil {
		return kyb.Verification{}, mapError(err)
	}
	if err = lockActiveOwner(ctx, tx, ownerID); err != nil {
		return kyb.Verification{}, err
	}
	item, err := scanVerification(tx.QueryRow(ctx, verificationSelect+` WHERE v.id=$1 FOR UPDATE OF v`, id))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	toStatus, action, reason, err := apply(tx, item)
	if err != nil {
		return kyb.Verification{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.kyb_verification_event(verification_id,organization_id,actor_id,action,from_status,to_status,reason)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''))`, item.ID, item.OrganizationID, reviewer, action, item.Status, toStatus, reason); err != nil {
		return kyb.Verification{}, mapError(err)
	}
	updated, err := scanVerification(tx.QueryRow(ctx, verificationSelect+` WHERE v.id=$1`, id))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	updated.Documents, err = listDocuments(ctx, tx, id)
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return kyb.Verification{}, mapError(err)
	}
	return updated, nil
}

const verificationSelect = `SELECT v.id::text,v.organization_id::text,o.usuario_id::text,v.submitted_by::text,
	v.legal_name,v.company_rut_fingerprint,v.company_rut_key_version,v.company_rut_ciphertext,
	v.representative_name,v.representative_rut_fingerprint,v.representative_rut_key_version,
	v.representative_rut_ciphertext,v.sii_giro_code,v.status,COALESCE(v.rejection_reason,''),
	v.submitted_at,v.reviewed_at,COALESCE(v.reviewer_id::text,''),v.created_at,v.updated_at,v.idempotency_key
	FROM public.kyb_verifications v JOIN public.organizacion o ON o.id=v.organization_id`

type rowScanner interface{ Scan(...any) error }
type documentQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanVerification(row rowScanner) (kyb.Verification, error) {
	var item kyb.Verification
	err := row.Scan(&item.ID, &item.OrganizationID, &item.OwnerID, &item.SubmittedBy,
		&item.LegalName, &item.CompanyRUTFingerprint, &item.CompanyRUTKeyVersion, &item.CompanyRUTCiphertext,
		&item.RepresentativeName, &item.RepresentativeRUTFprint, &item.RepresentativeRUTKey,
		&item.RepresentativeRUTCipher, &item.SIIGiroCode, &item.Status, &item.RejectionReason,
		&item.SubmittedAt, &item.ReviewedAt, &item.ReviewerID, &item.CreatedAt, &item.UpdatedAt, &item.IdempotencyKey)
	return item, err
}

func listDocuments(ctx context.Context, queryer documentQueryer, verificationID string) ([]kyb.Document, error) {
	rows, err := queryer.Query(ctx, `SELECT id::text,verification_id::text,document_kind,mime_type,size_bytes,content_sha256,storage_ref::text
		FROM public.kyb_verification_document WHERE verification_id=$1 ORDER BY document_kind`, verificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]kyb.Document, 0, 3)
	for rows.Next() {
		var item kyb.Document
		if err := rows.Scan(&item.ID, &item.VerificationID, &item.Kind, &item.MIME, &item.Size, &item.SHA256, &item.StorageRef); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func lockActiveOwner(ctx context.Context, tx pgx.Tx, ownerID string) error {
	active, err := accountlock.LockActive(ctx, tx, ownerID)
	if err != nil {
		return mapError(err)
	}
	if !active {
		return kyb.ErrNotFound
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return kyb.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" || pgErr.Code == "23514" || pgErr.Code == "23P01" {
			return kyb.ErrConflict
		}
	}
	return err
}
