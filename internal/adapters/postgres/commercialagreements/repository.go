package commercialagreementspg

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialauthorization"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type database interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct {
	db   database
	gate rentalflow.IdentityGate
	now  func() time.Time
}

func New(pool *pgxpool.Pool, gate rentalflow.IdentityGate, now func() time.Time) (*Repository, error) {
	if pool == nil || gate == nil {
		return nil, domain.ErrInvalid
	}
	if now == nil {
		now = time.Now
	}
	return &Repository{db: pool, gate: gate, now: now}, nil
}

const columns = `id::text,propuesta_id::text,cotizacion_id::text,espacio_id::text,propietario_id::text,arrendatario_id::text,propuesta_version,encode(documento_sha256,'hex'),estado,vence_en,pago_vence_en,ocupacion_id::text,modalidad_firma,revision,creada_en,actualizada_en,es_b2b,organizacion_id::text,empresa_razon_social_snapshot,empresa_rut_masked_snapshot,empresa_rut_verificada,representante_legal_id::text,representante_nombre_snapshot,representante_rut_masked_snapshot,representante_rut_verificada`

func scan(row pgx.Row) (domain.Agreement, error) {
	var a domain.Agreement
	var isB2B, companyRUTVerified, repRUTVerified bool
	var organizationID, representativeID *string
	var legalName, companyRUT, representativeName, representativeRUT *string
	a.Signatures = []domain.Signature{}
	err := row.Scan(&a.ID, &a.ProposalID, &a.QuoteID, &a.SpaceID, &a.OwnerID, &a.RenterID, &a.ProposalVersion, &a.SHA256, &a.State, &a.ExpiresAt, &a.PaymentExpiresAt, &a.OccupancyID, &a.SignatureMode, &a.Revision, &a.CreatedAt, &a.UpdatedAt, &isB2B, &organizationID, &legalName, &companyRUT, &companyRUTVerified, &representativeID, &representativeName, &representativeRUT, &repRUTVerified)
	if err == nil && isB2B {
		if organizationID == nil || legalName == nil || companyRUT == nil || representativeID == nil || representativeName == nil || representativeRUT == nil || !companyRUTVerified || !repRUTVerified {
			return a, domain.ErrConflict
		}
		a.CompanyRepresentation = &domain.CompanyRepresentation{OrganizationID: *organizationID, LegalName: *legalName, MaskedRUT: *companyRUT, RUTVerified: true, RepresentativeUserID: *representativeID, RepresentativeName: *representativeName, RepresentativeRUT: *representativeRUT, RepresentativeRUTVerified: true}
	}
	return a, mapError(err)
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, rentalflow.ErrNotFound) {
		return domain.ErrNotFound
	}
	if errors.Is(err, rentalflow.ErrConflict) || errors.Is(err, rentalflow.ErrUnavailable) {
		return domain.ErrConflict
	}
	if errors.Is(err, rentalflow.ErrInvalid) {
		return domain.ErrInvalid
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23P01":
			return domain.ErrConflict
		case "23503":
			return domain.ErrConflict
		case "23514", "22003":
			return domain.ErrInvalid
		}
	}
	return err
}

type rowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func signatures(ctx context.Context, db rowReader, a *domain.Agreement) error {
	rows, err := db.Query(ctx, `SELECT rol,firmada_en,encode(documento_sha256,'hex'),propuesta_version,consentimiento_version,tipo_firma,metodo_autenticacion,COALESCE(encode(evidencia_sha256,'hex'),'') FROM public.contrato_firma_comercial WHERE solicitud_id=$1 ORDER BY firmada_en,rol`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	a.Signatures = []domain.Signature{}
	for rows.Next() {
		var s domain.Signature
		if err = rows.Scan(&s.Role, &s.SignedAt, &s.SHA256, &s.ProposalVersion, &s.ConsentVersion, &s.SignatureType, &s.AuthMethod, &s.EvidenceSHA256); err != nil {
			return err
		}
		a.Signatures = append(a.Signatures, s)
	}
	return rows.Err()
}
func (r *Repository) Get(ctx context.Context, actor, id string) (domain.Agreement, error) {
	a, err := scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_solicitud_comercial WHERE id=$1 AND ($2=propietario_id OR $2=arrendatario_id OR (es_b2b AND $2=representante_legal_id))`, id, actor))
	if err != nil {
		return a, err
	}
	err = signatures(ctx, r.db, &a)
	return a, err
}
func (r *Repository) List(ctx context.Context, actor, role string) ([]domain.Agreement, error) {
	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM public.contrato_solicitud_comercial WHERE ($1=propietario_id OR $1=arrendatario_id OR (es_b2b AND $1=representante_legal_id)) AND ($2='' OR ($2='landlord' AND ($1=propietario_id OR (es_b2b AND $1=representante_legal_id))) OR ($2='renter' AND $1=arrendatario_id)) ORDER BY creada_en DESC,id DESC LIMIT 100`, actor, role)
	if err != nil {
		return nil, err
	}
	items := []domain.Agreement{}
	for rows.Next() {
		a, e := scan(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		items = append(items, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if err = signatures(ctx, r.db, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// Accounts are locked in canonical order before quote, space and request locks.
// This also serializes evidence with privacy suppression and identity revocation.
func lockParticipants(ctx context.Context, tx pgx.Tx, owner, renter string) error {
	ids := []string{owner, renter}
	sort.Strings(ids)
	if err := accountlock.LockAccountIDs(ctx, tx, ids...); err != nil {
		return err
	}
	for _, id := range ids {
		var state string
		if err := tx.QueryRow(ctx, `SELECT estado FROM public.usuario WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
			return mapError(err)
		}
	}
	return nil
}
func requireRole(ctx context.Context, tx pgx.Tx, actor, role string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario u JOIN public.rol_usuario r ON r.usuario_id=u.id WHERE u.id=$1 AND u.estado='activo' AND r.rol=$2 AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id))`, actor, role).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrNotFound
	}
	return nil
}
func (r *Repository) gateParties(ctx context.Context, tx pgx.Tx, a domain.Agreement, at time.Time) error {
	if err := requireRole(ctx, tx, a.OwnerID, "arrendador"); err != nil {
		return err
	}
	if err := requireRole(ctx, tx, a.RenterID, "arrendatario"); err != nil {
		return err
	}
	if err := r.gate.RequireForBooking(ctx, a.OwnerID, a.RenterID, at); err != nil {
		return domain.ErrIdentity
	}
	return nil
}

type source struct {
	agreement        domain.Agreement
	quote            rentalflow.Quote
	locationRevision int64
	templateID       string
	templateVersion  int64
}

func proposal(ctx context.Context, tx pgx.Tx, id string) (source, error) {
	var s source
	var blob []byte
	err := tx.QueryRow(ctx, `SELECT id::text,cotizacion_id::text,espacio_id::text,propietario_id::text,arrendatario_id::text,version,encode(documento_sha256,'hex'),terminos,plantilla_version_id::text FROM public.contrato_propuesta_comercial WHERE id=$1`, id).Scan(&s.agreement.ProposalID, &s.agreement.QuoteID, &s.agreement.SpaceID, &s.agreement.OwnerID, &s.agreement.RenterID, &s.agreement.ProposalVersion, &s.agreement.SHA256, &blob, &s.templateID)
	if err != nil {
		return s, mapError(err)
	}
	var terms struct {
		SpaceTitle      string    `json:"space_title"`
		TemplateVersion int64     `json:"template_version"`
		RateVersion     int64     `json:"rate_version"`
		RateUnit        string    `json:"rate_unit"`
		UnitPrice       int64     `json:"unit_price_clp"`
		Units           int64     `json:"units"`
		Subtotal        int64     `json:"subtotal_clp"`
		Currency        string    `json:"currency"`
		Start           time.Time `json:"start_at"`
		End             time.Time `json:"end_at"`
		Zone            string    `json:"time_zone"`
		Location        struct {
			Revision int64 `json:"revision"`
		} `json:"location"`
		CompanyRepresentation *struct {
			OrganizationID            string `json:"organization_id"`
			LegalName                 string `json:"legal_name"`
			MaskedRUT                 string `json:"rut_masked"`
			RUTVerified               bool   `json:"rut_verified"`
			RepresentativeUserID      string `json:"representative_user_id"`
			RepresentativeName        string `json:"representative_name"`
			RepresentativeRUT         string `json:"representative_rut_masked"`
			RepresentativeRUTVerified bool   `json:"representative_rut_verified"`
		} `json:"company_representation,omitempty"`
	}
	if err = json.Unmarshal(blob, &terms); err != nil {
		return s, err
	}
	if terms.CompanyRepresentation != nil {
		c := terms.CompanyRepresentation
		if c.OrganizationID == "" || c.LegalName == "" || c.MaskedRUT == "" || !c.RUTVerified || c.RepresentativeUserID == "" || c.RepresentativeName == "" || c.RepresentativeRUT == "" || !c.RepresentativeRUTVerified {
			return s, domain.ErrConflict
		}
		s.agreement.CompanyRepresentation = &domain.CompanyRepresentation{OrganizationID: c.OrganizationID, LegalName: c.LegalName, MaskedRUT: c.MaskedRUT, RUTVerified: c.RUTVerified, RepresentativeUserID: c.RepresentativeUserID, RepresentativeName: c.RepresentativeName, RepresentativeRUT: c.RepresentativeRUT, RepresentativeRUTVerified: c.RepresentativeRUTVerified}
	}
	s.locationRevision = terms.Location.Revision
	s.templateVersion = terms.TemplateVersion
	s.quote = rentalflow.Quote{ID: s.agreement.QuoteID, SpaceID: s.agreement.SpaceID, OwnerID: s.agreement.OwnerID, RenterID: s.agreement.RenterID, SpaceTitle: terms.SpaceTitle, RateVersion: terms.RateVersion, RateUnit: terms.RateUnit, UnitPriceCLP: terms.UnitPrice, Units: terms.Units, SubtotalCLP: terms.Subtotal, Currency: terms.Currency, StartAt: terms.Start, EndAt: terms.End, TimeZone: terms.Zone}
	if s.locationRevision < 0 || s.quote.RateVersion < 1 || s.quote.StartAt.IsZero() || s.quote.SubtotalCLP < 1 {
		return s, domain.ErrConflict
	}
	return s, nil
}
func hashMatches(actual, expected string) bool {
	a, e1 := hex.DecodeString(actual)
	b, e2 := hex.DecodeString(expected)
	return e1 == nil && e2 == nil && len(a) == 32 && len(b) == 32 && subtle.ConstantTimeCompare(a, b) == 1
}
func writeSignature(ctx context.Context, tx pgx.Tx, a domain.Agreement, actor domain.Actor, role, authMethod string, challengeID any, at time.Time) error {
	var ipMasked any
	if actor.IPMasked != "" {
		ipMasked = actor.IPMasked
	}
	var uaHash any
	if len(actor.UserAgentHash) == 32 {
		uaHash = actor.UserAgentHash
	}
	evSHA := computeEvidenceSHA256(a, actor, role, authMethod, challengeID, at)
	_, err := tx.Exec(ctx, `INSERT INTO public.contrato_firma_comercial(id,solicitud_id,actor_id,sesion_id,rol,propuesta_version,documento_sha256,consentimiento,consentimiento_version,firmada_en,tipo_firma,metodo_autenticacion,desafio_otp_id,ip_masked,user_agent_hash,evidencia_sha256) VALUES($1,$2,$3,$4,$5,$6,decode($7,'hex'),true,$8,$9,'FES',$10,$11,$12,$13,$14)`, uuid.NewString(), a.ID, actor.ID, actor.SessionID, role, a.ProposalVersion, a.SHA256, domain.ConsentVersion, at, authMethod, challengeID, ipMasked, uaHash, evSHA)
	return mapError(err)
}
func writeEvent(ctx context.Context, tx pgx.Tx, a domain.Agreement, actor any, kind string, at time.Time) error {
	return writeEventWithNotice(ctx, tx, a, actor, kind, kind, at)
}

func writeEventWithNotice(ctx context.Context, tx pgx.Tx, a domain.Agreement, actor any, kind, noticeKind string, at time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.contrato_solicitud_evento_comercial(id,solicitud_id,actor_id,tipo,revision,documento_sha256,ocurrida_en) VALUES($1,$2,$3,$4,$5,decode($6,'hex'),$7)`, uuid.NewString(), a.ID, actor, kind, a.Revision, a.SHA256, at)
	if err != nil {
		return err
	}
	recipients := []string{a.OwnerID}
	if kind == "landlord_signed" || kind == "rejected" {
		recipients = []string{a.RenterID}
	} else if kind == "expired" {
		recipients = []string{a.OwnerID, a.RenterID}
	}
	for _, recipient := range recipients {
		_, err = tx.Exec(ctx, `INSERT INTO public.contrato_solicitud_notice_outbox(id,solicitud_id,destinatario_id,tipo,disponible_en,creada_en) VALUES($1,$2,$3,$4,$5,$5) ON CONFLICT(solicitud_id,destinatario_id,tipo) DO NOTHING`, uuid.NewString(), a.ID, recipient, kind, at)
		if err != nil {
			return err
		}
		if err = insertUserNotification(ctx, tx, a, recipient, noticeKind, at); err != nil {
			return err
		}
	}
	return nil
}

func insertUserNotification(ctx context.Context, tx pgx.Tx, a domain.Agreement, recipient, kind string, at time.Time) error {
	title, body, href := notificationDetails(a.ID, recipient == a.OwnerID, kind)
	if title == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(referencia_id,usuario_id,tipo) WHERE referencia_id IS NOT NULL DO NOTHING`, uuid.NewString(), recipient, kind, title, body, href, a.ID, at)
	return err
}

func notificationDetails(agreementID string, isOwner bool, kind string) (title, body, href string) {
	switch kind {
	case "request_created":
		return "Nueva solicitud de contrato", "Tienes un contrato firmado por el arrendatario esperando tu firma.", "/reservas/contratos?rol=anfitrion&solicitud=" + agreementID
	case "request_auto_confirmed":
		return "Solicitud confirmada automáticamente", "Se aplicó tu firma automática acotada; el arrendatario ya puede pagar.", "/reservas/contratos?rol=anfitrion&solicitud=" + agreementID
	case "landlord_signed":
		return "Contrato firmado por el anfitrión", "El anfitrión firmó tu solicitud. Ya puedes continuar al pago.", "/reservas/contratos/" + agreementID
	case "rejected":
		return "Solicitud de contrato rechazada", "El anfitrión rechazó la solicitud de arriendo.", "/reservas/contratos/" + agreementID
	case "withdrawn":
		return "Solicitud de contrato retirada", "El arrendatario retiró su solicitud de contrato.", "/reservas/contratos?rol=anfitrion&solicitud=" + agreementID
	case "expired":
		if isOwner {
			return "Solicitud de contrato vencida", "El plazo de firma para la solicitud de contrato venció.", "/reservas/contratos?rol=anfitrion&solicitud=" + agreementID
		}
		return "Solicitud de contrato vencida", "El plazo de la solicitud de contrato venció.", "/reservas/contratos/" + agreementID
	default:
		return "", "", ""
	}
}
func complete(ctx context.Context, tx pgx.Tx, a domain.Agreement) (domain.Agreement, error) {
	if err := signatures(ctx, tx, &a); err != nil {
		return a, err
	}
	if err := tx.Commit(ctx); err != nil {
		return a, mapError(err)
	}
	return a, nil
}
func updateState(ctx context.Context, tx pgx.Tx, a *domain.Agreement, state string, actor any, kind string, at time.Time) error {
	a.State = state
	a.Revision++
	a.UpdatedAt = at
	_, err := tx.Exec(ctx, `UPDATE public.contrato_solicitud_comercial SET estado=$2,pago_vence_en=$3,ocupacion_id=$4,revision=$5,actualizada_en=$6 WHERE id=$1`, a.ID, a.State, a.PaymentExpiresAt, a.OccupancyID, a.Revision, at)
	if err != nil {
		return mapError(err)
	}
	return writeEvent(ctx, tx, *a, actor, kind, at)
}
func matchingAutoAuthorization(ctx context.Context, tx pgx.Tx, spaceID, ownerID string, templateVersion, subtotalCLP int64, currency string, at time.Time) (string, error) {
	if currency != "CLP" || subtotalCLP < 1 || templateVersion < 1 {
		return "", nil
	}
	var grantID string
	err := tx.QueryRow(ctx, `SELECT a.id::text FROM public.autorizacion_arriendo_comercial a JOIN public.espacio_contrato_borrador d ON d.espacio_id=a.espacio_id WHERE a.espacio_id=$1 AND a.propietario_id=$2 AND a.plantilla_version=$3 AND d.version_publicada=$3 AND a.revocada_en IS NULL AND a.vigente_desde<=$4 AND a.vence_en>$4 AND (a.monto_maximo_clp>=$6 OR a.monto_maximo_clp>=$5) FOR UPDATE OF a LIMIT 1`, spaceID, ownerID, templateVersion, at, subtotalCLP, commercialauthorization.MaxAmountCLP).Scan(&grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return grantID, err
}

func applyAutoAuthorization(ctx context.Context, tx pgx.Tx, a *domain.Agreement, s source, grantID string, at time.Time) error {
	hold, err := rentalflowpg.RetainCommercialAgreementInterval(ctx, tx, rentalflowpg.CommercialAgreementHoldInput{
		RequestID:        a.ID,
		Quote:            s.quote,
		LocationRevision: s.locationRevision,
	}, at)
	if err != nil {
		return mapError(err)
	}
	if err = writeSignature(ctx, tx, *a, domain.Actor{ID: a.OwnerID, SessionID: grantID}, "arrendador", "auto_authorization", nil, at); err != nil {
		return err
	}
	a.PaymentExpiresAt = &hold.ExpiresAt
	a.OccupancyID = &hold.OccupancyID
	if err = updateState(ctx, tx, a, domain.Ready, a.OwnerID, "landlord_signed", at); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.autorizacion_arriendo_comercial_evento(id,autorizacion_id,espacio_id,actor_id,accion,motivo,ocurrida_en) VALUES($1,$2,$3,$4,'auto_confirmed','within_authorized_limits',$5)`, uuid.NewString(), grantID, a.SpaceID, a.OwnerID, at)
	return mapError(err)
}

func (r *Repository) Create(ctx context.Context, actor domain.Actor, in domain.CreateInput, key string) (domain.Agreement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Agreement{}, err
	}
	defer tx.Rollback(ctx)
	s, err := proposal(ctx, tx, in.ProposalID)
	if err != nil {
		return domain.Agreement{}, err
	}
	a := s.agreement
	if actor.ID != a.RenterID {
		return domain.Agreement{}, domain.ErrNotFound
	}
	if err = lockParticipants(ctx, tx, a.OwnerID, a.RenterID); err != nil {
		return a, err
	}
	if err = rentalflowpg.LockCommercialAgreementInterval(ctx, tx, a.QuoteID, a.SpaceID); err != nil {
		return a, mapError(err)
	}
	var oldFingerprint []byte
	prior, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_solicitud_comercial WHERE arrendatario_id=$1 AND idempotency_key=$2 FOR UPDATE`, actor.ID, key))
	if err == nil {
		if err = tx.QueryRow(ctx, `SELECT fingerprint FROM public.contrato_solicitud_comercial WHERE id=$1`, prior.ID).Scan(&oldFingerprint); err != nil {
			return a, err
		}
		if subtle.ConstantTimeCompare(oldFingerprint, domain.Fingerprint(in)) != 1 {
			return a, domain.ErrIdempotency
		}
		prior.Replayed = true
		return complete(ctx, tx, prior)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return a, err
	}
	if !hashMatches(a.SHA256, in.SHA256) {
		return a, domain.ErrHash
	}
	at := r.now().UTC()
	if err = r.gateParties(ctx, tx, a, at); err != nil {
		return a, err
	}
	var current bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.cotizacion_arriendo_comercial q JOIN public.espacio e ON e.id=q.espacio_id WHERE q.id=$1 AND q.arrendatario_id=$2 AND q.vence_en>$3 AND e.estado='activa' AND e.ubicacion_confirmada_en IS NOT NULL AND e.ubicacion_revision=$4 AND NOT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c WHERE c.cotizacion_id=q.id) AND NOT EXISTS(SELECT 1 FROM public.contrato_solicitud_comercial s WHERE s.cotizacion_id=q.id AND s.estado IN ('pending_landlord','ready_for_payment')) AND (SELECT id FROM public.espacio_contrato_version WHERE espacio_id=e.id ORDER BY version DESC LIMIT 1)=$5 AND (SELECT version FROM public.tarifa_espacio WHERE espacio_id=e.id ORDER BY version DESC LIMIT 1)=$6)`, a.QuoteID, actor.ID, at, s.locationRevision, s.templateID, s.quote.RateVersion).Scan(&current)
	if err != nil {
		return a, err
	}
	if !current {
		return a, domain.ErrConflict
	}
	if !s.quote.StartAt.After(at) {
		return a, domain.ErrExpired
	}
	a.ID = uuid.NewString()
	a.State = domain.Pending
	a.Revision = 1
	a.CreatedAt = at
	a.UpdatedAt = at
	a.ExpiresAt = at.Add(domain.RequestLifetime)
	if s.quote.StartAt.Before(a.ExpiresAt) {
		a.ExpiresAt = s.quote.StartAt
	}
	authMethod, challengeID, err := r.verifyOTPInTx(ctx, tx, actor, a.ProposalID, a.SHA256, "arrendatario", in.OTPChallengeID, in.OTPCode, at)
	if err != nil {
		return a, err
	}
	mode := domain.ModeFESInternal
	if in.SignatureMode == domain.ModeFirmaVirtualNotarial || in.SignatureMode == domain.ModeFEAExternal {
		mode = in.SignatureMode
	}
	a.SignatureMode = mode
	c := a.CompanyRepresentation
	var isB2B bool
	var orgID, legalName, companyRUT, repID, repName, repRUT any
	var companyRUTVerified, repRUTVerified bool
	if c != nil {
		isB2B, orgID, legalName, companyRUT, repID, repName, repRUT = true, c.OrganizationID, c.LegalName, c.MaskedRUT, c.RepresentativeUserID, c.RepresentativeName, c.RepresentativeRUT
		companyRUTVerified, repRUTVerified = c.RUTVerified, c.RepresentativeRUTVerified
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.contrato_solicitud_comercial(id,propuesta_id,cotizacion_id,espacio_id,propietario_id,arrendatario_id,propuesta_version,documento_sha256,idempotency_key,fingerprint,estado,vence_en,modalidad_firma,creada_en,actualizada_en,es_b2b,organizacion_id,empresa_razon_social_snapshot,empresa_rut_masked_snapshot,empresa_rut_verificada,representante_legal_id,representante_nombre_snapshot,representante_rut_masked_snapshot,representante_rut_verificada) VALUES($1,$2,$3,$4,$5,$6,$7,decode($8,'hex'),$9,$10,$11,$12,$13,$14,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`, a.ID, a.ProposalID, a.QuoteID, a.SpaceID, a.OwnerID, a.RenterID, a.ProposalVersion, a.SHA256, key, domain.Fingerprint(in), a.State, a.ExpiresAt, mode, at, isB2B, orgID, legalName, companyRUT, companyRUTVerified, repID, repName, repRUT, repRUTVerified)
	if err != nil {
		return a, mapError(err)
	}
	if err = writeSignature(ctx, tx, a, actor, "arrendatario", authMethod, challengeID, at); err != nil {
		return a, err
	}
	autoGrantID, err := matchingAutoAuthorization(ctx, tx, a.SpaceID, a.OwnerID, s.templateVersion, s.quote.SubtotalCLP, s.quote.Currency, at)
	if err != nil {
		return a, err
	}
	if autoGrantID == "" {
		if err = writeEvent(ctx, tx, a, actor.ID, "request_created", at); err != nil {
			return a, err
		}
		return complete(ctx, tx, a)
	}
	if err = writeEventWithNotice(ctx, tx, a, actor.ID, "request_created", "request_auto_confirmed", at); err != nil {
		return a, err
	}
	if err = applyAutoAuthorization(ctx, tx, &a, s, autoGrantID, at); err != nil {
		return a, err
	}
	return complete(ctx, tx, a)
}

