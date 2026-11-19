package safelog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// MaxEventBytes defines the maximum UTF-8 byte length for a single log event (A.8.15 / A.8.12).
	MaxEventBytes = 4096
	// TruncationMarker is appended when an event exceeds MaxEventBytes.
	TruncationMarker = "...[truncated]"
)

type contextKey string

const (
	requestIDContextKey contextKey = "safelog_request_id"
)

// ContextWithRequestID binds a correlation or request ID to a context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

// RequestIDFromContext extracts the correlation or request ID from a context, if present.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDContextKey).(string); ok {
		return v
	}
	return ""
}

// RequestIDFromRequest extracts the correlation or request ID from an http.Request.
// It checks Context, X-Request-ID, and X-Correlation-ID headers.
func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	if id := r.Header.Get("X-Correlation-ID"); id != "" {
		return id
	}
	return ""
}

// Precompiled regular expressions for PII and sensitive data sanitization.
var (
	// Authorization headers: Authorization: Bearer <token> or Authorization: <token>
	reAuthHeader = regexp.MustCompile(`(?i)\bAuthorization\s*:\s*(?:Bearer\s+)?[A-Za-z0-9._~+/=-]+`)

	// Standalone Bearer tokens in headers or text: Bearer <alphanumeric/base64/urlsafe>
	reBearer = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)

	// Sensitive keys with values in key-value or query or JSON-like assignments:
	// password=..., secret=..., token=..., apiKey=...
	reSensitiveKV = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|apikey|api_key|access_token|refresh_token|auth_token|credential|private_key|database_url)\s*([:=])\s*([^\s,;'"}{]+)`)

	// Emails: basic RFC 5322 address pattern
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

	// Chilean RUTs: optional dots, dash, verifier digit 0-9 or k/K
	reRUT = regexp.MustCompile(`\b\d{1,2}(?:\.?\d{3}){2}-?[\dkK]\b`)

	// Credit Card / PAN (13 to 19 consecutive or separated digits)
	reCreditCard = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)

	// Phone numbers (Chilean and generic international mobile e.g. +56 9 1234 5678, +1-xxx-xxx-xxxx)
	rePhone = regexp.MustCompile(`(?:\+?56\s?9|\b9)\s?\d{4}\s?\d{4}\b`)

	// Database URI passwords: postgresql://user:password@host...
	reDBPassword = regexp.MustCompile(`(?i)([a-z]+://[^:]+:)([^@]+)(@)`)

	// Multi-whitespace / control characters
	reCRLF = regexp.MustCompile(`[\r\n]+`)
)

// NormalizeCRLF replaces line breaks (\r, \n) with escaped representation \n or space,
// preventing log splitting / CRLF injection (CWE-117).
func NormalizeCRLF(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	// Replace \r\n with a single space or escaped \n
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// Sanitize removes PII, credentials, tokens, and secrets from the input string.
func Sanitize(s string) string {
	if s == "" {
		return s
	}

	// 1. Database URLs with credentials
	s = reDBPassword.ReplaceAllString(s, "${1}[REDACTED]${3}")

	// 2. Authorization headers and Bearer tokens
	s = reAuthHeader.ReplaceAllString(s, "Authorization: Bearer [REDACTED]")
	s = reBearer.ReplaceAllString(s, "Bearer [REDACTED]")

	// 3. Sensitive key-value pairs (passwords, tokens, secrets)
	s = reSensitiveKV.ReplaceAllString(s, "${1}${2}[REDACTED]")

	// 4. Email addresses
	s = reEmail.ReplaceAllString(s, "[REDACTED_EMAIL]")

	// 5. Chilean RUTs
	s = reRUT.ReplaceAllString(s, "[REDACTED_RUT]")

	// 6. Credit cards / PAN
	s = reCreditCard.ReplaceAllStringFunc(s, func(match string) string {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, match)
		if len(digits) >= 13 && len(digits) <= 19 {
			return "[REDACTED_CARD]"
		}
		return match
	})

	// 7. Phone numbers
	s = rePhone.ReplaceAllString(s, "[REDACTED_PHONE]")

	return s
}

// TruncateUTF8 truncates a string to at most maxBytes, ensuring it does not split
// a multi-byte UTF-8 rune and appending TruncationMarker if truncation was necessary.
func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= len(TruncationMarker) {
		// Edge case: maxBytes is very small
		if maxBytes <= 0 {
			return ""
		}
		for i := maxBytes; i > 0; i-- {
			if utf8.RuneStart(s[i]) {
				return s[:i]
			}
		}
		return ""
	}

	target := maxBytes - len(TruncationMarker)
	// Find the largest valid rune boundary <= target
	for target > 0 && !utf8.RuneStart(s[target]) {
		target--
	}
	return s[:target] + TruncationMarker
}

