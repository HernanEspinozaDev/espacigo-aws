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

func (r *Repository) SubmitPersonalSII(ctx context.Context, item kyb.ComplianceDocument) (kyb.ComplianceDocument, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(item.ID) || !kyb.ValidUUID(item.OwnerID) || !kyb.ValidUUID(item.StorageRef) || item.Kind != kyb.DocumentSIIStartActivities || !strings.HasPrefix(item.MIME, "application/pdf") && item.MIME != "image/jpeg" && item.MIME != "image/png" || item.Size < 1 || item.Size > kyb.MaxDocumentBytes || len(item.SHA256) != 32 {
		return kyb.ComplianceDocument{}, kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = lockActiveOwner(ctx, tx, item.OwnerID); err != nil {
		return kyb.ComplianceDocument{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.anfitrion_cumplimiento_personal(usuario_id,sii_giro_code,compliance_status,sii_status)
		VALUES($1,$2,'in_review','PENDING') ON CONFLICT(usuario_id) DO UPDATE SET sii_giro_code=EXCLUDED.sii_giro_code,sii_status='PENDING',compliance_status=CASE WHEN anfitrion_cumplimiento_personal.compliance_status IN ('disabled_missing_docs','compliant') THEN anfitrion_cumplimiento_personal.compliance_status ELSE 'in_review' END`, item.OwnerID, item.GiroCode)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.anfitrion_cumplimiento_documento(id,usuario_id,document_kind,storage_ref,mime_type,size_bytes,content_sha256,status,submitted_by,submitted_at)
		VALUES($1,$2,'sii_start_activities',$3,$4,$5,$6,'PENDING',$2,$7)`, item.ID, item.OwnerID, item.StorageRef, item.MIME, item.Size, item.SHA256, item.SubmittedAt)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(usuario_id,event_type,occurred_at) VALUES($1,'personal_sii_submitted',$2)`, item.OwnerID, item.SubmittedAt)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return item, nil
}

func (r *Repository) PersonalCompliance(ctx context.Context, ownerID string) (kyb.ComplianceProfile, error) {
	var item kyb.ComplianceProfile
	err := r.pool.QueryRow(ctx, `SELECT usuario_id::text,compliance_status,sii_giro_code,sii_status,grace_started_at,grace_due_at,disabled_at,COALESCE(disabled_reason,'')
		FROM public.anfitrion_cumplimiento_personal WHERE usuario_id=$1`, ownerID).Scan(&item.OwnerID, &item.Status, &item.GiroCode, &item.SIIStatus, &item.GraceStartedAt, &item.GraceDueAt, &item.DisabledAt, &item.DisabledReason)
	return item, mapError(err)
}

// The canonical owner scopes storage. Company permissions come from the
// acting member even when the canonical owner is the same account.
const canManageSpaceDocuments = `(
		(e.organizacion_id IS NULL AND e.propietario_id=$2)
		OR EXISTS (
			SELECT 1 FROM public.organizacion_miembro m
			WHERE m.organizacion_id=e.organizacion_id AND m.usuario_id=$2 AND m.estado='activo'
			  AND m.rol IN ('representante_legal','administrador')
			  AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,now()) k WHERE k.verified)
		)
	)`

func (r *Repository) SubmitSpaceProperty(ctx context.Context, item kyb.ComplianceDocument) (kyb.ComplianceDocument, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(item.ID) || !kyb.ValidUUID(item.OwnerID) || !kyb.ValidUUID(item.SpaceID) || !kyb.ValidUUID(item.StorageRef) || item.Kind != kyb.DocumentPropertyOrSublease || item.Size < 1 || item.Size > kyb.MaxDocumentBytes || len(item.SHA256) != 32 {
		return kyb.ComplianceDocument{}, kyb.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var hostMode string
	var orgID *string
	err = tx.QueryRow(ctx, `SELECT e.modo_anfitrion,e.organizacion_id::text FROM public.espacio e WHERE e.id=$1 AND e.estado<>'eliminada' AND `+canManageSpaceDocuments+` AND e.propietario_id=$3 FOR UPDATE OF e`, item.SpaceID, item.SubmittedBy, item.OwnerID).Scan(&hostMode, &orgID)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.space_kyb_verification(id,espacio_id,host_mode,owner_usuario_id,organizacion_id,document_storage_ref,mime_type,size_bytes,content_sha256,status,submitted_by,submitted_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'PENDING',$10,$11)`, item.ID, item.SpaceID, hostMode, item.OwnerID, orgID, item.StorageRef, item.MIME, item.Size, item.SHA256, item.SubmittedBy, item.SubmittedAt)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(espacio_id,actor_usuario_id,event_type,occurred_at) VALUES($1,$2,'space_property_submitted',$3)`, item.SpaceID, item.SubmittedBy, item.SubmittedAt)
	if err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return kyb.ComplianceDocument{}, mapError(err)
	}
	return item, nil
}

func (r *Repository) SpaceStorageScope(ctx context.Context, actorID, spaceID string) (string, string, error) {
	var ownerID string
	var organizationID *string
	err := r.pool.QueryRow(ctx, `SELECT e.propietario_id::text,e.organizacion_id::text FROM public.espacio e WHERE e.id=$1 AND e.estado<>'eliminada' AND `+canManageSpaceDocuments, spaceID, actorID).Scan(&ownerID, &organizationID)
	if err != nil {
		return "", "", mapError(err)
	}
	if organizationID == nil {
		return ownerID, "", nil
	}
	return ownerID, *organizationID, nil
}

func (r *Repository) SpaceEligibility(ctx context.Context, ownerID, spaceID string, at time.Time) (kyb.SpaceEligibility, error) {
	var item kyb.SpaceEligibility
	err := r.pool.QueryRow(ctx, `SELECT q.can_publish,q.can_book,q.space_verified,q.compliance_status,
		COALESCE(d.status,''),COALESCE(o.grace_due_at,p.grace_due_at)
		FROM public.espacio e
		CROSS JOIN LATERAL public.kyb_space_eligibility(e.id,$3) q
		LEFT JOIN public.organizacion o ON o.id=e.organizacion_id
		LEFT JOIN public.anfitrion_cumplimiento_personal p ON e.organizacion_id IS NULL AND p.usuario_id=e.propietario_id
		LEFT JOIN LATERAL (
			SELECT v.status FROM public.space_kyb_verification v WHERE v.espacio_id=e.id
			ORDER BY v.submitted_at DESC,v.id DESC LIMIT 1
		) d ON true
		WHERE e.id=$1 AND e.estado<>'eliminada' AND (
			(e.organizacion_id IS NULL AND e.propietario_id=$2)
			OR EXISTS (SELECT 1 FROM public.organizacion_miembro m
				WHERE m.organizacion_id=e.organizacion_id AND m.usuario_id=$2 AND m.estado='activo'
				  AND EXISTS (SELECT 1 FROM public.account_profile_identity_status(m.usuario_id,$3) k WHERE k.verified))
		)`, spaceID, ownerID, at).Scan(&item.CanPublish, &item.CanBook, &item.SpaceVerified, &item.ComplianceStatus, &item.VerificationStatus, &item.GraceDueAt)
	return item, mapError(err)
}

func (r *Repository) ListCompliance(ctx context.Context, filter string) ([]kyb.ComplianceProfile, error) {
	rows, err := r.pool.Query(ctx, `SELECT owner_usuario_id,organization_id,legal_name,compliance_status,sii_giro_code,sii_status,grace_started_at,grace_due_at,disabled_at,disabled_reason FROM (
		SELECT p.usuario_id::text owner_usuario_id,NULL::text organization_id,NULL::text legal_name,p.compliance_status,p.sii_giro_code,p.sii_status,
		p.grace_started_at,p.grace_due_at,p.disabled_at,COALESCE(p.disabled_reason,'') disabled_reason
		FROM public.anfitrion_cumplimiento_personal p
		UNION ALL
		SELECT o.created_by_usuario_id::text,o.id::text,o.legal_name,o.compliance_status,o.sii_giro_code,
		COALESCE((SELECT v.status FROM public.kyb_verifications v WHERE v.organization_id=o.id ORDER BY v.created_at DESC LIMIT 1),'MISSING'),
		o.grace_started_at,o.grace_due_at,o.disabled_at,COALESCE(o.disabled_reason,'')
		FROM public.organizacion o
	) profiles WHERE ($1='' OR compliance_status=$1) ORDER BY grace_due_at NULLS LAST,organization_id NULLS FIRST,owner_usuario_id`, filter)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	items := make([]kyb.ComplianceProfile, 0)
	for rows.Next() {
		var item kyb.ComplianceProfile
		if err = rows.Scan(&item.OwnerID, &item.OrganizationID, &item.LegalName, &item.Status, &item.GiroCode, &item.SIIStatus, &item.GraceStartedAt, &item.GraceDueAt, &item.DisabledAt, &item.DisabledReason); err != nil {
			return nil, mapError(err)
		}
		items = append(items, item)
	}
	return items, mapError(rows.Err())
}

func (r *Repository) SetComplianceDisabled(ctx context.Context, ownerID, actorID, reason string, now time.Time) error {
	var err error
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = lockActiveOwner(ctx, tx, ownerID); err != nil {
		return err
	}
	if reason == "" {
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE public.anfitrion_cumplimiento_personal SET compliance_status=CASE WHEN sii_status='APPROVED' THEN 'compliant' ELSE 'grace_active' END,disabled_at=NULL,disabled_reason=NULL,disabled_by=NULL WHERE usuario_id=$1`, ownerID)
		if err == nil && tag.RowsAffected() == 0 {
			return kyb.ErrNotFound
		}
	} else {
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE public.anfitrion_cumplimiento_personal SET compliance_status='disabled_missing_docs',disabled_at=$2,disabled_reason=$3,disabled_by=$4 WHERE usuario_id=$1`, ownerID, now, reason, actorID)
		if err == nil && tag.RowsAffected() == 0 {
			return kyb.ErrNotFound
		}
	}
	if err != nil {
		return mapError(err)
	}
	event := "re_enabled"
	if reason != "" {
		event = "disabled_manual"
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(usuario_id,actor_usuario_id,event_type,reason,occurred_at) VALUES($1,$2,$3,NULLIF($4,''),$5)`, ownerID, actorID, event, reason, now)
	if err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

