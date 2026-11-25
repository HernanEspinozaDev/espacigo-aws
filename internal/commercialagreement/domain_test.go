package commercialagreement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const owner = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const renter = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
const requestID = "11111111-1111-4111-8111-111111111111"
const proposalID = "22222222-2222-4222-8222-222222222222"
const sessionID = "33333333-3333-4333-8333-333333333333"

type probeRepository struct {
	Repository
	calls int
	input CreateInput
	key   string
}

func (r *probeRepository) Create(_ context.Context, _ Actor, in CreateInput, key string) (Agreement, error) {
	r.calls++
	r.input = in
	r.key = key
	return Agreement{ID: requestID}, nil
}
func TestCreateRequiresExactConsentAndAuthenticatedSession(t *testing.T) {
	in := CreateInput{ProposalID: proposalID, SHA256: strings.Repeat("a", 64), Consent: true, ConsentVersion: ConsentVersion}
	for _, tc := range []struct {
		name   string
		mutate func(*CreateInput, *Actor, *string)
	}{{"false consent", func(i *CreateInput, _ *Actor, _ *string) { i.Consent = false }}, {"wrong consent version", func(i *CreateInput, _ *Actor, _ *string) { i.ConsentVersion = "old" }}, {"wrong hash", func(i *CreateInput, _ *Actor, _ *string) { i.SHA256 = "bad" }}, {"no proposal", func(i *CreateInput, _ *Actor, _ *string) { i.ProposalID = "" }}, {"no session", func(_ *CreateInput, a *Actor, _ *string) { a.SessionID = "" }}, {"no actor", func(_ *CreateInput, a *Actor, _ *string) { a.ID = "" }}, {"empty key", func(_ *CreateInput, _ *Actor, key *string) { *key = "" }}, {"control key", func(_ *CreateInput, _ *Actor, key *string) { *key = "test\nsecret" }}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &probeRepository{}
			service, _ := New(repo)
			input := in
			actor := Actor{ID: renter, SessionID: sessionID}
			key := "intention-1"
			tc.mutate(&input, &actor, &key)
			if _, err := service.Create(context.Background(), actor, input, key); !errors.Is(err, ErrInvalid) || repo.calls != 0 {
				t.Fatalf("invalid evidence submitted: err=%v calls=%d", err, repo.calls)
			}
		})
	}
	repo := &probeRepository{}
	service, _ := New(repo)
	if _, err := service.Create(context.Background(), Actor{ID: renter, SessionID: sessionID}, in, "intention-1"); err != nil || repo.calls != 1 {
		t.Fatalf("valid create failed %v", err)
	}
}
func TestAgreementProjectionExcludesPrivateEvidence(t *testing.T) {
	at := time.Now().UTC()
	occupancyID := requestID
	a := Agreement{ID: requestID, OwnerID: owner, RenterID: renter, OccupancyID: &occupancyID, Signatures: []Signature{{Role: "arrendatario", SignedAt: at, ConsentVersion: ConsentVersion}}}
	blob, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{owner, renter, "session_id", "sesion_id", "actor_id", "ocupacion", "legal_name", "rut", "email", "phone"} {
		if strings.Contains(string(blob), private) {
			t.Fatalf("private evidence in projection: %s", blob)
		}
	}
}
func TestFingerprintBindsProposalHashAndConsentVersion(t *testing.T) {
	a := CreateInput{ProposalID: proposalID, SHA256: strings.Repeat("a", 64), Consent: true, ConsentVersion: ConsentVersion}
	first := Fingerprint(a)
	for _, mutate := range []func(*CreateInput){func(i *CreateInput) { i.ProposalID = requestID }, func(i *CreateInput) { i.SHA256 = strings.Repeat("b", 64) }, func(i *CreateInput) { i.ConsentVersion = "other" }, func(i *CreateInput) { i.Consent = false }} {
		b := a
		mutate(&b)
		if string(first) == string(Fingerprint(b)) {
			t.Fatal("intent fingerprint did not bind content")
		}
	}
}

type noticeRepository struct {
	NoticeRepository
	notice   Notice
	finished bool
	success  bool
	claimed  bool
}

func (r *noticeRepository) ClaimNotice(context.Context, time.Time) (Notice, bool, error) {
	if r.claimed {
		return Notice{}, false, nil
	}
	r.claimed = true
	return r.notice, true, nil
}
func (r *noticeRepository) FinishNotice(_ context.Context, _ Notice, success bool, _ time.Time) error {
	r.finished = true
	r.success = success
	return nil
}

type noticeSender struct {
	body string
	err  error
}

func (s *noticeSender) SendLocalBookingNotice(_ context.Context, _ string, _ string, body string) error {
	s.body = body
	return s.err
}
func TestDurableNoticeContainsOnlyRequestReferenceAndRetriesDelivery(t *testing.T) {
	for _, fail := range []bool{false, true} {
		repo := &noticeRepository{notice: Notice{ID: proposalID, AgreementID: requestID, Email: "private@example.test", Attempt: 1}}
		sender := &noticeSender{}
		if fail {
			sender.err = errors.New("smtp unavailable")
		}
		dispatcher, _ := NewNoticeDispatcher(repo, sender, time.Now)
		ok, err := dispatcher.DispatchOne(context.Background())
		if !ok || !repo.finished || repo.success == fail || (fail && err == nil) {
			t.Fatalf("delivery completion not persisted: %v %v %+v", ok, err, repo)
		}
		if strings.Contains(sender.body, repo.notice.Email) || !strings.Contains(sender.body, requestID) {
			t.Fatalf("message privacy/reference invalid: %s", sender.body)
		}
	}
}
