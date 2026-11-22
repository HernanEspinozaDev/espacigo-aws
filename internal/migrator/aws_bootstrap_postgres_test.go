package migrator

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRunnerAWSBootstrapReferenceCatalogPostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the networkless migration CI PostgreSQL harness")
	}
	adminURL := os.Getenv("TEST_DATABASE_URL")
	target, err := url.Parse(adminURL)
	if err != nil || !filepath.IsAbs(target.Query().Get("host")) {
		t.Fatal("requires a disposable Unix socket")
	}
	marker, err := os.Lstat(filepath.Join(target.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0o600 {
		t.Fatal("requires the private migration CI marker")
	}
	sourceDir := filepath.Join("..", "..", "db", "migrations")
	migrations, err := DiscoverMigrations(sourceDir)
	if err != nil || len(migrations) < 100 {
		t.Fatalf("requires the canonical AWS bootstrap inventory: %v", err)
	}
	migrations = migrations[:100]
	withTestDatabase(t, func(ctx context.Context, databaseURL string) {
		admin := connectTestDB(t, ctx, databaseURL)
		defer admin.Close(context.Background())
		migrationURL, _ := establishRentalCombinedFixture(t, ctx, databaseURL, admin, migrations[:98], 0)
		if !t.Run("v97", func(t *testing.T) {
			assertV97AuthorityLifecycle(t, ctx, admin, databaseURL, migrationURL, migrations[:97])
		}) || !t.Run("v98", func(t *testing.T) {
			assertV98SpaceInventoryMigration(t, ctx, admin, migrationURL, migrations[:98])
		}) {
			t.Fatal("bootstrap predecessor failed")
		}

		// Clone only the fresh test database using its isolated server. The
		// production bootstrap SQL itself requires the exact database name.
		templateName := pgx.Identifier{admin.Config().Database}.Sanitize()
		if err := admin.Close(ctx); err != nil {
			t.Fatal(err)
		}
		control := connectTestDB(t, ctx, adminURL)
		defer control.Close(context.Background())
		if _, err := control.Exec(ctx, "CREATE DATABASE espacigo_prod TEMPLATE "+templateName); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if os.Getenv("TEST_MIGRATOR_RETAIN_AWS_PREVIEW") == "1" {
				return
			}
			if _, err := control.Exec(context.Background(), "DROP DATABASE espacigo_prod"); err != nil {
				t.Error(err)
			}
		}()
		productionURL := *target
		productionURL.Path = "/espacigo_prod"
		clone := connectTestDB(t, ctx, productionURL.String())
		defer clone.Close(context.Background())
		beforeV100 := prefixMigrationDirForCombined(t, migrations, 99)
		op, certificate := testE1RDSOperation(t)
		op.Migration.InventorySHA256 = inventoryFingerprint(migrations[:99])
		bound := &op.Migration.Target
		if err := clone.QueryRow(ctx, `SELECT d.oid,
			(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),
			current_setting('server_version_num')::int
			FROM pg_database d WHERE datname=current_database()`).Scan(
			&bound.DatabaseOID, &bound.SystemIdentifier, &bound.ServerVersionNum); err != nil {
			t.Fatal(err)
		}
		cfg, err := e1ConnectionConfig(op)
		if err != nil {
			t.Fatal(err)
		}
		relayE1TestPostgres(t, filepath.Join(target.Query().Get("host"), ".s.PGSQL.5432"), certificate, cfg)
		badTLS := cfg.Copy()
		badTLS.TLSConfig.ServerName = "unreviewed.us-east-2.rds.amazonaws.com"
		if unexpected, err := connectE1(ctx, badTLS, true); err == nil {
			_ = unexpected.Close(ctx)
			t.Fatal("accepted an unrelated certificate hostname")
		}
		writer, err := connectE1(ctx, cfg, true)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Close(context.Background())
		gate := func(ctx context.Context, tx pgx.Tx, after bool) error {
			if after {
				return guardE1SchemaCatalog(ctx, tx)
			}
			return nil
		}
		if rejected, err := runSingleOwnerMigration(ctx, writer, beforeV100, op.Migration, gate, true); err == nil || rejected.Committed {
			t.Fatalf("unexpected legacy owners accepted: %+v %v", rejected, err)
		}
		// Prepare only the disposable clone's pre-existing ownership contract.
		if _, err := clone.Exec(ctx, `ALTER TABLE public.ocupacion OWNER TO espacigo_schema_owner;
			ALTER TABLE public.schema_migrations OWNER TO espacigo_schema_owner;
			GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
			GRANT REFERENCES(id) ON public.usuario, public.espacio TO espacigo_schema_owner`); err != nil {
			t.Fatal(err)
		}
		badTarget := op.Migration
		badTarget.Target.DatabaseOID++
		if rejected, err := runSingleOwnerMigration(ctx, writer, beforeV100, badTarget, gate, true); err == nil || rejected.Committed {
			t.Fatalf("unreviewed database identifier accepted: %+v %v", rejected, err)
		}
		result, err := runSingleOwnerMigration(ctx, writer, beforeV100, op.Migration, gate, true)
		if err != nil || !result.Committed || !result.Verified {
			t.Fatalf("canonical V99 over verified TLS failed: %+v %v", result, err)
		}
		if rejected, err := runSingleOwnerMigration(ctx, writer, beforeV100, op.Migration, gate, true); err == nil ||
			rejected.Committed || !strings.Contains(err.Error(), "exact predecessor ledger") {
			t.Fatalf("V99 replay was not blocked: %+v %v", rejected, err)
		}
		if err := writer.Close(ctx); err != nil {
			t.Fatal(err)
		}
		t.Run("v99-independent-tls-readback", func(t *testing.T) {
			reader, err := connectE1(ctx, cfg, true)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close(context.Background())
			tx, err := beginE1Inspection(ctx, reader, op.Migration.Target)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if err := verifySingleOwnerLedger(ctx, tx, migrations[:99]); err != nil {
				t.Fatal(err)
			}
			if err := guardE1SchemaCatalog(ctx, tx); err != nil {
				t.Fatal(err)
			}
		})
		if _, err := clone.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
			REVOKE REFERENCES(id) ON public.usuario, public.espacio FROM espacigo_schema_owner`); err != nil {
			t.Fatal(err)
		}
		bootstrapDir := prefixMigrationDirForCombined(t, migrations, 100)
		// V100 replaces a trigger function. PostgreSQL requires CREATE on
		// the containing schema even when the effective role owns it.
		if _, err := clone.Exec(ctx, "GRANT CREATE ON SCHEMA public TO espacigo_schema_owner"); err != nil {
			t.Fatal(err)
		}
		readerURL := productionURL
		readerURL.User = url.User(bound.SessionUser)
		applied, err := RunWithRoleInRange(ctx, readerURL.String(), bootstrapDir, "espacigo_schema_owner", 99, 100)
		if _, revokeErr := clone.Exec(ctx, "REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner"); revokeErr != nil {
			t.Fatal(revokeErr)
		}
		if err != nil || len(applied.Applied) != 1 || applied.Applied[0] != 100 {
			t.Fatalf("canonical V100 failed: %+v %v", applied, err)
		}
		finalize, err := os.ReadFile(filepath.Join("..", "..", "db", "bootstrap", "aws-main", "finalize_reference_only.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := clone.PgConn().Exec(ctx, string(finalize)).ReadAll(); err != nil {
			t.Fatal(err)
		}
		var categories, regions, communes, demoTerms int
		if err := clone.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.categoria_espacio WHERE activa),
			(SELECT count(*) FROM public.region_publica), (SELECT count(*) FROM public.comuna_publica),
			(SELECT count(*) FROM public.version_terminos)`).Scan(&categories, &regions, &communes, &demoTerms); err != nil {
			t.Fatal(err)
		}
		if categories != 9 || regions != 16 || communes != 345 || demoTerms != 0 {
			t.Fatalf("unexpected bootstrap data: categories=%d regions=%d communes=%d demo terms=%d", categories, regions, communes, demoTerms)
		}
		if err := VerifyAllApplied(ctx, clone, bootstrapDir, 100); err != nil {
			t.Fatal(err)
		}
		productionURL.User = url.User("espacigo_api_login")
		api := connectTestDB(t, ctx, productionURL.String())
		defer api.Close(context.Background())
		if _, err := api.Exec(ctx, "SET ROLE espacigo_runtime"); err != nil {
			t.Fatal(err)
		}
		var catalogsReadable bool
		if err := api.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.categoria_espacio)
			AND EXISTS (SELECT 1 FROM public.region_publica)
			AND EXISTS (SELECT 1 FROM public.comuna_publica)`).Scan(&catalogsReadable); err != nil || !catalogsReadable {
			t.Fatalf("API runtime could not read the initial catalogs: %v", err)
		}
	})
}
