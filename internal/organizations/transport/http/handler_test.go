package organizationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/organizations"
)

type stubAuth struct{}

func (stubAuth) Authorize(_ context.Context, token identity.Secret, _ identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if token == "host-token" {
		return identity.Principal{AccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}, nil
	}
	return identity.Principal{}, identity.ErrForbidden
}

type stubApp struct {
	updatedLegal   string
	updatedTrade   string
	updatedSummary string
}

func (*stubApp) List(context.Context, string) ([]organizations.Organization, error) { return nil, nil }
func (*stubApp) Create(context.Context, string, string, string, string) (organizations.Organization, error) {
	return organizations.Organization{}, nil
}
func (*stubApp) Get(context.Context, string, string) (organizations.Organization, error) {
	return organizations.Organization{}, nil
}
func (a *stubApp) Update(_ context.Context, _, id, legal, trade, summary string) (organizations.Organization, error) {
	a.updatedLegal = legal
	a.updatedTrade = trade
	a.updatedSummary = summary
	return organizations.Organization{
		ID:            id,
		LegalName:     legal,
		TradeName:     trade,
		PublicSummary: summary,
		CreatedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}
func (*stubApp) Members(context.Context, string, string) ([]organizations.Member, error) { return nil, nil }
func (*stubApp) Invite(context.Context, string, string, string, organizations.Role) (organizations.Invitation, string, error) {
	return organizations.Invitation{}, "", nil
}
func (*stubApp) Accept(context.Context, string, string, string, string) (organizations.Member, error) {
	return organizations.Member{}, nil
}
func (*stubApp) Revoke(context.Context, string, string, string) error                         { return nil }
func (*stubApp) SetRole(context.Context, string, string, string, organizations.Role) error     { return nil }
func (*stubApp) RequestJoin(context.Context, string, string, organizations.Role) (organizations.JoinRequest, error) {
	return organizations.JoinRequest{}, nil
}
func (*stubApp) OwnJoinRequest(context.Context, string, string) (organizations.JoinRequest, error) {
	return organizations.JoinRequest{}, nil
}
func (*stubApp) JoinRequests(context.Context, string, string) ([]organizations.JoinRequest, error) {
	return nil, nil
}
func (*stubApp) ApproveJoinRequest(context.Context, string, string, string) (organizations.Invitation, string, error) {
	return organizations.Invitation{}, "", nil
}
func (*stubApp) RejectJoinRequest(context.Context, string, string, string) error { return nil }

func TestPatchOrganizationForwardsPublicSummary(t *testing.T) {
	app := &stubApp{}
	h := NewHandler(stubAuth{}, app)
	orgID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/host/organizations/"+orgID, strings.NewReader(`{"legal_name":"Empresa SpA","trade_name":"Marca","public_summary":"Descripción comercial"}`))
	req.Header.Set("Authorization", "Bearer host-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if app.updatedLegal != "Empresa SpA" || app.updatedTrade != "Marca" || app.updatedSummary != "Descripción comercial" {
		t.Fatalf("unexpected forwarded update args: %+v", app)
	}
	var payload struct {
		Organization organizations.Organization `json:"organization"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Organization.PublicSummary != "Descripción comercial" {
		t.Fatalf("response organization missing public_summary: %+v", payload.Organization)
	}
}
