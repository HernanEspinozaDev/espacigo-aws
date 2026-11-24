package rentalflowpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/calendar"
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
	if r.contractsRequired {
		return r.beginContractCheckout(ctx, renterID, quoteID, key, operationID, fingerprint, checkoutMethod, testMode, now)
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
	var existingKey string
	var existingClaimedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text, cotizacion_id::text, idempotency_key, estado, COALESCE(preference_id,''), COALESCE(init_point,''), vence_en, actualizada_en, fingerprint,base_clp,commission_clp,iva_clp,buyer_commission_clp,buyer_iva_clp,buyer_fee_clp,total_charged_clp,marketplace_fee_clp,test_mode,checkout_method,COALESCE(provider_payment_id,''),payment_create_claimed_at FROM public.checkout_preferencia_comercial WHERE arrendatario_id=$1 AND cotizacion_id=$2 ORDER BY creada_en DESC LIMIT 1 FOR UPDATE`, renterID, quoteID).Scan(&op.ID, &op.QuoteID, &existingKey, &op.Status, &op.PreferenceID, &op.InitPoint, &op.ExpiresAt, &op.UpdatedAt, &oldFingerprint, &op.Amounts.BaseCLP, &op.Amounts.CommissionCLP, &op.Amounts.VATCLP, &op.Amounts.BuyerCommissionCLP, &op.Amounts.BuyerVATCLP, &op.Amounts.BuyerFeeCLP, &op.Amounts.TotalChargedCLP, &op.Amounts.MarketplaceFeeCLP, &op.TestMode, &op.CheckoutMethod, &op.ProviderPaymentID, &existingClaimedAt)
	hasExistingOp := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	if hasExistingOp && existingKey == key {
		if !equalBytes(oldFingerprint, fingerprint) || op.CheckoutMethod != checkoutMethod || op.TestMode != testMode {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrIdempotencyConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		return q, op, false, nil
	}
	canRecycleExisting := false
	if hasExistingOp {
		switch {
		case op.ProviderPaymentID != "":
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		case op.Status == "approved" || op.Status == "reconciliation_required":
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		case op.Status == "rejected" || op.Status == "cancelled":
			canRecycleExisting = true
		case op.Status == "pending" && op.CheckoutMethod == "checkout_pro" && checkoutMethod == "payments_api":
			canRecycleExisting = true
		case op.Status == "pending" && op.CheckoutMethod == "payments_api" && checkoutMethod == "checkout_pro" && existingClaimedAt == nil:
			canRecycleExisting = true
		default:
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		}
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
	var quoteIntervals []rentalflow.QuoteInterval
	ivRows, ivErr := tx.Query(ctx, `SELECT inicio, termino, unidades FROM public.cotizacion_arriendo_tramo WHERE cotizacion_id=$1 ORDER BY orden`, q.ID)
	if ivErr != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, ivErr
	}
	for ivRows.Next() {
		var iv rentalflow.QuoteInterval
		if scanErr := ivRows.Scan(&iv.StartAt, &iv.EndAt, &iv.Units); scanErr != nil {
			ivRows.Close()
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, scanErr
		}
		quoteIntervals = append(quoteIntervals, iv)
	}
	if ivRows.Err() != nil {
		ivRows.Close()
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, ivRows.Err()
	}
	ivRows.Close()
	if len(quoteIntervals) == 0 {
		quoteIntervals = []rentalflow.QuoteInterval{{StartAt: q.StartAt, EndAt: q.EndAt, Units: q.Units}}
	}

	for _, iv := range quoteIntervals {
		var occupied bool
		if canRecycleExisting {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.ocupacion WHERE espacio_id=$1 AND activo AND COALESCE(reserva_arriendo_comercial_id::text,'')<>$4::text AND intervalo && tstzrange($2,$3,'[)'))`, q.SpaceID, iv.StartAt, iv.EndAt, op.ID).Scan(&occupied)
		} else {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.ocupacion WHERE espacio_id=$1 AND activo AND intervalo && tstzrange($2,$3,'[)'))`, q.SpaceID, iv.StartAt, iv.EndAt).Scan(&occupied)
		}
		if err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		if occupied {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		}
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
	listing := rentalflow.Listing{ID: q.SpaceID, OwnerID: q.HostID, State: "activa", Title: q.Title, RateUnit: q.RateUnit, UnitPriceCLP: q.UnitPriceCLP, Currency: q.Currency, RateVersion: q.RateVersion, TimeZone: q.TimeZone}
	if !canRecycleExisting {
		for _, iv := range quoteIntervals {
			dateText := iv.StartAt.In(zone).Format("2006-01-02")
			availableIntervals, availErr := (&Repository{db: tx, now: r.now, newID: r.newID}).PublicIntervals(ctx, listing, dateText, int(iv.Units))
			if availErr != nil {
				return q, checkoutpro.Operation{}, false, availErr
			}
			available := false
			for _, candidate := range availableIntervals {
				if candidate.StartAt.Equal(iv.StartAt) && candidate.EndAt.Equal(iv.EndAt) {
					available = true
					break
				}
			}
			if !available {
				return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
			}
		}
	}
	for _, iv := range quoteIntervals {
		var conflict bool
		if canRecycleExisting {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c JOIN public.cotizacion_arriendo_comercial x ON x.id=c.cotizacion_id LEFT JOIN public.cotizacion_arriendo_tramo t ON t.cotizacion_id=x.id WHERE c.espacio_id=$1 AND c.id<>$5::uuid AND c.estado IN ('pending','reconciliation_required') AND (c.estado='reconciliation_required' OR c.vence_en>$2) AND COALESCE(t.inicio,x.inicio) < $4 AND $3 < COALESCE(t.termino,x.termino))`, q.SpaceID, now, iv.StartAt, iv.EndAt, op.ID).Scan(&conflict)
		} else {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.checkout_preferencia_comercial c JOIN public.cotizacion_arriendo_comercial x ON x.id=c.cotizacion_id LEFT JOIN public.cotizacion_arriendo_tramo t ON t.cotizacion_id=x.id WHERE c.espacio_id=$1 AND c.estado IN ('pending','reconciliation_required') AND (c.estado='reconciliation_required' OR c.vence_en>$2) AND COALESCE(t.inicio,x.inicio) < $4 AND $3 < COALESCE(t.termino,x.termino))`, q.SpaceID, now, iv.StartAt, iv.EndAt).Scan(&conflict)
		}
		if err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		if conflict {
			return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
		}
	}
	amounts, amountErr := checkoutpro.CalculateAmounts(q.SubtotalCLP)
	if amountErr != nil {
		return q, checkoutpro.Operation{}, false, checkoutpro.ErrConflict
	}
	if canRecycleExisting {
		existingID := op.ID
		if _, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial SET idempotency_key=$2,fingerprint=$3,estado='pending',preference_id=NULL,init_point=NULL,preference_claimed_at=NULL,payment_create_claimed_at=NULL,provider_payment_id=NULL,provider_payment_status=NULL,provider_payment_updated_at=NULL,payment_verified_at=NULL,reconciliation_reason=NULL,base_clp=$4,commission_clp=$5,iva_clp=$6,buyer_commission_clp=$7,buyer_iva_clp=$8,buyer_fee_clp=$9,total_charged_clp=$10,marketplace_fee_clp=$11,test_mode=$12,vence_en=$13,actualizada_en=$14,checkout_method=$15 WHERE id=$1`, existingID, key, fingerprint, amounts.BaseCLP, amounts.CommissionCLP, amounts.VATCLP, amounts.BuyerCommissionCLP, amounts.BuyerVATCLP, amounts.BuyerFeeCLP, amounts.TotalChargedCLP, amounts.MarketplaceFeeCLP, testMode, q.ExpiresAt, now, checkoutMethod); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE public.reserva_arriendo_comercial SET estado='pendiente_de_pago',pago_vence_en=$2,actualizada_en=$3 WHERE operacion_checkout_id=$1`, existingID, q.ExpiresAt, now); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE public.ocupacion SET activo=true,desactivada_en=NULL,tipo='retencion_comercial',expira_en=$2 WHERE reserva_arriendo_comercial_id=$1`, existingID, q.ExpiresAt); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		if err = r.syncMonthlyInstallments(ctx, tx, existingID, q.ID, q, quoteIntervals, zone, now); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		op = checkoutpro.Operation{ID: existingID, QuoteID: q.ID, Status: "pending", CheckoutMethod: checkoutMethod, ExpiresAt: q.ExpiresAt, UpdatedAt: now, Amounts: amounts, TestMode: testMode}
		if err = tx.Commit(ctx); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
		return q, op, true, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.checkout_preferencia_comercial(id,cotizacion_id,espacio_id,arrendatario_id,host_id,idempotency_key,fingerprint,estado,base_clp,commission_clp,iva_clp,buyer_commission_clp,buyer_iva_clp,buyer_fee_clp,total_charged_clp,marketplace_fee_clp,test_mode,vence_en,creada_en,actualizada_en,checkout_method) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18,$19)`, operationID, q.ID, q.SpaceID, renterID, q.HostID, key, fingerprint, amounts.BaseCLP, amounts.CommissionCLP, amounts.VATCLP, amounts.BuyerCommissionCLP, amounts.BuyerVATCLP, amounts.BuyerFeeCLP, amounts.TotalChargedCLP, amounts.MarketplaceFeeCLP, testMode, q.ExpiresAt, now, checkoutMethod); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	occupancyID := r.newID()
	if _, err = tx.Exec(ctx, `INSERT INTO public.reserva_arriendo_comercial(id,operacion_checkout_id,cotizacion_id,espacio_id,anfitrion_id,arrendatario_id,estado,tarifa_version,modalidad,precio_unitario_clp,unidades,subtotal_clp,moneda,inicio,termino,zona_horaria,ocupacion_id,pago_vence_en,creada_en,actualizada_en) VALUES($1,$1,$2,$3,$4,$5,'pendiente_de_pago',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, operationID, q.ID, q.SpaceID, q.HostID, renterID, q.RateVersion, q.RateUnit, q.UnitPriceCLP, q.Units, q.SubtotalCLP, q.Currency, q.StartAt, q.EndAt, q.TimeZone, occupancyID, q.ExpiresAt, now); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	for idx, iv := range quoteIntervals {
		rowOccupancyID := occupancyID
		if idx > 0 {
			rowOccupancyID = r.newID()
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,reserva_arriendo_comercial_id,intervalo,tipo,activo,expira_en) VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'retencion_comercial',true,$6)`, rowOccupancyID, q.SpaceID, operationID, iv.StartAt, iv.EndAt, q.ExpiresAt); err != nil {
			return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
		}
	}
	if err = r.syncMonthlyInstallments(ctx, tx, operationID, q.ID, q, quoteIntervals, zone, now); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	op = checkoutpro.Operation{ID: operationID, QuoteID: q.ID, Status: "pending", CheckoutMethod: checkoutMethod, ExpiresAt: q.ExpiresAt, UpdatedAt: now, Amounts: amounts, TestMode: testMode}
	if err = tx.Commit(ctx); err != nil {
		return checkoutpro.Quote{}, checkoutpro.Operation{}, false, err
	}
	return q, op, true, nil
}

func (r *Repository) syncMonthlyInstallments(ctx context.Context, tx pgx.Tx, reservaID, quoteID string, q checkoutpro.Quote, quoteIntervals []rentalflow.QuoteInterval, zone *time.Location, now time.Time) error {
	if q.RateUnit != "mes" {
		return nil
	}
	var billingMode string
	err := tx.QueryRow(ctx, `SELECT modalidad_cobro FROM public.cotizacion_modalidad_cobro WHERE cotizacion_id=$1`, quoteID).Scan(&billingMode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if billingMode != "monthly_installments" {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM public.reserva_cuota_mensual WHERE reserva_arriendo_comercial_id=$1`, reservaID); err != nil {
		return err
	}
	cuotaNum := 1
	for _, iv := range quoteIntervals {
		ivUnits := iv.Units
		if ivUnits < 1 {
			ivUnits = 1
		}
		curStart := iv.StartAt.In(zone)
		for m := int64(0); m < ivUnits; m++ {
			annivStart, startOK := calendar.AddMonthsClamped(curStart, m)
			annivEnd, endOK := calendar.AddMonthsClamped(curStart, m+1)
			if !startOK || !endOK {
				return checkoutpro.ErrInvalid
			}
			pStart, pStartOK := calendar.FirstInstantOfDate(annivStart.Year(), annivStart.Month(), annivStart.Day(), zone)
			pEnd, pEndOK := calendar.FirstInstantOfDate(annivEnd.Year(), annivEnd.Month(), annivEnd.Day(), zone)
			if !pStartOK || !pEndOK {
				return checkoutpro.ErrInvalid
			}
			cuotaID := r.newID()
			if _, err := tx.Exec(ctx, `INSERT INTO public.reserva_cuota_mensual(
				id, reserva_arriendo_comercial_id, numero_cuota,
				periodo_inicio, periodo_termino, monto_clp, estado, creado_en
			) VALUES ($1, $2, $3, $4, $5, $6, 'pendiente', $7)`,
				cuotaID, reservaID, cuotaNum, pStart.UTC(), pEnd.UTC(), q.UnitPriceCLP, now); err != nil {
				return err
			}
			cuotaNum++
		}
	}
	return nil
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
	err = r.db.QueryRow(ctx, `SELECT id::text,cotizacion_id::text,estado,COALESCE(preference_id,''),COALESCE(init_point,''),vence_en,actualizada_en,checkout_method,COALESCE(provider_payment_id,''),test_mode,base_clp,commission_clp,iva_clp,buyer_commission_clp,buyer_iva_clp,buyer_fee_clp,total_charged_clp,marketplace_fee_clp FROM public.checkout_preferencia_comercial WHERE cotizacion_id=$1 AND arrendatario_id=$2 ORDER BY creada_en DESC LIMIT 1`, quoteID, renterID).Scan(&op.ID, &op.QuoteID, &op.Status, &op.PreferenceID, &op.InitPoint, &op.ExpiresAt, &op.UpdatedAt, &op.CheckoutMethod, &op.ProviderPaymentID, &op.TestMode, &op.Amounts.BaseCLP, &op.Amounts.CommissionCLP, &op.Amounts.VATCLP, &op.Amounts.BuyerCommissionCLP, &op.Amounts.BuyerVATCLP, &op.Amounts.BuyerFeeCLP, &op.Amounts.TotalChargedCLP, &op.Amounts.MarketplaceFeeCLP)
	if errors.Is(err, pgx.ErrNoRows) {
		if r.contractsRequired {
			q, err = r.contractQuoteForStatus(ctx, q, nil)
			return q, nil, err
		}
		return q, nil, nil
	}
	if err != nil {
		return checkoutpro.Quote{}, nil, err
	}
	if r.contractsRequired {
		q, err = r.contractQuoteForStatus(ctx, q, &op)
		if err != nil {
			return q, nil, err
		}
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
