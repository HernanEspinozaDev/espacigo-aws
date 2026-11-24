package rentalflowpg

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/accountlock"
	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RequireCommercialContracts configures the repository before serving traffic.
// The composition root enables this only from an explicit runtime flag.
func (r *Repository) RequireCommercialContracts(required bool) { r.contractsRequired = required }

type commercialContractAuthorization struct {
	RequestID, State, OccupancyID, OccupancyType string
	Version                                      int64
	Digest                                       []byte
	ExpiresAt, OccupancyExpiry                   time.Time
	Active, Bound, CurrentLocation               bool
}

const commercialContractAuthorizationSQL = `SELECT s.id::text,s.estado,s.propuesta_version,s.documento_sha256,COALESCE(s.pago_vence_en,'epoch'::timestamptz),o.id::text,o.tipo,o.activo,COALESCE(o.expira_en,'epoch'::timestamptz),
 (p.cotizacion_id=q.id AND p.espacio_id=q.espacio_id AND p.propietario_id=q.propietario_id AND p.arrendatario_id=q.arrendatario_id AND p.version=s.propuesta_version AND p.documento_sha256=s.documento_sha256 AND p.inicio=q.inicio AND p.termino=q.termino AND p.zona_horaria=q.zona_horaria AND p.subtotal_clp=q.subtotal_clp AND p.moneda=q.moneda AND (p.terminos->>'rate_version')::bigint=q.tarifa_version AND p.terminos->>'rate_unit'=q.modalidad AND (p.terminos->>'unit_price_clp')::bigint=q.precio_unitario_clp AND (p.terminos->>'units')::bigint=q.unidades AND o.espacio_id=q.espacio_id AND o.intervalo=tstzrange(q.inicio,q.termino,'[)') AND
 (SELECT count(*) FROM public.contrato_firma_comercial f WHERE f.solicitud_id=s.id AND f.propuesta_version=s.propuesta_version AND f.documento_sha256=s.documento_sha256 AND f.consentimiento_version='commercial-agreement-v1' AND ((f.rol='arrendador' AND f.actor_id=s.propietario_id) OR (f.rol='arrendatario' AND f.actor_id=s.arrendatario_id)))=2),
 (e.estado='activa' AND e.propietario_id=s.propietario_id AND e.ubicacion_confirmada_en IS NOT NULL AND e.ubicacion_revision=(p.terminos->'location'->>'revision')::bigint)
FROM public.contrato_solicitud_comercial s JOIN public.contrato_propuesta_comercial p ON p.id=s.propuesta_id JOIN public.cotizacion_arriendo_comercial q ON q.id=s.cotizacion_id JOIN public.ocupacion o ON o.id=s.ocupacion_id AND o.solicitud_comercial_id=s.id JOIN public.espacio e ON e.id=s.espacio_id WHERE s.cotizacion_id=$1 AND s.arrendatario_id=$2`

func loadCommercialContractAuthorization(ctx context.Context, db DB, quoteID, renterID string, lock bool) (commercialContractAuthorization, error) {
	return loadCommercialContractAuthorizationForRequest(ctx, db, quoteID, renterID, "", lock)
}
func loadCommercialContractAuthorizationForRequest(ctx context.Context, db DB, quoteID, renterID, requestID string, lock bool) (commercialContractAuthorization, error) {
	var a commercialContractAuthorization
	query := commercialContractAuthorizationSQL + ` AND ($3::uuid IS NULL OR s.id=$3::uuid) ORDER BY CASE WHEN s.estado='ready_for_payment' AND o.activo AND o.tipo='retencion_contrato_comercial' AND o.expira_en>now() THEN 0 ELSE 1 END,s.creada_en DESC,s.id DESC LIMIT 1`
	var request any
	if requestID != "" {
		request = requestID
	}
	if lock {
		query += ` FOR UPDATE OF s,o`
	}
	err := db.QueryRow(ctx, query, quoteID, renterID, request).Scan(&a.RequestID, &a.State, &a.Version, &a.Digest, &a.ExpiresAt, &a.OccupancyID, &a.OccupancyType, &a.Active, &a.OccupancyExpiry, &a.Bound, &a.CurrentLocation)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, checkoutpro.ErrConflict
	}
	return a, err
}
func (a commercialContractAuthorization) canStart(now time.Time) bool {
	return a.Bound && a.CurrentLocation && a.State == "ready_for_payment" && a.Active && a.OccupancyType == "retencion_contrato_comercial" && a.ExpiresAt.After(now) && a.OccupancyExpiry.Equal(a.ExpiresAt)
}

