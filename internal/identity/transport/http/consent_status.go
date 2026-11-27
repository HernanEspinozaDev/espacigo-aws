package identityhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type legalAcceptanceReader interface {
	LegalAcceptances(context.Context, string) ([]privacy.TermsAcceptance, error)
}

type legalAcceptanceWriter interface {
	RecordLegalAcceptance(context.Context, identity.TermsAcceptance) error
}

type legalAcceptanceStatus struct {
	Type               string     `json:"type"`
	CurrentCode        string     `json:"current_code"`
	LatestAcceptedCode *string    `json:"latest_accepted_code"`
	LatestAcceptedAt   *time.Time `json:"latest_accepted_at"`
	CurrentAccepted    bool       `json:"current_accepted"`
}

func (h *Handler) writeLegalAcceptanceStatus(w http.ResponseWriter, r *http.Request, accountID string) {
	repo, ok := h.terms.(legalAcceptanceReader)
	documents, err := identity.PublicConsentDocuments()
	if !ok || err != nil {
		h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "Los documentos no están disponibles.")
		return
	}
	accepted, err := repo.LegalAcceptances(r.Context(), accountID)
	if err != nil {
		h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "No pudimos consultar tus aceptaciones.")
		return
	}
	items := make([]legalAcceptanceStatus, 0, len(documents))
	for _, document := range documents {
		if !h.isPublishedConsentDocument(r.Context(), document) {
			h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "Los documentos no están disponibles.")
			return
		}
		item := legalAcceptanceStatus{Type: document.Type, CurrentCode: document.Code}
		for _, acceptance := range accepted {
			if acceptance.Type != document.Type {
				continue
			}
			if item.LatestAcceptedAt == nil {
				acceptedAt := acceptance.AcceptedAt.UTC()
				item.LatestAcceptedCode = &acceptance.Code
				item.LatestAcceptedAt = &acceptedAt
			}
			item.CurrentAccepted = item.CurrentAccepted || acceptance.Code == document.Code
		}
		items = append(items, item)
	}
	h.write(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) isPublishedConsentDocument(ctx context.Context, document identity.ConsentDocument) bool {
	version, err := h.terms.TermsVersion(ctx, document.ID)
	return err == nil && version.Code == document.Code && version.Type == document.Type &&
		version.SHA256 == document.SHA256 && !version.PublishedAt.After(h.now())
}

func (h *Handler) recordLegalAcceptance(w http.ResponseWriter, r *http.Request, accountID string) {
	var input struct {
		Code string `json:"code"`
	}
	if !h.decode(w, r, &input) {
		return
	}
	if input.Code == "" || len(input.Code) > 160 {
		h.fail(w, http.StatusBadRequest, "invalid_request", "Indica la versión del documento que revisaste.")
		return
	}
	repo, ok := h.terms.(legalAcceptanceWriter)
	documents, err := identity.PublicConsentDocuments()
	if !ok || err != nil {
		h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "Los documentos no están disponibles.")
		return
	}
	for _, document := range documents {
		if document.Code != input.Code {
			continue
		}
		if !h.isPublishedConsentDocument(r.Context(), document) {
			h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "Los documentos no están disponibles.")
			return
		}
		id, err := (credentials.Generator{}).ID()
		if err == nil {
			err = repo.RecordLegalAcceptance(r.Context(), identity.TermsAcceptance{
				ID: id, AccountID: accountID, VersionID: document.ID,
				Channel: "web", AcceptedAt: h.now().UTC(),
			})
		}
		if errors.Is(err, identity.ErrNotFound) {
			h.fail(w, http.StatusUnauthorized, "unauthorized", "Vuelve a iniciar sesión para continuar.")
			return
		}
		if err != nil {
			h.fail(w, http.StatusServiceUnavailable, "terms_unavailable", "No pudimos guardar tu confirmación.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.fail(w, http.StatusConflict, "terms_version_changed", "La versión cambió. Vuelve a cargar y revisa el documento vigente.")
}
