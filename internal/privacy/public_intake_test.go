package privacy

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

type memoryPublicIntakeRepo struct {
	memoryRepository
	stored map[string]storedPublicRequest
	lastIn PublicRightsRequestInput
}

type storedPublicRequest struct {
	input        PublicRightsRequestInput
	folio        string
	trackingHash []byte
	status       PublicRightsStatus
}

func (m *memoryPublicIntakeRepo) CreatePublicRightsRequest(_ context.Context, in PublicRightsRequestInput, folio string, trackingHash []byte, now, dueAt time.Time) (PublicRightsReceipt, error) {
	if m.stored == nil {
		m.stored = make(map[string]storedPublicRequest)
	}
	m.lastIn = in
	hashCopy := append([]byte(nil), trackingHash...)
	dueCopy := dueAt.UTC()
	createdAt := now.UTC()
	state := "identidad_pendiente"
	m.stored[folio] = storedPublicRequest{
		input:        in,
		folio:        folio,
		trackingHash: hashCopy,
		status: PublicRightsStatus{
			Folio:         folio,
			RightType:     in.RightType,
			RequesterType: in.RequesterType,
			State:         state,
			CreatedAt:     createdAt,
			DueAt:         &dueCopy,
			Events: []RightsRequestEvent{{
				ID:        "ev-1",
				Sequence:  1,
				EventType: "recepcion",
				StateTo:   &state,
				NewDueAt:  &dueCopy,
				CreatedAt: createdAt,
			}},
		},
	}
	return PublicRightsReceipt{
		ID:            "req-public-1",
		Folio:         folio,
		RightType:     in.RightType,
		RequesterType: in.RequesterType,
		State:         state,
		Channel:       "publico",
		DueAt:         dueCopy,
		CreatedAt:     createdAt,
	}, nil
}

func (m *memoryPublicIntakeRepo) LookupPublicRightsRequest(_ context.Context, folio string, trackingHash []byte) (PublicRightsStatus, error) {
	rec, ok := m.stored[folio]
	if !ok {
		return PublicRightsStatus{}, ErrNotFound
	}
	if len(rec.trackingHash) != 32 || len(trackingHash) != 32 || subtle.ConstantTimeCompare(rec.trackingHash, trackingHash) != 1 {
		return PublicRightsStatus{}, ErrNotFound
	}
	return rec.status, nil
}

func newTestServiceWithClock(repo Repository, now time.Time) *Service {
	svc, err := NewService(repo)
	if err != nil {
		panic(err)
	}
	svc.now = func() time.Time { return now }
	return svc
}

