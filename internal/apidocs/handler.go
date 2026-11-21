// Package apidocs serves the OpenAPI 3.1 JSON contract and the interactive
// Scalar API Reference exclusively in non-production development runtimes.
// By default it projects the Mercado Pago / Payments module scope so developers
// can inspect and test OAuth, Checkout Pro, Wallet, Checkout API, and Webhooks.
package apidocs

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var mercadoPagoPaths = []string{
	"/host/mercadopago",
	"/host/mercadopago/oauth/start",
	"/host/mercadopago/oauth/callback",
	"/rental-quotes/{quoteID}/checkout",
	"/rental-quotes/{quoteID}/checkout-wallet",
	"/rental-quotes/{quoteID}/checkout-api",
	"/webhooks/mercadopago",
}

type Handler struct {
	fullJSON        []byte
	mercadoPagoJSON []byte
}

func Enabled(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(getenv("APP_PROFILE")), "production") {
		return false
	}
	env := strings.TrimSpace(getenv("ENVIRONMENT"))
	return env == "development" || getenv("LOCAL_AUTH_PROTOTYPE") == "1"
}

func NewHandlerFromFile(specPath string) (*Handler, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	return NewHandlerFromYAML(raw)
}

func NewHandlerFromYAML(raw []byte) (*Handler, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc) == 0 {
		return nil, errors.New("apidocs: empty OpenAPI specification")
	}
	fullJSON, err := marshalIndentJSON(doc)
	if err != nil {
		return nil, err
	}
	mpDoc, err := projectMercadoPagoSpec(doc)
	if err != nil {
		return nil, err
	}
	mpJSON, err := marshalIndentJSON(mpDoc)
	if err != nil {
		return nil, err
	}
	return &Handler{
		fullJSON:        fullJSON,
		mercadoPagoJSON: mpJSON,
	}, nil
}

func (h *Handler) Mount(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.HandleFunc("GET /openapi.json", h.ServeOpenAPIJSON)
	mux.HandleFunc("GET /docs", h.ServeScalarDocs)
}

