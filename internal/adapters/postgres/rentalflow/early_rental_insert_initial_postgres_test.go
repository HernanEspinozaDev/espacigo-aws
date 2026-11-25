package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"

	"os"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const earlyInsertOptIn = "TEST_RENTALFLOW_POSTGRES_ISOLATED"

// earlyInsertPostgresFixture owns an isolated V000001 baseline schema.
// Reuse it for adapter tests without duplicating authority or cleanup setup.
func earlyInsertPostgresFixture(t *testing.T) (context.Context, *url.URL) {
	t.Helper()
	if os.Getenv(earlyInsertOptIn) != "1" {
		t.Skip(earlyInsertOptIn + "=1 is required for disposable PostgreSQL integration")
	}
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("TEST_DATABASE_URL is required when PostgreSQL integration is opted in")
	}
	adminConfig, err := validateDisposablePostgresTarget(adminURL, os.Getenv("TEST_RENTALFLOW_POSTGRES_ISOLATION_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := admin.Close(cleanupCtx); err != nil {
			t.Errorf("close admin connection: %v", err)
		}
	})

	roles, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	roleNames := []string{"espacigo_schema_owner", "espacigo_runtime", "espacigo_privacy_cleanup_worker", "espacigo_gadm_bootstrap"}
	for _, role := range roleNames {
		var exists bool
		if err := roles.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1)`, role).Scan(&exists); err != nil {
			_ = roles.Rollback(ctx)
			t.Fatal(err)
		}
		if !exists {
			if _, err := roles.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`); err != nil {
				_ = roles.Rollback(ctx)
				t.Fatalf("create prerequisite role %s: %v", role, err)
			}
		}
		var login, super, createdb, createrole, inherit bool
		if err := roles.QueryRow(ctx, `SELECT rolcanlogin,rolsuper,rolcreatedb,rolcreaterole,rolinherit FROM pg_catalog.pg_roles WHERE rolname=$1`, role).Scan(&login, &super, &createdb, &createrole, &inherit); err != nil {
			_ = roles.Rollback(ctx)
			t.Fatal(err)
		}
		if login || super || createdb || createrole || inherit {
			_ = roles.Rollback(ctx)
			t.Fatalf("prerequisite role %s has unsafe flags (login=%v super=%v createdb=%v createrole=%v inherit=%v)", role, login, super, createdb, createrole, inherit)
		}
	}
	if err := roles.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	dbName := fmt.Sprintf("rentalflow_early_initial_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop owned database: %v", err)
			return
		}
		var remains bool
		if err := admin.QueryRow(cleanupCtx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname=$1)`, dbName).Scan(&remains); err != nil || remains {
			t.Errorf("owned database absence verification: remains=%v err=%v", remains, err)
		}
	})
	dbURL := *adminConfig
	dbURL.Path = "/" + dbName
	extConn, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		if _, err := extConn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
			_ = extConn.Close(context.Background())
			t.Fatalf("create extension %s: %v", ext, err)
		}
	}
	if err := extConn.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Run(ctx, dbURL.String(), "../../../../db/migrations"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}
	return ctx, &dbURL
}

func TestPostgresEarlyRentalInitialInsert(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	scenarios := []struct {
		name string
		kind string
	}{
		{"SuccessCommit", "success"},
		{"CallerRollback", "rollback"},
		{"ActualOccupancyFailure", "occupancy"},
		{"EventInsertFailure", "event"},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			caseCtx, caseCancel := context.WithTimeout(ctx, time.Minute)
			defer caseCancel()
			ids := earlyInsertIDs()
			seedEarlyInsertActors(t, caseCtx, dbURL.String(), ids)
			record := earlyInsertRecord(t, ids, time.Date(2030, 4, 5, 12, 0, 0, 123456000, time.UTC))
			if sc.kind == "occupancy" {
				seedEarlyInsertBlock(t, caseCtx, dbURL.String(), ids, record)
			}
			if sc.kind == "event" {
				installEarlyInsertEventFailure(t, caseCtx, dbURL.String(), ids.offer)
			}
			conn, err := pgx.Connect(caseCtx, dbURL.String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := conn.Close(context.Background()); err != nil {
					t.Errorf("close writer connection: %v", err)
				}
			})
			tx, err := conn.Begin(caseCtx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(caseCtx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
				_ = tx.Rollback(caseCtx)
				t.Fatal(err)
			}
			err = insertEarlyRentalInitial(caseCtx, tx, record)
			if sc.kind == "occupancy" {
				requirePGConstraint(t, err, "23P01", "ocupacion_no_solapa_excl")
				if err := tx.Rollback(caseCtx); err != nil {
					t.Fatal(err)
				}
				assertEarlyInsertCounts(t, caseCtx, dbURL.String(), ids, 0, 0, 1)
				return
			}
			if sc.kind == "event" {
				requirePGConstraint(t, err, "23514", "synthetic_event_insert_failure_ck")
				if err := tx.Rollback(caseCtx); err != nil {
					t.Fatal(err)
				}
				assertEarlyInsertCounts(t, caseCtx, dbURL.String(), ids, 0, 0, 0)
				return
			}
			if err != nil {
				_ = tx.Rollback(caseCtx)
				t.Fatalf("insertEarlyRentalInitial: %v", err)
			}
			assertEarlyInsertCounts(t, caseCtx, dbURL.String(), ids, 0, 0, 0)
			if sc.kind == "rollback" {
				if err := tx.Rollback(caseCtx); err != nil {
					t.Fatal(err)
				}
				assertEarlyInsertCounts(t, caseCtx, dbURL.String(), ids, 0, 0, 0)
				return
			}
			if err := tx.Commit(caseCtx); err != nil {
				t.Fatalf("caller commit (deferred integrity guards): %v", err)
			}
			assertEarlyInsertReadback(t, caseCtx, dbURL.String(), ids, record)
		})
	}
}

type earlyInsertFixtureIDs struct{ host, renter, space, offer, occupancy, blocker string }

func earlyInsertIDs() earlyInsertFixtureIDs {
	return earlyInsertFixtureIDs{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
}
func seedEarlyInsertActors(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs) {
	t.Helper()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	for _, a := range []struct {
		id     string
		suffix string
	}{{ids.host, "host"}, {ids.renter, "renter"}} {
		_, e = c.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'synthetic-test-hash','activo')`, a.id, a.id+"-"+a.suffix+"@example.test")
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e = c.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado,zona_horaria) VALUES($1,$2,'oficina','Synthetic rental',repeat('Synthetic fixture description. ',4),20,2,'Synthetic rules','hora',6000,'Synthetic address','activa','UTC')`, ids.space, ids.host)
	if e != nil {
		t.Fatal(e)
	}
}
func earlyInsertRecord(t *testing.T, ids earlyInsertFixtureIDs, now time.Time) earlyRentalInitialRecord {
	t.Helper()
	start := now.Add(2 * time.Hour)
	offer, event, err := earlyrental.NewOffer(earlyrental.NewOfferInput{ID: ids.offer, HostID: ids.host, RenterID: ids.renter, Snapshot: earlyrental.OfferSnapshotInput{SpaceID: uuid.MustParse(ids.space), SpaceTitle: "Synthetic rental", RateUnit: earlyrental.RateUnitHour, UnitPriceCLP: 6000, Units: 1, StartAt: start, EndAt: start.Add(time.Hour), TimeZone: "UTC", CalendarRateOverride: earlyrental.CalendarRateOverrideAbsent}, ContractVersion: 1, ContractText: "Synthetic manual rental contract"}, now)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := newEarlyRentalInitialRecord(offer, ids.occupancy, event, now)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}
func seedEarlyInsertBlock(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, r earlyRentalInitialRecord) {
	t.Helper()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	_, e = c.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,reserva_id,intervalo,tipo,activo,expira_en,motivo,creada_en,desactivada_en) VALUES($1,$2,NULL,tstzrange($3,$4,'[)'),'bloqueo_manual',true,NULL,'synthetic blocker',$5,NULL)`, ids.blocker, ids.space, r.Offer.Snapshot.StartAt, r.Offer.Snapshot.EndAt, r.CreatedAt)
	if e != nil {
		t.Fatal(e)
	}
}
func installEarlyInsertEventFailure(t *testing.T, ctx context.Context, dsn, offerID string) {
	t.Helper()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	// The identifier is a parsed canonical UUID, never user-provided SQL. Limit
	// failure injection to this fixture; retain every canonical integrity trigger.
	_, e = c.Exec(ctx, `CREATE FUNCTION public.synthetic_event_insert_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic event failure' USING ERRCODE='23514', CONSTRAINT='synthetic_event_insert_failure_ck'; END $$; CREATE TRIGGER synthetic_event_insert_failure BEFORE INSERT ON public.arriendo_temprano_evento FOR EACH ROW WHEN (NEW.arriendo_id='`+uuid.MustParse(offerID).String()+`'::uuid) EXECUTE FUNCTION public.synthetic_event_insert_failure()`)
	if e != nil {
		t.Fatal(e)
	}
}
func requirePGConstraint(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != code || pg.ConstraintName != constraint {
		t.Fatalf("error=%v, want SQLSTATE %s constraint %s", err, code, constraint)
	}
}
func assertEarlyInsertCounts(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, offers, events, occupancies int) {
	t.Helper()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	var o, v, n int
	if e = c.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.arriendo_temprano WHERE id=$1),(SELECT count(*) FROM public.arriendo_temprano_evento WHERE arriendo_id=$1),(SELECT count(*) FROM public.ocupacion WHERE espacio_id=$2)`, ids.offer, ids.space).Scan(&o, &v, &n); e != nil {
		t.Fatal(e)
	}
	if o != offers || v != events || n != occupancies {
		t.Fatalf("fixture counts offers/events/occupancies=%d/%d/%d, want %d/%d/%d", o, v, n, offers, events, occupancies)
	}
}
func assertEarlyInsertReadback(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, r earlyRentalInitialRecord) {
	t.Helper()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	var id, space, host, renter, title, mode, currency, state, occID string
	var price, units, subtotal, version, revision int64
	var start, end, created, updated, expires time.Time
	var digest []byte
	var bounds string
	var contractText, flowVersion string
	var halfOpen bool
	var timezone string
	e = c.QueryRow(ctx, `SELECT id::text,espacio_id::text,anfitrion_id::text,arrendatario_id::text,titulo_snapshot,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,contrato_version,contrato_digest,estado,revision,creada_en,actualizada_en,vence_en,ocupacion_id::text,lower(intervalo)::text||'|'||upper(intervalo)::text FROM public.arriendo_temprano WHERE id=$1`, ids.offer).Scan(&id, &space, &host, &renter, &title, &mode, &price, &currency, &units, &subtotal, &start, &end, &timezone, &version, &digest, &state, &revision, &created, &updated, &expires, &occID, &bounds)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.QueryRow(ctx, `SELECT contrato_texto,flow_version,lower_inc(intervalo) AND NOT upper_inc(intervalo) FROM public.arriendo_temprano WHERE id=$1`, ids.offer).Scan(&contractText, &flowVersion, &halfOpen); e != nil {
		t.Fatal(e)
	}
	expectedDigest := sha256.Sum256([]byte(r.Offer.ContractText))
	if contractText != r.Offer.ContractText || flowVersion != "manual_early_v1" || !halfOpen || string(digest) != string(expectedDigest[:]) {
		t.Fatal("contract text, independent SHA-256, flow version or range bounds differ")
	}
	if id != ids.offer || space != ids.space || host != ids.host || renter != ids.renter || title != r.Offer.Snapshot.SpaceTitle || mode != "hora" || price != 6000 || currency != "CLP" || units != 1 || subtotal != 6000 || timezone != "UTC" || !start.Equal(r.Offer.Snapshot.StartAt) || !end.Equal(r.Offer.Snapshot.EndAt) || version != 1 || string(digest) != string(r.ContractDigest) || state != string(earlyrental.StatePendingIdentity) || revision != 1 || !created.Equal(r.CreatedAt) || !updated.Equal(r.CreatedAt) || !expires.Equal(r.Offer.ExpiresAt) || occID != ids.occupancy || bounds == "" {
		t.Fatalf("offer readback mismatch: id=%s space=%s host=%s renter=%s title=%s mode=%s price=%d currency=%s units=%d subtotal=%d range=%s", id, space, host, renter, title, mode, price, currency, units, subtotal, bounds)
	}
	var eventCount, occCount int
	if e = c.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_evento WHERE arriendo_id=$1 AND id IS NOT NULL AND secuencia=1 AND revision=1 AND tipo='oferta_creada' AND actor_id=$2 AND estado_anterior IS NULL AND estado_nuevo=$3 AND ocurrido_en=$4 AND motivo_codigo IS NULL`, ids.offer, ids.renter, string(earlyrental.StatePendingIdentity), r.CreatedAt).Scan(&eventCount); e != nil {
		t.Fatal(e)
	}
	if e = c.QueryRow(ctx, `SELECT count(*) FROM public.ocupacion WHERE id=$1 AND espacio_id=$2 AND reserva_id IS NULL AND intervalo=tstzrange($3,$4,'[)') AND lower_inc(intervalo) AND NOT upper_inc(intervalo) AND tipo='retencion_temprana' AND activo AND expira_en=$5 AND arriendo_temprano_id=$6 AND motivo IS NULL AND desactivada_en IS NULL AND creada_en=$7`, ids.occupancy, ids.space, r.Offer.Snapshot.StartAt, r.Offer.Snapshot.EndAt, r.Offer.ExpiresAt, ids.offer, r.CreatedAt).Scan(&occCount); e != nil {
		t.Fatal(e)
	}
	if eventCount != 1 || occCount != 1 {
		t.Fatalf("event/occupancy exact rows=%d/%d", eventCount, occCount)
	}
	assertEarlyInsertCounts(t, ctx, dsn, ids, 1, 1, 1)
	var others int
	if e = c.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano WHERE espacio_id<>$1 OR id<>$2`, ids.space, ids.offer).Scan(&others); e != nil {
		t.Fatal(e)
	}
	if others != 0 {
		t.Fatalf("unexpected other offers=%d", others)
	}
}