func TestSubmitPublicRightsRequestValidAllRightsAndRepresentative(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 12, 16, 45, 0, 0, time.UTC)
	folioPattern := regexp.MustCompile(`^PRIV-2026-[0-9A-F]{8}$`)

	for _, right := range []string{"acceso", "rectificacion", "supresion", "oposicion", "portabilidad", "bloqueo"} {
		t.Run("titular_"+right, func(t *testing.T) {
			repo := &memoryPublicIntakeRepo{}
			svc := newTestServiceWithClock(repo, fixedNow)

			receipt, err := svc.SubmitPublicRightsRequest(context.Background(), PublicRightsRequestInput{
				RightType:      right,
				RequesterType:  "titular",
				RequesterName:  "  Ada Lovelace  ",
				RequesterEmail: "  ada@ejemplo.cl  ",
				Detail:         "  Solicito ejercer mi derecho de privacidad sobre mis registros.  ",
			})
			if err != nil {
				t.Fatalf("SubmitPublicRightsRequest(%s) error = %v", right, err)
			}
			if !folioPattern.MatchString(receipt.Folio) {
				t.Fatalf("unexpected folio %q", receipt.Folio)
			}
			if len(receipt.TrackingCode) != 32 {
				t.Fatalf("unexpected tracking code length %d", len(receipt.TrackingCode))
			}
			if receipt.State != "identidad_pendiente" || receipt.Channel != "publico" {
				t.Fatalf("unexpected state/channel: %+v", receipt)
			}
			wantDue := ComputeInitialDeadline(fixedNow, right)
			if !receipt.DueAt.Equal(wantDue) {
				t.Fatalf("due_at = %v, want %v", receipt.DueAt, wantDue)
			}
			if !strings.Contains(strings.ToLower(receipt.Notice), "no confirma") {
				t.Fatalf("notice should be neutral, got %q", receipt.Notice)
			}

			stored := repo.stored[receipt.Folio]
			sum := sha256.Sum256([]byte(receipt.TrackingCode))
			if subtle.ConstantTimeCompare(stored.trackingHash, sum[:]) != 1 {
				t.Fatal("stored tracking hash does not match SHA-256 of tracking code")
			}
			if repo.lastIn.RequesterName != "Ada Lovelace" || repo.lastIn.RequesterEmail != "ada@ejemplo.cl" {
				t.Fatalf("inputs were not trimmed: %+v", repo.lastIn)
			}
		})
	}

	for _, relation := range []string{"mandatario", "representante_legal", "tutor_nna"} {
		t.Run("representante_"+relation, func(t *testing.T) {
			repo := &memoryPublicIntakeRepo{}
			svc := newTestServiceWithClock(repo, fixedNow)

			receipt, err := svc.SubmitPublicRightsRequest(context.Background(), PublicRightsRequestInput{
				RightType:              "acceso",
				RequesterType:          "representante",
				RequesterName:          "Representante Legal",
				RequesterEmail:         "rep@ejemplo.cl",
				RepresentativeName:     "Estudio Jurídico",
				RepresentativeRelation: relation,
				SubjectReference:       "  Titular Cuenta Ejemplo  ",
				Detail:                 "Solicito copia de los datos personales en representación del titular.",
			})
			if err != nil {
				t.Fatalf("representative(%s) error = %v", relation, err)
			}
			if receipt.RequesterType != "representante" {
				t.Fatalf("requester_type = %q, want representante", receipt.RequesterType)
			}
			if repo.lastIn.SubjectReference != "Titular Cuenta Ejemplo" {
				t.Fatalf("subject_reference = %q", repo.lastIn.SubjectReference)
			}
		})
	}
}

func TestSubmitPublicRightsRequestValidationRejectsInvalidInputs(t *testing.T) {
	fixedNow := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	validBase := PublicRightsRequestInput{
		RightType:      "acceso",
		RequesterType:  "titular",
		RequesterName:  "Persona Titular",
		RequesterEmail: "persona@ejemplo.cl",
		Detail:         "Detalle suficientemente largo para cumplir validación.",
	}

	cases := []struct {
		name   string
		mutate func(*PublicRightsRequestInput)
	}{
		{name: "invalid right", mutate: func(in *PublicRightsRequestInput) { in.RightType = "otro" }},
		{name: "invalid requester type", mutate: func(in *PublicRightsRequestInput) { in.RequesterType = "admin" }},
		{name: "short name", mutate: func(in *PublicRightsRequestInput) { in.RequesterName = "A" }},
		{name: "long name", mutate: func(in *PublicRightsRequestInput) { in.RequesterName = strings.Repeat("a", 161) }},
		{name: "nul in name", mutate: func(in *PublicRightsRequestInput) { in.RequesterName = "Ada\x00Lovelace" }},
		{name: "email with display name", mutate: func(in *PublicRightsRequestInput) { in.RequesterEmail = "Ada <ada@ejemplo.cl>" }},
		{name: "email with spaces", mutate: func(in *PublicRightsRequestInput) { in.RequesterEmail = "ada @ejemplo.cl" }},
		{name: "email with nul", mutate: func(in *PublicRightsRequestInput) { in.RequesterEmail = "ada@ejem\x00plo.cl" }},
		{name: "email without dot in domain", mutate: func(in *PublicRightsRequestInput) { in.RequesterEmail = "ada@localhost" }},
		{name: "email too long", mutate: func(in *PublicRightsRequestInput) { in.RequesterEmail = strings.Repeat("a", 245) + "@ejemplo.cl" }},
		{name: "short detail for acceso", mutate: func(in *PublicRightsRequestInput) { in.Detail = "corto" }},
		{name: "long detail", mutate: func(in *PublicRightsRequestInput) { in.Detail = strings.Repeat("x", 1001) }},
		{name: "nul in detail", mutate: func(in *PublicRightsRequestInput) { in.Detail = "Detalle válido\x00con nul" }},
		{name: "titular with rep name", mutate: func(in *PublicRightsRequestInput) { in.RepresentativeName = "Rep" }},
		{name: "titular with rep relation", mutate: func(in *PublicRightsRequestInput) { in.RepresentativeRelation = "mandatario" }},
		{name: "representante missing rep name", mutate: func(in *PublicRightsRequestInput) {
			in.RequesterType = "representante"
			in.RepresentativeRelation = "mandatario"
		}},
		{name: "representante invalid relation", mutate: func(in *PublicRightsRequestInput) {
			in.RequesterType = "representante"
			in.RepresentativeName = "Estudio"
			in.RepresentativeRelation = "amigo"
		}},
		{name: "short subject reference", mutate: func(in *PublicRightsRequestInput) { in.SubjectReference = "A" }},
		{name: "long subject reference", mutate: func(in *PublicRightsRequestInput) { in.SubjectReference = strings.Repeat("r", 201) }},
		{name: "nul subject reference", mutate: func(in *PublicRightsRequestInput) { in.SubjectReference = "Ref\x001" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &memoryPublicIntakeRepo{}
			svc := newTestServiceWithClock(repo, fixedNow)
			in := validBase
			tc.mutate(&in)
			if _, err := svc.SubmitPublicRightsRequest(context.Background(), in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("%s: got err=%v, want ErrInvalid", tc.name, err)
			}
		})
	}

	t.Run("legacy repository unavailable", func(t *testing.T) {
		svc := newTestServiceWithClock(&memoryRepository{}, fixedNow)
		if _, err := svc.SubmitPublicRightsRequest(context.Background(), validBase); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("expected unavailable error, got %v", err)
		}
	})
}

