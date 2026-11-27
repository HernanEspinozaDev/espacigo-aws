package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	identitypg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/dbbootstrap"
	"github.com/HernanEspinozaDev/espaciGo/internal/privacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recordingNoticeSender struct {
	failErr error
	sent    []string
}

func (s *recordingNoticeSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	if s.failErr != nil {
		return s.failErr
	}
	s.sent = append(s.sent, email+"|"+subject+"|"+body)
	return nil
}

func TestPrivacyPublicIntakeLookupAndAuthenticatedOutboxPostgres(t *testing.T) {
	ctx, pool := newIdentityTestPool(t)
	repo := identitypg.NewIdentityRepository(pool)
	svc, err := privacy.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := svc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:              "rectificacion",
		RequesterType:          "representante",
		RequesterName:          "Estudio Representante",
		RequesterEmail:         "rep@ejemplo.cl",
		RepresentativeName:     "Abogada Titular",
		RepresentativeRelation: "mandatario",
		SubjectReference:       "Titular Referencia",
		Detail:                 "Solicitamos rectificación de datos de contacto del titular.",
	})
	if err != nil {
		t.Fatalf("SubmitPublicRightsRequest error = %v", err)
	}
	if receipt.State != "identidad_pendiente" || receipt.Channel != "publico" || !strings.HasPrefix(receipt.Folio, "PRIV-") {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	var usuarioID *string
	var hashLen int
	if err := pool.QueryRow(ctx, `SELECT usuario_id::text, octet_length(consulta_token_hash) FROM public.solicitud_titular WHERE id = $1`, receipt.ID).Scan(&usuarioID, &hashLen); err != nil {
		t.Fatal(err)
	}
	if usuarioID != nil || hashLen != 32 {
		t.Fatalf("unexpected row metadata usuario_id=%v hashLen=%d", usuarioID, hashLen)
	}

	var actorID *string
	var eventType, visibility string
	if err := pool.QueryRow(ctx, `SELECT actor_id::text, tipo_evento, visibilidad FROM public.solicitud_titular_evento WHERE solicitud_id = $1 AND secuencia = 1`, receipt.ID).Scan(&actorID, &eventType, &visibility); err != nil {
		t.Fatal(err)
	}
	if actorID != nil || eventType != "recepcion" || visibility != "titular" {
		t.Fatalf("unexpected initial event actor=%v type=%s visibility=%s", actorID, eventType, visibility)
	}

	// Internal note must never leak to public lookup.
	if _, err := pool.Exec(ctx, `INSERT INTO public.solicitud_titular_evento (id, solicitud_id, secuencia, tipo_evento, visibilidad, detalle) VALUES (gen_random_uuid(), $1, 2, 'nota_interna', 'interna', 'Nota interna confidencial')`, receipt.ID); err != nil {
		t.Fatal(err)
	}

	status, err := svc.LookupPublicRightsRequest(ctx, privacy.PublicRightsLookupInput{
		Folio:        receipt.Folio,
		TrackingCode: receipt.TrackingCode,
	})
	if err != nil {
		t.Fatalf("LookupPublicRightsRequest error = %v", err)
	}
	if status.Folio != receipt.Folio || len(status.Events) != 1 || status.Events[0].EventType != "recepcion" {
		t.Fatalf("unexpected lookup status: %+v", status)
	}

	if _, err := svc.LookupPublicRightsRequest(ctx, privacy.PublicRightsLookupInput{
		Folio:        receipt.Folio,
		TrackingCode: strings.Repeat("Z", len(receipt.TrackingCode)),
	}); !errors.Is(err, privacy.ErrNotFound) {
		t.Fatalf("wrong tracking code err = %v, want ErrNotFound", err)
	}

	// Authenticated portal intake also gets a public folio and enqueues acuse_recepcion.
	owner := testAccount("00000000-0000-4000-8000-000000000091", "titular.portal@ejemplo.cl")
	if err := repo.CreateWithTerms(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	authReq, err := repo.CreateRightsRequestWithDetail(ctx, owner.ID, "acceso", "web", nil)
	if err != nil {
		t.Fatalf("CreateRightsRequestWithDetail error = %v", err)
	}
	var authFolio, outboxKey, outboxEmail string
	if err := pool.QueryRow(ctx, `SELECT folio_publico FROM public.solicitud_titular WHERE id = $1`, authReq.ID).Scan(&authFolio); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(authFolio, "PRIV-") {
		t.Fatalf("expected PRIV- folio on authenticated request, got %q", authFolio)
	}
	if err := pool.QueryRow(ctx, `SELECT clave_idempotencia, destinatario_correo FROM public.privacidad_notificacion_outbox WHERE solicitud_id = $1 AND tipo_aviso = 'acuse_recepcion'`, authReq.ID).Scan(&outboxKey, &outboxEmail); err != nil {
		t.Fatal(err)
	}
	if outboxKey != "acuse:"+authReq.ID || outboxEmail != owner.Email {
		t.Fatalf("unexpected authenticated outbox row key=%q email=%q", outboxKey, outboxEmail)
	}
}

func TestPrivacyOutboxLifecycleAndRuntimeLeastPrivilegePostgres(t *testing.T) {
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

	receipt, err := svc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:      "acceso",
		RequesterType:  "titular",
		RequesterName:  "Persona Outbox",
		RequesterEmail: "outbox@ejemplo.cl",
		Detail:         "Solicito acceso completo a mis datos personales locales.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify runtime can run Claim/Finish and cannot DELETE or UPDATE immutable columns.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT sp_del`); err != nil {
		t.Fatal(err)
	}
	_, delErr := tx.Exec(ctx, `DELETE FROM public.privacidad_notificacion_outbox WHERE solicitud_id = $1`, receipt.ID)
	var pgErr *pgconn.PgError
	if !errors.As(delErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected 42501 on DELETE by espacigo_runtime, got %v", delErr)
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sp_del`); err != nil {
		t.Fatal(err)
	}
	_, updErr := tx.Exec(ctx, `UPDATE public.privacidad_notificacion_outbox SET asunto = 'tamper' WHERE solicitud_id = $1`, receipt.ID)
	pgErr = nil
	if !errors.As(updErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected 42501 on UPDATE asunto by espacigo_runtime, got %v", updErr)
	}
	_ = tx.Rollback(ctx)

	// Exercise retry backoff -> terminal -> new delivered row using repository methods.
	now := time.Now().UTC().Add(time.Minute)
	for attempt := 1; attempt <= 6; attempt++ {
		job, ok, err := repo.ClaimPrivacyNotice(ctx, now)
		if err != nil || !ok {
			t.Fatalf("attempt %d ClaimPrivacyNotice ok=%t err=%v", attempt, ok, err)
		}
		if job.Attempt != attempt {
			t.Fatalf("attempt %d got job.Attempt=%d", attempt, job.Attempt)
		}
		if err := repo.FinishPrivacyNotice(ctx, job, false, "mail_delivery_failed", now); err != nil {
			t.Fatalf("attempt %d FinishPrivacyNotice err=%v", attempt, err)
		}
		now = now.Add(time.Duration(attempt+1) * time.Minute)
	}
	var state, lastErr string
	if err := pool.QueryRow(ctx, `SELECT estado, ultimo_error_codigo FROM public.privacidad_notificacion_outbox WHERE solicitud_id = $1`, receipt.ID).Scan(&state, &lastErr); err != nil {
		t.Fatal(err)
	}
	if state != "terminal" || lastErr != "attempt_limit_reached" {
		t.Fatalf("expected terminal/attempt_limit_reached, got %s/%s", state, lastErr)
	}

	// Enqueue a second notice and deliver it via Service.DispatchPrivacyNoticeOnce.
	receipt2, err := svc.SubmitPublicRightsRequest(ctx, privacy.PublicRightsRequestInput{
		RightType:      "bloqueo",
		RequesterType:  "titular",
		RequesterName:  "Persona Entrega",
		RequesterEmail: "entrega@ejemplo.cl",
		Detail:         "Solicito bloqueo temporal inmediato de mis datos personales.",
	})
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingNoticeSender{}
	dispatched, err := svc.DispatchPrivacyNoticeOnce(ctx, sender)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DispatchPrivacyNoticeOnce err=%v", err)
	}
	if !dispatched || len(sender.sent) != 1 {
		t.Fatalf("expected 1 dispatched notice, dispatched=%t sent=%d", dispatched, len(sender.sent))
	}
	if err := pool.QueryRow(ctx, `SELECT estado FROM public.privacidad_notificacion_outbox WHERE solicitud_id = $1`, receipt2.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "delivered" {
		t.Fatalf("expected delivered state, got %s", state)
	}
}
