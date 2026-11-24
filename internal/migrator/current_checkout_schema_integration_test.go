package migrator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestCurrentSchemaMigrationCheckoutProUpToV111 keeps the legacy V1..V98
// fixtures historical and exercises Checkout Pro only after its canonical V111
// schema is installed on an isolated PostgreSQL target.
func TestCurrentSchemaMigrationCheckoutProUpToV111(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the disposable migration CI harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || u.Query().Get("host") == "" {
		t.Fatal("disposable CI requires its private Unix socket")
	}
	if _, err := os.Stat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker)); err != nil {
		t.Fatal("disposable CI marker is missing")
	}
	all, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil || len(all) < 111 || all[110].Version != 111 || all[110].Name != "V000111__commercial_checkout_preferences.sql" {
		t.Fatalf("requires canonical contiguous migrations through V111: %v", err)
	}
	legacy, err := discoverLegacyV98Migrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil || len(legacy) != 98 {
		t.Fatalf("requires exact V1..V98 legacy inventory: %v", err)
	}

	for _, baseline := range []int{0, 75} {
		t.Run(fmt.Sprintf("baseline-%d", baseline), func(t *testing.T) {
			withTestDatabase(t, func(ctx context.Context, databaseURL string) {
				admin := connectTestDB(t, ctx, databaseURL)
				defer admin.Close(context.Background())
				migrationURL, _ := establishRentalCombinedFixture(t, ctx, databaseURL, admin, legacy, baseline)

				t.Run("v97-upgrade", func(t *testing.T) {
					assertV97AuthorityLifecycle(t, ctx, admin, databaseURL, migrationURL, legacy[:97])
				})
				t.Run("v98-expand", func(t *testing.T) {
					assertV98SpaceInventoryMigration(t, ctx, admin, migrationURL, legacy)
				})
				post98, err := Plan(ctx, migrationURL, prefixMigrationDirForCombined(t, legacy, 98), "espacigo_schema_owner")
				if err != nil || !post98.ReadOnly || len(post98.Applied) != 98 || len(post98.Pending) != 0 {
					t.Fatalf("legacy V98 fixture is not complete: plan=%+v err=%v", post98, err)
				}

				// V99 is intentionally excluded from the generic runner. Apply the
				// exact packaged SQL through its existing single-owner path, against
				// this disposable socket target only.
				target := SingleOwnerMigrationTarget{Host: admin.Config().Host, Port: admin.Config().Port}
				if err := admin.QueryRow(ctx, `SELECT current_database(),d.oid,
					(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),
					current_setting('server_version_num')::int,session_user::text,
					(SELECT pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.ocupacion'::regclass),
					(SELECT pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.schema_migrations'::regclass)
					FROM pg_catalog.pg_database d WHERE datname=current_database()`).Scan(
					&target.Database, &target.DatabaseOID, &target.SystemIdentifier,
					&target.ServerVersionNum, &target.SessionUser, &target.OccupancyOwner, &target.LedgerOwner,
				); err != nil {
					t.Fatal(err)
				}
				if target.SessionUser != "postgres" || target.OccupancyOwner != "postgres" || target.LedgerOwner != "postgres" {
					t.Fatalf("unexpected isolated V98 owners/session: %+v", target)
				}
				const v99Setup = `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
GRANT REFERENCES(id) ON public.usuario TO espacigo_schema_owner;
GRANT REFERENCES(id) ON public.espacio TO espacigo_schema_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT ALL ON TABLES TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO espacigo_runtime`
				const v99Revoke = `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner;
REVOKE REFERENCES(id) ON public.espacio FROM espacigo_schema_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public REVOKE ALL ON TABLES FROM espacigo_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM espacigo_runtime`
				if _, err := admin.Exec(ctx, v99Setup); err != nil {
					t.Fatalf("prepare isolated V99 authority: %v", err)
				}
				defer func() {
					if _, err := admin.Exec(context.Background(), v99Revoke); err != nil {
						t.Errorf("revoke isolated V99 authority: %v", err)
					}
				}()
				v99Dir := prefixMigrationDirForCombined(t, all, 99)
				v99 := all[98]
				v99Result, err := RunSingleOwnerMigration(ctx, admin, v99Dir, SingleOwnerMigrationContract{
					InventorySHA256: inventoryFingerprint(all[:99]),
					Version:         v99.Version, Name: v99.Name, Checksum: v99.Checksum, Target: target,
				})
				if err != nil || !v99Result.Committed || !v99Result.Verified {
					t.Fatalf("apply canonical V99 on isolated fixture: result=%+v err=%v", v99Result, err)
				}
				if _, err := admin.Exec(ctx, v99Revoke); err != nil {
					t.Fatalf("close isolated V99 authority: %v", err)
				}

				applyPrefix := func(version int) {
					t.Helper()
					result, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, version), "espacigo_schema_owner")
					if err != nil {
						t.Fatalf("apply canonical migration prefix through V%d: %v", version, err)
					}
					history, err := readHistory(ctx, admin)
					if err != nil || len(history) != version || len(result.Applied) == 0 {
						t.Fatalf("V%d ledger postcheck: applied=%v history=%d err=%v", version, result.Applied, len(history), err)
					}
				}
				if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner`); err != nil {
					t.Fatalf("grant isolated V100 DDL authority: %v", err)
				}
				applyPrefix(100)
				if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V100 DDL authority: %v", err)
				}
				if _, err := admin.Exec(ctx, `GRANT INSERT ON TABLE public.version_terminos TO espacigo_schema_owner`); err != nil {
					t.Fatalf("grant isolated V101 data authority: %v", err)
				}
				applyPrefix(101)
				if _, err := admin.Exec(ctx, `REVOKE INSERT ON TABLE public.version_terminos FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V101 data authority: %v", err)
				}
				var mediaVariantOwner, mediaIntentOwner string
				if err := admin.QueryRow(ctx, `SELECT
					(SELECT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.media_variant'::regclass),
					(SELECT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.media_variant_intent'::regclass)`).Scan(&mediaVariantOwner, &mediaIntentOwner); err != nil {
					t.Fatal(err)
				}
				if mediaVariantOwner != "postgres" || mediaIntentOwner != "postgres" {
					t.Fatalf("unexpected pre-V102 historical media owners: variant=%q intent=%q", mediaVariantOwner, mediaIntentOwner)
				}
				if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
					ALTER TABLE public.media_variant OWNER TO espacigo_schema_owner;
					ALTER TABLE public.media_variant_intent OWNER TO espacigo_schema_owner`); err != nil {
					t.Fatalf("prepare isolated V102 owner handoff: %v", err)
				}
				applyPrefix(102)
				if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V102 DDL authority: %v", err)
				}

				// New ordinary migration objects are owned by schema_owner. The
				// temporary CREATE grant is confined to this disposable fixture.
				if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner`); err != nil {
					t.Fatalf("grant isolated later-migration DDL authority: %v", err)
				}
				if _, err := admin.Exec(ctx, `GRANT REFERENCES(id) ON TABLE public.usuario TO espacigo_schema_owner`); err != nil {
					t.Fatalf("grant isolated V103 foreign-key authority: %v", err)
				}
				if _, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, 103), "espacigo_schema_owner"); err != nil {
					t.Fatalf("apply canonical V103 phone verification schema: %v", err)
				}
				if _, err := admin.Exec(ctx, `REVOKE REFERENCES(id) ON TABLE public.usuario FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V103 foreign-key authority: %v", err)
				}
				var sessionOwner string
				if err := admin.QueryRow(ctx, `SELECT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.sesion'::regclass`).Scan(&sessionOwner); err != nil {
					t.Fatal(err)
				}
				if sessionOwner != "postgres" {
					t.Fatalf("unexpected pre-V104 historical session owner: %q", sessionOwner)
				}
				if _, err := admin.Exec(ctx, `ALTER TABLE public.sesion OWNER TO espacigo_schema_owner`); err != nil {
					t.Fatalf("prepare isolated V104 owner authority: %v", err)
				}
				if _, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, 104), "espacigo_schema_owner"); err != nil {
					t.Fatalf("apply canonical V104 session metadata schema: %v", err)
				}
				if _, err := admin.Exec(ctx, `GRANT REFERENCES(id) ON TABLE public.usuario TO espacigo_schema_owner`); err != nil {
					t.Fatalf("grant isolated V105..V110 foreign-key authority: %v", err)
				}
				if _, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, 110), "espacigo_schema_owner"); err != nil {
					t.Fatalf("apply canonical V103..V110: %v", err)
				}
				if _, err := admin.Exec(ctx, `REVOKE REFERENCES(id) ON TABLE public.usuario FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V105..V110 foreign-key authority: %v", err)
				}
				if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V103..V110 DDL authority: %v", err)
				}

				// V111 extends the historical occupancy relation. Model the reviewed
				// one-time owner handoff in this disposable fixture so ALTER runs as
				// schema_owner; no shared/local or production database is touched.
				var occupancyOwner string
				if err := admin.QueryRow(ctx, `SELECT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.ocupacion'::regclass`).Scan(&occupancyOwner); err != nil {
					t.Fatal(err)
				}
				if occupancyOwner != "postgres" {
					t.Fatalf("unexpected pre-V111 occupancy owner: %q", occupancyOwner)
				}
				if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
					GRANT REFERENCES(id) ON TABLE public.usuario TO espacigo_schema_owner;
					GRANT REFERENCES(id) ON TABLE public.espacio TO espacigo_schema_owner;
					ALTER TABLE public.ocupacion OWNER TO espacigo_schema_owner`); err != nil {
					t.Fatalf("prepare isolated V111 owner authority: %v", err)
				}
				if _, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, 111), "espacigo_schema_owner"); err != nil {
					t.Fatalf("apply canonical V111 Checkout Pro schema: %v", err)
				}
				if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V111 DDL authority: %v", err)
				}
				if _, err := admin.Exec(ctx, `REVOKE REFERENCES(id) ON TABLE public.usuario, public.espacio FROM espacigo_schema_owner`); err != nil {
					t.Fatalf("close isolated V111 foreign-key authority: %v", err)
				}
				var postOwner string
				if err := admin.QueryRow(ctx, `SELECT pg_catalog.pg_get_userbyid(relowner) FROM pg_catalog.pg_class WHERE oid='public.ocupacion'::regclass`).Scan(&postOwner); err != nil || postOwner != "espacigo_schema_owner" {
					t.Fatalf("V111 occupancy ownership postcheck: owner=%q err=%v", postOwner, err)
				}
				history, err := readHistory(ctx, admin)
				if err != nil || len(history) != 111 {
					t.Fatalf("V111 exact ledger postcheck: history=%d err=%v", len(history), err)
				}
				for i, migration := range all[:111] {
					if history[i].Version != migration.Version || history[i].Name != migration.Name || history[i].Checksum != migration.Checksum {
						t.Fatalf("canonical migration ledger mismatch at V%d", migration.Version)
					}
				}
				if len(all) > 111 {
					if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
						GRANT REFERENCES(id) ON TABLE public.usuario, public.espacio TO espacigo_schema_owner;
						ALTER TABLE public.espacio OWNER TO espacigo_schema_owner`); err != nil {
						t.Fatalf("prepare isolated V112+ DDL authority: %v", err)
					}
					if _, err := RunWithRole(ctx, migrationURL, prefixMigrationDirForCombined(t, all, len(all)), "espacigo_schema_owner"); err != nil {
						t.Fatalf("apply canonical V112..V%d schema: %v", len(all), err)
					}
					if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
						REVOKE REFERENCES(id) ON TABLE public.usuario, public.espacio FROM espacigo_schema_owner`); err != nil {
						t.Fatalf("close isolated V112+ DDL authority: %v", err)
					}
				}

				t.Run("commercial-quote-suppression", func(t *testing.T) {
					assertCommercialQuoteSuppression(t, ctx, databaseURL, admin)
				})
				t.Run("commercial-quote-persistence", func(t *testing.T) {
					assertCommercialQuotePersistenceWithExistingRuntimeACL(t, ctx, databaseURL, admin)
				})
				t.Run("contract-identity-reader", func(t *testing.T) {
					assertContractIdentityReaderPostgres(t, ctx, databaseURL, admin)
				})
				var legacyReadable bool
				if err := admin.QueryRow(ctx, `SELECT has_table_privilege('espacigo_runtime','public.identity_verification_attempts','SELECT')`).Scan(&legacyReadable); err != nil || legacyReadable {
					t.Fatalf("Checkout Pro fixture reopened legacy identity access: readable=%t err=%v", legacyReadable, err)
				}
			})
		})
	}
}
