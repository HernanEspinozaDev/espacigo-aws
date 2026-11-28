package safelog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
)

func TestNormalizeCRLF(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain single line",
			input:    "normal log line",
			expected: "normal log line",
		},
		{
			name:     "crlf injected",
			input:    "line 1\r\nline 2",
			expected: "line 1\\nline 2",
		},
		{
			name:     "cr injected",
			input:    "line 1\rline 2",
			expected: "line 1\\nline 2",
		},
		{
			name:     "lf injected",
			input:    "line 1\nline 2\nline 3",
			expected: "line 1\\nline 2\\nline 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safelog.NormalizeCRLF(tt.input)
			if got != tt.expected {
				t.Fatalf("NormalizeCRLF(%q) = %q, want %q", tt.input, got, tt.expected)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Fatalf("NormalizeCRLF result still contains raw CR or LF: %q", got)
			}
		})
	}
}

func TestSanitizePIIAndSecrets(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		forbidden   []string
		mustContain []string
	}{
		{
			name:        "email redaction",
			input:       "User user.testing+filter@example.com requested password reset",
			forbidden:   []string{"user.testing+filter@example.com"},
			mustContain: []string{"[REDACTED_EMAIL]"},
		},
		{
			name:        "rut redaction with dots and dash",
			input:       "Identity rut=12.345.678-5 verified",
			forbidden:   []string{"12.345.678-5"},
			mustContain: []string{"[REDACTED_RUT]"},
		},
		{
			name:        "rut redaction without dots",
			input:       "RUT 12345678-k registered",
			forbidden:   []string{"12345678-k"},
			mustContain: []string{"[REDACTED_RUT]"},
		},
		{
			name:        "bearer token",
			input:       "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.t-IDomw4K7819",
			forbidden:   []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.t-IDomw4K7819"},
			mustContain: []string{"Bearer [REDACTED]"},
		},
		{
			name:        "password in query or kv",
			input:       "POST /login with password=SuperSecretP@ssw0rd! and user=test",
			forbidden:   []string{"SuperSecretP@ssw0rd!"},
			mustContain: []string{"password=[REDACTED]"},
		},
		{
			name:        "api key or token in kv",
			input:       "Config has api_key: 9876543210abcdef and secret: mySuperSecretKey",
			forbidden:   []string{"9876543210abcdef", "mySuperSecretKey"},
			mustContain: []string{"api_key:[REDACTED]", "secret:[REDACTED]"},
		},
		{
			name:        "database connection url",
			input:       "Connecting to postgresql://dbuser:mypassword123@db.example.internal:5432/espacigo",
			forbidden:   []string{"mypassword123"},
			mustContain: []string{"postgresql://dbuser:[REDACTED]@db.example.internal:5432/espacigo"},
		},
		{
			name:        "credit card number",
			input:       "Payment card 4111 2222 3333 4444 charged",
			forbidden:   []string{"4111 2222 3333 4444"},
			mustContain: []string{"[REDACTED_CARD]"},
		},
		{
			name:        "chilean mobile phone",
			input:       "SMS sent to +56 9 8765 4321 successfully",
			forbidden:   []string{"+56 9 8765 4321"},
			mustContain: []string{"[REDACTED_PHONE]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safelog.Sanitize(tt.input)
			for _, bad := range tt.forbidden {
				if strings.Contains(got, bad) {
					t.Fatalf("Sanitize(%q) leaked %q in output: %q", tt.input, bad, got)
				}
			}
			for _, good := range tt.mustContain {
				if !strings.Contains(got, good) {
					t.Fatalf("Sanitize(%q) missing expected marker %q in output: %q", tt.input, good, got)
				}
			}
		})
	}
}

func TestTruncateUTF8(t *testing.T) {
	// Test basic truncation
	longAscii := strings.Repeat("A", 5000)
	truncated := safelog.TruncateUTF8(longAscii, 4096)
	if len(truncated) > 4096 {
		t.Fatalf("length %d > 4096", len(truncated))
	}
	if !strings.HasSuffix(truncated, safelog.TruncationMarker) {
		t.Fatalf("missing truncation marker in: %s", truncated[len(truncated)-30:])
	}

	// Test multi-byte UTF-8 character boundary (e.g. Spanish ñ and accented vowels, 2 bytes each)
	// Make sure it doesn't split a rune
	multiByte := strings.Repeat("españa—áéíóú", 500)
	truncatedMB := safelog.TruncateUTF8(multiByte, 4096)
	if len(truncatedMB) > 4096 {
		t.Fatalf("multibyte length %d > 4096", len(truncatedMB))
	}
	if !strings.HasSuffix(truncatedMB, safelog.TruncationMarker) {
		t.Fatalf("multibyte missing truncation marker")
	}
	prefix := strings.TrimSuffix(truncatedMB, safelog.TruncationMarker)
	if !bytes.Equal([]byte(prefix), []byte(string([]rune(prefix)))) {
		t.Fatalf("multibyte truncation split a rune: %q", prefix)
	}
}

