package spacespg

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresDraftCRUDAndOwnerScope(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is required for disposable PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("space_draft_test_%d", time.Now().UnixNano())
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
		if err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role.name).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			loginClause := "NOLOGIN"
			if role.login {
				loginClause = "LOGIN"
			}
			if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role.name}.Sanitize()+" "+loginClause+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
				t.Fatal(err)
			}
			createdRoles = append(createdRoles, role.name)
		}
	}
	if _, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		for _, role := range createdRoles {
			_, _ = admin.Exec(context.Background(), `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize())
		}
	})
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		if _, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `GRANT CONNECT ON DATABASE `+pgx.Identifier{name}.Sanitize()+` TO espacigo_migrator_login;
		GRANT USAGE, CREATE ON SCHEMA public TO espacigo_schema_owner;
		GRANT espacigo_schema_owner TO espacigo_migrator_login WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;`); err != nil {
		t.Fatal(err)
	}
	migrations, err := migrator.DiscoverMigrations("../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	migConn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migConn.Exec(ctx, `SET SESSION AUTHORIZATION espacigo_migrator_login; SET ROLE espacigo_schema_owner;`); err != nil {
		_ = migConn.Close(ctx)
		t.Fatal(err)
	}
	if _, err = migConn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version bigint PRIMARY KEY CHECK (version > 0),
		name text NOT NULL UNIQUE,
		checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
		runner_checksum text NOT NULL CHECK (runner_checksum ~ '^[0-9a-f]{64}$'),
		applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
	)`); err != nil {
		_ = migConn.Close(ctx)
		t.Fatal(err)
	}
	for _, m := range migrations {
		tx, txErr := migConn.Begin(ctx)
		if txErr != nil {
			_ = migConn.Close(ctx)
			t.Fatal(txErr)
		}
		if _, execErr := tx.Conn().PgConn().Exec(ctx, string(m.SQL)).ReadAll(); execErr != nil {
			_ = tx.Rollback(ctx)
			_ = migConn.Close(ctx)
			t.Fatalf("execute migration %s: %v", m.Name, execErr)
		}
		if _, recErr := tx.Exec(ctx, `INSERT INTO public.schema_migrations(version,name,checksum,runner_checksum) VALUES($1,$2,$3,$4)`, m.Version, m.Name, m.Checksum, strings.Repeat("0", 64)); recErr != nil {
			_ = tx.Rollback(ctx)
			_ = migConn.Close(ctx)
			t.Fatalf("record migration %s: %v", m.Name, recErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			_ = migConn.Close(ctx)
			t.Fatalf("commit migration %s: %v", m.Name, commitErr)
		}
	}
	_ = migConn.Close(ctx)
	ownerA, ownerB := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for _, id := range []string{ownerA, ownerB} {
		if _, err := pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'x','activo')`, id, id+"@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	repo := New(pool, credentials.Generator{})
	categories, err := repo.Categories(ctx)
	if err != nil || len(categories) != 9 {
		t.Fatalf("category catalog count=%d err=%v", len(categories), err)
	}
	for _, category := range categories {
		profile, err := repo.Profile(ctx, category.Code, 0)
		if err != nil || profile.CategoryCode != category.Code || profile.SchemaVersion != 1 || len(profile.Attributes) == 0 {
			t.Fatalf("category %s profile=%+v err=%v", category.Code, profile, err)
		}
		for index, attribute := range profile.Attributes {
			if attribute.Order != index+1 || attribute.Code == "" || attribute.Label == "" || attribute.Type == "" {
				t.Fatalf("category %s has incomplete/unordered attribute %+v", category.Code, attribute)
			}
		}
	}
	in := spaces.Input{CategoryCode: "oficina", Title: "Oficina", Description: strings.Repeat("Espacio privado sintético. ", 5), AreaM2: 20, Capacity: 6, UsageRules: "No fumar", RateUnit: "dia", BasePriceCLP: 9000, Address: "Dirección privada de prueba"}
	in.AttributeSchemaVersion = 1
	in.Attributes = map[string]any{"puestos_trabajo": float64(6), "banos_disponibles": float64(0), "wifi": false}
	created, err := repo.Create(ctx, ownerA, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != "borrador" {
		t.Fatalf("state=%q", created.State)
	}
	var roleCount, eventCount int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.rol_usuario WHERE usuario_id=$1 AND rol='arrendador'), (SELECT count(*) FROM public.rol_usuario_evento WHERE usuario_id=$1 AND rol='arrendador' AND accion='primer_borrador_espacio' AND recurso_id=$2)`, ownerA, created.ID).Scan(&roleCount, &eventCount); err != nil {
		t.Fatal(err)
	}
	if roleCount != 1 || eventCount != 1 {
		t.Fatalf("first draft role grant not atomic/audited: roles=%d events=%d", roleCount, eventCount)
	}
	if created.AttributeSchemaVersion != 1 || created.Attributes["wifi"] != false || created.Attributes["puestos_trabajo"] != float64(6) || created.Attributes["banos_disponibles"] != float64(0) {
		t.Fatalf("created attributes not preserved: %+v", created)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.categoria_perfil_atributos(categoria_codigo,version,perfil) SELECT categoria_codigo,2,jsonb_set(perfil,'{schema_version}','2'::jsonb) FROM public.categoria_perfil_atributos WHERE categoria_codigo='oficina' AND version=1`); err != nil {
		t.Fatal(err)
	}
	latest, err := repo.Profile(ctx, "oficina", 0)
	if err != nil || latest.SchemaVersion != 2 {
		t.Fatalf("latest profile=%+v err=%v", latest, err)
	}
	old, err := repo.Profile(ctx, "oficina", 1)
	if err != nil || old.SchemaVersion != 1 {
		t.Fatalf("v1 profile=%+v err=%v", old, err)
	}
	service, err := spaces.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	editV1 := in
	editV1.AttributeSchemaVersion = 0 // The service must preserve the draft's stored version.
	editV1.Title = "Oficina editada con el perfil v1"
	editV1.RegionCode, editV1.CommuneCode = "13", "13123"
	updatedV1, err := service.UpdateOwn(ctx, ownerA, created.ID, editV1)
	if err != nil || updatedV1.AttributeSchemaVersion != 1 || updatedV1.Title != editV1.Title || updatedV1.RegionCode == nil || *updatedV1.RegionCode != "13" || updatedV1.CommuneCode == nil || *updatedV1.CommuneCode != "13123" {
		t.Fatalf("editing v1 draft after v2: %+v err=%v", updatedV1, err)
	}
	invalidLocation := editV1
	invalidLocation.CommuneCode = "13999"
	if _, err = service.UpdateOwn(ctx, ownerA, created.ID, invalidLocation); err != spaces.ErrInvalid {
		t.Fatalf("draft with an unknown commune code error=%v", err)
	}
	stillLocated, err := repo.GetOwn(ctx, ownerA, created.ID)
	if err != nil || stillLocated.CommuneCode == nil || *stillLocated.CommuneCode != "13123" {
		t.Fatalf("invalid location update changed saved location: %+v err=%v", stillLocated, err)
	}
	// PostgreSQL normalizes UUID output to lowercase; uppercase input remains the same identifier.
	editV1.AttributeSchemaVersion = 1
	editV1.Title = "Actualización con UUID mayúsculo"
	updatedUpper, err := service.UpdateOwn(ctx, ownerA, strings.ToUpper(created.ID), editV1)
	if err != nil || updatedUpper.Title != editV1.Title {
		t.Fatalf("uppercase UUID update=%+v err=%v", updatedUpper, err)
	}
	readUpper, err := repo.GetOwn(ctx, ownerA, created.ID)
	if err != nil || readUpper.Title != editV1.Title || readUpper.AttributeSchemaVersion != 1 {
		t.Fatalf("uppercase UUID update was not committed: %+v err=%v", readUpper, err)
	}
	invalidChange := in
	invalidChange.CategoryCode = "sala_multiproposito"
	invalidChange.AttributeSchemaVersion = 99
	invalidChange.Attributes = map[string]any{"proyector": true}
	if _, err := repo.UpdateOwn(ctx, ownerA, created.ID, invalidChange); err == nil {
		t.Fatal("update with missing destination profile unexpectedly succeeded")
	}
	afterRollback, err := repo.GetOwn(ctx, ownerA, created.ID)
	if err != nil || afterRollback.CategoryCode != "oficina" || afterRollback.Attributes["wifi"] != false {
		t.Fatalf("failed category change was not rolled back: %+v err=%v", afterRollback, err)
	}
	items, err := repo.ListOwn(ctx, ownerA)
	if err != nil || len(items) != 1 {
		t.Fatalf("owner list=%v err=%v", items, err)
	}
	items, err = repo.ListOwn(ctx, ownerB)
	if err != nil || len(items) != 0 {
		t.Fatalf("foreign list leaked: %v err=%v", items, err)
	}
	if _, err = repo.GetOwn(ctx, ownerB, created.ID); err != spaces.ErrNotFound {
		t.Fatalf("foreign read err=%v", err)
	}
	if _, err = repo.UpdateOwn(ctx, ownerB, created.ID, in); err != spaces.ErrNotFound {
		t.Fatalf("foreign update err=%v", err)
	}
	in.Title = "Oficina editada"
	in.CategoryCode = "sala_multiproposito"
	in.AttributeSchemaVersion = 1
	in.Attributes = map[string]any{"proyector": true}
	updated, err := repo.UpdateOwn(ctx, ownerA, created.ID, in)
	if err != nil || updated.Title != in.Title || updated.CategoryCode != in.CategoryCode || updated.CategoryName != "Sala o espacio multipropósito" {
		t.Fatalf("update=%v err=%v", updated, err)
	}
	if updated.AttributeSchemaVersion != 1 || updated.Attributes["proyector"] != true || updated.Attributes["wifi"] != nil {
		t.Fatalf("category change retained incompatible attributes: %+v", updated)
	}
	if updated.RegionCode != nil || updated.CommuneCode != nil {
		t.Fatalf("full draft replacement did not clear omitted optional location: %+v", updated)
	}
	updated.BasePriceCLP = 12000
	if _, err = service.UpdateOwn(ctx, ownerA, created.ID, updatedInput(in, updated)); err != nil {
		t.Fatal(err)
	}
	var tariffVersions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.tarifa_espacio WHERE espacio_id=$1`, created.ID).Scan(&tariffVersions); err != nil || tariffVersions != 2 {
		t.Fatalf("draft update tariff history versions=%d err=%v", tariffVersions, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.espacio SET estado='publicado' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("database accepted commercial state")
	}
}

func updatedInput(previous spaces.Input, draft spaces.Draft) spaces.Input {
	previous.CategoryCode = draft.CategoryCode
	previous.Title = draft.Title
	previous.Description = draft.Description
	previous.AreaM2 = draft.AreaM2
	previous.Capacity = draft.Capacity
	previous.UsageRules = draft.UsageRules
	previous.RateUnit = draft.RateUnit
	previous.BasePriceCLP = draft.BasePriceCLP
	previous.Address = draft.Address
	previous.AttributeSchemaVersion = draft.AttributeSchemaVersion
	previous.Attributes = draft.Attributes
	return previous
}
