package publiccatalogpg

import (
	"context"
	"encoding/json"
	"testing"
)

// Parent-owned acceptance: drive the real adapter, not just the validator.
func TestPublicAttributesOmitInvalidValuesWithoutLosingFalseOrZero(t *testing.T) {
	cases := []struct{ name, kind, raw, options string }{
		{"null boolean", "boolean", `null`, `[]`},
		{"string boolean", "boolean", `"false"`, `[]`},
		{"numeric boolean", "boolean", `0`, `[]`},
		{"fractional integer", "integer", `1.5`, `[]`},
		{"string integer", "integer", `"0"`, `[]`},
		{"overflow integer", "integer", `9223372036854775808`, `[]`},
		{"string number", "number", `"12.3"`, `[]`},
		{"object number", "number", `{"private":"PRIVATE_ATTRIBUTE_MARKER"}`, `[]`},
		{"unknown enum", "enum", `"private_value"`, `["public_value"]`},
		{"enum wrong type", "enum", `false`, `["public_value"]`},
		{"enum no options", "enum", `"public_value"`, `null`},
		{"list unknown option", "enum_list", `["public_value","private_value"]`, `["public_value"]`},
		{"list wrong member", "enum_list", `["public_value",0]`, `["public_value"]`},
		{"list wrong type", "enum_list", `"public_value"`, `["public_value"]`},
		{"unknown kind", "text", `"PRIVATE_ATTRIBUTE_MARKER"`, `[]`},
		{"invalid JSON", "boolean", `false trailing`, `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := &publicAttributesRows{data: [][]any{
				{"false_control", "Control false", "boolean", 1, "", []byte(`false`), []byte(`[]`)},
				{"zero_control", "Control zero", "integer", 2, "", []byte(`0`), []byte(`[]`)},
				{"candidate", "Candidate", tc.kind, 3, "", []byte(tc.raw), []byte(tc.options)},
			}}
			got, err := (&Repository{pool: &publicAttributesPool{rows: rows}}).PublicAttributes(context.Background(), "space-id")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || string(got[0].Value) != "false" || string(got[1].Value) != "0" {
				t.Fatalf("filtered attributes=%+v", got)
			}
		})
	}
}

func TestPublicAttributesPreserveLargeIntegerLexeme(t *testing.T) {
	rows := &publicAttributesRows{data: [][]any{{"large", "Large", "integer", 1, "", []byte(`9007199254740993`), []byte(`[]`)}}}
	got, err := (&Repository{pool: &publicAttributesPool{rows: rows}}).PublicAttributes(context.Background(), "space-id")
	if err != nil || len(got) != 1 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	wire, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["value"]) != "9007199254740993" {
		t.Fatalf("rounded integer: %s", fields["value"])
	}
}
