package commercialauthorizationhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialauthorization"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

const (
	testOwnerID     = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testSpaceID     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testGrantID     = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	testProposalID  = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	testSessionID   = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	testChallengeID = "ffffffff-ffff-4fff-8fff-ffffffffffff"
)

type stubAuth struct{}

func (stubAuth) Authorize(_ context.Context, token identity.Secret, role identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	if token != "owner" {
		return identity.Principal{}, identity.ErrUnauthorized
	}
	if role != identity.RoleLandlord {
		return identity.Principal{}, identity.ErrForbidden
	}
	return identity.Principal{AccountID: testOwnerID, SessionID: testSessionID}, nil
}

type stubRepo struct {
	grant          commercialauthorization.Grant
	events         []commercialauthorization.Event
	challenge      commercialauthorization.OTPChallengeResult
	challengeEmail string
	challengeCode  string
	lastActivation commercialauthorization.Activation
	err            error
}

func (r *stubRepo) Current(context.Context, string, string, time.Time) (commercialauthorization.Grant, error) {
	return r.grant, r.err
}
func (r *stubRepo) History(context.Context, string, string) ([]commercialauthorization.Event, error) {
	return r.events, r.err
}
func (r *stubRepo) CreateOTPChallenge(context.Context, string, string, commercialauthorization.OTPChallengeInput, time.Time) (commercialauthorization.OTPChallengeResult, string, string, error) {
	return r.challenge, r.challengeEmail, r.challengeCode, r.err
}
func (r *stubRepo) Activate(_ context.Context, _ string, _ string, a commercialauthorization.Activation, _ time.Time) (commercialauthorization.Grant, error) {
	r.lastActivation = a
	return r.grant, r.err
}
func (r *stubRepo) Revoke(context.Context, string, string, string, string, time.Time) (commercialauthorization.Grant, error) {
	return r.grant, r.err
}

type stubSender struct {
	email   string
	subject string
	body    string
	err     error
}

func (s *stubSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	s.email = email
	s.subject = subject
	s.body = body
	return s.err
}

type stubProposals struct {
	record commercialproposal.Record
	err    error
}

func (p *stubProposals) Get(context.Context, string, string) (commercialproposal.Record, error) {
	return p.record, p.err
}

