package earlyrentalhttp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed demo/*
var demoAssets embed.FS

func registerDemoScreen(mux *http.ServeMux) {
	assets, err := fs.Sub(demoAssets, "demo")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(assets)
	mux.Handle("GET /demo/arriendo/", http.StripPrefix("/demo/arriendo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/demo.js", "/demo.css":
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'")
		files.ServeHTTP(w, r)
	})))
}