func (r *Repository) SetOrganizationDisabled(ctx context.Context, organizationID, actorID, reason string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var existingID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM public.organizacion WHERE id=$1 FOR UPDATE`, organizationID).Scan(&existingID); err != nil {
		return mapError(err)
	}
	if reason == "" {
		_, err = tx.Exec(ctx, `UPDATE public.organizacion SET compliance_status=CASE WHEN EXISTS(SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=$1 AND v.status='APPROVED' AND (SELECT count(DISTINCT d.document_kind) FROM public.kyb_verification_document d WHERE d.verification_id=v.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3) THEN 'compliant' ELSE 'grace_active' END,disabled_at=NULL,disabled_reason=NULL,disabled_by=NULL,updated_at=$2 WHERE id=$1`, organizationID, now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE public.organizacion SET compliance_status='disabled_missing_docs',disabled_at=$2,disabled_reason=$3,disabled_by=$4,updated_at=$2 WHERE id=$1`, organizationID, now, reason, actorID)
	}
	if err != nil {
		return mapError(err)
	}
	event := "re_enabled"
	if reason != "" {
		event = "disabled_manual"
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.host_compliance_event(organizacion_id,actor_usuario_id,event_type,reason,occurred_at) VALUES($1,$2,$3,NULLIF($4,''),$5)`, organizationID, actorID, event, reason, now)
	if err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

func (r *Repository) ExpireNoncompliant(ctx context.Context, now time.Time) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT usuario_id::text,organizacion_id::text FROM public.kyb_expire_noncompliant_profiles($1)`, now)
	if err != nil {
		return 0, mapError(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var userID, organizationID *string
		if err = rows.Scan(&userID, &organizationID); err != nil {
			return count, mapError(err)
		}
		count++
	}
	return count, mapError(rows.Err())
}

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
	var organizationID string
	err = tx.QueryRow(ctx, `SELECT o.id::text FROM public.organizacion o JOIN public.organizacion_miembro m ON m.organizacion_id=o.id WHERE m.usuario_id=$1 AND m.estado='activo' AND m.rol='representante_legal' ORDER BY o.created_at,o.id LIMIT 1`, ownerID).Scan(&organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, `INSERT INTO public.organizacion(id,usuario_id,created_by_usuario_id) VALUES($1,$2,$2)`, candidateID, ownerID); err != nil {
			return "", mapError(err)
		}
		organizationID = candidateID
		if _, err = tx.Exec(ctx, `INSERT INTO public.organizacion_miembro(organizacion_id,usuario_id,rol,estado,joined_at) VALUES($1,$2,'representante_legal','activo',clock_timestamp())`, organizationID, ownerID); err != nil {
			return "", mapError(err)
		}
	} else if err != nil {
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
	var memberID string
	if err = tx.QueryRow(ctx, `SELECT m.usuario_id::text FROM public.organizacion o JOIN public.organizacion_miembro m ON m.organizacion_id=o.id WHERE o.id=$1 AND m.usuario_id=$2 AND m.estado='activo' AND m.rol='representante_legal' FOR UPDATE OF o,m`, item.OrganizationID, item.OwnerID).Scan(&memberID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return kyb.Verification{}, false, kyb.ErrForbidden
		}
		return kyb.Verification{}, false, mapError(err)
	}
	if memberID != item.OwnerID {
		return kyb.Verification{}, false, kyb.ErrForbidden
	}
	if hasCompleteCompanyPacket(documents) {
		var matched bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM public.account_identity_rut_fingerprint f
			JOIN public.account_identity_rut_claim c ON c.account_id=f.account_id
			JOIN public.account_identity_profile p ON p.account_id=c.account_id AND p.current_revision=c.identity_revision
			CROSS JOIN LATERAL public.account_profile_identity_status(p.account_id,$3) k
			WHERE p.account_id=$1 AND f.fingerprint=ANY($2::text[]) AND k.verified
		)`, item.OwnerID, item.IdentityRUTFingerprints, item.SubmittedAt).Scan(&matched)
		if err != nil {
			return kyb.Verification{}, false, mapError(err)
		}
		if !matched {
			return kyb.Verification{}, false, kyb.ErrForbidden
		}
	}
	// Reuse an idempotent submission only while it is still being processed.
	// A rejected case is not replayed; corrected submissions use a fresh key.
	prior, err := scanVerification(tx.QueryRow(ctx, verificationSelect+` WHERE v.submitted_by=$1 AND v.idempotency_key=$2 AND v.status IN ('PENDING','IN_REVIEW','APPROVED') ORDER BY v.created_at DESC LIMIT 1`, item.OwnerID, item.IdempotencyKey))
	if err == nil {
		if prior.OrganizationID != item.OrganizationID {
			return kyb.Verification{}, false, kyb.ErrConflict
		}

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
	if err = bindCompanyRUTFingerprints(ctx, tx, item.OrganizationID, item.CompanyRUTFingerprints); err != nil {
		return kyb.Verification{}, false, err
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
	if hasCompleteCompanyPacket(documents) {
		_, err = tx.Exec(ctx, `UPDATE public.organizacion SET sii_giro_code=$2,compliance_status=CASE WHEN compliance_status IN ('disabled_missing_docs','compliant') THEN compliance_status ELSE 'in_review' END,updated_at=$3 WHERE id=$1`, item.OrganizationID, item.SIIGiroCode, item.SubmittedAt)
		if err != nil {
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

func hasCompleteCompanyPacket(documents []kyb.Document) bool {
	seen := map[kyb.DocumentKind]bool{}
	for _, doc := range documents {
		seen[doc.Kind] = true
	}
	return seen[kyb.DocumentERUT] && seen[kyb.DocumentSIIStartActivities] && seen[kyb.DocumentPowerCertificate]
}

func bindCompanyRUTFingerprints(ctx context.Context, tx pgx.Tx, organizationID string, fingerprints []string) error {
	if len(fingerprints) == 0 {
		return kyb.ErrInvalid
	}
	var existing int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.organizacion_rut_fingerprint WHERE organizacion_id=$1`, organizationID).Scan(&existing); err != nil {
		return mapError(err)
	}
	if existing > 0 {
		var overlaps bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.organizacion_rut_fingerprint WHERE organizacion_id=$1 AND fingerprint=ANY($2::varchar[]))`, organizationID, fingerprints).Scan(&overlaps); err != nil {
			return mapError(err)
		}
		if !overlaps {
			return kyb.ErrConflict
		}
	}
	for _, fingerprint := range fingerprints {
		if _, err := tx.Exec(ctx, `INSERT INTO public.organizacion_rut_fingerprint(organizacion_id,fingerprint) VALUES($1,$2) ON CONFLICT(organizacion_id,fingerprint) DO NOTHING`, organizationID, fingerprint); err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (r *Repository) GetOwn(ctx context.Context, ownerID string) (kyb.Verification, error) {
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+` WHERE v.submitted_by=$1 AND EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=v.organization_id AND m.usuario_id=$1 AND m.estado='activo') ORDER BY v.created_at DESC,v.id DESC LIMIT 1`, ownerID))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	item.Documents, err = listDocuments(ctx, r.pool, item.ID)
	return item, mapError(err)
}

func (r *Repository) GetOwnForOrganization(ctx context.Context, ownerID, organizationID string) (kyb.Verification, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(ownerID) || !kyb.ValidUUID(organizationID) {
		return kyb.Verification{}, kyb.ErrInvalid
	}
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+` WHERE v.organization_id=$1 AND EXISTS (
		SELECT 1 FROM public.organizacion_miembro m
		CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,clock_timestamp()) k
		WHERE m.organizacion_id=v.organization_id AND m.usuario_id=$2 AND m.estado='activo'
		  AND m.rol IN ('representante_legal','administrador') AND k.verified
	) ORDER BY v.created_at DESC,v.id DESC LIMIT 1`, organizationID, ownerID))
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	item.Documents, err = listDocuments(ctx, r.pool, item.ID)
	return item, mapError(err)
}

// GetContractIdentity returns only the current approved identity for an active
// legal representative. The service decrypts the RUTs only after role checks.
func (r *Repository) GetContractIdentity(ctx context.Context, actorID, organizationID string, asOf time.Time) (kyb.Verification, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(actorID) || !kyb.ValidUUID(organizationID) {
		return kyb.Verification{}, kyb.ErrInvalid
	}
	var verified bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.verificacion WHERE usuario_id=$1 AND tipo='kyc' AND estado='aprobada' AND revocada_en IS NULL)`, actorID).Scan(&verified); err != nil {
		return kyb.Verification{}, mapError(err)
	}
	if !verified {
		return kyb.Verification{}, kyb.ErrForbidden
	}
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+` JOIN public.organizacion_miembro m ON m.organizacion_id=o.id AND m.usuario_id=$1 AND m.estado='activo' AND m.rol='representante_legal' WHERE v.organization_id=$2 AND v.status='APPROVED' AND v.reviewed_at <= $3 ORDER BY v.reviewed_at DESC,v.id DESC LIMIT 1`, actorID, organizationID, asOf))
	return item, mapError(err)
}