func invokeAuth(h http.Handler, method, action, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/spaces/"+testSpaceID+"/commercial-authorization/"+action, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer owner")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCommercialAuthorizationHandlerPhase3ActivationAndEvaluate(t *testing.T) {
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	grant := commercialauthorization.Grant{
		ID:              testGrantID,
		SpaceID:         testSpaceID,
		OwnerID:         testOwnerID,
		TemplateVersion: 3,
		MaxAmountCLP:    50000,
		ValidFrom:       now.Add(-time.Hour),
		ExpiresAt:       now.Add(24 * time.Hour),
		CreatedAt:       now.Add(-time.Hour),
	}
	proposals := &stubProposals{
		record: commercialproposal.Record{
			ID:      testProposalID,
			OwnerID: testOwnerID,
			Terms: commercialproposal.Terms{
				SpaceID:         testSpaceID,
				TemplateVersion: 3,
				Currency:        "CLP",
				SubtotalCLP:     45000,
			},
		},
	}
	repo := &stubRepo{
		grant: grant,
		events: []commercialauthorization.Event{
			{GrantID: testGrantID, SpaceID: testSpaceID, ActorID: testOwnerID, Action: "activated", Reason: "owner_opt_in", OccurredAt: now},
		},
		challenge: commercialauthorization.OTPChallengeResult{
			ChallengeID: testChallengeID,
			Channel:     "email_otp",
			ExpiresAt:   now.Add(5 * time.Minute),
			MaskedEmail: "an***@ejemplo.cl",
		},
		challengeEmail: "anfitrion@ejemplo.cl",
		challengeCode:  "654321",
	}
	sender := &stubSender{}
	h := New(stubAuth{}, repo, proposals, sender, nil, func() time.Time { return now })

	t.Run("current and activate expose commercial_activation_enabled=true", func(t *testing.T) {
		for _, tc := range []struct {
			method string
			action string
			body   string
		}{
			{method: http.MethodGet, action: "current"},
			{method: http.MethodPost, action: "activate", body: `{"template_version":3,"consent":true,"otp_challenge_id":"` + testChallengeID + `","otp_code":"654321","max_amount_clp":50000,"valid_from":"2030-01-02T12:00:00Z","expires_at":"2030-01-03T12:00:00Z"}`},
			{method: http.MethodPost, action: "revoke", body: `{"authorization_id":"` + testGrantID + `","reason":"owner_request"}`},
		} {
			rec := invokeAuth(h, tc.method, tc.action, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status=%d body=%s", tc.action, rec.Code, rec.Body.String())
			}
			var payload struct {
				Data struct {
					CommercialActivationEnabled bool   `json:"commercial_activation_enabled"`
					HumanSignatureRecorded      bool   `json:"human_signature_recorded"`
					Notice                      string `json:"notice"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if !payload.Data.CommercialActivationEnabled || payload.Data.HumanSignatureRecorded {
				t.Fatalf("%s unexpected flags: %+v", tc.action, payload.Data)
			}
			if tc.action == "activate" && !strings.Contains(payload.Data.Notice, "Firma automática acotada activa") {
				t.Fatalf("unexpected activate notice: %q", payload.Data.Notice)
			}
		}
	})

	t.Run("evaluate eligible mandate sets manual_review_required=false and human_signature_recorded=true", func(t *testing.T) {
		rec := invokeAuth(h, http.MethodPost, "evaluate", `{"proposal_id":"`+testProposalID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Data struct {
				Decision                    commercialauthorization.Decision `json:"decision"`
				ManualReviewRequired        bool                             `json:"manual_review_required"`
				HumanSignatureRecorded      bool                             `json:"human_signature_recorded"`
				CommercialActivationEnabled bool                             `json:"commercial_activation_enabled"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.Data.Decision.Eligible || payload.Data.Decision.Reason != "within_authorized_limits" {
			t.Fatalf("unexpected decision: %+v", payload.Data.Decision)
		}
		if payload.Data.ManualReviewRequired || !payload.Data.HumanSignatureRecorded || !payload.Data.CommercialActivationEnabled {
			t.Fatalf("unexpected evaluation flags: %+v", payload.Data)
		}
	})

	t.Run("evaluate over cap sets manual_review_required=true and human_signature_recorded=false", func(t *testing.T) {
		proposals.record.Terms.SubtotalCLP = 50001
		rec := invokeAuth(h, http.MethodPost, "evaluate", `{"proposal_id":"`+testProposalID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Data struct {
				Decision                    commercialauthorization.Decision `json:"decision"`
				ManualReviewRequired        bool                             `json:"manual_review_required"`
				HumanSignatureRecorded      bool                             `json:"human_signature_recorded"`
				CommercialActivationEnabled bool                             `json:"commercial_activation_enabled"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.Decision.Eligible || payload.Data.Decision.Reason != "amount_out_of_bounds" {
			t.Fatalf("unexpected decision: %+v", payload.Data.Decision)
		}
		if !payload.Data.ManualReviewRequired || payload.Data.HumanSignatureRecorded || !payload.Data.CommercialActivationEnabled {
			t.Fatalf("unexpected evaluation flags: %+v", payload.Data)
		}
	})

	t.Run("otp-challenge sends email code and returns masked challenge", func(t *testing.T) {
		rec := invokeAuth(h, http.MethodPost, "otp-challenge", `{"template_version":3}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Data commercialauthorization.OTPChallengeResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.ChallengeID != testChallengeID || payload.Data.Channel != "email_otp" || payload.Data.MaskedEmail != "an***@ejemplo.cl" {
			t.Fatalf("unexpected otp challenge payload: %+v", payload.Data)
		}
		if sender.email != "anfitrion@ejemplo.cl" || !strings.Contains(sender.body, "654321") {
			t.Fatalf("unexpected sent notice: email=%q body=%q", sender.email, sender.body)
		}
	})

	t.Run("simplified activate defaults to 3 months and unlimited amount", func(t *testing.T) {
		rec := invokeAuth(h, http.MethodPost, "activate", `{"template_version":3,"consent":true,"otp_challenge_id":"`+testChallengeID+`","otp_code":"654321"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if repo.lastActivation.MaxAmountCLP != commercialauthorization.MaxAmountCLP {
			t.Fatalf("max_amount_clp=%d want %d", repo.lastActivation.MaxAmountCLP, commercialauthorization.MaxAmountCLP)
		}
		if !repo.lastActivation.ValidFrom.Equal(now) || !repo.lastActivation.ExpiresAt.Equal(now.Add(commercialauthorization.MaxValidity)) {
			t.Fatalf("unexpected default window: valid_from=%v expires_at=%v", repo.lastActivation.ValidFrom, repo.lastActivation.ExpiresAt)
		}
	})

	t.Run("activate validates consent and otp errors", func(t *testing.T) {
		cases := []struct {
			name       string
			body       string
			repoErr    error
			wantStatus int
			wantCode   string
		}{
			{
				name:       "missing consent",
				body:       `{"template_version":3,"consent":false,"otp_challenge_id":"` + testChallengeID + `","otp_code":"654321"}`,
				wantStatus: http.StatusUnprocessableEntity,
				wantCode:   "invalid_request",
			},
			{
				name:       "missing otp",
				body:       `{"template_version":3,"consent":true}`,
				wantStatus: http.StatusUnprocessableEntity,
				wantCode:   "otp_required",
			},
			{
				name:       "invalid otp",
				body:       `{"template_version":3,"consent":true,"otp_challenge_id":"` + testChallengeID + `","otp_code":"000000"}`,
				repoErr:    commercialauthorization.ErrOTPInvalid,
				wantStatus: http.StatusConflict,
				wantCode:   "otp_invalid",
			},
			{
				name:       "expired otp",
				body:       `{"template_version":3,"consent":true,"otp_challenge_id":"` + testChallengeID + `","otp_code":"654321"}`,
				repoErr:    commercialauthorization.ErrOTPExpired,
				wantStatus: http.StatusConflict,
				wantCode:   "otp_expired",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo.err = tc.repoErr
				defer func() { repo.err = nil }()
				rec := invokeAuth(h, http.MethodPost, "activate", tc.body)
				if rec.Code != tc.wantStatus {
					t.Fatalf("status=%d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
				}
				var errPayload struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &errPayload); err != nil {
					t.Fatal(err)
				}
				if errPayload.Error.Code != tc.wantCode {
					t.Fatalf("code=%q want %q", errPayload.Error.Code, tc.wantCode)
				}
			})
		}
	})
}
