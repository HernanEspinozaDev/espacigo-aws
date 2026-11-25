package didit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func sign(secret string, message []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookV2CanonicalAndRotation(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"status":"Approved","vendor_data":"attempt-uuid","decision":{"id_verifications":[{"first_name":"José","status":"Approved"}]},"webhook_type":"status.updated","session_id":"session-1","event_id":"event-1","application_id":"app-sandbox","environment":"sandbox","workflow_id":"wf"}`)
	canonical, err := canonicalJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("X-Timestamp", timestamp)
	headers.Set("X-Signature-V2-Previous", sign("rotated-secret", canonical))
	got, err := VerifyWebhook(headers, body, WebhookSecrets{Current: "current-secret", Previous: "rotated-secret"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "event-1" || got.ApplicationID != "app-sandbox" || got.Environment != "sandbox" || got.WorkflowID != "wf" || got.VendorData != "attempt-uuid" || got.Decision.IDVerifications[0].FirstName != "José" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
}

func TestVerifyWebhookRawAndSimpleSignatures(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"session_id":"session-1","status":"Approved","webhook_type":"status.updated","event_id":"evt"}`)

	headers := make(http.Header)
	headers.Set("X-Timestamp", ts)
	headers.Set("X-Signature", sign("secret", body))
	if _, err := VerifyWebhook(headers, body, WebhookSecrets{Current: "secret"}, now); err != nil {
		t.Fatalf("raw signature: %v", err)
	}

	headers = make(http.Header)
	headers.Set("X-Timestamp", ts)
	headers.Set("X-Signature-Simple", sign("secret", []byte(ts+":session-1:Approved:status.updated")))
	got, err := VerifyWebhook(headers, body, WebhookSecrets{Current: "secret"}, now)
	if err != nil || got.SessionID != "session-1" || !got.DecisionMustBeFetched {
		t.Fatalf("simple signature: envelope=%+v err=%v", got, err)
	}
}

func TestVerifyWebhookRejectsBadOrStaleDelivery(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"session_id":"session-1","status":"Approved","webhook_type":"status.updated"}`)
	for _, tc := range []struct{ name, timestamp, signature string }{
		{name: "stale", timestamp: strconv.FormatInt(now.Add(-301*time.Second).Unix(), 10), signature: sign("secret", body)},
		{name: "missing signature", timestamp: strconv.FormatInt(now.Unix(), 10)},
		{name: "invalid timestamp", timestamp: "yesterday", signature: sign("secret", body)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("X-Timestamp", tc.timestamp)
			headers.Set("X-Signature", tc.signature)
			if _, err := VerifyWebhook(headers, body, WebhookSecrets{Current: "secret"}, now); err != ErrInvalidWebhook {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestVerifyWebhookRejectsTrailingJSONAndOversize(t *testing.T) {
	now := time.Now().UTC()
	header := make(http.Header)
	header.Set("X-Timestamp", strconv.FormatInt(now.Unix(), 10))
	for _, body := range [][]byte{[]byte(`{"session_id":"s","webhook_type":"status.updated"} {}`), make([]byte, (2<<20)+1)} {
		if _, err := VerifyWebhook(header, body, WebhookSecrets{Current: "secret"}, now); err != ErrInvalidWebhook {
			t.Fatalf("error = %v", err)
		}
	}
}
