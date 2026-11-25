package commercialrentaldocumentspg

import (
	"context"
	"errors"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialrentaldocument"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Database interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Repository struct{ db Database }

func New(db Database) *Repository { return &Repository{db: db} }

const verifiedPaid = `r.estado='pagada' AND c.estado='approved' AND c.provider_payment_status='approved' AND c.payment_verified_at IS NOT NULL AND c.provider_payment_id IS NOT NULL AND c.base_clp=r.subtotal_clp AND r.moneda='CLP'`

func (r *Repository) AuthorizeUpload(ctx context.Context, actor, reservationID string) error {
	var allowed bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_arriendo_comercial r JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id WHERE r.id=$1 AND r.anfitrion_id=$2 AND `+verifiedPaid+")", reservationID, actor).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, actor string, doc domain.Document) error {
	tag, err := r.db.Exec(ctx, `INSERT INTO public.reserva_documento_tributario_comercial
(id,reserva_id,anfitrion_id,object_id,bucket_name,sha256,size_bytes,inspected_at,uploaded_at)
SELECT $1,$2,$3,$4,$5,decode($6,'hex'),$7,$8,$9
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
WHERE r.id=$2 AND r.anfitrion_id=$3 AND `+verifiedPaid,
		doc.ID, doc.ReservationID, actor, doc.ObjectID, doc.Bucket, doc.SHA256, doc.Size, doc.InspectedAt, doc.UploadedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return domain.ErrConflict
			}
			if pgErr.Code == "23514" {
				return domain.ErrNotFound
			}
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, actor, reservationID string) (domain.Document, error) {
	var doc domain.Document
	err := r.db.QueryRow(ctx, `SELECT d.id::text,d.reserva_id::text,d.anfitrion_id::text,d.object_id::text,d.bucket_name,encode(d.sha256,'hex'),d.size_bytes,d.inspected_at,d.uploaded_at
FROM public.reserva_documento_tributario_comercial d
JOIN public.reserva_arriendo_comercial r ON r.id=d.reserva_id
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
WHERE d.reserva_id=$2 AND (r.anfitrion_id=$1 OR r.arrendatario_id=$1) AND `+verifiedPaid,
		actor, reservationID).Scan(&doc.ID, &doc.ReservationID, &doc.HostID, &doc.ObjectID, &doc.Bucket, &doc.SHA256, &doc.Size, &doc.InspectedAt, &doc.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Document{}, err
	}
	return doc, nil
}