func (r *Repository) locked(ctx context.Context, tx pgx.Tx, actor, id string) (domain.Agreement, error) {
	a, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_solicitud_comercial WHERE id=$1`, id))
	if err != nil {
		return a, err
	}
	if actor != "" && actor != a.OwnerID && actor != a.RenterID && (a.CompanyRepresentation == nil || actor != a.CompanyRepresentation.RepresentativeUserID) {
		return a, domain.ErrNotFound
	}
	if err = lockParticipants(ctx, tx, a.OwnerID, a.RenterID); err != nil {
		return a, err
	}
	if err = rentalflowpg.LockCommercialAgreementInterval(ctx, tx, a.QuoteID, a.SpaceID); err != nil {
		return a, mapError(err)
	}
	return scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_solicitud_comercial WHERE id=$1 FOR UPDATE`, id))
}

func requireCompanyRepresentative(ctx context.Context, tx pgx.Tx, actor, ownerID, spaceID string, c *domain.CompanyRepresentation, asOf time.Time) error {
	if c == nil || actor == "" || actor != c.RepresentativeUserID {
		return domain.ErrNotFound
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.organizacion_miembro m JOIN public.usuario u ON u.id=m.usuario_id JOIN public.espacio e ON e.id=$3 AND e.organizacion_id=m.organizacion_id AND e.es_b2b AND e.propietario_id=$4 JOIN LATERAL public.account_profile_identity_status(m.usuario_id,$5) k ON k.verified WHERE m.organizacion_id=$1 AND m.usuario_id=$2 AND m.rol='representante_legal' AND m.estado='activo' AND u.estado='activo' AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=m.usuario_id))`, c.OrganizationID, actor, spaceID, ownerID, asOf.UTC()).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrNotFound
	}
	return nil
}
func (r *Repository) Accept(ctx context.Context, actor domain.Actor, id string, revision int64, in domain.SignInput) (domain.Agreement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Agreement{}, err
	}
	defer tx.Rollback(ctx)
	a, err := r.locked(ctx, tx, actor.ID, id)
	if err != nil {
		return a, err
	}
	if a.CompanyRepresentation != nil {
		if err = requireCompanyRepresentative(ctx, tx, actor.ID, a.OwnerID, a.SpaceID, a.CompanyRepresentation, r.now().UTC()); err != nil {
			return a, err
		}
	} else if actor.ID != a.OwnerID {
		return a, domain.ErrNotFound
	}
	if !hashMatches(a.SHA256, in.SHA256) {
		return a, domain.ErrHash
	}
	if a.CompanyRepresentation == nil {
		if err = requireRole(ctx, tx, actor.ID, "arrendador"); err != nil {
			return a, err
		}
	}
	var signed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.contrato_firma_comercial WHERE solicitud_id=$1 AND rol='arrendador')`, id).Scan(&signed); err != nil {
		return a, err
	}
	if signed {
		a.Replayed = true
		return complete(ctx, tx, a)
	}
	if a.Revision != revision {
		return a, domain.ErrPrecondition
	}
	if a.State != domain.Pending {
		return a, domain.ErrConflict
	}
	at := r.now().UTC()
	if !a.ExpiresAt.After(at) {
		if err = updateState(ctx, tx, &a, domain.Expired, nil, "expired", at); err != nil {
			return a, err
		}
		if _, err = complete(ctx, tx, a); err != nil {
			return a, err
		}
		return a, domain.ErrExpired
	}
	if err = r.gateParties(ctx, tx, a, at); err != nil {
		return a, err
	}
	s, err := proposal(ctx, tx, a.ProposalID)
	if err != nil {
		return a, err
	}
	hold, err := rentalflowpg.RetainCommercialAgreementInterval(ctx, tx, rentalflowpg.CommercialAgreementHoldInput{RequestID: a.ID, Quote: s.quote, LocationRevision: s.locationRevision}, at)
	if err != nil {
		return a, mapError(err)
	}
	authMethod, challengeID, err := r.verifyOTPInTx(ctx, tx, actor, a.ProposalID, a.SHA256, "arrendador", in.OTPChallengeID, in.OTPCode, at)
	if err != nil {
		return a, err
	}
	if err = writeSignature(ctx, tx, a, actor, "arrendador", authMethod, challengeID, at); err != nil {
		return a, err
	}
	a.PaymentExpiresAt = &hold.ExpiresAt
	a.OccupancyID = &hold.OccupancyID
	if err = updateState(ctx, tx, &a, domain.Ready, actor.ID, "landlord_signed", at); err != nil {
		return a, err
	}
	return complete(ctx, tx, a)
}
func (r *Repository) Decide(ctx context.Context, actor domain.Actor, id string, revision int64, in domain.DecisionInput) (domain.Agreement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Agreement{}, err
	}
	defer tx.Rollback(ctx)
	a, err := r.locked(ctx, tx, actor.ID, id)
	if err != nil {
		return a, err
	}
	if !hashMatches(a.SHA256, in.SHA256) {
		return a, domain.ErrHash
	}
	target, role := domain.Withdrawn, "arrendatario"
	if in.Decision == "reject" {
		target, role = domain.Rejected, "arrendador"
		if actor.ID != a.OwnerID {
			return a, domain.ErrNotFound
		}
	} else if actor.ID != a.RenterID {
		return a, domain.ErrNotFound
	}
	if err = requireRole(ctx, tx, actor.ID, role); err != nil {
		return a, err
	}
	if a.State == target {
		a.Replayed = true
		return complete(ctx, tx, a)
	}
	if a.Revision != revision {
		return a, domain.ErrPrecondition
	}
	if a.State != domain.Pending && (target != domain.Withdrawn || a.State != domain.Ready) {
		return a, domain.ErrConflict
	}
	at := r.now().UTC()
	deadline := a.ExpiresAt
	if a.State == domain.Ready && a.PaymentExpiresAt != nil {
		deadline = *a.PaymentExpiresAt
	}
	if !deadline.After(at) {
		if err = r.expire(ctx, tx, &a, at); err != nil {
			return a, err
		}
		if _, err = complete(ctx, tx, a); err != nil {
			return a, err
		}
		return a, domain.ErrExpired
	}
	if a.State == domain.Ready {
		var checkout bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial WHERE cotizacion_id=$1 AND estado IN ('pending','approved','reconciliation_required'))`, a.QuoteID).Scan(&checkout); err != nil {
			return a, err
		}
		if checkout {
			return a, domain.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=$2 WHERE solicitud_comercial_id=$1 AND tipo='retencion_contrato_comercial' AND activo`, id, at); err != nil {
			return a, err
		}
	}
	if err = updateState(ctx, tx, &a, target, actor.ID, target, at); err != nil {
		return a, err
	}
	return complete(ctx, tx, a)
}
func (r *Repository) expire(ctx context.Context, tx pgx.Tx, a *domain.Agreement, at time.Time) error {
	if a.State == domain.Ready {
		var checkout bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial WHERE cotizacion_id=$1 AND estado IN ('pending','approved','reconciliation_required'))`, a.QuoteID).Scan(&checkout); err != nil {
			return err
		}
		if checkout {
			return domain.ErrConflict
		}
		if err := rentalflowpg.ExpireCommercialAgreementInterval(ctx, tx, a.ID, a.SpaceID, at); err != nil {
			return mapError(err)
		}
	}
	return updateState(ctx, tx, a, domain.Expired, nil, "expired", at)
}
func (r *Repository) ExpireDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	at := r.now().UTC()
	rows, err := r.db.Query(ctx, `SELECT id::text FROM public.contrato_solicitud_comercial s WHERE ((estado='pending_landlord' AND vence_en<=$1) OR (estado='ready_for_payment' AND pago_vence_en<=$1)) AND NOT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c WHERE c.cotizacion_id=s.cotizacion_id AND c.estado IN ('pending','approved','reconciliation_required')) ORDER BY actualizada_en,id LIMIT $2`, at, limit)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		tx, e := r.db.Begin(ctx)
		if e != nil {
			return count, e
		}
		a, e := r.locked(ctx, tx, "", id)
		now := r.now().UTC()
		due := a.State == domain.Pending && !a.ExpiresAt.After(now) || a.State == domain.Ready && a.PaymentExpiresAt != nil && !a.PaymentExpiresAt.After(now)
		if e == nil && due {
			e = r.expire(ctx, tx, &a, now)
			if e == nil {
				_, e = complete(ctx, tx, a)
				if e == nil {
					count++
				}
			}
		}
		_ = tx.Rollback(ctx)
		if e != nil && !errors.Is(e, domain.ErrConflict) && !errors.Is(e, domain.ErrNotFound) {
			return count, e
		}
	}
	return count, nil
}
