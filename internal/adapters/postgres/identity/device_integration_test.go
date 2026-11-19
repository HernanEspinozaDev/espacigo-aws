package postgres_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getIntegrationPool(t *testing.T) (context.Context, *pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	// Try local development PostgreSQL database
	pwBytes, err := os.ReadFile("../../../../.local/secrets/runtime_password")
	if err == nil {
		pw := strings.TrimSpace(string(pwBytes))
		connStr := fmt.Sprintf("postgres://espacigo_api_login:%s@172.24.0.2:5432/espacigo_aws_local?sslmode=disable", pw)
		cfg, err := pgxpool.ParseConfig(connStr)
		if err == nil {
			cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
				_, err := conn.Exec(ctx, "SET ROLE espacigo_runtime")
				return err
			}
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err == nil {
				pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				if err := pool.Ping(pingCtx); err == nil {
					return ctx, pool, func() { pool.Close() }
				}
				pool.Close()
			}
		}
	}

	// Fallback to TEST_DATABASE_URL if configured
	if os.Getenv("TEST_DATABASE_URL") != "" {
		ctx, pool := newIdentityTestPool(t)
		return ctx, pool, func() { pool.Close() }
	}

	t.Skip("Neither local postgres nor TEST_DATABASE_URL available")
	return nil, nil, nil
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func TestPostgresDeviceSessions_Flow(t *testing.T) {
	ctx, pool, closePool := getIntegrationPool(t)
	defer closePool()

	repo := identitypg.NewIdentityRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	accountAID := randomUUID()
	accountBID := randomUUID()

	accountA := testAccount(accountAID, fmt.Sprintf("dev-%s@ejemplo.invalid", accountAID[:8]))
	if err := repo.CreateWithTerms(ctx, accountA, nil); err != nil {
		t.Fatalf("create accountA: %v", err)
	}
	accountB := testAccount(accountBID, fmt.Sprintf("dev-%s@ejemplo.invalid", accountBID[:8]))
	if err := repo.CreateWithTerms(ctx, accountB, nil); err != nil {
		t.Fatalf("create accountB: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM public.sesion WHERE usuario_id IN ($1, $2)", accountA.ID, accountB.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM public.usuario WHERE id IN ($1, $2)", accountA.ID, accountB.ID)
	}()

	sesA1 := identity.Session{
		ID:             randomUUID(),
		AccountID:      accountA.ID,
		TokenHash:      randomHex(64),
		CreatedAt:      now.Add(-2 * time.Hour),
		LastActivityAt: now.Add(-10 * time.Minute),
		ExpiresAt:      now.Add(6 * time.Hour),
		DeviceLabel:    "Chrome en Linux",
		IPMasked:       "190.162.•••.•••",
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0",
	}
	if err := repo.CreateSession(ctx, sesA1); err != nil {
		t.Fatalf("create sesA1: %v", err)
	}

	sesA2 := identity.Session{
		ID:             randomUUID(),
		AccountID:      accountA.ID,
		TokenHash:      randomHex(64),
		CreatedAt:      now.Add(-1 * time.Hour),
		LastActivityAt: now.Add(-5 * time.Minute),
		ExpiresAt:      now.Add(7 * time.Hour),
		DeviceLabel:    "Safari en iPhone",
		IPMasked:       "200.54.•••.•••",
		UserAgent:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) Safari/604.1",
	}
	if err := repo.CreateSession(ctx, sesA2); err != nil {
		t.Fatalf("create sesA2: %v", err)
	}

	sesB1 := identity.Session{
		ID:             randomUUID(),
		AccountID:      accountB.ID,
		TokenHash:      randomHex(64),
		CreatedAt:      now.Add(-30 * time.Minute),
		LastActivityAt: now.Add(-2 * time.Minute),
		ExpiresAt:      now.Add(7 * time.Hour + 30*time.Minute),
		DeviceLabel:    "Firefox en Windows",
		IPMasked:       "186.105.•••.•••",
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; rv:119.0) Firefox/119.0",
	}
	if err := repo.CreateSession(ctx, sesB1); err != nil {
		t.Fatalf("create sesB1: %v", err)
	}

	// 1. List active sessions for accountA
	sessionsA, err := repo.ListActiveSessions(ctx, accountA.ID, now)
	if err != nil {
		t.Fatalf("ListActiveSessions accountA: %v", err)
	}
	if len(sessionsA) != 2 {
		t.Fatalf("expected 2 active sessions for accountA, got %d", len(sessionsA))
	}
	// Ordered by last activity DESC -> sesA2 should be first
	if sessionsA[0].ID != sesA2.ID || sessionsA[1].ID != sesA1.ID {
		t.Fatalf("expected order [sesA2, sesA1], got [%s, %s]", sessionsA[0].ID, sessionsA[1].ID)
	}
	if sessionsA[0].DeviceLabel != "Safari en iPhone" || sessionsA[0].IPMasked != "200.54.•••.•••" {
		t.Fatalf("sesA2 metadata mismatch: %+v", sessionsA[0])
	}
	if sessionsA[1].DeviceLabel != "Chrome en Linux" || sessionsA[1].IPMasked != "190.162.•••.•••" {
		t.Fatalf("sesA1 metadata mismatch: %+v", sessionsA[1])
	}

	// 2. Isolation: accountA cannot revoke accountB's session
	revoked, err := repo.RevokeSessionForAccount(ctx, accountA.ID, sesB1.ID, now)
	if err != nil {
		t.Fatalf("RevokeSessionForAccount error: %v", err)
	}
	if revoked {
		t.Fatalf("accountA successfully revoked accountB session; isolation broken!")
	}
	// Verify sesB1 is still active
	sessionsB, err := repo.ListActiveSessions(ctx, accountB.ID, now)
	if err != nil || len(sessionsB) != 1 || sessionsB[0].ID != sesB1.ID {
		t.Fatalf("sesB1 should still be active, got %+v", sessionsB)
	}

	// 3. Revoke sesA2 with accountA
	revoked, err = repo.RevokeSessionForAccount(ctx, accountA.ID, sesA2.ID, now)
	if err != nil {
		t.Fatalf("RevokeSessionForAccount sesA2: %v", err)
	}
	if !revoked {
		t.Fatalf("failed to revoke sesA2")
	}

	// Verify only sesA1 remains active
	sessionsA, err = repo.ListActiveSessions(ctx, accountA.ID, now)
	if err != nil || len(sessionsA) != 1 || sessionsA[0].ID != sesA1.ID {
		t.Fatalf("expected only sesA1 active, got %+v", sessionsA)
	}

	// 4. Create sesA3 and test RevokeOtherSessionsForAccount
	sesA3 := identity.Session{
		ID:             randomUUID(),
		AccountID:      accountA.ID,
		TokenHash:      randomHex(64),
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(8 * time.Hour),
		DeviceLabel:    "Edge en Windows",
		IPMasked:       "190.162.•••.•••",
	}
	if err := repo.CreateSession(ctx, sesA3); err != nil {
		t.Fatalf("create sesA3: %v", err)
	}

	count, err := repo.RevokeOtherSessionsForAccount(ctx, accountA.ID, sesA1.ID, now)
	if err != nil {
		t.Fatalf("RevokeOtherSessionsForAccount: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session revoked (sesA3), got %d", count)
	}

	// Verify only sesA1 is active
	sessionsA, err = repo.ListActiveSessions(ctx, accountA.ID, now)
	if err != nil || len(sessionsA) != 1 || sessionsA[0].ID != sesA1.ID {
		t.Fatalf("expected only sesA1 active, got %+v", sessionsA)
	}
}
