package adminglobal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReportPeriodRequiresIANAZoneAndBoundedWindow(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, zone string
		until      time.Time
		valid      bool
	}{{"valid", "America/Santiago", from.Add(30 * 24 * time.Hour), true}, {"unknown zone", "SCL", from.Add(time.Hour), false}, {"too long", "UTC", from.Add(32 * 24 * time.Hour), false}, {"empty window", "UTC", from, false}} {
		t.Run(tc.name, func(t *testing.T) {
			err := (ReportPeriod{From: from, Until: tc.until, TimeZone: tc.zone}).Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate()=%v want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestFinancialCLPSerializesAsExactText(t *testing.T) {
	data, err := json.Marshal(FinanceKPI{Kind: "cobro", AmountCLP: "9007199254740993"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"amount_clp":"9007199254740993"`) {
		t.Fatalf("amount lost exact integer text: %s", data)
	}
}

func TestAuditCSVQuotesAndNeutralizesFormulaCells(t *testing.T) {
	at := time.Date(2026, 10, 15, 1, 2, 3, 0, time.UTC)
	p := AuditPage{Items: []AuditEntry{{ID: "123", ResourceType: "=HYPERLINK(\"bad\")", ResourceID: "00000000-0000-4000-8000-000000000001", Action: "+cmd", Result: "exito", Reason: "@SUM(1)", CorrelationID: "-x", OccurredAt: at, RecordedAt: at}}}
	var out bytes.Buffer
	if err := WriteAuditCSV(&out, p); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"'=HYPERLINK(\"\"bad\"\")", "'+cmd", "'@SUM(1)", "'-x"} {
		if !strings.Contains(got, want) {
			t.Fatalf("CSV missing protected cell %q: %s", want, got)
		}
	}
	if !strings.Contains(got, "\"'=HYPERLINK(\"") {
		t.Fatalf("CSV quoting not applied: %s", got)
	}
}

func TestRestrictedAuditReasonsHideHistoricalFreeText(t *testing.T) {
	for _, action := range []string{
		"admin.global.assisted_view.start",
		"admin.global.accounts.sensitive_access",
		"admin.global.accounts.update",
		"admin.global.accounts.block",
		"admin.global.accounts.unblock",
		"admin.global.accounts.sessions.revoke",
		"admin.global.accounts.legacy-administrator.grant",
		"admin.global.accounts.roles.grant.arrendador",
		"admin.global.publications.update",
		"admin.global.publications.state",
		"admin.global.support_tickets.create",
	} {
		if got := restrictedAuditReason(action, "texto histórico confidencial"); got != "detalle_restringido" {
			t.Errorf("restrictedAuditReason(%q)=%q", action, got)
		}
	}
	for action, reason := range map[string]string{
		"admin.global.accounts.read":               "consulta_administrativa",
		"admin.global.audit.read":                  "consulta_administrativa",
		"admin.global.approvals.create":            "motivo_operativo",
		"admin.global.publications.list":           "consulta_administrativa",
		"admin.global.privacy_incidents.create":    "alta",
		"admin.global.privacy_incidents.transition": "contain",
		"admin.global.privacy_legal_holds.create":  "expediente_privacidad",
		"admin.global.privacy_legal_holds.release": "retencion_liberada",
	} {
		if got := restrictedAuditReason(action, reason); got != reason {
			t.Errorf("standard reason for %q changed: got %q want %q", action, got, reason)
		}
	}
}
