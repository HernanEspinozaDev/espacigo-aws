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
