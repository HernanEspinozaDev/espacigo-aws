package adminglobal_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devauth"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/fakebooking"
	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	bookingpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/booking"
	gallerypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/gallery"
	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	spacespg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/spaces"
	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/adminlocal"
	bookingdomain "github.com/HernanEspinozaDev/espaciGo/internal/booking"
	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	spacesdomain "github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGlobalBootstrapDirectoryAndLastMasterConcurrency(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_gadm01_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const owner = "00000000-0000-4000-8000-000000000001"
	const other = "00000000-0000-4000-8000-000000000002"
	const legacy = "00000000-0000-4000-8000-000000000003"
	const landlord = "00000000-0000-4000-8000-000000000004"
	const renter = "00000000-0000-4000-8000-000000000005"
	const pending = "00000000-0000-4000-8000-000000000006"
	_, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en)
		VALUES($1,'owner@example.invalid','owner@example.invalid','synthetic','activo','2026-01-01'),
		($2,'other@example.invalid','other@example.invalid','synthetic','activo','2026-01-01'),
		($3,'legacy@example.invalid','legacy@example.invalid','synthetic','activo','2026-01-01'),
		($4,'host@example.invalid','host@example.invalid','synthetic','activo','2026-01-01'),
		($5,'renter@example.invalid','renter@example.invalid','synthetic','activo','2026-01-01'),
		($6,'pending@example.invalid','pending@example.invalid','synthetic','correo_pendiente','2026-01-01')`, owner, other, legacy, landlord, renter, pending)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, name, role string }{{owner, "Owner Test", "arrendatario"}, {other, "Second Master", "arrendatario"}, {legacy, "Legacy Admin", "administrador"}, {landlord, "Test Host", "arrendador"}, {renter, "Test Renter", "arrendatario"}, {pending, "Pending User", "arrendatario"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible) VALUES($1,$2)`, item.id, item.name); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,$2)`, item.id, item.role); err != nil {
			t.Fatal(err)
		}
	}
	corr := "10000000-0000-4000-8000-000000000001"
	if err = adminglobal.Bootstrap(ctx, pool, "00000000-0000-4000-8000-000000000099", "bootstrap", corr); err == nil {
		t.Fatal("unknown target accepted")
	}
	if err = adminglobal.Bootstrap(ctx, pool, pending, "bootstrap", corr); err == nil {
		t.Fatal("unverified target accepted")
	}
	var bootstrapWG sync.WaitGroup
	bootstrapResults := make(chan error, 2)
	for _, candidate := range []string{owner, other} {
		bootstrapWG.Add(1)
		go func(id string) {
			defer bootstrapWG.Done()
			bootstrapResults <- adminglobal.Bootstrap(ctx, pool, id, "bootstrap", corr)
		}(candidate)
	}
	bootstrapWG.Wait()
	close(bootstrapResults)
	twins, rejectedRaces := 0, 0
	for e := range bootstrapResults {
		if e == nil {
			twins++
		} else {
			rejectedRaces++
		}
	}
	if twins != 1 || rejectedRaces != 1 {
		t.Fatalf("concurrent bootstrap results success=%d rejected=%d", twins, rejectedRaces)
	}
	var master string
	if err = pool.QueryRow(ctx, `SELECT usuario_id::text FROM public.administrador_global_bootstrap WHERE singleton=true`).Scan(&master); err != nil {
		t.Fatal(err)
	}
	loser := owner
	if master == owner {
		loser = other
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatalf("same-target replay should be idempotent: %v", err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, loser, "bootstrap", corr); err == nil {
		t.Fatal("different-target replay accepted")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM public.administrador_global WHERE usuario_id=$1`, master); err != nil {
		t.Fatal(err)
	}
	credentialFile := makeLocalCLICredentials(t, u.String(), dbName)
	root := repositoryRoot(source)
	if err = runBootstrapCLI(ctx, root, credentialFile, "recover", master); err != nil {
		t.Fatalf("gadm-admin recover failed to restore the bound UUID: %v", err)
	}
	var restored bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.administrador_global WHERE usuario_id=$1)`, master).Scan(&restored); err != nil || !restored {
		t.Fatalf("recover did not restore membership: restored=%v err=%v", restored, err)
	}
	if err = runBootstrapCLI(ctx, root, credentialFile, "recover", loser); err == nil {
		t.Fatal("gadm-admin recover accepted a different UUID")
	}
	privacyNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	privacyService, err := adminglobal.New(pool, func() time.Time { return privacyNow })
	if err != nil {
		t.Fatal(err)
	}
	adminCreated, err := privacyService.CreatePrivacyRequest(ctx, adminglobal.CreatePrivacyRequestInput{ActorID: master, CorrelationID: corr, AccountID: owner, RightType: "acceso", Reason: "Admin assisted access request", CaseRef: "ADMIN-PRIV-1"})
	if err != nil {
		t.Fatalf("create canonical admin request: %v", err)
	}
	if adminCreated.Status != "pendiente" || adminCreated.CanPublishResponse || adminCreated.RequestedBy != master {
		t.Fatalf("admin request status/eligibility/actor = %s/%v/%s", adminCreated.Status, adminCreated.CanPublishResponse, adminCreated.RequestedBy)
	}
	var dbRequestedBy, dbReason, dbCaseRef string
	if err = pool.QueryRow(ctx, `SELECT solicitado_por::text, motivo_administrativo, caso_ref_administrativo FROM public.solicitud_titular WHERE id=$1`, adminCreated.ID).Scan(&dbRequestedBy, &dbReason, &dbCaseRef); err != nil {
		t.Fatal(err)
	}
	if dbRequestedBy != master || dbReason != "Admin assisted access request" || dbCaseRef != "ADMIN-PRIV-1" {
		t.Fatalf("persisted admin request columns mismatch: actor=%s reason=%q case=%q", dbRequestedBy, dbReason, dbCaseRef)
	}
	const portalRequestID = "20000000-0000-4000-8000-000000000001"
	const nonAccessRequestID = "20000000-0000-4000-8000-000000000002"
	const runtimeRequestID = "20000000-0000-4000-8000-000000000003"
	portalCreatedAt := privacyNow.Add(-2 * time.Hour)
	if _, err = pool.Exec(ctx, `INSERT INTO public.solicitud_titular(id,usuario_id,tipo,canal,estado,solicitada_en) VALUES($1,$4,'acceso','web','en_revision',$5),($2,$4,'supresion','web','en_revision',$5),($3,$4,'acceso','web','en_revision',$5)`, portalRequestID, nonAccessRequestID, runtimeRequestID, owner, portalCreatedAt); err != nil {
		t.Fatal(err)
	}
	listed, err := privacyService.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, PageSize: 100})
	if err != nil {
		t.Fatalf("list canonical and legacy privacy requests: %v", err)
	}
	foundPortal, foundAdmin := false, false
	for _, item := range listed.Items {
		if item.ID == portalRequestID {
			foundPortal = item.CanPublishResponse && !item.CanRecordDispatch
		}
		if item.ID == adminCreated.ID {
			foundAdmin = true
		}
	}
	if !foundPortal || !foundAdmin {
		t.Fatalf("admin list omitted canonical requests or did not mark portal resolution eligible: portal=%v admin=%v", foundPortal, foundAdmin)
	}
	filteredAccess, err := privacyService.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, RightType: "acceso", PageSize: 100})
	if err != nil || len(filteredAccess.Items) != 3 {
		t.Fatalf("right_type=acceso filter count=%d err=%v", len(filteredAccess.Items), err)
	}
	filteredSuppression, err := privacyService.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, RightType: "supresion", PageSize: 100})
	if err != nil || len(filteredSuppression.Items) != 1 || filteredSuppression.Items[0].ID != nonAccessRequestID {
		t.Fatalf("right_type=supresion filter items=%+v err=%v", filteredSuppression.Items, err)
	}
	resolution := adminglobal.ResolvePrivacyRequestInput{ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Decision: "resuelta", Response: "La información de acceso está lista para la persona titular."}
	resolved, err := privacyService.ResolvePrivacyRequest(ctx, resolution)
	if err != nil || resolved.Status != "completada" || !resolved.CanRecordDispatch {
		t.Fatalf("resolve access request: item=%+v err=%v", resolved, err)
	}
	replayedResolution, err := privacyService.ResolvePrivacyRequest(ctx, resolution)
	if err != nil || !replayedResolution.CanRecordDispatch {
		t.Fatalf("same response replay: item=%+v err=%v", replayedResolution, err)
	}
	if _, err = privacyService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Decision: "denegada_fundada", Response: "No puede resolverse sin verificar identidad primero."}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("pending-identity resolution error = %v, want conflict", err)
	}
	var resolutionAudits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.evento_auditoria_local WHERE recurso_id=$1 AND accion='admin.global.privacy_requests.resolve'`, portalRequestID).Scan(&resolutionAudits); err != nil || resolutionAudits != 1 {
		t.Fatalf("resolution audit count=%d err=%v, want 1", resolutionAudits, err)
	}
	var responseNotifications int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.notificacion_usuario WHERE referencia_id=$1 AND usuario_id=$2 AND tipo='privacidad_respuesta'`, portalRequestID, owner).Scan(&responseNotifications); err != nil || responseNotifications != 1 {
		t.Fatalf("response notification count=%d err=%v, want 1", responseNotifications, err)
	}
	var recoverySuccess, recoveryReject int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE resultado='exito'),count(*) FILTER (WHERE resultado='rechazo') FROM public.administrador_global_bootstrap_evento WHERE accion='recuperacion'`).Scan(&recoverySuccess, &recoveryReject); err != nil {
		t.Fatal(err)
	}
	if recoverySuccess != 1 || recoveryReject != 1 {
		t.Fatalf("recover audit counts success=%d rejection=%d; want 1/1", recoverySuccess, recoveryReject)
	}
	overprivileged, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = overprivileged.Exec(ctx, "SET ROLE "+pgx.Identifier{adminglobal.RestrictedLocalBootstrapRole}.Sanitize()); err != nil {
		overprivileged.Close(ctx)
		t.Fatal(err)
	}
	if err = adminglobal.ValidateRestrictedLocalBootstrapConnection(ctx, overprivileged); !errors.Is(err, adminglobal.ErrForbidden) {
		overprivileged.Close(ctx)
		t.Fatalf("restricted role guard accepted an overprivileged login: %v", err)
	}
	if err = overprivileged.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// Exercise the production least-privilege connection contract using a
	// short-lived login in this disposable database only.
	roleSuffix := make([]byte, 6)
	if _, err := rand.Read(roleSuffix); err != nil {
		t.Fatal(err)
	}
	loginRole := "gadm_activation_" + hex.EncodeToString(roleSuffix)
	loginPasswordBytes := make([]byte, 24)
	if _, err := rand.Read(loginPasswordBytes); err != nil {
		t.Fatal(err)
	}
	loginPassword := hex.EncodeToString(loginPasswordBytes)
	if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{loginRole}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD '"+loginPassword+"'"); err != nil {
		t.Fatal(err)
	}
	var restricted *pgxpool.Pool
	defer func() {
		if restricted != nil {
			restricted.Close()
		}
		_, _ = admin.Exec(context.Background(), "REVOKE espacigo_gadm_bootstrap FROM "+pgx.Identifier{loginRole}.Sanitize())
		_, _ = admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+pgx.Identifier{loginRole}.Sanitize())
	}()
	if _, err := admin.Exec(ctx, "GRANT espacigo_gadm_bootstrap TO "+pgx.Identifier{loginRole}.Sanitize()+" WITH INHERIT FALSE, SET TRUE, ADMIN FALSE"); err != nil {
		t.Fatal(err)
	}
	loginURL, err := url.Parse(u.String())
	if err != nil {
		t.Fatal(err)
	}
	loginURL.User = url.UserPassword(loginRole, loginPassword)
	loginCfg, err := pgxpool.ParseConfig(loginURL.String())
	if err != nil {
		t.Fatal(err)
	}
	loginCfg.AfterConnect = adminglobal.SetRestrictedLocalBootstrapRole
	restricted, err = pgxpool.NewWithConfig(ctx, loginCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = restricted.Ping(ctx); err != nil {
		restricted.Close()
		t.Fatalf("restricted connection rejected: %v", err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, master, "bootstrap", corr); err != nil {
		restricted.Close()
		t.Fatalf("restricted same-target bootstrap was rejected: %v", err)
	}
	for _, deniedSQL := range []string{
		"SELECT hash_clave FROM public.usuario LIMIT 1",
		"UPDATE public.usuario SET estado='bloqueado' WHERE id='" + master + "'",
		"UPDATE public.usuario SET id='00000000-0000-4000-8000-000000000099' WHERE id='" + master + "'",
		"DELETE FROM public.administrador_global_bootstrap",
		"TRUNCATE public.administrador_global_bootstrap_evento",
		"SELECT * FROM public.reserva LIMIT 1",
	} {
		if _, deniedErr := restricted.Exec(ctx, deniedSQL); deniedErr == nil {
			restricted.Close()
			t.Errorf("restricted role unexpectedly allowed %s", deniedSQL)
		}
	}
	// The admin connection performs fixture repair, while the activation
	// operation itself is still executed as the restricted CLI role.
	if _, err = pool.Exec(ctx, `DELETE FROM public.administrador_global WHERE usuario_id=$1`, master); err != nil {
		restricted.Close()
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, master, "recover", corr); err != nil {
		restricted.Close()
		t.Fatalf("restricted recovery failed: %v", err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, loser, "recover", corr); err == nil {
		restricted.Close()
		t.Fatal("restricted recovery accepted a different UUID")
	}
	restricted.Close()
	if _, err = admin.Exec(ctx, "REVOKE espacigo_gadm_bootstrap FROM "+pgx.Identifier{loginRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "DROP ROLE "+pgx.Identifier{loginRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var loginStillExists, membershipStillExists bool
	if err = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=$1),
		EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m JOIN pg_catalog.pg_roles r ON r.oid=m.roleid JOIN pg_catalog.pg_roles member ON member.oid=m.member
		WHERE r.rolname='espacigo_gadm_bootstrap' AND member.rolname=$1)`, loginRole).Scan(&loginStillExists, &membershipStillExists); err != nil || loginStillExists || membershipStillExists {
		t.Fatalf("temporary role cleanup failed: exists=%v membership=%v err=%v", loginStillExists, membershipStillExists, err)
	}
	var rejected int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.administrador_global_bootstrap_evento WHERE resultado='rechazo'`).Scan(&rejected); err != nil || rejected < 3 {
		t.Fatalf("bootstrap rejects not audited: count=%d err=%v", rejected, err)
	}
	service, err := adminglobal.New(pool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	const publicationA = "80000000-0000-4000-8000-000000000001"
	const publicationB = "80000000-0000-4000-8000-000000000002"
	for _, id := range []string{publicationA, publicationB} {
		if _, err = pool.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado) VALUES($1,$2,'oficina','Global managed space','Descripción sintética suficientemente larga para cumplir el mínimo de cien caracteres en una ficha local de ensayo.',20,4,'Cuidar los bienes y respetar el horario','hora',12000,'Dirección de ensayo','borrador')`, id, landlord); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) VALUES($1,1,'hora',12000)`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio_caracteristicas(espacio_id,categoria_codigo,perfil_version,valores) SELECT $1,'oficina',max(version),'{}'::jsonb FROM public.categoria_perfil_atributos WHERE categoria_codigo='oficina'`, publicationB); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio_caracteristicas(espacio_id,categoria_codigo,perfil_version,valores) SELECT $1,'oficina',max(version),' {"wifi":true}'::jsonb FROM public.categoria_perfil_atributos WHERE categoria_codigo='oficina'`, publicationA); err != nil {
		t.Fatal(err)
	}
	publicationPage, err := service.ListPublications(ctx, master, corr, adminglobal.PublicationFilter{OwnerID: landlord, Category: "oficina", State: "borrador", PageSize: 1})
	if err != nil || publicationPage.MatchingTotal != 2 || len(publicationPage.Items) != 1 || publicationPage.NextCursor == "" || publicationPage.Items[0].PubliclyVisible || publicationPage.Items[0].BasePriceCLP != "12000" {
		t.Fatalf("publication inventory/filter/page: %+v err=%v", publicationPage, err)
	}
	publicationNext, err := service.ListPublications(ctx, master, corr, adminglobal.PublicationFilter{OwnerID: landlord, Category: "oficina", State: "borrador", PageSize: 1, Cursor: publicationPage.NextCursor})
	if err != nil || len(publicationNext.Items) != 1 || publicationNext.Items[0].ID == publicationPage.Items[0].ID {
		t.Fatalf("publication cursor repeated item: %+v err=%v", publicationNext, err)
	}
	if _, err = service.ListPublications(ctx, master, corr, adminglobal.PublicationFilter{OwnerID: landlord, Category: "bodega", State: "borrador", PageSize: 1, Cursor: publicationPage.NextCursor}); !errors.Is(err, adminglobal.ErrInvalid) {
		t.Fatalf("publication cursor accepted changed filter: %v", err)
	}
	var publicationVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationA).Scan(&publicationVersion); err != nil {
		t.Fatal(err)
	}
	newTitle := "Global correction"
	newPrice := "18000"
	updatedPublication, err := service.UpdatePublication(ctx, master, publicationA, corr, adminglobal.PublicationContentUpdate{Title: &newTitle, BasePriceCLP: &newPrice, ExpectedUpdatedAt: publicationVersion, Reason: "revision_administrativa"})
	if err != nil || updatedPublication.ID != publicationA || updatedPublication.OwnerID != landlord {
		t.Fatalf("global publication update: %+v err=%v", updatedPublication, err)
	}
	if _, err = service.UpdatePublication(ctx, master, publicationA, corr, adminglobal.PublicationContentUpdate{Title: &newTitle, ExpectedUpdatedAt: publicationVersion, Reason: "revision_administrativa"}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale publication update returned %v", err)
	}
	var attributeSchemaVersion int
	if err = pool.QueryRow(ctx, `SELECT perfil_version FROM public.espacio_caracteristicas WHERE espacio_id=$1`, publicationA).Scan(&attributeSchemaVersion); err != nil {
		t.Fatal(err)
	}
	var attributesVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationA).Scan(&attributesVersion); err != nil {
		t.Fatal(err)
	}
	updatedAttributes, err := service.UpdatePublication(ctx, master, publicationA, corr, adminglobal.PublicationContentUpdate{AttributeSchemaVersion: &attributeSchemaVersion, Attributes: map[string]any{"wifi": false}, ExpectedUpdatedAt: attributesVersion, Reason: "correccion_perfil"})
	if err != nil || updatedAttributes.ID != publicationA {
		t.Fatalf("category attribute correction: %+v err=%v", updatedAttributes, err)
	}
	var wifi bool
	if err = pool.QueryRow(ctx, `SELECT (valores->>'wifi')::boolean FROM public.espacio_caracteristicas WHERE espacio_id=$1`, publicationA).Scan(&wifi); err != nil || wifi {
		t.Fatalf("versioned category attribute not persisted: wifi=%v err=%v", wifi, err)
	}
	const parcelID = "80000000-0000-4000-8000-000000000003"
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,estado) VALUES($1,$2,'parcela_eventos','Parcela de ensayo','Descripción sintética suficientemente larga para validar las áreas parciales en la corrección global.',100,20,'Uso de prueba','hora',12000,'Dirección de ensayo','borrador')`, parcelID, landlord); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.tarifa_espacio(espacio_id,version,modalidad,precio_base_clp) VALUES($1,1,'hora',12000)`, parcelID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio_caracteristicas(espacio_id,categoria_codigo,perfil_version,valores) SELECT $1,'parcela_eventos',max(version),'{}'::jsonb FROM public.categoria_perfil_atributos WHERE categoria_codigo='parcela_eventos'`, parcelID); err != nil {
		t.Fatal(err)
	}
	var parcelSchema int
	if err = pool.QueryRow(ctx, `SELECT c.perfil_version,e.actualizado_en FROM public.espacio_caracteristicas c JOIN public.espacio e ON e.id=c.espacio_id WHERE e.id=$1`, parcelID).Scan(&parcelSchema, &publicationVersion); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"superficie_exterior_util_m2", "superficie_cubierta_util_m2"} {
		if _, err = service.UpdatePublication(ctx, master, parcelID, corr, adminglobal.PublicationContentUpdate{AttributeSchemaVersion: &parcelSchema, Attributes: map[string]any{code: 101.0}, ExpectedUpdatedAt: publicationVersion, Reason: "revision_administrativa"}); !errors.Is(err, adminglobal.ErrInvalid) {
			t.Fatalf("administrator accepted %s above the total parcel area: %v", code, err)
		}
	}
	if _, err = service.UpdatePublication(ctx, master, parcelID, corr, adminglobal.PublicationContentUpdate{AttributeSchemaVersion: &parcelSchema, Attributes: map[string]any{"superficie_exterior_util_m2": 80.0, "superficie_cubierta_util_m2": 90.0}, ExpectedUpdatedAt: publicationVersion, Reason: "revision_administrativa"}); err != nil {
		t.Fatalf("valid parcel areas rejected: %v", err)
	}
	var rateCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.tarifa_espacio WHERE espacio_id=$1`, publicationA).Scan(&rateCount); err != nil || rateCount != 2 {
		t.Fatalf("tariff version history count=%d err=%v", rateCount, err)
	}
	const verificationID = "90000000-0000-4000-8000-000000000001"
	if _, err = pool.Exec(ctx, `INSERT INTO public.verificacion(id,usuario_id,tipo,estado,referencia_evidencia,clave_idempotencia,revisor_id,creada_en,resuelta_en) VALUES($1,$2,'kyc','aprobada','fixture:90000000-0000-4000-8000-000000000001','gadm02-kyc',$3,now(),now())`, verificationID, landlord, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.elegibilidad_verificacion_local(usuario_id,tipo,verificacion_id,estado,concedida_en) VALUES($1,'kyc',$2,'elegible',now())`, landlord, verificationID); err != nil {
		t.Fatal(err)
	}
	var draftVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationA).Scan(&draftVersion); err != nil {
		t.Fatal(err)
	}
	activated, err := service.SetPublicationState(ctx, master, publicationA, corr, adminglobal.PublicationStateInput{State: "activa", ExpectedUpdatedAt: draftVersion, Reason: "revision_administrativa"})
	if err != nil || activated.State != "activa" || activated.OwnerID != landlord {
		t.Fatalf("policy-authorized publish: %+v err=%v", activated, err)
	}
	publicVisible, err := service.ListPublications(ctx, master, corr, adminglobal.PublicationFilter{Visible: ptr(true), PageSize: 10})
	if err != nil || publicVisible.MatchingTotal != 1 || publicVisible.Items[0].ID != publicationA {
		t.Fatalf("actual visibility filter: %+v err=%v", publicVisible, err)
	}
	if _, err = service.SetPublicationState(ctx, master, publicationA, corr, adminglobal.PublicationStateInput{State: "activa", ExpectedUpdatedAt: draftVersion, Reason: "revision_administrativa"}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale publication transition returned %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.espacio SET zona_horaria='America/Santiago' WHERE id=$1`, publicationB); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.reserva_ensayo_local_fixture(espacio_id,anfitrion_id,arrendatario_id,habilitada) VALUES($1,$2,$3,true)`, publicationB, landlord, renter); err != nil {
		t.Fatal(err)
	}
	localPayment, err := fakebooking.New([]byte("integration-only-local-payment-webhook-key-32-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	bookingService, err := bookingdomain.NewService(bookingpg.New(pool), credentials.Generator{}, time.Now, localPayment)
	if err != nil {
		t.Fatal(err)
	}
	service.SetWeeklyHoursManager(bookingService)
	adminSchedule, err := service.PublicationSchedule(ctx, master, publicationB, landlord, corr)
	if err != nil || adminSchedule.SpaceID != publicationB || adminSchedule.TimeZone != "America/Santiago" {
		t.Fatalf("global schedule read: %+v err=%v", adminSchedule, err)
	}
	var scheduleVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&scheduleVersion); err != nil {
		t.Fatal(err)
	}
	weekly := bookingdomain.WeeklyHours{SpaceID: publicationB, Enabled: false, Days: make([]bookingdomain.WeeklyDay, 7)}
	for index := range weekly.Days {
		weekly.Days[index] = bookingdomain.WeeklyDay{Weekday: index + 1, Periods: []bookingdomain.WeeklyPeriod{}}
	}
	savedSchedule, err := service.SavePublicationSchedule(ctx, master, publicationB, landlord, corr, scheduleVersion, "revision_administrativa", weekly)
	if err != nil || savedSchedule.SpaceID != publicationB || savedSchedule.Enabled {
		t.Fatalf("global schedule write: %+v err=%v", savedSchedule, err)
	}
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&scheduleVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = bookingService.SaveWeeklyHours(ctx, landlord, publicationB, weekly); err != nil {
		t.Fatalf("owner weekly schedule write: %v", err)
	}
	var ownerScheduleVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&ownerScheduleVersion); err != nil || !ownerScheduleVersion.After(scheduleVersion) {
		t.Fatalf("owner weekly schedule did not advance shared publication version: prior=%s updated=%s err=%v", scheduleVersion, ownerScheduleVersion, err)
	}
	if _, err = service.SavePublicationSchedule(ctx, master, publicationB, landlord, corr, scheduleVersion, "revision_administrativa", weekly); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale admin schedule write after owner update returned %v", err)
	}
	const bookingID = "82000000-0000-4000-8000-000000000001"
	const quoteID = "82000000-0000-4000-8000-000000000002"
	const occupancyID = "82000000-0000-4000-8000-000000000003"
	const rejectedBookingID = "82000000-0000-4000-8000-000000000004"
	const rejectedQuoteID = "82000000-0000-4000-8000-000000000005"
	const rejectedOccupancyID = "82000000-0000-4000-8000-000000000006"
	bookingTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bookingTx.Rollback(ctx)
	if _, err = bookingTx.Exec(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		t.Fatal(err)
	}
	bookingStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	bookingEnd := bookingStart.Add(time.Hour)
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,reserva_id,intervalo,tipo,activo) VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'reserva',true)`, occupancyID, publicationB, bookingID, bookingStart, bookingEnd); err != nil {
		t.Fatal(err)
	}
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.cotizacion_reserva_ensayo(id,espacio_id,anfitrion_id,arrendatario_id,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,condiciones_snapshot,categoria_codigo,perfil_version,perfil_valores_snapshot,creada_en,vence_en) VALUES($1,$2,$3,$4,1,'hora',12000,'CLP',1,12000,$5,$6,'America/Santiago','Condiciones sintéticas de ensayo','oficina',1,'{}'::jsonb,now(),now()+interval '15 minutes')`, quoteID, publicationB, landlord, renter, bookingStart, bookingEnd); err != nil {
		t.Fatal(err)
	}
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.reserva_ensayo_local(id,cotizacion_id,espacio_id,anfitrion_id,arrendatario_id,clave_idempotencia,huella_solicitud,ocupacion_id,estado,precio_unitario_clp,unidades,subtotal_clp,modalidad,moneda,inicio,termino,zona_horaria,condiciones_snapshot,pago_vence_en,creada_en,actualizada_en) VALUES($1,$2,$3,$4,$5,'gadm02-resource-booking',decode(repeat('00',32),'hex'),$6,'pagada',12000,1,12000,'hora','CLP',$7,$8,'America/Santiago','Condiciones sintéticas de ensayo',now()+interval '15 minutes',now(),now())`, bookingID, quoteID, publicationB, landlord, renter, occupancyID, bookingStart, bookingEnd); err != nil {
		t.Fatal(err)
	}
	rejectedStart, rejectedEnd := bookingStart.Add(2*time.Hour), bookingEnd.Add(2*time.Hour)
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.ocupacion(id,espacio_id,reserva_id,intervalo,tipo,activo) VALUES($1,$2,$3,tstzrange($4,$5,'[)'),'reserva',true)`, rejectedOccupancyID, publicationB, rejectedBookingID, rejectedStart, rejectedEnd); err != nil {
		t.Fatal(err)
	}
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.cotizacion_reserva_ensayo(id,espacio_id,anfitrion_id,arrendatario_id,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,condiciones_snapshot,categoria_codigo,perfil_version,perfil_valores_snapshot,creada_en,vence_en) VALUES($1,$2,$3,$4,1,'hora',12000,'CLP',1,12000,$5,$6,'America/Santiago','Condiciones sintéticas de ensayo','oficina',1,'{}'::jsonb,now(),now()+interval '15 minutes')`, rejectedQuoteID, publicationB, landlord, renter, rejectedStart, rejectedEnd); err != nil {
		t.Fatal(err)
	}
	if _, err = bookingTx.Exec(ctx, `INSERT INTO public.reserva_ensayo_local(id,cotizacion_id,espacio_id,anfitrion_id,arrendatario_id,clave_idempotencia,huella_solicitud,ocupacion_id,estado,precio_unitario_clp,unidades,subtotal_clp,modalidad,moneda,inicio,termino,zona_horaria,condiciones_snapshot,pago_vence_en,creada_en,actualizada_en) VALUES($1,$2,$3,$4,$5,'gadm02-resource-booking-rejected',decode(repeat('00',32),'hex'),$6,'rechazada_arrendador',12000,1,12000,'hora','CLP',$7,$8,'America/Santiago','Condiciones sintéticas de ensayo',now()+interval '15 minutes',now(),now())`, rejectedBookingID, rejectedQuoteID, publicationB, landlord, renter, rejectedOccupancyID, rejectedStart, rejectedEnd); err != nil {
		t.Fatal(err)
	}
	if err = bookingTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	resourcePage, err := service.ListAccountResources(ctx, master, renter, corr, "reservations", "", 10)
	if err != nil || resourcePage.MatchingTotal != 2 || len(resourcePage.Items) != 2 || resourcePage.Items[0].Participant != "arrendatario" || resourcePage.Items[1].Participant != "arrendatario" {
		t.Fatalf("account reservation resources: %+v err=%v", resourcePage, err)
	}
	resourceIDs := map[string]bool{}
	for _, item := range resourcePage.Items {
		resourceIDs[item.ID] = true
	}
	if !resourceIDs[bookingID] || !resourceIDs[rejectedBookingID] {
		t.Fatalf("account reservation resources omitted fixture rows: %+v", resourcePage.Items)
	}
	resourceDetail, err := service.GetAccountResource(ctx, master, renter, bookingID, corr, "reservations")
	if err != nil || resourceDetail["state"] != "pagada" {
		t.Fatalf("account reservation detail: %+v err=%v", resourceDetail, err)
	}
	decisionAt := time.Now().UTC()
	if _, err = pool.Exec(ctx, `UPDATE public.reserva_ensayo_local SET estado='aprobada_host' WHERE id=$1`, bookingID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.reserva_ensayo_transicion(id,reserva_id,secuencia,estado_anterior,estado_nuevo,actor_id,motivo,creada_en)
		VALUES('82000000-0000-4000-8000-000000000007',$1,1,'pagada','aprobada_host',$2,'fixture KPI aprobación',$3),
		('82000000-0000-4000-8000-000000000008',$4,1,'pendiente_de_pago','rechazada_arrendador',$2,'fixture KPI rechazo',$3)`, bookingID, landlord, decisionAt, rejectedBookingID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.reserva_pago_ensayo(id,reserva_id,resultado,clave_idempotencia,creada_en,importe_clp) VALUES('82000000-0000-4000-8000-000000000009',$1,'exito_simulado','gadm03-kpi-payment', $2,12000)`, bookingID, decisionAt); err != nil {
		t.Fatal(err)
	}
	checkoutFixture := func(reservation, operation, evidence, claim string, claimState string, at time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO public.operacion_arriendo_ensayo_local(id,reserva_id,tipo,actor_id,ocurrio_en,zona_horaria,ubicacion_sintetica,resultado,clave_idempotencia,huella_solicitud,creada_en)
			VALUES($1,$2,'checkout',$3,$4,'America/Santiago','{"source":"synthetic-fixture-v1","location_code":"santiago-demo-center-v1","latitude":-33.456,"longitude":-70.6693}'::jsonb,'registrada',$5,decode(repeat('44',32),'hex'),$4)`, operation, reservation, renter, at, "gadm03-checkout-"+operation); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO public.operacion_arriendo_evidencia_ensayo_local(id,operacion_id,fixture_code,mime_type,sha256,size_bytes,creada_en) VALUES($1,$2,'synthetic-png-v1','image/png',repeat('a',64),128,$3)`, evidence, operation, at); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO public.reclamo_dano_ensayo_local(id,reserva_id,anfitrion_id,arrendatario_id,checkout_operacion_id,checkout_evidencia_id,descripcion,estado,clave_idempotencia,huella_solicitud,abierto_en,plazo_reclamo_hasta)
			VALUES($1,$2,$3,$4,$5,$6,'reclamo sintético de KPI',$7,$8,decode(repeat('55',32),'hex'),$9::timestamptz,$9::timestamptz+interval '24 hours')`, claim, reservation, landlord, renter, operation, evidence, claimState, "gadm03-claim-"+claim, at); err != nil {
			t.Fatal(err)
		}
	}
	checkoutFixture(bookingID, "82000000-0000-4000-8000-000000000010", "82000000-0000-4000-8000-000000000011", "82000000-0000-4000-8000-000000000012", "abierto", decisionAt)
	checkoutFixture(rejectedBookingID, "82000000-0000-4000-8000-000000000013", "82000000-0000-4000-8000-000000000014", "82000000-0000-4000-8000-000000000015", "resuelta", decisionAt)
	if _, err = pool.Exec(ctx, `INSERT INTO public.reclamo_dano_resolucion_ensayo_local(reclamo_id,resultado,motivo_codigo,administrador_id,resuelta_en,clave_idempotencia,huella_solicitud) VALUES('82000000-0000-4000-8000-000000000015','rechazado','hecho_no_acreditado',$1,$2,'gadm03-kpi-resolution',decode(repeat('66',32),'hex'))`, master, decisionAt); err != nil {
		t.Fatal(err)
	}
	period := adminglobal.ReportPeriod{From: decisionAt.Add(-time.Hour), Until: decisionAt.Add(time.Hour), TimeZone: "America/Santiago"}
	kpis, err := service.KPIs(ctx, master, corr, period)
	if err != nil {
		t.Fatalf("persisted KPI report: %v", err)
	}
	var decisionCount, approvedCount int64
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT reserva_id),count(DISTINCT reserva_id) FILTER(WHERE estado_nuevo='aprobada_host') FROM public.reserva_ensayo_transicion WHERE creada_en >= $1 AND creada_en < $2 AND estado_nuevo IN ('aprobada_host','rechazada_arrendador')`, period.From, period.Until).Scan(&decisionCount, &approvedCount); err != nil {
		t.Fatal(err)
	}
	var approvalRate *adminglobal.KPIValue
	for i := range kpis.Reservations {
		if kpis.Reservations[i].Key == "approval_rate" {
			approvalRate = &kpis.Reservations[i]
		}
	}
	if approvalRate == nil || approvalRate.Value != approvedCount || approvalRate.Denominator != decisionCount || approvalRate.Value != 1 || approvalRate.Denominator != 2 {
		t.Fatalf("host approval KPI differs from SQL fixture: metric=%+v SQL approved=%d decisions=%d", approvalRate, approvedCount, decisionCount)
	}
	var openClaims int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.reclamo_dano_ensayo_local WHERE estado='abierto' AND id IN ('82000000-0000-4000-8000-000000000012','82000000-0000-4000-8000-000000000015')`).Scan(&openClaims); err != nil {
		t.Fatal(err)
	}
	findKPI := func(values []adminglobal.KPIValue, key string) *adminglobal.KPIValue {
		for i := range values {
			if values[i].Source == key || values[i].Definition == key || values[i].Key == key {
				return &values[i]
			}
		}
		return nil
	}
	claimMetric := findKPI(kpis.Queues, "reclamo_dano_ensayo_local")
	operationMetric := findKPI(kpis.Operations, "operacion_arriendo_ensayo_local")
	var expectedOperations int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.operacion_arriendo_ensayo_local WHERE reserva_id IN($1,$2) AND tipo IN ('checkout','recepcion') AND ocurrio_en >= $3 AND ocurrio_en < $4`, bookingID, rejectedBookingID, period.From, period.Until).Scan(&expectedOperations); err != nil {
		t.Fatal(err)
	}
	if claimMetric == nil || claimMetric.Value != openClaims || claimMetric.Value != 1 || operationMetric == nil || operationMetric.Value != expectedOperations || operationMetric.Value != 2 || !strings.Contains(operationMetric.Definition, "no equivalen a reservas cerradas") {
		t.Fatalf("claim/operation KPI mismatch: claim=%+v operations=%+v SQL open=%d checkout=%d", claimMetric, operationMetric, openClaims, expectedOperations)
	}
	var paymentCount int64
	var paymentAmount string
	if err = pool.QueryRow(ctx, `SELECT count(*),coalesce(sum(importe_clp),0)::text FROM public.reserva_pago_ensayo WHERE reserva_id=$1 AND resultado='exito_simulado' AND creada_en >= $2 AND creada_en < $3`, bookingID, period.From, period.Until).Scan(&paymentCount, &paymentAmount); err != nil {
		t.Fatal(err)
	}
	var financeMetric *adminglobal.FinanceKPI
	for i := range kpis.Finance {
		if kpis.Finance[i].Kind == "cobro_arriendo" {
			financeMetric = &kpis.Finance[i]
		}
	}
	if financeMetric == nil || financeMetric.Count != paymentCount || financeMetric.AmountCLP != paymentAmount || paymentAmount != "12000" {
		t.Fatalf("financial KPI differs from exact SQL CLP: metric=%+v SQL count=%d amount=%s", financeMetric, paymentCount, paymentAmount)
	}
	const photoID = "81000000-0000-4000-8000-000000000001"
	const fileID = "81000000-0000-4000-8000-000000000002"
	const photoTwoID = "81000000-0000-4000-8000-000000000003"
	const fileTwoID = "81000000-0000-4000-8000-000000000004"
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio_galeria_sintetica_local(id,espacio_id,propietario_id,archivo_id,fixture_code,mime_type,sha256,size_bytes,estado,creada_en,clave_idempotencia) VALUES($1,$2,$3,$4,'synthetic-png-v1','image/png',repeat('a',64),100,'activa',now(),'gadm02-gallery')`, photoID, publicationB, landlord, fileID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio_galeria_sintetica_local(id,espacio_id,propietario_id,archivo_id,fixture_code,mime_type,sha256,size_bytes,estado,creada_en,clave_idempotencia) VALUES($1,$2,$3,$4,'synthetic-png-v1','image/png',repeat('b',64),100,'activa',now(),'gadm02-gallery-two')`, photoTwoID, publicationB, landlord, fileTwoID); err != nil {
		t.Fatal(err)
	}
	service.SetPublicationGallery(gallerypg.New(pool))
	photos, err := service.PublicationGallery(ctx, master, publicationB, landlord, corr)
	if err != nil || len(photos) != 2 || photos[0].ID != photoID || photos[0].State != "activa" {
		t.Fatalf("global gallery read: %+v err=%v", photos, err)
	}
	var galleryVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&galleryVersion); err != nil {
		t.Fatal(err)
	}
	ownerGallery := gallerypg.New(pool)
	if _, err = ownerGallery.SetPublicInCatalog(ctx, landlord, publicationB, photoID, true); err != nil {
		t.Fatalf("owner gallery visibility update: %v", err)
	}
	if _, err = service.SetPublicationPhotoVisibility(ctx, master, publicationB, landlord, photoID, corr, false, galleryVersion, "revision_administrativa"); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale admin gallery write after owner visibility returned %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&galleryVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetPublicationPhotoVisibility(ctx, master, publicationB, landlord, photoID, corr, true, galleryVersion, "revision_administrativa"); err != nil {
		t.Fatalf("global photo visibility: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&galleryVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SelectPublicationCover(ctx, master, publicationB, landlord, photoID, corr, galleryVersion, "revision_administrativa"); err != nil {
		t.Fatalf("global cover select: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&galleryVersion); err != nil {
		t.Fatal(err)
	}
	removedPhoto, err := service.RemovePublicationPhoto(ctx, master, publicationB, landlord, photoID, corr, galleryVersion, "moderacion_contenido")
	if err != nil || !removedPhoto.Removed {
		t.Fatalf("global photo remove: %+v err=%v", removedPhoto, err)
	}
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&galleryVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerGallery.Remove(ctx, landlord, publicationB, photoTwoID); err != nil {
		t.Fatalf("owner non-cover photo removal: %v", err)
	}
	if _, err = service.SetPublicationPhotoVisibility(ctx, master, publicationB, landlord, photoID, corr, false, galleryVersion, "revision_administrativa"); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale admin gallery write after owner remove returned %v", err)
	}
	retirementService, err := spacesdomain.NewService(spacespg.New(pool, credentials.Generator{}))
	if err != nil {
		t.Fatal(err)
	}
	service.SetPublicationRetirer(retirementService)
	var retirementVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationA).Scan(&retirementVersion); err != nil {
		t.Fatal(err)
	}
	retired, err := service.RetirePublication(ctx, master, publicationA, landlord, corr, retirementVersion, "revision_administrativa")
	if err != nil || retired.State != "eliminada" || retired.SpaceID != publicationA {
		t.Fatalf("global retirement: %+v err=%v", retired, err)
	}
	retriedRetirement, err := service.RetirePublication(ctx, master, publicationA, landlord, corr, retirementVersion, "revision_administrativa")
	if err != nil || !retriedRetirement.Reused {
		t.Fatalf("global retirement retry: %+v err=%v", retriedRetirement, err)
	}
	var retirementActor string
	if err = pool.QueryRow(ctx, `SELECT actor_id::text FROM public.espacio_eliminacion_historial_local WHERE espacio_id=$1`, publicationA).Scan(&retirementActor); err != nil || retirementActor != master {
		t.Fatalf("retirement actor=%s err=%v", retirementActor, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.espacio SET estado='activa' WHERE id=$1`, publicationB); err != nil {
		t.Fatal(err)
	}
	var blockedRetirementVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.espacio WHERE id=$1`, publicationB).Scan(&blockedRetirementVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RetirePublication(ctx, master, publicationB, landlord, corr, blockedRetirementVersion, "revision_administrativa"); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("retirement ignored active booking fixture: %v", err)
	}
	auth, err := identity.NewAuthenticationService(identitypg.NewIdentityRepository(pool), password.Hasher{}, devauth.Mailer{}, devauth.NewIPLimiter(50), credentials.Generator{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	globalToken := issueSyntheticSession(t, ctx, pool, master, "synthetic-global-bearer")
	legacyToken := issueSyntheticSession(t, ctx, pool, legacy, "synthetic-legacy-bearer")
	if _, err = auth.AuthorizeCapability(ctx, globalToken, identity.CapabilityAccountsList, identity.UserOperation); err != nil {
		t.Fatalf("global capability denied: %v", err)
	}
	managementPrincipal, err := auth.AuthorizeCapability(ctx, globalToken, identity.CapabilityAccountsUpdate, identity.UserOperation)
	if err != nil || time.Since(managementPrincipal.AuthenticatedAt) > time.Minute {
		t.Fatalf("recent authentication timestamp missing/stale: %+v err=%v", managementPrincipal, err)
	}
	if _, err = auth.AuthorizeCapability(ctx, legacyToken, identity.CapabilityLegacyAdministration, identity.UserOperation); err != nil {
		t.Fatalf("legacy capability changed: %v", err)
	}
	if _, err = auth.AuthorizeCapability(ctx, legacyToken, identity.CapabilityAccountsList, identity.UserOperation); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("legacy admin escalated to directory: %v", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM public.administrador_global WHERE usuario_id=$1`, master); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.AuthorizeCapability(ctx, globalToken, identity.CapabilityAccountsList, identity.UserOperation); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("old bearer kept removed capability: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.administrador_global(usuario_id,concedido_en,motivo_codigo) VALUES($1,now(),'recuperacion_local')`, master); err != nil {
		t.Fatal(err)
	}
	blocker, _ := adminlocal.New(pool, time.Now)
	if _, err = blocker.BlockAccount(ctx, master, renter, "revision_administrativa", corr); err != nil {
		t.Fatal(err)
	}
	restrictedToken := issueSyntheticSession(t, ctx, pool, renter, "synthetic-restricted-bearer")
	if _, err = auth.AuthorizeCapability(ctx, restrictedToken, identity.CapabilityAccountsList, identity.UserOperation); !errors.Is(err, identity.ErrRestrictedMode) {
		t.Fatalf("restricted account gained directory access: %v", err)
	}
	issueSyntheticSession(t, ctx, pool, master, "synthetic-global-second-session")
	overview, err := service.Overview(ctx, master, corr)
	if err != nil || overview.TotalAccounts != 6 || overview.ConnectedAccounts != 2 {
		t.Fatalf("overview should count distinct accounts and exclude administrative blocks: %+v, %v", overview, err)
	}
	page, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.MatchingTotal != 6 || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", page)
	}
	second, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.Items[0].AccountID == page.Items[1].AccountID {
		t.Fatalf("keyset cursor repeated/omitted tie page: %+v", second)
	}
	filtered, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 10, Role: identity.RoleAdministrator})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.MatchingTotal != 1 || filtered.Items[0].AccountID != legacy {
		t.Fatalf("role filter leaked global bootstrap owner or missed legacy: %+v", filtered)
	}
	byEmail, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 10, EmailPrefix: "LEGACY@EXAMPLE"})
	if err != nil || byEmail.MatchingTotal != 1 || byEmail.Items[0].AccountID != legacy {
		t.Fatalf("email prefix filter: %+v %v", byEmail, err)
	}
	byName, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 10, NamePrefix: "second"})
	if err != nil || byName.MatchingTotal != 1 || byName.Items[0].AccountID != other {
		t.Fatalf("name prefix filter: %+v %v", byName, err)
	}
	blockedFilter, err := service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 10, AdministrativeBlocked: ptr(true)})
	if err != nil || blockedFilter.MatchingTotal != 1 || blockedFilter.Items[0].AccountID != renter {
		t.Fatalf("administrative blocked filter: %+v %v", blockedFilter, err)
	}
	lookup, err := service.LookupAccount(ctx, master, legacy, corr)
	if err != nil || lookup.AccountID != legacy {
		t.Fatalf("minimal detail lookup: %+v %v", lookup, err)
	}
	const missingAccountID = "00000000-0000-4000-8000-000000000099"
	if _, err = service.LookupAccount(ctx, master, missingAccountID, corr); !errors.Is(err, adminglobal.ErrNotFound) {
		t.Fatalf("missing detail returned %v", err)
	}
	var missingAudit bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.evento_auditoria_local WHERE actor_id=$1 AND recurso_id=$2 AND accion='admin.global.accounts.read' AND resultado='rechazo' AND motivo_codigo='cuenta_no_encontrada' AND cuenta_objetivo_id IS NULL)`, master, missingAccountID).Scan(&missingAudit); err != nil || !missingAudit {
		t.Fatalf("missing account rejection was not audited safely: exists=%v err=%v", missingAudit, err)
	}
	for range 3 {
		if _, err = service.LookupAccount(ctx, master, legacy, corr); err != nil {
			t.Fatalf("create audit page fixture: %v", err)
		}
	}
	const auditTieTime = "2026-10-15T01:00:00Z"
	if _, err = pool.Exec(ctx, `UPDATE public.evento_auditoria_local SET ocurrido_en=$1,retirar_en=$1::timestamptz+interval '5 years' WHERE actor_id=$2 AND cuenta_objetivo_id=$3 AND accion='admin.global.accounts.read' AND resultado='exito' AND correlacion_id=$4`, auditTieTime, master, legacy, corr); err != nil {
		t.Fatal(err)
	}
	auditFilter := adminglobal.AuditFilter{From: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), ActorID: master, TargetID: legacy, ResourceType: "cuenta_global", ResourceID: legacy, Action: "admin.global.accounts.read", Result: "exito", ReasonCode: "consulta_administrativa", CorrelationID: corr, PageSize: 1}
	seenAuditIDs := map[string]bool{}
	var firstAuditPage adminglobal.AuditPage
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		page, auditErr := service.Audit(ctx, master, corr, auditFilter, false)
		if auditErr != nil {
			t.Fatalf("audit page %d: %v", pageNumber+1, auditErr)
		}
		if pageNumber == 0 {
			firstAuditPage = page
		}
		for _, event := range page.Items {
			if seenAuditIDs[event.ID] {
				t.Fatalf("audit cursor repeated event %s on page %d", event.ID, pageNumber+1)
			}
			seenAuditIDs[event.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		auditFilter.Cursor = page.NextCursor
		auditFilter.SnapshotCutoff = &page.SnapshotCutoff
	}
	if len(seenAuditIDs) < 3 {
		t.Fatalf("audit cursor did not traverse multiple pages: %d events", len(seenAuditIDs))
	}
	changedAuditFilter := auditFilter
	changedAuditFilter.Action = "admin.global.accounts.update"
	if _, err = service.Audit(ctx, master, corr, changedAuditFilter, false); !errors.Is(err, adminglobal.ErrInvalid) {
		t.Fatalf("audit cursor accepted changed filters: %v", err)
	}
	if _, err = service.LookupAccount(ctx, master, legacy, corr); err != nil {
		t.Fatal(err)
	}
	exportFilter := firstAuditPage.AppliedFilters
	exportFilter.Format = "json"
	for _, format := range []string{"json", "csv"} {
		exportFilter.Format = format
		exported, exportErr := service.Audit(ctx, master, corr, exportFilter, true)
		if exportErr != nil {
			t.Fatalf("audit %s export: %v", format, exportErr)
		}
		for _, event := range exported.Items {
			if event.RecordedAt.After(firstAuditPage.SnapshotCutoff) {
				t.Fatalf("%s export contains event after displayed snapshot cutoff", format)
			}
		}
		if !exported.GeneratedAt.After(exported.SnapshotCutoff) {
			t.Fatalf("%s export generation time should be distinct from the pinned data cutoff", format)
		}
	}
	var profileVersion time.Time
	if err = pool.QueryRow(ctx, `SELECT actualizado_en FROM public.perfil_usuario WHERE usuario_id=$1`, legacy).Scan(&profileVersion); err != nil {
		t.Fatal(err)
	}
	newPhone := "987654321"
	profile, err := service.UpdateAccountProfile(ctx, master, legacy, corr, adminglobal.AccountProfileInput{DisplayName: "Legacy Updated", Phone: &newPhone, ExpectedAt: profileVersion, Reason: "correccion_perfil"})
	if err != nil || profile.Name != "Legacy Updated" || profile.Email != "legacy@example.invalid" {
		t.Fatalf("allowlisted profile update: %+v err=%v", profile, err)
	}
	if _, err = service.UpdateAccountProfile(ctx, master, legacy, corr, adminglobal.AccountProfileInput{DisplayName: "Stale", ExpectedAt: profileVersion, Reason: "correccion_perfil"}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale profile version returned %v", err)
	}
	if _, err = service.SetAccountRole(ctx, master, legacy, "arrendador", "revision_administrativa", corr, true); err != nil {
		t.Fatalf("grant ordinary role: %v", err)
	}
	if _, err = service.SetAccountRole(ctx, master, legacy, "arrendador", "revision_administrativa", corr, true); err != nil {
		t.Fatalf("idempotent role grant: %v", err)
	}
	if _, err = service.SetAccountRole(ctx, master, legacy, "administrador_global", "revision_administrativa", corr, true); !errors.Is(err, adminglobal.ErrInvalid) {
		t.Fatalf("runtime global role grant accepted: %v", err)
	}
	legacyGrant, err := service.SetLegacyAdministrator(ctx, master, landlord, "aprovisionamiento_sintetico", corr, true)
	if err != nil || !legacyGrant.Granted || legacyGrant.Role != "administrador" {
		t.Fatalf("explicit legacy administrator grant: %+v err=%v", legacyGrant, err)
	}
	if _, err = service.SetLegacyAdministrator(ctx, master, landlord, "aprovisionamiento_sintetico", corr, true); err != nil {
		t.Fatalf("idempotent legacy administrator grant: %v", err)
	}
	var legacyRole, globalMembership bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.rol_usuario WHERE usuario_id=$1 AND rol='administrador'),
		EXISTS(SELECT 1 FROM public.administrador_global WHERE usuario_id=$1)`, landlord).Scan(&legacyRole, &globalMembership); err != nil || !legacyRole || globalMembership {
		t.Fatalf("legacy role/global membership separation failed: legacy=%v global=%v err=%v", legacyRole, globalMembership, err)
	}
	if _, err = service.SetLegacyAdministrator(ctx, master, landlord, "retirada_acceso", corr, false); err != nil {
		t.Fatalf("legacy administrator revoke: %v", err)
	}
	var legacyAuditEvents int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.evento_auditoria_local
		WHERE recurso_id=$1 AND accion LIKE 'admin.global.accounts.legacy-administrator.%'`, landlord).Scan(&legacyAuditEvents); err != nil || legacyAuditEvents != 3 {
		t.Fatalf("legacy administrator grant/replay/revoke audit events=%d err=%v", legacyAuditEvents, err)
	}
	if _, err = service.SetAccountBlocked(ctx, master, legacy, "revision_administrativa", corr, true); err != nil {
		t.Fatalf("global account block: %v", err)
	}
	if _, err = auth.AuthorizeCapability(ctx, legacyToken, identity.CapabilityLegacyAdministration, identity.UserOperation); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("blocked account bearer remained active: %v", err)
	}
	if _, err = service.SetAccountBlocked(ctx, master, legacy, "riesgo_resuelto", corr, false); err != nil {
		t.Fatalf("global account unblock: %v", err)
	}
	if _, err = service.RevokeAccountSessions(ctx, master, landlord, "gestion_operativa", corr); err != nil {
		t.Fatalf("revoke sessions: %v", err)
	}
	if _, err = service.ListAccounts(ctx, legacy, corr, adminglobal.AccountFilter{PageSize: 10}); !errors.Is(err, adminglobal.ErrForbidden) {
		t.Fatalf("legacy admin accessed global directory: %v", err)
	}
	if _, err = service.ListAccounts(ctx, renter, corr, adminglobal.AccountFilter{PageSize: 10}); !errors.Is(err, adminglobal.ErrForbidden) {
		t.Fatalf("renter accessed global directory: %v", err)
	}
	if _, err = service.ListAccounts(ctx, master, corr, adminglobal.AccountFilter{PageSize: 10, Role: identity.Role("administrador' OR true --")}); !errors.Is(err, adminglobal.ErrInvalid) {
		t.Fatalf("role filter injection not rejected: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.administrador_global(usuario_id,concedido_en,motivo_codigo) VALUES($1,now(),'concesion_administrativa'),($2,now(),'concesion_administrativa') ON CONFLICT DO NOTHING`, owner, other); err != nil {
		t.Fatal(err)
	}
	var globalWG sync.WaitGroup
	globalBlockResults := make(chan error, 2)
	for _, pair := range [][2]string{{owner, other}, {other, owner}} {
		globalWG.Add(1)
		go func(actor, target string) {
			defer globalWG.Done()
			_, e := service.SetAccountBlocked(ctx, actor, target, "revision_administrativa", corr, true)
			globalBlockResults <- e
		}(pair[0], pair[1])
	}
	globalWG.Wait()
	close(globalBlockResults)
	globalSuccess, globalRejected := 0, 0
	for e := range globalBlockResults {
		if e == nil {
			globalSuccess++
		} else if errors.Is(e, adminglobal.ErrForbidden) || errors.Is(e, adminglobal.ErrConflict) {
			globalRejected++
		} else {
			t.Errorf("unexpected concurrent global block: %v", e)
		}
	}
	if globalSuccess != 1 || globalRejected != 1 {
		t.Fatalf("global-block last-master race success=%d rejected=%d", globalSuccess, globalRejected)
	}
	var survivor, blockedMaster string
	if err = pool.QueryRow(ctx, `SELECT u.id::text FROM public.administrador_global g JOIN public.usuario u ON u.id=g.usuario_id WHERE NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id) LIMIT 1`).Scan(&survivor); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT cuenta_id::text FROM public.bloqueo_cuenta_administrativo_local WHERE cuenta_id IN($1,$2)`, owner, other).Scan(&blockedMaster); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetAccountBlocked(ctx, survivor, blockedMaster, "riesgo_resuelto", corr, false); err != nil {
		t.Fatalf("restore second master after concurrency test: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pair := range [][2]string{{owner, owner}, {other, other}} {
		wg.Add(1)
		go func(actor, target string) {
			defer wg.Done()
			_, e := blocker.BlockAccount(ctx, actor, target, "revision_administrativa", corr)
			errs <- e
		}(pair[0], pair[1])
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, adminlocal.ErrConflict) {
			conflict++
		} else {
			t.Errorf("unexpected concurrent block result: %v", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("last-master race results success=%d conflict=%d", success, conflict)
	}
	var active int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.administrador_global g JOIN public.usuario u ON u.id=g.usuario_id WHERE u.estado='activo' AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id)`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("global admins after concurrent blocks=%d err=%v", active, err)
	}
}

