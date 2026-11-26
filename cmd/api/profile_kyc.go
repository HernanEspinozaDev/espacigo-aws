package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilekyc"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilekyc/didit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileDiditProvider struct {
	client             *didit.Client
	workflow, callback string
}

func (p profileDiditProvider) Start(ctx context.Context, attempt string, data profilekyc.Data, returnTo string) (profilekyc.Session, error) {
	callback := p.callback
	if returnTo != "" {
		callback += "?" + url.Values{"return_to": []string{returnTo}}.Encode()
	}
	s, err := p.client.CreateSession(ctx, didit.CreateSessionRequest{
		WorkflowID: p.workflow, VendorData: attempt, Callback: callback, Language: "es",
		ExpectedDetails: didit.ExpectedDetails{
			FirstName: data.FirstName, LastName: data.LastName(), DateOfBirth: data.DateOfBirth, IdentificationNumber: data.RUT,
			ExpectedDocumentTypes: []string{"ID"},
		},
	})
	if err != nil || s.WorkflowID != p.workflow || s.VendorData != attempt {
		return profilekyc.Session{}, profilekyc.ErrUnavailable
	}
	return profilekyc.Session{ID: s.SessionID, URL: s.SessionURL}, nil
}

func (p profileDiditProvider) Decision(ctx context.Context, session string) (profilekyc.Decision, error) {
	d, err := p.client.GetDecision(ctx, session)
	if err != nil || d.SessionID != session || d.WorkflowID != p.workflow {
		return profilekyc.Decision{}, profilekyc.ErrUnavailable
	}
	out := profilekyc.Decision{SessionID: session, AttemptID: d.VendorData}
	switch strings.ToLower(strings.TrimSpace(d.Status)) {
	case "approved":
		out.State = "verified"
	case "declined", "rejected", "resubmission requested", "resubmission_required", "awaiting user":
		out.State = "needs_correction"
	case "in review", "in_review":
		out.State = "in_review"
	case "expired", "abandoned":
		out.State = "expired"
	case "revoked":
		out.State = "revoked"
	case "not started", "in progress", "resubmitted", "in_progress":
		out.State = "in_progress"
	}
	if out.State == "verified" && len(d.IDVerifications) == 1 {
		doc := d.IDVerifications[0]
		out.FirstName, out.LastName = doc.FirstName, doc.LastName
		out.RUT, out.DateOfBirth, out.DocumentExpires = doc.PersonalNumber, doc.DateOfBirth, doc.ExpirationDate
		if out.RUT == "" {
			out.RUT = doc.IdentificationNumber
		}
		out.ChecksPassed = approvedChileanIdentityCard(doc) && d.LivenessPassed() && d.FaceMatchPassed()
	}
	return out, nil
}

func approvedChileanIdentityCard(doc didit.IDVerification) bool {
	return strings.EqualFold(doc.Status, "approved") && strings.EqualFold(doc.IssuingState, "CHL") &&
		(strings.EqualFold(doc.DocumentType, "Identity Card") || strings.EqualFold(doc.DocumentType, "ID"))
}

func mountIdentityProfile(mux *http.ServeMux, pool *pgxpool.Pool, auth *identity.AuthenticationService, mailer identity.AuthenticationMailer, cfg config, getenv func(string) string, production bool) (*profilekyc.Service, profilekyc.Keyring, error) {
	key, err := loadRentalContractDeclarationKeyring(getenv)
	if err != nil {
		return nil, nil, err
	}
	var keys profilekyc.Keyring
	if key != nil {
		keys = key
	}
	service := profilekyc.New(pool, keys, nil, time.Now)
	apiKey := strings.TrimSpace(getenv("PROFILE_KYC_DIDIT_API_KEY"))
	var provider profilekyc.Provider
	var decoder profilekyc.DecodeNotice
	if apiKey != "" {
		if keys == nil {
			return nil, nil, errors.New("identity profile encryption is unavailable")
		}
		rutKeys, err := loadRuntimeRutHMACKeyring(getenv)
		if err != nil {
			return nil, nil, err
		}
		service = profilekyc.New(pool, keys, rutKeys, time.Now)
		workflow := strings.TrimSpace(getenv("PROFILE_KYC_DIDIT_WORKFLOW_ID"))
		application := strings.TrimSpace(getenv("PROFILE_KYC_DIDIT_APPLICATION_ID"))
		environment := strings.TrimSpace(getenv("PROFILE_KYC_DIDIT_ENVIRONMENT"))
		secret := getenv("PROFILE_KYC_DIDIT_WEBHOOK_SECRET")
		if _, err := uuid.Parse(workflow); err != nil {
			return nil, nil, errors.New("identity profile Didit workflow is invalid")
		}
		if _, err := uuid.Parse(application); err != nil {
			return nil, nil, errors.New("identity profile Didit application is invalid")
		}
		if secret == "" || production && environment != "live" || !production && environment != "sandbox" {
			return nil, nil, errors.New("identity profile Didit environment or webhook configuration is invalid")
		}
		client, err := didit.NewClient(didit.Config{APIKey: apiKey})
		if err != nil {
			return nil, nil, err
		}
		provider = profileDiditProvider{client: client, workflow: workflow, callback: strings.TrimRight(cfg.frontendPublicOrigin, "/") + "/verificaciones/retorno"}
		secrets := didit.WebhookSecrets{Current: secret, Previous: getenv("PROFILE_KYC_DIDIT_PREVIOUS_WEBHOOK_SECRET")}
		decoder = func(h http.Header, body []byte) (profilekyc.Notice, error) {
			event, err := didit.VerifyWebhook(h, body, secrets, time.Now())
			if err != nil || event.DecisionMustBeFetched || event.ApplicationID != application || event.Environment != environment ||
				event.WorkflowID != workflow || event.WebhookType != "status.updated" {
				return profilekyc.Notice{}, didit.ErrInvalidWebhook
			}
			timestamp, err := strconv.ParseInt(string(event.Timestamp), 10, 64)
			if err != nil {
				return profilekyc.Notice{}, didit.ErrInvalidWebhook
			}
			return profilekyc.Notice{ID: event.EventID, SessionID: event.SessionID, AttemptID: event.VendorData, OccurredAt: time.Unix(timestamp, 0)}, nil
		}
	}
	keyring, err := identity.LoadTOTPKeyring(getenv)
	if err != nil {
		return nil, nil, err
	}
	stepUp := pinpassStepUp{pool: pool, totp: keyring}
	pinpass, err := mountPinpass(mux, pool, auth, service, stepUp, mailer, getenv)
	if err != nil {
		return nil, nil, err
	}
	if pinpass != nil {
		auth.SetPinpassStatusReader(pinpass.Configured)
	}
	profilekyc.Register(mux, auth, profilekyc.NewVerifier(service, provider), decoder, stepUp, pinpass)
	return service, keys, nil
}
