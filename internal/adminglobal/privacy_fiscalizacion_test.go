package adminglobal_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/adminglobal"
	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/migrator"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fiscalizacionNoticeSender struct {
	fail bool
	sent []fiscalizacionSentNotice
}

type fiscalizacionSentNotice struct {
	email   string
	subject string
	body    string
}

func (s *fiscalizacionNoticeSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	if s.fail {
		return errors.New("smtp connection refused")
	}
	s.sent = append(s.sent, fiscalizacionSentNotice{
		email:   email,
		subject: subject,
		body:    body,
	})
	return nil
}

func TestPrivacyPhase9FiscalizacionDrill(t *testing.T) {
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

	dbName := fmt.Sprintf("espacigo_priv_p9_%08x", uint32(time.Now().UnixNano()))
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

	const master = "00000000-0000-4000-8000-000000000091"
	const deputy = "00000000-0000-4000-8000-000000000092"
	const owner = "00000000-0000-4000-8000-000000000093"
	const corr = "10000000-0000-4000-8000-000000000091"
	const ownerEmail = "p9-owner@example.invalid"
	const repEmail = "rep.fiscalizacion@ejemplo.cl"

	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado,creado_en) VALUES
		($1,'p9-master@example.invalid','p9-master@example.invalid','synthetic','activo','2026-01-01'),
		($2,'p9-deputy@example.invalid','p9-deputy@example.invalid','synthetic','activo','2026-01-01'),
		($3,$4,$4,'synthetic','activo','2026-01-01')`, master, deputy, owner, ownerEmail); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.perfil_usuario(usuario_id,nombre_visible,telefono_normalizado) VALUES
		($1,'Oficial Privacidad',NULL),
		($2,'Titular Original','912345678')`, master, owner); err != nil {
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

	testStart := time.Now().UTC()
	runtimeService, err := adminglobal.New(runtimePool, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	identityRepo := identitypg.NewIdentityRepository(runtimePool)
	privacySvc, err := privacy.NewService(identityRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Recepción multicanal bajo espacigo_runtime.
	santiagoLoc := privacy.SantiagoLocation()

	pubReceipt, err := privacySvc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:              "bloqueo",
		RequesterType:          "representante",
		RequesterName:          "Estudio Jurídico Fiscalización",
		RequesterEmail:         repEmail,
		RepresentativeName:     "Valentina Representante",
		RepresentativeRelation: "representante_legal",
		SubjectReference:       "Titular sujeto a simulacro de fiscalización",
		Detail:                 "Solicitamos bloqueo temporal de comunicaciones comerciales mientras se verifica representación.",
	})
	if err != nil {
		t.Fatalf("SubmitPublicRightsRequest(bloqueo): %v", err)
	}
	if pubReceipt.Channel != "publico" || pubReceipt.RightType != "bloqueo" || pubReceipt.State != "identidad_pendiente" ||
		pubReceipt.Folio == "" || pubReceipt.TrackingCode == "" {
		t.Fatalf("unexpected public receipt: %+v", pubReceipt)
	}
	expectedBlockDue := privacy.ComputeInitialDeadline(pubReceipt.CreatedAt, "bloqueo")
	if !pubReceipt.DueAt.Equal(expectedBlockDue) {
		t.Fatalf("bloqueo DueAt=%v, want %v (2 business days in America/Santiago)", pubReceipt.DueAt, expectedBlockDue)
	}
	if localDue := pubReceipt.DueAt.In(santiagoLoc); localDue.Hour() != 23 || localDue.Minute() != 59 || localDue.Second() != 59 {
		t.Fatalf("expected bloqueo DueAt at end of day in America/Santiago, got %v", localDue)
	}

	rectDetail := "Solicito rectificar nombre visible y teléfono de contacto."
	rectReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "rectificacion", "web", &rectDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(rectificacion): %v", err)
	}
	portDetail := "Solicito portabilidad interoperable de mis datos personales."
	portReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "portabilidad", "web", &portDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(portabilidad): %v", err)
	}
	oppDetail := "Me opongo a comunicaciones comerciales promocionales."
	oppReq, err := identityRepo.CreateRightsRequestWithDetail(ctx, owner, "oposicion", "web", &oppDetail)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail(oposicion): %v", err)
	}

	for _, req := range []privacy.RightsRequest{rectReq, portReq, oppReq} {
		if req.DueAt == nil {
			t.Fatalf("expected DueAt on authenticated request %s (%s)", req.ID, req.Kind)
		}
		expectedDue := privacy.ComputeInitialDeadline(req.CreatedAt, req.Kind)
		if !req.DueAt.Equal(expectedDue) {
			t.Fatalf("request %s (%s) DueAt=%v, want %v (30 calendar days in America/Santiago)", req.ID, req.Kind, *req.DueAt, expectedDue)
		}
		localCreated := req.CreatedAt.In(santiagoLoc)
		localDue := req.DueAt.In(santiagoLoc)
		expectedLocalDate := localCreated.AddDate(0, 0, 30)
		if localDue.Year() != expectedLocalDate.Year() || localDue.Month() != expectedLocalDate.Month() ||
			localDue.Day() != expectedLocalDate.Day() || localDue.Hour() != 23 || localDue.Minute() != 59 || localDue.Second() != 59 {
			t.Fatalf("request %s (%s) America/Santiago due date mismatch: created=%v due=%v", req.ID, req.Kind, localCreated, localDue)
		}
	}

	// 2. Verificación y expediente único sobre el caso público.
	const internalSecretNote = "SECRETO-FISCALIZACION-NO-EXPONER"
	detail, err := runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "verify_identity", AccountID: owner, ExpectedVersion: 1,
		Note: "Mandato de representación legal verificado y vinculado al titular.",
	})
	if err != nil || detail.Status != "en_revision" || detail.AccountID != owner || detail.Version != 2 {
		t.Fatalf("verify_identity: detail=%+v err=%v", detail, err)
	}

	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "assign", AssignedToID: master, ExpectedVersion: 2,
		Note: "Asignado a oficial de protección de datos.",
	})
	if err != nil || detail.AssignedToID != master || detail.Version != 3 {
		t.Fatalf("assign: detail=%+v err=%v", detail, err)
	}

	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "add_internal_note", ExpectedVersion: 3,
		Note: internalSecretNote,
	})
	if err != nil || detail.Version != 4 {
		t.Fatalf("add_internal_note: detail=%+v err=%v", detail, err)
	}

	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "request_clarification", ExpectedVersion: 4,
		Note: "Confirmar periodo de vigencia requerido para el bloqueo.",
	})
	if err != nil || detail.Version != 5 {
		t.Fatalf("request_clarification: detail=%+v err=%v", detail, err)
	}

	detail, err = runtimeService.TransitionPrivacyRequest(ctx, adminglobal.TransitionPrivacyRequestInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "extend_deadline", ExpectedVersion: 5, ExtraDays: 10,
		Note: "Prórroga fundada para validación documental del mandato.",
	})
	if err != nil || detail.Version != 6 || detail.ExtendedDueAt == nil || detail.ExtensionReason == nil {
		t.Fatalf("extend_deadline: detail=%+v err=%v", detail, err)
	}

	// 3. Ejecución efectiva Fase 4.
	reviewAt := time.Now().UTC().Add(48 * time.Hour)
	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "apply_block", ExpectedVersion: 6,
		Summary:  "Bloqueo temporal aplicado en ámbito comunicaciones comerciales.",
		Scope:    privacy.ScopeCommercialComms,
		ReviewAt: &reviewAt,
	})
	if err != nil || detail.Version != 7 || len(detail.TreatmentBlocks) != 1 ||
		detail.TreatmentBlocks[0].State != "activo" || detail.TreatmentBlocks[0].Scope != privacy.ScopeCommercialComms {
		t.Fatalf("ExecutePrivacyRight(apply_block): detail=%+v err=%v", detail, err)
	}
	blockID := detail.TreatmentBlocks[0].ID

	hasBlock, err := privacySvc.HasActiveTreatmentBlock(ctx, owner, privacy.ScopeCommercialComms)
	if err != nil || !hasBlock {
		t.Fatalf("HasActiveTreatmentBlock(before lift) = %v, err=%v", hasBlock, err)
	}

	rectifiedName := "Titular Rectificado Fiscalizacion"
	rectifiedPhone := "987654321"
	rectExecDetail, err := runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: rectReq.ID,
		Action: "rectify_profile", ExpectedVersion: 1,
		Summary:     "Rectificación de nombre visible y teléfono según verificación.",
		DisplayName: &rectifiedName,
		Phone:       &rectifiedPhone,
	})
	if err != nil || rectExecDetail.Version != 2 || len(rectExecDetail.Executions) != 1 ||
		rectExecDetail.Executions[0].ExecutionType != "rectificacion_perfil" {
		t.Fatalf("ExecutePrivacyRight(rectify_profile): detail=%+v err=%v", rectExecDetail, err)
	}
	var gotProfileName string
	var gotProfilePhone *string
	if err = runtimePool.QueryRow(ctx, `SELECT nombre_visible, telefono_normalizado FROM public.perfil_usuario WHERE usuario_id=$1`, owner).Scan(&gotProfileName, &gotProfilePhone); err != nil {
		t.Fatal(err)
	}
	if gotProfileName != rectifiedName || gotProfilePhone == nil || *gotProfilePhone != rectifiedPhone {
		t.Fatalf("profile not rectified in DB: got name=%q phone=%v, want %q/%q", gotProfileName, gotProfilePhone, rectifiedName, rectifiedPhone)
	}

	oppExecDetail, err := runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: oppReq.ID,
		Action: "apply_opposition", ExpectedVersion: 1,
		Summary:      "Oposición aplicada en finalidad comunicaciones comerciales.",
		Purpose:      privacy.PurposeCommercialComms,
		PurposeState: privacy.PurposeStateOpposed,
	})
	if err != nil || oppExecDetail.Version != 2 || len(oppExecDetail.Executions) != 1 ||
		oppExecDetail.Executions[0].ExecutionType != "oposicion_finalidad" {
		t.Fatalf("ExecutePrivacyRight(apply_opposition): detail=%+v err=%v", oppExecDetail, err)
	}
	prefs, err := privacySvc.OwnPurposePreferences(ctx, owner)
	if err != nil {
		t.Fatalf("OwnPurposePreferences: %v", err)
	}
	var sawOpposed bool
	for _, pref := range prefs {
		if pref.Purpose == privacy.PurposeCommercialComms && pref.State == privacy.PurposeStateOpposed &&
			pref.RequestID != nil && *pref.RequestID == oppReq.ID {
			sawOpposed = true
		}
	}
	if !sawOpposed {
		t.Fatalf("expected %s opposed by %s in preferences: %+v", privacy.PurposeCommercialComms, oppReq.ID, prefs)
	}

	portExecDetail, err := runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: portReq.ID,
		Action: "issue_portability", ExpectedVersion: 1,
		Summary: "Emisión de paquete JSON interoperable de portabilidad.",
	})
	if err != nil || portExecDetail.Version != 2 || len(portExecDetail.Executions) != 1 ||
		portExecDetail.Executions[0].ExecutionType != "portabilidad_emitida" {
		t.Fatalf("ExecutePrivacyRight(issue_portability): detail=%+v err=%v", portExecDetail, err)
	}
	shaPtr := portExecDetail.Executions[0].ArtifactSHA256
	if shaPtr == nil || len(*shaPtr) != 64 {
		t.Fatalf("expected 64-hex sha256_artefacto, got %v", shaPtr)
	}
	if _, err = hex.DecodeString(*shaPtr); err != nil {
		t.Fatalf("sha256_artefacto is not valid hex (%q): %v", *shaPtr, err)
	}

	detail, err = runtimeService.ExecutePrivacyRight(ctx, adminglobal.ExecutePrivacyRightInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Action: "lift_block", ExpectedVersion: 7,
		BlockID: blockID,
		Summary: "Levantamiento formal de bloqueo tras acreditarse expiración de causal.",
	})
	if err != nil || detail.Version != 8 || len(detail.TreatmentBlocks) != 1 ||
		detail.TreatmentBlocks[0].State != "levantado" || detail.TreatmentBlocks[0].LiftReason == nil {
		t.Fatalf("ExecutePrivacyRight(lift_block): detail=%+v err=%v", detail, err)
	}
	hasBlock, err = privacySvc.HasActiveTreatmentBlock(ctx, owner, privacy.ScopeCommercialComms)
	if err != nil || hasBlock {
		t.Fatalf("expected treatment block lifted, got hasBlock=%v err=%v", hasBlock, err)
	}

	// 4. Resolución, despacho, outbox y reintento.
	const resolvedRectResp = "Rectificación ejecutada sobre perfil del titular conforme a respaldo."
	const resolvedPortResp = "Paquete interoperable de portabilidad generado y disponible."
	const resolvedOppResp = "Oposición registrada sobre comunicaciones comerciales."
	const deniedBlockResp = "Denegación fundada parcial: se aplicó bloqueo preventivo durante revisión y se levantó formalmente al expirar la causal invocada."

	for _, tc := range []struct {
		id       string
		decision string
		response string
		wantStat string
	}{
		{rectReq.ID, "resuelta", resolvedRectResp, "completada"},
		{portReq.ID, "resuelta", resolvedPortResp, "completada"},
		{oppReq.ID, "resuelta", resolvedOppResp, "completada"},
		{pubReceipt.ID, "denegada_fundada", deniedBlockResp, "rechazada"},
	} {
		resolved, resErr := runtimeService.ResolvePrivacyRequest(ctx, adminglobal.ResolvePrivacyRequestInput{
			ActorID: master, CorrelationID: corr, RequestID: tc.id,
			Decision: tc.decision, Response: tc.response,
		})
		if resErr != nil || resolved.Status != tc.wantStat || !resolved.CanRecordDispatch {
			t.Fatalf("ResolvePrivacyRequest(%s, %s): item=%+v err=%v", tc.id, tc.decision, resolved, resErr)
		}
	}

	const sensitiveEvidenceRef = "RESPALDO-CONFIDENCIAL-001"
	dispatchSentAt := time.Now().UTC()
	dispatched, err := runtimeService.RecordPrivacyDispatch(ctx, adminglobal.RecordPrivacyDispatchInput{
		ActorID: master, CorrelationID: corr, RequestID: pubReceipt.ID,
		Channel: "email", Destination: repEmail, SentAt: dispatchSentAt, EvidenceRef: sensitiveEvidenceRef,
	})
	if err != nil || dispatched.DispatchRecordedAt == nil || dispatched.CanRecordDispatch {
		t.Fatalf("RecordPrivacyDispatch: item=%+v err=%v", dispatched, err)
	}

	// Simular 6 fallos de envío en outbox hasta estado terminal.
	failingSender := &fiscalizacionNoticeSender{fail: true}
	for step := 0; step < 120; step++ {
		if _, err = runtimePool.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox SET disponible_en = now() - interval '1 second' WHERE estado = 'pending'`); err != nil {
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

	pubAfterFail, err := runtimeService.GetPrivacyRequest(ctx, master, corr, pubReceipt.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest after outbox failures: %v", err)
	}
	if pubAfterFail.NotificationSummary == nil || pubAfterFail.NotificationSummary.Terminal != 4 ||
		pubAfterFail.NotificationSummary.Pending != 0 || pubAfterFail.NotificationSummary.Delivered != 0 ||
		pubAfterFail.NotificationSummary.LastErrorCode == nil || *pubAfterFail.NotificationSummary.LastErrorCode != "attempt_limit_reached" {
		t.Fatalf("expected 4 terminal notices with attempt_limit_reached on public request, got %+v", pubAfterFail.NotificationSummary)
	}

	// RetryPrivacyNotifications y entrega exitosa con sender fake en memoria.
	for _, reqID := range []string{pubReceipt.ID, rectReq.ID, portReq.ID, oppReq.ID} {
		retried, retryErr := runtimeService.RetryPrivacyNotifications(ctx, master, corr, reqID)
		if retryErr != nil {
			t.Fatalf("RetryPrivacyNotifications(%s): %v", reqID, retryErr)
		}
		if retried.NotificationSummary == nil || retried.NotificationSummary.Terminal != 0 || retried.NotificationSummary.Pending == 0 {
			t.Fatalf("unexpected summary after retry(%s): %+v", reqID, retried.NotificationSummary)
		}
	}

	okSender := &fiscalizacionNoticeSender{fail: false}
	for step := 0; step < 30; step++ {
		processed, drainErr := privacySvc.DispatchPrivacyNoticeOnce(ctx, okSender)
		if drainErr != nil {
			t.Fatalf("DispatchPrivacyNoticeOnce(ok) step=%d: %v", step, drainErr)
		}
		if !processed {
			break
		}
	}

	pubAfterDelivery, err := runtimeService.GetPrivacyRequest(ctx, master, corr, pubReceipt.ID)
	if err != nil {
		t.Fatalf("GetPrivacyRequest after delivery: %v", err)
	}
	if pubAfterDelivery.NotificationSummary == nil || pubAfterDelivery.NotificationSummary.Delivered != 4 ||
		pubAfterDelivery.NotificationSummary.Pending != 0 || pubAfterDelivery.NotificationSummary.Terminal != 0 ||
		pubAfterDelivery.NotificationSummary.LastDeliveredAt == nil {
		t.Fatalf("expected 4 delivered notices on public request, got %+v", pubAfterDelivery.NotificationSummary)
	}

	var repDelivered int
	for _, sent := range okSender.sent {
		if sent.email == repEmail {
			repDelivered++
		}
		if strings.Contains(sent.subject, internalSecretNote) || strings.Contains(sent.body, internalSecretNote) ||
			strings.Contains(sent.subject, sensitiveEvidenceRef) || strings.Contains(sent.body, sensitiveEvidenceRef) {
			t.Fatalf("outbox notice leaked secret or evidence_ref: %+v", sent)
		}
	}
	if repDelivered != 4 || len(okSender.sent) != 10 {
		t.Fatalf("expected 4 notices to %s and 10 total delivered notices, got rep=%d total=%d", repEmail, repDelivered, len(okSender.sent))
	}

	// 5. Incidente de datos personales y retención legal (Fase 7).
	inc, err := runtimeService.CreatePrivacyIncident(ctx, adminglobal.CreatePrivacyIncidentInput{
		ActorID:               master,
		CorrelationID:         corr,
		Title:                 "Simulacro de acceso no autorizado en respaldo temporal",
		Severity:              "alta",
		DataCategories:        []string{"correo", "identificacion"},
		AffectedSystems:       []string{"s3_backup", "api"},
		EstimatedSubjects:     15,
		Summary:               "Detección de acceso temporal en entorno controlado durante simulacro de fiscalización.",
		RequiresAgencyNotice:  false,
		RequiresSubjectNotice: false,
		LeaderID:              master,
		DeputyID:              deputy,
		DetectedAt:            testStart.Add(-30 * time.Minute),
	})
	if err != nil || inc.Status != "abierto" || inc.Version != 1 {
		t.Fatalf("CreatePrivacyIncident: inc=%+v err=%v", inc, err)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "contain", ExpectedVersion: 1,
		Detail: "Credenciales rotadas y acceso temporal revocado inmediatamente.",
	})
	if err != nil || inc.Status != "contenido" || inc.Version != 2 {
		t.Fatalf("TransitionPrivacyIncident(contain): inc=%+v err=%v", inc, err)
	}

	reqAgency := true
	reqSubjects := true
	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "evaluate", ExpectedVersion: 2,
		Detail:                "Evaluación determina obligación de notificar a la Agencia y a titulares.",
		RequiresAgencyNotice:  &reqAgency,
		RequiresSubjectNotice: &reqSubjects,
	})
	if err != nil || inc.Status != "evaluado" || inc.Version != 3 || !inc.RequiresAgencyNotice {
		t.Fatalf("TransitionPrivacyIncident(evaluate): inc=%+v err=%v", inc, err)
	}

	if _, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "close", ExpectedVersion: 3,
		Detail: "Intento de cierre sin notificar a la Agencia.",
	}); !errors.Is(err, adminglobal.ErrConflict) {
		t.Fatalf("expected ErrConflict closing incident without notify_agency, got %v", err)
	}

	const agencyRef = "AGPD-F9-2026-001"
	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "notify_agency", ExpectedVersion: 3,
		Detail:          "Notificación formal remitida a la Agencia dentro del plazo.",
		AgencyReference: agencyRef,
	})
	if err != nil || inc.Status != "notificado" || inc.Version != 4 || inc.AgencyNotifiedAt == nil {
		t.Fatalf("TransitionPrivacyIncident(notify_agency): inc=%+v err=%v", inc, err)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "notify_subjects", ExpectedVersion: 4,
		Detail: "Aviso enviado a los titulares involucrados.",
	})
	if err != nil || inc.SubjectsNotifiedAt == nil || inc.Version != 5 {
		t.Fatalf("TransitionPrivacyIncident(notify_subjects): inc=%+v err=%v", inc, err)
	}

	inc, err = runtimeService.TransitionPrivacyIncident(ctx, adminglobal.TransitionPrivacyIncidentInput{
		ActorID: master, CorrelationID: corr, IncidentID: inc.ID,
		Action: "close", ExpectedVersion: 5,
		Detail: "Cierre formal tras completar remediación y notificaciones legales.",
	})
	if err != nil || inc.Status != "cerrado" || inc.ClosedAt == nil || inc.Version != 6 {
		t.Fatalf("TransitionPrivacyIncident(close): inc=%+v err=%v", inc, err)
	}

	holdReviewAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	hold, err := runtimeService.CreatePrivacyLegalHold(ctx, adminglobal.CreatePrivacyLegalHoldInput{
		ActorID:       master,
		CorrelationID: corr,
		AccountID:     owner,
		RequestID:     pubReceipt.ID,
		IncidentID:    inc.ID,
		Scope:         "incidente_seguridad",
		Authority:     "Agencia de Protección de Datos",
		LegalBasis:    "Conservación preventiva de antecedentes vinculados a fiscalización simulada.",
		ReviewAt:      &holdReviewAt,
	})
	if err != nil || hold.Status != "activa" || hold.AccountID == nil || *hold.AccountID != owner ||
		hold.RequestID == nil || *hold.RequestID != pubReceipt.ID || hold.IncidentID == nil || *hold.IncidentID != inc.ID {
		t.Fatalf("CreatePrivacyLegalHold: hold=%+v err=%v", hold, err)
	}

	releasedHold, err := runtimeService.ReleasePrivacyLegalHold(ctx, adminglobal.ReleasePrivacyLegalHoldInput{
		ActorID: master, CorrelationID: corr, HoldID: hold.ID,
		ReleaseReason: "Liberación formal tras cierre de expediente e incidente en simulacro.",
	})
	if err != nil || releasedHold.Status != "liberada" || releasedHold.ReleasedAt == nil ||
		releasedHold.ReleasedBy == nil || *releasedHold.ReleasedBy != master {
		t.Fatalf("ReleasePrivacyLegalHold: hold=%+v err=%v", releasedHold, err)
	}

	// 6. Reconstrucción por CorrelationID, higiene e inmutabilidad.
	auditPage, err := runtimeService.Audit(ctx, master, corr, adminglobal.AuditFilter{
		From:          testStart.Add(-time.Minute),
		Until:         time.Now().UTC().Add(time.Minute),
		CorrelationID: corr,
		PageSize:      100,
	}, false)
	if err != nil {
		t.Fatalf("Audit(CorrelationID): %v", err)
	}
	if auditPage.NextCursor != "" {
		t.Fatalf("expected all correlation audit events on single page, got NextCursor=%q", auditPage.NextCursor)
	}

	seenActions := map[string]bool{}
	for _, entry := range auditPage.Items {
		if entry.CorrelationID != corr {
			t.Fatalf("unexpected correlation_id in filtered audit: %+v", entry)
		}
		seenActions[entry.Action] = true
	}
	for _, wantAction := range []string{
		"admin.global.privacy_requests.verify_identity",
		"admin.global.privacy_requests.assign",
		"admin.global.privacy_requests.add_internal_note",
		"admin.global.privacy_requests.request_clarification",
		"admin.global.privacy_requests.extend_deadline",
		"admin.global.privacy_requests.execute.apply_block",
		"admin.global.privacy_requests.execute.lift_block",
		"admin.global.privacy_requests.execute.rectify_profile",
		"admin.global.privacy_requests.execute.apply_opposition",
		"admin.global.privacy_requests.execute.issue_portability",
		"admin.global.privacy_requests.resolve",
		"admin.global.privacy_requests.dispatch_receipt",
		"admin.global.privacy_requests.notifications.retry",
		"admin.global.privacy_incidents.create",
		"admin.global.privacy_incidents.transition",
		"admin.global.privacy_legal_holds.create",
		"admin.global.privacy_legal_holds.release",
	} {
		if !seenActions[wantAction] {
			t.Fatalf("missing expected lifecycle audit action %q for CorrelationID %s; seen=%v", wantAction, corr, seenActions)
		}
	}

	auditJSON, err := json.Marshal(auditPage)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		internalSecretNote,
		repEmail,
		ownerEmail,
		pubReceipt.TrackingCode,
		sensitiveEvidenceRef,
		agencyRef,
	} {
		if strings.Contains(string(auditJSON), forbidden) {
			t.Fatalf("audit JSON leaked forbidden value %q: %s", forbidden, string(auditJSON))
		}
	}

	// Higiene en vistas de titular (ListOwnRightsRequests) y consulta pública (LookupPublicRightsRequest).
	ownRequests, err := identityRepo.ListOwnRightsRequests(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnRightsRequests: %v", err)
	}
	if len(ownRequests) != 4 {
		t.Fatalf("expected 4 linked requests in ListOwnRightsRequests, got %d", len(ownRequests))
	}
	for _, req := range ownRequests {
		for _, ev := range req.Events {
			if ev.EventType == "nota_interna" || ev.EventType == "asignacion" {
				t.Fatalf("ListOwnRightsRequests leaked internal event %+v", ev)
			}
		}
	}
	ownJSON, err := json.Marshal(ownRequests)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		internalSecretNote,
		sensitiveEvidenceRef,
		pubReceipt.TrackingCode,
		repEmail,
		"actor_id",
		master,
		"nota_interna",
		"asignacion",
	} {
		if strings.Contains(string(ownJSON), forbidden) {
			t.Fatalf("ListOwnRightsRequests JSON leaked forbidden value %q: %s", forbidden, string(ownJSON))
		}
	}

	publicStatus, err := privacySvc.LookupPublicRightsRequest(ctx, privacy.PublicRightsLookupInput{
		Folio:        pubReceipt.Folio,
		TrackingCode: pubReceipt.TrackingCode,
	})
	if err != nil {
		t.Fatalf("LookupPublicRightsRequest: %v", err)
	}
	if publicStatus.State != "denegada_fundada" || publicStatus.Response == nil || *publicStatus.Response != deniedBlockResp {
		t.Fatalf("unexpected public lookup status: %+v", publicStatus)
	}
	for _, ev := range publicStatus.Events {
		if ev.EventType == "nota_interna" || ev.EventType == "asignacion" {
			t.Fatalf("LookupPublicRightsRequest leaked internal event %+v", ev)
		}
	}
	publicJSON, err := json.Marshal(publicStatus)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		internalSecretNote,
		sensitiveEvidenceRef,
		pubReceipt.TrackingCode,
		repEmail,
		"actor_id",
		master,
		"nota_interna",
		"asignacion",
	} {
		if strings.Contains(string(publicJSON), forbidden) {
			t.Fatalf("LookupPublicRightsRequest JSON leaked forbidden value %q: %s", forbidden, string(publicJSON))
		}
	}

	// Verificar SQLSTATE 42501 (permission denied) bajo espacigo_runtime en tablas inmutables.
	for _, stmt := range []struct {
		name string
		sql  string
		arg  string
	}{
		{"UPDATE solicitud_titular_evento", `UPDATE public.solicitud_titular_evento SET detalle = 'mutado' WHERE solicitud_id = $1`, pubReceipt.ID},
		{"DELETE solicitud_titular_evento", `DELETE FROM public.solicitud_titular_evento WHERE solicitud_id = $1`, pubReceipt.ID},
		{"UPDATE privacidad_ejecucion_derecho", `UPDATE public.privacidad_ejecucion_derecho SET resumen_operativo = 'mutado' WHERE solicitud_id = $1`, portReq.ID},
		{"DELETE privacidad_ejecucion_derecho", `DELETE FROM public.privacidad_ejecucion_derecho WHERE solicitud_id = $1`, portReq.ID},
		{"UPDATE privacidad_incidente_evento", `UPDATE public.privacidad_incidente_evento SET detalle = 'mutado' WHERE incidente_id = $1`, inc.ID},
		{"DELETE privacidad_incidente_evento", `DELETE FROM public.privacidad_incidente_evento WHERE incidente_id = $1`, inc.ID},
		{"UPDATE evento_auditoria_local", `UPDATE public.evento_auditoria_local SET motivo_codigo = 'mutado' WHERE correlacion_id = $1`, corr},
		{"DELETE evento_auditoria_local", `DELETE FROM public.evento_auditoria_local WHERE correlacion_id = $1`, corr},
	} {
		var pgErr *pgconn.PgError
		if _, err = runtimePool.Exec(ctx, stmt.sql, stmt.arg); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 on %s under espacigo_runtime, got err=%v", stmt.name, err)
		}
	}
}
