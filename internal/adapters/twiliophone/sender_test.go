package twiliophone

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSendPhoneCode(t *testing.T) {
	sidSuffix := strings.Repeat("1", 32)
	sender, err := New(Config{
		AccountSID: "AC" + sidSuffix, APIKeySID: "SK" + sidSuffix, APIKeySecret: "test-key",
		MessagingServiceSID: "MG" + sidSuffix,
		WhatsAppFrom:        "+15555550100", WhatsAppContentSID: "HX" + sidSuffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		channel identity.PhoneChannel
		status  int
		body    string
	}{
		{identity.PhoneChannelSMS, http.StatusCreated, `{"sid":"SM1","status":"queued"}`},
		{identity.PhoneChannelWhatsApp, http.StatusCreated, `{"sid":"SM2","status":"accepted"}`},
		{identity.PhoneChannelSMS, http.StatusTooManyRequests, `{"error":"rate_limited"}`},
	} {
		sender.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost || req.URL.Host != "api.twilio.com" {
				t.Fatalf("unexpected Twilio request: %s %s", req.Method, req.URL.Host)
			}
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if tc.channel == identity.PhoneChannelSMS {
				if req.Form.Get("To") != "+56912345678" || !strings.Contains(req.Form.Get("Body"), "123456") || req.Form.Get("MessagingServiceSid") != sender.cfg.MessagingServiceSID {
					t.Fatal("incorrect SMS form")
				}
			} else if req.Form.Get("To") != "whatsapp:+56912345678" || req.Form.Get("ContentSid") != sender.cfg.WhatsAppContentSID ||
				req.Form.Get("ContentVariables") != `{"1":"123456"}` || req.Form.Has("Body") {
				t.Fatal("incorrect WhatsApp template form")
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		if err := sender.SendPhoneCode(context.Background(), "+56912345678", "123456", tc.channel); (err != nil) != (tc.status != http.StatusCreated) {
			t.Fatalf("channel %s with HTTP %d: %v", tc.channel, tc.status, err)
		}
	}
}