func TestRestrictedActivationRejectsDirectLoginPrivilegesAndOwnership(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_gadm01_%08x", uint32(time.Now().UnixNano()))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); err != nil {
			t.Errorf("drop isolated database: %v", err)
		}
	}()
	databaseURL, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	databaseURL.Path = "/" + dbName
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration source directory")
	}
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, databaseURL.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	adminDB, err := pgx.Connect(ctx, databaseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close(context.Background())
	// Model the unrelated PUBLIC sequence ACL installed by PostGIS topology in
	// the persistent database. It must not be mistaken for a grant to the GADM
	// group, and the test-only schema is removed with this disposable database.
	if _, err = adminDB.Exec(ctx, `CREATE SCHEMA gadm_public_acl_probe;
		CREATE SEQUENCE gadm_public_acl_probe.topology_id_seq;
		GRANT USAGE ON SCHEMA gadm_public_acl_probe TO PUBLIC;
		GRANT SELECT ON SEQUENCE gadm_public_acl_probe.topology_id_seq TO PUBLIC`); err != nil {
		t.Fatal(err)
	}
	const initialID = "00000000-0000-4000-8000-000000000031"
	const differentID = "00000000-0000-4000-8000-000000000032"
	const correlationID = "10000000-0000-4000-8000-000000000031"
	if _, err = adminDB.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado)
		VALUES($1,'restricted@example.invalid','restricted@example.invalid','synthetic','activo')`, initialID); err != nil {
		t.Fatal(err)
	}
	roleSuffix := make([]byte, 6)
	if _, err = rand.Read(roleSuffix); err != nil {
		t.Fatal(err)
	}
	loginRole := "gadm_activation_" + hex.EncodeToString(roleSuffix)
	loginPasswordBytes := make([]byte, 24)
	if _, err = rand.Read(loginPasswordBytes); err != nil {
		t.Fatal(err)
	}
	loginPassword := hex.EncodeToString(loginPasswordBytes)
	if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{loginRole}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD '"+loginPassword+"'"); err != nil {
		t.Fatal(err)
	}
	var restricted *pgxpool.Pool
	defer func() {
		if restricted != nil {
			restricted.Close()
		}
		_, _ = adminDB.Exec(context.Background(), "ALTER TABLE IF EXISTS public.gadm_session_owner_probe OWNER TO postgres")
		_, _ = adminDB.Exec(context.Background(), "DROP TABLE IF EXISTS public.gadm_session_owner_probe")
		_, _ = adminDB.Exec(context.Background(), "REVOKE SELECT (hash_clave) ON public.usuario FROM "+pgx.Identifier{loginRole}.Sanitize())
		_, _ = admin.Exec(context.Background(), "REVOKE espacigo_gadm_bootstrap FROM "+pgx.Identifier{loginRole}.Sanitize())
		_, _ = admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+pgx.Identifier{loginRole}.Sanitize())
	}()
	if _, err = admin.Exec(ctx, "GRANT espacigo_gadm_bootstrap TO "+pgx.Identifier{loginRole}.Sanitize()+" WITH INHERIT FALSE, SET TRUE, ADMIN FALSE"); err != nil {
		t.Fatal(err)
	}
	openRestrictedPool := func() (*pgxpool.Pool, error) {
		loginURL, parseErr := url.Parse(databaseURL.String())
		if parseErr != nil {
			return nil, parseErr
		}
		loginURL.User = url.UserPassword(loginRole, loginPassword)
		config, configErr := pgxpool.ParseConfig(loginURL.String())
		if configErr != nil {
			return nil, configErr
		}
		config.AfterConnect = adminglobal.SetRestrictedLocalBootstrapRole
		return pgxpool.NewWithConfig(ctx, config)
	}
	restricted, err = openRestrictedPool()
	if err != nil {
		t.Fatal(err)
	}
	if err = restricted.Ping(ctx); err != nil {
		t.Fatalf("clean restricted login was rejected: %v", err)
	}
	var publicSequenceVisible, groupHasNoDirectSequenceGrant bool
	if err = restricted.QueryRow(ctx, `SELECT has_sequence_privilege(current_user,'gadm_public_acl_probe.topology_id_seq','SELECT'),
		NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c CROSS JOIN LATERAL pg_catalog.aclexplode(c.relacl) x
		JOIN pg_catalog.pg_roles r ON r.oid=x.grantee WHERE c.oid='gadm_public_acl_probe.topology_id_seq'::regclass AND r.rolname=current_user)`).Scan(&publicSequenceVisible, &groupHasNoDirectSequenceGrant); err != nil || !publicSequenceVisible || !groupHasNoDirectSequenceGrant {
		t.Fatalf("PUBLIC sequence fixture invalid: visible=%v no_direct_group_grant=%v err=%v", publicSequenceVisible, groupHasNoDirectSequenceGrant, err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, initialID, "bootstrap", correlationID); err != nil {
		t.Fatalf("clean restricted login could not perform initial bootstrap: %v", err)
	}
	if _, err = adminDB.Exec(ctx, `DELETE FROM public.administrador_global WHERE usuario_id=$1`, initialID); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, initialID, "recover", correlationID); err != nil {
		t.Fatalf("clean restricted login could not recover the anchored account: %v", err)
	}
	if err = adminglobal.Bootstrap(ctx, restricted, differentID, "recover", correlationID); err == nil {
		t.Fatal("clean restricted login recovered a different account")
	}
	restricted.Close()
	restricted = nil
	if _, err = adminDB.Exec(ctx, "GRANT SELECT (hash_clave) ON public.usuario TO "+pgx.Identifier{loginRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	restricted, err = openRestrictedPool()
	if err != nil {
		t.Fatal(err)
	}
	if err = restricted.Ping(ctx); err == nil {
		t.Fatal("login with a direct hash_clave grant passed restricted-role validation")
	}
	restricted.Close()
	restricted = nil
	if _, err = adminDB.Exec(ctx, "REVOKE SELECT (hash_clave) ON public.usuario FROM "+pgx.Identifier{loginRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = adminDB.Exec(ctx, `CREATE TABLE public.gadm_session_owner_probe(id integer)`); err != nil {
		t.Fatal(err)
	}
	if _, err = adminDB.Exec(ctx, "ALTER TABLE public.gadm_session_owner_probe OWNER TO "+pgx.Identifier{loginRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	restricted, err = openRestrictedPool()
	if err != nil {
		t.Fatal(err)
	}
	if err = restricted.Ping(ctx); err == nil {
		t.Fatal("login that owns a database object passed restricted-role validation")
	}
}

func repositoryRoot(source string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
}

func makeLocalCLICredentials(t *testing.T, databaseURL, databaseName string) string {
	t.Helper()
	root := repositoryRootFromTest(t)
	localTesting := filepath.Join(root, ".local", "testing")
	if err := os.MkdirAll(localTesting, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(localTesting, "gadm01-cli-")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "credentials.env")
	data := []byte("DATABASE_URL=" + databaseURL + "\nEXPECTED_DATABASE=" + databaseName + "\n")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func repositoryRootFromTest(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test source directory")
	}
	return repositoryRoot(source)
}

func runBootstrapCLI(ctx context.Context, root, credentialFile, command, accountID string) error {
	process := exec.CommandContext(ctx, "go", "run", "./cmd/gadm-admin", command, "--account-id", accountID, "--credentials", credentialFile)
	process.Dir = root
	process.Env = make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "GADM_BOOTSTRAP_ENV=") {
			process.Env = append(process.Env, item)
		}
	}
	process.Env = append(process.Env, "GADM_BOOTSTRAP_ENV=local-test")
	output, err := process.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ptr[T any](value T) *T { return &value }

func issueSyntheticSession(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accountID, token string) identity.Secret {
	t.Helper()
	id, err := (credentials.Generator{}).ID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	hash := identity.CredentialHash(identity.Secret(token))
	if _, err = pool.Exec(ctx, `INSERT INTO public.sesion(id,usuario_id,token_hash,creada_en,ultima_actividad_en,expira_en) VALUES($1,$2,$3,$4,$4,$5)`, id, accountID, hash, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return identity.Secret(token)
}

func TestPrivacyDispatchReceiptRuntimeAndAssistedView(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_priv_ops_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const master = "00000000-0000-4000-8000-000000000011"
	const owner = "00000000-0000-4000-8000-000000000012"
	const other = "00000000-0000-4000-8000-000000000013"
	const corr = "10000000-0000-4000-8000-000000000011"
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'master@example.invalid','master@example.invalid','synthetic','activo','2026-01-01'),
		($2,'owner@example.invalid','owner@example.invalid','synthetic','activo','2026-01-01'),
		($3,'other@example.invalid','other@example.invalid','synthetic','activo','2026-01-01')`, master, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible) VALUES($1,'Master Ops'),($2,'Titular Uno'),($3,'Titular Dos')`, master, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,'arrendatario'),($2,'arrendatario'),($3,'arrendatario')`, master, owner, other); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatal(err)
	}

	privacyNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	service, err := adminglobal.New(pool, func() time.Time { return privacyNow })
	if err != nil {
		t.Fatal(err)
	}
	adminCreated, err := service.CreatePrivacyRequest(ctx, adminglobal.CreatePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, AccountID: owner, RightType: "acceso", Reason: "Admin assisted access request", CaseRef: "ADMIN-PRIV-1",
	})
	if err != nil {
		t.Fatalf("create canonical admin request: %v", err)
	}
	if adminCreated.RequestedBy != master || adminCreated.Status != "pendiente" || adminCreated.CanPublishResponse {
		t.Fatalf("unexpected admin created request: %+v", adminCreated)
	}
	const portalRequestID = "20000000-0000-4000-8000-000000000001"
	const nonAccessRequestID = "20000000-0000-4000-8000-000000000002"
	const runtimeRequestID = "20000000-0000-4000-8000-000000000003"
	portalCreatedAt := privacyNow.Add(-2 * time.Hour)
	if _, err = pool.Exec(ctx, `INSERT INTO public.solicitud_titular(id,usuario_id,tipo,canal,estado,solicitada_en) VALUES($1,$4,'acceso','web','en_revision',$5),($2,$4,'supresion','web','en_revision',$5),($3,$4,'acceso','web','en_revision',$5)`,
		portalRequestID, nonAccessRequestID, runtimeRequestID, owner, portalCreatedAt); err != nil {
		t.Fatal(err)
	}

	filteredAccess, err := service.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, RightType: "acceso", PageSize: 100})
	if err != nil || len(filteredAccess.Items) != 3 {
		t.Fatalf("right_type=acceso filter count=%d err=%v", len(filteredAccess.Items), err)
	}
	filteredSuppression, err := service.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, RightType: "supresion", PageSize: 100})
	if err != nil || len(filteredSuppression.Items) != 1 || filteredSuppression.Items[0].ID != nonAccessRequestID {
		t.Fatalf("right_type=supresion filter items=%+v err=%v", filteredSuppression.Items, err)
	}

	resolved, err := service.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Decision: "resuelta", Response: "La información de acceso está lista para la persona titular.",
	})
	if err != nil || resolved.Status != "completada" || !resolved.CanRecordDispatch {
		t.Fatalf("resolve access request: item=%+v err=%v", resolved, err)
	}
	preDispatchList, err := service.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{AccountID: owner, RightType: "acceso", PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range preDispatchList.Items {
		if item.ID == portalRequestID && (!item.CanRecordDispatch || item.DispatchRecordedAt != nil) {
			t.Fatalf("expected can_record_dispatch=true before receipt, got %+v", item)
		}
	}

	for _, badDispatch := range []adminglobal.RecordPrivacyDispatchInput{
		{ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Channel: "email", Destination: "titular@ejemplo.invalid", SentAt: privacyNow.Add(-time.Minute), EvidenceRef: "   "},
		{ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Channel: "email", Destination: "invalido", SentAt: privacyNow.Add(-time.Minute), EvidenceRef: "REF-1"},
		{ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Channel: "email", Destination: "titular@ejemplo.invalid", SentAt: portalCreatedAt.Add(-time.Minute), EvidenceRef: "REF-1"},
		{ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Channel: "email", Destination: "titular@ejemplo.invalid", SentAt: privacyNow.Add(time.Minute), EvidenceRef: "REF-1"},
	} {
		if _, err = service.RecordPrivacyDispatch(ctx, badDispatch); !errors.Is(err, adminglobal.ErrInvalid) {
			t.Fatalf("invalid dispatch accepted (%+v): err=%v", badDispatch, err)
		}
	}
	dispatchInput := adminglobal.RecordPrivacyDispatchInput{
		ActorID: master, CorrelationID: corr, RequestID: portalRequestID, Channel: "email",
		Destination: "titular.privado@ejemplo.invalid", SentAt: privacyNow.Add(-10 * time.Minute), EvidenceRef: "MAILPIT-MSG-9876",
	}
	dispatched, err := service.RecordPrivacyDispatch(ctx, dispatchInput)
	if err != nil || dispatched.CanRecordDispatch || dispatched.DispatchRecordedAt == nil || dispatched.CreatedAt.IsZero() {
		t.Fatalf("record privacy dispatch: item=%+v err=%v", dispatched, err)
	}
	if _, err = service.RecordPrivacyDispatch(ctx, dispatchInput); err != nil {
		t.Fatalf("replay privacy dispatch: %v", err)
	}
	conflicting := dispatchInput
	conflicting.EvidenceRef = "MAILPIT-MSG-OTHER"
	if _, err = service.RecordPrivacyDispatch(ctx, conflicting); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("conflicting dispatch error = %v, want conflict", err)
	}
	var dispatchAudits int
	var auditReason string
	if err = pool.QueryRow(ctx, `SELECT count(*), max(motivo_codigo) FROM public.evento_auditoria_local WHERE recurso_id=$1 AND accion='admin.global.privacy_requests.dispatch_receipt'`, portalRequestID).Scan(&dispatchAudits, &auditReason); err != nil || dispatchAudits != 1 || auditReason != "respuesta_despachada" {
		t.Fatalf("dispatch audit count=%d reason=%q err=%v", dispatchAudits, auditReason, err)
	}
	var leakedAuditData bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.evento_auditoria_local WHERE recurso_id=$1 AND (motivo_codigo LIKE '%titular.privado%' OR motivo_codigo LIKE '%MAILPIT-MSG-9876%'))`, portalRequestID).Scan(&leakedAuditData); err != nil || leakedAuditData {
		t.Fatalf("dispatch audit leaked sensitive destination/evidence: leaked=%v err=%v", leakedAuditData, err)
	}

	if err = dbbootstrap.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("grant runtime permissions: %v", err)
	}
	runtimeCfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(c context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(c, `SET ROLE espacigo_runtime`)
		return e
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()
	runtimeService, err := adminglobal.New(runtimePool, func() time.Time { return privacyNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: runtimeRequestID, Decision: "resuelta", Response: "Respuesta publicada usando el rol espacigo_runtime.",
	}); err != nil {
		t.Fatalf("espacigo_runtime resolve privacy request: %v", err)
	}
	if _, err = runtimeService.RecordPrivacyDispatch(ctx, adminglobal.RecordPrivacyDispatchInput{
		ActorID: master, CorrelationID: corr, RequestID: runtimeRequestID, Channel: "postal", Destination: "Av. Providencia 1234", SentAt: privacyNow.Add(-5 * time.Minute), EvidenceRef: "CORREOS-777",
	}); err != nil {
		t.Fatalf("espacigo_runtime record privacy dispatch: %v", err)
	}

	const adminSessionID = "30000000-0000-4000-8000-000000000001"
	const closedTicketID = "30000000-0000-4000-8000-000000000002"
	const otherTicketID = "30000000-0000-4000-8000-000000000003"
	const activeTicketID = "30000000-0000-4000-8000-000000000004"
	if _, err = pool.Exec(ctx, `INSERT INTO public.sesion_administrativa(id,cuenta_id,token_hash,creada_en,ultima_actividad_en,expira_en,mfa_verificado_en,mfa_metodo) VALUES($1,$2,repeat('a',64),$3,$3,$4,$3,'totp')`, adminSessionID, master, privacyNow, privacyNow.Add(8*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.ticket_soporte_administrativo(id,cuenta_id,categoria,prioridad,estado,asunto,caso_ref) VALUES
		($1,$4,'cuenta','media','resuelto','Ticket cerrado','SUP-CLOSED'),
		($2,$5,'cuenta','media','abierto','Ticket otra cuenta','SUP-OTHER'),
		($3,$4,'cuenta','alta','abierto','Ticket activo para vista','SUP-ACTIVE')`,
		closedTicketID, otherTicketID, activeTicketID, owner, other); err != nil {
		t.Fatal(err)
	}
	for _, badCase := range []string{"SUP-MISSING", "SUP-CLOSED", "SUP-OTHER"} {
		if _, err = service.StartAssistedView(ctx, adminglobal.AssistedViewStartInput{
			ActorID: master, TargetID: owner, AdminSessionID: adminSessionID, CorrelationID: corr, Role: "arrendatario", CaseID: badCase, Reason: "Verificación de ticket requerido",
		}); !errors.Is(err, adminglobal.ErrForbidden) {
			t.Fatalf("StartAssistedView with case %s returned %v, want ErrForbidden", badCase, err)
		}
	}
	grant, err := service.StartAssistedView(ctx, adminglobal.AssistedViewStartInput{
		ActorID: master, TargetID: owner, AdminSessionID: adminSessionID, CorrelationID: corr, Role: "arrendatario", CaseID: "SUP-ACTIVE", Reason: "Soporte por ticket activo",
	})
	if err != nil {
		t.Fatalf("StartAssistedView with active ticket: %v", err)
	}
	if _, err = service.GetAssistedViewProjection(ctx, adminglobal.AssistedViewProjectionInput{
		GrantID: grant.GrantID, ActorID: master, AdminSessionID: adminSessionID, CorrelationID: corr,
	}); err != nil {
		t.Fatalf("GetAssistedViewProjection with active ticket: %v", err)
	}
	var assistedReadAudits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.evento_auditoria_local WHERE recurso_id=$1 AND accion='admin.global.assisted_view.read'`, grant.GrantID).Scan(&assistedReadAudits); err != nil || assistedReadAudits != 1 {
		t.Fatalf("assisted view read audit count=%d err=%v, want 1", assistedReadAudits, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.ticket_soporte_administrativo SET estado='cerrado' WHERE id=$1`, activeTicketID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.GetAssistedViewProjection(ctx, adminglobal.AssistedViewProjectionInput{
		GrantID: grant.GrantID, ActorID: master, AdminSessionID: adminSessionID, CorrelationID: corr,
	}); !errors.Is(err, adminglobal.ErrForbidden) {
		t.Fatalf("GetAssistedViewProjection after ticket close returned %v, want ErrForbidden", err)
	}
}

