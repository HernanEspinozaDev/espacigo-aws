package migrator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The candidate is not in db/migrations. V99 is staged only inside this test's
// temporary inventory, never allocated or enabled for a persistent target.
func TestRunnerEarlyRentalSchemaCandidatePostgres(t *testing.T) {
	withEarlyRentalCandidate(t, func(*testing.T, context.Context, *pgx.Conn, *pgx.Conn, string, []Migration) {})
}

// withEarlyRentalCandidate runs the shared fresh/upgrade integrity baseline
// before an additional candidate layer, retaining one owner for DB cleanup.
func withEarlyRentalCandidate(t *testing.T, extra func(*testing.T, context.Context, *pgx.Conn, *pgx.Conn, string, []Migration)) {
	t.Helper()
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
	info, err := os.Lstat(filepath.Join(u.Query().Get("host"), rentalCombinedFixtureMarker))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatal("requires regular mode-0600 isolated-target marker")
	}
	all, err := discoverLegacyV98Migrations(filepath.Join("..", "..", "db", "migrations"))
	if err != nil || len(all) != 98 {
		t.Fatalf("requires exact V1..V98 inventory: %v", err)
	}
	for _, baseline := range []int{0, 75} {
		t.Run(fmt.Sprintf("baseline-%d", baseline), func(t *testing.T) {
			withTestDatabase(t, func(ctx context.Context, databaseURL string) {
				admin := connectTestDB(t, ctx, databaseURL)
				defer admin.Close(context.Background())
				migrationURL, _ := establishRentalCombinedFixture(t, ctx, databaseURL, admin, all, baseline)
				// Each helper's temporary ACL cleanup must finish while admin is
				// open, before withTestDatabase drops this owned database.
				t.Run("v97-upgrade", func(t *testing.T) {
					assertV97AuthorityLifecycle(t, ctx, admin, databaseURL, migrationURL, all[:97])
				})
				t.Run("v98-expand", func(t *testing.T) {
					assertV98SpaceInventoryMigration(t, ctx, admin, migrationURL, all)
				})
				sql, err := os.ReadFile(filepath.Join("testdata", "early_rental_schema_candidate.sql"))
				if err != nil {
					t.Fatal(err)
				}
				dir := prefixMigrationDirForCombined(t, all, 98)
				writeMigration(t, dir, "V000099__early_rental_schema_candidate.sql", string(sql))
				inventory, err := DiscoverMigrations(dir)
				if err != nil {
					t.Fatal(err)
				}
				target := SingleOwnerMigrationTarget{Host: admin.Config().Host, Port: admin.Config().Port}
				if err := admin.QueryRow(ctx, `SELECT current_database(),d.oid,(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),current_setting('server_version_num')::int,session_user::text,(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.ocupacion'::regclass),(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.schema_migrations'::regclass) FROM pg_database d WHERE datname=current_database()`).Scan(&target.Database, &target.DatabaseOID, &target.SystemIdentifier, &target.ServerVersionNum, &target.SessionUser, &target.OccupancyOwner, &target.LedgerOwner); err != nil {
					t.Fatal(err)
				}
				if target.OccupancyOwner != "postgres" || target.LedgerOwner != "postgres" || target.SessionUser != "postgres" {
					t.Fatal("unexpected fixture ownership")
				}
				const revoke = `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner; REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner; REVOKE REFERENCES(id) ON public.espacio FROM espacigo_schema_owner`
				defer func() {
					if _, err := admin.Exec(context.Background(), revoke); err != nil {
						t.Errorf("close own candidate DDL window: %v", err)
					}
				}()
				if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner; GRANT REFERENCES(id) ON public.usuario TO espacigo_schema_owner; GRANT REFERENCES(id) ON public.espacio TO espacigo_schema_owner`); err != nil {
					t.Fatal(err)
				}
				contract := SingleOwnerMigrationContract{InventorySHA256: inventoryFingerprint(inventory), Version: 99, Name: inventory[98].Name, Checksum: inventory[98].Checksum, Target: target}
				result, err := RunSingleOwnerMigration(ctx, admin, dir, contract)
				if err != nil || !result.Committed || !result.Verified {
					t.Fatalf("candidate execution: %+v %v", result, err)
				}
				if _, err := admin.Exec(ctx, revoke); err != nil {
					t.Fatal(err)
				}
				reader := connectTestDB(t, ctx, databaseURL)
				defer reader.Close(context.Background())
				var catalog bool
				if err := reader.QueryRow(ctx, `SELECT
					(SELECT count(*)=1 FROM pg_constraint WHERE conrelid='public.ocupacion'::regclass AND conname='ocupacion_no_solapa_excl' AND contype='x')
					AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','SELECT')
					AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','INSERT')
					AND has_table_privilege('espacigo_runtime','public.arriendo_temprano','UPDATE')
					AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano','DELETE')
					AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano','TRUNCATE')
					AND NOT has_schema_privilege('espacigo_schema_owner','public','CREATE')
					AND NOT has_column_privilege('espacigo_schema_owner','public.usuario','id','REFERENCES')
					AND NOT has_column_privilege('espacigo_schema_owner','public.espacio','id','REFERENCES')
					AND (SELECT pg_get_userbyid(relowner)='espacigo_schema_owner' FROM pg_class WHERE oid='public.arriendo_temprano'::regclass)
					AND (SELECT pg_get_userbyid(relowner)='postgres' FROM pg_class WHERE oid='public.ocupacion'::regclass)
					AND (SELECT pg_get_userbyid(relowner)='postgres' FROM pg_class WHERE oid='public.schema_migrations'::regclass)`).Scan(&catalog); err != nil || !catalog {
					t.Fatalf("candidate catalog/ACL/owners: %t %v", catalog, err)
				}
				f := newEarlyRentalFixture(t, ctx, admin)
				t.Run("commit-valid-pair-independent-session", func(t *testing.T) {
					tx, err := admin.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback(context.Background())
					if err := f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
						t.Fatal(err)
					}
					if err := f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, "retencion_temprana", nil, f.deadline, true); err != nil {
						t.Fatal(err)
					}
					if err := tx.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					var n int
					if err := reader.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano a JOIN public.ocupacion o ON o.id=a.ocupacion_id AND o.arriendo_temprano_id=a.id AND o.espacio_id=a.espacio_id AND o.intervalo=a.intervalo WHERE a.id=$1`, f.offerID).Scan(&n); err != nil || n != 1 {
						t.Fatalf("committed pair=%d %v", n, err)
					}
				})
				for _, tc := range []struct{ name, code, constraint string }{
					{"no-link", "23503", "arriendo_temprano_ocupacion_fk"},
					{"no-offer", "23503", "ocupacion_temprana_agregado_fk"},
					{"space-mismatch", "23503", "arriendo_temprano_ocupacion_fk"},
					{"range-mismatch", "23503", "arriendo_temprano_ocupacion_fk"},
					{"id-mismatch", "23503", "arriendo_temprano_ocupacion_fk"},
					{"second-inverse-link", "23503", "ocupacion_temprana_agregado_fk"},
					{"partial-null", "23514", "ocupacion_tipo_reserva_ck"},
					{"wrong-type", "23514", "ocupacion_tipo_reserva_ck"},
					{"reservation-with-reason", "23514", "ocupacion_tipo_reserva_ck"},
					{"hold-without-expiry", "23514", "ocupacion_temprana_tipo_expiracion_ck"},
					{"overlap-sqlstate", "23P01", "ocupacion_no_solapa_excl"},
				} {
					t.Run("rollback-"+tc.name, func(t *testing.T) {
						before := earlyRentalCounts(t, ctx, reader)
						tx, err := admin.Begin(ctx)
						if err != nil {
							t.Fatal(err)
						}
						defer tx.Rollback(context.Background())
						id, occ := uuid.NewString(), uuid.NewString()
						start := f.start.Add(4 * time.Hour)
						if tc.name == "overlap-sqlstate" {
							start = f.start
						}
						if tc.name != "no-offer" && tc.name != "second-inverse-link" {
							err = f.insertOffer(ctx, tx, id, occ, f.spaceID, start)
						}
						if err != nil {
							t.Fatal(err)
						}
						if tc.name != "no-link" {
							space, link, occupancyStart := f.spaceID, any(id), start
							typ, reason, expiry, active := "reserva_temprana", any(nil), any(nil), true
							switch tc.name {
							case "space-mismatch":
								space = f.otherSpace
							case "range-mismatch":
								occupancyStart = start.Add(time.Hour)
							case "id-mismatch":
								occ = uuid.NewString()
							case "second-inverse-link":
								link, active = f.offerID, false
								occupancyStart = f.start
							case "partial-null":
								link = nil
							case "wrong-type":
								typ = "bloqueo_manual"
							case "reservation-with-reason":
								reason = "x"
							case "hold-without-expiry":
								typ = "retencion_temprana"
							}
							err = f.insertOccupancy(ctx, tx, occ, link, space, occupancyStart, typ, reason, expiry, active)
						}
						if err == nil {
							err = tx.Commit(ctx)
						} else if rb := tx.Rollback(ctx); rb != nil {
							t.Fatal(rb)
						}
						var pgErr *pgconn.PgError
						if !errors.As(err, &pgErr) || pgErr.Code != tc.code || pgErr.ConstraintName != tc.constraint {
							t.Fatalf("want %s/%s got %v", tc.code, tc.constraint, err)
						}
						if after := earlyRentalCounts(t, ctx, reader); after != before {
							t.Fatalf("rollback counts before=%v after=%v", before, after)
						}
					})
				}
				t.Run("legacy-preserved", func(t *testing.T) {
					start := f.start.Add(10 * time.Hour)
					id := uuid.NewString()
					if _, err := admin.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,intervalo,tipo,motivo) VALUES($1,$2,tstzrange($3,$4,'[)'),'bloqueo_manual','Synthetic legacy block')`, id, f.spaceID, start, start.Add(time.Hour)); err != nil {
						t.Fatal(err)
					}
					var valid bool
					if err := reader.QueryRow(ctx, `SELECT tipo='bloqueo_manual' AND reserva_id IS NULL AND arriendo_temprano_id IS NULL AND activo FROM public.ocupacion WHERE id=$1`, id).Scan(&valid); err != nil || !valid {
						t.Fatalf("legacy readback=%t %v", valid, err)
					}
					_, err := admin.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,intervalo,tipo,motivo) VALUES($1,$2,tstzrange($3,$4,'[)'),'bloqueo_manual','Overlap must fail')`, uuid.NewString(), f.spaceID, f.start, f.start.Add(time.Hour))
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "23P01" || pgErr.ConstraintName != "ocupacion_no_solapa_excl" {
						t.Fatalf("legacy/early exclusion: %v", err)
					}
				})
				extra(t, ctx, admin, reader, databaseURL, inventory)
			})
		})
	}
}