// Field represents a structured key-value pair for logging.
type Field struct {
	Key   string
	Value any
}

// F creates a Field.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Event represents a single secure log entry.
type Event struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Error     string         `json:"error,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger is a concurrency-safe structured logger with sanitization, CRLF normalization,
// and 4096-byte truncation.
type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	clock  func() time.Time
	prefix string
}

var defaultLogger = New(os.Stderr)

// Default returns the default safelog Logger instance.
func Default() *Logger {
	return defaultLogger
}

// New creates a new secure Logger outputting to out.
func New(out io.Writer) *Logger {
	if out == nil {
		out = os.Stderr
	}
	return &Logger{
		out:   out,
		clock: time.Now,
	}
}

// SetOutput changes the output destination of the logger.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w == nil {
		w = os.Stderr
	}
	l.out = w
}

// Log writes a sanitized, bounded event.
func (l *Logger) Log(ctx context.Context, level, msg string, err error, fields ...Field) {
	now := l.clock()

	reqID := RequestIDFromContext(ctx)

	// Clean and normalize message
	cleanMsg := NormalizeCRLF(Sanitize(msg))

	var errStr string
	if err != nil {
		// Normalize and sanitize error message, but never include stack traces or credentials
		errStr = NormalizeCRLF(Sanitize(err.Error()))
	}

	eventMap := make(map[string]any, len(fields)+5)
	eventMap["timestamp"] = now.UTC().Format(time.RFC3339Nano)
	eventMap["level"] = level
	eventMap["message"] = cleanMsg

	if reqID != "" {
		eventMap["request_id"] = reqID
	}
	if errStr != "" {
		eventMap["error"] = errStr
	}

	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		// Sanitize field values
		switch val := f.Value.(type) {
		case string:
			eventMap[f.Key] = NormalizeCRLF(Sanitize(val))
		case fmt.Stringer:
			eventMap[f.Key] = NormalizeCRLF(Sanitize(val.String()))
		case error:
			eventMap[f.Key] = NormalizeCRLF(Sanitize(val.Error()))
		default:
			eventMap[f.Key] = val
		}
	}

	data, jsonErr := json.Marshal(eventMap)
	if jsonErr != nil {
		// Fallback minimal event
		data = []byte(fmt.Sprintf(`{"timestamp":%q,"level":%q,"message":%q,"error":"json_marshal_error"}`,
			now.UTC().Format(time.RFC3339Nano), level, cleanMsg))
	}

	// Ensure newline fits within MaxEventBytes: data + '\n' <= MaxEventBytes
	maxBody := MaxEventBytes - 1
	if len(data) > maxBody {
		truncated := TruncateUTF8(string(data), maxBody)
		data = []byte(truncated)
	}

	var buf bytes.Buffer
	buf.Grow(len(data) + 1)
	buf.Write(data)
	buf.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(buf.Bytes())
}

// Info logs an informational event.
func (l *Logger) Info(ctx context.Context, msg string, fields ...Field) {
	l.Log(ctx, "INFO", msg, nil, fields...)
}

// Warn logs a warning event.
func (l *Logger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.Log(ctx, "WARN", msg, nil, fields...)
}

// Error logs an error event.
func (l *Logger) Error(ctx context.Context, msg string, err error, fields ...Field) {
	l.Log(ctx, "ERROR", msg, err, fields...)
}

// Package-level convenience functions using Default()

// Info logs an informational message via Default().
func Info(ctx context.Context, msg string, fields ...Field) {
	defaultLogger.Info(ctx, msg, fields...)
}

// Warn logs a warning message via Default().
func Warn(ctx context.Context, msg string, fields ...Field) {
	defaultLogger.Warn(ctx, msg, fields...)
}

// Error logs an error message via Default().
func Error(ctx context.Context, msg string, err error, fields ...Field) {
	defaultLogger.Error(ctx, msg, err, fields...)
}

// LogHTTPError logs an HTTP error event safely using the request's context and headers.
func LogHTTPError(r *http.Request, status int, msg string, err error, fields ...Field) {
	reqID := RequestIDFromRequest(r)
	ctx := r.Context()
	if reqID != "" && RequestIDFromContext(ctx) == "" {
		ctx = ContextWithRequestID(ctx, reqID)
	}
	allFields := append([]Field{
		F("http_status", status),
		F("http_method", r.Method),
		F("http_path", r.URL.Path),
	}, fields...)
	defaultLogger.Error(ctx, msg, err, allFields...)
}
