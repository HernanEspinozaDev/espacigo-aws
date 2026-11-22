package commercialproposalspg

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	contracttemplatespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/contracttemplates"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommercialProposalPostgresImmutableSourceSnapshots(t *testing.T) {
	if os.Getenv("TEST_COMMERCIAL_PROPOSALS_POSTGRES") != "1" {
		t.Skip("set TEST_COMMERCIAL_PROPOSALS_POSTGRES=1 with administrative local TEST_DATABASE_URL")
	}
	ctx := context.Background()
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatal("loopback PostgreSQL URL required")
	}
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := "test_contratos_proposals_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	}()
	u.Path = "/" + name
	setup, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close(ctx)
	const fixture = `GRANT USAGE,CREATE ON SCHEMA public TO espacigo_schema_owner; GRANT USAGE ON SCHEMA public TO espacigo_runtime;
SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner;
CREATE TABLE public.usuario(id uuid PRIMARY KEY);
CREATE TABLE public.espacio(id uuid PRIMARY KEY,propietario_id uuid NOT NULL REFERENCES public.usuario(id),estado text NOT NULL,zona_horaria text,direccion text,ubicacion_latitud double precision,ubicacion_longitud double precision,ubicacion_confirmada_en timestamptz,ubicacion_revision bigint DEFAULT 1,region_codigo text,comuna_codigo text);
CREATE TABLE public.tarifa_espacio(espacio_id uuid,version integer,modalidad text,precio_base_clp bigint,moneda text);
CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY,espacio_id uuid,propietario_id uuid,arrendatario_id uuid,titulo_espacio text,tarifa_version integer,modalidad text,precio_unitario_clp bigint,unidades bigint,subtotal_clp bigint,moneda text,inicio timestamptz,termino timestamptz,zona_horaria text,vence_en timestamptz);
GRANT SELECT,UPDATE ON public.espacio TO espacigo_runtime;
GRANT SELECT,DELETE ON public.cotizacion_arriendo_comercial TO espacigo_runtime;
GRANT SELECT ON public.tarifa_espacio TO espacigo_runtime;`
	if _, err = setup.Exec(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"V000117__commercial_space_contract_templates.sql", "V000118__commercial_contract_proposals.sql"} {
		sql, e := os.ReadFile("../../../../db/migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = setup.Exec(ctx, string(sql)); e != nil {
			t.Fatal(file, e)
		}
	}
	owner, renter, space, quoteID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err = setup.Exec(ctx, `INSERT INTO public.usuario(id) VALUES($1),($2)`, owner, renter); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,estado,zona_horaria,direccion,ubicacion_latitud,ubicacion_longitud,ubicacion_confirmada_en,region_codigo,comuna_codigo) VALUES($1,$2,'activa','America/Santiago','Dirección sintética 123',-33.4,-70.6,now(),'13','13101')`, space, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.tarifa_espacio VALUES($1,1,'hora',10000,'CLP')`, space); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if _, err = setup.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial VALUES($1,$2,$3,$4,'Sala sintética',1,'hora',10000,1,10000,'CLP',$5,$6,'America/Santiago',now()+interval '15 minutes')`, quoteID, space, owner, renter, start, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	config.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, e := c.Exec(ctx, `SET ROLE espacigo_runtime`)
		return e
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	templates := contracttemplatespg.New(pool)
	sections := []contracttemplate.Section{{Key: "uso", Title: "Uso", Body: "Reglas sintéticas."}}
	if _, err = templates.Save(ctx, owner, space, 0, sections); err != nil {
		t.Fatal(err)
	}
	if _, _, err = templates.Publish(ctx, owner, space, 1); err != nil {
		t.Fatal(err)
	}
	r := New(pool)
	quote := rentalflow.Quote{ID: quoteID, SpaceID: space, OwnerID: owner, RenterID: renter}
	policy := commercialproposal.CancellationPolicy{Version: "synthetic-v1", Text: "Política sintética de prueba."}
	terms, err := r.CurrentTerms(ctx, renter, quote, policy)
	if err != nil {
		t.Fatal(err)
	}
	record := commercialproposal.Record{ID: uuid.NewString(), OwnerID: owner, RenterID: renter, Terms: terms, SHA256: strings.Repeat("a", 64), KeyVersion: "synthetic-v1", Ciphertext: []byte(strings.Repeat("x", 40))}
	stored, err := r.Create(ctx, renter, quote, record, func() error { return nil })
	if err != nil || stored.Version != 1 {
		t.Fatalf("create proposal %+v: %v", stored, err)
	}
	record.ID = uuid.NewString()
	replayed, err := r.Create(ctx, renter, quote, record, func() error { return nil })
	if err != nil || replayed.ID != stored.ID {
		t.Fatalf("replay %+v: %v", replayed, err)
	}
	if _, err = r.Get(ctx, uuid.NewString(), stored.ID); !errors.Is(err, commercialproposal.ErrNotFound) {
		t.Fatalf("third party read: %v", err)
	}
	if _, err = setup.Exec(ctx, `UPDATE public.contrato_propuesta_comercial SET version=9 WHERE id=$1`, stored.ID); err == nil {
		t.Fatal("published proposal mutated")
	}
	if _, err = setup.Exec(ctx, `UPDATE public.espacio SET direccion='Dirección nueva 456',ubicacion_revision=2 WHERE id=$1`, space); err != nil {
		t.Fatal(err)
	}
	record.SHA256 = strings.Repeat("b", 64)
	if _, err = r.Create(ctx, renter, quote, record, func() error { return nil }); !errors.Is(err, commercialproposal.ErrConflict) {
		t.Fatalf("stale source accepted: %v", err)
	}
	if _, err = setup.Exec(ctx, `DELETE FROM public.cotizacion_arriendo_comercial WHERE id=$1`, quoteID); err != nil {
		t.Fatalf("historical proposal blocked quote purge: %v", err)
	}
	old, err := r.Get(ctx, renter, stored.ID)
	if err != nil || old.Terms.Location.Address != terms.Location.Address || old.Terms.TemplateVersion != 1 {
		t.Fatalf("immutable historical source lost %+v %v", old, err)
	}
}
