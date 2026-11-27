package identityhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
)

type legalAcceptanceReader interface {
	LegalAcceptances(context.Context, string) ([]privacy.TermsAcceptance, error)
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
		version, err := h.terms.TermsVersion(r.Context(), document.ID)
		if err != nil || version.Code != document.Code || version.Type != document.Type ||
			version.SHA256 != document.SHA256 || version.PublishedAt.After(h.now()) {
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
