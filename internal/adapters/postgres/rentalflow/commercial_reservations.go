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
	if r.beginner != nil && r.now != nil {
		_, _ = r.PurgeExpiredCommercialQuotes(ctx, r.now(), 100)
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
r.estado,c.estado,c.checkout_method,COALESCE(c.provider_payment_id,''),
c.commission_clp,c.iva_clp,c.buyer_commission_clp,c.buyer_iva_clp,c.buyer_fee_clp,c.total_charged_clp,c.marketplace_fee_clp,(c.base_clp - (c.commission_clp + c.iva_clp)),
r.modalidad,r.precio_unitario_clp,r.unidades,r.subtotal_clp,r.moneda,
r.inicio,r.termino,r.zona_horaria,r.pago_vence_en,r.creada_en,r.actualizada_en,
COALESCE(q.politica_cancelacion_version,''),COALESCE(q.politica_cancelacion_texto,''),
CASE WHEN r.anfitrion_id=$1 THEN c.provider_settlement END
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
		var policyVersion, policyText string
		if err := rows.Scan(
			&item.ID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle, &item.ParticipantRole,
			&item.Status, &item.CheckoutStatus, &item.CheckoutMethod, &item.ProviderPaymentID,
			&item.CommissionCLP, &item.VATCLP, &item.BuyerCommissionCLP, &item.BuyerVATCLP, &item.BuyerFeeCLP, &item.TotalChargedCLP, &item.MarketplaceFeeCLP, &item.NetHostAmountCLP,
			&item.RateUnit, &item.UnitPriceCLP, &item.Units, &item.SubtotalCLP, &item.Currency,
			&start, &end, &item.TimeZone, &paymentExpires, &created, &updated, &policyVersion, &policyText, &item.ProviderSettlement,
		); err != nil {
			return rentalflow.CommercialReservationPage{}, err
		}
		item.StartAt = start.UTC().Format(time.RFC3339Nano)
		item.EndAt = end.UTC().Format(time.RFC3339Nano)
		item.PaymentExpiresAt = paymentExpires.UTC().Format(time.RFC3339Nano)
		item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		item.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
		if policyVersion != "" {
			item.CancellationPolicyVersion = &policyVersion
		}
		if policyText != "" {
			item.CancellationPolicyText = &policyText
		}
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
	if len(page.Items) > 0 {
		quoteIDs := make([]string, 0, len(page.Items))
		resIDByQuote := make(map[string]string, len(page.Items))
		byQuote := make(map[string][]rentalflow.QuoteInterval, len(page.Items))
		for _, item := range page.Items {
			quoteIDs = append(quoteIDs, item.QuoteID)
			resIDByQuote[item.QuoteID] = item.ID
		}
		tRows, tErr := r.db.Query(ctx, `SELECT t.id::text, t.cotizacion_id::text, t.inicio, t.termino, t.unidades,
			COALESCE(u.estado, 'pendiente'),
			u.check_in_en,
			u.check_out_en,
			COALESCE(u.codigo_corto, upper(substr(md5(r.id::text || ':' || t.id::text), 1, 6))),
			r.id::text,
			r.estado,
			CASE WHEN r.anfitrion_id=$2 THEN 'anfitrion' ELSE 'arrendatario' END
		FROM public.cotizacion_arriendo_tramo t
		JOIN public.reserva_arriendo_comercial r ON r.cotizacion_id = t.cotizacion_id
		LEFT JOIN public.reserva_arriendo_tramo_estado u ON u.reserva_id = r.id AND u.tramo_id = t.id
		WHERE t.cotizacion_id = ANY($1::uuid[])
		ORDER BY t.cotizacion_id, t.orden`, quoteIDs, actor)
		if tErr == nil {
			for tRows.Next() {
				var qid, code, resID, resStatus, role string
				var iv rentalflow.QuoteInterval
				var checkIn, checkOut *time.Time
				if scanErr := tRows.Scan(&iv.ID, &qid, &iv.StartAt, &iv.EndAt, &iv.Units, &iv.UsageStatus, &checkIn, &checkOut, &code, &resID, &resStatus, &role); scanErr == nil {
					iv.StartAt = iv.StartAt.UTC()
					iv.EndAt = iv.EndAt.UTC()
					if checkIn != nil {
						iv.CheckInAt = checkIn.UTC().Format(time.RFC3339Nano)
					}
					if checkOut != nil {
						iv.CheckOutAt = checkOut.UTC().Format(time.RFC3339Nano)
					}
					if resStatus == "pagada" && role == "arrendatario" && code != "" {
						iv.PassCode = "ESP-" + code
						iv.QRPayload = "espacigo-pass:" + resID + ":" + iv.ID + ":" + code
					}
					byQuote[qid] = append(byQuote[qid], iv)
				}
			}
			tRows.Close()
		}
		for i := range page.Items {
			if ivs, ok := byQuote[page.Items[i].QuoteID]; ok && len(ivs) > 0 {
				page.Items[i].Intervals = ivs
			}
		}
	}
	return page, nil
}

