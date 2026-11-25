package rentalflowpg

import (
	"context"
	"encoding/json"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
	"github.com/jackc/pgx/v5"
)

type SettlementCandidate struct {
	OperationID string
	PaymentID   string
	HostID      string
}

func (r *Repository) ClaimSettlementCandidates(ctx context.Context) ([]SettlementCandidate, error) {
	rows, err := r.db.Query(ctx, `
WITH due AS (
    SELECT c.id FROM public.checkout_preferencia_comercial c
    JOIN public.mercadopago_seller_connection s ON s.anfitrion_id=c.host_id
        AND s.connection_state='active' AND s.live_mode<>c.test_mode
    WHERE c.estado='approved' AND c.checkout_method='checkout_pro' AND c.provider_payment_id IS NOT NULL
      AND (c.provider_settlement_attempted_at IS NULL OR c.provider_settlement_attempted_at < now()-interval '30 minutes')
      AND (c.provider_settlement IS NULL OR c.provider_settlement->>'reconciliation' <> 'reconciled'
           OR c.provider_settlement->>'release_status' IS DISTINCT FROM 'released')
    ORDER BY c.provider_settlement_attempted_at NULLS FIRST,c.id
    LIMIT 20 FOR UPDATE OF c SKIP LOCKED
)
UPDATE public.checkout_preferencia_comercial c SET provider_settlement_attempted_at=now()
FROM due WHERE c.id=due.id RETURNING c.id::text,c.provider_payment_id,c.host_id::text`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SettlementCandidate
	for rows.Next() {
		var item SettlementCandidate
		if err := rows.Scan(&item.OperationID, &item.PaymentID, &item.HostID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SyncPaymentSettlement updates only financial facts for an already verified
// Checkout Pro payment. Historical reads do not replay reservation transitions.
func (r *Repository) SyncPaymentSettlement(ctx context.Context, candidate SettlementCandidate, p checkoutpro.VerifiedPayment) error {
	if p.Settlement == nil || p.OperationID != candidate.OperationID || p.ExternalReference != candidate.OperationID ||
		p.ProviderPaymentID != candidate.PaymentID || p.Currency != "CLP" || (p.Status != "approved" && p.Status != "refunded" && p.Status != "charged_back") {
		return checkoutpro.ErrInvalid
	}
	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var marketplace int64
	err = tx.QueryRow(ctx, `SELECT c.marketplace_fee_clp FROM public.checkout_preferencia_comercial c
JOIN public.mercadopago_seller_connection s ON s.anfitrion_id=c.host_id
WHERE c.id=$1 AND c.provider_payment_id=$2 AND c.host_id=$3
  AND c.estado='approved' AND c.checkout_method='checkout_pro' AND c.total_charged_clp=$4
  AND c.test_mode<>$5 AND s.live_mode=$5 AND s.seller_id=$6 AND s.connection_state='active'
FOR UPDATE OF c`, p.OperationID, p.ProviderPaymentID, candidate.HostID, p.TransactionAmountCLP, p.LiveMode, p.SellerID).Scan(&marketplace)
	if err != nil {
		return err
	}
	if err := r.storePaymentSettlement(ctx, tx, p, marketplace); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) storePaymentSettlement(ctx context.Context, tx pgx.Tx, p checkoutpro.VerifiedPayment, marketplace int64) error {
	if p.Settlement == nil {
		return nil
	}
	snapshot := *p.Settlement
	snapshot.PaymentStatus = p.Status
	syncedAt := r.now().UTC()
	snapshot.SyncedAt = &syncedAt
	snapshot = snapshot.WithReconciliation(p.TransactionAmountCLP, marketplace)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE public.checkout_preferencia_comercial
SET provider_settlement = COALESCE(provider_settlement,'{}'::jsonb) ||
    CASE WHEN $2::jsonb->>'reconciliation'='pending' AND provider_settlement->>'reconciliation' IN ('reconciled','discrepancy')
         THEN $2::jsonb - 'reconciliation' - 'net_clp' - 'processing_fee_clp' - 'marketplace_fee_clp'
         ELSE $2::jsonb END,
    provider_settlement_attempted_at=now()
WHERE id=$1 AND (provider_settlement IS NULL OR
    (provider_settlement->>'provider_updated_at')::timestamptz <= $3)`, p.OperationID, encoded, p.PaymentUpdatedAt.UTC())
	return err
}
