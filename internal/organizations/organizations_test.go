package organizations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
)

type testFingerprinter struct{}

func (testFingerprinter) Fingerprints(string) (verification.RutFingerprintSet, error) {
	return verification.RutFingerprintSet{Active: "v2:" + repeatHex('a'), Candidates: []string{"v1:" + repeatHex('b'), "v2:" + repeatHex('a')}}, nil
}
func repeatHex(c byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

type testRepository struct {
	active             string
	candidates         []string
	createErr          error
	role               Role
	joinCandidates     []string
	joinRequest        JoinRequest
	approvedInvitation Invitation
}

func (r *testRepository) ListMine(context.Context, string) ([]Organization, error) { return nil, nil }
func (r *testRepository) Create(_ context.Context, _ string, _ string, _ string, active string, candidates []string) (Organization, error) {
	r.active = active
	r.candidates = candidates
	return Organization{}, r.createErr
}
func (r *testRepository) Get(context.Context, string, string) (Organization, error) {
	return Organization{}, nil
}
func (r *testRepository) Update(context.Context, string, string, string, string) (Organization, error) {
	return Organization{}, nil
}
func (r *testRepository) Members(context.Context, string, string) ([]Member, error) { return nil, nil }
func (r *testRepository) Invite(context.Context, string, string, string, Role, string, []byte, time.Time) (Invitation, error) {
	return Invitation{}, nil
}
func (r *testRepository) Accept(context.Context, string, string, string, []byte, time.Time) (Member, error) {
	return Member{}, nil
}
func (r *testRepository) Revoke(context.Context, string, string, string, time.Time) error { return nil }
func (r *testRepository) SetRole(context.Context, string, string, string, Role, time.Time) error {
	return nil
}
func (r *testRepository) Role(context.Context, string, string) (Role, error) { return r.role, nil }
func (r *testRepository) RequestJoin(_ context.Context, _ string, c []string, role Role, now time.Time) (JoinRequest, error) {
	r.joinCandidates = c
	r.joinRequest = JoinRequest{ID: "00000000-0000-4000-8000-000000000003", Role: role, State: "pendiente", ExpiresAt: now.Add(time.Hour)}
	return r.joinRequest, nil
}
func (r *testRepository) OwnJoinRequest(context.Context, string, string) (JoinRequest, error) {
	return r.joinRequest, nil
}
func (r *testRepository) JoinRequests(context.Context, string, string) ([]JoinRequest, error) {
	return []JoinRequest{r.joinRequest}, nil
}
func (r *testRepository) ApproveJoinRequest(context.Context, string, string, string, []byte, time.Time, time.Time) (Invitation, error) {
	return r.approvedInvitation, nil
}
func (r *testRepository) RejectJoinRequest(context.Context, string, string, string, time.Time) error {
	return nil
}

func TestCreateReservesEveryFingerprintVersion(t *testing.T) {
	repo := &testRepository{}
	svc, err := New(repo, testFingerprinter{}, func() time.Time { return time.Unix(1, 0) })
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(context.Background(), "00000000-0000-4000-8000-000000000001", "Empresa SpA", "Empresa", "12.345.678-5")
	if err != nil {
		t.Fatal(err)
	}
	if repo.active != "v2:"+repeatHex('a') || len(repo.candidates) != 2 || repo.candidates[0] != "v1:"+repeatHex('b') {
		t.Fatalf("did not reserve rotation candidates: %#v", repo.candidates)
	}
}
func TestCreateReturnsDuplicateRutConflict(t *testing.T) {
	repo := &testRepository{createErr: ErrConflict}
	svc, _ := New(repo, testFingerprinter{}, time.Now)
	_, err := svc.Create(context.Background(), "00000000-0000-4000-8000-000000000001", "Empresa SpA", "", "12.345.678-5")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
}
func TestAuthorizationRequiresLegalRepresentativeForContractSigning(t *testing.T) {
	repo := &testRepository{role: Administrator}
	svc, _ := New(repo, testFingerprinter{}, time.Now)
	if _, err := svc.Authorize(context.Background(), "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", ActionSignContract); !errors.Is(err, ErrForbidden) {
		t.Fatalf("administrator must not sign contracts, got %v", err)
	}
	repo.role = LegalRepresentative
	if role, err := svc.Authorize(context.Background(), "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", ActionSignContract); err != nil || role != LegalRepresentative {
		t.Fatalf("active representative should sign: role=%q err=%v", role, err)
	}
}

func TestJoinRequestUsesEveryRutFingerprintAndDoesNotGrantMembership(t *testing.T) {
	repo := &testRepository{role: LegalRepresentative, approvedInvitation: Invitation{ID: "00000000-0000-4000-8000-000000000004", State: "pendiente", Role: Operator}}
	svc, _ := New(repo, testFingerprinter{}, func() time.Time { return time.Unix(10, 0) })
	request, err := svc.RequestJoin(context.Background(), "00000000-0000-4000-8000-000000000001", "12.345.678-5", Operator)
	if err != nil {
		t.Fatal(err)
	}
	if request.State != "pendiente" || len(repo.joinCandidates) != 2 {
		t.Fatalf("request=%+v candidates=%v", request, repo.joinCandidates)
	}
	inv, token, err := svc.ApproveJoinRequest(context.Background(), "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000005", request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.State != "pendiente" || token == "" {
		t.Fatalf("approval must yield invitation, not member: %+v token-empty=%t", inv, token == "")
	}
}