func (r *Repository) GetCommercialReservation(ctx context.Context, actor, reservationID string) (rentalflow.CommercialReservation, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) || !rentalflow.ValidUUID(reservationID) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrInvalid
	}
	if r.beginner != nil && r.now != nil {
		_, _ = r.PurgeExpiredCommercialQuotes(ctx, r.now(), 100)
	}
	var item rentalflow.CommercialReservation
	var start, end, paymentExpires, created, updated time.Time
	var policyVersion, policyText string
	err := r.db.QueryRow(ctx, `SELECT r.id::text,r.cotizacion_id::text,r.espacio_id::text,q.titulo_espacio,
CASE WHEN r.anfitrion_id=$2 THEN 'anfitrion' ELSE 'arrendatario' END,
r.estado,c.estado,c.checkout_method,COALESCE(c.provider_payment_id,''),
c.commission_clp,c.iva_clp,c.buyer_commission_clp,c.buyer_iva_clp,c.buyer_fee_clp,c.total_charged_clp,c.marketplace_fee_clp,(c.base_clp - (c.commission_clp + c.iva_clp)),
r.modalidad,r.precio_unitario_clp,r.unidades,r.subtotal_clp,r.moneda,
r.inicio,r.termino,r.zona_horaria,r.pago_vence_en,r.creada_en,r.actualizada_en,
COALESCE(q.politica_cancelacion_version,''),COALESCE(q.politica_cancelacion_texto,''),COALESCE(cc.refund_status,''),CASE WHEN r.anfitrion_id=$2 THEN c.provider_settlement END
FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id
JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
LEFT JOIN public.reserva_comercial_cancelacion cc ON cc.reserva_id=r.id
WHERE r.id=$1 AND (r.anfitrion_id=$2 OR r.arrendatario_id=$2)`, reservationID, actor).Scan(
		&item.ID, &item.QuoteID, &item.SpaceID, &item.SpaceTitle, &item.ParticipantRole,
		&item.Status, &item.CheckoutStatus, &item.CheckoutMethod, &item.ProviderPaymentID,
		&item.CommissionCLP, &item.VATCLP, &item.BuyerCommissionCLP, &item.BuyerVATCLP, &item.BuyerFeeCLP, &item.TotalChargedCLP, &item.MarketplaceFeeCLP, &item.NetHostAmountCLP,
		&item.RateUnit, &item.UnitPriceCLP, &item.Units, &item.SubtotalCLP, &item.Currency,
		&start, &end, &item.TimeZone, &paymentExpires, &created, &updated, &policyVersion, &policyText, &item.RefundStatus, &item.ProviderSettlement,
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
	if policyVersion != "" {
		item.CancellationPolicyVersion = &policyVersion
	}
	if policyText != "" {
		item.CancellationPolicyText = &policyText
	}
	if *item.RefundStatus == "" {
		item.RefundStatus = nil
	}
	tRows, tErr := r.db.Query(ctx, `SELECT t.id::text, t.inicio, t.termino, t.unidades,
		COALESCE(u.estado, 'pendiente'),
		u.check_in_en,
		u.check_out_en,
		COALESCE(u.codigo_corto, upper(substr(md5($2::text || ':' || t.id::text), 1, 6)))
	FROM public.cotizacion_arriendo_tramo t
	LEFT JOIN public.reserva_arriendo_tramo_estado u ON u.reserva_id = $2::uuid AND u.tramo_id = t.id
	WHERE t.cotizacion_id=$1::uuid
	ORDER BY t.orden`, item.QuoteID, item.ID)
	if tErr == nil {
		for tRows.Next() {
			var iv rentalflow.QuoteInterval
			var checkIn, checkOut *time.Time
			var code string
			if scanErr := tRows.Scan(&iv.ID, &iv.StartAt, &iv.EndAt, &iv.Units, &iv.UsageStatus, &checkIn, &checkOut, &code); scanErr == nil {
				iv.StartAt = iv.StartAt.UTC()
				iv.EndAt = iv.EndAt.UTC()
				if checkIn != nil {
					iv.CheckInAt = checkIn.UTC().Format(time.RFC3339Nano)
				}
				if checkOut != nil {
					iv.CheckOutAt = checkOut.UTC().Format(time.RFC3339Nano)
				}
				if item.Status == "pagada" && item.ParticipantRole == "arrendatario" && code != "" {
					iv.PassCode = "ESP-" + code
					iv.QRPayload = "espacigo-pass:" + item.ID + ":" + iv.ID + ":" + code
				}
				item.Intervals = append(item.Intervals, iv)
			}
		}
		tRows.Close()
	}
	return item, nil
}

