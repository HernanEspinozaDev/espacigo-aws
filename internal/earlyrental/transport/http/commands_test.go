package earlyrentalhttp

import (
	"errors"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

func TestDecodeCommandBodyMapsOnlyAllowedFields(t *testing.T) {
	tests := []struct {
		name string
		kind earlyrental.CommandKind
		body string
		want earlyrental.Command
	}{
		{name: "identity", kind: earlyrental.CommandConfirmIdentity, body: `{}`, want: earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}},
		{name: "payment sent", kind: earlyrental.CommandSendPayment, body: `{}`, want: earlyrental.Command{Kind: earlyrental.CommandSendPayment}},
		{name: "payment received", kind: earlyrental.CommandReceivePayment, body: `{}`, want: earlyrental.Command{Kind: earlyrental.CommandReceivePayment}},
		{name: "contract", kind: earlyrental.CommandAcceptContract, body: `{"contract_version":2,"contract_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`, want: earlyrental.Command{
			Kind: earlyrental.CommandAcceptContract, ContractVersion: 2, ContractDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}},
		{name: "reject", kind: earlyrental.CommandReject, body: `{"reason_code":"condiciones_no_aceptadas"}`, want: earlyrental.Command{Kind: earlyrental.CommandReject, ReasonCode: "condiciones_no_aceptadas"}},
		{name: "cancel", kind: earlyrental.CommandCancel, body: `{"reason_code":"cambio_de_planes"}`, want: earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeCommandBody(test.kind, []byte(test.body))
			if err != nil {
				t.Fatalf("DecodeCommandBody() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("DecodeCommandBody() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestDecodeCommandBodyRejectsMalformedOrClientOwnedFields(t *testing.T) {
	for _, test := range []struct {
		name string
		kind earlyrental.CommandKind
		body string
	}{
		{name: "empty bytes", kind: earlyrental.CommandConfirmIdentity, body: ``},
		{name: "null", kind: earlyrental.CommandConfirmIdentity, body: `null`},
		{name: "array", kind: earlyrental.CommandConfirmIdentity, body: `[]`},
		{name: "unknown identity field", kind: earlyrental.CommandConfirmIdentity, body: `{"actor_id":"host"}`},
		{name: "case variant of contract field", kind: earlyrental.CommandAcceptContract, body: `{"Contract_Version":1,"contract_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`},
		{name: "duplicate contract field", kind: earlyrental.CommandAcceptContract, body: `{"contract_version":1,"contract_version":2,"contract_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`},
		{name: "client payment id", kind: earlyrental.CommandSendPayment, body: `{"payment_id":"550e8400-e29b-41d4-a716-446655440000"}`},
		{name: "client timestamp", kind: earlyrental.CommandSendPayment, body: `{"sent_at":"2026-11-17T10:00:00Z"}`},
		{name: "trailing object", kind: earlyrental.CommandConfirmIdentity, body: `{} {}`},
		{name: "missing contract version", kind: earlyrental.CommandAcceptContract, body: `{"contract_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`},
		{name: "unknown reason", kind: earlyrental.CommandCancel, body: `{"reason_code":"free_text"}`},
		{name: "system expiry is not a client command", kind: earlyrental.CommandExpire, body: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if command, err := DecodeCommandBody(test.kind, []byte(test.body)); err == nil {
				t.Fatalf("DecodeCommandBody() = %+v, want error", command)
			}
		})
	}
}

func TestDecodeCommandBodyDistinguishesMalformedFromInvalidValues(t *testing.T) {
	if _, err := DecodeCommandBody(earlyrental.CommandConfirmIdentity, []byte(`{"actor_id":"client"}`)); !errors.Is(err, ErrMalformedCommandBody) {
		t.Fatalf("unknown field error = %v, want ErrMalformedCommandBody", err)
	}
	if _, err := DecodeCommandBody(earlyrental.CommandAcceptContract, []byte(`{"contract_version":0,"contract_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)); !errors.Is(err, ErrInvalidCommandValue) {
		t.Fatalf("invalid version error = %v, want ErrInvalidCommandValue", err)
	}
	if _, err := DecodeCommandBody(earlyrental.CommandCancel, []byte(`{"reason_code":"free_text"}`)); !errors.Is(err, ErrInvalidCommandValue) {
		t.Fatalf("invalid reason error = %v, want ErrInvalidCommandValue", err)
	}
}

func TestDecodeCommandBodyRejectsNoncanonicalContractDigestAndOversizedBody(t *testing.T) {
	for name, body := range map[string]string{
		"uppercase digest": `{"contract_version":1,"contract_digest":"0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"}`,
		"nonhex digest":    `{"contract_version":1,"contract_digest":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCommandBody(earlyrental.CommandAcceptContract, []byte(body)); !errors.Is(err, ErrInvalidCommandValue) {
				t.Fatalf("DecodeCommandBody() error = %v, want ErrInvalidCommandValue", err)
			}
		})
	}
	if _, err := DecodeCommandBody(earlyrental.CommandConfirmIdentity, make([]byte, maxCommandBodyBytes+1)); !errors.Is(err, ErrMalformedCommandBody) {
		t.Fatalf("oversized body error = %v, want ErrMalformedCommandBody", err)
	}
}
