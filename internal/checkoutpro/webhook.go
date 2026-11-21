package checkoutpro

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxWebhookBody = 64 << 10

var ErrReconciliationRequired = errors.New("checkoutpro: reconciliation required")

type PaymentNotification struct {
	Topic              string
	Action             string
	PaymentID          string
	SellerIDHint       string
	RequestID          string
	SignatureTimestamp string
	ReceivedAt         time.Time
}

type WebhookReceipt struct {
	ID                 string
	Topic              string
	Action             string
	PaymentID          string
	SellerIDHint       string
	RequestID          string
	SignatureTimestamp string
	State              string
	Attempts           int
	NextAttemptAt      time.Time
}

// WebhookInbox stores a verified notification before the receiver returns 2xx.
// Implementations must deduplicate (topic, request ID) and reject a conflicting
// replay that reuses that key with a different payment ID.
type WebhookInbox interface {
	Receive(context.Context, PaymentNotification) (WebhookReceipt, error)
	Pending(context.Context, time.Time, time.Duration, int) ([]WebhookReceipt, error)
	Claim(context.Context, string, time.Time, time.Duration) (bool, error)
	MarkProcessed(context.Context, string, string, time.Time) error
	MarkRetry(context.Context, string, string, time.Time) error
	MarkReconciliationRequired(context.Context, string, string, time.Time) error
}

// SellerCredentialResolver uses the notification's seller ID only to select a
// locally connected account. The payment lookup must independently confirm the
// returned seller ID before any checkout state can change.
type SellerCredentialResolver interface {
	AccessTokenForSeller(context.Context, string) ([]byte, error)
}

type WebhookReceiver struct {
	secret []byte
	inbox  WebhookInbox
	now    func() time.Time
	wake   func()
}

func NewWebhookReceiver(secret []byte, inbox WebhookInbox, now func() time.Time, wake func()) (*WebhookReceiver, error) {
	if len(secret) < 32 || inbox == nil || now == nil {
		return nil, ErrNotConfigured
	}
	return &WebhookReceiver{secret: append([]byte(nil), secret...), inbox: inbox, now: now, wake: wake}, nil
}