func (r *Repository) UpdateCommercialIntervalUsage(ctx context.Context, actor, reservationID, intervalID, status string) (rentalflow.CommercialReservation, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) || !rentalflow.ValidUUID(reservationID) || !rentalflow.ValidUUID(intervalID) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrInvalid
	}
	var hostID, resState, quoteID string
	err := r.db.QueryRow(ctx, `SELECT anfitrion_id::text, estado, cotizacion_id::text FROM public.reserva_arriendo_comercial WHERE id=$1::uuid`, reservationID).Scan(&hostID, &resState, &quoteID)
	if errors.Is(err, pgx.ErrNoRows) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
	}
	if err != nil {
		return rentalflow.CommercialReservation{}, err
	}
	if !strings.EqualFold(hostID, actor) {
		return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
	}
	if resState != "pagada" {
		return rentalflow.CommercialReservation{}, rentalflow.ErrConflict
	}
	var tramoExists bool
	if err = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.cotizacion_arriendo_tramo WHERE id=$1::uuid AND cotizacion_id=$2::uuid)`, intervalID, quoteID).Scan(&tramoExists); err != nil {
		return rentalflow.CommercialReservation{}, err
	}
	if !tramoExists {
		return rentalflow.CommercialReservation{}, rentalflow.ErrNotFound
	}
	now := r.now().UTC()
	const upsertSQL = `INSERT INTO public.reserva_arriendo_tramo_estado (reserva_id, tramo_id, estado, check_in_en, check_out_en, actualizado_por, actualizada_en, codigo_corto)