func TestLoggerMaxEventBytes(t *testing.T) {
	var buf bytes.Buffer
	logger := safelog.New(&buf)

	hugePayload := strings.Repeat("X", 10000)
	ctx := safelog.ContextWithRequestID(context.Background(), "req-12345")
	logger.Error(ctx, "Huge event occurred", errors.New(hugePayload), safelog.F("detail", hugePayload))

	output := buf.Bytes()
	if len(output) > safelog.MaxEventBytes {
		t.Fatalf("logged event size %d exceeds MaxEventBytes (%d)", len(output), safelog.MaxEventBytes)
	}
	if output[len(output)-1] != '\n' {
		t.Fatalf("log line does not end with newline")
	}
	// Verify it contains truncation marker
	if !bytes.Contains(output, []byte(safelog.TruncationMarker)) {
		t.Fatalf("log event expected to contain truncation marker")
	}
	var event safelog.Event
	if err := json.Unmarshal(output, &event); err != nil || event.RequestID != "req-12345" || event.Level != "ERROR" {
		t.Fatalf("truncation lost structured event metadata: %v, %#v", err, event)
	}
}

func TestLoggerRejectsSensitiveAndUnstructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := safelog.New(&buf)
	ctx := safelog.ContextWithRequestID(context.Background(), "person@example.invalid")
	logger.Info(ctx, "started",
		safelog.F("password", "arbitrary-secret"),
		safelog.F("fields", map[string]string{"data": "nested-secret"}),
		safelog.F("message", "forged-message"),
		safelog.F("cookie", 123456),
		safelog.F("attempt", 3),
	)
	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["message"] != "started" || event["password"] != "[REDACTED]" || event["cookie"] != "[REDACTED]" || event["attempt"] != float64(3) {
		t.Fatalf("unexpected event fields: %v", event)
	}
	if event["request_id"] != nil || event["fields"] != nil || strings.Contains(buf.String(), "-secret") {
		t.Fatalf("unexpected private data in event: %v", event)
	}
}

func TestLoggerStructuredJSONAndRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := safelog.New(&buf)

	ctx := safelog.ContextWithRequestID(context.Background(), "corr-id-987")
	logger.Error(ctx, "User login failed with invalid password", errors.New("invalid password for user test@example.com"),
		safelog.F("email", "test@example.com"),
		safelog.F("attempt", 3),
	)

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to parse log line as JSON: %v, raw: %q", err, line)
	}

	if parsed["level"] != "ERROR" {
		t.Fatalf("expected level ERROR, got %v", parsed["level"])
	}
	if parsed["request_id"] != "corr-id-987" {
		t.Fatalf("expected request_id corr-id-987, got %v", parsed["request_id"])
	}
	if strings.Contains(line, "test@example.com") {
		t.Fatalf("email leaked into JSON log: %s", line)
	}
	if !strings.Contains(line, "[REDACTED_EMAIL]") {
		t.Fatalf("expected [REDACTED_EMAIL] in log: %s", line)
	}
}

func TestLogHTTPError(t *testing.T) {
	var buf bytes.Buffer
	safelog.Default().SetOutput(&buf)
	defer safelog.Default().SetOutput(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"foo@example.com"}`))
	req.Header.Set("X-Request-ID", "req-xyz-999")

	safelog.LogHTTPError(req, http.StatusUnauthorized, "Authentication failed", errors.New("user foo@example.com unauthorized"))

	output := buf.String()
	if !strings.Contains(output, "req-xyz-999") {
		t.Fatalf("expected request ID in log: %s", output)
	}
	if strings.Contains(output, "foo@example.com") {
		t.Fatalf("email leaked in HTTP error log: %s", output)
	}
	if !strings.Contains(output, "401") || !strings.Contains(output, "/api/v1/auth/login") {
		t.Fatalf("missing http_status or http_path in log: %s", output)
	}
}

func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := safelog.New(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := safelog.ContextWithRequestID(context.Background(), "req-conc")
			logger.Info(ctx, "Concurrent log test", safelog.F("worker", n))
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 log lines, got %d", len(lines))
	}
}
