package checkoutpro

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type webhookInboxStub struct {
	received []PaymentNotification
	rows     []WebhookReceipt
	state    string
	error    error
}

func (s *webhookInboxStub) Receive(_ context.Context, n PaymentNotification) (WebhookReceipt, error) {
	if s.error != nil {
		return WebhookReceipt{}, s.error
	}
	s.received = append(s.received, n)
	return WebhookReceipt{ID: "receipt-1", Topic: n.Topic, PaymentID: n.PaymentID, State: "received"}, nil
}
func (s *webhookInboxStub) Pending(context.Context, time.Time, time.Duration, int) ([]WebhookReceipt, error) {
	return s.rows, nil
}
func (s *webhookInboxStub) Claim(context.Context, string, time.Time, time.Duration) (bool, error) {
	return true, nil
}
func (s *webhookInboxStub) MarkProcessed(context.Context, string, string, time.Time) error {
	s.state = "processed"
	return nil
}
func (s *webhookInboxStub) MarkRetry(context.Context, string, string, time.Time) error {
	s.state = "retry"
	return nil
}
func (s *webhookInboxStub) MarkReconciliationRequired(context.Context, string, string, time.Time) error {
	s.state = "reconciliation_required"
	return nil
}

type webhookCredentialsStub struct {
	seller string
	token  []byte
	err    error
}

func (s webhookCredentialsStub) AccessTokenForSeller(_ context.Context, seller string) ([]byte, error) {
	if seller != s.seller {
		return nil, ErrNotFound
	}
	return append([]byte(nil), s.token...), s.err
}

type webhookProviderStub struct {
	payment  VerifiedPayment
	err      error
	gotID    string
	gotToken []byte
}

func (s *webhookProviderStub) LookupPayment(_ context.Context, token []byte, paymentID string) (VerifiedPayment, error) {
	s.gotID = paymentID
	s.gotToken = append([]byte(nil), token...)
	return s.payment, s.err
}

type webhookOutcomeStub struct {
	called  bool
	payment VerifiedPayment
	err     error
}

func (s *webhookOutcomeStub) ApplyVerifiedPayment(_ context.Context, p VerifiedPayment) (bool, error) {
	s.called = true
	s.payment = p
	return true, s.err
}

func signTestNotification(secret []byte, id, requestID, ts string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(fmt.Sprintf("id:%s;request-id:%s;ts:%s;", id, requestID, ts)))
	return "ts=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookReceiverVerifiesAndPersistsBeforeAcknowledgement(t *testing.T) {
	secret := []byte("synthetic webhook signing secret for tests only")
	box := &webhookInboxStub{}
	now := time.Date(2030, 1, 15, 12, 0, 0, 0, time.UTC)
	wakeCount := 0
	h, err := NewWebhookReceiver(secret, box, func() time.Time { return now }, func() { wakeCount++ })
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/mercadopago/webhook", strings.NewReader(`{"type":"payment","action":"payment.updated","user_id":123456789,"data":{"id":987654321}}`))
	req.Header.Set("x-request-id", "request-abc")
	req.Header.Set("x-signature", signTestNotification(secret, "987654321", "request-abc", "1894737600"))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || len(box.received) != 1 || wakeCount != 1 {
		t.Fatalf("unexpected response/receipt/wake: %d %d %d", res.Code, len(box.received), wakeCount)
	}
	received := box.received[0]
	if received.PaymentID != "987654321" || received.SellerIDHint != "123456789" || received.RequestID != "request-abc" || !received.ReceivedAt.Equal(now) {
		t.Fatalf("unexpected durable receipt input: %+v", received)
	}
}

func TestWebhookReceiverRejectsInvalidSignatureBeforeWriting(t *testing.T) {
	secret := []byte("synthetic webhook signing secret for tests only")
	box := &webhookInboxStub{}
	h, err := NewWebhookReceiver(secret, box, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"type":"payment","user_id":123456789,"data":{"id":"987654321"}}`))
	req.Header.Set("x-request-id", "request-abc")
	req.Header.Set("x-signature", "ts=1894737600,v1="+strings.Repeat("0", sha256.Size*2))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || len(box.received) != 0 {
		t.Fatalf("invalid signature reached persistence: status=%d received=%d", res.Code, len(box.received))
	}
}

func TestWebhookReceiverAcknowledgesUnsupportedSignedTopic(t *testing.T) {
	secret := []byte("synthetic webhook signing secret for tests only")
	box := &webhookInboxStub{}
	h, err := NewWebhookReceiver(secret, box, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"type":"merchant_order","data":{"id":"order-abc"}}`))
	req.Header.Set("x-request-id", "request-abc")
	req.Header.Set("x-signature", signTestNotification(secret, "order-abc", "request-abc", "1894737600"))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || len(box.received) != 0 {
		t.Fatalf("unsupported signed topic was not safely ignored: status=%d received=%d", res.Code, len(box.received))
	}
}

func TestPaymentWorkerChecksSellerFromAuthenticatedPayment(t *testing.T) {
	box := &webhookInboxStub{rows: []WebhookReceipt{{ID: "receipt-1", Topic: "payment", PaymentID: "987654321", SellerIDHint: "123456789", Attempts: 0}}}
	provider := &webhookProviderStub{payment: VerifiedPayment{ProviderPaymentID: "987654321", SellerID: "different-seller"}}
	outcomes := &webhookOutcomeStub{}
	worker, err := NewPaymentWorker(box, webhookCredentialsStub{seller: "123456789", token: []byte("synthetic token")}, provider, outcomes, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	worker.processBatch(context.Background())
	if provider.gotID != "987654321" || outcomes.called || box.state != "reconciliation_required" {
		t.Fatalf("mismatched seller reached sink: id=%q sink=%t state=%q", provider.gotID, outcomes.called, box.state)
	}
}

func TestPaymentWorkerAppliesOnlyProviderLookupResult(t *testing.T) {
	payment := VerifiedPayment{OperationID: "operation", ProviderPaymentID: "987654321", SellerID: "123456789", Status: "approved"}
	box := &webhookInboxStub{rows: []WebhookReceipt{{ID: "receipt-1", Topic: "payment", PaymentID: "987654321", SellerIDHint: "123456789"}}}
	provider := &webhookProviderStub{payment: payment}
	outcomes := &webhookOutcomeStub{}
	worker, err := NewPaymentWorker(box, webhookCredentialsStub{seller: "123456789", token: []byte("synthetic token")}, provider, outcomes, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	worker.processBatch(context.Background())
	if !outcomes.called || outcomes.payment.ProviderPaymentID != payment.ProviderPaymentID || box.state != "processed" {
		t.Fatalf("provider lookup result was not applied: sink=%t state=%q payment=%+v", outcomes.called, box.state, outcomes.payment)
	}
}
