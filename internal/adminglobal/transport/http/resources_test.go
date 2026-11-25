package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type fakeAccountResources struct {
	listPage adminglobal.AccountResourcePage
	listErr  error
	detail   map[string]any
	getErr   error
	lastKind string
	lastID   string
	lastPage int
	lastCur  string
}

func (f *fakeAccountResources) ListAccountResources(_ context.Context, _, _, _, kind, encodedCursor string, pageSize int) (adminglobal.AccountResourcePage, error) {
	f.lastKind = kind
	f.lastCur = encodedCursor
	f.lastPage = pageSize
	if f.listErr != nil {
		return adminglobal.AccountResourcePage{}, f.listErr
	}
	return f.listPage, nil
}

func (f *fakeAccountResources) GetAccountResource(_ context.Context, _, _, resourceID, _, kind string) (map[string]any, error) {
	f.lastKind = kind
	f.lastID = resourceID
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}

func TestAccountResourcesCommercialKindsAndInvalidKind(t *testing.T) {
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	accountID := "00000000-0000-4000-8000-000000000001"
	actorID := "00000000-0000-4000-8000-000000000010"
	resourceID := "00000000-0000-4000-8000-000000000020"
	spaceID := "00000000-0000-4000-8000-000000000030"

	auth := accountEmailAuth{identity.Principal{
		AccountID:       actorID,
		Capabilities:    []identity.Capability{identity.CapabilityAccountsRead},
		AuthenticatedAt: now,
	}}

	boolTrue := true
	amount := int64(50000)
	totalCharged := int64(51785)

	cases := []struct {
		name string
		kind string
		item adminglobal.AccountResource
	}{
		{
			name: "commercial_reservations",
			kind: "commercial_reservations",
			item: adminglobal.AccountResource{
				ID:              resourceID,
				Kind:            "commercial_reservation",
				DataOrigin:      "commercial",
				TestMode:        &boolTrue,
				RoleInRecord:    "arrendatario",
				AmountCLP:       &amount,
				TotalChargedCLP: &totalCharged,
				Currency:        "CLP",
				SpaceID:         spaceID,
				SpaceTitle:      "Bodega Central",
				ReservationID:   resourceID,
				State:           "pagada",
				Participant:     "arrendatario",
				RelatedIDs: map[string]string{
					"checkout_operation_id": "00000000-0000-4000-8000-000000000040",
					"contract_request_id":   "00000000-0000-4000-8000-000000000050",
				},
				OccurredAt: now,
			},
		},
		{
			name: "commercial_contracts",
			kind: "commercial_contracts",
			item: adminglobal.AccountResource{
				ID:             resourceID,
				Kind:           "commercial_contract",
				DataOrigin:     "commercial",
				RoleInRecord:   "arrendador",
				AmountCLP:      &amount,
				Currency:       "CLP",
				SpaceID:        spaceID,
				SpaceTitle:     "Bodega Central",
				State:          "ready_for_payment",
				Participant:    "arrendador",
				DocumentStatus: "ready",
				RelatedIDs: map[string]string{
					"proposal_id":  "00000000-0000-4000-8000-000000000060",
					"quote_id":     "00000000-0000-4000-8000-000000000070",
					"signing_mode": "fes_interna",
				},
				OccurredAt: now,
			},
		},
		{
			name: "commercial_payments",
			kind: "commercial_payments",
			item: adminglobal.AccountResource{
				ID:                resourceID,
				Kind:              "commercial_payment",
				DataOrigin:        "commercial",
				TestMode:          &boolTrue,
				RoleInRecord:      "anfitrion",
				AmountCLP:         &amount,
				TotalChargedCLP:   &totalCharged,
				Currency:          "CLP",
				SpaceID:           spaceID,
				SpaceTitle:        "Bodega Central",
				State:             "approved",
				Participant:       "anfitrion",
				ProviderPaymentID: "123456789",
				ProviderStatus:    "approved",
				RelatedIDs: map[string]string{
					"quote_id":            "00000000-0000-4000-8000-000000000070",
					"checkout_method":     "checkout_pro",
					"contract_request_id": "00000000-0000-4000-8000-000000000050",
				},
				OccurredAt: now,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_list_and_detail", func(t *testing.T) {
			fake := &fakeAccountResources{
				listPage: adminglobal.AccountResourcePage{
					Items:         []adminglobal.AccountResource{tc.item},
					MatchingTotal: 1,
					GeneratedAt:   now,
				},
				detail: map[string]any{
					"id":          resourceID,
					"kind":        tc.item.Kind,
					"data_origin": "commercial",
					"state":       tc.item.State,
				},
			}
			h := &Handler{auth: auth, resources: fake}

			listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/accounts/"+accountID+"/resources/"+tc.kind+"?page_size=10&cursor=abc", nil)
			listReq.Header.Set("Authorization", "Bearer test-token")
			listRec := httptest.NewRecorder()
			h.ServeHTTP(listRec, listReq)

			if listRec.Code != http.StatusOK {
				t.Fatalf("list %s: got status %d body=%s", tc.kind, listRec.Code, listRec.Body.String())
			}
			if fake.lastKind != tc.kind || fake.lastPage != 10 || fake.lastCur != "abc" {
				t.Fatalf("unexpected service call args: kind=%q page=%d cur=%q", fake.lastKind, fake.lastPage, fake.lastCur)
			}

			var listResp struct {
				Data adminglobal.AccountResourcePage `json:"data"`
			}
			if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
				t.Fatalf("unmarshal list response: %v", err)
			}
			if len(listResp.Data.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(listResp.Data.Items))
			}
			got := listResp.Data.Items[0]
			if got.DataOrigin != "commercial" || got.SpaceTitle != "Bodega Central" || got.RoleInRecord != tc.item.RoleInRecord {
				t.Fatalf("unexpected item fields: %+v", got)
			}

			detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/accounts/"+accountID+"/resources/"+tc.kind+"/"+resourceID, nil)
			detailReq.Header.Set("Authorization", "Bearer test-token")
			detailRec := httptest.NewRecorder()
			h.ServeHTTP(detailRec, detailReq)

			if detailRec.Code != http.StatusOK {
				t.Fatalf("detail %s: got status %d body=%s", tc.kind, detailRec.Code, detailRec.Body.String())
			}
			if fake.lastKind != tc.kind || fake.lastID != resourceID {
				t.Fatalf("unexpected detail args: kind=%q id=%q", fake.lastKind, fake.lastID)
			}
		})
	}

	t.Run("invalid_kind_returns_404", func(t *testing.T) {
		h := &Handler{auth: auth, resources: &fakeAccountResources{}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/accounts/"+accountID+"/resources/unknown_kind", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for invalid kind, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not_found_from_service_returns_404", func(t *testing.T) {
		h := &Handler{auth: auth, resources: &fakeAccountResources{getErr: adminglobal.ErrNotFound}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/global/accounts/"+accountID+"/resources/commercial_contracts/"+resourceID, nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for ErrNotFound, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}
