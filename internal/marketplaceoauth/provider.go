package marketplaceoauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPExchanger struct {
	ClientID, ClientSecret, RedirectURI string
	TokenURL                            string
	Client                              *http.Client
}

func (e *HTTPExchanger) Exchange(ctx context.Context, code, verifier string, testMode bool) (TokenResponse, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {e.ClientID}, "client_secret": {e.ClientSecret}, "code": {code}, "redirect_uri": {e.RedirectURI}, "code_verifier": {verifier}}
	if testMode {
		form.Set("test_token", "true")
	}
	return e.request(ctx, form)
}
func (e *HTTPExchanger) Refresh(ctx context.Context, refreshToken string, testMode bool) (TokenResponse, error) {
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {e.ClientID}, "client_secret": {e.ClientSecret}, "refresh_token": {refreshToken}}
	if testMode {
		form.Set("test_token", "true")
	}
	return e.request(ctx, form)
}
func (e *HTTPExchanger) request(ctx context.Context, form url.Values) (TokenResponse, error) {
	endpoint := e.TokenURL
	if endpoint == "" {
		endpoint = "https://api.mercadopago.com/oauth/token"
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return TokenResponse{}, ErrProvider
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, ErrProvider
	}
	var result TokenResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 32<<10))
	if err = decoder.Decode(&result); err != nil {
		return TokenResponse{}, ErrProvider
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return TokenResponse{}, ErrProvider
	}
	return result, nil
}
