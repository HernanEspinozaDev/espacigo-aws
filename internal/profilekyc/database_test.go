package profilekyc_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	profilepinpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/profilepin"
	rentalflowpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identityverification"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilekyc"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type syntheticProvider struct {
	mu        sync.Mutex
	decisions map[string]profilekyc.Decision
}

func (p *syntheticProvider) Start(_ context.Context, attempt string, d profilekyc.Data, _ string) (profilekyc.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := uuid.NewString()
	p.decisions[id] = profilekyc.Decision{
		SessionID: id, AttemptID: attempt, State: "verified", FirstName: d.FirstName, LastName: d.LastName(),
		RUT: d.RUT, DateOfBirth: d.DateOfBirth, DocumentExpires: "2035-01-01", ChecksPassed: true,
	}
	return profilekyc.Session{ID: id, URL: "https://verify.didit.me/session/synthetic"}, nil
}

func (p *syntheticProvider) Decision(_ context.Context, id string) (profilekyc.Decision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.decisions[id], nil
}

type testAuthorizer struct{ account, session string }

func (a testAuthorizer) Authorize(_ context.Context, _ identity.Secret, _ identity.Role, _ identity.ActivityKind) (identity.Principal, error) {
	return identity.Principal{AccountID: a.account, SessionID: a.session}, nil
}

type testStepUp struct{}

func (testStepUp) Verify(_ context.Context, _, _ string, proof profilepin.Proof) error {
	if proof.CurrentPassword != "synthetic factor" && proof.OTPCode != "654321" {
		return profilepin.ErrStepUpRejected
	}
	return nil
}

