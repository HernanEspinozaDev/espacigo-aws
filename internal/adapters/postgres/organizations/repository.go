package organizationspg

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/organizations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) valid() bool        { return r != nil && r.pool != nil }
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return organizations.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505", "23P01":
			return organizations.ErrConflict
		case "23503", "23514", "22P02":
			return organizations.ErrInvalid
		}
	}
	return err
}
func lastRepresentativeConflict(role string, count int) bool {
	return role == string(organizations.LegalRepresentative) && count <= 1
}

const orgSelect = `SELECT o.id::text,COALESCE(o.legal_name,''),COALESCE(o.trade_name,''),COALESCE(o.public_summary,''),o.created_at,m.rol,o.compliance_status,o.grace_due_at FROM public.organizacion o JOIN public.organizacion_miembro m ON m.organizacion_id=o.id AND m.usuario_id=$1 AND m.estado='activo'`

func (r *Repository) ListMine(ctx context.Context, user string) ([]organizations.Organization, error) {
	if !r.valid() {
		return nil, organizations.ErrInvalid
	}
	rows, err := r.pool.Query(ctx, orgSelect+` ORDER BY o.created_at DESC,o.id`, user)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	items := []organizations.Organization{}
	for rows.Next() {
		var o organizations.Organization
		if err = rows.Scan(&o.ID, &o.LegalName, &o.TradeName, &o.PublicSummary, &o.CreatedAt, &o.MyRole, &o.ComplianceStatus, &o.GraceDueAt); err != nil {
			return nil, mapErr(err)
		}
		items = append(items, o)
	}
	return items, mapErr(rows.Err())
}
func (r *Repository) Create(ctx context.Context, user, legal, trade, activeFingerprint string, fingerprints []string) (organizations.Organization, error) {
	if !r.valid() {
		return organizations.Organization{}, organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return organizations.Organization{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var verified bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,clock_timestamp()) k WHERE k.verified)`, user).Scan(&verified); err != nil {
		return organizations.Organization{}, mapErr(err)
	}
	if !verified {
		return organizations.Organization{}, organizations.ErrForbidden
	}
	var o organizations.Organization
	err = tx.QueryRow(ctx, `INSERT INTO public.organizacion(usuario_id,created_by_usuario_id,legal_name,trade_name,company_rut_fingerprint) VALUES($1,$1,$2,$3,$4) RETURNING id::text,legal_name,COALESCE(trade_name,''),COALESCE(public_summary,''),created_at,compliance_status,grace_due_at`, user, legal, trade, activeFingerprint).Scan(&o.ID, &o.LegalName, &o.TradeName, &o.PublicSummary, &o.CreatedAt, &o.ComplianceStatus, &o.GraceDueAt)
	if err != nil {
		return o, mapErr(err)
	}
	for _, fingerprint := range fingerprints {
		if _, err = tx.Exec(ctx, `INSERT INTO public.organizacion_rut_fingerprint(organizacion_id,fingerprint) VALUES($1,$2)`, o.ID, fingerprint); err != nil {
			return o, mapErr(err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.organizacion_miembro(organizacion_id,usuario_id,rol,estado,joined_at) VALUES($1,$2,'representante_legal','activo',clock_timestamp())`, o.ID, user)
	if err != nil {
		return o, mapErr(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return o, mapErr(err)
	}
	o.MyRole = organizations.LegalRepresentative
	return o, nil
}
func (r *Repository) Get(ctx context.Context, user, id string) (organizations.Organization, error) {
	if !r.valid() {
		return organizations.Organization{}, organizations.ErrInvalid
	}
	var o organizations.Organization
	err := r.pool.QueryRow(ctx, orgSelect+` WHERE o.id=$2`, user, id).Scan(&o.ID, &o.LegalName, &o.TradeName, &o.PublicSummary, &o.CreatedAt, &o.MyRole, &o.ComplianceStatus, &o.GraceDueAt)
	return o, mapErr(err)
}
func (r *Repository) Update(ctx context.Context, user, id, legal, trade, publicSummary string) (organizations.Organization, error) {
	if !r.valid() {
		return organizations.Organization{}, organizations.ErrInvalid
	}
	var o organizations.Organization
	err := r.pool.QueryRow(ctx, `UPDATE public.organizacion o SET trade_name=$4,public_summary=$5,updated_at=clock_timestamp() FROM public.organizacion_miembro m WHERE o.id=$2 AND m.organizacion_id=o.id AND m.usuario_id=$1 AND m.estado='activo' AND m.rol='representante_legal' AND o.legal_name=$3 RETURNING o.id::text,o.legal_name,COALESCE(o.trade_name,''),COALESCE(o.public_summary,''),o.created_at,m.rol,o.compliance_status,o.grace_due_at`, user, id, legal, trade, publicSummary).Scan(&o.ID, &o.LegalName, &o.TradeName, &o.PublicSummary, &o.CreatedAt, &o.MyRole, &o.ComplianceStatus, &o.GraceDueAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var role string
		checkErr := r.pool.QueryRow(ctx, `SELECT rol FROM public.organizacion_miembro WHERE organizacion_id=$1 AND usuario_id=$2 AND estado='activo' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,clock_timestamp()) k WHERE k.verified)`, id, user).Scan(&role)
		if checkErr == nil && role == string(organizations.LegalRepresentative) {
			return o, organizations.ErrConflict
		}
		if checkErr == nil {
			return o, organizations.ErrForbidden
		}
	}
	return o, mapErr(err)
}
func (r *Repository) Members(ctx context.Context, user, id string) ([]organizations.Member, error) {
	if !r.valid() {
		return nil, organizations.ErrInvalid
	}
	if _, err := r.Role(ctx, user, id); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text,usuario_id::text,rol,estado,joined_at FROM public.organizacion_miembro WHERE organizacion_id=$1 AND estado='activo' ORDER BY rol,joined_at,id`, id)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []organizations.Member{}
	for rows.Next() {
		var m organizations.Member
		if err = rows.Scan(&m.ID, &m.UserID, &m.Role, &m.State, &m.JoinedAt); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, m)
	}
	return out, mapErr(rows.Err())
}
func (r *Repository) Invite(ctx context.Context, actor, id, email string, role organizations.Role, token string, hash []byte, expires time.Time) (organizations.Invitation, error) {
	if !r.valid() {
		return organizations.Invitation{}, organizations.ErrInvalid
	}
	if _, err := r.Role(ctx, actor, id); err != nil {
		return organizations.Invitation{}, err
	}
	var inv organizations.Invitation
	err := r.pool.QueryRow(ctx, `INSERT INTO public.organizacion_invitacion(organizacion_id,tipo,target_email_normalized,rol_propuesto,token_sha256,estado,expires_at,created_by) SELECT $2,'invitacion',$3,$4,$5,'pendiente',$6,$1 WHERE EXISTS(SELECT 1 FROM public.organizacion_miembro WHERE organizacion_id=$2 AND usuario_id=$1 AND estado='activo' AND rol='representante_legal' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,clock_timestamp()) k WHERE k.verified)) RETURNING id::text,organizacion_id::text,target_email_normalized,rol_propuesto,estado,expires_at,created_at`, actor, id, email, role, hash, expires).Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.State, &inv.ExpiresAt, &inv.CreatedAt)
	return inv, mapErr(err)
}
func (r *Repository) Accept(ctx context.Context, user, id, invitationID string, hash []byte, now time.Time) (organizations.Member, error) {
	if !r.valid() {
		return organizations.Member{}, organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return organizations.Member{}, mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var invID, org string
	err = tx.QueryRow(ctx, `SELECT i.id::text,i.organizacion_id::text FROM public.organizacion_invitacion i JOIN public.usuario u ON u.id=$4 AND u.estado='activo' AND (i.target_usuario_id=u.id OR lower(u.correo_normalizado)=i.target_email_normalized) WHERE i.id=$5 AND i.organizacion_id=$1 AND i.token_sha256=$2 AND i.estado='pendiente' AND i.expires_at>$3 FOR UPDATE OF i`, id, hash, now, user, invitationID).Scan(&invID, &org)
	if err != nil {
		return organizations.Member{}, mapErr(err)
	}
	var kyc bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,clock_timestamp()) k WHERE k.verified)`, user).Scan(&kyc)
	if err != nil {
		return organizations.Member{}, mapErr(err)
	}
	if !kyc {
		return organizations.Member{}, organizations.ErrForbidden
	}
	var m organizations.Member
	err = tx.QueryRow(ctx, `INSERT INTO public.organizacion_miembro(organizacion_id,usuario_id,rol,estado,invited_by,joined_at) SELECT organizacion_id,$2,rol_propuesto,'activo',created_by,$3 FROM public.organizacion_invitacion WHERE id=$1 RETURNING id::text,usuario_id::text,rol,estado,joined_at`, invID, user, now).Scan(&m.ID, &m.UserID, &m.Role, &m.State, &m.JoinedAt)
	if err != nil {
		return m, mapErr(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE public.organizacion_invitacion SET estado='aceptada',resolved_by=$2,resolved_at=$3 WHERE id=$1`, invID, user, now); err != nil {
		return m, mapErr(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return m, mapErr(err)
	}
	return m, nil
}
func (r *Repository) Revoke(ctx context.Context, actor, id, member string, now time.Time) error {
	if !r.valid() {
		return organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var role string
	err = tx.QueryRow(ctx, `SELECT m.rol FROM public.organizacion_miembro m WHERE m.id=$1 AND m.organizacion_id=$2 AND m.estado='activo' AND EXISTS(SELECT 1 FROM public.organizacion_miembro a WHERE a.organizacion_id=m.organizacion_id AND a.usuario_id=$3 AND a.estado='activo' AND a.rol='representante_legal' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($3,clock_timestamp()) k WHERE k.verified)) FOR UPDATE`, member, id, actor).Scan(&role)
	if err != nil {
		return mapErr(err)
	}
	if role == string(organizations.LegalRepresentative) {
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM public.organizacion_miembro WHERE organizacion_id=$1 AND estado='activo' AND rol='representante_legal'`, id).Scan(&count)
		if err != nil {
			return mapErr(err)
		}
		if lastRepresentativeConflict(role, count) {
			return organizations.ErrConflict
		}
	}
	_, err = tx.Exec(ctx, `UPDATE public.organizacion_miembro SET estado='retirado',removed_at=$2,updated_at=$2 WHERE id=$1`, member, now)
	if err != nil {
		return mapErr(err)
	}
	return mapErr(tx.Commit(ctx))
}
func (r *Repository) SetRole(ctx context.Context, actor, id, member string, role organizations.Role, now time.Time) error {
	if !r.valid() {
		return organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var current string
	err = tx.QueryRow(ctx, `SELECT m.rol FROM public.organizacion_miembro m WHERE m.id=$1 AND m.organizacion_id=$2 AND m.estado='activo' AND EXISTS(SELECT 1 FROM public.organizacion_miembro a WHERE a.organizacion_id=m.organizacion_id AND a.usuario_id=$3 AND a.estado='activo' AND a.rol='representante_legal' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($3,clock_timestamp()) k WHERE k.verified)) FOR UPDATE`, member, id, actor).Scan(&current)
	if err != nil {
		return mapErr(err)
	}
	if current == string(organizations.LegalRepresentative) && role != organizations.LegalRepresentative {
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM public.organizacion_miembro WHERE organizacion_id=$1 AND estado='activo' AND rol='representante_legal'`, id).Scan(&count)
		if err != nil {
			return mapErr(err)
		}
		if lastRepresentativeConflict(current, count) {
			return organizations.ErrConflict
		}
	}
	_, err = tx.Exec(ctx, `UPDATE public.organizacion_miembro SET rol=$2,updated_at=$3 WHERE id=$1`, member, role, now)
	if err != nil {
		return mapErr(err)
	}
	return mapErr(tx.Commit(ctx))
}
func (r *Repository) Role(ctx context.Context, user, id string) (organizations.Role, error) {
	if !r.valid() {
		return "", organizations.ErrInvalid
	}
	var role string
	err := r.pool.QueryRow(ctx, `SELECT m.rol FROM public.organizacion_miembro m WHERE m.usuario_id=$1 AND m.organizacion_id=$2 AND m.estado='activo' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,clock_timestamp()) k WHERE k.verified)`, user, id).Scan(&role)
	if err != nil {
		return "", mapErr(err)
	}
	return organizations.Role(strings.TrimSpace(role)), nil
}

func (r *Repository) RequestJoin(ctx context.Context, user string, fingerprints []string, role organizations.Role, now time.Time) (organizations.JoinRequest, error) {
	if !r.valid() || !validUUID(user) || len(fingerprints) == 0 {
		return organizations.JoinRequest{}, organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var verified bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,$2) k WHERE k.verified)`, user, now).Scan(&verified)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	if !verified {
		return organizations.JoinRequest{}, organizations.ErrForbidden
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT count(DISTINCT f.organizacion_id) FROM public.organizacion_rut_fingerprint f WHERE f.fingerprint=ANY($1::varchar[])`, fingerprints).Scan(&count)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	if count == 0 {
		return organizations.JoinRequest{}, organizations.ErrNotFound
	}
	if count > 1 {
		return organizations.JoinRequest{}, organizations.ErrConflict
	}
	var orgID string
	err = tx.QueryRow(ctx, `SELECT DISTINCT f.organizacion_id::text FROM public.organizacion_rut_fingerprint f WHERE f.fingerprint=ANY($1::varchar[])`, fingerprints).Scan(&orgID)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	var alreadyMember bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.organizacion_miembro WHERE organizacion_id=$1 AND usuario_id=$2 AND estado='activo')`, orgID, user).Scan(&alreadyMember)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	if alreadyMember {
		return organizations.JoinRequest{}, organizations.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE public.organizacion_invitacion SET estado='expirada',resolved_by=$2,resolved_at=$3 WHERE organizacion_id=$1 AND target_usuario_id=$2 AND tipo='solicitud_adhesion' AND estado='pendiente' AND expires_at<=$3`, orgID, user, now)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	var req organizations.JoinRequest
	err = tx.QueryRow(ctx, `INSERT INTO public.organizacion_invitacion(organizacion_id,tipo,target_usuario_id,rol_propuesto,estado,expires_at,created_by) VALUES($1,'solicitud_adhesion',$2,$3,'pendiente',$4,$2) RETURNING id::text,rol_propuesto,estado,expires_at,created_at`, orgID, user, role, now.Add(7*24*time.Hour)).Scan(&req.ID, &req.Role, &req.State, &req.ExpiresAt, &req.CreatedAt)
	if err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	req.UserID = user
	if err = tx.Commit(ctx); err != nil {
		return organizations.JoinRequest{}, mapErr(err)
	}
	return req, nil
}

func (r *Repository) OwnJoinRequest(ctx context.Context, user, requestID string) (organizations.JoinRequest, error) {
	if !r.valid() {
		return organizations.JoinRequest{}, organizations.ErrInvalid
	}
	var req organizations.JoinRequest
	err := r.pool.QueryRow(ctx, `SELECT id::text,rol_propuesto,CASE WHEN estado='pendiente' AND expires_at<=clock_timestamp() THEN 'expirada' ELSE estado END,expires_at,created_at FROM public.organizacion_invitacion WHERE id=$1 AND target_usuario_id=$2 AND tipo='solicitud_adhesion'`, requestID, user).Scan(&req.ID, &req.Role, &req.State, &req.ExpiresAt, &req.CreatedAt)
	return req, mapErr(err)
}

func (r *Repository) JoinRequests(ctx context.Context, actor, orgID string) ([]organizations.JoinRequest, error) {
	if !r.valid() {
		return nil, organizations.ErrInvalid
	}
	allowed := `EXISTS(SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=$1 AND m.usuario_id=$2 AND m.estado='activo' AND m.rol='representante_legal'
		AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,clock_timestamp()) k WHERE k.verified))`
	rows, err := r.pool.Query(ctx, `SELECT `+allowed+`,i.id::text,i.target_usuario_id::text,i.rol_propuesto,i.estado,i.expires_at,i.created_at
		FROM (SELECT 1) seed LEFT JOIN public.organizacion_invitacion i ON `+allowed+`
		AND i.organizacion_id=$1 AND i.tipo='solicitud_adhesion' AND i.estado='pendiente' AND i.expires_at>clock_timestamp()
		ORDER BY i.created_at,i.id`, orgID, actor)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	items := []organizations.JoinRequest{}
	for rows.Next() {
		var allowed bool
		var id, user, role, state *string
		var expires, created *time.Time
		if err = rows.Scan(&allowed, &id, &user, &role, &state, &expires, &created); err != nil {
			return nil, mapErr(err)
		}
		if !allowed {
			return nil, organizations.ErrForbidden
		}
		if id == nil {
			continue
		}
		item := organizations.JoinRequest{ID: *id, Role: organizations.Role(*role), State: *state, ExpiresAt: *expires, CreatedAt: *created}
		if user != nil {
			item.UserID = *user
		}
		items = append(items, item)
	}
	return items, mapErr(rows.Err())
}

func (r *Repository) ApproveJoinRequest(ctx context.Context, actor, orgID, requestID string, tokenHash []byte, now, expires time.Time) (organizations.Invitation, error) {
	if !r.valid() {
		return organizations.Invitation{}, organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var actorMembershipID string
	err = tx.QueryRow(ctx, `SELECT m.id::text FROM public.organizacion_miembro m WHERE m.organizacion_id=$1 AND m.usuario_id=$2 AND m.estado='activo' AND m.rol='representante_legal' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,$3) k WHERE k.verified) FOR SHARE`, orgID, actor, now).Scan(&actorMembershipID)
	if errors.Is(err, pgx.ErrNoRows) {
		return organizations.Invitation{}, organizations.ErrForbidden
	}
	if err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	var target string
	var role organizations.Role
	err = tx.QueryRow(ctx, `SELECT target_usuario_id::text,rol_propuesto FROM public.organizacion_invitacion WHERE id=$1 AND organizacion_id=$2 AND tipo='solicitud_adhesion' AND estado='pendiente' AND expires_at>$3 FOR UPDATE`, requestID, orgID, now).Scan(&target, &role)
	if err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	var targetVerified bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.account_profile_identity_status($1,$2) k WHERE k.verified)`, target, now).Scan(&targetVerified)
	if err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	if !targetVerified {
		return organizations.Invitation{}, organizations.ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE public.organizacion_invitacion SET estado='aceptada',resolved_by=$2,resolved_at=$3 WHERE id=$1`, requestID, actor, now); err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	var inv organizations.Invitation
	err = tx.QueryRow(ctx, `INSERT INTO public.organizacion_invitacion(organizacion_id,tipo,target_usuario_id,rol_propuesto,token_sha256,estado,expires_at,created_by) VALUES($1,'invitacion',$2,$3,$4,'pendiente',$5,$6) RETURNING id::text,organizacion_id::text,rol_propuesto,estado,expires_at,created_at`, orgID, target, role, tokenHash, expires, actor).Scan(&inv.ID, &inv.OrganizationID, &inv.Role, &inv.State, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return organizations.Invitation{}, mapErr(err)
	}
	return inv, nil
}

func (r *Repository) RejectJoinRequest(ctx context.Context, actor, orgID, requestID string, now time.Time) error {
	if !r.valid() {
		return organizations.ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var actorMembershipID string
	err = tx.QueryRow(ctx, `SELECT m.id::text FROM public.organizacion_miembro m WHERE m.organizacion_id=$1 AND m.usuario_id=$2 AND m.estado='activo' AND m.rol='representante_legal' AND EXISTS(SELECT 1 FROM public.account_profile_identity_status($2,$3) k WHERE k.verified) FOR SHARE`, orgID, actor, now).Scan(&actorMembershipID)
	if errors.Is(err, pgx.ErrNoRows) {
		return organizations.ErrForbidden
	}
	if err != nil {
		return mapErr(err)
	}
	result, err := tx.Exec(ctx, `UPDATE public.organizacion_invitacion SET estado='rechazada',resolved_by=$3,resolved_at=$4 WHERE id=$1 AND organizacion_id=$2 AND tipo='solicitud_adhesion' AND estado='pendiente' AND expires_at>$4`, requestID, orgID, actor, now)
	if err != nil {
		return mapErr(err)
	}
	if result.RowsAffected() == 0 {
		return organizations.ErrNotFound
	}
	return mapErr(tx.Commit(ctx))
}

func validUUID(value string) bool {
	return len(value) == 36 && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-'
}
