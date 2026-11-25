package main

import (
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/profilekyc/didit"
)

func TestApprovedChileanIdentityCardUsesDiditDocumentType(t *testing.T) {
	for _, tc := range []struct {
		status, issuer, kind string
		want                 bool
	}{
		{"Approved", "CHL", "Identity Card", true},
		{"Approved", "CHL", "ID", true},
		{"Approved", "CHL", "Passport", false},
		{"Approved", "ARG", "Identity Card", false},
		{"Declined", "CHL", "Identity Card", false},
	} {
		got := approvedChileanIdentityCard(didit.IDVerification{
			Status: tc.status, IssuingState: tc.issuer, DocumentType: tc.kind,
		})
		if got != tc.want {
			t.Errorf("accepted %s/%s/%s = %v, want %v", tc.status, tc.issuer, tc.kind, got, tc.want)
		}
	}
}