func TestPrivacyCaseWorkflowDeadlinesEventsAndIsolation(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_priv_wf_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const master = "00000000-0000-4000-8000-000000000021"
	const owner = "00000000-0000-4000-8000-000000000022"
	const corr = "10000000-0000-4000-8000-000000000021"
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'wf-master@example.invalid','wf-master@example.invalid','synthetic','activo','2026-01-01'),
		($2,'wf-owner@example.invalid','wf-owner@example.invalid','synthetic','activo','2026-01-01')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible) VALUES($1,'Master Workflow'),($2,'Titular Workflow')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,'arrendatario'),($2,'arrendatario')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatal(err)
	}
	if err = dbbootstrap.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("grant runtime permissions: %v", err)
	}
	runtimeCfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(c context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(c, `SET ROLE espacigo_runtime`)
		return e
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()

	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	runtimeService, err := adminglobal.New(runtimePool, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypg.NewIdentityRepository(runtimePool)

	bloqueoDetail := "Solicito bloqueo temporal mientras se resuelve la impugnacion de datos."
	bloqueoReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "bloqueo", "web", &bloqueoDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(bloqueo): %v", err)
	}
	if bloqueoReq.DueAt == nil {
		t.Fatalf("expected DueAt on bloqueo request: %+v", bloqueoReq)
	}
	rectDetail := "Corregir mi razon social registrada en perfil comercial."
	rectReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "rectificacion", "web", &rectDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(rectificacion): %v", err)
	}
	if rectReq.DueAt == nil {
		t.Fatalf("expected DueAt on rectificacion request: %+v", rectReq)
	}

	adminCreated, err := runtimeService.CreatePrivacyRequest(ctx, adminglobal.CreatePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, AccountID: owner, RightType: "portabilidad", Reason: "Solicitud asistida de portabilidad", CaseRef: "CASO-PORT-24",
	})
	if err != nil {
		t.Fatalf("CreatePrivacyRequest(portabilidad): %v", err)
	}
	if adminCreated.Status != "pendiente" || adminCreated.Version != 1 || adminCreated.DueAt == nil {
		t.Fatalf("unexpected adminCreated fields: %+v", adminCreated)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Decision: "resuelta", Response: "No debe resolverse antes de verificar identidad.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("ResolvePrivacyRequest on identidad_pendiente = %v, want ErrConflict", err)
	}

	// Verify identity -> en_revision (v1 -> v2)
	detail, err := runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "verify_identity", ExpectedVersion: 1, Note: "Cedula validada presencialmente",
	})
	if err != nil || detail.Status != "en_revision" || detail.Version != 2 {
		t.Fatalf("verify_identity: detail=%+v err=%v", detail, err)
	}
	// Stale version conflict
	if _, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "assign", ExpectedVersion: 1, AssignedToID: master,
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("stale version assign err = %v, want ErrConflict", err)
	}
	// Assign (v2 -> v3), internal note (v3 -> v4), clarification request (v4 -> v5), extension (v5 -> v6)
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "assign", ExpectedVersion: 2, AssignedToID: master,
	})
	if err != nil || detail.Version != 3 || detail.AssignedToID != master {
		t.Fatalf("assign: detail=%+v err=%v", detail, err)
	}
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "add_internal_note", ExpectedVersion: 3, Note: "SECRETO INTERNO NO EXPONER AL TITULAR",
	})
	if err != nil || detail.Version != 4 {
		t.Fatalf("add_internal_note: detail=%+v err=%v", detail, err)
	}
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "request_clarification", ExpectedVersion: 4, Note: "Confirmar formato JSON o CSV preferido.",
	})
	if err != nil || detail.Version != 5 {
		t.Fatalf("request_clarification: detail=%+v err=%v", detail, err)
	}
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "extend_deadline", ExpectedVersion: 5, ExtraDays: 15, Note: "Consolidacion multi-modulo en curso",
	})
	if err != nil || detail.Version != 6 || detail.ExtendedDueAt == nil || detail.ExtensionReason == nil {
		t.Fatalf("extend_deadline: detail=%+v err=%v", detail, err)
	}
	if _, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Action: "extend_deadline", ExpectedVersion: 6, ExtraDays: 5, Note: "Segunda prorroga prohibida",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("second extend_deadline err = %v, want ErrConflict", err)
	}

	// Test start_review transition on a 'recibida' case (bloqueoReq and rectReq default to 'en_revision')
	const receivedID = "20000000-0000-4000-8000-000000000024"
	if _, err = pool.Exec(ctx, `INSERT INTO public.solicitud_titular (id, usuario_id, tipo, canal, estado, solicitada_en, vence_en)
		VALUES ($1, $2, 'oposicion', 'web', 'recibida', $3, $4)`,
		receivedID, owner, now, now.AddDate(0, 0, 30)); err != nil {
		t.Fatalf("insert recibida request: %v", err)
	}
	reviewDetail, err := runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: receivedID, Action: "start_review", ExpectedVersion: 1,
	})
	if err != nil || reviewDetail.Status != "en_revision" || reviewDetail.Version != 2 {
		t.Fatalf("start_review(%s): detail=%+v err=%v", receivedID, reviewDetail, err)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: bloqueoReq.ID, Decision: "resuelta", Response: "Bloqueo temporal aplicado mientras dure la revision.",
	}); err != nil {
		t.Fatalf("resolve bloqueo: %v", err)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: rectReq.ID, Decision: "resuelta", Response: "Razon social rectificada conforme al respaldo.",
	}); err != nil {
		t.Fatalf("resolve rectificacion: %v", err)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Decision: "resuelta", Response: "Archivo de portabilidad emitido al titular.",
	}); err != nil {
		t.Fatalf("resolve portabilidad: %v", err)
	}
	if _, err = runtimeService.RecordPrivacyDispatch(ctx, adminglobal.RecordPrivacyDispatchInput{
		ActorID: master, CorrelationID: corr, RequestID: adminCreated.ID, Channel: "email",
		Destination: "titular.wf@ejemplo.invalid", SentAt: now, EvidenceRef: "MSG-WF-001",
	}); err != nil {
		t.Fatalf("RecordPrivacyDispatch(portabilidad): %v", err)
	}

	// Admin detail includes internal notes & all events
	adminDetail, err := runtimeService.GetPrivacyRequest(ctx, master, corr, adminCreated.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest: %v", err)
	}
	var sawInternalInAdmin bool
	for _, ev := range adminDetail.Events {
		if ev.EventType == "nota_interna" && ev.Detail != nil && strings.Contains(*ev.Detail, "SECRETO INTERNO") {
			sawInternalInAdmin = true
		}
	}
	if !sawInternalInAdmin {
		t.Fatalf("expected nota_interna in admin detail events: %+v", adminDetail.Events)
	}

	// Titular portal list must isolate internal notes/assignments and expose titular events & deadlines
	ownRequests, err := identityRepo.ListOwnRightsRequests(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnRightsRequests: %v", err)
	}
	var foundPortabilidad bool
	for _, req := range ownRequests {
		if req.ID == adminCreated.ID {
			foundPortabilidad = true
			if req.DueAt == nil || req.ExtendedDueAt == nil || req.ExtensionReason == nil {
				t.Fatalf("expected deadline & extension fields on titular portabilidad request: %+v", req)
			}
			var hasClarif, hasExt, hasRes, hasDispatch bool
			for _, ev := range req.Events {
				if ev.EventType == "nota_interna" || ev.EventType == "asignacion" || (ev.Detail != nil && (strings.Contains(*ev.Detail, "SECRETO INTERNO") || strings.Contains(*ev.Detail, "MSG-WF-001"))) {
					t.Fatalf("leaked internal/sensitive event to titular: %+v", ev)
				}
				switch ev.EventType {
				case "aclaracion_solicitada":
					hasClarif = true
				case "prorroga":
					hasExt = true
				case "resolucion":
					hasRes = true
				case "despacho_registrado":
					hasDispatch = true
				}
			}
			if !hasClarif || !hasExt || !hasRes || !hasDispatch {
				t.Fatalf("missing titular visible events (clarif=%v ext=%v res=%v dispatch=%v) in %+v", hasClarif, hasExt, hasRes, hasDispatch, req.Events)
			}
		}
	}
	if !foundPortabilidad {
		t.Fatalf("portabilidad request missing from ListOwnRightsRequests")
	}

	// Overdue filter matches open expired case and excludes completed expired case
	openExpired, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "acceso", "web", nil)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(openExpired): %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.solicitud_titular SET vence_en = $2 WHERE id IN ($1, $3)`, rectReq.ID, now.Add(-24*time.Hour), openExpired.ID); err != nil {
		t.Fatal(err)
	}
	overdueFlag := true
	overdueList, err := runtimeService.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{
		AccountID: owner, Channel: "web", Overdue: &overdueFlag, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListPrivacyRequests overdue: %v", err)
	}
	if len(overdueList.Items) != 1 || overdueList.Items[0].ID != openExpired.ID || !overdueList.Items[0].IsOverdue {
		t.Fatalf("expected only openExpired in overdue list, got: %+v", overdueList.Items)
	}

	// Verify espacigo_runtime append-only enforcement on solicitud_titular_evento
	if _, err = runtimePool.Exec(ctx, `UPDATE public.solicitud_titular_evento SET detalle = 'mutado' WHERE solicitud_id = $1`, adminCreated.ID); err == nil {
		t.Fatalf("expected UPDATE on solicitud_titular_evento to fail under espacigo_runtime")
	}
	if _, err = runtimePool.Exec(ctx, `DELETE FROM public.solicitud_titular_evento WHERE solicitud_id = $1`, adminCreated.ID); err == nil {
		t.Fatalf("expected DELETE on solicitud_titular_evento to fail under espacigo_runtime")
	}
}

type fakePrivacyNoticeSender struct {
	fail bool
	sent []string
}

func (f *fakePrivacyNoticeSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	if f.fail {
		return errors.New("smtp connection refused")
	}
	f.sent = append(f.sent, email+"|"+subject+"|"+body)
	return nil
}

func TestPrivacyPublicIntakeOutboxAndRetry(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_priv_pub_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const master = "00000000-0000-4000-8000-000000000031"
	const owner = "00000000-0000-4000-8000-000000000032"
	const corr = "10000000-0000-4000-8000-000000000031"
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'pub-master@example.invalid','pub-master@example.invalid','synthetic','activo','2026-01-01'),
		($2,'pub-owner@example.invalid','pub-owner@example.invalid','synthetic','activo','2026-01-01')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible) VALUES($1,'Master Public'),($2,'Titular Vinculado')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,'arrendatario'),($2,'arrendatario')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatal(err)
	}
	if err = dbbootstrap.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("grant runtime permissions: %v", err)
	}
	runtimeCfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(c context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(c, `SET ROLE espacigo_runtime`)
		return e
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()

	runtimeService, err := adminglobal.New(runtimePool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypg.NewIdentityRepository(runtimePool)
	privacySvc, err := privacy.NewService(identityRepo)
	if err != nil {
		t.Fatal(err)
	}

	const repEmail = "representante.priv@ejemplo.cl"
	receipt, err := privacySvc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:              "rectificacion",
		RequesterType:          "representante",
		RequesterName:          "Estudio Juridico Norte",
		RequesterEmail:         repEmail,
		RepresentativeName:     "Camila Representante",
		RepresentativeRelation: "mandatario",
		SubjectReference:       "Titular sin cuenta vinculada inicialmente",
		Detail:                 "Solicitamos rectificación de datos personales en nombre del titular.",
	})
	if err != nil {
		t.Fatalf("SubmitPublicRightsRequest: %v", err)
	}
	if receipt.Folio == "" || receipt.TrackingCode == "" || receipt.Channel != "publico" {
		t.Fatalf("unexpected public receipt: %+v", receipt)
	}

	// Verify ListPrivacyRequests shows public intake with folio & masked email and zero plain email/token exposure.
	listed, err := runtimeService.ListPrivacyRequests(ctx, master, corr, adminglobal.PrivacyRequestFilter{
		Channel:  "publico",
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListPrivacyRequests(channel=publico): %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != receipt.ID {
		t.Fatalf("expected public request in ListPrivacyRequests, got %+v", listed.Items)
	}
	item := listed.Items[0]
	if item.Folio != receipt.Folio || item.AccountID != "" || item.RequesterType != "representante" ||
		item.RequesterName != "Estudio Juridico Norte" || item.RepresentativeName != "Camila Representante" ||
		item.RepresentativeRelation != "mandatario" || item.SubjectReference == "" {
		t.Fatalf("unexpected projected public item fields: %+v", item)
	}
	if item.RequesterEmailMasked == "" || item.RequesterEmailMasked == repEmail || strings.Contains(item.RequesterEmailMasked, "@") {
		t.Fatalf("expected masked email without clear address, got %q", item.RequesterEmailMasked)
	}
	listedJSON, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listedJSON), repEmail) || strings.Contains(string(listedJSON), receipt.TrackingCode) {
		t.Fatalf("list JSON exposed plain email or tracking code: %s", string(listedJSON))
	}

	// Verify identity linking to real account (v1 -> v2)
	detail, err := runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID:         master,
		CorrelationID:   corr,
		RequestID:       receipt.ID,
		Action:          "verify_identity",
		AccountID:       owner,
		ExpectedVersion: 1,
		Note:            "Poder notarial validado y vinculado al titular",
	})
	if err != nil {
		t.Fatalf("verify_identity with AccountID: %v", err)
	}
	if detail.AccountID != owner || detail.Status != "en_revision" || detail.Version != 2 {
		t.Fatalf("unexpected detail after verify_identity: %+v", detail)
	}
	// Attempting to re-verify or link conflicting account must fail with ErrConflict.
	if _, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID:         master,
		CorrelationID:   corr,
		RequestID:       receipt.ID,
		Action:          "verify_identity",
		AccountID:       master,
		ExpectedVersion: 2,
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict on second verify_identity, got %v", err)
	}

	// Add internal note (v2 -> v3), request clarification (v3 -> v4), extend deadline (v4 -> v5), resolve (v5 -> v6)
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID:         master,
		CorrelationID:   corr,
		RequestID:       receipt.ID,
		Action:          "add_internal_note",
		ExpectedVersion: 2,
		Note:            "SECRETO INTERNO PUBLICO NO EXPONER",
	})
	if err != nil || detail.Version != 3 {
		t.Fatalf("add_internal_note: detail=%+v err=%v", detail, err)
	}
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID:         master,
		CorrelationID:   corr,
		RequestID:       receipt.ID,
		Action:          "request_clarification",
		ExpectedVersion: 3,
		Note:            "Indicar el nuevo correo de contacto a rectificar.",
	})
	if err != nil || detail.Version != 4 {
		t.Fatalf("request_clarification: detail=%+v err=%v", detail, err)
	}
	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID:         master,
		CorrelationID:   corr,
		RequestID:       receipt.ID,
		Action:          "extend_deadline",
		ExpectedVersion: 4,
		ExtraDays:       10,
		Note:            "Revisión de antecedentes documentales del mandato.",
	})
	if err != nil || detail.Version != 5 {
		t.Fatalf("extend_deadline: detail=%+v err=%v", detail, err)
	}
	const resolvedResponse = "Rectificación aplicada según mandato acreditado."
	resolvedItem, err := runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID:       master,
		CorrelationID: corr,
		RequestID:     receipt.ID,
		Decision:      "resuelta",
		Response:      resolvedResponse,
	})
	if err != nil || resolvedItem.Status != "completada" || resolvedItem.Version != 6 {
		t.Fatalf("ResolvePrivacyRequest: item=%+v err=%v", resolvedItem, err)
	}

	// Confirm 4 outbox rows were enqueued (acuse_recepcion, aclaracion_solicitada, prorroga_plazo, resolucion_publicada)
	// and never include internal note or response text in outbox bodies.
	detailBeforeDrain, err := runtimeService.GetPrivacyRequest(ctx, master, corr, receipt.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest before drain: %v", err)
	}
	if detailBeforeDrain.NotificationSummary == nil || detailBeforeDrain.NotificationSummary.Pending != 4 ||
		detailBeforeDrain.NotificationSummary.Delivered != 0 || detailBeforeDrain.NotificationSummary.Terminal != 0 {
		t.Fatalf("unexpected notification summary before drain: %+v", detailBeforeDrain.NotificationSummary)
	}
	detailJSON, err := json.Marshal(detailBeforeDrain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(detailJSON), repEmail) || strings.Contains(string(detailJSON), receipt.TrackingCode) {
		t.Fatalf("detail JSON exposed plain email or tracking code: %s", string(detailJSON))
	}

	// Drain with failing sender: 6 failures per notice -> all 4 become terminal.
	failingSender := &fakePrivacyNoticeSender{fail: true}
	for step := 0; step < 30; step++ {
		if _, err = runtimePool.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox SET disponible_en = now() - interval '1 second' WHERE solicitud_id = $1 AND estado = 'pending'`, receipt.ID); err != nil {
			t.Fatal(err)
		}
		processed, drainErr := privacySvc.DispatchPrivacyNoticeOnce(ctx, failingSender)
		if drainErr != nil {
			t.Fatalf("DispatchPrivacyNoticeOnce(failing) step=%d: %v", step, drainErr)
		}
		if !processed {
			break
		}
	}

	detailAfterFail, err := runtimeService.GetPrivacyRequest(ctx, master, corr, receipt.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest after failures: %v", err)
	}
	if detailAfterFail.NotificationSummary == nil || detailAfterFail.NotificationSummary.Terminal != 4 ||
		detailAfterFail.NotificationSummary.Pending != 0 || detailAfterFail.NotificationSummary.Delivered != 0 ||
		detailAfterFail.NotificationSummary.LastErrorCode == nil || *detailAfterFail.NotificationSummary.LastErrorCode != "attempt_limit_reached" {
		t.Fatalf("expected 4 terminal notices with attempt_limit_reached, got %+v", detailAfterFail.NotificationSummary)
	}

	// RetryPrivacyNotifications resets terminal rows to pending.
	retriedDetail, err := runtimeService.RetryPrivacyNotifications(ctx, master, corr, receipt.ID)
	if err != nil {
		t.Fatalf("RetryPrivacyNotifications: %v", err)
	}
	if retriedDetail.NotificationSummary == nil || retriedDetail.NotificationSummary.Pending != 4 ||
		retriedDetail.NotificationSummary.Terminal != 0 || retriedDetail.NotificationSummary.LastErrorCode != nil {
		t.Fatalf("unexpected notification summary after retry: %+v", retriedDetail.NotificationSummary)
	}
	// Second retry with no terminal rows returns ErrConflict.
	if _, err = runtimeService.RetryPrivacyNotifications(ctx, master, corr, receipt.ID); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict when no terminal notices remain, got %v", err)
	}

	// Drain with succeeding sender -> all 4 delivered.
	okSender := &fakePrivacyNoticeSender{fail: false}
	for step := 0; step < 10; step++ {
		processed, drainErr := privacySvc.DispatchPrivacyNoticeOnce(ctx, okSender)
		if drainErr != nil {
			t.Fatalf("DispatchPrivacyNoticeOnce(ok) step=%d: %v", step, drainErr)
		}
		if !processed {
			break
		}
	}
	if len(okSender.sent) != 4 {
		t.Fatalf("expected 4 delivered emails, got %d (%v)", len(okSender.sent), okSender.sent)
	}
	for _, sent := range okSender.sent {
		if !strings.HasPrefix(sent, repEmail+"|") {
			t.Fatalf("expected email sent to %s, got %q", repEmail, sent)
		}
		if strings.Contains(sent, "SECRETO INTERNO PUBLICO") || strings.Contains(sent, resolvedResponse) {
			t.Fatalf("outbox email leaked internal note or response text: %q", sent)
		}
	}

	detailAfterDelivery, err := runtimeService.GetPrivacyRequest(ctx, master, corr, receipt.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest after delivery: %v", err)
	}
	if detailAfterDelivery.NotificationSummary == nil || detailAfterDelivery.NotificationSummary.Delivered != 4 ||
		detailAfterDelivery.NotificationSummary.Pending != 0 || detailAfterDelivery.NotificationSummary.Terminal != 0 ||
		detailAfterDelivery.NotificationSummary.LastErrorCode != nil || detailAfterDelivery.NotificationSummary.LastDeliveredAt == nil {
		t.Fatalf("unexpected notification summary after delivery: %+v", detailAfterDelivery.NotificationSummary)
	}

	// Verify audit table never recorded clear email or tracking code.
	var auditLeak bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM public.evento_auditoria_local
			WHERE recurso_id = $1
			  AND (motivo_codigo LIKE '%' || $2 || '%' OR motivo_codigo LIKE '%' || $3 || '%')
		)
	`, receipt.ID, repEmail, receipt.TrackingCode).Scan(&auditLeak); err != nil || auditLeak {
		t.Fatalf("audit log leaked sensitive email or tracking code: leak=%v err=%v", auditLeak, err)
	}

	// Public lookup isolates internal notes and exposes response and titular events.
	publicStatus, err := privacySvc.LookupPublicRightsRequest(ctx, privacy.PublicRightsLookupInput{
		Folio:        receipt.Folio,
		TrackingCode: receipt.TrackingCode,
	})
	if err != nil {
		t.Fatalf("LookupPublicRightsRequest: %v", err)
	}
	if publicStatus.State != "resuelta" || publicStatus.Response == nil || *publicStatus.Response != resolvedResponse {
		t.Fatalf("unexpected public lookup status/response: %+v", publicStatus)
	}
	var sawVerified, sawClarif, sawExt, sawRes bool
	for _, ev := range publicStatus.Events {
		if ev.EventType == "nota_interna" || ev.EventType == "asignacion" || (ev.Detail != nil && strings.Contains(*ev.Detail, "SECRETO INTERNO")) {
			t.Fatalf("public lookup leaked internal event: %+v", ev)
		}
		switch ev.EventType {
		case "identidad_verificada":
			sawVerified = true
		case "aclaracion_solicitada":
			sawClarif = true
		case "prorroga":
			sawExt = true
		case "resolucion":
			sawRes = true
		}
	}
	if !sawVerified || !sawClarif || !sawExt || !sawRes {
		t.Fatalf("public lookup missing titular events (verified=%v clarif=%v ext=%v res=%v): %+v", sawVerified, sawClarif, sawExt, sawRes, publicStatus.Events)
	}
}

func TestPrivacyPhase4RightsExecutionEndToEnd(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_priv_p4_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const master = "00000000-0000-4000-8000-000000000041"
	const owner = "00000000-0000-4000-8000-000000000042"
	const corr = "10000000-0000-4000-8000-000000000041"
	const ownerEmail = "p4-owner@example.invalid"
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'p4-master@example.invalid','p4-master@example.invalid','synthetic','activo','2026-01-01'),
		($2,$3,$3,'synthetic','activo','2026-01-01')`, master, owner, ownerEmail); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible,telefono_normalizado) VALUES
		($1,'Master Ejecucion',NULL),($2,'Nombre Inicial','912345678')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,'arrendatario'),($2,'arrendatario')`, master, owner); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatal(err)
	}
	if err = dbbootstrap.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("grant runtime permissions: %v", err)
	}
	runtimeCfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(c context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(c, `SET ROLE espacigo_runtime`)
		return e
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()

	now := time.Date(2026, 11, 27, 17, 0, 0, 0, time.UTC)
	runtimeService, err := adminglobal.New(runtimePool, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypg.NewIdentityRepository(runtimePool)
	privacySvc, err := privacy.NewService(identityRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Unlinked public request cannot be executed until linked to an account.
	pubReceipt, err := privacySvc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:      "rectificacion",
		RequesterType:  "titular",
		RequesterName:  "Persona Sin Vincular",
		RequesterEmail: "unlinked@ejemplo.cl",
		Detail:         "Solicitud de rectificación pública pendiente de vinculación.",
	})
	if err != nil {
		t.Fatalf("SubmitPublicRightsRequest: %v", err)
	}
	if _, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID, Action: "verify_identity", ExpectedVersion: 1, Note: "Identidad verificada sin cuenta",
	}); err != nil {
		t.Fatalf("verify_identity without account: %v", err)
	}
	unlinkedName := "Sin Cuenta"
	if _, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID, Action: "rectify_profile", ExpectedVersion: 2,
		Summary: "Intento sobre caso sin cuenta vinculada", DisplayName: &unlinkedName,
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict on unlinked request, got %v", err)
	}

	// 2. rectify_profile preserves untouched fields, clears empty phone, and keeps audit PII-free.
	rectDetail := "Solicito corregir mi nombre visible registrado."
	rectReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "rectificacion", "web", &rectDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(rectificacion): %v", err)
	}
	newName := "Camila Rectificada"
	detail, err := runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: rectReq.ID, Action: "rectify_profile", ExpectedVersion: 1,
		Summary: "Rectificación de nombre visible según respaldo validado", DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(rectify_profile name): %v", err)
	}
	if detail.Version != 2 || len(detail.Executions) != 1 || detail.Executions[0].ExecutionType != "rectificacion_perfil" {
		t.Fatalf("unexpected detail after rectify_profile(name): %+v", detail)
	}
	var gotName string
	var gotPhone *string
	if err = pool.QueryRow(ctx, `SELECT nombre_visible, telefono_normalizado FROM public.perfil_usuario WHERE usuario_id=$1`, owner).Scan(&gotName, &gotPhone); err != nil {
		t.Fatal(err)
	}
	if gotName != newName || gotPhone == nil || *gotPhone != "912345678" {
		t.Fatalf("expected name=%q and preserved phone='912345678', got name=%q phone=%v", newName, gotName, gotPhone)
	}

	clearPhone := ""
	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: rectReq.ID, Action: "rectify_profile", ExpectedVersion: 2,
		Summary: "Limpieza de teléfono de contacto según solicitud", Phone: &clearPhone,
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(rectify_profile clear phone): %v", err)
	}
	if detail.Version != 3 || len(detail.Executions) != 2 {
		t.Fatalf("unexpected detail after rectify_profile(clear phone): %+v", detail)
	}
	if err = pool.QueryRow(ctx, `SELECT nombre_visible, telefono_normalizado FROM public.perfil_usuario WHERE usuario_id=$1`, owner).Scan(&gotName, &gotPhone); err != nil {
		t.Fatal(err)
	}
	if gotName != newName || gotPhone != nil {
		t.Fatalf("expected preserved name=%q and NULL phone, got name=%q phone=%v", newName, gotName, gotPhone)
	}

	var auditCount int
	var auditLeak bool
	if err = pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       BOOL_OR(motivo_codigo LIKE '%Camila%' OR motivo_codigo LIKE '%912345678%' OR motivo_codigo LIKE '%' || $2 || '%')
		FROM public.evento_auditoria_local
		WHERE recurso_id = $1 AND accion = 'admin.global.privacy_requests.execute.rectify_profile'
	`, rectReq.ID, ownerEmail).Scan(&auditCount, &auditLeak); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 || auditLeak {
		t.Fatalf("unexpected audit state for rectify_profile: count=%d leak=%v", auditCount, auditLeak)
	}

	// 3. apply_opposition updates purpose preferences and is visible to owner.
	oppDetail := "Me opongo a recibir comunicaciones comerciales promocionales."
	oppReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "oposicion", "web", &oppDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(oposicion): %v", err)
	}
	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: oppReq.ID, Action: "apply_opposition", ExpectedVersion: 1,
		Summary: "Oposición aplicada sobre comunicaciones comerciales", Purpose: privacy.PurposeCommercialComms, PurposeState: privacy.PurposeStateOpposed,
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(apply_opposition): %v", err)
	}
	if len(detail.PurposePreferences) != 4 || detail.PurposePreferences[0].Purpose != privacy.PurposeCommercialComms || detail.PurposePreferences[0].State != privacy.PurposeStateOpposed {
		t.Fatalf("unexpected purpose preferences in detail: %+v", detail.PurposePreferences)
	}
	ownPrefs, err := privacySvc.OwnPurposePreferences(ctx, owner)
	if err != nil {
		t.Fatalf("OwnPurposePreferences: %v", err)
	}
	if ownPrefs[0].State != privacy.PurposeStateOpposed || ownPrefs[0].RequestID == nil || *ownPrefs[0].RequestID != oppReq.ID {
		t.Fatalf("owner did not observe updated preference: %+v", ownPrefs)
	}

	// 4. apply_block + duplicate conflict + lift_block.
	blockDetail := "Solicito bloqueo temporal de analítica y perfilado."
	blockReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "bloqueo", "web", &blockDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(bloqueo): %v", err)
	}
	reviewAt := now.Add(72 * time.Hour)
	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: blockReq.ID, Action: "apply_block", ExpectedVersion: 1,
		Summary: "Bloqueo temporal aplicado mientras dura la revisión", Scope: privacy.ScopeAnalyticsProfile, ReviewAt: &reviewAt,
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(apply_block): %v", err)
	}
	if len(detail.TreatmentBlocks) != 1 || detail.TreatmentBlocks[0].State != "activo" || detail.TreatmentBlocks[0].ReviewAt == nil {
		t.Fatalf("unexpected treatment blocks after apply_block: %+v", detail.TreatmentBlocks)
	}
	blockID := detail.TreatmentBlocks[0].ID

	hasBlock, err := privacySvc.HasActiveTreatmentBlock(ctx, owner, privacy.ScopeAnalyticsProfile)
	if err != nil || !hasBlock {
		t.Fatalf("HasActiveTreatmentBlock = %v, err=%v", hasBlock, err)
	}

	// Duplicate active block on same account + scope returns ErrConflict.
	if _, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: blockReq.ID, Action: "apply_block", ExpectedVersion: 2,
		Summary: "Intento de segundo bloqueo activo en mismo ámbito", Scope: privacy.ScopeAnalyticsProfile,
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate active block, got %v", err)
	}

	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: blockReq.ID, Action: "lift_block", ExpectedVersion: 2,
		Summary: "Revisión finalizada, se levanta el bloqueo temporal", BlockID: blockID,
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(lift_block): %v", err)
	}
	if len(detail.TreatmentBlocks) != 1 || detail.TreatmentBlocks[0].State != "levantado" || detail.TreatmentBlocks[0].LiftedAt == nil || detail.TreatmentBlocks[0].LiftReason == nil {
		t.Fatalf("unexpected treatment block after lift_block: %+v", detail.TreatmentBlocks)
	}
	hasBlock, err = privacySvc.HasActiveTreatmentBlock(ctx, owner, privacy.ScopeAnalyticsProfile)
	if err != nil || hasBlock {
		t.Fatalf("expected active block to be lifted, got hasBlock=%v err=%v", hasBlock, err)
	}
	ownBlocks, err := privacySvc.OwnTreatmentRestrictions(ctx, owner)
	if err != nil || len(ownBlocks) != 1 || ownBlocks[0].State != "levantado" {
		t.Fatalf("unexpected OwnTreatmentRestrictions: %+v err=%v", ownBlocks, err)
	}

	// 5. issue_portability emits 64-hex SHA-256 and supports portability transfer reference.
	portDetail := "Solicito exportación interoperable de mis datos personales."
	portReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "portabilidad", "web", &portDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(portabilidad): %v", err)
	}
	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: portReq.ID, Action: "issue_portability", ExpectedVersion: 1,
		Summary: "Emisión del paquete JSON de portabilidad interoperable",
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(issue_portability): %v", err)
	}
	if len(detail.Executions) != 1 || detail.Executions[0].ExecutionType != "portabilidad_emitida" ||
		detail.Executions[0].ArtifactSHA256 == nil || len(*detail.Executions[0].ArtifactSHA256) != 64 {
		t.Fatalf("unexpected portability execution: %+v", detail.Executions)
	}

	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: portReq.ID, Action: "issue_portability", ExpectedVersion: 2,
		Summary: "Transferencia directa de paquete de portabilidad al receptor", TargetReference: "RECEPTOR-EXT-2026-01",
	})
	if err != nil {
		t.Fatalf("ExecutePrivacyRight(issue_portability transfer): %v", err)
	}
	var foundTransfer bool
	for _, ex := range detail.Executions {
		if ex.ExecutionType == "transferencia_portabilidad" &&
			ex.TargetReference != nil && *ex.TargetReference == "RECEPTOR-EXT-2026-01" &&
			ex.ArtifactSHA256 != nil && len(*ex.ArtifactSHA256) == 64 {
			foundTransfer = true
		}
	}
	if len(detail.Executions) != 2 || !foundTransfer {
		t.Fatalf("unexpected portability transfer execution: %+v", detail.Executions)
	}

	portBytes, portSHA, err := privacySvc.ExportOwnPortability(ctx, owner)
	if err != nil {
		t.Fatalf("ExportOwnPortability: %v", err)
	}
	if len(portSHA) != 64 {
		t.Fatalf("expected 64-hex SHA from ExportOwnPortability, got %q", portSHA)
	}
	var portPkg privacy.PortabilityPackage
	if err = json.Unmarshal(portBytes, &portPkg); err != nil {
		t.Fatalf("unmarshal portability package: %v", err)
	}
	if portPkg.Profile == nil || portPkg.Profile.Name != newName || portPkg.Profile.Phone != nil ||
		len(portPkg.PurposePreferences) != 4 || portPkg.PurposePreferences[0].State != privacy.PurposeStateOpposed ||
		len(portPkg.TreatmentRestrictions) != 1 || portPkg.TreatmentRestrictions[0].State != "levantado" {
		t.Fatalf("unexpected owner portability package contents: %+v", portPkg)
	}

	// 6. Verify privacidad_ejecucion_derecho rejects UPDATE and DELETE for espacigo_runtime.
	var pgErr *pgconn.PgError
	if _, err = runtimePool.Exec(ctx, `UPDATE public.privacidad_ejecucion_derecho SET resumen_operativo = 'mutado' WHERE solicitud_id = $1`, portReq.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on UPDATE privacidad_ejecucion_derecho, got %v", err)
	}
	pgErr = nil
	if _, err = runtimePool.Exec(ctx, `DELETE FROM public.privacidad_ejecucion_derecho WHERE solicitud_id = $1`, portReq.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on DELETE privacidad_ejecucion_derecho, got %v", err)
	}
}