const commercialContractCheckoutSelectSQL = `SELECT id::text,cotizacion_id::text,estado,COALESCE(preference_id,''),COALESCE(init_point,''),vence_en,actualizada_en,fingerprint,base_clp,commission_clp,iva_clp,buyer_commission_clp,buyer_iva_clp,buyer_fee_clp,total_charged_clp,marketplace_fee_clp,test_mode,checkout_method,COALESCE(provider_payment_id,''),COALESCE(solicitud_comercial_id::text,''),COALESCE(contrato_version,0),contrato_sha256 FROM public.checkout_preferencia_comercial`

func scanCommercialContractOp(row pgx.Row) (checkoutpro.Operation, []byte, string, int64, []byte, error) {
	var op checkoutpro.Operation
	var oldFingerprint, bindingDigest []byte
	var requestID string
	var version int64
	err := row.Scan(&op.ID, &op.QuoteID, &op.Status, &op.PreferenceID, &op.InitPoint, &op.ExpiresAt, &op.UpdatedAt, &oldFingerprint, &op.Amounts.BaseCLP, &op.Amounts.CommissionCLP, &op.Amounts.VATCLP, &op.Amounts.BuyerCommissionCLP, &op.Amounts.BuyerVATCLP, &op.Amounts.BuyerFeeCLP, &op.Amounts.TotalChargedCLP, &op.Amounts.MarketplaceFeeCLP, &op.TestMode, &op.CheckoutMethod, &op.ProviderPaymentID, &requestID, &version, &bindingDigest)
	return op, oldFingerprint, requestID, version, bindingDigest, err
}

func (a commercialContractAuthorization) matchesOperation(op checkoutpro.Operation, requestID string, version int64, bindingDigest []byte, now time.Time, requirePendingHold bool) bool {
	if !a.Bound || a.RequestID != requestID || a.Version != version || !equalBytes(a.Digest, bindingDigest) {
		return false
	}
	if (requirePendingHold || op.Status == "pending") && (!a.Active || a.OccupancyType != "retencion_comercial" || a.State != "ready_for_payment" || !a.CurrentLocation || !a.OccupancyExpiry.Equal(op.ExpiresAt) || !op.ExpiresAt.After(now)) {
		return false
	}
	return true
}
func loadCommercialCheckoutQuote(ctx context.Context, db DB, renterID, quoteID string) (checkoutpro.Quote, error) {
	var q checkoutpro.Quote
	err := db.QueryRow(ctx, `SELECT id::text,espacio_id::text,propietario_id::text,arrendatario_id::text,titulo_espacio,tarifa_version,modalidad,moneda,precio_unitario_clp,unidades,subtotal_clp,inicio,termino,zona_horaria,vence_en FROM public.cotizacion_arriendo_comercial WHERE id=$1 AND arrendatario_id=$2`, quoteID, renterID).Scan(&q.ID, &q.SpaceID, &q.HostID, &q.RenterID, &q.Title, &q.RateVersion, &q.RateUnit, &q.Currency, &q.UnitPriceCLP, &q.Units, &q.SubtotalCLP, &q.StartAt, &q.EndAt, &q.TimeZone, &q.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, checkoutpro.ErrNotFound
	}
	return q, err
}

