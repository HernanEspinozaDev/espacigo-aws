package earlyrentalhttp

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/booking"
	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
)

type demoRepository interface {
	CreateEarlyRental(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Time, string) (earlyrental.Result, error)
	GetEarlyRental(context.Context, uuid.UUID, uuid.UUID) (earlyrental.Result, error)
	ConfirmEarlyRental(context.Context, uuid.UUID, uuid.UUID, earlyrental.Command) (earlyrental.Result, error)
	PayEarlyRental(context.Context, uuid.UUID, uuid.UUID, uint64, booking.LocalPaymentAdapter) (earlyrental.Result, error)
}

func RegisterDemo(mux *http.ServeMux, auth Authenticator, repo demoRepository, processor booking.LocalPaymentAdapter, origins []string) {
	h := demoHandler{auth: auth, repo: repo, processor: processor, origins: origins}
	const base = "/api/v1/local/early-rentals"
	mux.HandleFunc("POST "+base, h.serve("create"))
	mux.HandleFunc("GET "+base+"/{offerID}", h.serve("detail"))
	mux.HandleFunc("POST "+base+"/{offerID}/identity-confirmations", h.serve(earlyrental.CommandConfirmIdentity))
	mux.HandleFunc("POST "+base+"/{offerID}/contract-acceptances", h.serve(earlyrental.CommandAcceptContract))
	mux.HandleFunc("POST "+base+"/{offerID}/pay", h.serve(earlyrental.CommandSendPayment))
	mux.HandleFunc("OPTIONS "+base, h.serve(""))
	mux.HandleFunc("OPTIONS "+base+"/{offerID}", h.serve(""))
	mux.HandleFunc("OPTIONS "+base+"/{offerID}/{command}", h.serve(""))
	registerDemoScreen(mux)
}

type demoHandler struct {
	auth      Authenticator
	repo      demoRepository
	processor booking.LocalPaymentAdapter
	origins   []string
}

func (h demoHandler) serve(command earlyrental.CommandKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Origin")
		if !h.allowOrigin(w, r) {
			capabilitiesFail(w, http.StatusForbidden, "origin_denied", requestID)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		principal, err := h.auth.Authorize(r.Context(), identity.Secret(bearerToken(r.Header.Get("Authorization"))), "", identity.UserOperation)
		if err != nil {
			demoError(w, err, requestID)
			return
		}
		actor, err := uuid.Parse(principal.AccountID)
		if err != nil || actor == uuid.Nil {
			demoError(w, identity.ErrUnauthorized, requestID)
			return
		}
		if r.URL.RawQuery != "" {
			demoError(w, earlyrental.ErrInvalid, requestID)
			return
		}
		result, err := h.dispatch(w, r, actor, command)
		if err != nil {
			demoError(w, err, requestID)
			return
		}
		view, err := projectEarlyRentalDetail(result.Offer, actor.String(), result.CreatedAt, result.UpdatedAt, time.Now())
		if err != nil {
			demoError(w, err, requestID)
			return
		}
		view.Data.AllowedActions = slices.DeleteFunc(view.Data.AllowedActions, func(action earlyrental.CommandKind) bool {
			return action != earlyrental.CommandAcceptContract && action != earlyrental.CommandConfirmIdentity && action != earlyrental.CommandSendPayment
		})
		etag, err := FormatEarlyRentalETag(result.Offer.ID, result.Offer.Revision)
		if err != nil {
			demoError(w, err, requestID)
			return
		}
		w.Header().Set("ETag", etag)
		if command == "create" {
			w.Header().Set("Location", "/api/v1/local/early-rentals/"+result.Offer.ID)
			writeCapabilities(w, http.StatusCreated, view)
			return
		}
		writeCapabilities(w, http.StatusOK, view)
	}
}

func (h demoHandler) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 || (!slices.Contains(h.origins, origins[0]) &&
		origins[0] != "http://"+r.Host && origins[0] != "https://"+r.Host) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origins[0])
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, Idempotency-Key")
	w.Header().Set("Access-Control-Expose-Headers", "ETag, Location, X-Request-ID")
	return true
}

func (h demoHandler) dispatch(w http.ResponseWriter, r *http.Request, actor uuid.UUID, kind earlyrental.CommandKind) (earlyrental.Result, error) {
	ctx := r.Context()
	var id uuid.UUID
	if kind != "create" {
		raw := r.PathValue("offerID")
		if !validEarlyRentalUUID(raw) {
			return earlyrental.Result{}, earlyrental.ErrInvalid
		}
		id = uuid.MustParse(raw)
	}
	if kind == "detail" {
		return h.repo.GetEarlyRental(ctx, actor, id)
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCommandBodyBytes))
	if err != nil {
		return earlyrental.Result{}, earlyrental.ErrInvalid
	}
	if kind == "create" {
		key, err := DecodeIdempotencyKey(r.Header)
		if err != nil {
			return earlyrental.Result{}, err
		}
		input, err := DecodeCreateOfferRequest(body)
		if err != nil {
			return earlyrental.Result{}, err
		}
		return h.repo.CreateEarlyRental(ctx, actor, input.SpaceID, input.StartAt, input.EndAt, key)
	}
	revision, err := ParseIfMatchRevision(r.Header.Values("If-Match"), id.String())
	if err != nil {
		return earlyrental.Result{}, err
	}
	input, err := DecodeCommandBody(kind, body)
	if err != nil {
		return earlyrental.Result{}, err
	}
	if kind == earlyrental.CommandSendPayment {
		return h.repo.PayEarlyRental(ctx, actor, id, revision, h.processor)
	}
	if _, err := DecodeIdempotencyKey(r.Header); err != nil {
		return earlyrental.Result{}, err
	}
	input.ExpectedRevision = revision
	return h.repo.ConfirmEarlyRental(ctx, actor, id, input)
}

func demoError(w http.ResponseWriter, err error, requestID string) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, identity.ErrUnauthorized):
		status, code = http.StatusUnauthorized, "unauthenticated"
	case errors.Is(err, identity.ErrForbidden), errors.Is(err, identity.ErrRestrictedMode), errors.Is(err, earlyrental.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, rentalflow.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, earlyrental.ErrPreconditionFailed), errors.Is(err, ErrStaleIfMatch):
		status, code = http.StatusPreconditionFailed, "stale_revision"
	case errors.Is(err, ErrMissingIfMatch):
		status, code = http.StatusPreconditionRequired, "revision_required"
	case errors.Is(err, earlyrental.ErrExpired):
		status, code = http.StatusConflict, "expired"
	case errors.Is(err, rentalflow.ErrConflict), errors.Is(err, rentalflow.ErrUnavailable),
		errors.Is(err, earlyrental.ErrWrongState), errors.Is(err, earlyrental.ErrContractMismatch):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, earlyrental.ErrInvalid), errors.Is(err, rentalflow.ErrInvalid),
		errors.Is(err, ErrMalformedCreateOfferBody), errors.Is(err, ErrInvalidCreateOfferValue), errors.Is(err, ErrMalformedIdempotencyKey),
		errors.Is(err, ErrMalformedCommandBody), errors.Is(err, ErrInvalidCommandValue), errors.Is(err, ErrInvalidIfMatch):
		status, code = http.StatusBadRequest, "invalid_request"
	default:
		if dbErr, ok := errors.AsType[interface {
			error
			SQLState() string
		}](err); ok {
			log.Printf("early rental request %s failed, SQLSTATE %s", requestID, dbErr.SQLState())
		} else {
			log.Printf("early rental request %s failed (%T)", requestID, err)
		}
	}
	capabilitiesFail(w, status, code, requestID)
}