func TestPrivacyPhase57IncidentsLegalHoldsAndKPIs(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; needs a disposable PostgreSQL 18/PostGIS 3.6 instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("espacigo_priv_p57_%08x", uint32(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); e != nil {
			t.Errorf("drop isolated database: %v", e)
		}
	}()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	extPool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"unaccent", "postgis", "btree_gist", "pgcrypto"} {
		var installed bool
		if err = extPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = $1)`, ext).Scan(&installed); err != nil {
			extPool.Close()
			t.Fatal(err)
		}
		if !installed {
			if _, err = extPool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{ext}.Sanitize()+" SCHEMA public"); err != nil {
				extPool.Close()
				t.Fatal(err)
			}
		}
	}
	extPool.Close()
	_, source, _, _ := runtime.Caller(0)
	migrationDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations"))
	if _, err = migrator.Run(ctx, u.String(), migrationDir); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const master = "00000000-0000-4000-8000-000000000051"
	const deputy = "00000000-0000-4000-8000-000000000052"
	const owner = "00000000-0000-4000-8000-000000000053"
	const corr = "10000000-0000-4000-8000-000000000051"
	const ownerEmail = "p57-owner@example.invalid"
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'p57-master@example.invalid','p57-master@example.invalid','synthetic','activo','2026-01-01'),
		($2,'p57-deputy@example.invalid','p57-deputy@example.invalid','synthetic','activo','2026-01-01'),
		($3,$4,$4,'synthetic','activo','2026-01-01')`, master, deputy, owner, ownerEmail); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.rol_usuario(usuario_id,rol) VALUES($1,'arrendatario'),($2,'arrendatario'),($3,'arrendatario')`, master, deputy, owner); err != nil {
		t.Fatal(err)
	}
	if err = adminglobal.Bootstrap(ctx, pool, master, "bootstrap", corr); err != nil {
		t.Fatal(err)
	}
	if err = dbbootstrap.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("grant runtime permissions: %v", err)
	}

	runtimeCfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.AfterConnect = func(c context.Context, conn *pgx.Conn) error {
		_, e := conn.Exec(c, `SET ROLE espacigo_runtime`)
		return e
	}
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()

	now := time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC)
	runtimeService, err := adminglobal.New(runtimePool, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypg.NewIdentityRepository(runtimePool)

	// Seed privacy requests + terminal outbox row for KPI verification.
	openReq, err := identityRepo.CreateRightsRequest(ctx, owner, "acceso", "web")
	if err != nil {
		t.Fatalf("CreateRightsRequest(open): %v", err)
	}
	overdueReq, err := identityRepo.CreateRightsRequest(ctx, owner, "supresion", "web")
	if err != nil {
		t.Fatalf("CreateRightsRequest(overdue): %v", err)
	}
	resolvedReq, err := identityRepo.CreateRightsRequest(ctx, owner, "portabilidad", "web")
	if err != nil {
		t.Fatalf("CreateRightsRequest(resolved): %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE public.solicitud_titular SET vence_en = $2 WHERE id = $1`, overdueReq.ID, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: resolvedReq.ID, Decision: "resuelta", Response: "Paquete de portabilidad emitido y disponible para el titular.",
	}); err != nil {
		t.Fatalf("ResolvePrivacyRequest: %v", err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO public.privacidad_notificacion_outbox (
			solicitud_id, tipo_aviso, clave_idempotencia, destinatario_correo, asunto, cuerpo_texto, estado, intentos, disponible_en, terminal_en, ultimo_error_codigo, creado_en
		) VALUES ($1, 'acuse_recepcion', 'term-kpi-1', $2, 'Asunto', 'Cuerpo de prueba', 'terminal', 5, $3, $3, 'mail_delivery_failed', $3)
	`, openReq.ID, ownerEmail, now); err != nil {
		t.Fatal(err)
	}

	// 1. CreatePrivacyIncident validation + creation.
	if _, err = runtimeService.CreatePrivacyIncident(ctx, adminglobal.CreatePrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, Title: "Falta líder inexistente", Severity: "alta",
		DataCategories: []string{"correo"}, AffectedSystems: []string{"api"}, Summary: "Resumen suficiente para pasar validación.",
		LeaderID: "00000000-0000-4000-8000-999999999999", DetectedAt: now.Add(-time.Hour),
	}); !errors.Is(err, adminglobal.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing leader, got %v", err)
	}

	const sensitiveTitle = "Incidente confidencial de prueba con p57-owner@example.invalid"
	const sensitiveSummary = "Exposición temporal de respaldos detectada en monitoreo para p57-owner@example.invalid."
	inc, err := runtimeService.CreatePrivacyIncident(ctx, adminglobal.CreatePrivacyIncidentInput{
		ActorID:               master,
		CorrelationID:         corr,
		Title:                 sensitiveTitle,
		Severity:              "alta",
		DataCategories:        []string{"correo", "identificacion"},
		AffectedSystems:       []string{"s3_backup", "api"},
		EstimatedSubjects:     42,
		Summary:               sensitiveSummary,
		RequiresAgencyNotice:  true,
		RequiresSubjectNotice: false,
		LeaderID:              master,
		DeputyID:              deputy,
		DetectedAt:            now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreatePrivacyIncident: %v", err)
	}
	if !strings.HasPrefix(inc.Folio, "INC-2026-") || inc.Status != "abierto" || inc.Version != 1 || len(inc.Events) != 1 || inc.Events[0].EventType != "apertura" {
		t.Fatalf("unexpected created incident: %+v", inc)
	}

	// 2. Incident transitions: conflict, contain, evaluate, close-guard, notify_agency, notify_subjects, add_note, close.
	if _, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "contain", ExpectedVersion: 99,
		Detail: "Medidas de contención con versión incorrecta.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict on stale version, got %v", err)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "contain", ExpectedVersion: 1,
		Detail: "Se revocaron credenciales y se cerró acceso al bucket de respaldo.",
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(contain): %v", err)
	}
	if inc.Status != "contenido" || inc.Version != 2 || inc.ContainmentMeasures == nil || !strings.Contains(*inc.ContainmentMeasures, "credenciales") {
		t.Fatalf("unexpected incident after contain: %+v", inc)
	}

	reqSubjects := true
	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "evaluate", ExpectedVersion: 2,
		Detail: "Evaluación determina impacto sobre titulares.", RequiresSubjectNotice: &reqSubjects,
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(evaluate): %v", err)
	}
	if inc.Status != "evaluado" || inc.Version != 3 || !inc.RequiresSubjectNotice {
		t.Fatalf("unexpected incident after evaluate: %+v", inc)
	}

	// Cannot close while agency notice is required and not sent.
	if _, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "close", ExpectedVersion: 3,
		Detail: "Intento de cierre antes de notificar a la Agencia.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict closing before agency notification, got %v", err)
	}

	const sensitiveRef = "AGPD-OFICIO-2026-999"
	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "notify_agency", ExpectedVersion: 3,
		Detail: "Notificación formal enviada dentro de plazo legal.", AgencyReference: sensitiveRef,
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(notify_agency): %v", err)
	}
	if inc.Status != "notificado" || inc.Version != 4 || inc.AgencyNotifiedAt == nil || inc.AgencyReference == nil || *inc.AgencyReference != sensitiveRef {
		t.Fatalf("unexpected incident after notify_agency: %+v", inc)
	}

	// Cannot disable already-sent agency notification during evaluate.
	disableAgency := false
	if _, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "evaluate", ExpectedVersion: 4,
		Detail: "Intento de desactivar notificación ya enviada.", RequiresAgencyNotice: &disableAgency,
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict disabling already-sent agency notice, got %v", err)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "notify_subjects", ExpectedVersion: 4,
		Detail: "Aviso remitido a los titulares afectados.",
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(notify_subjects): %v", err)
	}
	if inc.SubjectsNotifiedAt == nil || inc.Version != 5 {
		t.Fatalf("unexpected incident after notify_subjects: %+v", inc)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "add_note", ExpectedVersion: 5,
		Detail: "Bitácora de seguimiento post-notificación.",
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(add_note): %v", err)
	}
	if inc.Status != "notificado" || inc.Version != 6 {
		t.Fatalf("unexpected incident after add_note: %+v", inc)
	}

	// 3. CreatePrivacyLegalHold + ListPrivacyLegalHolds + KPIs while open/active.
	if _, err = runtimeService.CreatePrivacyLegalHold(ctx, adminglobal.CreatePrivacyLegalHoldInput{
		ActorID: master, CorrelationID: corr, AccountID: "00000000-0000-4000-8000-999999999999",
		Scope: "expediente_privacidad", Authority: "Agencia de Protección de Datos",
		LegalBasis: "Resolución de conservación preventiva para expediente en curso.",
	}); !errors.Is(err, adminglobal.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing account_id in legal hold, got %v", err)
	}

	reviewAt := now.Add(30 * 24 * time.Hour)
	const sensitiveBasis = "Conservación ordenada respecto de p57-owner@example.invalid bajo expediente fiscal."
	hold, err := runtimeService.CreatePrivacyLegalHold(ctx, adminglobal.CreatePrivacyLegalHoldInput{
		ActorID:       master,
		CorrelationID: corr,
		AccountID:     owner,
		RequestID:     openReq.ID,
		IncidentID:    inc.ID,
		Scope:         "incidente_seguridad",
		Authority:     "Agencia de Protección de Datos",
		LegalBasis:    sensitiveBasis,
		ReviewAt:      &reviewAt,
	})
	if err != nil {
		t.Fatalf("CreatePrivacyLegalHold: %v", err)
	}
	if !strings.HasPrefix(hold.Folio, "HOLD-2026-") || hold.Status != "activa" || hold.AccountID == nil || *hold.AccountID != owner {
		t.Fatalf("unexpected created legal hold: %+v", hold)
	}

	incList, err := runtimeService.ListPrivacyIncidents(ctx, master, corr, adminglobal.PrivacyIncidentFilter{Status: "notificado", Severity: "alta", PageSize: 25})
	if err != nil || incList.MatchingTotal != 1 || len(incList.Items) != 1 || incList.Items[0].ID != inc.ID {
		t.Fatalf("ListPrivacyIncidents failed: list=%+v err=%v", incList, err)
	}
	gotInc, err := runtimeService.GetPrivacyIncident(ctx, master, corr, inc.ID)
	if err != nil || len(gotInc.Events) != 6 {
		t.Fatalf("GetPrivacyIncident failed: detail=%+v err=%v", gotInc, err)
	}
	holdList, err := runtimeService.ListPrivacyLegalHolds(ctx, master, corr, adminglobal.PrivacyLegalHoldFilter{Status: "activa", AccountID: owner, Scope: "incidente_seguridad", PageSize: 25})
	if err != nil || holdList.MatchingTotal != 1 || len(holdList.Items) != 1 || holdList.Items[0].ID != hold.ID {
		t.Fatalf("ListPrivacyLegalHolds failed: list=%+v err=%v", holdList, err)
	}

	// Verify KPIs while incident is open and legal hold is active.
	kpis, err := runtimeService.KPIs(ctx, master, corr, adminglobal.ReportPeriod{
		From: now.Add(-24 * time.Hour), Until: now.Add(24 * time.Hour), TimeZone: "America/Santiago",
	})
	if err != nil {
		t.Fatalf("KPIs(active): %v", err)
	}
	findQueueKPI := func(key string) *adminglobal.KPIValue {
		for i := range kpis.Queues {
			if kpis.Queues[i].Key == key {
				return &kpis.Queues[i]
			}
		}
		return nil
	}
	for key, wantVal := range map[string]int64{
		"privacy_requests_open":      2,
		"privacy_requests_overdue":   1,
		"privacy_outbox_terminal":    1,
		"privacy_incidents_open":     1,
		"privacy_legal_holds_active": 1,
	} {
		metric := findQueueKPI(key)
		if metric == nil || metric.Source != "privacidad_canonica" || !metric.Available || metric.Value != wantVal {
			t.Fatalf("KPI %s mismatch: got=%+v want value=%d", key, metric, wantVal)
		}
	}

	// 4. Close incident and release legal hold, then re-verify KPIs.
	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "close", ExpectedVersion: 6,
		Detail: "Incidente cerrado tras completar notificaciones y remediación.",
	})
	if err != nil {
		t.Fatalf("TransitionPrivacyIncident(close): %v", err)
	}
	if inc.Status != "cerrado" || inc.ClosedAt == nil || inc.Version != 7 || len(inc.Events) != 7 {
		t.Fatalf("unexpected incident after close: %+v", inc)
	}
	if _, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID, Action: "add_note", ExpectedVersion: 7,
		Detail: "Nota tardía sobre incidente ya cerrado.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict transitioning closed incident, got %v", err)
	}

	const sensitiveRelease = "Archivo definitivo confirmado para p57-owner@example.invalid."
	released, err := runtimeService.ReleasePrivacyLegalHold(ctx, adminglobal.ReleasePrivacyLegalHoldInput{
		ActorID: master, CorrelationID: corr, HoldID: hold.ID, ReleaseReason: sensitiveRelease,
	})
	if err != nil {
		t.Fatalf("ReleasePrivacyLegalHold: %v", err)
	}
	if released.Status != "liberada" || released.ReleasedAt == nil || released.ReleasedBy == nil || *released.ReleasedBy != master {
		t.Fatalf("unexpected released legal hold: %+v", released)
	}
	if _, err = runtimeService.ReleasePrivacyLegalHold(ctx, adminglobal.ReleasePrivacyLegalHoldInput{
		ActorID: master, CorrelationID: corr, HoldID: hold.ID, ReleaseReason: "Segundo intento de liberación sobre retención ya liberada.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict releasing already-released hold, got %v", err)
	}

	kpis, err = runtimeService.KPIs(ctx, master, corr, adminglobal.ReportPeriod{
		From: now.Add(-24 * time.Hour), Until: now.Add(24 * time.Hour), TimeZone: "America/Santiago",
	})
	if err != nil {
		t.Fatalf("KPIs(closed): %v", err)
	}
	if m := findQueueKPI("privacy_incidents_open"); m == nil || m.Value != 0 {
		t.Fatalf("expected privacy_incidents_open=0 after close, got %+v", m)
	}
	if m := findQueueKPI("privacy_legal_holds_active"); m == nil || m.Value != 0 {
		t.Fatalf("expected privacy_legal_holds_active=0 after release, got %+v", m)
	}

	// 5. Verify audit rows contain only fixed reason codes and zero PII / free text.
	var auditRows int
	var auditLeak bool
	if err = pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       BOOL_OR(
		           motivo_codigo LIKE '%' || $3 || '%'
		           OR motivo_codigo LIKE '%credenciales%'
		           OR motivo_codigo LIKE '%Oficio%'
		           OR motivo_codigo LIKE '%OFICIO%'
		           OR motivo_codigo LIKE '%Archivo%'
		           OR motivo_codigo LIKE '%Conservación%'
		           OR motivo_codigo NOT IN ('alta','contain','evaluate','notify_agency','notify_subjects','add_note','close','incidente_seguridad','retencion_liberada')
		       )
		FROM public.evento_auditoria_local
		WHERE recurso_id IN ($1, $2)
		  AND accion IN (
		      'admin.global.privacy_incidents.create',
		      'admin.global.privacy_incidents.transition',
		      'admin.global.privacy_legal_holds.create',
		      'admin.global.privacy_legal_holds.release'
		  )
	`, inc.ID, hold.ID, ownerEmail).Scan(&auditRows, &auditLeak); err != nil {
		t.Fatal(err)
	}
	if auditRows != 9 || auditLeak {
		t.Fatalf("unexpected audit state for phase 5-7: count=%d leak=%v", auditRows, auditLeak)
	}

	// 6. Verify espacigo_runtime receives SQLSTATE 42501 on prohibited UPDATE/DELETE statements.
	var pgErr *pgconn.PgError
	if _, err = runtimePool.Exec(ctx, `UPDATE public.privacidad_incidente_evento SET detalle = 'mutado' WHERE incidente_id = $1`, inc.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on UPDATE privacidad_incidente_evento, got %v", err)
	}
	pgErr = nil
	if _, err = runtimePool.Exec(ctx, `DELETE FROM public.privacidad_incidente_evento WHERE incidente_id = $1`, inc.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on DELETE privacidad_incidente_evento, got %v", err)
	}
	pgErr = nil
	if _, err = runtimePool.Exec(ctx, `DELETE FROM public.privacidad_incidente_datos WHERE id = $1`, inc.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on DELETE privacidad_incidente_datos, got %v", err)
	}
	pgErr = nil
	if _, err = runtimePool.Exec(ctx, `DELETE FROM public.privacidad_retencion_legal WHERE id = $1`, hold.ID); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected SQLSTATE 42501 on DELETE privacidad_retencion_legal, got %v", err)
	}
}
