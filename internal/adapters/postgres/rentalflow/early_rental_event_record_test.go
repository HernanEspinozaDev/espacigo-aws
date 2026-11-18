package rentalflowpg

import (
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
)

const eventRecordActor = "22222222-2222-4222-8222-222222222222"
const eventRecordPayment = "11111111-1111-4111-8111-111111111111"

func TestMapEarlyRentalEventRecordCanonical(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
	for _, c := range []struct {
		kind, previous, next, reason, actor, pay string
		seq, version                             int64
		exposePayment                            bool
	}{
		{"oferta_creada", "", "pendiente_identidad", "", eventRecordActor, "", 1, 0, false},
		{"identidad_simulada_confirmada", "pendiente_identidad", "pendiente_contrato", "", eventRecordActor, "", 3, 0, false},
		{"identidad_simulada_confirmada", "pendiente_identidad", "pendiente_identidad", "", eventRecordActor, "", 2, 0, false},
		{"contrato_aceptado", "pendiente_contrato", "pendiente_envio_pago", "", eventRecordActor, "", 5, 2, false},
		{"contrato_aceptado", "pendiente_contrato", "pendiente_contrato", "", eventRecordActor, "", 4, 2, false},
		{"pago_marcado_enviado", "pendiente_envio_pago", "pendiente_recepcion_pago", "", eventRecordActor, eventRecordPayment, 6, 0, true},
		{"pago_marcado_recibido", "pendiente_recepcion_pago", "confirmado", "", eventRecordActor, eventRecordPayment, 7, 0, true},
		{"oferta_rechazada", "pendiente_identidad", "rechazado", "otro", eventRecordActor, "", 2, 0, false},
		{"oferta_cancelada", "pendiente_recepcion_pago", "cancelado", "cambio_de_planes", eventRecordActor, eventRecordPayment, 7, 0, false},
		{"oferta_vencida", "pendiente_recepcion_pago", "vencido", "", "", eventRecordPayment, 7, 0, false},
		{"oferta_cancelada", "pendiente_identidad", "cancelado", "otro", eventRecordActor, "", 2, 0, false},
		{"oferta_vencida", "pendiente_identidad", "vencido", "", "", "", 2, 0, false},
	} {
		t.Run(c.kind+"/"+c.previous+"_a_"+c.next, func(t *testing.T) {
			row := earlyRentalEventRecord{Sequence: c.seq, Revision: c.seq, Kind: c.kind, PreviousState: c.previous, NextState: c.next, ReasonCode: c.reason, ActorID: c.actor, PaymentID: c.pay, OccurredAt: at, ContractVersion: c.version}
			want := earlyrental.Event{Kind: earlyrental.EventKind(c.kind), ActorID: c.actor, Sequence: uint64(c.seq), Revision: uint64(c.seq), At: at.UTC(), PreviousState: earlyrental.State(c.previous), NextState: earlyrental.State(c.next), ReasonCode: c.reason, ContractVersion: c.version}
			if c.kind == "oferta_vencida" {
				want.ActorID = "system"
			}
			if c.version > 0 {
				row.ContractDigest = []byte(strings.Repeat("\xab", 32))
				want.ContractDigest = strings.Repeat("ab", 32)
			}
			got, ok := mapEarlyRentalEventRecord(row)
			if !ok {
				t.Fatal("valid persisted event rejected")
			}
			if c.exposePayment {
				if got.PaymentID == nil || *got.PaymentID != eventRecordPayment {
					t.Fatalf("payment ID: %#v", got)
				}
				got.PaymentID = nil
			}
			if got != want {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestMapEarlyRentalEventRecordRejectsInvalidRows(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	base := earlyRentalEventRecord{Sequence: 2, Revision: 2, Kind: "oferta_cancelada", ActorID: eventRecordActor, PreviousState: "pendiente_identidad", NextState: "cancelado", ReasonCode: "otro", OccurredAt: at}
	if _, ok := mapEarlyRentalEventRecord(base); !ok {
		t.Fatal("valid control rejected")
	}
	for _, c := range []struct {
		name   string
		mutate func(*earlyRentalEventRecord)
	}{
		{"negative sequence", func(r *earlyRentalEventRecord) { r.Sequence = -1; r.Revision = -1 }},
		{"mismatched revision", func(r *earlyRentalEventRecord) { r.Revision = 3 }},
		{"noninitial sequence one", func(r *earlyRentalEventRecord) { r.Sequence = 1; r.Revision = 1 }},
		{"empty time", func(r *earlyRentalEventRecord) { r.OccurredAt = time.Time{} }},
		{"unknown kind", func(r *earlyRentalEventRecord) { r.Kind = "unknown" }},
		{"wrong state", func(r *earlyRentalEventRecord) { r.NextState = "pendiente_identidad" }},
		{"invalid actor", func(r *earlyRentalEventRecord) { r.ActorID = "00000000-0000-0000-0000-000000000000" }},
		{"actor missing", func(r *earlyRentalEventRecord) { r.ActorID = "" }},
		{"expiry with actor", func(r *earlyRentalEventRecord) { r.Kind = "oferta_vencida"; r.NextState = "vencido"; r.ReasonCode = "" }},
		{"reason missing", func(r *earlyRentalEventRecord) { r.ReasonCode = "" }},
		{"unknown reason", func(r *earlyRentalEventRecord) { r.ReasonCode = "other" }},
		{"unexpected contract metadata", func(r *earlyRentalEventRecord) { r.ContractDigest = make([]byte, 32) }},
		{"invalid payment reference", func(r *earlyRentalEventRecord) { r.PaymentID = "not-a-uuid" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.mutate(&r)
			if got, ok := mapEarlyRentalEventRecord(r); ok || got != (earlyrental.Event{}) {
				t.Fatalf("expected zero rejected event, got %#v accepted=%v", got, ok)
			}
		})
	}
}

func TestMapEarlyRentalEventRecordRejectsMissingEvidence(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	contract := earlyRentalEventRecord{Sequence: 4, Revision: 4, Kind: "contrato_aceptado", ActorID: eventRecordActor, PreviousState: "pendiente_contrato", NextState: "pendiente_contrato", OccurredAt: at, ContractVersion: 1, ContractDigest: make([]byte, 32)}
	sent := earlyRentalEventRecord{Sequence: 6, Revision: 6, Kind: "pago_marcado_enviado", ActorID: eventRecordActor, PreviousState: "pendiente_envio_pago", NextState: "pendiente_recepcion_pago", OccurredAt: at, PaymentID: eventRecordPayment}
	for _, c := range []struct {
		name   string
		row    earlyRentalEventRecord
		mutate func(*earlyRentalEventRecord)
	}{
		{"contract version", contract, func(r *earlyRentalEventRecord) { r.ContractVersion = 0 }},
		{"contract digest", contract, func(r *earlyRentalEventRecord) { r.ContractDigest = make([]byte, 31) }},
		{"payment ID", sent, func(r *earlyRentalEventRecord) { r.PaymentID = "" }},
		{"unexpected reason", sent, func(r *earlyRentalEventRecord) { r.ReasonCode = "otro" }},
		{"unexpected payment", contract, func(r *earlyRentalEventRecord) { r.PaymentID = eventRecordPayment }},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := c.row
			if _, ok := mapEarlyRentalEventRecord(r); !ok {
				t.Fatal("valid control rejected")
			}
			c.mutate(&r)
			if got, ok := mapEarlyRentalEventRecord(r); ok || got != (earlyrental.Event{}) {
				t.Fatalf("expected zero rejected event, got %#v accepted=%v", got, ok)
			}
		})
	}
}
