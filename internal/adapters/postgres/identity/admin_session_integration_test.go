package postgres_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	identityhttp "github.com/HernanEspinozaDev/espaciGo/internal/identity/transport/http"
	"github.com/jackc/pgx/v5"
)

func seedGlobalAdminMembership(t *testing.T, ctx context.Context, accountID string, grantedAt time.Time) func() {
	t.Helper()
	var connStr string
	if pwBytes, err := os.ReadFile("../../../../.local/secrets/db_admin_password"); err == nil {
		pw := strings.TrimSpace(string(pwBytes))
		connStr = fmt.Sprintf("postgres://espacigo_admin:%s@172.24.0.2:5432/espacigo_aws_local?sslmode=disable", pw)
	} else if envURL := os.Getenv("TEST_DATABASE_URL"); envURL != "" {
		connStr = envURL
	} else {
		t.Fatal("cannot seed administrador_global: neither .local/secrets/db_admin_password nor TEST_DATABASE_URL available")
	}
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Fatalf("connect admin db to seed administrador_global: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO public.administrador_global (usuario_id, concedido_en, motivo_codigo) VALUES ($1, $2, 'concesion_administrativa')`, accountID, grantedAt); err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("insert administrador_global membership: %v", err)
	}
	return func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM public.administrador_global WHERE usuario_id = $1`, accountID)
		_ = conn.Close(context.Background())
	}
}

func TestPostgresAdminSession_LifecycleAndCapabilityIsolation(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	adminID := randomUUID()
	adminEmail := fmt.Sprintf("gadm-%s@ejemplo.invalid", adminID[:8])
	rawPassword := identity.Secret("AdminSecret#123")
	hashedPassword, err := (authTestHasher{}).Hash(rawPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	account := testAccount(adminID, adminEmail)
	account.State = identity.AccountActive
	account.PasswordHash = hashedPassword
	account.CreatedAt = now.Add(-24 * time.Hour)
	account.UpdatedAt = account.CreatedAt
	if err := repo.CreateWithTerms(ctx, account, nil); err != nil {
		t.Fatalf("create admin account: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.desafio_mfa_administrativo WHERE cuenta_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.sesion_administrativa WHERE cuenta_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.sesion WHERE usuario_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_totp_backup_code WHERE account_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.cuenta_totp WHERE account_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.rol_usuario WHERE usuario_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id = $1", adminID)
	}()

	cleanupGlobalAdmin := seedGlobalAdminMembership(t, ctx, adminID, now)
	defer cleanupGlobalAdmin()

	// Enroll TOTP for the admin account.
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatalf("rand aes key: %v", err)
	}
	kr, err := identity.NewTOTPKeyring("v1", map[string][]byte{"v1": aesKey})
	if err != nil {
		t.Fatalf("NewTOTPKeyring: %v", err)
	}
	totpSecret, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	keyVersion, ciphertext, err := kr.Seal(totpSecret)
	if err != nil {
		t.Fatalf("Seal TOTP secret: %v", err)
	}
	if err := repo.SavePendingTOTPSecret(ctx, adminID, ciphertext, keyVersion); err != nil {
		t.Fatalf("SavePendingTOTPSecret: %v", err)
	}
	_, backupHashes, err := identity.GenerateBackupCodes(8)
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if err := repo.ActivateTOTP(ctx, adminID, now.Add(-time.Hour).Unix()/30, backupHashes); err != nil {
		t.Fatalf("ActivateTOTP: %v", err)
	}

	// Persist a real commercial session token in public.sesion to verify audience isolation.
	commercialToken := identity.Secret("commercial-session-token-" + randomHex(16))
	commercialSessionID := randomUUID()
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.sesion (id, usuario_id, token_hash, creada_en, ultima_actividad_en, expira_en, cliente_resumen)
		VALUES ($1, $2, $3, $4, $4, $5, 'test-browser')
	`, commercialSessionID, adminID, identity.CredentialHash(commercialToken), now, now.Add(8*time.Hour)); err != nil {
		t.Fatalf("insert commercial session: %v", err)
	}

	adminSvc, err := identity.NewAdminSessionService(
		repo,
		authTestHasher{},
		kr,
		credentials.Generator{},
		func() time.Time { return now },
		true,
	)
	if err != nil {
		t.Fatalf("NewAdminSessionService: %v", err)
	}
	handler := identityhttp.NewAdminSessionHandler(adminSvc, nil)

	// 1. Start admin session -> returns mfa_required challenge.
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/start", bytes.NewBufferString(
		fmt.Sprintf(`{"email":%q,"password":%q}`, adminEmail, string(rawPassword)),
	))
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/admin/session/start status=%d body=%s", startRec.Code, startRec.Body.String())
	}
	var startResp struct {
		Status      string `json:"status"`
		ChallengeID string `json:"challenge_id"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if startResp.Status != "mfa_required" || startResp.ChallengeID == "" {
		t.Fatalf("expected mfa_required with challenge_id, got %+v", startResp)
	}

	// 2. Verify MFA -> issues sesion_administrativa (audiencia='admin').
	loginCode := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	mfaReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/mfa", bytes.NewBufferString(
		fmt.Sprintf(`{"challenge_id":%q,"code":%q}`, startResp.ChallengeID, loginCode),
	))
	mfaRec := httptest.NewRecorder()
	handler.ServeHTTP(mfaRec, mfaReq)
	if mfaRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/admin/session/mfa status=%d body=%s", mfaRec.Code, mfaRec.Body.String())
	}
	var mfaResp struct {
		Status   string `json:"status"`
		Audience string `json:"audience"`
		Token    string `json:"token"`
	}
	if err := json.Unmarshal(mfaRec.Body.Bytes(), &mfaResp); err != nil {
		t.Fatalf("decode mfa response: %v", err)
	}
	if mfaResp.Status != "authenticated" || mfaResp.Audience != "admin" || mfaResp.Token == "" {
		t.Fatalf("unexpected mfa response: %+v", mfaResp)
	}
	adminToken := identity.Secret(mfaResp.Token)

	var persistedAudience string
	var initialMFAVerifiedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT audiencia, mfa_verificado_en
		FROM public.sesion_administrativa
		WHERE token_hash = $1
	`, identity.CredentialHash(adminToken)).Scan(&persistedAudience, &initialMFAVerifiedAt); err != nil {
		t.Fatalf("query sesion_administrativa: %v", err)
	}
	if persistedAudience != "admin" || initialMFAVerifiedAt == nil {
		t.Fatalf("expected audiencia='admin' and mfa_verificado_en set, got audience=%q mfa_verificado_en=%v", persistedAudience, initialMFAVerifiedAt)
	}

	// 3. Commercial public.sesion token must be rejected on AuthorizeAdminCapability.
	if _, err := adminSvc.AuthorizeAdminCapability(ctx, commercialToken, identity.CapabilityAccountsRead, identity.UserOperation); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for commercial token, got %v", err)
	}

	// Admin token is accepted on AuthorizeAdminCapability.
	principal, err := adminSvc.AuthorizeAdminCapability(ctx, adminToken, identity.CapabilityAccountsRead, identity.UserOperation)
	if err != nil {
		t.Fatalf("AuthorizeAdminCapability with admin token: %v", err)
	}
	if principal.AccountID != adminID {
		t.Fatalf("expected principal AccountID=%s, got %s", adminID, principal.AccountID)
	}

	// 4. Step-up MFA refreshes mfa_verificado_en.
	now = now.Add(15 * time.Minute)
	stepCode := identity.GenerateTOTPCode(totpSecret, now.Unix()/30, 6)
	stepReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/session/mfa", bytes.NewBufferString(
		fmt.Sprintf(`{"code":%q}`, stepCode),
	))
	stepReq.Header.Set("Authorization", "Bearer "+string(adminToken))
	stepRec := httptest.NewRecorder()
	handler.ServeHTTP(stepRec, stepReq)
	if stepRec.Code != http.StatusOK {
		t.Fatalf("step-up POST /api/v1/admin/session/mfa status=%d body=%s", stepRec.Code, stepRec.Body.String())
	}

	var refreshedMFAVerifiedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT mfa_verificado_en
		FROM public.sesion_administrativa
		WHERE token_hash = $1
	`, identity.CredentialHash(adminToken)).Scan(&refreshedMFAVerifiedAt); err != nil {
		t.Fatalf("query refreshed mfa_verificado_en: %v", err)
	}
	if refreshedMFAVerifiedAt == nil || !refreshedMFAVerifiedAt.After(*initialMFAVerifiedAt) {
		t.Fatalf("expected mfa_verificado_en to advance after step-up: initial=%v refreshed=%v", initialMFAVerifiedAt, refreshedMFAVerifiedAt)
	}

	// 5. Revoke admin session -> revocada_en set and subsequent AuthorizeAdminCapability rejected.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/session", nil)
	delReq.Header.Set("Authorization", "Bearer "+string(adminToken))
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/admin/session status=%d body=%s", delRec.Code, delRec.Body.String())
	}

	var revokedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT revocada_en
		FROM public.sesion_administrativa
		WHERE token_hash = $1
	`, identity.CredentialHash(adminToken)).Scan(&revokedAt); err != nil {
		t.Fatalf("query revocada_en: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("expected revocada_en to be set after revocation")
	}
	if _, err := adminSvc.AuthorizeAdminCapability(ctx, adminToken, identity.CapabilityAccountsRead, identity.UserOperation); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized after admin session revocation, got %v", err)
	}
}

