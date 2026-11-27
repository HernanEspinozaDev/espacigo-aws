package kybpg

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed testdata/*.sql
var kybRegressionSQL embed.FS

func TestKYBPostgresPolicyRegressions(t *testing.T) {
	ctx, admin, _ := setupKYBTestDB(t)
	for _, name := range []string{"shared_deadline", "compliance_notices", "company_signature"} {
		t.Run(name, func(t *testing.T) {
			query, err := kybRegressionSQL.ReadFile("testdata/" + name + ".sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(ctx, string(query)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func setupKYBTestDB(t *testing.T) (context.Context, *pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is required for disposable PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	var createdRoles []string
	for _, role := range []struct {
		name  string
		login bool
	}{
		{"espacigo_runtime", false},
		{"espacigo_schema_owner", false},
		{"espacigo_migrator_login", true},
		{"espacigo_privacy_cleanup_worker", false},
		{"espacigo_gadm_bootstrap", false},
		{"espacigo_document_verification_fn", false},
	} {
		var exists bool
		if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role.name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			loginClause := "NOLOGIN"
			if role.login {
				loginClause = "LOGIN"
			}
			if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role.name}.Sanitize()+" "+loginClause+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
				t.Fatal(err)
			}
			createdRoles = append(createdRoles, role.name)
		}
	}

	dbName := fmt.Sprintf("kyb_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanCancel()
		_, _ = admin.Exec(cleanCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)")
		for _, role := range createdRoles {
			_, _ = admin.Exec(cleanCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		}
	})

	parsedURL, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	parsedURL.Path = "/" + dbName
	dbURL := parsedURL.String()

	setupConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer setupConn.Close(context.Background())

	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		if _, err := setupConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
			t.Fatalf("create extension %s: %v", ext, err)
		}
	}
	if _, err := setupConn.Exec(ctx, `GRANT CONNECT ON DATABASE `+pgx.Identifier{dbName}.Sanitize()+` TO espacigo_migrator_login;
		GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner;
		GRANT espacigo_schema_owner TO espacigo_migrator_login WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;`); err != nil {
		t.Fatal(err)
	}

	migrations, err := migrator.DiscoverMigrations("../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	migConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer migConn.Close(context.Background())

	if _, err := migConn.Exec(ctx, `SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner;`); err != nil {
		t.Fatal(err)
	}
	if _, err := migConn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version bigint PRIMARY KEY CHECK (version > 0),
		name text NOT NULL UNIQUE,
		checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
		runner_checksum text NOT NULL CHECK (runner_checksum ~ '^[0-9a-f]{64}$'),
		applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
	)`); err != nil {
		t.Fatal(err)
	}
	runnerDigest := strings.Repeat("0", 64)
	for _, m := range migrations {
		tx, err := migConn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Conn().PgConn().Exec(ctx, string(m.SQL)).ReadAll(); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("execute migration %s: %v", m.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations(version,name,checksum,runner_checksum) VALUES($1,$2,$3,$4)`, m.Version, m.Name, m.Checksum, runnerDigest); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("record migration %s: %v", m.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %s: %v", m.Name, err)
		}
	}

	if err := dbbootstrap.GrantRuntimePermissions(ctx, setupConn); err != nil {
		t.Fatal(err)
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	runtimeCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE espacigo_runtime`)
		return err
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimePool.Close)

	var currentRole string
	if err := runtimePool.QueryRow(ctx, `SELECT current_user`).Scan(&currentRole); err != nil || currentRole != "espacigo_runtime" {
		t.Fatalf("runtime pool current_user=%q err=%v", currentRole, err)
	}

	return ctx, adminPool, runtimePool
}

func makeKYBDocuments(verificationID, prefix string) []kyb.Document {
	sum := sha256.Sum256([]byte(prefix + "-doc"))
	return []kyb.Document{
		{
			ID:             prefix + "11111111-1111-4111-8111-111111111111"[len(prefix):],
			VerificationID: verificationID,
			Kind:           kyb.DocumentERUT,
			MIME:           "application/pdf",
			Size:           1024,
			SHA256:         sum[:],
			StorageRef:     prefix + "22222222-2222-4222-8222-222222222222"[len(prefix):],
		},
		{
			ID:             prefix + "33333333-3333-4333-8333-333333333333"[len(prefix):],
			VerificationID: verificationID,
			Kind:           kyb.DocumentPowerCertificate,
			MIME:           "application/pdf",
			Size:           2048,
			SHA256:         sum[:],
			StorageRef:     prefix + "44444444-4444-4444-8444-444444444444"[len(prefix):],
		},
		{
			ID:             prefix + "55555555-5555-4555-8555-555555555555"[len(prefix):],
			VerificationID: verificationID,
			Kind:           kyb.DocumentPropertyOrSublease,
			MIME:           "image/png",
			Size:           512,
			SHA256:         sum[:],
			StorageRef:     prefix + "66666666-6666-4666-8666-666666666666"[len(prefix):],
		},
	}
}

func makeVerificationFixture(id, orgID, ownerID, idempotencyKey, companyFingerprint, repFingerprint string, now time.Time) kyb.Verification {
	cipher := make([]byte, 32)
	for i := range cipher {
		cipher[i] = byte(i + 1)
	}
	return kyb.Verification{
		ID:                       id,
		OrganizationID:           orgID,
		OwnerID:                  ownerID,
		SubmittedBy:              ownerID,
		LegalName:                "Empresa Comercial SpA",
		CompanyRUTFingerprint:    companyFingerprint,
		CompanyRUTFingerprints:   []string{companyFingerprint},
		CompanyRUTKeyVersion:     "v1",
		CompanyRUTCiphertext:     cipher,
		RepresentativeName:       "Representante Legal",
		RepresentativeRUTFprint:  repFingerprint,
		RepresentativeRUTFprints: []string{repFingerprint},
		RepresentativeRUTKey:     "v1",
		RepresentativeRUTCipher:  cipher,
		SIIGiroCode:              "620100",
		Status:                   kyb.StatusPending,
		SubmittedAt:              now,
		CreatedAt:                now,
		UpdatedAt:                now,
		IdempotencyKey:           idempotencyKey,
	}
}

func TestKYBRepositoryPostgresLifecycleResubmissionAndImmutability(t *testing.T) {
	ctx, adminPool, runtimePool := setupKYBTestDB(t)
	repo := New(runtimePool)

	ownerA := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	ownerB := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	reviewerID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	for _, user := range []struct {
		id    string
		email string
	}{
		{ownerA, "kyb-owner-a@example.test"},
		{ownerB, "kyb-owner-b@example.test"},
		{reviewerID, "kyb-reviewer@example.test"},
	} {
		if _, err := adminPool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'x','activo')`, user.id, user.email); err != nil {
			t.Fatal(err)
		}
	}

	// 1. GetOrCreateOrganization (idempotent + rejects unknown user)
	orgACandidate1 := "10000000-0000-4000-8000-000000000001"
	orgACandidate2 := "10000000-0000-4000-8000-000000000002"
	orgA, err := repo.GetOrCreateOrganization(ctx, ownerA, orgACandidate1)
	if err != nil || orgA != orgACandidate1 {
		t.Fatalf("first GetOrCreateOrganization=%q err=%v", orgA, err)
	}
	orgAAgain, err := repo.GetOrCreateOrganization(ctx, ownerA, orgACandidate2)
	if err != nil || orgAAgain != orgA {
		t.Fatalf("idempotent GetOrCreateOrganization=%q err=%v want %q", orgAAgain, err, orgA)
	}
	if _, err := repo.GetOrCreateOrganization(ctx, "99999999-9999-4999-8999-999999999999", orgACandidate2); !errors.Is(err, kyb.ErrNotFound) {
		t.Fatalf("missing user GetOrCreateOrganization err=%v want ErrNotFound", err)
	}
	orgB, err := repo.GetOrCreateOrganization(ctx, ownerB, "20000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create + idempotency by idempotency_key
	now := time.Now().UTC().Truncate(time.Microsecond)
	companyFP := "v1:" + strings.Repeat("a", 64)
	repFP := "v1:" + strings.Repeat("b", 64)
	v1ID := "30000000-0000-4000-8000-000000000001"
	v1Docs := makeKYBDocuments(v1ID, "a1")
	v1Item := makeVerificationFixture(v1ID, orgA, ownerA, "kyb-idem-1", companyFP, repFP, now)

	created, reused, err := repo.Create(ctx, v1Item, v1Docs)
	if err != nil || reused || created.ID != v1ID || created.Status != kyb.StatusPending || len(created.Documents) != 3 {
		t.Fatalf("Create v1=%+v reused=%t err=%v", created, reused, err)
	}

	v1RetryID := "30000000-0000-4000-8000-000000000099"
	v1RetryDocs := makeKYBDocuments(v1RetryID, "a9")
	v1RetryItem := makeVerificationFixture(v1RetryID, orgA, ownerA, "kyb-idem-1", companyFP, repFP, now.Add(time.Second))
	replayed, reused, err := repo.Create(ctx, v1RetryItem, v1RetryDocs)
	if err != nil || !reused || replayed.ID != v1ID || len(replayed.Documents) != 3 {
		t.Fatalf("idempotent Create=%+v reused=%t err=%v", replayed, reused, err)
	}

	// 3. Duplicate company RUT blocked for another organization while PENDING
	vOtherID := "40000000-0000-4000-8000-000000000001"
	vOtherDocs := makeKYBDocuments(vOtherID, "b1")
	vOtherItem := makeVerificationFixture(vOtherID, orgB, ownerB, "kyb-orgb-1", companyFP, "v1:"+strings.Repeat("c", 64), now.Add(2*time.Second))
	if _, _, err := repo.Create(ctx, vOtherItem, vOtherDocs); !errors.Is(err, kyb.ErrConflict) {
		t.Fatalf("duplicate company RUT across orgs err=%v want ErrConflict", err)
	}

	// 4. ListPending and GetDocument with rejection_reason NULL (must not 500 / fail Scan)
	pendingList, err := repo.ListPending(ctx)
	if err != nil || len(pendingList) != 1 || pendingList[0].ID != v1ID || pendingList[0].RejectionReason != "" || len(pendingList[0].Documents) != 3 {
		t.Fatalf("ListPending=%+v err=%v", pendingList, err)
	}
	doc, docVerif, err := repo.GetDocument(ctx, v1ID, v1Docs[0].ID)
	if err != nil || doc.ID != v1Docs[0].ID || docVerif.ID != v1ID || docVerif.RejectionReason != "" {
		t.Fatalf("GetDocument doc=%+v verif=%+v err=%v", doc, docVerif, err)
	}
	docByRef, docByRefVerif, err := repo.GetDocumentByStorageRef(ctx, v1Docs[0].StorageRef)
	if err != nil || docByRef.StorageRef != v1Docs[0].StorageRef || docByRefVerif.RejectionReason != "" {
		t.Fatalf("GetDocumentByStorageRef doc=%+v verif=%+v err=%v", docByRef, docByRefVerif, err)
	}

	// 5. StartReview -> Decide(REJECT) -> verify claims deactivated -> successful resubmission by same org with same company RUT
	inReview, err := repo.StartReview(ctx, v1ID, reviewerID, now.Add(time.Minute))
	if err != nil || inReview.Status != kyb.StatusInReview || inReview.RejectionReason != "" {
		t.Fatalf("StartReview=%+v err=%v", inReview, err)
	}
	rejected, err := repo.Decide(ctx, v1ID, reviewerID, kyb.StatusRejected, "Certificado e-RUT ilegible", now.Add(2*time.Minute))
	if err != nil || rejected.Status != kyb.StatusRejected || rejected.RejectionReason != "Certificado e-RUT ilegible" {
		t.Fatalf("Decide(REJECT)=%+v err=%v", rejected, err)
	}
	var activeClaims int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM public.kyb_rut_claim WHERE verification_id=$1 AND active`, v1ID).Scan(&activeClaims); err != nil || activeClaims != 0 {
		t.Fatalf("rejected verification kept active RUT claims=%d err=%v", activeClaims, err)
	}

	v2ID := "30000000-0000-4000-8000-000000000002"
	v2Docs := makeKYBDocuments(v2ID, "a2")
	v2Item := makeVerificationFixture(v2ID, orgA, ownerA, "kyb-idem-2", companyFP, repFP, now.Add(3*time.Minute))
	resubmitted, reused, err := repo.Create(ctx, v2Item, v2Docs)
	if err != nil || reused || resubmitted.ID != v2ID || resubmitted.Status != kyb.StatusPending {
		t.Fatalf("resubmission after REJECTED=%+v reused=%t err=%v", resubmitted, reused, err)
	}
	ownLatest, err := repo.GetOwn(ctx, ownerA)
	if err != nil || ownLatest.ID != v2ID || ownLatest.Status != kyb.StatusPending {
		t.Fatalf("GetOwn after resubmission=%+v err=%v", ownLatest, err)
	}
	var activeResubmissionClaims int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM public.kyb_rut_claim WHERE verification_id=$1 AND active`, v2ID).Scan(&activeResubmissionClaims); err != nil || activeResubmissionClaims != 2 {
		t.Fatalf("resubmitted verification active RUT claims=%d err=%v", activeResubmissionClaims, err)
	}

	// 6. StartReview -> Decide(APPROVE)
	inReview2, err := repo.StartReview(ctx, v2ID, reviewerID, now.Add(4*time.Minute))
	if err != nil || inReview2.Status != kyb.StatusInReview {
		t.Fatalf("StartReview v2=%+v err=%v", inReview2, err)
	}
	approved, err := repo.Decide(ctx, v2ID, reviewerID, kyb.StatusApproved, "", now.Add(5*time.Minute))
	if err != nil || approved.Status != kyb.StatusApproved || approved.RejectionReason != "" || approved.ReviewedAt == nil || approved.ReviewerID != reviewerID {
		t.Fatalf("Decide(APPROVE)=%+v err=%v", approved, err)
	}
	if _, _, err := repo.Create(ctx, vOtherItem, vOtherDocs); !errors.Is(err, kyb.ErrConflict) {
		t.Fatalf("duplicate company RUT after APPROVED err=%v want ErrConflict", err)
	}
	if _, docApproved, err := repo.GetDocument(ctx, v2ID, v2Docs[0].ID); err != nil || docApproved.RejectionReason != "" || docApproved.Status != kyb.StatusApproved {
		t.Fatalf("GetDocument on approved verification=%+v err=%v", docApproved, err)
	}

	// 7. Append-only immutability of kyb_verification_event against UPDATE / DELETE
	var eventCount int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM public.kyb_verification_event WHERE verification_id IN ($1,$2)`, v1ID, v2ID).Scan(&eventCount); err != nil || eventCount != 6 {
		t.Fatalf("unexpected KYB event count=%d err=%v", eventCount, err)
	}
	if _, err := adminPool.Exec(ctx, `UPDATE public.kyb_verification_event SET action='approved' WHERE verification_id=$1`, v1ID); err == nil {
		t.Fatal("UPDATE on public.kyb_verification_event unexpectedly succeeded")
	}
	if _, err := adminPool.Exec(ctx, `DELETE FROM public.kyb_verification_event WHERE verification_id=$1`, v1ID); err == nil {
		t.Fatal("DELETE on public.kyb_verification_event unexpectedly succeeded")
	}
	if _, err := runtimePool.Exec(ctx, `UPDATE public.kyb_verification_event SET action='approved' WHERE verification_id=$1`, v1ID); err == nil {
		t.Fatal("runtime UPDATE on public.kyb_verification_event unexpectedly succeeded")
	}
	if _, err := runtimePool.Exec(ctx, `DELETE FROM public.kyb_verification_event WHERE verification_id=$1`, v1ID); err == nil {
		t.Fatal("runtime DELETE on public.kyb_verification_event unexpectedly succeeded")
	}
}