func TestLookupPublicRightsRequestConstantTimeAndMinimizedFields(t *testing.T) {
	fixedNow := time.Date(2026, time.August, 3, 14, 0, 0, 0, time.UTC)
	repo := &memoryPublicIntakeRepo{}
	svc := newTestServiceWithClock(repo, fixedNow)

	receipt, err := svc.SubmitPublicRightsRequest(context.Background(), PublicRightsRequestInput{
		RightType:      "portabilidad",
		RequesterType:  "titular",
		RequesterName:  "Camila Titular",
		RequesterEmail: "camila@ejemplo.cl",
		Detail:         "Solicito portabilidad de mis datos en formato estructurado.",
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := svc.LookupPublicRightsRequest(context.Background(), PublicRightsLookupInput{
		Folio:        "  " + strings.ToLower(receipt.Folio) + "  ",
		TrackingCode: "  " + receipt.TrackingCode + "  ",
	})
	if err != nil {
		t.Fatalf("LookupPublicRightsRequest error = %v", err)
	}
	if status.Folio != receipt.Folio || status.State != "identidad_pendiente" || len(status.Events) != 1 {
		t.Fatalf("unexpected lookup status: %+v", status)
	}

	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"usuario_id", "requester_email", "solicitante_correo", "actor_id", "camila@ejemplo.cl"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public status leaked %q: %s", forbidden, encoded)
		}
	}

	// Non-existent folio and wrong tracking code must both return ErrNotFound uniformly.
	if _, err := svc.LookupPublicRightsRequest(context.Background(), PublicRightsLookupInput{
		Folio:        "PRIV-2026-DEADBEEF",
		TrackingCode: receipt.TrackingCode,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown folio err=%v, want ErrNotFound", err)
	}
	wrongCode := strings.Repeat("0", len(receipt.TrackingCode))
	if wrongCode == receipt.TrackingCode {
		wrongCode = strings.Repeat("1", len(receipt.TrackingCode))
	}
	if _, err := svc.LookupPublicRightsRequest(context.Background(), PublicRightsLookupInput{
		Folio:        receipt.Folio,
		TrackingCode: wrongCode,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong tracking code err=%v, want ErrNotFound", err)
	}

	// Malformed inputs fail validation with ErrInvalid.
	for _, bad := range []PublicRightsLookupInput{
		{Folio: "INVALID", TrackingCode: receipt.TrackingCode},
		{Folio: receipt.Folio, TrackingCode: "short"},
		{Folio: receipt.Folio, TrackingCode: "has spaces inside token"},
		{Folio: receipt.Folio, TrackingCode: "has\x00nulinside123456"},
	} {
		if _, err := svc.LookupPublicRightsRequest(context.Background(), bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad lookup %+v: err=%v, want ErrInvalid", bad, err)
		}
	}
}