type earlyRentalFixture struct {
	owner, renter, spaceID, otherSpace, offerID, occupancyID string
	created, deadline, start                                 time.Time
}

func newEarlyRentalFixture(t *testing.T, ctx context.Context, db *pgx.Conn) *earlyRentalFixture {
	t.Helper()
	f := &earlyRentalFixture{owner: uuid.NewString(), renter: uuid.NewString(), spaceID: uuid.NewString(), otherSpace: uuid.NewString(), offerID: uuid.NewString(), occupancyID: uuid.NewString()}
	f.created = time.Now().UTC().Truncate(time.Microsecond)
	f.deadline = f.created.Add(30 * time.Minute)
	f.start = f.created.Add(48 * time.Hour).Truncate(time.Hour)
	for _, id := range []string{f.owner, f.renter} {
		if _, err := db.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'synthetic-only','activo')`, id, id+"@early-rental.invalid"); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{f.spaceID, f.otherSpace} {
		if _, err := db.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado,zona_horaria) VALUES($1,$2,'oficina','Early rental fixture',repeat('Synthetic fixture description. ',4),10,1,'Synthetic','hora',10000,'Synthetic','activa','UTC')`, id, f.owner); err != nil {
			t.Fatal(err)
		}
	}
	// withTestDatabase owns and drops the entire fixture database after closing
	// all connections. Row deletion cannot safely break the deferred FK cycle
	// with ON DELETE RESTRICT, and is not the application's retention policy.
	return f
}