func (h *Handler) ServeOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, `{"error":{"code":"method_not_allowed"}}`, http.StatusMethodNotAllowed)
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	payload := h.mercadoPagoJSON
	if scope == "all" || scope == "full" {
		payload = h.fullJSON
	} else if scope != "" && scope != "mercadopago" && scope != "payments" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_scope"}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *Handler) ServeScalarDocs(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	specURL := "/openapi.json?scope=mercadopago"
	title := "EspaciGo API — Módulo Mercado Pago (OpenAPI 3.1)"
	if scope == "all" || scope == "full" {
		specURL = "/openapi.json?scope=all"
		title = "EspaciGo API — Contrato Completo (OpenAPI 3.1)"
	} else if scope != "" && scope != "mercadopago" && scope != "payments" {
		http.Error(w, "Invalid scope", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; style-src 'self' https://cdn.jsdelivr.net https://fonts.googleapis.com 'unsafe-inline'; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(scalarHTML(title, specURL)))
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func scalarHTML(title, specURL string) string {
	return `<!doctype html>
<html lang="es">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="robots" content="noindex, nofollow" />
  <title>` + title + `</title>
  <style>
    body { margin: 0; font-family: system-ui, -apple-system, sans-serif; background: #0f172a; }
    .espacigo-doc-banner {
      display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between;
      gap: 12px; padding: 10px 18px; background: #1e293b; color: #f8fafc;
      border-bottom: 1px solid #334155; font-size: 13px;
    }
    .espacigo-doc-banner a {
      color: #38bdf8; text-decoration: none; font-weight: 600; margin-left: 12px;
    }
    .espacigo-doc-banner a:hover { text-decoration: underline; }
    .espacigo-badge {
      display: inline-block; padding: 2px 8px; border-radius: 999px;
      background: #0284c7; color: #fff; font-weight: 600; font-size: 11px;
    }
  </style>
</head>
<body>
  <header class="espacigo-doc-banner">
    <div>
      <span class="espacigo-badge">OpenAPI 3.1 · Local Dev</span>
      <strong style="margin-left:8px;">` + title + `</strong>
    </div>
    <nav aria-label="Alcance de documentación OpenAPI">
      <a href="/docs?scope=mercadopago">Módulo Mercado Pago</a>
      <a href="/docs?scope=all">API Completa</a>
      <a href="` + specURL + `" target="_blank" rel="noreferrer">JSON Spec</a>
    </nav>
  </header>
  <script id="api-reference" data-url="` + specURL + `"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
}

func projectMercadoPagoSpec(full map[string]any) (map[string]any, error) {
	allPaths, ok := full["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("apidocs: missing paths object")
	}
	selectedPaths := make(map[string]any, len(mercadoPagoPaths))
	for _, p := range mercadoPagoPaths {
		if item, exists := allPaths[p]; exists {
			selectedPaths[p] = item
		}
	}
	if len(selectedPaths) == 0 {
		return nil, errors.New("apidocs: no Mercado Pago paths found in OpenAPI spec")
	}

	info := map[string]any{
		"title":       "EspaciGo API — Módulo de Pagos y Mercado Pago",
		"version":     "1.0.0",
		"description": "Contrato OpenAPI 3.1 focalizado en todas las interacciones con Mercado Pago en EspaciGo: vinculación OAuth 2.0 + PKCE del anfitrión vendedor, creación y consulta de preferencias Checkout Pro y Wallet, cobro con tarjeta tokenizada mediante Checkout API (Payments API) y recepción de Webhooks firmados con HMAC-SHA256.",
	}
	tags := []map[string]string{
		{
			"name":        "Payments",
			"description": "Operaciones de pago, preferencias Checkout Pro, cobros tokenizados Checkout API y recepción de webhooks de Mercado Pago.",
		},
		{
			"name":        "Host",
			"description": "Vinculación OAuth 2.0 + PKCE de la cuenta Mercado Pago del anfitrión vendedor.",
		},
		{
			"name":        "Rental",
			"description": "Flujo comercial de cotizaciones de arriendo asociado al checkout.",
		},
	}

	projected := map[string]any{
		"openapi": full["openapi"],
		"info":    info,
		"servers": full["servers"],
		"tags":    tags,
		"paths":   selectedPaths,
	}
	if sec, exists := full["security"]; exists {
		projected["security"] = sec
	}
	if comp, exists := full["components"]; exists {
		projected["components"] = filterReferencedComponents(selectedPaths, comp)
	}
	return projected, nil
}

func filterReferencedComponents(selectedPaths map[string]any, rawComponents any) any {
	components, ok := rawComponents.(map[string]any)
	if !ok {
		return rawComponents
	}
	filtered := make(map[string]any, len(components))
	if secSchemes, exists := components["securitySchemes"]; exists {
		filtered["securitySchemes"] = secSchemes
	}

	needed := map[string]map[string]bool{
		"schemas":    {},
		"responses":  {},
		"parameters": {},
		"headers":    {},
	}
	queue := []any{selectedPaths}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		collectRefs(current, func(section, name string) {
			bucket, tracked := needed[section]
			if !tracked || bucket[name] {
				return
			}
			bucket[name] = true
			if secMap, ok := components[section].(map[string]any); ok {
				if target, exists := secMap[name]; exists {
					queue = append(queue, target)
				}
			}
		})
	}

	for section, names := range needed {
		secMap, ok := components[section].(map[string]any)
		if !ok || len(names) == 0 {
			continue
		}
		sub := make(map[string]any, len(names))
		for name := range names {
			if val, exists := secMap[name]; exists {
				sub[name] = val
			}
		}
		if len(sub) > 0 {
			filtered[section] = sub
		}
	}
	return filtered
}

func collectRefs(node any, visit func(section, name string)) {
	switch typed := node.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			const prefix = "#/components/"
			if strings.HasPrefix(ref, prefix) {
				rest := strings.TrimPrefix(ref, prefix)
				if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 {
					visit(parts[0], parts[1])
				}
			}
		}
		for _, v := range typed {
			collectRefs(v, visit)
		}
	case []any:
		for _, item := range typed {
			collectRefs(item, visit)
		}
	}
}

func marshalIndentJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
