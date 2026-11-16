package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicSpaceMonthRoutePreservesCatalogAndPrivateDispatch(t *testing.T) {
	mux := http.NewServeMux()
	marker := func(value string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(value))
		})
	}
	if err := mountPublicCatalogRoutes(mux, marker("catalog"), false, true, false); err != nil {
		t.Fatal(err)
	}
	mux.Handle("/api/v1/spaces/", marker("owner"))
	registerPublicSpaceMonthRoute(mux, marker("month"))

	const id = "00000000-0000-4000-8000-000000000099"
	for _, tc := range []struct{ path, want string }{
		{"/api/v1/public/spaces", "catalog"},
		{"/api/v1/public/spaces/" + id, "catalog"},
		{"/api/v1/public/spaces/" + id + "/gallery", "catalog"},
		{"/api/v1/public/spaces/" + id + "/month?month=2026-11", "month"},
		{"/api/v1/spaces/" + id + "/availability", "owner"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusOK || w.Body.String() != tc.want {
				t.Fatalf("dispatch = %d %q, want 200 %q", w.Code, w.Body.String(), tc.want)
			}
		})
	}
}
