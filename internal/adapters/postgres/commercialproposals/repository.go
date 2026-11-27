package commercialproposalspg

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func currentTerms(ctx context.Context, db queryer, actor string, quote rentalflow.Quote, policy commercialproposal.CancellationPolicy) (commercialproposal.Terms, error) {
	const query = `SELECT q.id::text,q.espacio_id::text,q.titulo_espacio,q.tarifa_version,q.modalidad,q.precio_unitario_clp,q.unidades,q.subtotal_clp,q.moneda,q.inicio,q.termino,q.zona_horaria,q.politica_cancelacion_version,q.politica_cancelacion_texto,
e.direccion,e.ubicacion_latitud,e.ubicacion_longitud,e.ubicacion_confirmada_en IS NOT NULL,e.ubicacion_revision,COALESCE(e.region_codigo,''),COALESCE(e.comuna_codigo,''),v.id::text,v.version,v.secciones,e.modo_anfitrion,e.organizacion_id::text
FROM public.cotizacion_arriendo_comercial q JOIN public.espacio e ON e.id=q.espacio_id AND e.propietario_id=q.propietario_id
JOIN LATERAL (SELECT version,modalidad,precio_base_clp,moneda FROM public.tarifa_espacio WHERE espacio_id=e.id ORDER BY version DESC LIMIT 1) rate ON true
LEFT JOIN public.espacio_contrato_borrador b ON b.espacio_id=e.id
LEFT JOIN public.espacio_contrato_version v ON v.espacio_id=e.id AND v.version=b.version_publicada
WHERE q.id=$1 AND ($2=q.propietario_id OR $2=q.arrendatario_id OR (e.modo_anfitrion='empresa' AND EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=e.organizacion_id AND m.usuario_id=$2 AND m.rol='representante_legal' AND m.estado='activo'))) AND q.propietario_id=$3 AND q.arrendatario_id=$4
AND q.vence_en>clock_timestamp() AND q.inicio>clock_timestamp() AND e.estado='activa' AND e.zona_horaria=q.zona_horaria
AND COALESCE((SELECT can_publish FROM public.kyb_space_eligibility(e.id,clock_timestamp())),false)
AND rate.version=q.tarifa_version AND rate.modalidad=q.modalidad AND rate.precio_base_clp=q.precio_unitario_clp AND rate.moneda=q.moneda`
	var terms commercialproposal.Terms
	var lat, lon *float64
	var confirmed bool
	var templateID *string
	var templateVersion *int64
	var sections []byte
	var hostMode string
	var organizationID *string
	var policyVersion, policyText *string
	err := db.QueryRow(ctx, query, quote.ID, actor, quote.OwnerID, quote.RenterID).Scan(&terms.QuoteID, &terms.SpaceID, &terms.SpaceTitle, &terms.RateVersion, &terms.RateUnit, &terms.UnitPriceCLP, &terms.Units, &terms.SubtotalCLP, &terms.Currency, &terms.StartAt, &terms.EndAt, &terms.TimeZone, &policyVersion, &policyText, &terms.Location.Address, &lat, &lon, &confirmed, &terms.Location.Revision, &terms.Location.RegionCode, &terms.Location.CommuneCode, &templateID, &templateVersion, &sections, &hostMode, &organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return terms, commercialproposal.ErrConflict
	}
	if err != nil {
		return terms, err
	}
	if !confirmed || lat == nil || lon == nil {
		return terms, commercialproposal.ErrLocationRequired
	}
	if templateID == nil || templateVersion == nil {
		return terms, commercialproposal.ErrTemplateRequired
	}
	terms.Location.Latitude = *lat
	terms.Location.Longitude = *lon
	terms.TemplateID = *templateID
	terms.TemplateVersion = *templateVersion
	if hostMode == "empresa" {
		if organizationID == nil {
			return terms, commercialproposal.ErrConflict
		}
		terms.CompanyRepresentation = &commercialproposal.CompanyRepresentation{OrganizationID: *organizationID}
	} else if hostMode != "personal" || organizationID != nil {
		return terms, commercialproposal.ErrConflict
	}
	if err = json.Unmarshal(sections, &terms.Sections); err != nil {
		return terms, err
	}
	if _, _, err = contracttemplate.CanonicalSections(terms.Sections); err != nil {
		return terms, commercialproposal.ErrTemplateRequired
	}
	terms.Sections = append(terms.Sections, marketplacePaymentSection(terms.Sections))
	terms.StartAt = terms.StartAt.UTC()
	terms.EndAt = terms.EndAt.UTC()
	if (policyVersion == nil) != (policyText == nil) {
		return terms, commercialproposal.ErrConflict
	}
	if policyVersion != nil {
		terms.CancellationPolicy = commercialproposal.CancellationPolicy{Version: *policyVersion, Text: *policyText}
	} else {
		terms.CancellationPolicy = policy
	}
	return terms, nil
}

