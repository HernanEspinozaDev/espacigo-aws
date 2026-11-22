package commercialauthorization

import (
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
)

func validGrant(now time.Time) Grant {
	return Grant{ID: "10000000-0000-4000-8000-000000000001", SpaceID: "20000000-0000-4000-8000-000000000002", TemplateVersion: 3, MaxAmountCLP: 50000, ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
}
func validTerms() commercialproposal.Terms {
	return commercialproposal.Terms{SpaceID: "20000000-0000-4000-8000-000000000002", TemplateVersion: 3, Currency: "CLP", SubtotalCLP: 50000}
}

func TestEvaluateAllowsOnlyExactPublishedVersionWithinAmountAndValidity(t *testing.T) {
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	grant := validGrant(now)
	terms := validTerms()
	if d := Evaluate(grant, terms, now); !d.Eligible || d.Reason != "within_authorized_limits_manual_review_required" {
		t.Fatalf("eligible decision=%+v", d)
	}
	cases := []struct {
		name   string
		mutate func(*Grant, *commercialproposal.Terms, time.Time)
		reason string
	}{
		{"version changed", func(_ *Grant, t *commercialproposal.Terms, _ time.Time) { t.TemplateVersion++ }, "template_version_mismatch"},
		{"different space", func(_ *Grant, t *commercialproposal.Terms, _ time.Time) {
			t.SpaceID = "30000000-0000-4000-8000-000000000003"
		}, "template_version_mismatch"},
		{"over cap", func(_ *Grant, t *commercialproposal.Terms, _ time.Time) { t.SubtotalCLP++ }, "amount_out_of_bounds"},
		{"wrong currency", func(_ *Grant, t *commercialproposal.Terms, _ time.Time) { t.Currency = "USD" }, "amount_out_of_bounds"},
		{"revoked", func(g *Grant, _ *commercialproposal.Terms, now time.Time) { v := now; g.RevokedAt = &v }, "authorization_unavailable"},
		{"expired", func(g *Grant, _ *commercialproposal.Terms, now time.Time) { g.ExpiresAt = now }, "authorization_expired_or_not_yet_valid"},
		{"not started", func(g *Grant, _ *commercialproposal.Terms, now time.Time) { g.ValidFrom = now.Add(time.Minute) }, "authorization_expired_or_not_yet_valid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, tm := grant, terms
			tc.mutate(&g, &tm, now)
			d := Evaluate(g, tm, now)
			if d.Eligible || d.Reason != tc.reason {
				t.Fatalf("decision=%+v want reason %q", d, tc.reason)
			}
		})
	}
}

func TestValidateActivationBoundsVersionAmountAndDuration(t *testing.T) {
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	valid := Activation{TemplateVersion: 1, MaxAmountCLP: 1, ValidFrom: now, ExpiresAt: now.Add(MaxValidity)}
	if !ValidateActivation(valid, now) {
		t.Fatal("valid maximum duration rejected")
	}
	for _, mutate := range []func(*Activation){func(a *Activation) { a.TemplateVersion = 0 }, func(a *Activation) { a.MaxAmountCLP = 0 }, func(a *Activation) { a.MaxAmountCLP = MaxAmountCLP + 1 }, func(a *Activation) { a.ExpiresAt = a.ValidFrom.Add(MaxValidity + time.Second) }, func(a *Activation) { a.ValidFrom = now.Add(-time.Minute - time.Nanosecond) }} {
		bad := valid
		mutate(&bad)
		if ValidateActivation(bad, now) {
			t.Fatalf("invalid activation accepted: %+v", bad)
		}
	}
}
