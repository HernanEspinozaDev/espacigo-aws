// Package apidocs serves the OpenAPI 3.1 JSON contract and the interactive
// Scalar API Reference exclusively in non-production development runtimes.
// It exposes the complete contract and focused views for payments and administration.
package apidocs

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var mercadoPagoPaths = []string{
	"/host/settlements",
	"/host/settlements/{reservationID}",
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
	fullYAML        []byte
	mercadoPagoJSON []byte
	adminJSON       []byte
}

func Enabled(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	if profile := strings.ToLower(strings.TrimSpace(getenv("APP_PROFILE"))); profile != "" && profile != "local" && profile != "development" {
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
	adminDoc, err := projectSpec(doc, "EspaciGo API — Administración", "Operaciones de administración y revisión interna. Consulta la matriz de montaje para conocer el entorno.", isAdminPath)
	if err != nil {
		return nil, err
	}
	adminJSON, err := marshalIndentJSON(adminDoc)
	if err != nil {
		return nil, err
	}
	return &Handler{
		fullYAML:        append([]byte(nil), raw...),
		adminJSON:       adminJSON,
		fullJSON:        fullJSON,
		mercadoPagoJSON: mpJSON,
	}, nil
}

func (h *Handler) Mount(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.HandleFunc("GET /openapi.json", localOnly(h.ServeOpenAPIJSON))
	mux.HandleFunc("GET /openapi.yaml", localOnly(h.serveOpenAPIYAML))
	mux.HandleFunc("GET /docs", localOnly(h.ServeScalarDocs))
}

func localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remote, _, _ := net.SplitHostPort(r.RemoteAddr)
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		if !net.ParseIP(remote).IsLoopback() ||
			(host != "localhost" && !net.ParseIP(strings.Trim(host, "[]")).IsLoopback()) {
			http.NotFound(w, r)
			return
		}
		for name := range r.Header {
			key := strings.ToLower(name)
			switch {
			case key == "forwarded", key == "x-real-ip", key == "cf-connecting-ip", strings.HasPrefix(key, "x-forwarded-"):
				http.NotFound(w, r)
				return
			}
		}
		next(w, r)
	}
}

func (h *Handler) scope(raw string) (name, title string, payload []byte) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all", "full":
		return "all", "EspaciGo API — Contrato completo", h.fullJSON
	case "mercadopago", "payments":
		return "mercadopago", "EspaciGo API — Pagos y Mercado Pago", h.mercadoPagoJSON
	case "admin":
		return "admin", "EspaciGo API — Administración", h.adminJSON
	default:
		return "", "", nil
	}
}

func (h *Handler) ServeOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _, payload := h.scope(r.URL.Query().Get("scope"))
	if payload == nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_scope"}}`))
		return
	}
	_, _ = w.Write(payload)
}

func (h *Handler) serveOpenAPIYAML(w http.ResponseWriter, _ *http.Request) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(h.fullYAML)
}

func (h *Handler) ServeScalarDocs(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	name, title, _ := h.scope(r.URL.Query().Get("scope"))
	if name == "" {
		http.Error(w, "Invalid scope", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; style-src 'self' https://cdn.jsdelivr.net https://fonts.googleapis.com 'unsafe-inline'; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(scalarHTML(title, "/openapi.json?scope="+name)))
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
      <a href="/docs?scope=all">API completa</a>
      <a href="/docs?scope=admin">Administración</a>
      <a href="` + specURL + `" target="_blank" rel="noreferrer">JSON Spec</a>
    </nav>
  </header>
  <div id="api-reference" data-url="` + specURL + `"></div>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  <script>
    const reference = document.getElementById('api-reference');
    Scalar.createApiReference('#api-reference', {
      url: reference.dataset.url,
      hideTestRequestButton: true,
      hideClientButton: true,
      showOperationId: true,
    });
  </script>
</body>
</html>`
}

func isAdminPath(path string) bool {
	return strings.HasPrefix(path, "/admin/") ||
		strings.HasPrefix(path, "/document-verification/reviewer/") ||
		strings.HasPrefix(path, "/privacy/suppression-requests") ||
		path == "/privacy/retention/purge"
}

func projectMercadoPagoSpec(full map[string]any) (map[string]any, error) {
	return projectSpec(full, "EspaciGo API — Pagos y Mercado Pago",
		"OAuth del anfitrión, Checkout Pro, Wallet, Checkout API, webhooks y liquidaciones del anfitrión.",
		func(path string) bool {
			for _, selected := range mercadoPagoPaths {
				if selected == path {
					return true
				}
			}
			return false
		})
}

func projectSpec(full map[string]any, title, description string, includes func(string) bool) (map[string]any, error) {
	allPaths, ok := full["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("apidocs: missing paths object")
	}
	selectedPaths := make(map[string]any)
	for path, item := range allPaths {
		if includes(path) {
			selectedPaths[path] = item
		}
	}
	if len(selectedPaths) == 0 {
		return nil, errors.New("apidocs: no paths found for documentation scope")
	}
	info, _ := full["info"].(map[string]any)
	projected := map[string]any{
		"openapi": full["openapi"],
		"info":    map[string]any{"title": title, "description": description, "version": info["version"]},
		"servers": full["servers"],
		"paths":   selectedPaths,
	}
	if tags, ok := full["tags"].([]any); ok {
		usedTags := make(map[string]bool)
		for _, rawPath := range selectedPaths {
			path, _ := rawPath.(map[string]any)
			for _, rawOperation := range path {
				operation, _ := rawOperation.(map[string]any)
				operationTags, _ := operation["tags"].([]any)
				for _, rawTag := range operationTags {
					if name, ok := rawTag.(string); ok {
						usedTags[name] = true
					}
				}
			}
		}
		var selectedTags []any
		for _, rawTag := range tags {
			tag, _ := rawTag.(map[string]any)
			name, _ := tag["name"].(string)
			if usedTags[name] {
				selectedTags = append(selectedTags, tag)
			}
		}
		if len(selectedTags) > 0 {
			projected["tags"] = selectedTags
		}
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

	needed := make(map[string]map[string]bool, len(components))
	for section := range components {
		if section != "securitySchemes" {
			needed[section] = make(map[string]bool)
		}
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
