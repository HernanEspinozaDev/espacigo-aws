package spaces

import (
	"math"
	"testing"
)

func locationNumber(v float64) *float64 { return &v }
func TestLocationConfirmationValidation(t *testing.T) {
	valid := LocationInput{Address: "Av. Prueba 123", RegionCode: "13", CommuneCode: "13101", Latitude: locationNumber(-33.45), Longitude: locationNumber(-70.66), PublicConfirmed: true}
	if fields := valid.FieldErrors(); len(fields) > 0 {
		t.Fatalf("valid confirmation rejected: %v", fields)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*LocationInput)
	}{
		{"missing pin", func(i *LocationInput) { i.Latitude = nil; i.Longitude = nil }},
		{"partial pin", func(i *LocationInput) { i.Longitude = nil }},
		{"nan", func(i *LocationInput) { i.Latitude = locationNumber(math.NaN()) }},
		{"infinity", func(i *LocationInput) { i.Longitude = locationNumber(math.Inf(1)) }},
		{"latitude range", func(i *LocationInput) { i.Latitude = locationNumber(91) }},
		{"longitude range", func(i *LocationInput) { i.Longitude = locationNumber(-181) }},
		{"no codes", func(i *LocationInput) { i.RegionCode = ""; i.CommuneCode = "" }},
		{"mismatched codes", func(i *LocationInput) { i.CommuneCode = "14101" }},
		{"empty address", func(i *LocationInput) { i.Address = " " }},
		{"public email", func(i *LocationInput) { i.Address = "Av. Prueba 123 contacto@example.test" }},
		{"public phone", func(i *LocationInput) { i.Address = "Av. Prueba 123 +56 9 1234 5678" }},
		{"public link", func(i *LocationInput) { i.Address = "Av. Prueba 123 https://example.test" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			if len(in.FieldErrors()) == 0 {
				t.Fatal("invalid location accepted")
			}
		})
	}
	draft := LocationInput{Address: "Av. Prueba 123"}
	if fields := draft.FieldErrors(); len(fields) > 0 {
		t.Fatalf("draft without confirmed pin rejected: %v", fields)
	}
}

func TestLocationPinSourceAndPrecision(t *testing.T) {
	for _, test := range []struct {
		name, source, precision string
		withPin, valid          bool
	}{
		{name: "historical", withPin: true, valid: true},
		{name: "manual", source: "manual", withPin: true, valid: true},
		{name: "geocoded", source: "geocoded", precision: "address", withPin: true, valid: true},
		{name: "manual precision", source: "manual", precision: "address", withPin: true},
		{name: "geocoded no precision", source: "geocoded", withPin: true},
		{name: "metadata without pin", source: "manual"},
		{name: "unknown source", source: "gps", withPin: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := LocationInput{Address: "Avenida de Prueba 123", Source: test.source, Precision: test.precision}
			if test.withPin {
				input.Latitude, input.Longitude = locationNumber(-33.45), locationNumber(-70.66)
			}
			if fields := input.FieldErrors(); (len(fields) == 0) != test.valid {
				t.Fatalf("unexpected fields: %v", fields)
			}
		})
	}
}

func TestStripPostalCodeFromAddress(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "comma before and space after",
			input: "Alameda 123, 8320000 Santiago, Región Metropolitana, Chile",
			want:  "Alameda 123, Santiago, Región Metropolitana, Chile",
		},
		{
			name:  "comma before and comma after",
			input: "Av. Providencia 1234, 7500000, Providencia, Santiago",
			want:  "Av. Providencia 1234, Providencia, Santiago",
		},
		{
			name:  "space before and space after",
			input: "Alameda 123 8320000 Santiago",
			want:  "Alameda 123 Santiago",
		},
		{
			name:  "space before and comma after",
			input: "Alameda 123, Santiago 8320000, Chile",
			want:  "Alameda 123, Santiago, Chile",
		},
		{
			name:  "leading postal code with space",
			input: "8320000 Santiago, Chile",
			want:  "Santiago, Chile",
		},
		{
			name:  "leading postal code with comma",
			input: "8320000, Santiago, Chile",
			want:  "Santiago, Chile",
		},
		{
			name:  "trailing postal code with comma",
			input: "Alameda 123, Santiago, 8320000",
			want:  "Alameda 123, Santiago",
		},
		{
			name:  "trailing postal code with space",
			input: "Alameda 123, Santiago 8320000",
			want:  "Alameda 123, Santiago",
		},
		{
			name:  "only postal code",
			input: "8320000",
			want:  "",
		},
		{
			name:  "multiple adjacent postal codes",
			input: "Alameda 123, 8320000, 7500000",
			want:  "Alameda 123",
		},
		{
			name:  "preserves 1 to 5 digit street numbers",
			input: "Av. Vicuña Mackenna 10234, Depto 5, Santiago",
			want:  "Av. Vicuña Mackenna 10234, Depto 5, Santiago",
		},
		{
			name:  "preserves 6 and 8 digit numbers and hyphenated tokens",
			input: "Lote 123456, Folio 12345678, Ref 8320000-1",
			want:  "Lote 123456, Folio 12345678, Ref 8320000-1",
		},
		{
			name:  "empty and whitespace",
			input: "   ",
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripPostalCodeFromAddress(tc.input); got != tc.want {
				t.Fatalf("StripPostalCodeFromAddress(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
