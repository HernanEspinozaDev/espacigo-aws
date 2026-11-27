package kyb

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
)

const (
	testOwnerID        = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testOrganizationID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testVerificationID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	testDocumentID1    = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	testStorageRef1    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	testDocumentID2    = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	testStorageRef2    = "11111111-1111-4111-8111-111111111111"
	testDocumentID3    = "22222222-2222-4222-8222-222222222222"
	testStorageRef3    = "33333333-3333-4333-8333-333333333333"
)

type testIDs struct{ values []string }

func (g *testIDs) ID() (string, error) {
	value := g.values[0]
	g.values = g.values[1:]
	return value, nil
}

type testDeclarationKeys struct{ key *declarationcrypto.Keyring }

func (k testDeclarationKeys) Seal(aad, plaintext []byte) (string, []byte, error) {
	return k.key.Seal(aad, plaintext)
}
func (k testDeclarationKeys) Open(version string, aad, ciphertext []byte) ([]byte, error) {
	return k.key.Open(version, aad, ciphertext)
}

type testRutKeys struct{ keyring *verification.RutHMACKeyring }

func (k testRutKeys) Fingerprints(value string) (verification.RutFingerprintSet, error) {
	return k.keyring.Fingerprints(value)
}

type testDocuments struct{ objects map[string][]byte }

func (s *testDocuments) Put(_ context.Context, _, _, _, ref string, content []byte) error {
	s.objects[ref] = append([]byte(nil), content...)
	return nil
}
func (s *testDocuments) Read(context.Context, string, string, string, string) ([]byte, error) {
	return nil, nil
}
func (s *testDocuments) Delete(_ context.Context, _, _, _, ref string) error {
	delete(s.objects, ref)
	return nil
}
func (s *testDocuments) SignReviewAccess(string, int64) (string, string, error) {
	return "v1", "signature", nil
}
func (s *testDocuments) VerifyReviewAccess(string, int64, string, string, time.Time) bool {
	return true
}

type testRepository struct {
	item      Verification
	documents []Document
	creates   int
	decisions []DocumentDecision
	decisionStatus Status
	decisionReason string
}

func (r *testRepository) GetProposalContractIdentity(_ context.Context, organizationID string, _ time.Time) (Verification, string, error) {
	item := r.item
	item.OrganizationID = organizationID
	item.Status = StatusApproved
	return item, testOwnerID, nil
}

func (r *testRepository) GetOrCreateOrganization(context.Context, string, string) (string, error) {
	return testOrganizationID, nil
}
func (r *testRepository) Create(_ context.Context, item Verification, documents []Document) (Verification, bool, error) {
	r.creates++
	r.item = item
	r.documents = append([]Document(nil), documents...)
	item.Documents = append([]Document(nil), documents...)
	return item, false, nil
}
func (r *testRepository) GetOwn(context.Context, string) (Verification, error) {
	return r.item, nil
}
func (r *testRepository) ListPending(context.Context) ([]Verification, error) { return nil, nil }
func (r *testRepository) GetForReview(context.Context, string) (Verification, error) {
	item:=r.item;item.Documents=append([]Document(nil),r.documents...);if len(item.Documents)==0{item.Documents=append([]Document(nil),r.item.Documents...)};return item, nil
}
func (r *testRepository) GetDocument(context.Context, string, string) (Document, Verification, error) {
	return Document{}, r.item, nil
}
func (r *testRepository) GetDocumentByStorageRef(context.Context, string) (Document, Verification, error) {
	return Document{}, r.item, nil
}
func (r *testRepository) StartReview(context.Context, string, string, time.Time) (Verification, error) {
	return r.item, nil
}
func (r *testRepository) Decide(context.Context, string, string, Status, string, time.Time) (Verification, error) {
	return r.item, nil
}
func(r *testRepository)DecideDocuments(_ context.Context,_ string,_ string,status Status,reason string,decisions []DocumentDecision,_ time.Time)(Verification,error){r.decisionStatus=status;r.decisionReason=reason;r.decisions=append([]DocumentDecision(nil),decisions...);return r.item,nil}

