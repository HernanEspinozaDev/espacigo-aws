package contracttemplatespg

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This opt-in suite creates and drops its own database. It never migrates or
// truncates the database named in TEST_DATABASE_URL. The local PostgreSQL must
// already have the repository's migrator, owner and runtime roles.
func TestContractTemplatePostgresIsolationAndPublication(t *testing.T) {
	if os.Getenv("TEST_CONTRACT_TEMPLATES_POSTGRES") != "1" {
		t.Skip("set TEST_CONTRACT_TEMPLATES_POSTGRES=1 with a local administrative TEST_DATABASE_URL")
	}
	ctx := context.Background()
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatal("a loopback PostgreSQL URL is required")
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect to isolated PostgreSQL: ", err)
	}
	defer admin.Close(ctx)
	name := "test_contratos_templates_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if _, err = setup.Exec(ctx, `GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner; GRANT USAGE ON SCHEMA public TO espacigo_runtime; SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner; CREATE TABLE public.usuario(id uuid PRIMARY KEY); CREATE TABLE public.espacio(id uuid PRIMARY KEY, propietario_id uuid NOT NULL REFERENCES public.usuario(id),estado text NOT NULL); GRANT SELECT, UPDATE ON public.espacio TO espacigo_runtime;`); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../../db/migrations/V000124__commercial_space_contract_templates.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, string(sql)); err != nil {
		t.Fatal("migration under migrator session/schema owner role: ", err)
	}
	var session, effective string
	if err = setup.QueryRow(ctx, `SELECT session_user,current_user`).Scan(&session, &effective); err != nil || session != "espacigo_migrator_login" || effective != "espacigo_schema_owner" {
		t.Fatalf("wrong migration authority %s/%s: %v", session, effective, err)
	}
	actor, outsider, space, copySpace, foreignSpace := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err = setup.Exec(ctx, `INSERT INTO public.usuario(id) VALUES($1),($2)`, actor, outsider); err != nil {
		t.Fatal(err)
	}
	if _, err = setup.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,estado) VALUES($1,$2,'borrador'),($3,$2,'activa'),($4,$5,'activa')`, space, actor, copySpace, foreignSpace, outsider); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	config.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, `SET ROLE espacigo_runtime`)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	r := New(pool)
	d, err := r.Get(ctx, actor, space)
	if err != nil || d.Revision != 0 || d.Sections == nil {
		t.Fatalf("initial draft %+v %v", d, err)
	}
	if _, err = r.Get(ctx, outsider, space); !errors.Is(err, contracttemplate.ErrNotFound) {
		t.Fatalf("outsider read: %v", err)
	}
	clauses := []contracttemplate.Section{{Key: "uso", Title: "Uso permitido", Body: "Mantener limpio y respetar los horarios."}}
	d, err = r.Save(ctx, actor, space, 0, clauses)
	if err != nil || d.Revision != 1 {
		t.Fatalf("save: %+v %v", d, err)
	}
	if _, err = r.Save(ctx, actor, space, 0, clauses); !errors.Is(err, contracttemplate.ErrPrecondition) {
		t.Fatalf("stale save: %v", err)
	}
	v, revision, err := r.Publish(ctx, actor, space, 1)
	if err != nil || v.Version != 1 || revision != 2 {
		t.Fatalf("publish: %+v %d %v", v, revision, err)
	}
	replay, _, err := r.Publish(ctx, actor, space, 1)
	if err != nil || replay.ID != v.ID {
		t.Fatalf("publication replay: %+v %v", replay, err)
	}
	if _, err = setup.Exec(ctx, `UPDATE public.espacio_contrato_version SET version=2 WHERE id=$1`, v.ID); err == nil {
		t.Fatal("owner changed immutable published version")
	}
	if _, err = pool.Exec(ctx, `UPDATE public.espacio_contrato_version SET version=2 WHERE id=$1`, v.ID); err == nil {
		t.Fatal("runtime can update published versions")
	}
	if _, err = r.Copy(ctx, actor, copySpace, 0, foreignSpace, 1); !errors.Is(err, contracttemplate.ErrNotFound) {
		t.Fatalf("copy from another owner: %v", err)
	}
	copied, err := r.Copy(ctx, actor, copySpace, 0, space, 1)
	if err != nil || copied.Revision != 1 || copied.PublishedVersion != 0 || copied.Sections[0] != clauses[0] {
		t.Fatalf("independent copied draft: %+v %v", copied, err)
	}
	clauses[0].Body = "Nueva versión de reglas."
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := r.Save(ctx, actor, space, 2, clauses); results <- e }()
	}
	wg.Wait()
	close(results)
	wins, stale := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, contracttemplate.ErrPrecondition) {
			stale++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || stale != 1 {
		t.Fatalf("concurrent writes wins=%d stale=%d", wins, stale)
	}
	v2, _, err := r.Publish(ctx, actor, space, 3)
	if err != nil || v2.Version != 2 || v2.SHA256 == v.SHA256 {
		t.Fatalf("new immutable version: %+v %v", v2, err)
	}
	versions, err := r.Versions(ctx, actor, space)
	if err != nil || len(versions) != 2 || versions[1].ID != v.ID || versions[1].Sections[0].Body == clauses[0].Body {
		t.Fatalf("historical publication changed: %+v %v", versions, err)
	}
	var owner string
	if err = setup.QueryRow(ctx, `SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.espacio_contrato_version'::regclass`).Scan(&owner); err != nil || owner != "espacigo_schema_owner" {
		t.Fatalf("wrong object owner %s: %v", owner, err)
	}
}