func marketplacePaymentSection(ownerSections []contracttemplate.Section) contracttemplate.Section {
	const baseKey = "reparto_pago_hotusoft"
	key := baseKey
	for suffix := 1; slices.ContainsFunc(ownerSections, func(section contracttemplate.Section) bool {
		return section.Key == key
	}); suffix++ {
		key = baseKey + "_" + strconv.Itoa(suffix)
	}
	return contracttemplate.Section{
		Key:   key,
		Title: "Reparto del pago y devoluciones",
		Body:  "Mercado Pago procesa y distribuye el pago de este arriendo. El arrendador recibe en su cuenta vinculada el importe neto que le corresponde y administra su saldo. HOTU SOFTWARE SOLUTIONS SPA recibe la comisión de intermediación en la cuenta del marketplace. Hotusoft gestiona los comprobantes de la operación y tramita las solicitudes de devolución en contacto@hotusoft.com.",
	}
}

func (r *Repository) CurrentTerms(ctx context.Context, actor string, quote rentalflow.Quote, policy commercialproposal.CancellationPolicy) (commercialproposal.Terms, error) {
	return currentTerms(ctx, r.pool, actor, quote, policy)
}

const columns = `id::text,version,propietario_id::text,arrendatario_id::text,terminos_canonicos,encode(documento_sha256,'hex'),clave_version,identidades_cifradas,creada_en`

