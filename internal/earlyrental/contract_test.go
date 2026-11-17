package earlyrental

import (
	"errors"
	"strings"
	"testing"
)

func TestDigestContractTextHashesExactUTF8Bytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{name: "without final newline", text: "EspaciGo contract v1", want: "23d81455dcee9a65cc87cdee1171a3e07429c4cb9d9461423200a4d37266b1d1"},
		{name: "with final newline", text: "EspaciGo contract v1\n", want: "dd1882977e62f7727a9e472dd8aff944ba2ba3f6e4cb65e1617c3449b2fb5366"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DigestContractText(tc.text)
			if err != nil {
				t.Fatalf("DigestContractText() error = %v", err)
			}
			if got != tc.want || !ValidContractDigest(got) {
				t.Fatalf("DigestContractText() = %q, want canonical %q", got, tc.want)
			}
		})
	}
}

func TestDigestContractTextRejectsEmptyAndInvalidUTF8(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "empty", text: ""},
		{name: "invalid utf8", text: string([]byte{0xff})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := DigestContractText(tc.text); !errors.Is(err, ErrInvalid) || got != "" {
				t.Fatalf("DigestContractText() = %q, %v; want empty digest and ErrInvalid", got, err)
			}
		})
	}
}

func TestDigestContractTextEnforcesOpenAPITermsLength(t *testing.T) {
	if _, err := DigestContractText(strings.Repeat("x", 20_000)); err != nil {
		t.Fatalf("DigestContractText(maximum length) error = %v", err)
	}
	if digest, err := DigestContractText(strings.Repeat("x", 20_001)); !errors.Is(err, ErrInvalid) || digest != "" {
		t.Fatalf("DigestContractText(over maximum) = %q, %v; want empty digest and ErrInvalid", digest, err)
	}
}
