package commercialproposal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
)

const ownerID = "10000000-0000-4000-8000-000000000001"
const renterID = "10000000-0000-4000-8000-000000000002"
const quoteID = "10000000-0000-4000-8000-000000000003"
const outsiderID = "10000000-0000-4000-8000-000000000004"

type repoStub struct {
	record Record
	terms  Terms
	err    error
	writes int
	reads  int
}

func (r *repoStub) CurrentTerms(context.Context, string, rentalflow.Quote, CancellationPolicy) (Terms, error) {
	return r.terms, r.err
}
func (r *repoStub) Create(_ context.Context, _ string, _ rentalflow.Quote, record Record, gate func() error) (Record, error) {
	if err := gate(); err != nil {
		return Record{}, err
	}
	r.writes++
	record.Version = 1
	record.CreatedAt = time.Now().UTC()
	r.record = record
	return record, r.err
}
func (r *repoStub) Get(_ context.Context, actor, _ string) (Record, error) {
	r.reads++
	if actor != ownerID && actor != renterID {
		return Record{}, ErrNotFound
	}
	return r.record, r.err
}

type quoteStub struct{}

func (quoteStub) CreateValidatedQuote(context.Context, string, rentalflow.Listing, rentalflow.Interval, int64, int64, time.Time) (rentalflow.Quote, error) {
	panic("not used")
}
func (quoteStub) GetForParticipant(context.Context, string, string) (rentalflow.Quote, error) {
	return rentalflow.Quote{ID: quoteID, OwnerID: ownerID, RenterID: renterID}, nil
}

type previewStub struct {
	err   error
	calls int
}

func (p *previewStub) Preview(context.Context, string, string) (rentalflow.ContractPreview, error) {
	p.calls++
	return rentalflow.ContractPreview{}, p.err
}

type gateStub struct {
	err   error
	calls int
}

func (g *gateStub) RequireForBooking(context.Context, string, string, time.Time) error {
	g.calls++
	return g.err
}

type identityStub struct {
	calls     int
	malformed bool
}

func (i *identityStub) CurrentForRentalContract(_ context.Context, _, _, subject string, _ time.Time) (rentalflow.ContractIdentity, error) {
	i.calls++
	name, rut := "Propietario Privado", "11111111-1"
	if subject == renterID {
		name = "Arrendataria Privada"
		rut = "22222222-2"
	}
	if i.malformed {
		rut = "11111111-2"
	}
	return rentalflow.ContractIdentity{SubjectID: subject, LegalName: name, RUT: rut}, nil
}
func fixture(t *testing.T) (*Service, *repoStub, *previewStub, *gateStub, *identityStub) {
	t.Helper()
	keys, err := declarationcrypto.New("local-v1", map[string][]byte{"local-v1": []byte(strings.Repeat("x", 32))})
	if err != nil {
		t.Fatal(err)
	}
	policy := CancellationPolicy{Version: "synthetic-v1", Text: "Política sintética exclusiva de pruebas."}
	r := &repoStub{terms: Terms{QuoteID: quoteID, SpaceID: "10000000-0000-4000-8000-000000000005", TemplateID: "10000000-0000-4000-8000-000000000006", TemplateVersion: 1, Sections: []contracttemplate.Section{{Key: "uso", Title: "Uso", Body: "Respetar horarios."}}, CancellationPolicy: policy}}
	p, g, i := &previewStub{}, &gateStub{}, &identityStub{}
	return New(r, quoteStub{}, p, g, i, keys, policy, time.Now), r, p, g, i
}
func TestCommercialProposalEncryptsEvidenceAndRedactsBothDirections(t *testing.T) {
	s, r, _, gate, identities := fixture(t)
	v, err := s.Create(context.Background(), renterID, quoteID)
	if err != nil {
		t.Fatal(err)
	}
	if identities.calls != 2 || gate.calls != 1 || r.writes != 1 {
		t.Fatalf("gate/reads/writes %d/%d/%d", gate.calls, identities.calls, r.writes)
	}
	if strings.Contains(string(r.record.Ciphertext), "Privad") || strings.Contains(string(r.record.Ciphertext), "22222222") {
		t.Fatal("identity evidence not encrypted")
	}
	check := func(view View, actor string) {
		t.Helper()
		blob, _ := json.Marshal(view)
		text := string(blob)
		for _, forbidden := range []string{ownerID, renterID, "11111111-1", "22222222-2", "ciphertext", "key_version", "subject_id"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("private evidence leaked %s", forbidden)
			}
		}
		if actor == renterID && strings.Contains(text, "Propietario Privado") {
			t.Fatal("counterpart legal name leaked")
		}
		if actor == ownerID && strings.Contains(text, "Arrendataria Privada") {
			t.Fatal("counterpart legal name leaked")
		}
		if len(view.Parties) != 2 || view.ContactNotice == "" {
			t.Fatal("missing redacted parties or notice")
		}
	}
	check(v, renterID)
	other, err := s.Get(context.Background(), ownerID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(other, ownerID)
	if _, err = s.Get(context.Background(), outsiderID, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider access: %v", err)
	}
}
func TestCommercialProposalFailsClosedOnKeysPolicyGateAndMalformedIdentity(t *testing.T) {
	for _, name := range []string{"keys", "policy", "preview gate", "locked gate", "identity"} {
		t.Run(name, func(t *testing.T) {
			s, r, p, g, i := fixture(t)
			switch name {
			case "keys":
				s.cipher = nil
			case "policy":
				s.policy = CancellationPolicy{}
			case "preview gate":
				p.err = rentalflow.ErrIdentityRequired
			case "locked gate":
				g.err = rentalflow.ErrIdentityRequired
			case "identity":
				i.malformed = true
			}
			if _, err := s.Create(context.Background(), renterID, quoteID); err == nil || r.writes != 0 {
				t.Fatalf("created without required dependency %v writes=%d", err, r.writes)
			}
		})
	}
}
func TestCommercialProposalRejectsCiphertextTermsOrDigestTampering(t *testing.T) {
	for _, name := range []string{"ciphertext", "terms", "digest", "id"} {
		t.Run(name, func(t *testing.T) {
			s, r, _, _, _ := fixture(t)
			v, err := s.Create(context.Background(), renterID, quoteID)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "ciphertext":
				r.record.Ciphertext[len(r.record.Ciphertext)-1] ^= 1
			case "terms":
				r.record.Terms.Location.Address = "Changed address"
			case "digest":
				r.record.SHA256 = strings.Repeat("0", 64)
			case "id":
				r.record.ID = outsiderID
			}
			if _, err = s.Get(context.Background(), renterID, v.ID); !errors.Is(err, ErrEvidenceUnavailable) {
				t.Fatalf("tampered record opened: %v", err)
			}
		})
	}
}
