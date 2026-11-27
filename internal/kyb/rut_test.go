package kyb

import "testing"

func TestNormalizeChileanRUT(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "formatted company rut", input: "76.123.456-0", want: "76123456-0", valid: true},
		{name: "representative rut with K", input: "12.345.678-5", want: "12345678-5", valid: true},
		{name: "bad check digit", input: "76.123.456-1", valid: false},
		{name: "missing separator", input: "761234560", valid: false},
		{name: "non-digit body", input: "76A23456-0", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeChileanRUT(test.input)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("NormalizeChileanRUT(%q) = %q, %v; want %q", test.input, got, err, test.want)
				}
				return
			}
			if err == nil || ValidateChileanRUT(test.input) {
				t.Fatalf("invalid RUT %q was accepted", test.input)
			}
		})
	}
}
