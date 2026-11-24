package commercialdocument

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
)

func documentFixture(t *testing.T) (Source, []commercialproposal.EvidenceParty, commercialproposal.Cipher) {
	t.Helper()
	now := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	record := commercialproposal.Record{ID: "10000000-0000-4000-8000-000000000001", OwnerID: "20000000-0000-4000-8000-000000000002", RenterID: "30000000-0000-4000-8000-000000000003", Version: 1, CreatedAt: now.Add(-time.Hour), Terms: commercialproposal.Terms{SpaceID: "40000000-0000-4000-8000-000000000004", QuoteID: "50000000-0000-4000-8000-000000000005", SpaceTitle: "Sala Ñuñoa", StartAt: now.Add(24 * time.Hour), EndAt: now.Add(25 * time.Hour), TimeZone: "America/Santiago", Location: commercialproposal.Location{Address: "Avenida Sintética 123", Latitude: -33.45, Longitude: -70.66}, RateUnit: "hora", UnitPriceCLP: 10000, Units: 1, SubtotalCLP: 10000, Currency: "CLP", Sections: []contracttemplate.Section{{Key: "uso", Title: "Reglas de uso", Body: "Respetar el horario y la capacidad acordados."}}, CancellationPolicy: commercialproposal.CancellationPolicy{Version: "cancelacion-v1", Text: "Política de cancelación sintética."}}}
	parties := []commercialproposal.EvidenceParty{{Role: "arrendador", SubjectID: record.OwnerID, LegalName: "José Muñoz", RUT: "11111111-1"}, {Role: "arrendatario", SubjectID: record.RenterID, LegalName: "María Pérez", RUT: "22222222-2"}}
	full, _ := json.Marshal(struct {
		FormatVersion int                                `json:"format_version"`
		Terms         commercialproposal.Terms           `json:"terms"`
		Parties       []commercialproposal.EvidenceParty `json:"parties"`
	}{1, record.Terms, parties})
	record.SHA256 = commercialproposal.Digest(full)
	cipher, err := declarationcrypto.New("local-v1", map[string][]byte{"local-v1": bytes.Repeat([]byte{3}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	private, _ := json.Marshal(parties)
	record.KeyVersion, record.Ciphertext, err = cipher.Seal(commercialproposal.AAD(record), private)
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Proposal: record, Currency: "CLP", AmountCLP: 10000, VerifiedAt: now, ProviderPaymentID: "123456", Signatures: []Signature{{Role: "arrendador", ActorID: record.OwnerID, SessionID: "60000000-0000-4000-8000-000000000006", ConsentVersion: "commercial-agreement-v1", SignedAt: now.Add(-10 * time.Minute)}, {Role: "arrendatario", ActorID: record.RenterID, SessionID: "70000000-0000-4000-8000-000000000007", ConsentVersion: "commercial-agreement-v1", SignedAt: now.Add(-20 * time.Minute)}}}
	return source, parties, cipher
}

type documentFakeRepo struct {
	source          Source
	job             Job
	ready           bool
	failCode        string
	terminal        bool
	completionError error
	artifact        Artifact
}

func (f *documentFakeRepo) Claim(context.Context, time.Time, time.Duration) (Job, bool, error) {
	return f.job, !f.ready, nil
}
func (f *documentFakeRepo) Load(context.Context, Job) (Source, error) { return f.source, nil }
func (f *documentFakeRepo) Complete(_ context.Context, _ Job, a Artifact, _ time.Time) error {
	if f.completionError != nil {
		return f.completionError
	}
	f.artifact = a
	f.ready = true
	return nil
}
func (f *documentFakeRepo) Fail(_ context.Context, _ Job, code string, terminal bool, _ time.Time) error {
	f.failCode = code
	f.terminal = terminal
	return nil
}

type documentFakeStore struct {
	blob           []byte
	metadata       map[string]string
	writes         int
	failAfterWrite bool
	readError      error
}

func (f *documentFakeStore) PutWithMetadata(_ context.Context, _ string, blob []byte, m map[string]string) error {
	if f.blob != nil {
		return errors.New("precondition failed")
	}
	f.writes++
	f.blob = append([]byte(nil), blob...)
	f.metadata = m
	if f.failAfterWrite {
		return errors.New("uncertain remote response")
	}
	return nil
}
func (f *documentFakeStore) GetWithMetadata(context.Context, string, int64) ([]byte, map[string]string, error) {
	return f.blob, f.metadata, f.readError
}
func documentProcessor(t *testing.T) (*Processor, *documentFakeRepo, *documentFakeStore) {
	t.Helper()
	source, _, cipher := documentFixture(t)
	job := Job{ID: "80000000-0000-4000-8000-000000000008", OperationID: "80000000-0000-4000-8000-000000000008", RequestID: "90000000-0000-4000-8000-000000000009", ProposalID: source.Proposal.ID, Version: 1, DocumentSHA256: source.Proposal.SHA256, ObjectID: "10000000-0000-4000-8000-000000000011", Attempts: 1, LeaseToken: "10000000-0000-4000-8000-000000000012"}
	repo := &documentFakeRepo{source: source, job: job}
	store := &documentFakeStore{}
	return &Processor{Repo: repo, Store: store, Cipher: cipher, Renderer: PDFRenderer{}, Bucket: "espacigo-evidence-local", Now: func() time.Time { return source.VerifiedAt.Add(time.Minute) }}, repo, store
}
func TestDocumentJobRecoversUncertainWriteAndDBCrashWithoutReplacingObject(t *testing.T) {
	p, repo, store := documentProcessor(t)
	store.failAfterWrite = true
	repo.completionError = errors.New("synthetic database unavailable")
	if _, err := p.RunOnce(context.Background()); err == nil || store.writes != 1 {
		t.Fatalf("first run err=%v writes=%d", err, store.writes)
	}
	original := append([]byte(nil), store.blob...)
	repo.completionError = nil
	repo.job.Attempts = 2
	if _, err := p.RunOnce(context.Background()); err != nil || !repo.ready || store.writes != 1 || !bytes.Equal(original, store.blob) {
		t.Fatalf("recovery ready=%v writes=%d err=%v", repo.ready, store.writes, err)
	}
	if repo.artifact.Size != int64(len(original)) || repo.artifact.SHA256 == "" {
		t.Fatal("missing persisted artifact evidence")
	}
}
func TestDocumentJobNeverOverwritesConflictOrIgnoresTamperedEvidence(t *testing.T) {
	p, repo, store := documentProcessor(t)
	store.blob = []byte("conflicting private object")
	_, err := p.RunOnce(context.Background())
	if err != nil || repo.ready || repo.failCode != "object_conflict" || !repo.terminal || store.writes != 0 {
		t.Fatalf("conflict code=%s terminal=%v err=%v", repo.failCode, repo.terminal, err)
	}
	p, repo, store = documentProcessor(t)
	repo.source.Proposal.Ciphertext[0] ^= 1
	_, err = p.RunOnce(context.Background())
	if err != nil || repo.failCode != "evidence_unavailable" || !repo.terminal || store.writes != 0 {
		t.Fatalf("tamper code=%s err=%v", repo.failCode, err)
	}
}

func TestDocumentJobRetriesStorageReadFailureAndThenCompletes(t *testing.T) {
	p, repo, store := documentProcessor(t)
	store.readError = errors.New("temporary storage outage")
	if _, err := p.RunOnce(context.Background()); err != nil || repo.failCode != "storage_failed" || repo.terminal {
		t.Fatalf("temporary read failure code=%s terminal=%v err=%v", repo.failCode, repo.terminal, err)
	}
	store.readError = nil
	repo.job.Attempts = 2
	if _, err := p.RunOnce(context.Background()); err != nil || !repo.ready {
		t.Fatalf("retry did not complete ready=%v err=%v", repo.ready, err)
	}
}
func TestPDFRendererProducesDeterministicMultipageUnicodeWithoutTruncation(t *testing.T) {
	source, parties, _ := documentFixture(t)
	var clauses strings.Builder
	for i := 0; i < 200; i++ {
		clauses.WriteString("Cláusula íntegra: áéíóú ñ ü; derechos y obligaciones de ambas partes.\n")
	}
	clauses.WriteString("FIN ÚLTIMA CLÁUSULA CONSERVADA")
	source.Proposal.Terms.Sections[0].Body = clauses.String()
	first, err := (PDFRenderer{}).Render(context.Background(), source, parties)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (PDFRenderer{}).Render(context.Background(), source, parties)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("PDF is not deterministic across retries")
	}
	if !bytes.HasPrefix(first, []byte("%PDF-1.7")) || len(first) > MaxPDFBytes {
		t.Fatal("invalid PDF bounds")
	}
	path := filepath.Join(t.TempDir(), "contrato.pdf")
	if output := os.Getenv("COMMERCIAL_PDF_SAMPLE_PATH"); output != "" {
		path = output
	}
	if err = os.WriteFile(path, first, 0600); err != nil {
		t.Fatal(err)
	}
	if tool, e := exec.LookPath("pdftotext"); e == nil {
		out, e := exec.Command(tool, "-layout", path, "-").Output()
		if e != nil {
			t.Fatal(e)
		}
		for _, expected := range []string{"José Muñoz", "María Pérez", "FIN ÚLTIMA CLÁUSULA CONSERVADA", "CONFIRMACIÓN DE PAGO", "123456"} {
			if !strings.Contains(string(out), expected) {
				t.Fatalf("Unicode or final content lost: %q", expected)
			}
		}
	}
}
func TestPDFRendererRejectsUnsupportedGlyphInsteadOfCorruptingIdentity(t *testing.T) {
	source, parties, _ := documentFixture(t)
	parties[0].LegalName = "漢字"
	if _, err := (PDFRenderer{}).Render(context.Background(), source, parties); !errors.Is(err, ErrUnsupportedGlyph) {
		t.Fatalf("unsupported glyph error=%v", err)
	}
}

func TestPDFRendererStripsPostalCodeFromLocationAddress(t *testing.T) {
	withPostal, parties, _ := documentFixture(t)
	withPostal.Proposal.Terms.Location.Address = "Avenida Sintética 123, 8320000 Santiago, Chile"
	withoutPostal, _, _ := documentFixture(t)
	withoutPostal.Proposal.Terms.Location.Address = "Avenida Sintética 123, Santiago, Chile"

	pdfWithPostal, err := (PDFRenderer{}).Render(context.Background(), withPostal, parties)
	if err != nil {
		t.Fatal(err)
	}
	pdfWithoutPostal, err := (PDFRenderer{}).Render(context.Background(), withoutPostal, parties)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pdfWithPostal, pdfWithoutPostal) {
		t.Fatal("PDFRenderer did not strip 7-digit postal code from Clause 1 location address")
	}
	if tool, e := exec.LookPath("pdftotext"); e == nil {
		path := filepath.Join(t.TempDir(), "contrato-sin-postal.pdf")
		if err = os.WriteFile(path, pdfWithPostal, 0600); err != nil {
			t.Fatal(err)
		}
		out, e := exec.Command(tool, "-layout", path, "-").Output()
		if e != nil {
			t.Fatal(e)
		}
		text := string(out)
		if !strings.Contains(text, "Avenida Sintética 123, Santiago, Chile") {
			t.Fatalf("expected stripped address in PDF text, got: %s", text)
		}
		if strings.Contains(text, "8320000") {
			t.Fatalf("unexpected postal code 8320000 in PDF text: %s", text)
		}
	}
}
