package commercialauthorizationhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialauthorization"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Authenticator interface {
	Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error)
}
type Sender interface {
	SendLocalBookingNotice(context.Context, string, string, string) error
}
type Handler struct {
	auth      Authenticator
	repo      commercialauthorization.Repository
	proposals interface {
		Get(context.Context, string, string) (commercialproposal.Record, error)
	}
	sender  Sender
	origins map[string]bool
	now     func() time.Time
}

func New(auth Authenticator, repo commercialauthorization.Repository, proposals interface {
	Get(context.Context, string, string) (commercialproposal.Record, error)
}, sender Sender, origins []string, now func() time.Time) http.Handler {
	h := &Handler{auth: auth, repo: repo, proposals: proposals, sender: sender, origins: map[string]bool{}, now: now}
	if h.now == nil {
		h.now = time.Now
	}
	for _, origin := range origins {
		h.origins[origin] = true
	}
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Add("Vary", "Origin")
	reqID, err := (credentials.Generator{}).ID()
	if err != nil {
		fail(w, 500, "internal_error")
		return
	}
	w.Header().Set("X-Request-ID", reqID)
	if origin := r.Header.Get("Origin"); origin != "" {
		if !h.origins[origin] {
			fail(w, 403, "origin_denied")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "spaces" || parts[4] != "commercial-authorization" {
		fail(w, 404, "not_found")
		return
	}
	action := parts[5]
	allowed := "POST, OPTIONS"
	if action == "current" || action == "history" {
		allowed = "GET, OPTIONS"
	}
	switch action {
	case "current", "history", "activate", "revoke", "evaluate", "otp-challenge":
	default:
		fail(w, 404, "not_found")
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", allowed)
		w.WriteHeader(204)
		return
	}
	if !strings.Contains(", "+allowed+",", ", "+r.Method+",") {
		w.Header().Set("Allow", allowed)
		fail(w, 405, "method_not_allowed")
		return
	}
	p, err := h.auth.Authorize(r.Context(), identity.Secret(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), identity.RoleLandlord, identity.UserOperation)
	if err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			fail(w, 403, "forbidden")
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, 401, "unauthenticated")
		}
		return
	}
	space := parts[3]
	if !commercialauthorization.ValidIDs(p.AccountID, space) || r.URL.RawQuery != "" {
		fail(w, 422, "invalid_request")
		return
	}
	now := h.now().UTC()
	switch action {
	case "current":
		g, e := h.repo.Current(r.Context(), p.AccountID, space, now)
		if e != nil {
			replyErr(w, e)
			return
		}
		reply(w, map[string]any{"authorization": g, "human_signature_recorded": false, "commercial_activation_enabled": true})
	case "history":
		events, e := h.repo.History(r.Context(), p.AccountID, space)
		if e != nil {
			replyErr(w, e)
			return
		}
		reply(w, map[string]any{"items": events, "human_signature_recorded": false})
	case "otp-challenge":
		var b struct {
			TemplateVersion int64 `json:"template_version"`
		}
		if !decode(w, r, &b) {
			return
		}
		if b.TemplateVersion < 1 {
			fail(w, 422, "invalid_request")
			return
		}
		if h.sender == nil {
			fail(w, 500, "internal_error")
			return
		}
		res, email, code, e := h.repo.CreateOTPChallenge(r.Context(), p.AccountID, space, commercialauthorization.OTPChallengeInput{TemplateVersion: b.TemplateVersion}, now)
		if e != nil {
			replyErr(w, e)
			return
		}
		subject := "Código para activar firma automática en EspaciGo"
		body := "Tu código de verificación para activar la firma automática del contrato es: " + code + ". Expira en 5 minutos. No lo compartas con nadie."
		if sendErr := h.sender.SendLocalBookingNotice(r.Context(), email, subject, body); sendErr != nil {
			fail(w, 500, "internal_error")
			return
		}
		reply(w, res)
	case "activate":
		var b struct {
			TemplateVersion int64     `json:"template_version"`
			Consent         bool      `json:"consent"`
			OTPChallengeID  string    `json:"otp_challenge_id"`
			OTPCode         string    `json:"otp_code"`
			MaxAmountCLP    int64     `json:"max_amount_clp"`
			ValidFrom       time.Time `json:"valid_from"`
			ExpiresAt       time.Time `json:"expires_at"`
		}
		if !decode(w, r, &b) {
			return
		}
		if !b.Consent {
			fail(w, 422, "invalid_request")
			return
		}
		if b.MaxAmountCLP <= 0 {
			b.MaxAmountCLP = commercialauthorization.MaxAmountCLP
		}
		if b.ValidFrom.IsZero() && b.ExpiresAt.IsZero() {
			b.ValidFrom = now
			b.ExpiresAt = now.Add(commercialauthorization.MaxValidity)
		}
		act := commercialauthorization.Activation{
			TemplateVersion: b.TemplateVersion,
			MaxAmountCLP:    b.MaxAmountCLP,
			ValidFrom:       b.ValidFrom,
			ExpiresAt:       b.ExpiresAt,
			Consent:         b.Consent,
			OTPChallengeID:  b.OTPChallengeID,
			OTPCode:         b.OTPCode,
		}
		if !commercialauthorization.ValidateActivation(act, now) {
			fail(w, 422, "invalid_request")
			return
		}
		if !commercialauthorization.ValidIDs(b.OTPChallengeID) || !commercialauthorization.ValidOTPCode(b.OTPCode) {
			replyErr(w, commercialauthorization.ErrOTPRequired)
			return
		}
		g, e := h.repo.Activate(r.Context(), p.AccountID, space, act, now)
		if e != nil {
			replyErr(w, e)
			return
		}
		reply(w, map[string]any{"authorization": g, "human_signature_recorded": false, "commercial_activation_enabled": true, "notice": "Firma automática acotada activa: las nuevas solicitudes que cumplan estos límites y usen la plantilla publicada se confirmarán automáticamente y pasarán directo al pago."})
	case "revoke":
		var b struct {
			AuthorizationID string `json:"authorization_id"`
			Reason          string `json:"reason"`
		}
		if !decode(w, r, &b) {
			return
		}
		if !commercialauthorization.ValidIDs(b.AuthorizationID) {
			fail(w, 422, "invalid_request")
			return
		}
		g, e := h.repo.Revoke(r.Context(), p.AccountID, space, b.AuthorizationID, b.Reason, now)
		if e != nil {
			replyErr(w, e)
			return
		}
		reply(w, map[string]any{"authorization": g, "human_signature_recorded": false, "commercial_activation_enabled": true})
	case "evaluate":
		var b struct {
			ProposalID string `json:"proposal_id"`
		}
		if !decode(w, r, &b) {
			return
		}
		if !commercialauthorization.ValidIDs(b.ProposalID) {
			fail(w, 422, "invalid_request")
			return
		}
		g, e := h.repo.Current(r.Context(), p.AccountID, space, now)
		if e != nil {
			replyErr(w, e)
			return
		}
		proposal, e := h.proposals.Get(r.Context(), p.AccountID, b.ProposalID)
		if e != nil || proposal.OwnerID != p.AccountID || proposal.Terms.SpaceID != space {
			fail(w, 404, "not_found")
			return
		}
		decision := commercialauthorization.Evaluate(g, proposal.Terms, now)
		reply(w, map[string]any{
			"decision":                      decision,
			"manual_review_required":        !decision.Eligible,
			"human_signature_recorded":      decision.Eligible,
			"commercial_activation_enabled": true,
		})
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || media != "application/json" {
		fail(w, 415, "unsupported_media_type")
		return false
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 8193))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, 422, "invalid_request")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 422, "invalid_request")
		return false
	}
	return true
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func fail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No fue posible completar la operación de autorización.", "request_id": w.Header().Get("X-Request-ID")}})
}
func replyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, commercialauthorization.ErrInvalid):
		fail(w, 422, "invalid_request")
	case errors.Is(err, commercialauthorization.ErrOTPRequired):
		fail(w, 422, "otp_required")
	case errors.Is(err, commercialauthorization.ErrOTPInvalid):
		fail(w, 409, "otp_invalid")
	case errors.Is(err, commercialauthorization.ErrOTPExpired):
		fail(w, 409, "otp_expired")
	case errors.Is(err, commercialauthorization.ErrNotFound):
		fail(w, 404, "not_found")
	case errors.Is(err, commercialauthorization.ErrConflict):
		fail(w, 409, "conflict")
	default:
		fail(w, 500, "internal_error")
	}
}
