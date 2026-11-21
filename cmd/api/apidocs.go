package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/HernanEspinozaDev/espaciGo/internal/apidocs"
)

func mountLocalAPIDocs(mux *http.ServeMux, getenv func(string) string) {
	if mux == nil || !apidocs.Enabled(getenv) {
		return
	}
	candidates := []string{
		"planning/openapi.yaml",
		"/openapi.yaml",
		filepath.Join("..", "..", "planning", "openapi.yaml"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if h, err := apidocs.NewHandlerFromFile(candidate); err == nil {
				h.Mount(mux)
				return
			}
		}
	}
}
