package postgres_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPrivacyExecutionAndRestrictionsLeastPrivilegePostgres(t *testing.T) {
	ctx, pool := newIdentityTestPool(t)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbbootstrap.GrantRuntimePermissions(ctx, conn.Conn()); err != nil {
		conn.Release()
		t.Fatalf("GrantRuntimePermissions error = %v", err)
	}
	conn.Release()

	repo := identitypg.NewIdentityRepository(pool)
	svc, err := privacy.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	owner := testAccount("00000000-0000-4000-8000-000000000101", "exec-owner@ejemplo.cl")
	owner.State = identity.AccountActive
	inactive := testAccount("00000000-0000-4000-8000-000000000102", "exec-inactive@ejemplo.cl")
	other := testAccount("00000000-0000-4000-8000-000000000103", "exec-other@ejemplo.cl")
	other.State = identity.AccountActive
	for _, acc := range []identity.Account{owner, inactive, other} {
		if err := repo.CreateWithTerms(ctx, acc, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Inactive accounts cannot update purpose preferences.
	if _, err := svc.UpdateOwnPurposePreference(ctx, inactive.ID, privacy.UpdatePurposePreferenceInput{
		Purpose: privacy.PurposeCommercialComms,
		State:   privacy.PurposeStateOpposed,
		Reason:  "No deseo recibir comunicaciones comerciales",
	}); !errors.Is(err, privacy.ErrNotFound) {
		t.Fatalf("inactive account update err = %v, want ErrNotFound", err)
	}

	// Default purpose preferences are all 'permitido'.
	initialPrefs, err := svc.OwnPurposePreferences(ctx, owner.ID)
	if err != nil {
		t.Fatalf("OwnPurposePreferences error = %v", err)
	}
	if len(initialPrefs) != 4 {
		t.Fatalf("initialPrefs len = %d, want 4", len(initialPrefs))
	}
	for _, p := range initialPrefs {
		if p.State != privacy.PurposeStateAllowed {
			t.Fatalf("expected default permitido for %s, got %s", p.Purpose, p.State)
		}
	}

	// Update two preferences and check audit hygiene.
	if _, err := svc.UpdateOwnPurposePreference(ctx, owner.ID, privacy.UpdatePurposePreferenceInput{
		Purpose: privacy.PurposeCommercialComms,
		State:   privacy.PurposeStateOpposed,
		Reason:  "No autorizo correos promocionales",
	}); err != nil {
		t.Fatalf("UpdateOwnPurposePreference(opuesto) error = %v", err)
	}
	if _, err := svc.UpdateOwnPurposePreference(ctx, owner.ID, privacy.UpdatePurposePreferenceInput{
		Purpose: privacy.PurposeProductAnalytics,
		State:   privacy.PurposeStateWithdrawn,
	}); err != nil {
		t.Fatalf("UpdateOwnPurposePreference(retirado) error = %v", err)
	}

	updatedPrefs, err := svc.OwnPurposePreferences(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedPrefs[0].State != privacy.PurposeStateOpposed || updatedPrefs[0].Reason == nil || *updatedPrefs[0].Reason != "No autorizo correos promocionales" {
		t.Fatalf("unexpected commercial comms pref: %+v", updatedPrefs[0])
	}
	if updatedPrefs[1].State != privacy.PurposeStateWithdrawn || updatedPrefs[1].Reason != nil {
		t.Fatalf("unexpected product analytics pref: %+v", updatedPrefs[1])
	}
	if updatedPrefs[2].State != privacy.PurposeStateAllowed || updatedPrefs[3].State != privacy.PurposeStateAllowed {
		t.Fatalf("expected remaining purposes permitido: %+v", updatedPrefs[2:])
	}

	var auditCount int
	var auditReason string
	if err := pool.QueryRow(ctx, `SELECT count(*), max(motivo_codigo)
		FROM public.evento_auditoria_local
		WHERE actor_id = $1 AND accion = 'privacy.purpose_preference.update'`, owner.ID).Scan(&auditCount, &auditReason); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 || strings.Contains(auditReason, "promocionales") {
		t.Fatalf("unexpected audit log count=%d reason=%q", auditCount, auditReason)
	}

	// Create a blocking rights request and verify V000027 event constraint + execution + treatment restriction.
	blockReq, err := repo.CreateRightsRequestWithDetail(ctx, owner.ID, "bloqueo", "web", ptrStr("Solicito bloqueo temporal de analitica"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify ejecucion_derecho is allowed in solicitud_titular_evento and visibilidad_ck is preserved.
	if _, err := pool.Exec(ctx, `INSERT INTO public.solicitud_titular_evento (
		id, solicitud_id, secuencia, tipo_evento, visibilidad, actor_id, detalle
	) VALUES (gen_random_uuid(), $1, 2, 'ejecucion_derecho', 'titular', $2, 'Bloqueo aplicado en analitica')`, blockReq.ID, owner.ID); err != nil {
		t.Fatalf("insert ejecucion_derecho event failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.solicitud_titular_evento (
		id, solicitud_id, secuencia, tipo_evento, visibilidad, actor_id, detalle
	) VALUES (gen_random_uuid(), $1, 3, 'nota_interna', 'titular', $2, 'Debe fallar visibilidad')`, blockReq.ID, owner.ID); err == nil {
		t.Fatal("expected nota_interna with titular visibility to violate constraint")
	}

	var restrictionID string
	if err := pool.QueryRow(ctx, `INSERT INTO public.privacidad_bloqueo_tratamiento (
		cuenta_id, solicitud_id, ambito, estado, motivo, aplicado_por, revisar_en
	) VALUES ($1, $2, 'analitica_y_perfilado', 'activo', 'Bloqueo preventivo mientras se verifica exactitud', $1, $3)
	RETURNING id::text`, owner.ID, blockReq.ID, time.Now().UTC().Add(72*time.Hour)).Scan(&restrictionID); err != nil {
		t.Fatalf("insert privacidad_bloqueo_tratamiento failed: %v", err)
	}

	var executionID string
	if err := pool.QueryRow(ctx, `INSERT INTO public.privacidad_ejecucion_derecho (
		solicitud_id, cuenta_id, tipo_ejecucion, estado, ambito, resumen_operativo, ejecutado_por
	) VALUES ($1, $2, 'bloqueo_aplicado', 'ejecutada', 'analitica_y_perfilado', 'Se aplico bloqueo temporal sobre analitica y perfilado', $2)
	RETURNING id::text`, blockReq.ID, owner.ID).Scan(&executionID); err != nil {
		t.Fatalf("insert privacidad_ejecucion_derecho failed: %v", err)
	}

	hasAnalyticsBlock, err := svc.HasActiveTreatmentBlock(ctx, owner.ID, privacy.ScopeAnalyticsProfile)
	if err != nil || !hasAnalyticsBlock {
		t.Fatalf("HasActiveTreatmentBlock(analitica_y_perfilado) = %t err=%v", hasAnalyticsBlock, err)
	}
	hasPubBlock, err := svc.HasActiveTreatmentBlock(ctx, owner.ID, privacy.ScopePublicationsVis)
	if err != nil || hasPubBlock {
		t.Fatalf("HasActiveTreatmentBlock(publicaciones_y_visibilidad) = %t err=%v", hasPubBlock, err)
	}

	// Verify general block applies to every specific scope.
	otherReq, err := repo.CreateRightsRequestWithDetail(ctx, other.ID, "bloqueo", "web", ptrStr("Solicito bloqueo general temporal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.privacidad_bloqueo_tratamiento (
		cuenta_id, solicitud_id, ambito, estado, motivo, aplicado_por
	) VALUES ($1, $2, 'tratamiento_general', 'activo', 'Bloqueo general preventivo por impugnacion en curso', $1)`, other.ID, otherReq.ID); err != nil {
		t.Fatal(err)
	}
	hasGeneralBlock, err := svc.HasActiveTreatmentBlock(ctx, other.ID, privacy.ScopePublicationsVis)
	if err != nil || !hasGeneralBlock {
		t.Fatalf("expected tratamiento_general to block publicaciones_y_visibilidad, got %t err=%v", hasGeneralBlock, err)
	}

	// Verify ExportOwnData populates PurposePreferences and TreatmentRestrictions for owner,
	// and leaves them omitted cleanly when absent (fresh account).
	ownerExport, err := svc.ExportOwnData(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerExport.PurposePreferences) != 2 || len(ownerExport.TreatmentRestrictions) != 1 {
		t.Fatalf("unexpected ExportOwnData collections: prefs=%d restrictions=%d", len(ownerExport.PurposePreferences), len(ownerExport.TreatmentRestrictions))
	}

	fresh := testAccount("00000000-0000-4000-8000-000000000104", "exec-fresh@ejemplo.cl")
	if err := repo.CreateWithTerms(ctx, fresh, nil); err != nil {
		t.Fatal(err)
	}
	freshExport, err := svc.ExportOwnData(ctx, fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if freshExport.PurposePreferences != nil || freshExport.TreatmentRestrictions != nil {
		t.Fatalf("expected nil omitempty fields on fresh export, got %+v / %+v", freshExport.PurposePreferences, freshExport.TreatmentRestrictions)
	}
	freshJSON, err := json.Marshal(freshExport)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(freshJSON), "purpose_preferences") || strings.Contains(string(freshJSON), "treatment_restrictions") {
		t.Fatalf("expected omitempty to hide empty fields in OwnData JSON: %s", freshJSON)
	}

	// Verify ExportOwnPortability returns schema v1 JSON + matching SHA-256.
	portBytes, portSHA, err := svc.ExportOwnPortability(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ExportOwnPortability err = %v", err)
	}
	sum := sha256.Sum256(portBytes)
	if len(portSHA) != 64 || hex.EncodeToString(sum[:]) != portSHA {
		t.Fatalf("portability SHA mismatch: %s", portSHA)
	}
	var portPkg privacy.PortabilityPackage
	if err := json.Unmarshal(portBytes, &portPkg); err != nil {
		t.Fatal(err)
	}
	if portPkg.SchemaVersion != privacy.PortabilitySchemaVersion || len(portPkg.PurposePreferences) != 4 || len(portPkg.TreatmentRestrictions) != 1 {
		t.Fatalf("unexpected PortabilityPackage: %+v", portPkg)
	}

	// Verify least-privilege enforcement under espacigo_runtime.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}

	expect42501 := func(label, query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, `SAVEPOINT sp_priv`); err != nil {
			t.Fatal(err)
		}
		_, execErr := tx.Exec(ctx, query, args...)
		var pgErr *pgconn.PgError
		if !errors.As(execErr, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s: expected SQLSTATE 42501, got %v", label, execErr)
		}
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sp_priv`); err != nil {
			t.Fatal(err)
		}
	}

	expect42501("UPDATE privacidad_ejecucion_derecho",
		`UPDATE public.privacidad_ejecucion_derecho SET resumen_operativo = 'tamper_attempt' WHERE id = $1`, executionID)
	expect42501("DELETE privacidad_ejecucion_derecho",
		`DELETE FROM public.privacidad_ejecucion_derecho WHERE id = $1`, executionID)
	expect42501("DELETE privacidad_bloqueo_tratamiento",
		`DELETE FROM public.privacidad_bloqueo_tratamiento WHERE id = $1`, restrictionID)
	expect42501("UPDATE immutable motivo on privacidad_bloqueo_tratamiento",
		`UPDATE public.privacidad_bloqueo_tratamiento SET motivo = 'tamper_motivo_bloqueo' WHERE id = $1`, restrictionID)
	expect42501("DELETE privacidad_preferencia_finalidad",
		`DELETE FROM public.privacidad_preferencia_finalidad WHERE cuenta_id = $1`, owner.ID)

	// Verify permitted operations succeed under espacigo_runtime.
	if _, err := tx.Exec(ctx, `UPDATE public.privacidad_preferencia_finalidad
		SET estado = 'permitido', causal = NULL, actualizado_en = now()
		WHERE cuenta_id = $1 AND finalidad = 'comunicaciones_comerciales'`, owner.ID); err != nil {
		t.Fatalf("espacigo_runtime UPDATE privacidad_preferencia_finalidad failed: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE public.privacidad_bloqueo_tratamiento
		SET estado = 'levantado', levantado_por = $2, levantado_en = now(), motivo_levantamiento = 'Verificacion completada por titular'
		WHERE id = $1`, restrictionID, owner.ID); err != nil {
		t.Fatalf("espacigo_runtime UPDATE allowed columns on privacidad_bloqueo_tratamiento failed: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.privacidad_ejecucion_derecho (
		solicitud_id, cuenta_id, tipo_ejecucion, estado, ambito, resumen_operativo, ejecutado_por
	) VALUES ($1, $2, 'bloqueo_levantado', 'ejecutada', 'analitica_y_perfilado', 'Bloqueo levantado tras verificacion', $2)`, blockReq.ID, owner.ID); err != nil {
		t.Fatalf("espacigo_runtime INSERT privacidad_ejecucion_derecho failed: %v", err)
	}
}

func ptrStr(s string) *string { return &s }