func scan(row pgx.Row) (commercialproposal.Record, error) {
	var r commercialproposal.Record
	var canonical string
	err := row.Scan(&r.ID, &r.Version, &r.OwnerID, &r.RenterID, &canonical, &r.SHA256, &r.KeyVersion, &r.Ciphertext, &r.CreatedAt)
	if err == nil {
		err = json.Unmarshal([]byte(canonical), &r.Terms)
	}
	return r, err
}
func (r *Repository) Create(ctx context.Context, actor string, quote rentalflow.Quote, record commercialproposal.Record, revalidate func() error) (commercialproposal.Record, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)
	if err = accountlock.LockAccountIDs(ctx, tx, record.OwnerID, record.RenterID); err != nil {
		return record, err
	}
	var space string
	err = tx.QueryRow(ctx, `SELECT id::text FROM public.espacio WHERE id=$1 AND propietario_id=$2 AND estado='activa' FOR UPDATE`, quote.SpaceID, quote.OwnerID).Scan(&space)
	if errors.Is(err, pgx.ErrNoRows) {
		return record, commercialproposal.ErrConflict
	}
	if err != nil {
		return record, err
	}
	if revalidate == nil {
		return record, commercialproposal.ErrEvidenceUnavailable
	}
	if err = revalidate(); err != nil {
		return record, err
	}
	terms, err := currentTerms(ctx, tx, actor, quote, record.Terms.CancellationPolicy)
	if err != nil {
		return record, err
	}
	if terms.CompanyRepresentation != nil {
		provided := record.Terms.CompanyRepresentation
		if provided == nil || provided.OrganizationID != terms.CompanyRepresentation.OrganizationID || provided.RepresentativeUserID == "" || !provided.RUTVerified || provided.MaskedRUT == "" || !provided.RepresentativeRUTVerified || provided.RepresentativeRUT == "" || provided.LegalName == "" || provided.RepresentativeName == "" {
			return record, commercialproposal.ErrConflict
		}
		if _, parseErr := uuid.Parse(provided.RepresentativeUserID); parseErr != nil {
			return record, commercialproposal.ErrConflict
		}
		var repActive bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=$1 AND m.usuario_id=$2 AND m.rol='representante_legal' AND m.estado='activo')`, terms.CompanyRepresentation.OrganizationID, provided.RepresentativeUserID).Scan(&repActive); err != nil {
			return record, err
		}
		if !repActive {
			return record, commercialproposal.ErrConflict
		}
		terms.CompanyRepresentation = provided
	} else if record.Terms.CompanyRepresentation != nil {
		return record, commercialproposal.ErrConflict
	}
	expected, _ := json.Marshal(record.Terms)
	canonical, _ := json.Marshal(terms)
	if string(expected) != string(canonical) {
		return record, commercialproposal.ErrConflict
	}
	prior, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_propuesta_comercial WHERE cotizacion_id=$1 AND documento_sha256=decode($2,'hex')`, quote.ID, record.SHA256))
	if err == nil {
		prior.CompanyRepresentativeAuthorized = terms.CompanyRepresentation != nil
		return prior, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return record, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM public.contrato_propuesta_comercial WHERE cotizacion_id=$1`, quote.ID).Scan(&record.Version); err != nil {
		return record, err
	}
	company := terms.CompanyRepresentation
	var isB2B bool
	var orgID, legalName, companyRUT, repID, repName, repRUT any
	var companyRUTVerified, repRUTVerified bool
	if company != nil {
		isB2B, orgID, legalName, companyRUT, repID, repName, repRUT = true, company.OrganizationID, company.LegalName, company.MaskedRUT, company.RepresentativeUserID, company.RepresentativeName, company.RepresentativeRUT
		companyRUTVerified, repRUTVerified = company.RUTVerified, company.RepresentativeRUTVerified
	}
	stored, err := scan(tx.QueryRow(ctx, `INSERT INTO public.contrato_propuesta_comercial(id,cotizacion_id,espacio_id,propietario_id,arrendatario_id,version,plantilla_version_id,inicio,termino,zona_horaria,subtotal_clp,moneda,terminos,terminos_canonicos,terminos_sha256,documento_sha256,identidades_cifradas,clave_version,es_b2b,organizacion_id,empresa_razon_social_snapshot,empresa_rut_masked_snapshot,empresa_rut_verificada,representante_legal_id,representante_nombre_snapshot,representante_rut_masked_snapshot,representante_rut_verificada)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,decode($15,'hex'),decode($16,'hex'),$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27) RETURNING `+columns, record.ID, quote.ID, quote.SpaceID, record.OwnerID, record.RenterID, record.Version, terms.TemplateID, terms.StartAt, terms.EndAt, terms.TimeZone, terms.SubtotalCLP, terms.Currency, canonical, string(canonical), commercialproposal.Digest(canonical), record.SHA256, record.Ciphertext, record.KeyVersion, isB2B, orgID, legalName, companyRUT, companyRUTVerified, repID, repName, repRUT, repRUTVerified))
	if err != nil {
		return record, err
	}
	stored.CompanyRepresentativeAuthorized = terms.CompanyRepresentation != nil
	return stored, tx.Commit(ctx)
}
func (r *Repository) Get(ctx context.Context, actor, id string) (commercialproposal.Record, error) {
	record, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM public.contrato_propuesta_comercial p WHERE p.id=$1 AND ($2=p.propietario_id OR $2=p.arrendatario_id OR (p.es_b2b AND ($2=p.representante_legal_id OR EXISTS (SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=p.organizacion_id AND m.usuario_id=$2 AND m.rol='representante_legal' AND m.estado='activo'))))`, id, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return record, commercialproposal.ErrNotFound
	}
	if err == nil && record.Terms.CompanyRepresentation != nil {
		record.CompanyRepresentativeAuthorized = true
	}
	return record, err
}

func (r *Repository) IsPaid(ctx context.Context, proposalID string) (bool, error) {
	var paid bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.contrato_solicitud_comercial s JOIN public.checkout_preferencia_comercial c ON c.solicitud_comercial_id=s.id JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id WHERE s.propuesta_id=$1 AND c.estado='approved' AND c.payment_verified_at IS NOT NULL AND r.estado='pagada')`, proposalID).Scan(&paid)
	return paid, err
}
