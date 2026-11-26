package profilekyc

import (
	"errors"
	"testing"
	"time"
)

func exampleData() Data {
	return Data{
		Username: "Rincon-Santiago", FirstName: "María José", FirstLastName: "O’Neill", NoSecondLastName: true,
		RUT: "12.345.678-5", DateOfBirth: "1990-06-15", DocumentType: "cl_id", Phone: "+56 9 1234 5678", Consent: true,
	}
}

func TestNormalizeIdentityProfile(t *testing.T) {
	now := time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC)
	input := exampleData()
	got, err := normalize(input, now)
	if err != nil || got.RUT != "12345678-5" || got.Phone != "+56912345678" || got.LegalName() != "María José O’Neill" {
		t.Fatalf("normalization failed: %#v, %v", got, err)
	}
	for _, tc := range []struct {
		field   string
		invalid func(*Data)
	}{
		{"username", func(d *Data) { d.Username = "" }},
		{"first_name", func(d *Data) { d.FirstName = "Name1" }},
		{"first_last_name", func(d *Data) { d.FirstLastName = "" }},
		{"second_last_name", func(d *Data) { d.NoSecondLastName = false }},
		{"rut", func(d *Data) { d.RUT = "12345678-0" }},
		{"date_of_birth", func(d *Data) { d.DateOfBirth = "2027-01-01" }},
		{"document_type", func(d *Data) { d.DocumentType = "passport" }},
		{"phone", func(d *Data) { d.Phone = "1234" }},
		{"consent", func(d *Data) { d.Consent = false }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			d := input
			tc.invalid(&d)
			_, err := normalize(d, now)
			var fields *FieldError
			if !errors.As(err, &fields) || fields.Fields[tc.field] == "" {
				t.Fatalf("expected field-specific error for %s: %v", tc.field, err)
			}
		})
	}
}

func TestNormalizeRUTAcceptsCompactAndRejectsWrongVerifier(t *testing.T) {
	for _, value := range []string{"12.345.678-5", "12345678-5", "123456785", "12.345.678–5", "6.000.000-k"} {
		if _, err := normalizeRUT(value); err != nil {
			t.Errorf("expected valid format for %q: %v", value, err)
		}
	}
	for _, value := range []string{"0-0", "00.000.000-0", "1.23.45678-5", "12.345.678--5", "12abc345678-5", "12345678-0", "999999999-1", "12345678-"} {
		if _, err := normalizeRUT(value); err == nil {
			t.Errorf("accepted invalid %q", value)
		}
	}
}

func TestDecisionRequiresAllExpectedFieldsAndDocumentValidity(t *testing.T) {
	now := time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC)
	data, _ := normalize(exampleData(), now)
	approved := Decision{State: "verified", FirstName: "MARÍA JOSÉ", LastName: "O’NEILL",
		RUT: "12.345.678-5", DateOfBirth: "1990-06-15", DocumentExpires: "2027-01-01", ChecksPassed: true}
	state, until := decisionState(approved, data, now)
	if state != "verified" || until == nil || until.Format(time.RFC3339) != "2027-01-02T03:00:00Z" {
		t.Fatalf("unexpected approved state %s, %v", state, until)
	}
	for _, change := range []func(*Decision){
		func(d *Decision) { d.RUT = "6.000.001-K" },
		func(d *Decision) { d.DateOfBirth = "1991-06-15" },
		func(d *Decision) { d.FirstName = "Ana" },
		func(d *Decision) { d.ChecksPassed = false },
		func(d *Decision) { d.DocumentExpires = "2020-01-01" },
	} {
		d := approved
		change(&d)
		if state, until := decisionState(d, data, now); state != "needs_correction" || until != nil {
			t.Errorf("accepted non-matching decision: %s, %v", state, until)
		}
	}
}

func TestReturnPathStaysInsideHostDraft(t *testing.T) {
	valid := []string{
		"/anfitrion/espacios",
		"/anfitrion/espacios/nuevo",
		"/anfitrion/espacios/0a6d1c19-b7f7-4da6-a1a0-ac14c599d6d8/editar?tab=contrato",
	}
	for _, path := range valid {
		if !hostReturnPath.MatchString(path) {
			t.Errorf("rejected allowed draft path %q", path)
		}
	}
	invalid := []string{
		"//example.test/anfitrion/espacios",
		"https://example.test/anfitrion/espacios",
		"/anfitrion/espacios/nuevo?return_to=//example.test",
		"/anfitrion/espacios/%2F..%2F..",
	}
	for _, path := range invalid {
		if hostReturnPath.MatchString(path) {
			t.Errorf("accepted invalid return path %q", path)
		}
	}
}