func TestPostgresProfileVerificationAndPinpass(t *testing.T) {
	dsn := os.Getenv("ESPACIGO_PROFILE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("run against the disposable profile database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE espacigo_runtime")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	first, second, session, anotherSession := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for i, account := range []string{first, second} {
		if _, err := admin.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,estado) VALUES ($1,$2,'activo')`, account, []string{"first@example.test", "second@example.test"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, sessionID := range []string{session, anotherSession} {
		if _, err := admin.Exec(ctx, `INSERT INTO public.sesion(id,usuario_id,expira_en,ultima_actividad_en) VALUES ($1,$2,now()+interval '1 hour',now())`, sessionID, first); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := declarationcrypto.New("test", map[string][]byte{"test": make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	rutKeys, err := verification.NewRutHMACKeyring("test", map[string][]byte{"test": make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	service := profilekyc.New(pool, keys, rutKeys, time.Now)
	provider := &syntheticProvider{decisions: make(map[string]profilekyc.Decision)}
	verifier := profilekyc.NewVerifier(service, provider)
	input := profilekyc.Data{Username: "alpha-profile", FirstName: "María", FirstLastName: "Ejemplo", NoSecondLastName: true, RUT: "12.345.678-5",
		DateOfBirth: "1990-01-01", DocumentType: "cl_id", Phone: "+56912345678", Consent: true}
	if err := service.SaveDraft(ctx, first, profilekyc.Data{FirstName: "María"}); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(ctx, first)
	if err != nil || status.Complete || !status.DraftSaved {
		t.Fatalf("draft status: %#v %v", status, err)
	}
	draft, err := service.ReadDraft(ctx, first)
	if err != nil || draft.FirstName != "María" {
		t.Fatalf("draft restore: %v", err)
	}

	if status, err := service.Save(ctx, first, input); err != nil || status.State != "ready" || status.DraftSaved {
		t.Fatalf("save: %#v %v", status, err)
	}
	current, err := service.ReadForEdit(ctx, first)
	if err != nil || current.FirstName != "María" || current.Username != input.Username || current.Phone != input.Phone {
		t.Fatalf("read protected editor data: %v", err)
	}
	if _, err := service.ReadForEdit(ctx, second); !errors.Is(err, profilekyc.ErrNotReady) {
		t.Fatalf("incomplete account editor data: %v", err)
	}
	input.Username = "ALPHA-PROFILE"
	if _, err := service.Save(ctx, second, input); !errors.Is(err, profilekyc.ErrUsernameTaken) {
		t.Fatalf("case insensitive username: %v", err)
	}
	input.Username = "beta-profile"
	if _, err := service.Save(ctx, second, input); err != nil {
		t.Fatal(err)
	}

	start, err := verifier.Start(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	decision, _ := provider.Decision(ctx, start.ID)
	notice := profilekyc.Notice{ID: uuid.NewString(), SessionID: start.ID, AttemptID: decision.AttemptID, OccurredAt: time.Now()}
	if err := verifier.ProcessNotice(ctx, notice); err != nil {
		t.Fatal(err)
	}
	if err := verifier.ProcessNotice(ctx, notice); err != nil {
		t.Fatalf("duplicated event: %v", err)
	}
	status, err = service.Status(ctx, first)
	if err != nil || status.State != "verified" || !status.RUTLocked {
		t.Fatalf("verified status: %#v %v", status, err)
	}
	read, err := profilekyc.ReadVerified(ctx, pool, keys, first, time.Now())
	if err != nil || read.LegalName() != "María Ejemplo" || read.RUT != "12345678-5" {
		t.Fatalf("contract projection from current identity: %v", err)
	}

	quote := uuid.NewString()
	if _, err := admin.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial(id,propietario_id,arrendatario_id) VALUES ($1,$2,$3)`, quote, first, second); err != nil {
		t.Fatal(err)
	}
	contracts, err := rentalflowpg.NewRepository(pool, pool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	contracts.UseVerifiedIdentityProfile(keys)
	for _, tc := range []struct {
		name, account, email, firstName, surname string
	}{
		{name: "verified", account: first, email: "first@example.test", firstName: "María", surname: "Ejemplo"},
		{name: "unverified", account: second, email: "second@example.test"},
	} {
		t.Run("checkout buyer "+tc.name, func(t *testing.T) {
			buyer, err := contracts.CheckoutBuyer(ctx, tc.account)
			if err != nil || buyer.Email != tc.email || buyer.Name != tc.firstName || buyer.Surname != tc.surname {
				t.Fatalf("unexpected checkout buyer projection for %s: %v", tc.name, err)
			}
		})
	}
	party, err := contracts.CurrentForRentalContract(ctx, second, quote, first, time.Now())
	if err != nil || party.LegalName != "María Ejemplo" || party.RUT != "12345678-5" {
		t.Fatalf("contract party from KYC: %v", err)
	}
	if _, err := contracts.CurrentForRentalContract(ctx, uuid.NewString(), quote, first, time.Now()); !errors.Is(err, rentalflow.ErrNotFound) {
		t.Fatalf("contract data available outside quote participants: %v", err)
	}

	if _, err := verifier.Start(ctx, second); err != nil {
		t.Fatal(err)
	}
	if status, err := verifier.Refresh(ctx, second); err != nil || status.State != "needs_correction" {
		t.Fatalf("duplicate ownership: %#v %v", status, err)
	}
	gate, _ := identityverification.NewService(service)
	if err := gate.RequireForBooking(ctx, first, second, time.Now()); !errors.Is(err, identityverification.ErrNotVerified) {
		t.Fatalf("booking gate: %v", err)
	}

	input.Username = "alpha-profile"
	input.RUT = "6.000.000-K"
	if _, err := service.Save(ctx, first, input); !errors.Is(err, profilekyc.ErrRUTLocked) {
		t.Fatalf("locked RUN edited: %v", err)
	}
	input.RUT = ""
	if status, err := service.Save(ctx, first, input); err != nil || status.State != "verified" {
		t.Fatalf("retain verified RUN: %#v %v", status, err)
	}

	otherAccounts := []string{uuid.NewString(), uuid.NewString()}
	sessions := make([]string, len(otherAccounts))
	for i, id := range otherAccounts {
		if _, err := admin.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,estado) VALUES ($1,$2,'activo')`, id, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
		form := input
		form.Username = []string{"tercer-perfil", "cuarto-perfil"}[i]
		form.RUT = "6.000.000-K"
		if _, err := service.Save(ctx, id, form); err != nil {
			t.Fatal(err)
		}
		session, err := verifier.Start(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		sessions[i] = session.ID
	}
	var group sync.WaitGroup
	verifications := make(chan error, len(otherAccounts))
	for i := range otherAccounts {
		group.Go(func() {
			d, _ := provider.Decision(ctx, sessions[i])
			verifications <- verifier.ProcessNotice(ctx, profilekyc.Notice{
				ID: uuid.NewString(), SessionID: sessions[i], AttemptID: d.AttemptID, OccurredAt: time.Now(),
			})
		})
	}
	group.Wait()
	close(verifications)
	for err := range verifications {
		if err != nil {
			t.Fatal(err)
		}
	}
	approved := 0
	winner := -1
	for i, account := range otherAccounts {
		status, err := service.Status(ctx, account)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "verified" {
			approved++
			winner = i
		}
	}
	if approved != 1 {
		t.Fatalf("concurrent claims approved %d accounts", approved)
	}

	provider.mu.Lock()
	revocation := provider.decisions[sessions[winner]]
	revocation.State = "needs_correction"
	provider.decisions[sessions[winner]] = revocation
	provider.mu.Unlock()
	if err := verifier.ProcessNotice(ctx, profilekyc.Notice{ID: uuid.NewString(), SessionID: sessions[winner], AttemptID: revocation.AttemptID, OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if status, err := service.Status(ctx, otherAccounts[winner]); err != nil || status.State != "revoked" || !status.RUTLocked {
		t.Fatalf("revocation preserved active identity or released RUN: %#v %v", status, err)
	}
	loser := otherAccounts[1-winner]
	if _, err := verifier.Start(ctx, loser); err != nil {
		t.Fatal(err)
	}
	if status, err := verifier.Refresh(ctx, loser); err != nil || status.State != "needs_correction" {
		t.Fatalf("revoked identity RUN reused: %#v %v", status, err)
	}

	pin, err := profilepin.NewService(profilepinpg.New(pool), testStepUp{}, service, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err := pin.Set(ctx, first, session, "361909", "361909", profilepin.Proof{CurrentPassword: "synthetic factor", EmailCode: "123456"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	profilekyc.Register(mux, testAuthorizer{first, session}, verifier, nil, testStepUp{}, pin)
	for _, tc := range []struct {
		name, proof string
		status      int
	}{
		{"missing", `{}`, http.StatusForbidden},
		{"password", `{"current_password":"synthetic factor"}`, http.StatusOK},
		{"authenticator", `{"otp_code":"654321"}`, http.StatusOK},
		{"invalid PIN", `{"pin":"111111"}`, http.StatusForbidden},
		{"PIN", `{"pin":"361909"}`, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, profilekyc.BaseRoute+"/edit", strings.NewReader(tc.proof))
		req.Header.Set("Authorization", "Bearer synthetic-session")
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, req)
		if result.Code != tc.status || result.Header().Get("Cache-Control") != "no-store" ||
			strings.Contains(result.Body.String(), "alpha-profile") != (tc.status == http.StatusOK) {
			t.Fatalf("protected edit with %s: HTTP %d", tc.name, result.Code)
		}
	}
	before := time.Now()
	expiry, err := pin.Unlock(ctx, first, session, "361909")
	if err != nil || expiry.Before(before.Add(14*time.Minute)) || expiry.After(time.Now().Add(15*time.Minute)) {
		t.Fatalf("PIN grant: %v", err)
	}
	if value, expires, err := pin.Reveal(ctx, first, session, "run"); err != nil || value != "12345678-5" || !expires.Equal(expiry) {
		t.Fatalf("PIN own reveal: %v", err)
	}
	if _, _, err := pin.Reveal(ctx, first, anotherSession, "email"); !errors.Is(err, profilepin.ErrNoAccess) {
		t.Fatalf("PIN grant crossed sessions: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE public.account_pinpass_access SET created_at=now()-interval '20 minutes',
		expires_at=now()-interval '5 minutes' WHERE account_id=$1 AND session_id=$2`, first, session); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pin.Reveal(ctx, first, session, "email"); !errors.Is(err, profilepin.ErrNoAccess) {
		t.Fatalf("expired grant allowed reveal: %v", err)
	}
	if _, err := pin.Unlock(ctx, first, session, "361909"); err != nil {
		t.Fatal(err)
	}
	if err := pin.Reset(ctx, first, session, profilepin.Proof{OTPCode: "654321", EmailCode: "123456"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pin.Reveal(ctx, first, session, "run"); !errors.Is(err, profilepin.ErrNoAccess) {
		t.Fatalf("grant survived reset: %v", err)
	}
	input.FirstName = "María Paz"
	if _, err := service.Save(ctx, first, input); !errors.Is(err, profilekyc.ErrLegalIdentityLocked) {
		t.Fatalf("verified legal name was editable: %v", err)
	}
	input.FirstName = "María"
	input.DateOfBirth = "1991-01-01"
	if _, err := service.Save(ctx, first, input); !errors.Is(err, profilekyc.ErrLegalIdentityLocked) {
		t.Fatalf("verified birth date was editable: %v", err)
	}
	input.DateOfBirth = "1990-01-01"
	input.Phone = "+56987654321"
	if status, err := service.Save(ctx, first, input); err != nil || status.State != "verified" || status.MaskedPhone != "••••4321" {
		t.Fatalf("phone edit affected identity: %#v %v", status, err)
	}
	current, err = service.ReadForEdit(ctx, first)
	if err != nil || current.Phone != input.Phone {
		t.Fatalf("editable contact number was not reflected: %v", err)
	}
	if _, err := contracts.CurrentForRentalContract(ctx, second, quote, first, time.Now()); err != nil {
		t.Fatalf("contact update invalidated legal identity in contract: %v", err)
	}
}
