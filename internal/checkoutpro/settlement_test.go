package checkoutpro

import "testing"

func TestDecodeVerifiedCheckoutProSplitSettlement(t *testing.T) {
	p, err := decodeVerifiedPaymentJSON([]byte(`{
		"id": 42, "collector_id": 91, "external_reference": "op", "metadata": {"operation_id":"op"},
		"status":"approved","transaction_amount":1036,"transaction_amount_refunded":0,"currency_id":"CLP","live_mode":true,
		"payment_type_id":"debit_card","date_last_updated":"2026-11-25T11:37:30-04:00",
		"money_release_date":"2026-11-25T11:37:30-04:00","money_release_status":"released",
		"transaction_details":{"total_paid_amount":1036,"net_received_amount":925},
		"fee_details":[{"type":"mercadopago_fee","amount":39,"fee_payer":"collector"}],
		"charges_details":[
			{"name":"mercadopago_fee","type":"fee","amounts":{"original":39},"accounts":{"from":"collector","to":"mp"}},
			{"name":"third_payment","type":"fee","amounts":{"original":72},"accounts":{"from":"collector","to":"marketplace_owner"}}
		]
	}`))
	if err != nil || p.Settlement == nil {
		t.Fatalf("decode settlement: %+v, %v", p.Settlement, err)
	}
	s := p.Settlement.WithReconciliation(p.TransactionAmountCLP, 72)
	if s.NetCLP == nil || *s.NetCLP != 925 || s.ProcessingFeeCLP == nil || *s.ProcessingFeeCLP != 39 ||
		s.MarketplaceCLP == nil || *s.MarketplaceCLP != 72 || s.ReleaseStatus != "released" ||
		s.ReleaseAt == nil || s.RefundedCLP == nil || *s.RefundedCLP != 0 || s.Reconciliation != "reconciled" {
		t.Fatalf("unexpected settlement: %+v", s)
	}
	if got := s.WithReconciliation(1036, 73); got.Reconciliation != "discrepancy" {
		t.Fatalf("expected marketplace mismatch, got %+v", got)
	}
	s.ProcessingFeeCLP = nil
	if got := s.WithReconciliation(1036, 72); got.Reconciliation != "pending" {
		t.Fatalf("a missing fee must remain pending, got %+v", got)
	}
	zero := optionalCLP([]byte("0"))
	if zero == nil || *zero != 0 || optionalCLP([]byte("39.5")) != nil {
		t.Fatal("CLP amounts must preserve explicit zero and reject fractional values")
	}
	s.ProcessingFeeCLP = zero
	withoutProcessing := int64(964)
	s.NetCLP = &withoutProcessing
	if got := s.WithReconciliation(1036, 72); got.Reconciliation != "reconciled" {
		t.Fatalf("expected explicit zero fee to reconcile: %+v", got)
	}

}
