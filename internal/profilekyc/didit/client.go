// Package didit contains the narrow Didit Sessions API v3 adapter used by
// profile KYC. It deliberately keeps provider transport types out of domain
// and HTTP handler packages.
package didit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const APIBaseURL = "https://verification.didit.me"

var (
	ErrInvalidConfig = errors.New("didit: invalid configuration")
	ErrInvalidInput  = errors.New("didit: invalid request")
	ErrRateLimited   = errors.New("didit: rate limited")
	ErrUnavailable   = errors.New("didit: unavailable")
	ErrHTTPStatus    = errors.New("didit: unexpected HTTP status")
	ErrInvalidURL    = errors.New("didit: invalid hosted session URL")
)

// Config holds transport settings. Didit selects sandbox vs live from the
// application/API key; both use the documented Sessions API base URL.
type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: api key required", ErrInvalidConfig)
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = APIBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !allowedAPIHost(u.Hostname()) {
		return nil, fmt.Errorf("%w: base URL must use a Didit API host over HTTPS", ErrInvalidConfig)
	}
	if u.Port() != "" && u.Port() != "443" {
		return nil, fmt.Errorf("%w: non-standard API port", ErrInvalidConfig)
	}
	if cfg.HTTPClient == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		cfg.HTTPClient = &http.Client{Timeout: timeout, Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 3 * time.Second,
		}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Client{baseURL: base, apiKey: cfg.APIKey, http: cfg.HTTPClient}, nil
}

func allowedAPIHost(host string) bool {
	return strings.EqualFold(host, "verification.didit.me") || strings.EqualFold(host, "verification-sandbox.didit.me")
}

type ExpectedDetails struct {
	FirstName             string   `json:"first_name,omitempty"`
	LastName              string   `json:"last_name,omitempty"`
	DateOfBirth           string   `json:"date_of_birth,omitempty"`
	IdentificationNumber  string   `json:"identification_number,omitempty"`
	ExpectedDocumentTypes []string `json:"expected_document_types,omitempty"`
}

// CreateSessionRequest sends only the profile fields needed for comparison.
// VendorData must be an opaque local attempt reference (for example a UUID),
// never an account identifier or personal data.
type CreateSessionRequest struct {
	WorkflowID      string          `json:"workflow_id"`
	VendorData      string          `json:"vendor_data"`
	ExpectedDetails ExpectedDetails `json:"expected_details,omitempty"`
	Callback        string          `json:"callback,omitempty"`
	Language        string          `json:"language,omitempty"`
}

type CreateSessionResponse struct {
	SessionID   string `json:"session_id"`
	SessionKind string `json:"session_kind,omitempty"`
	SessionURL  string `json:"url"`
	Status      string `json:"status,omitempty"`
	WorkflowID  string `json:"workflow_id,omitempty"`
	VendorData  string `json:"vendor_data,omitempty"`
}

type IDVerification struct {
	NodeID               string `json:"node_id,omitempty"`
	Status               string `json:"status"`
	FirstName            string `json:"first_name,omitempty"`
	LastName             string `json:"last_name,omitempty"`
	FullName             string `json:"full_name,omitempty"`
	DateOfBirth          string `json:"date_of_birth,omitempty"`
	IdentificationNumber string `json:"identification_number,omitempty"`
	PersonalNumber       string `json:"personal_number,omitempty"`
	DocumentNumber       string `json:"document_number,omitempty"`
	SerialNumber         string `json:"serial_number,omitempty"`
	IssuingState         string `json:"issuing_state,omitempty"`
	DocumentType         string `json:"document_type,omitempty"`
	ExpirationDate       string `json:"expiration_date,omitempty"`
}

type CheckResult struct {
	NodeID string  `json:"node_id,omitempty"`
	Status string  `json:"status"`
	Score  float64 `json:"score,omitempty"`
}

type Decision struct {
	SessionID       string           `json:"session_id"`
	WorkflowID      string           `json:"workflow_id,omitempty"`
	VendorData      string           `json:"vendor_data,omitempty"`
	Status          string           `json:"status"`
	IDVerifications []IDVerification `json:"id_verifications,omitempty"`
	LivenessChecks  []CheckResult    `json:"liveness_checks,omitempty"`
	FaceMatches     []CheckResult    `json:"face_matches,omitempty"`
}

func (d Decision) LivenessPassed() bool  { return allApproved(d.LivenessChecks) }
func (d Decision) FaceMatchPassed() bool { return allApproved(d.FaceMatches) }

func allApproved(items []CheckResult) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Status), "approved") {
			return false
		}
	}
	return true
}

func (c *Client) CreateSession(ctx context.Context, in CreateSessionRequest) (CreateSessionResponse, error) {
	if strings.TrimSpace(in.WorkflowID) == "" || strings.TrimSpace(in.VendorData) == "" {
		return CreateSessionResponse{}, fmt.Errorf("%w: workflow_id and opaque vendor_data are required", ErrInvalidInput)
	}
	var out CreateSessionResponse
	err := c.request(ctx, http.MethodPost, "/v3/session/", in, &out)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if strings.TrimSpace(out.SessionID) == "" || !validHostedURL(out.SessionURL) {
		return CreateSessionResponse{}, ErrInvalidURL
	}
	return out, nil
}

func (c *Client) GetDecision(ctx context.Context, sessionID string) (Decision, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || strings.ContainsAny(sessionID, "/?#") {
		return Decision{}, fmt.Errorf("%w: invalid session id", ErrInvalidInput)
	}
	var out Decision
	err := c.request(ctx, http.MethodGet, "/v3/session/"+url.PathEscape(sessionID)+"/decision/", nil, &out)
	if err != nil {
		return Decision{}, err
	}
	return out, nil
}

func validHostedURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	if u.Port() != "" && u.Port() != "443" {
		return false
	}
	return strings.EqualFold(u.Hostname(), "verify.didit.me")
}

func (c *Client) request(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.apiKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %d", ErrHTTPStatus, resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("didit: decode response: %w", err)
	}
	return nil
}
