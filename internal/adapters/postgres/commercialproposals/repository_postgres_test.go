package commercialproposalspg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	contracttemplatespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/contracttemplates"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
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
CREATE TABLE public.organizacion_miembro(organizacion_id uuid NOT NULL,usuario_id uuid NOT NULL,rol text NOT NULL,estado text NOT NULL,PRIMARY KEY(organizacion_id,usuario_id));
CREATE TABLE public.espacio(id uuid PRIMARY KEY,propietario_id uuid NOT NULL REFERENCES public.usuario(id),estado text NOT NULL,zona_horaria text,direccion text,ubicacion_latitud double precision,ubicacion_longitud double precision,ubicacion_confirmada_en timestamptz,ubicacion_revision bigint DEFAULT 1,region_codigo text,comuna_codigo text,modo_anfitrion text NOT NULL DEFAULT 'personal',organizacion_id uuid);
CREATE TABLE public.tarifa_espacio(espacio_id uuid,version integer,modalidad text,precio_base_clp bigint,moneda text);
CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY,espacio_id uuid,propietario_id uuid,arrendatario_id uuid,titulo_espacio text,tarifa_version integer,modalidad text,precio_unitario_clp bigint,unidades bigint,subtotal_clp bigint,moneda text,inicio timestamptz,termino timestamptz,zona_horaria text,vence_en timestamptz,politica_cancelacion_version text,politica_cancelacion_texto text);
GRANT SELECT ON public.organizacion_miembro TO espacigo_runtime;
GRANT SELECT,UPDATE ON public.espacio TO espacigo_runtime;
GRANT SELECT,DELETE ON public.cotizacion_arriendo_comercial TO espacigo_runtime;
GRANT SELECT ON public.tarifa_espacio TO espacigo_runtime;`
	if _, err = setup.Exec(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"V000124__commercial_space_contract_templates.sql", "V000125__commercial_contract_proposals.sql"} {
		sql, e := os.ReadFile("../../../../db/migrations/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = setup.Exec(ctx, string(sql)); e != nil {
			t.Fatal(file, e)
		}
	}
	if _, err = setup.Exec(ctx, `ALTER TABLE public.contrato_propuesta_comercial ADD COLUMN es_b2b boolean NOT NULL DEFAULT false, ADD COLUMN organizacion_id uuid, ADD COLUMN empresa_razon_social_snapshot varchar(200), ADD COLUMN empresa_rut_masked_snapshot varchar(16), ADD COLUMN empresa_rut_verificada boolean NOT NULL DEFAULT false, ADD COLUMN representante_legal_id uuid, ADD COLUMN representante_nombre_snapshot varchar(200), ADD COLUMN representante_rut_masked_snapshot varchar(16), ADD COLUMN representante_rut_verificada boolean NOT NULL DEFAULT false`); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `CREATE FUNCTION public.kyb_space_eligibility(uuid,timestamptz DEFAULT clock_timestamp()) RETURNS TABLE(can_publish boolean,can_book boolean,space_verified boolean,compliance_status text) LANGUAGE sql AS $$ SELECT true,true,false,'grace_active'::text $$; GRANT EXECUTE ON FUNCTION public.kyb_space_eligibility(uuid,timestamptz) TO espacigo_runtime;`); err != nil {
		t.Fatal(err)
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
	if _, err = setup.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial(id,espacio_id,propietario_id,arrendatario_id,titulo_espacio,tarifa_version,modalidad,precio_unitario_clp,unidades,subtotal_clp,moneda,inicio,termino,zona_horaria,vence_en,politica_cancelacion_version,politica_cancelacion_texto) VALUES($1,$2,$3,$4,'Sala sintética',1,'hora',10000,1,10000,'CLP',$5,$6,'America/Santiago',now()+interval '15 minutes','flexible_v1','Términos congelados en la cotización.')`, quoteID, space, owner, renter, start, start.Add(time.Hour)); err != nil {
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
	if terms.CancellationPolicy.Version != "flexible_v1" || terms.CancellationPolicy.Text != "Términos congelados en la cotización." {
		t.Fatalf("current terms ignored quote cancellation snapshot: %+v", terms.CancellationPolicy)
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
	if _, err = setup.Exec(ctx, `UPDATE public.cotizacion_arriendo_comercial SET politica_cancelacion_version='estricta_v1',politica_cancelacion_texto='Términos posteriores.' WHERE id=$1`, quoteID); err != nil {
		t.Fatal(err)
	}
	record.ID = uuid.NewString()
	record.SHA256 = strings.Repeat("c", 64)
	if _, err = r.Create(ctx, renter, quote, record, func() error { return nil }); !errors.Is(err, commercialproposal.ErrConflict) {
		t.Fatalf("changed quote cancellation snapshot accepted: %v", err)
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

func TestCommercialProposalPostgresMultiRepresentativeCompanyAccess(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required for disposable PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatal("loopback PostgreSQL URL required")
	}
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())

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

	name := "test_contratos_multirep_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanCancel()
		if _, e := admin.Exec(cleanCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
		for _, role := range createdRoles {
			_, _ = admin.Exec(cleanCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize())
		}
	}()

	u.Path = "/" + name
	dbURL := u.String()
	setup, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close(context.Background())
	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		if _, err := setup.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
			t.Fatalf("create extension %s: %v", ext, err)
		}
	}
	if _, err := setup.Exec(ctx, `GRANT CONNECT ON DATABASE `+pgx.Identifier{name}.Sanitize()+` TO espacigo_migrator_login;
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
	if err := dbbootstrap.GrantRuntimePermissions(ctx, setup); err != nil {
		t.Fatal(err)
	}

	orgID := uuid.NewString()
	owner, rep2, adminMember, operatorMember, inactiveRep, renter := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	space, quoteID := uuid.NewString(), uuid.NewString()
	if _, err = setup.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES
		($1,'owner-multirep@example.test','owner-multirep@example.test','fixture','activo'),
		($2,'rep2-multirep@example.test','rep2-multirep@example.test','fixture','activo'),
		($3,'admin-multirep@example.test','admin-multirep@example.test','fixture','activo'),
		($4,'op-multirep@example.test','op-multirep@example.test','fixture','activo'),
		($5,'inact-multirep@example.test','inact-multirep@example.test','fixture','activo'),
		($6,'renter-multirep@example.test','renter-multirep@example.test','fixture','activo')`,
		owner, rep2, adminMember, operatorMember, inactiveRep, renter); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.organizacion(id,usuario_id,created_by_usuario_id,legal_name) VALUES($1,$2,$2,'Empresa Sintética SpA')`, orgID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.organizacion_miembro(organizacion_id,usuario_id,rol,estado,joined_at) VALUES
		($1,$2,'representante_legal','activo',now()),
		($1,$3,'representante_legal','activo',now()),
		($1,$4,'administrador','activo',now()),
		($1,$5,'operador','activo',now()),
		($1,$6,'representante_legal','suspendido',now())`,
		orgID, owner, rep2, adminMember, operatorMember, inactiveRep); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,ubicacion_catalogo_version,region_codigo,comuna_codigo,ubicacion_latitud,ubicacion_longitud,ubicacion_confirmada_en,zona_horaria,es_b2b,organizacion_id,modo_anfitrion)
		VALUES($1,$2,'oficina','Sala empresa','Fixture multirep',30,2,'Reglas','hora',10000,'Dirección empresa 123','SUBDERE_DPA_2018','13','13101',-33.4,-70.6,now(),'America/Santiago',true,$3,'empresa')`,
		space, owner, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `UPDATE public.espacio SET estado='activa' WHERE id=$1`, space); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) VALUES($1,1,'hora',10000)`, space); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if _, err = setup.Exec(ctx, `INSERT INTO public.cotizacion_arriendo_comercial(id,espacio_id,propietario_id,arrendatario_id,titulo_espacio,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,creada_en,vence_en,politica_cancelacion_version,politica_cancelacion_texto)
		VALUES($1,$2,$3,$4,'Sala empresa',1,'hora',10000,'CLP',1,10000,$5,$6,'America/Santiago',now(),now()+interval '6 minutes','flexible_v1','Términos congelados en la cotización.')`,
		quoteID, space, owner, renter, start, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	templateID := uuid.NewString()
	sections := []contracttemplate.Section{{Key: "reparto_pago_hotusoft", Title: "Regla del anfitrión", Body: "Reglas empresa."}}
	for index := len(sections); index < 12; index++ {
		sections = append(sections, contracttemplate.Section{Key: fmt.Sprintf("regla_%d", index), Title: "Regla", Body: "Texto de prueba."})
	}
	rawClauses, err := json.Marshal(sections)
	if err != nil {
		t.Fatal(err)
	}
	clauses := string(rawClauses)
	if _, err = setup.Exec(ctx, `INSERT INTO public.espacio_contrato_borrador(espacio_id,revision,secciones,version_publicada) VALUES($1,1,$2::jsonb,1)`, space, clauses); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.espacio_contrato_version(id,espacio_id,version,revision_origen,texto_canonico,secciones,sha256,publicada_por) VALUES($1,$2,1,1,$3,$4::jsonb,sha256(convert_to($3,'UTF8')),$5)`, templateID, space, clauses, clauses, owner); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dbURL)
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
	r := New(pool)
	quote := rentalflow.Quote{ID: quoteID, SpaceID: space, OwnerID: owner, RenterID: renter}
	policy := commercialproposal.CancellationPolicy{Version: "synthetic-v1", Text: "Política sintética de prueba."}

	for _, blocked := range []string{adminMember, operatorMember, inactiveRep} {
		if _, err = r.CurrentTerms(ctx, blocked, quote, policy); !errors.Is(err, commercialproposal.ErrConflict) {
			t.Fatalf("blocked member %s unexpectedly read CurrentTerms: %v", blocked, err)
		}
	}

	terms, err := r.CurrentTerms(ctx, rep2, quote, policy)
	if err != nil {
		t.Fatalf("second active legal representative CurrentTerms failed: %v", err)
	}
	if terms.CompanyRepresentation == nil || terms.CompanyRepresentation.OrganizationID != orgID {
		t.Fatalf("expected company representation for org %s, got %+v", orgID, terms.CompanyRepresentation)
	}
	terms.CompanyRepresentation = &commercialproposal.CompanyRepresentation{
		OrganizationID:            orgID,
		LegalName:                 "Empresa Sintética SpA",
		MaskedRUT:                 "**.***.*28-5",
		RUTVerified:               true,
		RepresentativeUserID:      owner,
		RepresentativeName:        "Representante Uno",
		RepresentativeRUT:         "**.***.*11-1",
		RepresentativeRUTVerified: true,
	}
	record := commercialproposal.Record{ID: uuid.NewString(), OwnerID: owner, RenterID: renter, Terms: terms, SHA256: strings.Repeat("d", 64), KeyVersion: "synthetic-v1", Ciphertext: []byte(strings.Repeat("x", 40))}
	for _, blocked := range []string{adminMember, operatorMember, inactiveRep} {
		if _, err = r.Create(ctx, blocked, quote, record, func() error { return nil }); !errors.Is(err, commercialproposal.ErrConflict) {
			t.Fatalf("blocked member %s unexpectedly created proposal: %v", blocked, err)
		}
	}
	badRepTerms := terms
	badRep := *terms.CompanyRepresentation
	badRep.RepresentativeUserID = adminMember
	badRepTerms.CompanyRepresentation = &badRep
	badRecord := record
	badRecord.Terms = badRepTerms
	if _, err = r.Create(ctx, rep2, quote, badRecord, func() error { return nil }); !errors.Is(err, commercialproposal.ErrConflict) {
		t.Fatalf("administrator accepted as representative signatory: %v", err)
	}

	stored, err := r.Create(ctx, rep2, quote, record, func() error { return nil })
	if err != nil || stored.Version != 1 || !stored.CompanyRepresentativeAuthorized {
		t.Fatalf("second active legal representative Create failed: %+v err=%v", stored, err)
	}
	if len(stored.Terms.Sections) != 13 || stored.Terms.Sections[0] != sections[0] {
		t.Fatalf("proposal did not preserve all 12 owner sections: %+v", stored.Terms.Sections)
	}
	paymentSection := stored.Terms.Sections[12]
	if paymentSection.Key != "reparto_pago_hotusoft_1" || !strings.Contains(paymentSection.Body, "HOTU SOFTWARE SOLUTIONS SPA") {
		t.Fatalf("proposal did not persist the platform payment clause: %+v", paymentSection)
	}
	gotByRep2, err := r.Get(ctx, rep2, stored.ID)
	if err != nil || gotByRep2.ID != stored.ID || !gotByRep2.CompanyRepresentativeAuthorized {
		t.Fatalf("second active legal representative Get failed: %+v err=%v", gotByRep2, err)
	}
	for _, blocked := range []string{adminMember, operatorMember, inactiveRep} {
		if _, err = r.Get(ctx, blocked, stored.ID); !errors.Is(err, commercialproposal.ErrNotFound) {
			t.Fatalf("blocked member %s unexpectedly read proposal: %v", blocked, err)
		}
	}
}