// GetProposalContractIdentity resolves a verified representative without
// granting the proposal requester access to either full RUT. The values are
// decrypted only by kyb.Service and are immediately masked by the caller.
func (r *Repository) GetProposalContractIdentity(ctx context.Context, organizationID string, asOf time.Time) (kyb.Verification, string, error) {
	if r == nil || r.pool == nil || !kyb.ValidUUID(organizationID) || asOf.IsZero() {
		return kyb.Verification{}, "", kyb.ErrInvalid
	}
	item, err := scanVerification(r.pool.QueryRow(ctx, verificationSelect+`
		WHERE v.id=(
			SELECT approved.id FROM public.kyb_verifications approved
			WHERE approved.organization_id=$1 AND approved.status='APPROVED' AND approved.reviewed_at<=$2
			  AND (SELECT count(*) FROM public.kyb_verification_document d
				WHERE d.verification_id=approved.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3
			ORDER BY approved.reviewed_at DESC,approved.id DESC LIMIT 1
		)
		AND EXISTS (
			SELECT 1 FROM public.organizacion_miembro m
			CROSS JOIN LATERAL public.account_profile_identity_status(m.usuario_id,$2) k
			WHERE m.organizacion_id=v.organization_id AND m.usuario_id=v.submitted_by
			  AND m.rol='representante_legal' AND m.estado='activo' AND k.verified
		)`, organizationID, asOf.UTC()))
	return item, item.SubmittedBy, mapError(err)
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
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, mapError(err)
	}
	rows.Close()
	for index := range items {
		items[index].Documents, err = listDocuments(ctx, r.pool, items[index].ID)
		if err != nil {
			return nil, mapError(err)
		}
	}
	return items, nil
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
		d.review_status,COALESCE(d.rejection_reason,''),d.reviewed_at,COALESCE(d.reviewer_id::text,''),
		v.id::text,v.organization_id::text,v.submitted_by::text,v.submitted_by::text,v.legal_name,v.company_rut_fingerprint,
		v.company_rut_key_version,v.company_rut_ciphertext,v.representative_name,v.representative_rut_fingerprint,
		v.representative_rut_key_version,v.representative_rut_ciphertext,v.sii_giro_code,v.status,COALESCE(v.rejection_reason,''),
		v.submitted_at,v.reviewed_at,COALESCE(v.reviewer_id::text,''),v.created_at,v.updated_at,v.idempotency_key
		FROM public.kyb_verification_document d
		JOIN public.kyb_verifications v ON v.id=d.verification_id
		JOIN public.organizacion o ON o.id=v.organization_id
		WHERE ` + predicate
	var document kyb.Document
	var item kyb.Verification
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&document.ID, &document.VerificationID, &document.Kind, &document.MIME, &document.Size, &document.SHA256, &document.StorageRef,
		&document.ReviewStatus,&document.RejectionReason,&document.ReviewedAt,&document.ReviewerID,
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
		if _,err:=tx.Exec(ctx,`UPDATE public.kyb_verification_document SET review_status='IN_REVIEW' WHERE verification_id=$1 AND review_status='PENDING'`,id);err!=nil{return "","","",mapError(err)}
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
		if _,err:=tx.Exec(ctx,`UPDATE public.kyb_verification_document SET review_status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5 WHERE verification_id=$1`,id,status,rejection,reviewer,now);err!=nil{return "","","",mapError(err)}
		if status == kyb.StatusRejected {
			if _, err := tx.Exec(ctx, `UPDATE public.kyb_rut_claim SET active=false WHERE verification_id=$1`, id); err != nil {
				return "", "", "", mapError(err)
			}
			if hasCompleteCompanyPacketFromDB(ctx, tx, id) {
				if _, err := tx.Exec(ctx, `UPDATE public.organizacion o SET compliance_status=CASE WHEN o.compliance_status='disabled_missing_docs' THEN o.compliance_status WHEN EXISTS(SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.id<>$2 AND v.status='APPROVED' AND (SELECT count(DISTINCT d.document_kind) FROM public.kyb_verification_document d WHERE d.verification_id=v.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3) THEN 'compliant' ELSE 'grace_active' END,updated_at=$3 WHERE o.id=$1`, item.OrganizationID, id, now); err != nil {
					return "", "", "", mapError(err)
				}
			}
		} else if hasCompleteCompanyPacketFromDB(ctx, tx, id) {
			if _, err := tx.Exec(ctx, `UPDATE public.organizacion SET compliance_status='compliant',sii_giro_code=$2,disabled_at=NULL,disabled_reason=NULL,disabled_by=NULL,updated_at=$3 WHERE id=$1`, item.OrganizationID, item.SIIGiroCode, now); err != nil {
				return "", "", "", mapError(err)
			}
		}
		return status, strings.ToLower(string(status)), reason, nil
	})
}

func(r *Repository) DecideDocuments(ctx context.Context,id,reviewer string,status kyb.Status,reason string,decisions []kyb.DocumentDecision,now time.Time)(kyb.Verification,error){
	if status!=kyb.StatusApproved&&status!=kyb.StatusRejected||len(decisions)==0{return kyb.Verification{},kyb.ErrInvalid}
	return r.transition(ctx,id,reviewer,now,func(tx pgx.Tx,item kyb.Verification)(kyb.Status,string,string,error){
		if item.Status!=kyb.StatusInReview{return "","","",kyb.ErrConflict}
		rows,err:=tx.Query(ctx,`SELECT id::text FROM public.kyb_verification_document WHERE verification_id=$1 FOR UPDATE`,id);if err!=nil{return "","","",mapError(err)}
		loaded:=map[string]bool{};for rows.Next(){var documentID string;if err=rows.Scan(&documentID);err!=nil{rows.Close();return "","","",mapError(err)};loaded[documentID]=true};rowsErr:=rows.Err();rows.Close();if rowsErr!=nil{return "","","",mapError(rowsErr)}
		if len(loaded)!=len(decisions){return "","","",kyb.ErrInvalid}
		seen:=map[string]bool{}
		for _,decision:=range decisions{if !loaded[decision.DocumentID]||seen[decision.DocumentID]||decision.Status!=kyb.StatusApproved&&decision.Status!=kyb.StatusRejected{return "","","",kyb.ErrInvalid};seen[decision.DocumentID]=true
			var rejection any;if decision.Status==kyb.StatusRejected{rejection=decision.RejectionReason}
			result,updateErr:=tx.Exec(ctx,`UPDATE public.kyb_verification_document SET review_status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5 WHERE id=$1 AND verification_id=$6 AND review_status='IN_REVIEW'`,decision.DocumentID,decision.Status,rejection,reviewer,now,id);if updateErr!=nil{return "","","",mapError(updateErr)};if result.RowsAffected()!=1{return "","","",kyb.ErrConflict}
		}
		var packetRejection any;if status==kyb.StatusRejected{packetRejection=reason}
		if _,err=tx.Exec(ctx,`UPDATE public.kyb_verifications SET status=$2,rejection_reason=$3,reviewer_id=$4,reviewed_at=$5,updated_at=$5 WHERE id=$1`,id,status,packetRejection,reviewer,now);err!=nil{return "","","",mapError(err)}
		if status==kyb.StatusRejected{if _,err=tx.Exec(ctx,`UPDATE public.kyb_rut_claim SET active=false WHERE verification_id=$1`,id);err!=nil{return "","","",mapError(err)}
			if hasCompleteCompanyPacketFromDB(ctx,tx,id){if _,err=tx.Exec(ctx,`UPDATE public.organizacion o SET compliance_status=CASE WHEN o.compliance_status='disabled_missing_docs' THEN o.compliance_status WHEN EXISTS(SELECT 1 FROM public.kyb_verifications v WHERE v.organization_id=o.id AND v.id<>$2 AND v.status='APPROVED' AND (SELECT count(DISTINCT d.document_kind) FROM public.kyb_verification_document d WHERE d.verification_id=v.id AND d.document_kind IN ('erut','sii_start_activities','power_certificate'))=3) THEN 'compliant' ELSE 'grace_active' END,updated_at=$3 WHERE o.id=$1`,item.OrganizationID,id,now);err!=nil{return "","","",mapError(err)}}
		}else if hasCompleteCompanyPacketFromDB(ctx,tx,id){if _,err=tx.Exec(ctx,`UPDATE public.organizacion SET compliance_status='compliant',sii_giro_code=$2,disabled_at=NULL,disabled_reason=NULL,disabled_by=NULL,updated_at=$3 WHERE id=$1`,item.OrganizationID,item.SIIGiroCode,now);err!=nil{return "","","",mapError(err)}}
		return status,strings.ToLower(string(status)),reason,nil
	})
}

func hasCompleteCompanyPacketFromDB(ctx context.Context, tx pgx.Tx, verificationID string) bool {
	var complete bool
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT document_kind)=3 FROM public.kyb_verification_document WHERE verification_id=$1 AND document_kind IN ('erut','sii_start_activities','power_certificate')`, verificationID).Scan(&complete); err != nil {
		return false
	}
	return complete
}