func (h *WebhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h == nil || len(h.secret) < 32 || h.inbox == nil || h.now == nil {
		http.Error(w, "webhook unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBody)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	var wire struct {
		Type   string          `json:"type"`
		Action string          `json:"action"`
		UserID json.RawMessage `json:"user_id"`
		Data   struct {
			ID json.RawMessage `json:"id"`
		} `json:"data"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	queryID, queryType, ok := signedNotificationQuery(r.URL.RawQuery)
	if !ok || !safeNotificationID(queryID) || (queryType != "" && queryType != wire.Type) {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	if len(wire.Data.ID) > 0 && !bytes.Equal(wire.Data.ID, []byte("null")) {
		bodyID, bodyIDOK := notificationValue(wire.Data.ID)
		if !bodyIDOK || !safeNotificationID(bodyID) || bodyID != queryID {
			http.Error(w, "invalid notification", http.StatusBadRequest)
			return
		}
	}
	requestID := strings.TrimSpace(r.Header.Get("x-request-id"))
	ts, signature, ok := parseSignature(r.Header.Get("x-signature"))
	if !ok || requestID == "" || len(requestID) > 256 || !validSignatureTimestamp(ts) || !verifyWebhookSignature(h.secret, queryID, requestID, ts, signature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if wire.Type != "payment" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !decimalID(queryID) {
		http.Error(w, "invalid payment notification", http.StatusBadRequest)
		return
	}
	log.Printf("mercadopago_webhook stage=notification_verified type=payment payment_id=%s signature_valid=true", queryID)
	sellerID, _ := notificationValue(wire.UserID)
	if !decimalID(sellerID) {
		// user_id is not part of MP's HMAC canonical string. Keep it only as an
		// untrusted lookup hint; the authenticated GET result must match it.
		sellerID = ""
	}
	_, err = h.inbox.Receive(r.Context(), PaymentNotification{
		Topic: wire.Type, Action: strings.TrimSpace(wire.Action), PaymentID: queryID,
		SellerIDHint: sellerID, RequestID: requestID, SignatureTimestamp: ts,
		ReceivedAt: h.now().UTC(),
	})
	if err != nil {
		log.Printf("mercadopago_webhook stage=notification_receipt payment_id=%s result=storage_failed", queryID)
		http.Error(w, "notification receipt unavailable", http.StatusServiceUnavailable)
		return
	}
	log.Printf("mercadopago_webhook stage=notification_received payment_id=%s result=stored", queryID)
	if h.wake != nil {
		h.wake()
	}
	w.WriteHeader(http.StatusOK)
}

// signedNotificationQuery extracts the canonical data.id from Mercado Pago's
// query string. Duplicate keys and malformed escapes are rejected so proxies
// and parsers cannot disagree about which ID was signed.
func signedNotificationQuery(raw string) (dataID, topic string, ok bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", "", false
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "data.id" && key != "type") {
			return "", "", false
		}
	}
	ids := values["data.id"]
	if len(ids) != 1 || ids[0] == "" {
		return "", "", false
	}
	types := values["type"]
	if len(types) > 1 || (len(types) == 1 && types[0] == "") {
		return "", "", false
	}
	if len(types) == 1 {
		topic = types[0]
	}
	return ids[0], topic, true
}

func notificationValue(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	var value string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
	} else {
		value = string(raw)
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func safeNotificationID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func decimalID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func validSignatureTimestamp(value string) bool {
	if value == "" || len(value) > 20 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func parseSignature(value string) (string, string, bool) {
	var ts, v1 string
	seenTS, seenV1 := false, false
	for _, rawPart := range strings.Split(value, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(rawPart), "=")
		if !ok {
			return "", "", false
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "ts":
			if seenTS {
				return "", "", false
			}
			ts, seenTS = val, true
		case "v1":
			if seenV1 {
				return "", "", false
			}
			v1, seenV1 = val, true
		}
	}
	decoded, err := hex.DecodeString(v1)
	return ts, v1, seenTS && seenV1 && err == nil && len(decoded) == sha256.Size
}

func verifyWebhookSignature(secret []byte, dataID, requestID, timestamp, signature string) bool {
	got, err := hex.DecodeString(signature)
	if err != nil || len(got) != sha256.Size {
		return false
	}
	canonical := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, timestamp)
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte(canonical))
	want := m.Sum(nil)
	return subtle.ConstantTimeCompare(got, want) == 1
}

type PaymentWorker struct {
	inbox       WebhookInbox
	credentials SellerCredentialResolver
	provider    ProviderLookup
	outcomes    PaymentOutcomeSink
	now         func() time.Time
	interval    time.Duration
	lease       time.Duration
	wake        chan struct{}
}

func NewPaymentWorker(inbox WebhookInbox, credentials SellerCredentialResolver, provider ProviderLookup, outcomes PaymentOutcomeSink, now func() time.Time) (*PaymentWorker, error) {
	if inbox == nil || credentials == nil || provider == nil || outcomes == nil || now == nil {
		return nil, ErrNotConfigured
	}
	return &PaymentWorker{inbox: inbox, credentials: credentials, provider: provider, outcomes: outcomes, now: now, interval: 2 * time.Second, lease: time.Minute, wake: make(chan struct{}, 1)}, nil
}

func (w *PaymentWorker) Notify() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *PaymentWorker) Run(ctx context.Context) error {
	if w == nil || ctx == nil {
		return ErrInvalid
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		w.processBatch(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

func (w *PaymentWorker) processBatch(ctx context.Context) {
	rows, err := w.inbox.Pending(ctx, w.now().UTC(), w.lease, 32)
	if err != nil {
		return
	}
	for _, receipt := range rows {
		if ctx.Err() != nil {
			return
		}
		_ = w.process(ctx, receipt)
	}
}

func (w *PaymentWorker) process(ctx context.Context, receipt WebhookReceipt) error {
	now := w.now().UTC()
	claimed, err := w.inbox.Claim(ctx, receipt.ID, now, w.lease)
	if err != nil || !claimed {
		return err
	}
	if !decimalID(receipt.SellerIDHint) {
		return w.inbox.MarkReconciliationRequired(ctx, receipt.ID, "seller_hint_missing", now)
	}
	token, err := w.credentials.AccessTokenForSeller(ctx, receipt.SellerIDHint)
	if err != nil || len(token) == 0 {
		return w.retry(ctx, receipt, "seller_token_unavailable", now)
	}
	defer clear(token)
	payment, err := w.provider.LookupPayment(ctx, token, receipt.PaymentID)
	if err != nil {
		return w.retry(ctx, receipt, "provider_lookup_failed", now)
	}
	if payment.ProviderPaymentID != receipt.PaymentID || payment.SellerID != receipt.SellerIDHint {
		return w.inbox.MarkReconciliationRequired(ctx, receipt.ID, "provider_identity_mismatch", now)
	}
	if _, err = w.outcomes.ApplyVerifiedPayment(ctx, payment); err != nil {
		if errors.Is(err, ErrReconciliationRequired) {
			return w.inbox.MarkReconciliationRequired(ctx, receipt.ID, "payment_requires_reconciliation", now)
		}
		return w.retry(ctx, receipt, "payment_outcome_unavailable", now)
	}
	return w.inbox.MarkProcessed(ctx, receipt.ID, payment.Status, now)
}

func (w *PaymentWorker) retry(ctx context.Context, receipt WebhookReceipt, code string, now time.Time) error {
	return w.inbox.MarkRetry(ctx, receipt.ID, code, now.Add(paymentRetryDelay(receipt.Attempts+1)))
}

func paymentRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := 5 * time.Second * time.Duration(1<<uint(attempt-1))
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

var _ http.Handler = (*WebhookReceiver)(nil)
