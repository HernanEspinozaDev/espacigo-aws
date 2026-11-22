package publiccataloghttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
)

func TestPublicSpaceListOmitsDetailOnlyFields(t *testing.T) {
	name := "Host público"
	cancellation, guarantee := "cancelación", "garantía"
	repo := &pageSizeRepository{spaces: []publiccatalog.Space{{
		SpaceID: "11111111-1111-4111-8111-111111111111", CapacityMax: 12,
		Host:               &publiccatalog.PublicHost{Name: &name, JoinedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), SpacesCount: 3, IdentityVerified: true},
		CancellationPolicy: &cancellation, Guarantee: &guarantee,
	}}}
	service, err := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items=%d, want 1", len(body.Items))
	}
	for _, field := range []string{"capacity_max", "host", "cancellation_policy", "guarantee"} {
		if _, ok := body.Items[0][field]; ok {
			t.Errorf("list unexpectedly includes %s", field)
		}
	}
}

func TestPublicSpaceDetailPreservesHostCapacityAndNullTerms(t *testing.T) {
	name := "Host público"
	repo := &pageSizeRepository{detail: &publiccatalog.Space{SpaceID: "11111111-1111-4111-8111-111111111111", CapacityMax: 12,
		Host: &publiccatalog.PublicHost{Name: &name, JoinedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), SpacesCount: 3, IdentityVerified: true}}}
	service, err := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/11111111-1111-4111-8111-111111111111", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(body["data"], &data); err != nil {
		t.Fatal(err)
	}
	if string(data["capacity_max"]) != "12" || !strings.Contains(string(data["host"]), `"name":"Host"`) || strings.Contains(string(data["host"]), "público") {
		t.Fatalf("detail fields capacity=%s host=%s", data["capacity_max"], data["host"])
	}
	for _, field := range []string{"cancellation_policy", "guarantee"} {
		if string(data[field]) != "null" {
			t.Errorf("%s=%s, want null", field, data[field])
		}
	}
}

func TestPublicSpaceDetailResolvesKnownCancellationPolicyAndHidesUnknownVersion(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, version string
		want          string
	}{
		{name: "known version", version: "flexible_v1", want: "Flexible: reembolso total hasta 24 horas antes del inicio del arriendo."},
		{name: "unknown version", version: "future_policy", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &pageSizeRepository{detail: &publiccatalog.Space{
				SpaceID: id, CancellationPolicyVersion: tc.version,
			}}
			service, err := publiccatalog.New(repo, []byte("01234567890123456789012345678901"))
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+id, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var envelope struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			want, _ := json.Marshal(any(nil))
			if tc.want != "" {
				want, _ = json.Marshal(tc.want)
			}
			if got := string(envelope.Data["cancellation_policy"]); got != string(want) {
				t.Fatalf("cancellation_policy=%s want=%s", got, want)
			}
		})
	}
}