func TestSubmitEncryptsRUTsAndStoresRequiredDocuments(t *testing.T) {
	encryptionKey := bytes.Repeat([]byte{0x42}, 32)
	keyring, err := declarationcrypto.New("v1", map[string][]byte{"v1": encryptionKey})
	if err != nil {
		t.Fatal(err)
	}
	rutKeyring, err := verification.NewRutHMACKeyring("h1", map[string][]byte{"h1": bytes.Repeat([]byte{0x31}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	repository := &testRepository{}
	documents := &testDocuments{objects: map[string][]byte{}}
	ids := &testIDs{values: []string{
		"44444444-4444-4444-8444-444444444444", testVerificationID,
		testDocumentID1, testStorageRef1, testDocumentID2,
		testStorageRef2, testDocumentID3, testStorageRef3,
	}}
	service, err := NewService(repository, documents, testDeclarationKeys{keyring}, testRutKeys{rutKeyring}, ids, func() time.Time {
		return time.Date(2026, time.November, 24, 10, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.4\nsynthetic test document")
	input := SubmissionInput{
		LegalName: "Empresa Sintética SpA", CompanyRUT: "76.123.456-0",
		RepresentativeName: "Representante Sintético", RepresentativeRUT: "12.345.678-5",
		SIIGiroCode: "620100", IdempotencyKey: "kyb-submit-test-01",
		Documents: []Upload{
			{Kind: DocumentERUT, MIME: "application/pdf", Content: pdf},
			{Kind: DocumentPowerCertificate, MIME: "application/pdf", Content: pdf},
			{Kind: DocumentPropertyOrSublease, MIME: "application/pdf", Content: pdf},
		},
	}
	result, err := service.Submit(context.Background(), testOwnerID, input)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != StatusPending || result.OrganizationID != testOrganizationID || len(result.Documents) != 3 || repository.creates != 1 {
		t.Fatalf("unexpected KYB submission result: %+v repository=%+v", result, repository)
	}
	if bytes.Contains(repository.item.CompanyRUTCiphertext, []byte("76123456-0")) || bytes.Contains(repository.item.RepresentativeRUTCipher, []byte("12345678-5")) {
		t.Fatal("RUT ciphertext contains canonical plaintext")
	}
	if repository.item.CompanyRUTFingerprint == "" || repository.item.RepresentativeRUTFprint == "" || len(documents.objects) != 3 {
		t.Fatal("submission did not persist HMAC fingerprints and three private documents")
	}
	contractIdentity, err := service.CurrentOrganizationContractIdentityForProposal(context.Background(), testOrganizationID, time.Now())
	if err != nil || contractIdentity.LegalName != "Empresa Sintética SpA" || contractIdentity.CompanyRUT != "76123456-0" || contractIdentity.RepresentativeRUT != "12345678-5" || contractIdentity.RepresentativeUserID != testOwnerID {
		t.Fatalf("proposal signing identity was not resolved from approved KYB: %+v err=%v", contractIdentity, err)
	}
	review, err := service.ForReview(context.Background(), result.ID)
	if err != nil || review.CompanyRUT != "76123456-0" || review.RepresentativeRUT != "12345678-5" {
		t.Fatalf("admin review RUT projection=%+v err=%v", review, err)
	}
	ownerView, err := service.Own(context.Background(), testOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ownerView)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "76123456-0") || strings.Contains(string(encoded), "12345678-5") {
		t.Fatal("owner status response unexpectedly includes plaintext RUT fields")
	}
}

func TestDecideDocumentsValidatesExactPacketAndDerivesAggregate(t *testing.T){
	repo:=&testRepository{item:Verification{ID:testVerificationID,Status:StatusInReview,Documents:[]Document{{ID:testDocumentID1,Kind:DocumentERUT},{ID:testDocumentID2,Kind:DocumentPowerCertificate}}}}
	svc:=&Service{repository:repo,now:func()time.Time{return time.Date(2026,time.November,27,0,0,0,0,time.UTC)}}
	decisions:=[]DocumentDecision{{DocumentID:testDocumentID1,Status:StatusApproved},{DocumentID:testDocumentID2,Status:StatusRejected,RejectionReason:"Vigencia vencida"}}
	if _,err:=svc.DecideWithDocuments(context.Background(),testVerificationID,testOwnerID,false,decisions);err!=nil{t.Fatal(err)}
	if repo.decisionStatus!=StatusRejected||len(repo.decisions)!=2||repo.decisionReason==""{t.Fatalf("bad derived packet decision: status=%q decisions=%+v reason=%q",repo.decisionStatus,repo.decisions,repo.decisionReason)}
	if _,err:=svc.DecideWithDocuments(context.Background(),testVerificationID,testOwnerID,true,decisions);err==nil{t.Fatal("aggregate packet status mismatch should fail")}
	if _,err:=svc.DecideWithDocuments(context.Background(),testVerificationID,testOwnerID,false,decisions[:1]);err==nil{t.Fatal("missing document decision should fail")}
	duplicate:=[]DocumentDecision{decisions[0],decisions[0]};if _,err:=svc.DecideWithDocuments(context.Background(),testVerificationID,testOwnerID,true,duplicate);err==nil{t.Fatal("duplicate document decision should fail")}
	unknown:=[]DocumentDecision{decisions[0],{DocumentID:testDocumentID3,Status:StatusApproved}};if _,err:=svc.DecideWithDocuments(context.Background(),testVerificationID,testOwnerID,false,unknown);err==nil{t.Fatal("unknown document should fail")}
}

func TestSubmitRejectsInvalidRUTAndDocumentSetBeforePersistence(t *testing.T) {
	keyring, _ := declarationcrypto.New("v1", map[string][]byte{"v1": bytes.Repeat([]byte{0x42}, 32)})
	rutKeyring, _ := verification.NewRutHMACKeyring("h1", map[string][]byte{"h1": bytes.Repeat([]byte{0x31}, 32)})
	repository := &testRepository{}
	store := &testDocuments{objects: map[string][]byte{}}
	service, err := NewService(repository, store, testDeclarationKeys{keyring}, testRutKeys{rutKeyring}, &testIDs{values: []string{testVerificationID}}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	input := SubmissionInput{
		LegalName: "Empresa Sintética SpA", CompanyRUT: "76.123.456-1",
		RepresentativeName: "Representante Sintético", RepresentativeRUT: "12.345.678-5",
		SIIGiroCode: "620100", IdempotencyKey: "kyb-submit-test-02",
		Documents: []Upload{{Kind: DocumentERUT, MIME: "application/pdf", Content: []byte("%PDF-1.4\n")}},
	}
	if _, err := service.Submit(context.Background(), testOwnerID, input); err == nil {
		t.Fatal("invalid company RUT or incomplete document set was accepted")
	}
	if repository.creates != 0 || len(store.objects) != 0 {
		t.Fatal("invalid request reached persistence or object storage")
	}
}

func TestCompanyDocumentSetAcceptsSIIAndRetainsLegacySubmission(t *testing.T) {
	pdf := []byte("%PDF-1.4\nsynthetic")
	base := SubmissionInput{LegalName: "Empresa Sintética SpA", CompanyRUT: "76.123.456-0", RepresentativeName: "Representante Sintético", RepresentativeRUT: "12.345.678-5", SIIGiroCode: "620100", IdempotencyKey: "test"}
	legacy := base
	legacy.Documents = []Upload{{Kind: DocumentERUT, MIME: "application/pdf", Content: pdf}, {Kind: DocumentPowerCertificate, MIME: "application/pdf", Content: pdf}, {Kind: DocumentPropertyOrSublease, MIME: "application/pdf", Content: pdf}}
	if err := validateSubmission(legacy); err != nil {
		t.Fatalf("legacy submission: %v", err)
	}
	modern := base
	modern.OrganizationID = testOrganizationID
	modern.Documents = []Upload{{Kind: DocumentERUT, MIME: "application/pdf", Content: pdf}, {Kind: DocumentSIIStartActivities, MIME: "application/pdf", Content: pdf}, {Kind: DocumentPowerCertificate, MIME: "application/pdf", Content: pdf}}
	if err := validateSubmission(modern); err != nil {
		t.Fatalf("SII company submission: %v", err)
	}
}

type complianceTestRepo struct {
	*testRepository
	scopeOwner  string
	scopeOrg    string
	submitted   ComplianceDocument
	submitErr   error
}

func (*complianceTestRepo) SubmitPersonalSII(context.Context, ComplianceDocument) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (*complianceTestRepo) PersonalCompliance(context.Context, string) (ComplianceProfile, error) {
	return ComplianceProfile{}, nil
}
func (*complianceTestRepo) SubmitSpaceProperty(context.Context, ComplianceDocument) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (r *complianceTestRepo) SubmitSpaceCategorySII(_ context.Context, item ComplianceDocument) (ComplianceDocument, error) {
	r.submitted = item
	if r.submitErr != nil {
		return ComplianceDocument{}, r.submitErr
	}
	item.CategoryCode = "bodega"
	return item, nil
}
func (r *complianceTestRepo) SpaceStorageScope(context.Context, string, string) (string, string, error) {
	return r.scopeOwner, r.scopeOrg, nil
}
func (*complianceTestRepo) SpaceEligibility(context.Context, string, string, time.Time) (SpaceEligibility, error) {
	return SpaceEligibility{}, nil
}
func (*complianceTestRepo) ListCompliance(context.Context, string) ([]ComplianceProfile, error) {
	return nil, nil
}
func (*complianceTestRepo) SetComplianceDisabled(context.Context, string, string, string, time.Time) error {
	return nil
}
func (*complianceTestRepo) SetOrganizationDisabled(context.Context, string, string, string, time.Time) error {
	return nil
}
func (*complianceTestRepo) ListPendingComplianceDocuments(context.Context) ([]ComplianceDocument, error) {
	return nil, nil
}
func (*complianceTestRepo) GetComplianceDocument(context.Context, string) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (*complianceTestRepo) GetComplianceDocumentByStorageRef(context.Context, string) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (*complianceTestRepo) StartComplianceReview(context.Context, string, string, time.Time) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (*complianceTestRepo) DecideComplianceDocument(context.Context, string, string, Status, string, time.Time) (ComplianceDocument, error) {
	return ComplianceDocument{}, nil
}
func (*complianceTestRepo) ExpireNoncompliant(context.Context, time.Time) (int, error) { return 0, nil }

func TestSubmitSpaceCategorySIIValidatesAndStoresUnderOrganization(t *testing.T) {
	docs := &testDocuments{objects: map[string][]byte{}}
	repo := &complianceTestRepo{testRepository: &testRepository{}, scopeOwner: testOwnerID, scopeOrg: testOrganizationID}
	ids := &testIDs{values: []string{testDocumentID1, testStorageRef1, testDocumentID2, testStorageRef2}}
	svc := &Service{
		repository: repo,
		documents:  docs,
		ids:        ids,
		now:        func() time.Time { return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC) },
	}
	pdf := []byte("%PDF-1.4\ncategory sii")
	upload := Upload{Kind: DocumentSIIStartActivities, MIME: "application/pdf", Content: pdf}

	created, err := svc.SubmitSpaceCategorySII(context.Background(), testOwnerID, testVerificationID, "521020", upload)
	if err != nil {
		t.Fatalf("SubmitSpaceCategorySII: %v", err)
	}
	if created.Kind != DocumentSIIStartActivities || created.GiroCode != "521020" || created.CategoryCode != "bodega" || len(docs.objects) != 1 {
		t.Fatalf("unexpected created category SII: %+v objects=%d", created, len(docs.objects))
	}

	if _, err := svc.SubmitSpaceCategorySII(context.Background(), testOwnerID, testVerificationID, "bad", upload); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for invalid giro, got %v", err)
	}

	repo.scopeOrg = ""
	if _, err := svc.SubmitSpaceCategorySII(context.Background(), testOwnerID, testVerificationID, "521020", upload); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for personal space, got %v", err)
	}
	if len(docs.objects) != 1 {
		t.Fatalf("personal space rejection should not write to object store, got %d objects", len(docs.objects))
	}
}
