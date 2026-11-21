package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
)

// Begin stores the preference intent and the interval hold in one transaction.
// Per-space advisory locking serializes competing quotes for overlapping periods.
func (r *Repository) Begin(ctx context.Context, renterID, quoteID, key, operationID string, fingerprint []byte, testMode bool, now time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	return r.begin(ctx, renterID, quoteID, key, operationID, fingerprint, "checkout_pro", testMode, now)
}

// BeginPayment creates the same durable quote reservation and exclusive hold
// used by Checkout Pro, but records the direct Payments API path permanently.
// Payments API is intentionally test-only and never falls back to Checkout Pro.
func (r *Repository) BeginPayment(ctx context.Context, renterID, quoteID, key, operationID string, fingerprint []byte, now time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	return r.begin(ctx, renterID, quoteID, key, operationID, fingerprint, "payments_api", true, now)
}

func (r *Repository) begin(ctx context.Context, renterID, quoteID, key, operationID string, fingerprint []byte, checkoutMethod string, testMode bool, now time.Time) (checkoutpro.Quote, checkoutpro.Operation, bool, error) {
	if r == nil || ctx == nil || r.beginner == nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, checkoutpro.ErrInvalid
	}
	if checkoutMethod != "checkout_pro" && checkoutMethod != "payments_api" {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, checkoutpro.ErrInvalid
	}
	if checkoutMethod == "payments_api" {
		testMode = true
	}
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Serialize operations for this immutable quote without requiring UPDATE on
	// cotizacion_arriendo_comercial. The space lock below serializes other quotes.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('checkout-quote:' || $1, 0))`, quoteID); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	var q checkoutpro.Quote
	// Quote snapshots are immutable and runtime has no UPDATE privilege on the
	// quote table. The later per-space advisory/row lock serializes checkouts;
	// plain SELECT keeps this read within the least-privilege contract.
	err = tx.QueryRow(ctx, `SELECT id::text, espacio_id::text, propietario_id::text, arrendatario_id::text, titulo_espacio, tarifa_version,modalidad,moneda,precio_unitario_clp,unidades,subtotal_clp,inicio,termino,zona_horaria,vence_en FROM public.cotizacion_arriendo_comercial WHERE id=$1 AND arrendatario_id=$2`, quoteID, renterID).Scan(&q.ID, &q.SpaceID, &q.HostID, &q.RenterID, &q.Title, &q.RateVersion, &q.RateUnit, &q.Currency, &q.UnitPriceCLP, &q.Units, &q.SubtotalCLP, &q.StartAt, &q.EndAt, &q.TimeZone, &q.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, checkoutpro.ErrNotFound
	}
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if !q.ExpiresAt.After(now) {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrQuoteExpired
	}
	var op checkoutpro.Operation
	var oldFingerprint []byte
	err = tx.QueryRow(ctx, `SELECT id::text, cotizacion_id::text, estado, COALESCE(preference_id,''), COALESCE(init_point,''), vence_en, actualizada_en, fingerprint,base_clp,commission_clp,iva_clp,marketplace_fee_clp,test_mode,checkout_method,COALESCE(provider_payment_id,'') FROM public.checkout_preferencia_comercial WHERE arrendatario_id=$1 AND cotizacion_id=$2 AND idempotency_key=$3 FOR UPDATE`, renterID, quoteID, key).Scan(&op.ID, &op.QuoteID, &op.Status, &op.PreferenceID, &op.InitPoint, &op.ExpiresAt, &op.UpdatedAt, &oldFingerprint, &op.Amounts.BaseCLP, &op.Amounts.CommissionCLP, &op.Amounts.VATCLP, &op.Amounts.MarketplaceFeeCLP, &op.TestMode, &op.CheckoutMethod, &op.ProviderPaymentID)
	if err == nil {
		if !equalBytes(oldFingerprint, fingerprint) || op.CheckoutMethod != checkoutMethod || op.TestMode != testMode {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrIdempotencyConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		return q, op, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if !q.ExpiresAt.After(now) {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, checkoutpro.ErrQuoteExpired
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, q.SpaceID); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	var activeSpace string
	err = tx.QueryRow(ctx, `SELECT id::text FROM public.espacio WHERE id=$1 AND propietario_id=$2 AND estado='activa' FOR UPDATE`, q.SpaceID, q.HostID).Scan(&activeSpace)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if !q.ExpiresAt.After(now) {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrQuoteExpired
	}
	var occupied bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.ocupacion WHERE espacio_id=$1 AND activo AND intervalo && tstzrange($2,$3,'[)'))`, q.SpaceID, q.StartAt, q.EndAt).Scan(&occupied)
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if occupied {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	var rateCurrent bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.espacio e JOIN public.tarifa_espacio t ON t.espacio_id=e.id WHERE e.id=$1 AND e.propietario_id=$2 AND e.estado='activa' AND t.version=$3 AND t.modalidad=$4 AND t.precio_base_clp=$5 AND t.moneda=$6)`, q.SpaceID, q.HostID, q.RateVersion, q.RateUnit, q.UnitPriceCLP, q.Currency).Scan(&rateCurrent)
	if err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	if !rateCurrent {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	var zone *time.Location
	zone, err = time.LoadLocation(q.TimeZone)
	if err != nil {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	dateText := q.StartAt.In(zone).Format("2006-01-02")
	listing := rentalflow.Listing{ID: q.SpaceID, OwnerID: q.HostID, State: "activa", Title: q.Title, RateUnit: q.RateUnit, UnitPriceCLP: q.UnitPriceCLP, Currency: q.Currency, RateVersion: q.RateVersion, TimeZone: q.TimeZone}
	availableIntervals, err := (&Repository{db: tx, now: r.now, newID: r.newID}).PublicIntervals(ctx, listing, dateText, int(q.Units))
	if err != nil {
		return q, checkoutpro.Operation{}, false, err
	}
	available := false
	for _, candidate := range availableIntervals {
		if candidate.StartAt.Equal(q.StartAt) && candidate.EndAt.Equal(q.EndAt) {
			available = true
			break
		}
	}
	if !available {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	var conflict bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c JOIN public.cotizacion_arriendo_comercial x ON x.id=c.cotizacion_id WHERE c.espacio_id=$1 AND c.estado IN ('pending','reconciliation_required') AND (c.estado='reconciliation_required' OR c.vence_en>$2) AND x.inicio < $4 AND $3 < x.termino)`, q.SpaceID, now, q.StartAt, q.EndAt).Scan(&conflict)
	if err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if conflict {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	amounts, amountErr := checkoutpro.CalculateAmounts(q.SubtotalCLP)
	if amountErr != nil {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	// Payments API local test operations currently validate the direct seller
	// transfer without charging an EspaciGo marketplace fee. Persist the exact
	// expected charge breakdown for webhook reconciliation. Existing operations
	// and Checkout Pro continue to retain their original fee calculation.
	if checkoutMethod == "payments_api" {
		amounts = checkoutpro.Amounts{BaseCLP: q.SubtotalCLP}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.checkout_preferencia_comercial(id,cotizacion_id,espacio_id,arrendatario_id,host_id,idempotency_key,fingerprint,estado,base_clp,commission_clp,iva_clp,marketplace_fee_clp,test_mode,vence_en,creada_en,actualizada_en,checkout_method) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$11,$12,$13,$14,$14,$15)`, operationID, q.ID, q.SpaceID, renterID, q.HostID, key, fingerprint, amounts.BaseCLP, amounts.CommissionCLP, amounts.VATCLP, amounts.MarketplaceFeeCLP, testMode, q.ExpiresAt, now, checkoutMethod); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	occupancyID := r.newID()
	if _, err = tx.Exec(ctx, `INSERT INTO public.reserva_arriendo_comercial(id,operacion_checkout_id,cotizacion_id,espacio_id,anfitrion_id,arrendatario_id,estado,tarifa_version,modalidad,precio_unitario_clp,unidades,subtotal_clp,moneda,inicio,termino,zona_horaria,ocupacion_id,pago_vence_en,creada_en,actualizada_en) VALUES($1,$1,$2,$3,$4,$5,'pendiente_de_pago',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, operationID, q.ID, q.SpaceID, q.HostID, renterID, q.RateVersion, q.RateUnit, q.UnitPriceCLP, q.Units, q.SubtotalCLP, q.Currency, q.StartAt, q.EndAt, q.TimeZone, occupancyID, q.ExpiresAt, now); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,reserva_arriendo_comercial_id,intervalo,tipo,activo,expira_en) VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'retencion_comercial',true,$6)`, occupancyID, q.SpaceID, operationID, q.StartAt, q.EndAt, q.ExpiresAt); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	op = checkoutpro.Operation{ID: operationID, QuoteID: q.ID, Status: "pending", CheckoutMethod: checkoutMethod, ExpiresAt: q.ExpiresAt, UpdatedAt: now, Amounts: amounts, TestMode: testMode}
	if err = tx.Commit(ctx); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	return q, op, true, nil
}

