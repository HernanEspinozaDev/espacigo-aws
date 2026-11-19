package identity_test

import (
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ana@ejemplo.cl", "a••••l"},
		{"user@domain.com", "u••••m"},
		{"a@b.cl", "a••••l"},
		{"invalid", "••••"},
		{"", "••••"},
		{"   ", "••••"},
		{"@domain.com", "••••"},
		{"user@", "••••"},
	}

	for _, tc := range tests {
		got := identity.MaskEmail(tc.input)
		if got != tc.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMaskRUT(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"12.345.678-5", "••••6785"},
		{"12345678-5", "••••6785"},
		{"11.111.111-1", "••••1111"},
		{"11111111-1", "••••1111"},
		{"22.222.222-2", "••••2222"},
		{"12.345.678-k", "••••678K"},
		{"12.345.678-K", "••••678K"},
		{"12345678-k", "••••678K"},
		{"1234", "••••1234"},
		{"123", "••••"},
		{"", "••••"},
		{"   ", "••••"},
		{"invalid", "••••"},
		{"12.345.67K-5", "••••"},
		{"12.345.678-5; injection", "••••"},
		{"••••6785", "••••6785"},
		{"••••678k", "••••678K"},
		{"••••", "••••"},
	}

	for _, tc := range tests {
		got := identity.MaskRUT(tc.input)
		if got != tc.want {
			t.Errorf("MaskRUT(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
