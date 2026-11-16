package spaceshttp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

type configurationHTTPRepo struct {
	memoryRepo
	observed time.Time
	writes   int
}

func (r *configurationHTTPRepo) GetCalendarConfiguration(_ context.Context, owner, id string) (spaces.CalendarConfiguration, error) {
	if owner != r.owner || id != draftID {
		return spaces.CalendarConfiguration{}, spaces.ErrNotFound
	}
	return spaces.CalendarConfiguration{SpaceID: id, TimeZone: "America/Santiago", UpdatedAt: r.draft.UpdatedAt, Rule: spaces.AvailabilityRule{SpaceID: id, DiasOperacion: []int{2, 4}, HoraApertura: "10:00", HoraCierre: "18:00", TarifaBaseDia: 35000}, Overrides: []spaces.CalendarOverrideInput{{Fecha: "2027-01-02", Estado: "bloqueado"}}}, nil
}
func (r *configurationHTTPRepo) UpdateAvailability(_ context.Context, owner, id string, in spaces.AvailabilityUpdateInput) (spaces.AvailabilityRule, string, error) {
	if owner != r.owner || id != draftID {
		return spaces.AvailabilityRule{}, "", spaces.ErrNotFound
	}
	r.observed = in.ExpectedUpdatedAt
	if !in.ExpectedUpdatedAt.Equal(r.draft.UpdatedAt) {
		return spaces.AvailabilityRule{}, "", spaces.ErrPreconditionFailed
	}
	r.writes++
	r.draft.UpdatedAt = r.draft.UpdatedAt.Add(time.Microsecond)
	return spaces.AvailabilityRule{SpaceID: id, TarifaBaseDia: 35000, ActualizadoEn: r.draft.UpdatedAt}, "America/Santiago", nil
}
func TestPrivateCalendarConfigurationAuthReadbackAndCAS(t *testing.T) {
	now := time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC)
	repo := &configurationHTTPRepo{memoryRepo: memoryRepo{owner: ownerA, draft: spaces.Draft{ID: draftID, State: "activa", UpdatedAt: now}}}
	svc, err := spaces.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(fakeAuth{}, svc, nil)
	path := "/api/v1/spaces/" + draftID + "/availability/configuration"
	for _, tc := range []struct {
		token, suffix string
		status        int
	}{{"", "", 401}, {"other", "", 404}, {"user", "?month=2026-11", 400}, {"user", "", 200}} {
		w := invoke(h, http.MethodGet, path+tc.suffix, tc.token, "")
		if w.Code != tc.status {
			t.Fatalf("GET %s %s got %d body=%s", tc.token, tc.suffix, w.Code, w.Body.String())
		}
		if tc.status == 200 {
			if w.Header().Get("ETag") != draftETag(repo.draft) || !strings.Contains(w.Body.String(), `"dias_operacion":[2,4]`) || strings.Contains(w.Body.String(), ownerA) {
				t.Fatalf("configuration revision/body incorrect: %s", w.Body.String())
			}
		}
	}
	body := `{"overrides":[{"fecha":"2027-01-02","estado":"bloqueado"}]}`
	if w := invoke(h, http.MethodPut, path, "user", body); w.Code != 428 {
		t.Fatalf("missing If-Match=%d %s", w.Code, w.Body.String())
	}
	stale := DraftETag(spaces.Draft{ID: draftID, UpdatedAt: now.Add(-time.Second)})
	if w := invokeIfMatch(h, path, "user", body, stale); w.Code != 412 || repo.writes != 0 {
		t.Fatalf("stale accepted: %d %s writes=%d", w.Code, w.Body.String(), repo.writes)
	}
	old := draftETag(repo.draft)
	w := invokeIfMatch(h, path, "user", body, old)
	if w.Code != 200 || repo.writes != 1 || !repo.observed.Equal(now) || w.Header().Get("ETag") == old {
		t.Fatalf("CAS forwarding failed: %d %s expected=%s writes=%d", w.Code, w.Body.String(), repo.observed, repo.writes)
	}
	read := invoke(h, http.MethodGet, path, "user", "")
	if read.Code != 200 || read.Header().Get("ETag") != w.Header().Get("ETag") {
		t.Fatalf("readback revision mismatch: %d %s", read.Code, read.Body.String())
	}
}
