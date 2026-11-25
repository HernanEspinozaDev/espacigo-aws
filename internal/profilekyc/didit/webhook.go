package didit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidWebhook = errors.New("didit: invalid webhook")

// WebhookSecrets supports an overlap during secret rotation. Previous is
// optional and should be removed after Didit's rotation window ends.
type WebhookSecrets struct {
	Current  string
	Previous string
}

// WebhookEnvelope intentionally keeps only the small set of fields needed to
// correlate a status event. Decision is reduced to identity data fields used
// by the profile KYC domain; media URLs and biometric artifacts are ignored.
type WebhookEnvelope struct {
	EventID               string      `json:"event_id"`
	WebhookType           string      `json:"webhook_type"`
	Timestamp             json.Number `json:"timestamp"`
	ApplicationID         string      `json:"application_id"`
	Environment           string      `json:"environment"`
	SessionID             string      `json:"session_id"`
	Status                string      `json:"status"`
	WorkflowID            string      `json:"workflow_id"`
	WorkflowVersion       int         `json:"workflow_version"`
	VendorData            string      `json:"vendor_data"`
	Decision              Decision    `json:"decision"`
	SignatureScheme       string      `json:"-"`
	DecisionMustBeFetched bool        `json:"-"`
}

// VerifyWebhook checks the replay window and Didit's verification webhook
// signatures. It tries V2 canonical JSON first, then the raw body signature,
// and finally the deprecated envelope-only signature. Callers MUST re-fetch
// the authoritative decision using GetDecision when the simple signature was
// accepted; the webhook decision body is not authenticated by that scheme.
func VerifyWebhook(headers http.Header, rawBody []byte, secrets WebhookSecrets, now time.Time) (WebhookEnvelope, error) {
	if len(rawBody) == 0 || len(rawBody) > 2<<20 || strings.TrimSpace(secrets.Current) == "" {
		return WebhookEnvelope{}, ErrInvalidWebhook
	}
	timestamp := strings.TrimSpace(headers.Get("X-Timestamp"))
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+300 {
		return WebhookEnvelope{}, ErrInvalidWebhook
	}
	var envelope WebhookEnvelope
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return WebhookEnvelope{}, ErrInvalidWebhook
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return WebhookEnvelope{}, ErrInvalidWebhook
	}
	if envelope.SessionID == "" || envelope.WebhookType == "" {
		return WebhookEnvelope{}, ErrInvalidWebhook
	}
	checks := []struct {
		currentHeader  string
		previousHeader string
		message        []byte
	}{
		{currentHeader: "X-Signature-V2", previousHeader: "X-Signature-V2-Previous"},
		{currentHeader: "X-Signature", previousHeader: "X-Signature-Previous", message: rawBody},
	}
	canonical, canonErr := canonicalJSON(rawBody)
	if canonErr == nil {
		checks[0].message = canonical
	}
	for _, check := range checks {
		if len(check.message) == 0 {
			continue
		}
		provided := strings.TrimSpace(headers.Get(check.currentHeader))
		previous := strings.TrimSpace(headers.Get(check.previousHeader))
		if signatureMatches(provided, secrets.Current, check.message) {
			envelope.SignatureScheme = check.currentHeader
			return envelope, nil
		}
		if signatureMatches(previous, secrets.Previous, check.message) {
			envelope.SignatureScheme = check.previousHeader
			return envelope, nil
		}
	}
	// Deprecated signature covers only this compact envelope; decision must
	// be fetched again from Didit before the caller makes a state transition.
	simple := []byte(timestamp + ":" + envelope.SessionID + ":" + envelope.Status + ":" + envelope.WebhookType)
	if signatureMatches(strings.TrimSpace(headers.Get("X-Signature-Simple")), secrets.Current, simple) {
		envelope.SignatureScheme = "X-Signature-Simple"
		envelope.DecisionMustBeFetched = true
		return envelope, nil
	}
	if signatureMatches(strings.TrimSpace(headers.Get("X-Signature-Simple-Previous")), secrets.Previous, simple) {
		envelope.SignatureScheme = "X-Signature-Simple-Previous"
		envelope.DecisionMustBeFetched = true
		return envelope, nil
	}
	return WebhookEnvelope{}, ErrInvalidWebhook
}

func signatureMatches(signature, secret string, message []byte) bool {
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(secret) == "" {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(message)
	return hmac.Equal(mac.Sum(nil), provided)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON", ErrInvalidWebhook)
	}
	var out bytes.Buffer
	if err := writeCanonical(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		writeJSONString(out, v)
	case json.Number:
		// Didit emits JSON numbers in a stable form; retaining the parsed token
		// preserves its numeric value while avoiding float precision loss.
		if _, err := strconv.ParseFloat(v.String(), 64); err != nil {
			return err
		}
		out.WriteString(v.String())
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			writeJSONString(out, key)
			out.WriteByte(':')
			if err := writeCanonical(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("%w: unsupported JSON value", ErrInvalidWebhook)
	}
	return nil
}

func writeJSONString(out *bytes.Buffer, value string) {
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}
