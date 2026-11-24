package commercialagreementspg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testIdentityGate struct{ denied atomic.Bool }

func (g *testIdentityGate) RequireForBooking(context.Context, string, string, time.Time) error {
	if g.denied.Load() {
		return errors.New("verification no longer current")
	}
	return nil
}

// Opt-in creates and destroys its own database. It exercises actual V117-V119
// as migrator/schema owner and all aggregate writes as runtime. The minimal
// foundation below models pre-existing tables; the full upgrade is a separate CI gate.
func TestCommercialAgreementPostgresAtomicSignaturesAndHolds(t *testing.T) {
	if os.Getenv("TEST_COMMERCIAL_AGREEMENTS_POSTGRES") != "1" {
		t.Skip("set TEST_COMMERCIAL_AGREEMENTS_POSTGRES=1 with administrative local TEST_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("explicit local PostgreSQL URL required")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && !(host == "" && filepath.IsAbs(u.Query().Get("host"))) {
		t.Fatal("loopback or private Unix socket PostgreSQL required")
	}
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := "test_contratos_agreements_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	}()
	u.Path = "/" + name
	setup, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close(context.Background())
	if _, err = setup.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS btree_gist; GRANT USAGE,CREATE ON SCHEMA public TO espacigo_schema_owner; GRANT USAGE ON SCHEMA public TO espacigo_runtime; SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner;`); err != nil {
		t.Fatal(err)
	}
	const foundation = `CREATE TABLE public.usuario(id uuid PRIMARY KEY,estado text NOT NULL DEFAULT 'activo',correo_original text);
CREATE TABLE public.rol_usuario(usuario_id uuid,rol text,PRIMARY KEY(usuario_id,rol));
CREATE TABLE public.bloqueo_cuenta_administrativo_local(cuenta_id uuid PRIMARY KEY);
CREATE TABLE public.espacio(id uuid PRIMARY KEY,propietario_id uuid NOT NULL REFERENCES public.usuario(id),estado text NOT NULL,zona_horaria text,direccion text,ubicacion_latitud double precision,ubicacion_longitud double precision,ubicacion_confirmada_en timestamptz,ubicacion_revision bigint NOT NULL DEFAULT 1,region_codigo text,comuna_codigo text);
CREATE TABLE public.tarifa_espacio(espacio_id uuid,version integer,modalidad text,precio_base_clp bigint,moneda text);
CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY,espacio_id uuid,propietario_id uuid,arrendatario_id uuid,titulo_espacio text,tarifa_version integer,modalidad text,precio_unitario_clp bigint,unidades bigint,subtotal_clp bigint,moneda text,inicio timestamptz,termino timestamptz,zona_horaria text,vence_en timestamptz);
CREATE TABLE public.cotizacion_arriendo_tramo(id uuid PRIMARY KEY,cotizacion_id uuid,inicio timestamptz,termino timestamptz,unidades bigint,orden integer);
CREATE TABLE public.checkout_preferencia_comercial(id uuid PRIMARY KEY,cotizacion_id uuid,espacio_id uuid,estado text,vence_en timestamptz);
CREATE TABLE public.ocupacion(id uuid PRIMARY KEY,espacio_id uuid,intervalo tstzrange NOT NULL,tipo text NOT NULL CONSTRAINT ocupacion_tipo_check CHECK(tipo IN ('bloqueo_manual','retencion','reserva','retencion_temprana','reserva_temprana','retencion_comercial','reserva_comercial')),activo boolean NOT NULL DEFAULT true,expira_en timestamptz,motivo text,reserva_id uuid,arriendo_temprano_id uuid,reserva_arriendo_comercial_id uuid,desactivada_en timestamptz,CONSTRAINT ocupacion_tipo_reserva_ck CHECK(true),EXCLUDE USING gist(espacio_id WITH =,intervalo WITH &&) WHERE(activo));
CREATE TABLE public.espacio_horario_semanal(espacio_id uuid,activo boolean);
CREATE TABLE public.espacio_horario_semanal_tramo(espacio_id uuid,dia_iso integer,apertura_minuto integer,cierre_minuto integer,orden integer);
CREATE TABLE public.espacio_regla_disponibilidad(espacio_id uuid,dias_operacion smallint[],hora_apertura time,hora_cierre time,estadia_minima integer,estadia_maxima integer);
CREATE TABLE public.espacio_calendario_override(espacio_id uuid,fecha date,estado text);
GRANT SELECT,UPDATE ON public.usuario,public.espacio TO espacigo_runtime;
GRANT SELECT ON public.rol_usuario,public.bloqueo_cuenta_administrativo_local,public.tarifa_espacio,public.cotizacion_arriendo_tramo,public.checkout_preferencia_comercial,public.espacio_horario_semanal,public.espacio_horario_semanal_tramo,public.espacio_regla_disponibilidad,public.espacio_calendario_override TO espacigo_runtime;
GRANT SELECT,DELETE ON public.cotizacion_arriendo_comercial TO espacigo_runtime;
GRANT SELECT,INSERT,UPDATE ON public.ocupacion TO espacigo_runtime;`
	if _, err = setup.Exec(ctx, foundation); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		"V000124__commercial_space_contract_templates.sql",
		"V000125__commercial_contract_proposals.sql",
		"V000126__commercial_agreement_signatures.sql",
		"V000128__commercial_contract_document_jobs.sql",
		"V000129__commercial_auto_authorization.sql",
		"V000133__user_in_app_notifications.sql",
		"V000134__fes_reinforced_signatures_and_otp.sql",
		"V000137__commercial_auto_authorization_activation.sql",
	} {
		blob, e := os.ReadFile("../../../../db/migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = setup.Exec(ctx, string(blob)); e != nil {
			t.Fatal(file, e)
		}
	}
	config, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 6
	config.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, e := c.Exec(ctx, `SET ROLE espacigo_runtime`)
		return e
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	base := time.Now().UTC().Truncate(time.Second)
	var clock atomic.Int64
	clock.Store(base.UnixNano())
	gate := &testIdentityGate{}
	repo, _ := New(pool, gate, func() time.Time { return time.Unix(0, clock.Load()).UTC() })
	service, _ := domain.New(repo)
	type fixture struct {
		owner, renter, space, template, quote, proposal, hash string
		start                                                 time.Time
	}
	makeFixtureAt := func(t *testing.T, prior *fixture, startOverride *time.Time) fixture {
		t.Helper()
		clock.Store(base.UnixNano())
		f := fixture{owner: uuid.NewString(), renter: uuid.NewString(), space: uuid.NewString(), template: uuid.NewString(), quote: uuid.NewString(), proposal: uuid.NewString(), start: base.Add(48 * time.Hour).Truncate(24 * time.Hour).Add(10 * time.Hour)}
		if prior != nil {
			f.owner = prior.owner
			f.space = prior.space
			f.template = prior.template
			f.start = prior.start
		}
		if startOverride != nil {
			f.start = *startOverride
		}
		for _, id := range []string{f.owner, f.renter} {
			if _, e := setup.Exec(ctx, `INSERT INTO public.usuario(id,correo_original) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, id+"@example.test"); e != nil {
				t.Fatal(e)
			}
		}
		for _, party := range []struct{ id, role string }{{f.owner, "arrendador"}, {f.renter, "arrendatario"}} {
			if _, e := setup.Exec(ctx, `INSERT INTO public.rol_usuario VALUES($1,$2) ON CONFLICT DO NOTHING`, party.id, party.role); e != nil {
				t.Fatal(e)
			}
		}
		if prior == nil {
			if _, e := setup.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,estado,zona_horaria,direccion,ubicacion_latitud,ubicacion_longitud,ubicacion_confirmada_en,region_codigo,comuna_codigo) VALUES($1,$2,'activa','UTC','Dirección sintética 123',-33.4,-70.6,now(),'13','13101')`, f.space, f.owner); e != nil {
				t.Fatal(e)
			}
			if _, e := setup.Exec(ctx, `INSERT INTO public.tarifa_espacio VALUES($1,1,'hora',10000,'CLP')`, f.space); e != nil {
				t.Fatal(e)
			}
			sections := []contracttemplate.Section{{Key: "uso", Title: "Uso", Body: "Reglas sintéticas."}}
			blob, _, _ := contracttemplate.CanonicalSections(sections)
			if _, e := setup.Exec(ctx, `INSERT INTO public.espacio_contrato_borrador(espacio_id,revision,secciones,version_publicada) VALUES($1,1,($2::text)::jsonb,1)`, f.space, string(blob)); e != nil {
				t.Fatal(e)
			}
			if _, e := setup.Exec(ctx, `INSERT INTO public.espacio_contrato_version(id,espacio_id,version,revision_origen,texto_canonico,secciones,sha256,publicada_por) VALUES($1,$2,1,1,$3::text,($3::text)::jsonb,sha256(convert_to($3::text,'UTF8')),$4)`, f.template, f.space, string(blob), f.owner); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := setup.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial VALUES($1,$2,$3,$4,'Sala sintética',1,'hora',10000,1,10000,'CLP',$5,$6,'UTC',$7)`, f.quote, f.space, f.owner, f.renter, f.start, f.start.Add(time.Hour), base.Add(15*time.Minute)); e != nil {
			t.Fatal(e)
		}
		terms := commercialproposal.Terms{QuoteID: f.quote, SpaceID: f.space, SpaceTitle: "Sala sintética", TemplateID: f.template, TemplateVersion: 1, Location: commercialproposal.Location{Address: "Dirección sintética 123", Latitude: -33.4, Longitude: -70.6, Revision: 1, RegionCode: "13", CommuneCode: "13101"}, Sections: []contracttemplate.Section{{Key: "uso", Title: "Uso", Body: "Reglas sintéticas."}}, RateUnit: "hora", RateVersion: 1, UnitPriceCLP: 10000, Units: 1, SubtotalCLP: 10000, Currency: "CLP", StartAt: f.start, EndAt: f.start.Add(time.Hour), TimeZone: "UTC", CancellationPolicy: commercialproposal.CancellationPolicy{Version: "fixture-v1", Text: "Política sintética."}}
		blob, _ := json.Marshal(terms)
		sum := sha256.Sum256([]byte(f.proposal))
		f.hash = hex.EncodeToString(sum[:])
		if _, e := setup.Exec(ctx, `INSERT INTO public.contrato_propuesta_comercial(id,cotizacion_id,espacio_id,propietario_id,arrendatario_id,version,plantilla_version_id,inicio,termino,zona_horaria,subtotal_clp,moneda,terminos,terminos_canonicos,terminos_sha256,documento_sha256,identidades_cifradas,clave_version) VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,'UTC',10000,'CLP',($9::text)::jsonb,$9::text,sha256(convert_to($9::text,'UTF8')),decode($10,'hex'),decode(repeat('ab',40),'hex'),'fixture')`, f.proposal, f.quote, f.space, f.owner, f.renter, f.template, f.start, f.start.Add(time.Hour), string(blob), f.hash); e != nil {
			t.Fatal(e)
		}
		return f
	}
	makeFixture := func(t *testing.T, prior *fixture) fixture {
		return makeFixtureAt(t, prior, nil)
	}
	actor := func(id string) domain.Actor { return domain.Actor{ID: id, SessionID: uuid.NewString()} }
	create := func(t *testing.T, f fixture, key string) domain.Agreement {
		t.Helper()
		a, e := service.Create(ctx, actor(f.renter), domain.CreateInput{ProposalID: f.proposal, SHA256: f.hash, Consent: true, ConsentVersion: domain.ConsentVersion}, key)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	sign := func(f fixture, a domain.Agreement) (domain.Agreement, error) {
		return service.Accept(ctx, actor(f.owner), a.ID, a.Revision, domain.SignInput{SHA256: f.hash, Consent: true, ConsentVersion: domain.ConsentVersion})
	}
	t.Run("idempotency privacy and revoked identity", func(t *testing.T) {
		f := makeFixture(t, nil)
		a := create(t, f, "once")
		if a.State != domain.Pending || len(a.Signatures) != 1 {
			t.Fatalf("bad initial evidence %+v", a)
		}
		again := create(t, f, "once")
		if again.ID != a.ID || !again.Replayed || !again.Signatures[0].SignedAt.Equal(a.Signatures[0].SignedAt) {
			t.Fatal("create replay changed evidence")
		}
		if _, e := service.Get(ctx, uuid.NewString(), a.ID); !errors.Is(e, domain.ErrNotFound) {
			t.Fatalf("third party read %v", e)
		}
		if _, e := service.Accept(ctx, actor(f.renter), a.ID, a.Revision, domain.SignInput{SHA256: f.hash, Consent: true, ConsentVersion: domain.ConsentVersion}); !errors.Is(e, domain.ErrNotFound) {
			t.Fatalf("renter countersigned %v", e)
		}
		bad := domain.CreateInput{ProposalID: f.proposal, SHA256: strings.Repeat("a", 64), Consent: true, ConsentVersion: domain.ConsentVersion}
		if _, e := service.Create(ctx, actor(f.renter), bad, "once"); !errors.Is(e, domain.ErrIdempotency) {
			t.Fatalf("key payload not bound %v", e)
		}
		if _, e := service.Accept(ctx, actor(f.owner), a.ID, a.Revision+1, domain.SignInput{SHA256: f.hash, Consent: true, ConsentVersion: domain.ConsentVersion}); !errors.Is(e, domain.ErrPrecondition) {
			t.Fatalf("stale CAS accepted %v", e)
		}
		gate.denied.Store(true)
		if _, e := sign(f, a); !errors.Is(e, domain.ErrIdentity) {
			t.Fatalf("revoked identity signed %v", e)
		}
		gate.denied.Store(false)
		var n int
		if e := setup.QueryRow(ctx, `SELECT count(*) FROM public.ocupacion WHERE solicitud_comercial_id=$1`, a.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("failed gate wrote hold", e, n)
		}
	})
	t.Run("concurrent second signature keeps one immutable hold", func(t *testing.T) {
		f := makeFixture(t, nil)
		a := create(t, f, "double-sign")
		var wg sync.WaitGroup
		results := make(chan domain.Agreement, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); value, e := sign(f, a); results <- value; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var occupancy string
		for value := range results {
			if value.State != domain.Ready || len(value.Signatures) != 2 || value.PaymentExpiresAt == nil || !value.PaymentExpiresAt.Equal(base.Add(15*time.Minute)) {
				t.Fatalf("invalid acceptance %+v", value)
			}
			if occupancy != "" && *value.OccupancyID != occupancy {
				t.Fatal("duplicate holds")
			}
			occupancy = *value.OccupancyID
		}
		for table, want := range map[string]int{"contrato_firma_comercial": 2, "contrato_solicitud_evento_comercial": 2, "contrato_solicitud_notice_outbox": 2} {
			var n int
			if e := setup.QueryRow(ctx, `SELECT count(*) FROM public.`+table+` WHERE solicitud_id=$1`, a.ID).Scan(&n); e != nil || n != want {
				t.Fatalf("%s=%d want%d err%v", table, n, want, e)
			}
		}
		if _, e := setup.Exec(ctx, `UPDATE public.contrato_firma_comercial SET sesion_id=$2 WHERE solicitud_id=$1`, a.ID, uuid.NewString()); e == nil {
			t.Fatal("signature evidence was mutable")
		}
		other := makeFixture(t, &f)
		competing := create(t, other, "competing")
		if _, e := sign(other, competing); !errors.Is(e, domain.ErrConflict) {
			t.Fatalf("overlapping second approval accepted %v", e)
		}
	})
	t.Run("withdraw releases quote anchor and keeps replay history", func(t *testing.T) {
		f := makeFixture(t, nil)
		a := create(t, f, "withdraw")
		if _, e := setup.Exec(ctx, `DELETE FROM public.cotizacion_arriendo_comercial WHERE id=$1`, f.quote); e == nil {
			t.Fatal("active quote purged")
		}
		withdrawn, e := service.Decide(ctx, actor(f.renter), a.ID, a.Revision, domain.DecisionInput{SHA256: f.hash, Decision: "withdraw"})
		if e != nil || withdrawn.State != domain.Withdrawn {
			t.Fatal(e, withdrawn)
		}
		if _, e = setup.Exec(ctx, `DELETE FROM public.cotizacion_arriendo_comercial WHERE id=$1`, f.quote); e != nil {
			t.Fatal("terminal request retained quote anchor", e)
		}
		replayed := create(t, f, "withdraw")
		if replayed.State != domain.Withdrawn || !replayed.Replayed {
			t.Fatal("terminal replay created new request")
		}
	})
	t.Run("expired ready hold is released atomically", func(t *testing.T) {
		f := makeFixture(t, nil)
		a := create(t, f, "expire-ready")
		ready, e := sign(f, a)
		if e != nil {
			t.Fatal(e)
		}
		clock.Store(base.Add(16 * time.Minute).UnixNano())
		if _, e = service.ExpireDue(ctx, 100); e != nil {
			t.Fatal(e)
		}
		got, e := service.Get(ctx, f.renter, a.ID)
		if e != nil || got.State != domain.Expired || got.Revision != 3 {
			t.Fatal("request expiration not durable", got, e)
		}
		var active bool
		if e = setup.QueryRow(ctx, `SELECT activo FROM public.ocupacion WHERE id=$1`, *ready.OccupancyID).Scan(&active); e != nil || active {
			t.Fatal("expired hold remained active", e)
		}
	})
	t.Run("checkout ownership prevents withdrawal and request expiry", func(t *testing.T) {
		f := makeFixture(t, nil)
		a := create(t, f, "checkout-owned")
		ready, e := sign(f, a)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = setup.Exec(ctx, `INSERT INTO public.checkout_preferencia_comercial VALUES($1,$2,$3,'pending',$4)`, uuid.NewString(), f.quote, f.space, base.Add(15*time.Minute)); e != nil {
			t.Fatal(e)
		}
		if _, e = service.Decide(ctx, actor(f.renter), a.ID, ready.Revision, domain.DecisionInput{SHA256: f.hash, Decision: "withdraw"}); !errors.Is(e, domain.ErrConflict) {
			t.Fatalf("checkout-owned hold withdrawn: %v", e)
		}
		clock.Store(base.Add(16 * time.Minute).UnixNano())
		if _, e = service.ExpireDue(ctx, 100); e != nil {
			t.Fatal(e)
		}
		got, e := service.Get(ctx, f.owner, a.ID)
		if e != nil || got.State != domain.Ready {
			t.Fatal("request worker altered checkout-owned agreement", got, e)
		}
	})
	t.Run("bounded auto authorization confirms eligible requests directly to ready_for_payment", func(t *testing.T) {
		f := makeFixture(t, nil)
		grantID := uuid.NewString()
		if _, e := setup.Exec(ctx, `INSERT INTO public.autorizacion_arriendo_comercial(id,espacio_id,propietario_id,plantilla_version,monto_maximo_clp,vigente_desde,vence_en,creada_en) VALUES($1,$2,$3,1,10000,$4,$5,$4)`, grantID, f.space, f.owner, base.Add(-10*time.Second), base.Add(24*time.Hour)); e != nil {
			t.Fatal(e)
		}
		a := create(t, f, "auto-1")
		if a.State != domain.Ready || a.Revision != 2 || a.PaymentExpiresAt == nil || len(a.Signatures) != 2 {
			t.Fatalf("expected immediate ready_for_payment with 2 signatures, got %+v", a)
		}
		var hasLandlordAuto bool
		for _, sig := range a.Signatures {
			if sig.Role == "arrendador" && sig.AuthMethod == "auto_authorization" && sig.ConsentVersion == domain.ConsentVersion {
				hasLandlordAuto = true
			}
		}
		if !hasLandlordAuto {
			t.Fatalf("missing landlord auto_authorization signature: %+v", a.Signatures)
		}
		var holdCount int
		if e := setup.QueryRow(ctx, `SELECT count(*) FROM public.ocupacion WHERE solicitud_comercial_id=$1 AND tipo='retencion_contrato_comercial' AND activo`, a.ID).Scan(&holdCount); e != nil || holdCount != 1 {
			t.Fatalf("expected active hold for auto-confirmed request: count=%d err=%v", holdCount, e)
		}
		var autoEventCount int
		if e := setup.QueryRow(ctx, `SELECT count(*) FROM public.autorizacion_arriendo_comercial_evento WHERE autorizacion_id=$1 AND accion='auto_confirmed'`, grantID).Scan(&autoEventCount); e != nil || autoEventCount != 1 {
			t.Fatalf("expected 1 auto_confirmed event: count=%d err=%v", autoEventCount, e)
		}
		// Second non-overlapping request under the same grant must also auto-confirm.
		s2 := f.start.Add(2 * time.Hour)
		f2 := makeFixtureAt(t, &f, &s2)
		a2 := create(t, f2, "auto-2")
		if a2.State != domain.Ready || len(a2.Signatures) != 2 {
			t.Fatalf("second auto-confirm failed: %+v", a2)
		}
		if e := setup.QueryRow(ctx, `SELECT count(*) FROM public.autorizacion_arriendo_comercial_evento WHERE autorizacion_id=$1 AND accion='auto_confirmed'`, grantID).Scan(&autoEventCount); e != nil || autoEventCount != 2 {
			t.Fatalf("expected 2 auto_confirmed events: count=%d err=%v", autoEventCount, e)
		}
		// Over cap falls back to manual landlord signature (pending_landlord).
		if _, e := setup.Exec(ctx, `UPDATE public.autorizacion_arriendo_comercial SET revocada_en=$2,revocada_por=$3,motivo_revocacion='owner_request' WHERE id=$1`, grantID, base, f.owner); e != nil {
			t.Fatal(e)
		}
		lowCapID := uuid.NewString()
		if _, e := setup.Exec(ctx, `INSERT INTO public.autorizacion_arriendo_comercial(id,espacio_id,propietario_id,plantilla_version,monto_maximo_clp,vigente_desde,vence_en,creada_en) VALUES($1,$2,$3,1,9999,$4,$5,$4)`, lowCapID, f.space, f.owner, base.Add(-10*time.Second), base.Add(24*time.Hour)); e != nil {
			t.Fatal(e)
		}
		s3 := f.start.Add(4 * time.Hour)
		f3 := makeFixtureAt(t, &f, &s3)
		a3 := create(t, f3, "over-cap")
		if a3.State != domain.Pending || len(a3.Signatures) != 1 {
			t.Fatalf("over-cap request should stay pending_landlord, got %+v", a3)
		}
	})
}