func (r *Repository) beginContractCheckout(ctx context.Context, renterID, quoteID, key, operationID string, fingerprint []byte, method string, testMode bool, now time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	defer tx.Rollback(context.Background())
	q, err := loadCommercialCheckoutQuote(ctx, tx, renterID, quoteID)
	if err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	accounts := []string{q.HostID, q.RenterID}
	sort.Strings(accounts)
	for _, id := range accounts {
		active, err := accountlock.LockActive(ctx, tx, id)
		if err != nil {
			return q, checkoutpro.Operation{}, false, err
		}
		if !active {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		}
	}
	if err = LockCommercialAgreementInterval(ctx, tx, q.ID, q.SpaceID); err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	var replayRequestID string
	err = tx.QueryRow(ctx, `SELECT COALESCE(solicitud_comercial_id::text,'') FROM public.checkout_preferencia_comercial WHERE arrendatario_id=$1 AND cotizacion_id=$2 AND idempotency_key=$3`, renterID, quoteID, key).Scan(&replayRequestID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return q, checkoutpro.Operation{}, false, err
	}
	a, err := loadCommercialContractAuthorizationForRequest(ctx, tx, quoteID, renterID, replayRequestID, true)
	if err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	op, oldFingerprint, requestID, version, bindingDigest, err := scanCommercialContractOp(tx.QueryRow(ctx, commercialContractCheckoutSelectSQL+` WHERE arrendatario_id=$1 AND cotizacion_id=$2 AND idempotency_key=$3 FOR UPDATE`, renterID, quoteID, key))
	if err == nil {
		if !equalBytes(oldFingerprint, fingerprint) || op.CheckoutMethod != method || op.TestMode != testMode {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrIdempotencyConflict
		}
		if !a.matchesOperation(op, requestID, version, bindingDigest, now, false) {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		}
		q.ExpiresAt = op.ExpiresAt
		if err = tx.Commit(ctx); err != nil {
			return q, checkoutpro.Operation{}, false, err
		}
		return q, op, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return q, checkoutpro.Operation{}, false, err
	}
	if a.RequestID != "" {
		op, _, requestID, version, bindingDigest, err = scanCommercialContractOp(tx.QueryRow(ctx, commercialContractCheckoutSelectSQL+` WHERE solicitud_comercial_id=$1 AND arrendatario_id=$2 AND cotizacion_id=$3 AND estado='pending' AND checkout_method=$4 AND test_mode=$5 AND vence_en>$6 ORDER BY creada_en DESC LIMIT 1 FOR UPDATE`, a.RequestID, renterID, quoteID, method, testMode, now))
		if err == nil {
			if !a.matchesOperation(op, requestID, version, bindingDigest, now, true) {
				return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
			}
			q.ExpiresAt = op.ExpiresAt
			if err = tx.Commit(ctx); err != nil {
				return q, checkoutpro.Operation{}, false, err
			}
			return q, op, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return q, checkoutpro.Operation{}, false, err
		}
	}
	if !a.canStart(now) {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	q.ExpiresAt = a.ExpiresAt
	amounts, err := checkoutpro.CalculateAmounts(q.SubtotalCLP)
	if err != nil {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	if method == "payments_api" {
		amounts = checkoutpro.Amounts{BaseCLP: q.SubtotalCLP, TotalChargedCLP: q.SubtotalCLP}
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.checkout_preferencia_comercial(id,cotizacion_id,espacio_id,arrendatario_id,host_id,idempotency_key,fingerprint,estado,base_clp,commission_clp,iva_clp,buyer_commission_clp,buyer_iva_clp,buyer_fee_clp,total_charged_clp,marketplace_fee_clp,test_mode,vence_en,creada_en,actualizada_en,checkout_method,solicitud_comercial_id,contrato_version,contrato_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18,$19,$20,$21,$22)`, operationID, q.ID, q.SpaceID, renterID, q.HostID, key, fingerprint, amounts.BaseCLP, amounts.CommissionCLP, amounts.VATCLP, amounts.BuyerCommissionCLP, amounts.BuyerVATCLP, amounts.BuyerFeeCLP, amounts.TotalChargedCLP, amounts.MarketplaceFeeCLP, testMode, a.ExpiresAt, now, method, a.RequestID, a.Version, a.Digest)
	if err != nil {
		return q, checkoutpro.Operation{}, false, mapCommercialCheckoutConflict(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.reserva_arriendo_comercial(id,operacion_checkout_id,cotizacion_id,espacio_id,anfitrion_id,arrendatario_id,estado,tarifa_version,modalidad,precio_unitario_clp,unidades,subtotal_clp,moneda,inicio,termino,zona_horaria,ocupacion_id,pago_vence_en,creada_en,actualizada_en) VALUES($1,$1,$2,$3,$4,$5,'pendiente_de_pago',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, operationID, q.ID, q.SpaceID, q.HostID, renterID, q.RateVersion, q.RateUnit, q.UnitPriceCLP, q.Units, q.SubtotalCLP, q.Currency, q.StartAt, q.EndAt, q.TimeZone, a.OccupancyID, a.ExpiresAt, now)
	if err != nil {
		return q, checkoutpro.Operation{}, false, mapCommercialCheckoutConflict(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE public.ocupacion SET tipo='retencion_comercial',reserva_arriendo_comercial_id=$2 WHERE id=$1 AND solicitud_comercial_id=$3 AND tipo='retencion_contrato_comercial' AND activo AND expira_en=$4 AND expira_en>$5`, a.OccupancyID, operationID, a.RequestID, a.ExpiresAt, now)
	if err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	if tag.RowsAffected() != 1 {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	op = checkoutpro.Operation{ID: operationID, QuoteID: q.ID, Status: "pending", CheckoutMethod: method, ExpiresAt: a.ExpiresAt, UpdatedAt: now, Amounts: amounts, TestMode: testMode}
	if err = tx.Commit(ctx); err != nil {
		return q, checkoutpro.Operation{}, false, mapCommercialCheckoutConflict(err)
	}
	return q, op, true, nil
}

func mapCommercialCheckoutConflict(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23505" || p.Code == "23P01" || p.Code == "23514") {
		return checkoutpro.ErrConflict
	}
	return err
}

func (r *Repository) contractQuoteForStatus(ctx context.Context, q checkoutpro.Quote, op *checkoutpro.Operation) (checkoutpro.Quote, error) {
	if op == nil {
		a, err := loadCommercialContractAuthorization(ctx, r.db, q.ID, q.RenterID, false)
		if err != nil {
			return q, err
		}
		if !a.canStart(r.now().UTC()) {
			return q, checkoutpro.ErrConflict
		}
		q.ExpiresAt = a.ExpiresAt
		return q, nil
	}
	var requestID string
	var version int64
	var digest []byte
	err := r.db.QueryRow(ctx, `SELECT COALESCE(solicitud_comercial_id::text,''),COALESCE(contrato_version,0),contrato_sha256 FROM public.checkout_preferencia_comercial WHERE id=$1`, op.ID).Scan(&requestID, &version, &digest)
	if err != nil {
		return q, err
	}
	if requestID == "" {
		return q, checkoutpro.ErrConflict
	}
	a, err := loadCommercialContractAuthorizationForRequest(ctx, r.db, q.ID, q.RenterID, requestID, false)
	if err != nil {
		return q, err
	}
	if !a.Bound || a.Version != version || !equalBytes(a.Digest, digest) {
		return q, checkoutpro.ErrConflict
	}
	q.ExpiresAt = op.ExpiresAt
	return q, nil
}

func (r *Repository) contractQuotePurgeSQL(query string) string {
	if !r.contractsRequired {
		return query
	}
	return strings.Replace(query, "ORDER BY vence_en,id", "AND NOT EXISTS(SELECT 1 FROM public.contrato_solicitud_comercial s WHERE s.cotizacion_id=q.id AND s.estado IN ('pending_landlord','ready_for_payment'))\nORDER BY vence_en,id", 1)
}
func preservedCommercialQuoteRace(err error) bool {
	var p *pgconn.PgError
	return errors.As(err, &p) && p.Code == "23503"
}

func commercialContractPaymentBindingValid(ctx context.Context, db DB, operationID, renterID string) (bool, error) {
	var quoteID, requestID string
	var binding bool
	err := db.QueryRow(ctx, `SELECT c.cotizacion_id::text,COALESCE(c.solicitud_comercial_id::text,''),EXISTS(SELECT 1 FROM public.contrato_solicitud_comercial s WHERE s.id=c.solicitud_comercial_id AND s.cotizacion_id=c.cotizacion_id AND s.espacio_id=c.espacio_id AND s.propietario_id=c.host_id AND s.arrendatario_id=c.arrendatario_id AND s.propuesta_version=c.contrato_version AND s.documento_sha256=c.contrato_sha256) FROM public.checkout_preferencia_comercial c WHERE c.id=$1`, operationID).Scan(&quoteID, &requestID, &binding)
	if err != nil {
		return false, err
	}
	if !binding {
		return false, nil
	}
	a, err := loadCommercialContractAuthorizationForRequest(ctx, db, quoteID, renterID, requestID, false)
	if errors.Is(err, checkoutpro.ErrConflict) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.Bound, nil
}