VALUES (
	$1::uuid,
	$2::uuid,
	$3,
	CASE WHEN $3 = 'en_uso' THEN $4 ELSE NULL END,
	CASE WHEN $3 = 'completado' THEN $4 ELSE NULL END,
	$5::uuid,
	$4,
	upper(substr(md5($1::text || ':' || $2::text), 1, 6))
)
ON CONFLICT (reserva_id, tramo_id) DO UPDATE SET
	estado = EXCLUDED.estado,
	codigo_corto = COALESCE(public.reserva_arriendo_tramo_estado.codigo_corto, EXCLUDED.codigo_corto),
	check_in_en = CASE
		WHEN EXCLUDED.estado = 'pendiente' THEN NULL
		WHEN EXCLUDED.estado = 'en_uso' THEN COALESCE(public.reserva_arriendo_tramo_estado.check_in_en, $4)
		ELSE public.reserva_arriendo_tramo_estado.check_in_en
	END,
	check_out_en = CASE
		WHEN EXCLUDED.estado IN ('pendiente', 'en_uso') THEN NULL
		WHEN EXCLUDED.estado = 'completado' THEN $4
		ELSE public.reserva_arriendo_tramo_estado.check_out_en
	END,
	actualizado_por = EXCLUDED.actualizado_por,
	actualizada_en = EXCLUDED.actualizada_en`
	if _, err = r.db.Exec(ctx, upsertSQL, reservationID, intervalID, status, now, actor); err != nil {
		return rentalflow.CommercialReservation{}, err
	}
	return r.GetCommercialReservation(ctx, actor, reservationID)
}

func (r *Repository) VerifyAndApplyPass(ctx context.Context, actor, spaceID, codeOrQR, action string) (rentalflow.VerifiedPassResult, error) {
	if r == nil || r.db == nil || ctx == nil || !rentalflow.ValidUUID(actor) {
		return rentalflow.VerifiedPassResult{}, rentalflow.ErrInvalid
	}
	raw := strings.TrimSpace(codeOrQR)
	var targetReservationID, targetTramoID, shortCode string
	if strings.HasPrefix(raw, "espacigo-pass:") {
		parts := strings.Split(raw, ":")
		if len(parts) != 4 || !rentalflow.ValidUUID(parts[1]) || !rentalflow.ValidUUID(parts[2]) || len(parts[3]) != 6 {
			return rentalflow.VerifiedPassResult{}, rentalflow.ErrInvalid
		}
		targetReservationID = strings.ToLower(parts[1])
		targetTramoID = strings.ToLower(parts[2])
		shortCode = strings.ToUpper(parts[3])
	} else {
		cleaned := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(raw), "ESP-"))
		if len(cleaned) != 6 {
			return rentalflow.VerifiedPassResult{}, rentalflow.ErrInvalid
		}
		shortCode = cleaned
	}

	var matchedResID, matchedTramoID, currentStatus string
	var queryErr error
	if targetReservationID != "" {
		queryErr = r.db.QueryRow(ctx, `SELECT r.id::text, t.id::text, COALESCE(u.estado, 'pendiente')
			FROM public.reserva_arriendo_comercial r
			JOIN public.cotizacion_arriendo_tramo t ON t.cotizacion_id = r.cotizacion_id
			LEFT JOIN public.reserva_arriendo_tramo_estado u ON u.reserva_id = r.id AND u.tramo_id = t.id
			WHERE r.id = $1::uuid AND t.id = $2::uuid AND r.anfitrion_id = $3::uuid AND r.estado = 'pagada'
			  AND ($4 = '' OR r.espacio_id = $4::uuid)
			  AND COALESCE(u.codigo_corto, upper(substr(md5(r.id::text || ':' || t.id::text), 1, 6))) = $5
			LIMIT 1`, targetReservationID, targetTramoID, actor, spaceID, shortCode).Scan(&matchedResID, &matchedTramoID, &currentStatus)
	} else {
		queryErr = r.db.QueryRow(ctx, `SELECT r.id::text, t.id::text, COALESCE(u.estado, 'pendiente')
			FROM public.reserva_arriendo_comercial r
			JOIN public.cotizacion_arriendo_tramo t ON t.cotizacion_id = r.cotizacion_id
			LEFT JOIN public.reserva_arriendo_tramo_estado u ON u.reserva_id = r.id AND u.tramo_id = t.id
			WHERE r.anfitrion_id = $1::uuid AND r.estado = 'pagada'
			  AND ($2 = '' OR r.espacio_id = $2::uuid)
			  AND COALESCE(u.codigo_corto, upper(substr(md5(r.id::text || ':' || t.id::text), 1, 6))) = $3
			LIMIT 1`, actor, spaceID, shortCode).Scan(&matchedResID, &matchedTramoID, &currentStatus)
	}
	if errors.Is(queryErr, pgx.ErrNoRows) {
		return rentalflow.VerifiedPassResult{}, rentalflow.ErrNotFound
	}
	if queryErr != nil {
		return rentalflow.VerifiedPassResult{}, queryErr
	}

	var updatedRes rentalflow.CommercialReservation
	var resErr error
	switch action {
	case "preview":
		updatedRes, resErr = r.GetCommercialReservation(ctx, actor, matchedResID)
	case "check_in":
		if currentStatus == "completado" {
			return rentalflow.VerifiedPassResult{}, rentalflow.ErrConflict
		}
		updatedRes, resErr = r.UpdateCommercialIntervalUsage(ctx, actor, matchedResID, matchedTramoID, "en_uso")
	case "check_out":
		updatedRes, resErr = r.UpdateCommercialIntervalUsage(ctx, actor, matchedResID, matchedTramoID, "completado")
	default:
		return rentalflow.VerifiedPassResult{}, rentalflow.ErrInvalid
	}
	if resErr != nil {
		return rentalflow.VerifiedPassResult{}, resErr
	}
	for _, iv := range updatedRes.Intervals {
		if strings.EqualFold(iv.ID, matchedTramoID) {
			return rentalflow.VerifiedPassResult{Reservation: updatedRes, Interval: iv}, nil
		}
	}
	return rentalflow.VerifiedPassResult{}, rentalflow.ErrNotFound
}
