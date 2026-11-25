package apidocs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnabledRejectsProductionAndRequiresDevOrPrototype(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "production profile always disabled",
			env:  map[string]string{"APP_PROFILE": "production", "ENVIRONMENT": "development", "LOCAL_AUTH_PROTOTYPE": "1"},
			want: false,
		},
		{
			name: "empty environment disabled",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "development enabled",
			env:  map[string]string{"ENVIRONMENT": "development"},
			want: true,
		},
		{
			name: "local auth prototype enabled",
			env:  map[string]string{"LOCAL_AUTH_PROTOTYPE": "1"},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Enabled(func(k string) string { return tc.env[k] })
			if got != tc.want {
				t.Fatalf("Enabled()=%t, want %t", got, tc.want)
			}
		})
	}
}

func localRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+path, nil)
	req.RemoteAddr = "127.0.0.1:42000"
	return req
}

func TestHandlerProjectsMercadoPagoOpenAPI31AndScalarReference(t *testing.T) {
	specPath := filepath.Join("..", "..", "planning", "openapi.yaml")
	h, err := NewHandlerFromFile(specPath)
	if err != nil {
		t.Fatalf("NewHandlerFromFile: %v", err)
	}
	mux := http.NewServeMux()
	h.Mount(mux)

	t.Run("mercadopago_scope_projects_payment_paths", func(t *testing.T) {
		req := localRequest("/openapi.json?scope=mercadopago")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control=%q, want no-store", got)
		}
		var doc struct {
			OpenAPI    string                    `json:"openapi"`
			Paths      map[string]map[string]any `json:"paths"`
			Components map[string]map[string]any `json:"components"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("decode /openapi.json: %v", err)
		}
		if doc.OpenAPI != "3.1.0" {
			t.Fatalf("openapi=%q, want 3.1.0", doc.OpenAPI)
		}
		if len(doc.Paths) != len(mercadoPagoPaths) {
			t.Fatalf("projected paths count=%d, want %d", len(doc.Paths), len(mercadoPagoPaths))
		}
		for _, expectedPath := range mercadoPagoPaths {
			if _, ok := doc.Paths[expectedPath]; !ok {
				t.Fatalf("missing Mercado Pago path %q in projected /openapi.json", expectedPath)
			}
		}
		for _, requiredSchema := range []string{
			"MercadoPagoHostConnection",
			"MercadoPagoOAuthStart",
			"CheckoutPreferenceEnvelope",
			"CheckoutStatusEnvelope",
			"CheckoutAPIPaymentRequest",
			"CheckoutAPIPaymentEnvelope",
		} {
			if _, ok := doc.Components["schemas"][requiredSchema]; !ok {
				t.Fatalf("missing referenced schema %q in projected components", requiredSchema)
			}
		}
	})

	t.Run("scalar_docs_renders_html_with_strict_csp", func(t *testing.T) {
		req := localRequest("/docs")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control=%q, want no-store", got)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "connect-src 'self'") {
			t.Fatalf("unexpected CSP header: %q", csp)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-url="/openapi.json?scope=all"`) || !strings.Contains(body, "hideTestRequestButton: true") {
			t.Fatalf("Scalar HTML did not include expected data-url or script tag")
		}
	})

	t.Run("full_and_admin_scopes", func(t *testing.T) {
		for _, tc := range []struct {
			path string
			want []string
			omit string
		}{
			{"/openapi.json", []string{"/admin/global/me", "/public/spaces", "/host/settlements"}, ""},
			{"/openapi.json?scope=admin", []string{"/admin/global/me", "/document-verification/reviewer/cases"}, "/public/spaces"},
		} {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, localRequest(tc.path))
			var doc struct{ Paths map[string]any }
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &doc) != nil {
				t.Fatalf("could not read %s: status %d", tc.path, rec.Code)
			}
			for _, path := range tc.want {
				if doc.Paths[path] == nil {
					t.Errorf("%s missing from %s", path, tc.path)
				}
			}
			if tc.omit != "" && doc.Paths[tc.omit] != nil {
				t.Errorf("%s leaked into %s", tc.omit, tc.path)
			}
		}
	})

	t.Run("docs_and_specs_require_direct_loopback", func(t *testing.T) {
		for _, path := range []string{"/docs", "/openapi.json", "/openapi.yaml"} {
			for _, change := range []func(*http.Request){
				func(r *http.Request) { r.Host = "public.example.test" },
				func(r *http.Request) { r.RemoteAddr = "203.0.113.1:42000" },
				func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "public.example.test") },
			} {
				req := localRequest(path)
				change(req)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				if rec.Code != http.StatusNotFound {
					t.Errorf("%s returned %d", path, rec.Code)
				}
			}
		}
	})

	t.Run("invalid_scope_rejected", func(t *testing.T) {
		req := localRequest("/openapi.json?scope=unknown")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", rec.Code)
		}
	})
}
