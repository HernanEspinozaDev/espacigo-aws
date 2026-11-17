package migrator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestRunnerEarlyRentalOperationalCandidatePostgres exercises the real
// operational candidate SQL through the single-owner runner on a disposable,
// networkless PostgreSQL target. V99 is staged only in each test's temp dir.
func TestRunnerEarlyRentalOperationalCandidatePostgres(t *testing.T) {
	if os.Getenv("TEST_MIGRATOR_CI_ISOLATED") != "1" {
		t.Skip("requires the networkless disposable PostgreSQL harness")
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() != "postgres" || !filepath.IsAbs(u.Query().Get("host")) {
		t.Fatal("requires disposable postgres Unix-socket target")
	}
	if _, password := u.User.Password(); password {
		t.Fatal("disposable harness must not supply a password")
	}
	marker, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !marker.Mode().IsRegular() || marker.Mode().Perm() != 0o600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	all, err := DiscoverMigrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil || len(all) != 99 || all[98].Name != "V000099__early_rental_manual_contract.sql" {
		t.Fatalf("requires canonical V1..V99 inventory: %v", err)
	}
	canonical := all[98]
	all = all[:98]
	t.Run("empty-database-generic-refusal", func(t *testing.T) {
		withTestDatabase(t, func(ctx context.Context, dsn string) {
			if _, err := Run(ctx, dsn, filepath.Join("..", "..", "db", "migrations")); err == nil || !strings.Contains(err.Error(), "dedicated E1 operation") {
				t.Fatalf("generic fresh initialization was not refused: %v", err)
			}
			reader := connectTestDB(t, ctx, dsn)
			defer reader.Close(context.Background())
			var untouched bool
			if err := reader.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NULL AND to_regclass('public.ocupacion') IS NULL`).Scan(&untouched); err != nil || !untouched {
				t.Fatalf("generic refusal initialized schema: %t %v", untouched, err)
			}
		})
	})
	for _, baseline := range []int{0, 75} {
		t.Run(fmt.Sprintf("baseline-%d", baseline), func(t *testing.T) {
			withTestDatabase(t, func(ctx context.Context, databaseURL string) {
				admin := connectTestDB(t, ctx, databaseURL)
				defer admin.Close(context.Background())
				migrationURL, history := establishRentalCombinedFixture(t, ctx, databaseURL, admin, all, baseline)
				if len(history) != baseline {
					t.Fatalf("baseline history count=%d want=%d", len(history), baseline)
				}
				// The fixture has already exercised the real V97/V98 path; do not
				// call withEarlyRentalCandidate, which installs historical V99/V100.
				t.Run("v97-upgrade", func(t *testing.T) { assertV97AuthorityLifecycle(t, ctx, admin, databaseURL, migrationURL, all[:97]) })
				t.Run("v98-expand", func(t *testing.T) { assertV98SpaceInventoryMigration(t, ctx, admin, migrationURL, all) })

				sql, err := os.ReadFile(filepath.Join("testdata", "early_rental_operational_candidate.sql"))
				if err != nil {
					t.Fatal(err)
				}
				if sha256Hex(sql) != canonical.Checksum {
					t.Fatal("tested SQL differs from allocated canonical V99")
				}
				dir := t.TempDir()
				for _, m := range all {
					if err := os.WriteFile(filepath.Join(dir, m.Name), m.SQL, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				const candidateName = "V000099__early_rental_manual_contract.sql"
				if err := os.WriteFile(filepath.Join(dir, candidateName), sql, 0o600); err != nil {
					t.Fatal(err)
				}
				inventory, err := DiscoverMigrations(dir)
				if err != nil || len(inventory) != 99 {
					t.Fatalf("staged V1..V98 + operational V99: len=%d err=%v", len(inventory), err)
				}
				t.Run("generic-runner-refuses-v99", func(t *testing.T) {
					if _, err := Run(ctx, databaseURL, dir); err == nil || !strings.Contains(err.Error(), "dedicated E1 operation") {
						t.Fatalf("generic applying route accepted V99: %v", err)
					}
					var count int
					var present bool
					if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.schema_migrations),to_regclass('public.arriendo_temprano') IS NOT NULL`).Scan(&count, &present); err != nil || count != 98 || present {
						t.Fatalf("generic refusal mutated target: %d %t %v", count, present, err)
					}
				})

				target := SingleOwnerMigrationTarget{Host: admin.Config().Host, Port: admin.Config().Port}
				if err := admin.QueryRow(ctx, `SELECT current_database(),d.oid,(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),current_setting('server_version_num')::int,session_user::text,(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.ocupacion'::regclass),(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.schema_migrations'::regclass) FROM pg_database d WHERE datname=current_database()`).Scan(&target.Database, &target.DatabaseOID, &target.SystemIdentifier, &target.ServerVersionNum, &target.SessionUser, &target.OccupancyOwner, &target.LedgerOwner); err != nil {
					t.Fatal(err)
				}
				if target.SessionUser != "postgres" || target.OccupancyOwner != "postgres" || target.LedgerOwner != "postgres" {
					t.Fatalf("unexpected legacy fixture owners/session; operational evidence is fixture-only: %+v", target)
				}
				var ownerProfile string
				if err := admin.QueryRow(ctx, operationalRoleProfileSQL).Scan(&ownerProfile); err != nil {
					t.Fatal(err)
				}
				const setup = `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
GRANT REFERENCES(id) ON public.usuario TO espacigo_schema_owner;
GRANT REFERENCES(id) ON public.espacio TO espacigo_schema_owner;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT ALL ON TABLES TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO espacigo_runtime`
				const revokeSetup = `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner;
REVOKE REFERENCES(id) ON public.espacio FROM espacigo_schema_owner`
				// Make only this synthetic fixture hostile to test that candidate
				// explicit ACL statements override PostgreSQL's damaging defaults.
				if _, err := admin.Exec(ctx, setup); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := admin.Exec(context.Background(), revokeSetup+`;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public REVOKE ALL ON TABLES FROM espacigo_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM espacigo_runtime`); err != nil {
						t.Errorf("revoke synthetic candidate authority/default ACLs: %v", err)
					}
				}()
				contract := SingleOwnerMigrationContract{InventorySHA256: inventoryFingerprint(inventory), Version: 99, Name: inventory[98].Name, Checksum: inventory[98].Checksum, Target: target}
				result, err := RunSingleOwnerMigration(ctx, admin, dir, contract)
				if err != nil || !result.Committed || !result.Verified {
					t.Fatalf("operational V99 execution: %+v %v", result, err)
				}
				if _, err := admin.Exec(ctx, revokeSetup); err != nil {
					t.Fatal(err)
				}
				reader := connectTestDB(t, ctx, databaseURL)
				defer reader.Close(context.Background())
				var gotProfile string
				if err := reader.QueryRow(ctx, operationalRoleProfileSQL).Scan(&gotProfile); err != nil || gotProfile != ownerProfile {
					t.Fatalf("schema-owner flags/memberships changed: before=%s after=%s err=%v", ownerProfile, gotProfile, err)
				}
				assertOperationalCandidateCatalog(t, ctx, reader)
				if err := guardE1SchemaCatalog(ctx, reader); err != nil {
					t.Fatal(err)
				}
				t.Run("catalog-refuses-missing-trigger", func(t *testing.T) {
					tx, err := admin.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback(context.Background())
					if _, err := tx.Exec(ctx, `DROP TRIGGER arriendo_temprano_evento_guard ON public.arriendo_temprano_evento`); err != nil {
						t.Fatal(err)
					}
					if err := guardE1SchemaCatalog(ctx, tx); err == nil || !strings.Contains(err.Error(), "structural catalog differs") {
						t.Fatalf("missing binding accepted: %v", err)
					}
					if err := tx.Rollback(ctx); err != nil {
						t.Fatal(err)
					}
					if err := guardE1SchemaCatalog(ctx, reader); err != nil {
						t.Fatalf("rollback lost trigger: %v", err)
					}
				})
				var ledgerVersion int64
				var ledgerName, ledgerChecksum string
				if err := reader.QueryRow(ctx, `SELECT version,name,checksum FROM public.schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&ledgerVersion, &ledgerName, &ledgerChecksum); err != nil || ledgerVersion != 99 || ledgerName != candidateName || ledgerChecksum != inventory[98].Checksum {
					t.Fatalf("V99 checksum readback version=%d name=%q checksum=%s err=%v", ledgerVersion, ledgerName, ledgerChecksum, err)
				}
				if _, err := RunSingleOwnerMigration(ctx, admin, dir, contract); err == nil {
					t.Fatal("blind replay of committed operational V99 accepted")
				}
				t.Run("generic-applied-v99-is-noop", func(t *testing.T) {
					got, err := Run(ctx, databaseURL, dir)
					if err != nil || len(got.Applied) != 0 {
						t.Fatalf("verified applied V99 no-op failed: %+v %v", got, err)
					}
				})

				f := newEarlyRentalFixture(t, ctx, admin)
				if err := seedEvidenceOffer(ctx, admin, f); err != nil {
					t.Fatal(err)
				}
				t.Run("early-advance-is-rejected-atomically", func(t *testing.T) {
					before := earlyRentalCounts(t, ctx, reader)
					tx, err := admin.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					_, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado='pendiente_contrato',revision=2,actualizada_en=clock_timestamp() WHERE id=$1`, f.offerID)
					if err == nil {
						_, err = tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo) VALUES($1,2,2,'identidad_simulada_confirmada',$2,'pendiente_identidad','pendiente_contrato')`, f.offerID, f.owner)
					}
					if err == nil {
						err = tx.Commit(ctx)
					} else {
						_ = tx.Rollback(context.Background())
					}
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
						t.Fatalf("early advance must fail with 23514, got %v", err)
					}
					if after := earlyRentalCounts(t, ctx, reader); after != before {
						t.Fatalf("early advance changed committed rows: %v -> %v", before, after)
					}
				})
				assertEvidenceBilateralLifecycle(t, ctx, admin, reader)
				assertOperationalTerminalReasonRequired(t, ctx, admin, reader)
			})
		})
	}
}

const operationalRoleProfileSQL = `SELECT
(SELECT concat_ws('|',rolname,rolsuper,rolinherit,rolcreaterole,rolcreatedb,rolcanlogin,rolreplication,rolbypassrls,rolconnlimit,rolvaliduntil::text) FROM pg_roles WHERE rolname='espacigo_schema_owner') || ':' ||
COALESCE((SELECT string_agg(concat_ws('/',pg_get_userbyid(roleid),pg_get_userbyid(member),admin_option,inherit_option,set_option),' ; ' ORDER BY roleid,member) FROM pg_auth_members WHERE roleid=(SELECT oid FROM pg_roles WHERE rolname='espacigo_schema_owner') OR member=(SELECT oid FROM pg_roles WHERE rolname='espacigo_schema_owner')),'')`

func assertOperationalCandidateCatalog(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var valid bool
	if err := db.QueryRow(ctx, `SELECT
	(SELECT count(*)=5 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=ANY(ARRAY['arriendo_temprano','arriendo_temprano_confirmacion','arriendo_temprano_pago','arriendo_temprano_evento','arriendo_temprano_idempotencia']) AND c.relkind='r')
	AND (SELECT count(*)=7 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname=ANY(ARRAY['arriendo_temprano_immutable_update','arriendo_temprano_occupancy_consistency','arriendo_temprano_confirmacion_guard','arriendo_temprano_pago_guard','arriendo_temprano_evento_guard','arriendo_temprano_evento_consistency','arriendo_temprano_idempotencia_guard']))
	AND (SELECT pg_get_userbyid(relowner)='espacigo_schema_owner' FROM pg_class WHERE oid='public.arriendo_temprano'::regclass)
	AND (SELECT pg_get_userbyid(relowner)='postgres' FROM pg_class WHERE oid='public.ocupacion'::regclass)
	AND (SELECT pg_get_userbyid(relowner)='postgres' FROM pg_class WHERE oid='public.schema_migrations'::regclass)
	AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.ocupacion'::regclass AND conname='ocupacion_no_solapa_excl' AND contype='x')
	AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','SELECT') AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','INSERT') AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','UPDATE')
	AND has_table_privilege('espacigo_runtime','public.arriendo_temprano_pago','UPDATE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','UPDATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','DELETE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_evento','UPDATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_evento','DELETE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_idempotencia','UPDATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_idempotencia','DELETE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano','DELETE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano','TRUNCATE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_pago','DELETE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_pago','TRUNCATE')
	AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','TRUNCATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_evento','TRUNCATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_idempotencia','TRUNCATE')
	AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN LATERAL aclexplode(p.proacl) a WHERE n.nspname='public' AND p.proname LIKE 'arriendo_temprano%' AND a.grantee=0 AND a.privilege_type='EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_immutable_update()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_occupancy_consistency()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion_guard()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_pago_guard()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_evento_guard()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_evento_consistency()','EXECUTE')
	AND NOT has_function_privilege('espacigo_runtime','public.arriendo_temprano_idempotencia_guard()','EXECUTE')
	AND NOT has_schema_privilege('espacigo_schema_owner','public','CREATE')
	AND NOT has_column_privilege('espacigo_schema_owner','public.usuario','id','REFERENCES')
	AND NOT has_column_privilege('espacigo_schema_owner','public.espacio','id','REFERENCES')`).Scan(&valid); err != nil || !valid {
		t.Fatalf("operational candidate exact catalog/ACL/ownership readback: valid=%t err=%v", valid, err)
	}
	var rows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname LIKE 'arriendo_temprano%' AND c.relkind='r'`).Scan(&rows); err != nil || rows != 5 {
		t.Fatalf("unexpected early-rental table inventory: %d %v", rows, err)
	}
	var funcs int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname LIKE 'arriendo_temprano%'`).Scan(&funcs); err != nil || funcs != 7 {
		t.Fatalf("unexpected early-rental function inventory: %d %v", funcs, err)
	}
}
