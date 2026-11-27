package kybpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	"github.com/jackc/pgx/v5"
)

const complianceDocumentSelect = `SELECT id::text,usuario_id::text,''::text,''::text,document_kind,status,mime_type,size_bytes,content_sha256,storage_ref::text,submitted_at,COALESCE(rejection_reason,''),usuario_id::text,''::text,''::text
	FROM public.anfitrion_cumplimiento_documento`

const spaceComplianceDocumentSelect = `SELECT id::text,owner_usuario_id::text,COALESCE(organizacion_id::text,''),espacio_id::text,document_kind,status,mime_type,size_bytes,content_sha256,document_storage_ref::text,submitted_at,COALESCE(rejection_reason,''),COALESCE(organizacion_id::text,owner_usuario_id::text),COALESCE(sii_giro_code,''),COALESCE(categoria_codigo,'')
	FROM public.space_kyb_verification`

func (r *Repository) ListPendingComplianceDocuments(ctx context.Context) ([]kyb.ComplianceDocument, error) {
	rows, err := r.pool.Query(ctx, spaceComplianceDocumentSelect+` WHERE status IN ('PENDING','IN_REVIEW')
		UNION ALL
		`+complianceDocumentSelect+` WHERE status IN ('PENDING','IN_REVIEW') ORDER BY submitted_at,id`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	items := make([]kyb.ComplianceDocument, 0)
	for rows.Next() {
		item, scanErr := scanComplianceDocument(rows)
		if scanErr != nil {
			return nil, mapError(scanErr)
		}
		items = append(items, item)
	}
	return items, mapError(rows.Err())
}

func (r *Repository) GetComplianceDocument(ctx context.Context, id string) (kyb.ComplianceDocument, error) {
	item, err := scanComplianceDocument(r.pool.QueryRow(ctx, complianceDocumentSelect+` WHERE id=$1`, id))
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return scanComplianceDocument(r.pool.QueryRow(ctx, spaceComplianceDocumentSelect+` WHERE id=$1`, id))
}

func (r *Repository) GetComplianceDocumentByStorageRef(ctx context.Context, ref string) (kyb.ComplianceDocument, error) {
	item, err := scanComplianceDocument(r.pool.QueryRow(ctx, complianceDocumentSelect+` WHERE storage_ref=$1`, ref))
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return scanComplianceDocument(r.pool.QueryRow(ctx, spaceComplianceDocumentSelect+` WHERE document_storage_ref=$1`, ref))
}

func scanComplianceDocument(row rowScanner) (kyb.ComplianceDocument, error) {
	var item kyb.ComplianceDocument
	var organizationID string
	err := row.Scan(&item.ID, &item.OwnerID, &organizationID, &item.SpaceID, &item.Kind, &item.Status, &item.MIME, &item.Size, &item.SHA256, &item.StorageRef, &item.SubmittedAt, &item.RejectionReason, &item.ScopeOrganizationID, &item.GiroCode, &item.CategoryCode)
	item.OrganizationID = organizationID
	return item, err
}

func (r *Repository) StartComplianceReview(ctx context.Context, id, reviewer string, now time.Time) (kyb.ComplianceDocument, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	item, err := startPersonalComplianceReview(ctx, tx, id, reviewer, now)
	if errors.Is(err, pgx.ErrNoRows) {
		item, err = startSpaceComplianceReview(ctx, tx, id, reviewer, now)
	}
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return item, nil
}

func startPersonalComplianceReview(ctx context.Context, tx pgx.Tx, id, reviewer string, now time.Time) (kyb.ComplianceDocument, error) {
	item, err := scanComplianceDocument(tx.QueryRow(ctx, `UPDATE public.anfitrion_cumplimiento_documento SET status='IN_REVIEW' WHERE id=$1 AND status='PENDING' RETURNING id::text,usuario_id::text,''::text,''::text,document_kind,status,mime_type,size_bytes,content_sha256,storage_ref::text,submitted_at,COALESCE(rejection_reason,''),usuario_id::text,''::text,''::text`, id))
	if err != nil {
		return item, err
	}
	_, err = tx.Exec(ctx, `UPDATE public.anfitrion_cumplimiento_personal SET sii_status='IN_REVIEW',updated_at=$2 WHERE usuario_id=$1`, item.OwnerID, now)
	if err != nil {
		return item, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(usuario_id,actor_usuario_id,event_type,occurred_at) VALUES($1,$2,'personal_sii_reviewed',$3)`, item.OwnerID, reviewer, now)
	return item, err
}

func startSpaceComplianceReview(ctx context.Context, tx pgx.Tx, id, reviewer string, now time.Time) (kyb.ComplianceDocument, error) {
	item, err := scanComplianceDocument(tx.QueryRow(ctx, `UPDATE public.space_kyb_verification SET status='IN_REVIEW' WHERE id=$1 AND status='PENDING' RETURNING id::text,owner_usuario_id::text,COALESCE(organizacion_id::text,''),espacio_id::text,document_kind,status,mime_type,size_bytes,content_sha256,document_storage_ref::text,submitted_at,COALESCE(rejection_reason,''),COALESCE(organizacion_id::text,owner_usuario_id::text),COALESCE(sii_giro_code,''),COALESCE(categoria_codigo,'')`, id))
	if err != nil {
		return item, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(espacio_id,actor_usuario_id,event_type,occurred_at) VALUES($1,$2,'space_reviewed',$3)`, item.SpaceID, reviewer, now)
	return item, err
}

func (r *Repository) DecideComplianceDocument(ctx context.Context, id, reviewer string, status kyb.Status, reason string, now time.Time) (kyb.ComplianceDocument, error) {
	if status != kyb.StatusApproved && status != kyb.StatusRejected {
		return kyb.ComplianceDocument{}, kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	item, err := decidePersonalComplianceDocument(ctx, tx, id, reviewer, status, reason, now)
	if errors.Is(err, pgx.ErrNoRows) {
		item, err = decideSpaceComplianceDocument(ctx, tx, id, reviewer, status, reason, now)
	}
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return item, nil
}

func decidePersonalComplianceDocument(ctx context.Context, tx pgx.Tx, id, reviewer string, status kyb.Status, reason string, now time.Time) (kyb.ComplianceDocument, error) {
	var reject any
	if status == kyb.StatusRejected {
		reject = reason
	}
	item, err := scanComplianceDocument(tx.QueryRow(ctx, `UPDATE public.anfitrion_cumplimiento_documento SET status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5 WHERE id=$1 AND status='IN_REVIEW' RETURNING id::text,usuario_id::text,''::text,''::text,document_kind,status,mime_type,size_bytes,content_sha256,storage_ref::text,submitted_at,COALESCE(rejection_reason,''),usuario_id::text,''::text,''::text`, id, status, reject, reviewer, now))
	if err != nil {
		return item, err
	}
	if status == kyb.StatusApproved {
		_, err = tx.Exec(ctx, `UPDATE public.anfitrion_cumplimiento_personal SET sii_status='APPROVED',compliance_status='compliant',disabled_at=NULL,disabled_reason=NULL,disabled_by=NULL,updated_at=$2 WHERE usuario_id=$1`, item.OwnerID, now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE public.anfitrion_cumplimiento_personal SET sii_status='REJECTED',compliance_status=CASE WHEN compliance_status='disabled_missing_docs' THEN compliance_status ELSE 'grace_active' END,updated_at=$2 WHERE usuario_id=$1`, item.OwnerID, now)
	}
	if err != nil {
		return item, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(usuario_id,actor_usuario_id,event_type,reason,occurred_at) VALUES($1,$2,'personal_sii_reviewed',NULLIF($3,''),$4)`, item.OwnerID, reviewer, reason, now)
	return item, err
}

func decideSpaceComplianceDocument(ctx context.Context, tx pgx.Tx, id, reviewer string, status kyb.Status, reason string, now time.Time) (kyb.ComplianceDocument, error) {
	var reject any
	if status == kyb.StatusRejected {
		reject = reason
	}
	item, err := scanComplianceDocument(tx.QueryRow(ctx, `UPDATE public.space_kyb_verification SET status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5 WHERE id=$1 AND status='IN_REVIEW' RETURNING id::text,owner_usuario_id::text,COALESCE(organizacion_id::text,''),espacio_id::text,document_kind,status,mime_type,size_bytes,content_sha256,document_storage_ref::text,submitted_at,COALESCE(rejection_reason,''),COALESCE(organizacion_id::text,owner_usuario_id::text),COALESCE(sii_giro_code,''),COALESCE(categoria_codigo,'')`, id, status, reject, reviewer, now))
	if err != nil {
		return item, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(espacio_id,actor_usuario_id,event_type,reason,occurred_at) VALUES($1,$2,'space_reviewed',NULLIF($3,''),$4)`, item.SpaceID, reviewer, reason, now)
	return item, err
}