func (f *earlyRentalFixture) insertOffer(ctx context.Context, tx pgx.Tx, id, occupancy, space string, start time.Time) error {
	body := "synthetic early rental contract"
	sum := sha256.Sum256([]byte(body))
	_, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano(id,espacio_id,anfitrion_id,arrendatario_id,titulo_snapshot,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,contrato_version,contrato_texto,contrato_digest,estado,revision,creada_en,actualizada_en,vence_en,ocupacion_id) VALUES($1,$2,$3,$4,'Early rental fixture','hora',10000,'CLP',1,10000,$5,$6,'UTC',1,$7,$8,'pendiente_identidad',1,$9,$9,$10,$11)`, id, space, f.owner, f.renter, start, start.Add(time.Hour), body, sum[:], f.created, f.deadline, occupancy)
	return err
}

func (f *earlyRentalFixture) insertOccupancy(ctx context.Context, tx pgx.Tx, id string, link any, space string, start time.Time, kind string, reason, expiry any, active bool) error {
	var deactivated any
	if !active {
		deactivated = f.created
	}
	_, err := tx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,intervalo,tipo,motivo,expira_en,activo,desactivada_en,arriendo_temprano_id) VALUES($1,$2,tstzrange($3,$4,'[)'),$5,$6,$7,$8,$9,$10)`, id, space, start, start.Add(time.Hour), kind, reason, expiry, active, deactivated, link)
	return err
}

func earlyRentalCounts(t *testing.T, ctx context.Context, db *pgx.Conn) [2]int {
	t.Helper()
	var counts [2]int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.arriendo_temprano),(SELECT count(*) FROM public.ocupacion)`).Scan(&counts[0], &counts[1]); err != nil {
		t.Fatal(err)
	}
	return counts
}
