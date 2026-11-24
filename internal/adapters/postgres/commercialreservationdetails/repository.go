package commercialreservationdetailspg

import (
	"context"
	"errors"
	"regexp"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialreservationdetail"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Database interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Repository struct{ db Database }

func New(db Database) *Repository { return &Repository{db: db} }

// Contacts and artifact metadata are joined only under this same paid,
// provider-verified predicate. No pre-payment contact row enters the projection.
const verified = `r.estado='pagada' AND c.estado='approved' AND c.provider_payment_status='approved' AND c.payment_verified_at IS NOT NULL AND c.provider_payment_id IS NOT NULL AND c.base_clp=r.subtotal_clp AND r.moneda='CLP'`
const query = `SELECT r.id::text,CASE WHEN r.anfitrion_id=$2 THEN 'anfitrion' ELSE 'arrendatario' END,
r.estado,c.estado,COALESCE(c.provider_payment_status,''),COALESCE(c.provider_payment_id,''),c.payment_verified_at,r.subtotal_clp,c.base_clp,r.moneda,
d.estado,d.version,encode(d.pdf_sha256,'hex'),d.pdf_bytes,d.lista_en,d.object_id::text,d.bucket_name,encode(d.documento_sha256,'hex'),COALESCE(d.object_version,''),
p.nombre_visible,u.correo_original,p.telefono_normalizado,
td.id::text,encode(td.sha256,'hex'),td.size_bytes,td.uploaded_at
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
LEFT JOIN public.contrato_documento_comercial d ON d.operacion_id=c.id AND d.solicitud_id=c.solicitud_comercial_id AND d.version=c.contrato_version AND d.documento_sha256=c.contrato_sha256 AND (` + verified + `)
LEFT JOIN public.usuario u ON u.id=CASE WHEN r.anfitrion_id=$2 THEN r.arrendatario_id ELSE r.anfitrion_id END AND u.estado='activo' AND (` + verified + `)
LEFT JOIN public.perfil_usuario p ON p.usuario_id=u.id
LEFT JOIN public.reserva_documento_tributario_comercial td ON td.reserva_id=r.id AND (` + verified + `)
WHERE r.id=$1 AND (r.anfitrion_id=$2 OR r.arrendatario_id=$2)`

var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var providerID = regexp.MustCompile(`^[0-9]{1,64}$`)

func (r *Repository) Get(ctx context.Context, actor, id string) (domain.Record, error) {
	var result domain.Record
	var status, checkoutStatus, providerStatus, reference, currency string
	var verifiedAt *time.Time
	var amount, base int64
	var state, sha, object, bucket, docsha *string
	var version, size *int64
	var readyAt *time.Time
	var display, email, phone *string
	var taxID, taxSHA *string
	var taxSize *int64
	var taxUploadedAt *time.Time
	err := r.db.QueryRow(ctx, query, id, actor).Scan(&result.Documents.ReservationID, &result.Documents.ParticipantRole, &status, &checkoutStatus, &providerStatus, &reference, &verifiedAt, &amount, &base, &currency, &state, &version, &sha, &size, &readyAt, &object, &bucket, &docsha, &result.ObjectVersion, &display, &email, &phone, &taxID, &taxSHA, &taxSize, &taxUploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if status != "pagada" {
		return domain.Record{}, domain.ErrPaymentRequired
	}
	if checkoutStatus != "approved" || providerStatus != "approved" || verifiedAt == nil || verifiedAt.IsZero() || !providerID.MatchString(reference) || amount < 1 || amount != base || currency != "CLP" {
		return domain.Record{}, domain.ErrUnverified
	}
	result.Documents.Receipt = domain.Receipt{Kind: "internal_payment_receipt", Status: "approved", AmountCLP: amount, Currency: currency, Provider: "mercadopago", ProviderReference: reference, VerifiedAt: verifiedAt.UTC()}
	result.Documents.Contract.State = "unavailable"
	if state != nil {
		switch *state {
		case "pending", "processing", "retry", "ready", "terminal":
			result.Documents.Contract.State = *state
		default:
			return domain.Record{}, domain.ErrUnavailable
		}
		result.Documents.Contract.Version = version
	}
	if state != nil && *state == "ready" {
		if version == nil || *version < 1 || sha == nil || !hash.MatchString(*sha) || size == nil || *size < 1 || *size > domain.MaxPDFBytes || readyAt == nil || readyAt.IsZero() || object == nil || bucket == nil || *bucket == "" || docsha == nil || !hash.MatchString(*docsha) {
			return domain.Record{}, domain.ErrUnavailable
		}
		result.ObjectID = *object
		result.Bucket = *bucket
		result.PDFSHA256 = *sha
		result.DocumentSHA256 = *docsha
		result.Version = *version
		result.Size = *size
		result.Documents.Contract.SHA256 = sha
		result.Documents.Contract.SizeBytes = size
		result.Documents.Contract.AvailableAt = readyAt
	}
	if display != nil || email != nil || phone != nil {
		role := "anfitrion"
		if result.Documents.ParticipantRole == "anfitrion" {
			role = "arrendatario"
		}
		if phone != nil && *phone != "" {
			normalized := "+56" + *phone
			phone = &normalized
		}
		result.Documents.Counterpart = &domain.Contact{Role: role, DisplayName: display, Email: email, Phone: phone}
	}
	result.Documents.TaxDocument = domain.TaxDocument{State: "not_attached", Notice: "EspaciGo no emite ni valida documentos tributarios del arriendo. El anfitrión es responsable de entregarlos cuando corresponda."}
	if taxID != nil {
		if !validUUID(*taxID) {
			return domain.Record{}, domain.ErrUnavailable
		}
		if taxSHA == nil || !hash.MatchString(*taxSHA) || taxSize == nil || *taxSize < 1 || *taxSize > 10*1024*1024 || taxUploadedAt == nil || taxUploadedAt.IsZero() {
			return domain.Record{}, domain.ErrUnavailable
		}
		result.Documents.TaxDocument = domain.TaxDocument{State: "ready", Notice: "Documento adjunto por el anfitrión. EspaciGo no emite ni valida documentos tributarios."}
	}
	return result, nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
