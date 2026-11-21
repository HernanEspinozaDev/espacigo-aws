package marketplaceoauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPExchangerExchangeUsesExpectedFormAndAcceptsNumericUserID(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content type = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range map[string]string{
			"grant_type": "authorization_code", "client_id": "app-id", "client_secret": "secret",
			"code": "one-time-code", "redirect_uri": "https://example.test/callback",
			"code_verifier": "verifier", "test_token": "true",
		} {
			if form.Get(key) != value {
				t.Errorf("form %s = %q", key, form.Get(key))
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"a","refresh_token":"r","user_id":123456789,"expires_in":3600,"live_mode":false}`)), Header: make(http.Header)}, nil
	})}
	exchanger := &HTTPExchanger{ClientID: "app-id", ClientSecret: "secret", RedirectURI: "https://example.test/callback", Client: client}
	tokens, err := exchanger.Exchange(context.Background(), "one-time-code", "verifier", true)
	if err != nil {
		t.Fatal(err)
	}
	if tokens.UserID != "123456789" || tokens.LiveMode == nil || *tokens.LiveMode {
		t.Fatalf("unexpected normalized token fields: user_id=%q live_mode=%v", tokens.UserID, tokens.LiveMode)
	}
}

func TestHTTPExchangerReturnsOnlyAllowlistedProviderFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"error":"invalid_grant","error_description":"sensitive diagnostic and access_token=must-not-escape"}`
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	exchanger := &HTTPExchanger{Client: client}
	_, err := exchanger.Exchange(context.Background(), "code", "verifier", true)
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("error %v does not wrap ErrProvider", err)
	}
	var failure ProviderFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T is not ProviderFailure", err)
	}
	if failure.Status != http.StatusBadRequest || failure.Code != "invalid_grant" {
		t.Fatalf("unexpected diagnostic: %#v", failure)
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("provider body escaped in error: %v", err)
	}
}

func TestProviderErrorCodeRejectsUnrecognizedValues(t *testing.T) {
	for _, input := range []string{`{"error":"internal_server_error"}`, `not-json`} {
		if got := providerErrorCode([]byte(input)); got != "unknown" {
			t.Errorf("providerErrorCode(%q) = %q", input, got)
		}
	}
}

func TestAllowlistedAuthorizationErrorDoesNotEchoUnknownInput(t *testing.T) {
	for input, want := range map[string]string{
		"access_denied":               "access_denied",
		"server_error":                "server_error",
		"invalid_client secret-value": "unknown",
		"<untrusted>\nvalue":          "unknown",
	} {
		if got := allowlistedAuthorizationError(input); got != want {
			t.Errorf("allowlistedAuthorizationError(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTokenValidationReportsOnlyFieldLevelCodes(t *testing.T) {
	live := false
	base := TokenResponse{AccessToken: "access", RefreshToken: "refresh", UserID: "123", ExpiresIn: int64((180 * 24 * time.Hour) / time.Second), Scope: "offline_access read", LiveMode: &live}
	tests := []struct {
		name   string
		mutate func(*TokenResponse)
		want   string
	}{
		{name: "missing access", mutate: func(v *TokenResponse) { v.AccessToken = "" }, want: "missing_access_token"},
		{name: "missing refresh", mutate: func(v *TokenResponse) { v.RefreshToken = "" }, want: "missing_refresh_token"},
		{name: "missing seller id", mutate: func(v *TokenResponse) { v.UserID = "" }, want: "missing_user_id"},
		{name: "malformed seller id", mutate: func(v *TokenResponse) { v.UserID = "seller" }, want: "invalid_user_id"},
		{name: "invalid expiry", mutate: func(v *TokenResponse) { v.ExpiresIn = 0 }, want: "invalid_expires_in"},
		{name: "expiry range", mutate: func(v *TokenResponse) { v.ExpiresIn = int64((366 * 24 * time.Hour) / time.Second) }, want: "expires_in_out_of_range"},
		{name: "missing mode without sandbox proof", mutate: func(v *TokenResponse) { v.LiveMode = nil }, want: "sandbox_mode_unverified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := base
			tt.mutate(&response)
			err := validateTokens(response, true)
			var failure TokenValidationFailure
			if !errors.As(err, &failure) || failure.Reason != tt.want {
				t.Fatalf("validation result = %#v (%v), want reason %q", failure, err, tt.want)
			}
			if (response.AccessToken != "" && strings.Contains(err.Error(), response.AccessToken)) || (response.RefreshToken != "" && strings.Contains(err.Error(), response.RefreshToken)) || (response.UserID != "" && strings.Contains(err.Error(), response.UserID)) {
				t.Fatalf("token values escaped validation error: %v", err)
			}
		})
	}
	if err := validateTokens(base, true); err != nil {
		t.Fatalf("valid sandbox response rejected: %v", err)
	}
	testWithoutMode := base
	testWithoutMode.AccessToken = "TEST-sandbox-token"
	testWithoutMode.LiveMode = nil
	if err := validateTokens(testWithoutMode, true); err != nil {
		t.Fatalf("official TEST-prefixed sandbox token without live_mode rejected: %v", err)
	}
	appUserWithoutMode := base
	appUserWithoutMode.AccessToken = "APP_USR-test-or-live"
	appUserWithoutMode.LiveMode = nil
	var appUserFailure TokenValidationFailure
	if err := validateTokens(appUserWithoutMode, true); !errors.As(err, &appUserFailure) || appUserFailure.Reason != "sandbox_mode_unverified" {
		t.Fatalf("APP_USR without live_mode result = %v, want sandbox_mode_unverified", err)
	}
	var productionTestTokenFailure TokenValidationFailure
	if err := validateTokens(testWithoutMode, false); !errors.As(err, &productionTestTokenFailure) || productionTestTokenFailure.Reason != "missing_live_mode" {
		t.Fatalf("TEST token in production mode result = %v, want missing_live_mode", err)
	}
	trueMode := true
	testWithLiveMode := testWithoutMode
	testWithLiveMode.LiveMode = &trueMode
	if err := validateTokens(testWithLiveMode, true); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("TEST-prefixed token with live_mode=true result = %v, want ErrWrongMode", err)
	}
	live = true
	if err := validateTokens(TokenResponse{AccessToken: "a", RefreshToken: "r", UserID: "1", ExpiresIn: 3600, LiveMode: &live}, true); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("live token mode result = %v, want ErrWrongMode", err)
	}
}
