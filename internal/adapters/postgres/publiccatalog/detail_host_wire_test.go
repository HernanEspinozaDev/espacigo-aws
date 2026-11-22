package publiccatalogpg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog"
	publiccataloghttp "github.com/HernanEspinozaDev/espaciGo/internal/publiccatalog/transport/http"
)

func TestDetailHostWireUsesCurrentIdentityAndNullablePublicName(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, verified := range []bool{false, true} {
		for _, name := range []any{nil, "Nombre público"} {
			row := detailVerificationRow(id)
			row[17] = verified
			row[20] = name
			pool := &detailVerificationPool{detailRow: row, attributes: []any{"wifi", "Wi-Fi", "boolean", 1, "", []byte(`false`), []byte(`[]`)}}
			service, err := publiccatalog.New(&Repository{pool: pool}, []byte("01234567890123456789012345678901"))
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			publiccataloghttp.New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+id, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d", response.Code)
			}
			var envelope struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			var host map[string]json.RawMessage
			if err = json.Unmarshal(envelope.Data["host"], &host); err != nil {
				t.Fatal(err)
			}
			if len(host) != 5 {
				t.Fatalf("host keys=%v", host)
			}
			for _, key := range []string{"name", "avatar_url", "joined_at", "spaces_count", "identity_verified"} {
				if _, ok := host[key]; !ok {
					t.Fatalf("missing host key %q", key)
				}
			}
			expected := name
			if name != nil {
				expected = "Nombre"
			}
			expectedName, _ := json.Marshal(expected)
			if string(host["name"]) != string(expectedName) {
				t.Fatalf("name=%s want=%s", host["name"], expectedName)
			}
			expectedVerified, _ := json.Marshal(verified)
			if string(host["identity_verified"]) != string(expectedVerified) {
				t.Fatalf("identity=%s", host["identity_verified"])
			}
			if string(host["avatar_url"]) != `"/api/v1/public/spaces/`+id+`/host/avatar"` {
				t.Fatalf("avatar_url=%s", host["avatar_url"])
			}
			if string(host["joined_at"]) != `"2020-01-02T03:04:05Z"` || string(host["spaces_count"]) != "3" {
				t.Fatalf("host metadata=%v", host)
			}
			for _, key := range []string{"cancellation_policy", "guarantee"} {
				if string(envelope.Data[key]) != "null" {
					t.Fatalf("%s=%s", key, envelope.Data[key])
				}
			}
			if string(envelope.Data["capacity_max"]) != "8" {
				t.Fatalf("capacity=%s", envelope.Data["capacity_max"])
			}
		}
	}
}

func TestDetailCancellationPolicyUsesOnlyKnownPublishedVersion(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, version string
		want          string
	}{
		{name: "known", version: "flexible_v1", want: "Flexible: reembolso total hasta 24 horas antes del inicio del arriendo."},
		{name: "unknown", version: "future_policy", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := detailVerificationRow(id)
			row[26] = tc.version
			pool := &detailVerificationPool{detailRow: row, attributes: []any{"capacity", "Capacity", "integer", 1, "people", []byte(`8`), []byte(`[]`)}}
			service, err := publiccatalog.New(&Repository{pool: pool}, []byte("01234567890123456789012345678901"))
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			publiccataloghttp.New(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/spaces/"+id, nil))
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
			if string(envelope.Data["guarantee"]) != "null" {
				t.Fatalf("guarantee=%s want=null", envelope.Data["guarantee"])
			}
		})
	}
}