func (r *Repository) SavePreference(ctx context.Context, id, preferenceID, initPoint string, now time.Time) error {
	tag, err := r.db.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET preference_id=$2, init_point=$3, actualizada_en=$4 WHERE id=$1 AND checkout_method='checkout_pro' AND estado='pending' AND vence_en>$4`, id, preferenceID, initPoint, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return checkoutpro.ErrConflict
	}
	return nil
}

func (r *Repository) ClaimPreference(ctx context.Context, id string, now time.Time, lease time.Duration) (bool, error) {
	var claimed string
	err := r.db.QueryRow(ctx, `UPDATE public.checkout_preferencia_comercial SET preference_claimed_at=$2,actualizada_en=$2 WHERE id=$1 AND checkout_method='checkout_pro' AND estado='pending' AND preference_id IS NULL AND (preference_claimed_at IS NULL OR preference_claimed_at <= $3) RETURNING id::text`, id, now, now.Add(-lease)).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ClaimPayment is a one-shot durable claim. A claimed request is never
// automatically replayed with a potentially different single-use card token.
func (r *Repository) ClaimPayment(ctx context.Context, id string, now time.Time) (bool, error) {
	var claimed string
	err := r.db.QueryRow(ctx, `UPDATE public.checkout_preferencia_comercial SET payment_create_claimed_at=$2,actualizada_en=$2 WHERE id=$1 AND checkout_method='payments_api' AND test_mode AND estado='pending' AND preference_id IS NULL AND provider_payment_id IS NULL AND payment_create_claimed_at IS NULL AND vence_en>$2 RETURNING id::text`, id, now).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) ByQuote(ctx context.Context, renterID, quoteID string) (checkoutpro.Quote, *checkoutpro.Operation, error) {
	var q checkoutpro.Quote
	err := r.db.QueryRow(ctx, `SELECT id::text,espacio_id::text,propietario_id::text,arrendatario_id::text,titulo_espacio,tarifa_version,modalidad,moneda,precio_unitario_clp,unidades,subtotal_clp,inicio,termino,zona_horaria,vence_en FROM public.cotizacion_arriendo_comercial WHERE id=$1 AND arrendatario_id=$2`, quoteID, renterID).Scan(&q.ID, &q.SpaceID, &q.HostID, &q.RenterID, &q.Title, &q.RateVersion, &q.RateUnit, &q.Currency, &q.UnitPriceCLP, &q.Units, &q.SubtotalCLP, &q.StartAt, &q.EndAt, &q.TimeZone, &q.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return checkoutpro.Quote{}, nil, checkoutpro.ErrNotFound
	}
	if err != nil {
		return checkoutpro.Quote{}, nil, err
	}
	var op checkoutpro.Operation
	err = r.db.QueryRow(ctx, `SELECT id::text,cotizacion_id::text,estado,COALESCE(preference_id,''),COALESCE(init_point,''),vence_en,actualizada_en,checkout_method,COALESCE(provider_payment_id,''),test_mode FROM public.checkout_preferencia_comercial WHERE cotizacion_id=$1 AND arrendatario_id=$2 ORDER BY creada_en DESC LIMIT 1`, quoteID, renterID).Scan(&op.ID, &op.QuoteID, &op.Status, &op.PreferenceID, &op.InitPoint, &op.ExpiresAt, &op.UpdatedAt, &op.CheckoutMethod, &op.ProviderPaymentID, &op.TestMode)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, nil, nil
	}
	if err != nil {
		return checkoutpro.Quote{}, nil, err
	}
	return q, &op, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
