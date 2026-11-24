package publiccataloghttp

import (
	"net/http/httptest"
	"testing"
)

func TestPublicFiltersParseMinimumCapacity(t *testing.T) {
	f, ok := publicFilters(httptest.NewRequest("GET", "/api/v1/public/spaces?min_capacity=7", nil))
	if !ok || f.MinCapacity == nil || *f.MinCapacity != 7 {
		t.Fatalf("parsed filters=%+v ok=%v", f, ok)
	}
	for _, raw := range []string{"0", "-1", "nope", "1&min_capacity=2"} {
		f, ok := publicFilters(httptest.NewRequest("GET", "/api/v1/public/spaces?min_capacity="+raw, nil))
		if ok { t.Fatalf("invalid min_capacity %q accepted: %+v", raw, f) }
	}
}