type transitionFunc func(pgx.Tx, kyb.Verification) (kyb.Status, string, string, error)

func (r *Repository) transition(ctx context.Context, id, reviewer string, now time.Time, apply transitionFunc) (kyb.Verification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return kyb.Verification{}, mapError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var ownerID string
	if err = tx.QueryRow(ctx, `SELECT v.submitted_by::text FROM public.kyb_verifications v WHERE v.id=$1`, id).Scan(&ownerID); err != nil {
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

const verificationSelect = `SELECT v.id::text,v.organization_id::text,v.submitted_by::text,v.submitted_by::text,
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
	rows, err := queryer.Query(ctx, `SELECT id::text,verification_id::text,document_kind,mime_type,size_bytes,content_sha256,storage_ref::text,
		review_status,COALESCE(rejection_reason,''),reviewed_at,COALESCE(reviewer_id::text,'')
		FROM public.kyb_verification_document WHERE verification_id=$1 ORDER BY document_kind`, verificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]kyb.Document, 0, 3)
	for rows.Next() {
		var item kyb.Document
		if err := rows.Scan(&item.ID, &item.VerificationID, &item.Kind, &item.MIME, &item.Size, &item.SHA256, &item.StorageRef,&item.ReviewStatus,&item.RejectionReason,&item.ReviewedAt,&item.ReviewerID); err != nil {
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