func TestPostgresAdminGlobal_GrantSensitiveAccessPersistence(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)
	now := time.Date(2026, 11, 24, 13, 0, 0, 0, time.UTC)

	adminID := randomUUID()
	targetID := randomUUID()
	adminSessionID := randomUUID()
	adminEmail := fmt.Sprintf("gadm-sa-%s@ejemplo.invalid", adminID[:8])
	targetEmail := fmt.Sprintf("target-sa-%s@ejemplo.invalid", targetID[:8])

	adminAcc := testAccount(adminID, adminEmail)
	adminAcc.State = identity.AccountActive
	targetAcc := testAccount(targetID, targetEmail)
	targetAcc.State = identity.AccountActive

	if err := repo.CreateWithTerms(ctx, adminAcc, nil); err != nil {
		t.Fatalf("create admin account: %v", err)
	}
	if err := repo.CreateWithTerms(ctx, targetAcc, nil); err != nil {
		t.Fatalf("create target account: %v", err)
	}

	var auditEventID string
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.acceso_sensible_administrativo WHERE actor_cuenta_id = $1", adminID)
		if auditEventID != "" {
			_, _ = pool.Exec(ctx, "DELETE FROM public.evento_auditoria_local WHERE id = $1", auditEventID)
		}
		_, _ = pool.Exec(ctx, "DELETE FROM public.evento_auditoria_local WHERE actor_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.sesion_administrativa WHERE id = $1", adminSessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.rol_usuario WHERE usuario_id = $1", adminID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id IN ($1, $2)", adminID, targetID)
	}()

	cleanupGlobalAdmin := seedGlobalAdminMembership(t, ctx, adminID, now)
	defer cleanupGlobalAdmin()
	if _, err := pool.Exec(ctx, `
		INSERT INTO public.sesion_administrativa (
			id, cuenta_id, audiencia, token_hash, creada_en, ultima_actividad_en, expira_en, mfa_verificado_en, mfa_metodo
		) VALUES ($1, $2, 'admin', $3, $4, $4, $5, $4, 'totp')
	`, adminSessionID, adminID, identity.CredentialHash(identity.Secret("admin-sa-token-"+randomHex(8))), now, now.Add(8*time.Hour)); err != nil {
		t.Fatalf("insert sesion_administrativa: %v", err)
	}

	svc, err := adminglobal.New(pool, func() time.Time { return now })
	if err != nil {
		t.Fatalf("adminglobal.New: %v", err)
	}

	corrID := randomUUID()
	reason := "Verificación de soporte administrativo"
	caseID := "CASE-101"
	res, err := svc.GrantSensitiveAccess(ctx, adminglobal.SensitiveAccessInput{
		ActorID:        adminID,
		TargetID:       targetID,
		AdminSessionID: adminSessionID,
		CorrelationID:  corrID,
		Field:          "email",
		Scope:          "account_email",
		Reason:         reason,
		CaseID:         caseID,
	})
	if err != nil {
		t.Fatalf("GrantSensitiveAccess: %v", err)
	}
	auditEventID = res.AuditEventID
	if res.Value != targetEmail || res.AuditEventID == "" {
		t.Fatalf("unexpected GrantSensitiveAccess result: %+v", res)
	}

	var (
		dbActorID, dbTargetID, dbSessionID, dbField, dbScope, dbReason, dbCaseID, dbAuditID string
	)
	if err := pool.QueryRow(ctx, `
		SELECT actor_cuenta_id::text, objetivo_cuenta_id::text, sesion_administrativa_id::text,
		       campo, alcance, motivo, caso_id, evento_auditoria_id::text
		FROM public.acceso_sensible_administrativo
		WHERE actor_cuenta_id = $1 AND objetivo_cuenta_id = $2
	`, adminID, targetID).Scan(&dbActorID, &dbTargetID, &dbSessionID, &dbField, &dbScope, &dbReason, &dbCaseID, &dbAuditID); err != nil {
		t.Fatalf("query public.acceso_sensible_administrativo: %v", err)
	}
	if dbActorID != adminID || dbTargetID != targetID || dbSessionID != adminSessionID ||
		dbField != "email" || dbScope != "account_email" || dbReason != reason ||
		dbCaseID != caseID || dbAuditID != res.AuditEventID {
		t.Fatalf("unexpected acceso_sensible_administrativo row: actor=%s target=%s session=%s field=%s scope=%s reason=%s case=%s audit=%s",
			dbActorID, dbTargetID, dbSessionID, dbField, dbScope, dbReason, dbCaseID, dbAuditID)
	}

	var (
		audActorID, audTargetID, audResult, audReason, audCorrID string
	)
	if err := pool.QueryRow(ctx, `
		SELECT actor_id::text, cuenta_objetivo_id::text, resultado, motivo_codigo, correlacion_id::text
		FROM public.evento_auditoria_local
		WHERE id = $1
	`, res.AuditEventID).Scan(&audActorID, &audTargetID, &audResult, &audReason, &audCorrID); err != nil {
		t.Fatalf("query public.evento_auditoria_local: %v", err)
	}
	if audActorID != adminID || audTargetID != targetID || audResult != "exito" || audReason != reason || audCorrID != corrID {
		t.Fatalf("unexpected evento_auditoria_local row: actor=%s target=%s result=%s reason=%s corr=%s",
			audActorID, audTargetID, audResult, audReason, audCorrID)
	}
}
