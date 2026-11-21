package rentalflowpg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
)

type commercialReservationCursor struct {
	Version  int       `json:"v"`
	Actor    string    `json:"actor"`
	PageSize int       `json:"page_size"`
	Updated  time.Time `json:"updated_at"`
	ID       string    `json:"id"`
}

func encodeCommercialReservationCursor(c commercialReservationCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeCommercialReservationCursor(encoded, actor string, pageSize int) (*commercialReservationCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 4096 {
		return nil, rentalflow.ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(b) > 2048 {
		return nil, rentalflow.ErrInvalid
	}
	var c commercialReservationCursor
	if json.Unmarshal(b, &c) != nil || c.Version != 1 || c.Actor != actor || c.PageSize != pageSize || c.Updated.IsZero() || c.ID == "" {
		return nil, rentalflow.ErrInvalid
	}
	_, offset := c.Updated.Zone()
	if !rentalflow.ValidUUID(c.ID) || offset != 0 {
		return nil, rentalflow.ErrInvalid
	}
	return &c, nil
}

func (r *Repository) ListCommercialReservations(ctx context.Context, actor string, pageSize int, encodedCursor string) (rentalflow.CommercialReservationPage, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) || pageSize < 1 || pageSize > 100 {
		return rentalflow.CommercialReservationPage{}, rentalflow.ErrInvalid
	}
	actor = strings.ToLower(actor)
	cursor, err := decodeCommercialReservationCursor(encodedCursor, actor, pageSize)
	if err != nil {
		return rentalflow.CommercialReservationPage{}, err
	}
	var lastUpdated any
	var lastID any
	if cursor != nil {
		lastUpdated = cursor.Updated.UTC()
		lastID = cursor.ID
	}
	rows, err := r.db.Query(ctx, `SELECT r.id::text,r.cotizacion_id::text,r.espacio_id::text,q.titulo_espacio,
CASE WHEN r.anfitrion_id=$1 THEN 'anfitrion' ELSE 'arrendatario' END,
r.estado,c.estado,r.modalidad,r.precio_unitario_clp,r.unidades,r.subtotal_clp,r.moneda,
r.inicio,r.termino,r.zona_horaria,r.pago_vence_en,r.creada_en,r.actualizada_en
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
WHERE (r.anfitrion_id=$1 OR r.arrendatario_id=$1)
  AND ($2::timestamptz IS NULL OR (r.actualizada_en,r.id)<($2::timestamptz,$3::uuid))
ORDER BY r.actualizada_en DESC,r.id DESC LIMIT $4`, actor, lastUpdated, lastID, pageSize+1)
	if err != nil {
		return rentalflow.CommercialReservationPage{}, err
	}
	defer rows.Close()
	page := rentalflow.CommercialReservationPage{Items: make([]rentalflow.CommercialReservation, 0, pageSize)}
	for rows.Next() {
		var item rentalflow.CommercialReservation
		var start, end, paymentExpires, created, updated time.Time
		if err := rows.Scan(&item.ID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle, &item.ParticipantRole, &item.Status, &item.CheckoutStatus, &item.RateUnit, &item.UnitPriceCLP, &item.Units, &item.SubtotalCLP, &item.Currency, &start, &end, &item.TimeZone, &paymentExpires, &created, &updated); err != nil {
			return rentalflow.CommercialReservationPage{}, err
		}
		item.StartAt = start.UTC().Format(time.RFC3339Nano)
		item.EndAt = end.UTC().Format(time.RFC3339Nano)
		item.PaymentExpiresAt = paymentExpires.UTC().Format(time.RFC3339Nano)
		item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		item.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return rentalflow.CommercialReservationPage{}, err
	}
	if len(page.Items) > pageSize {
		last := page.Items[pageSize-1]
		page.Items = page.Items[:pageSize]
		updated, parseErr := time.Parse(time.RFC3339Nano, last.UpdatedAt)
		if parseErr != nil {
			return rentalflow.CommercialReservationPage{}, parseErr
		}
		page.NextCursor, err = encodeCommercialReservationCursor(commercialReservationCursor{Version: 1, Actor: actor, PageSize: pageSize, Updated: updated.UTC(), ID: last.ID})
		if err != nil {
			return rentalflow.CommercialReservationPage{}, err
		}
	}
	return page, nil
}

func (r *Repository) GetCommercialReservation(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) || !rentalflow.ValidUUID(reservationID) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrInvalid
	}
	var item rentalflow.CommercialReservation
	var start, end, paymentExpires, created, updated time.Time
	err := r.db.QueryRow(ctx, `SELECT r.id::text,r.cotizacion_id::text,r.espacio_id::text,q.titulo_espacio,
CASE WHEN r.anfitrion_id=$2 THEN 'anfitrion' ELSE 'arrendatario' END,
r.estado,c.estado,r.modalidad,r.precio_unitario_clp,r.unidades,r.subtotal_clp,r.moneda,
r.inicio,r.termino,r.zona_horaria,r.pago_vence_en,r.creada_en,r.actualizada_en
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
WHERE r.id=$1 AND (r.anfitrion_id=$2 OR r.arrendatario_id=$2)`, reservationID, actor).Scan(
		&item.ID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle, &item.ParticipantRole, &item.Status, &item.CheckoutStatus, &item.RateUnit,
		&item.UnitPriceCLP, &item.Units, &item.SubtotalCLP, &item.Currency, &start, &end, &item.TimeZone, &paymentExpires, &created, &updated,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
	}
	if err != nil {
		return rentalflow.CommercialReservation{}, err
	}
	item.StartAt = start.UTC().Format(time.RFC3339Nano)
	item.EndAt = end.UTC().Format(time.RFC3339Nano)
	item.PaymentExpiresAt = paymentExpires.UTC().Format(time.RFC3339Nano)
	item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
	return item, nil
}
