package commercialcommunityhttp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	conversationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/conversation"
	reputationpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/reputation"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/HernanEspinozaDev/espaciGo/internal/reputation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	communityHost          = "11111111-1111-4111-8111-111111111111"
	communityRenter        = "22222222-2222-4222-8222-222222222222"
	communityOther         = "33333333-3333-4333-8333-333333333333"
	communitySpace         = "44444444-4444-4444-8444-444444444444"
	communityCompanySpace  = "44444444-4444-4444-8444-444444444445"
	communityOrganization  = "44444444-4444-4444-8444-444444444446"
	communityCompanyMember = "33333333-3333-4333-8333-333333333334"
	communityRevokedMember = "33333333-3333-4333-8333-333333333335"
	communityEnded         = "55555555-5555-4555-8555-555555555555"
	communityCompanyEnded  = "55555555-5555-4555-8555-555555555556"
	communityFuture        = "66666666-6666-4666-8666-666666666666"
	communityUnpaid        = "77777777-7777-4777-8777-777777777777"
)

func TestCommercialConversationAndReviewPostgres(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL required; creates and drops a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	dbName := fmt.Sprintf("commercial_community_%x", uint64(time.Now().UnixNano()))
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn, openErr := pgx.Connect(context.Background(), adminURL)
		if openErr != nil {
			t.Errorf("connect to drop disposable database: %v", openErr)
			return
		}
		defer conn.Close(context.Background())
		if _, dropErr := conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()); dropErr != nil {
			t.Errorf("drop disposable database: %v", dropErr)
		}
	})
	databaseURL, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	databaseURL.Path = "/" + dbName
	pool, err := pgxpool.New(ctx, databaseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='espacigo_runtime') THEN CREATE ROLE espacigo_runtime NOLOGIN; END IF; END $$;
CREATE TABLE public.usuario(id uuid PRIMARY KEY);
CREATE TABLE public.organizacion(id uuid PRIMARY KEY, usuario_id uuid NOT NULL, compliance_status text NOT NULL DEFAULT 'grace_active');
CREATE TABLE public.organizacion_miembro(id uuid PRIMARY KEY, organizacion_id uuid NOT NULL, usuario_id uuid NOT NULL, rol text NOT NULL, estado text NOT NULL);
CREATE TABLE public.espacio(id uuid PRIMARY KEY, estado text NOT NULL, propietario_id uuid, modo_anfitrion text NOT NULL DEFAULT 'personal', organizacion_id uuid);
-- The commercial review migration is incremental over the production baseline.
-- Keep the legacy review table needed by ListSpace's UNION in this isolated fixture.
CREATE TABLE public.resena_ensayo_local(
 id uuid PRIMARY KEY, reserva_id uuid NOT NULL, espacio_id uuid NOT NULL, autor_id uuid,
 destinatario_tipo text NOT NULL, destinatario_id uuid, puntuacion smallint NOT NULL,
 comentario text NOT NULL, estado text NOT NULL, creada_en timestamptz NOT NULL,
 retirar_en timestamptz NOT NULL, huella_solicitud bytea NOT NULL, clave_idempotencia text NOT NULL
);
CREATE TABLE public.reserva_arriendo_comercial(
 id uuid PRIMARY KEY, anfitrion_id uuid NOT NULL REFERENCES public.usuario(id),
 arrendatario_id uuid NOT NULL REFERENCES public.usuario(id), espacio_id uuid NOT NULL REFERENCES public.espacio(id),
 estado text NOT NULL, inicio timestamptz NOT NULL, termino timestamptz NOT NULL,
 CHECK (anfitrion_id <> arrendatario_id)
);
CREATE TABLE public.notificacion_usuario(
 id uuid PRIMARY KEY, usuario_id uuid NOT NULL REFERENCES public.usuario(id), tipo text NOT NULL,
 titulo text NOT NULL, mensaje text NOT NULL, href text NOT NULL, referencia_id uuid,
 creada_en timestamptz NOT NULL
);`); err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	for _, name := range []string{"V000003__mensajeria_comercial.sql", "V000004__resenas_comerciales.sql"} {
		migration := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "db", "migrations", name))
		sql, readErr := os.ReadFile(migration)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id) VALUES($1),($2),($3),($4),($5)`, communityHost, communityRenter, communityOther, communityCompanyMember, communityRevokedMember); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.organizacion(id,usuario_id,compliance_status) VALUES($1,$2,'disabled_missing_docs')`, communityOrganization, communityHost); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.organizacion_miembro(id,organizacion_id,usuario_id,rol,estado) VALUES
 ($1,$2,$3,'operador','activo'),($4,$2,$5,'administrador','retirado')`, "88888888-8888-4888-8888-888888888881", communityOrganization, communityCompanyMember, "88888888-8888-4888-8888-888888888882", communityRevokedMember); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio(id,estado,propietario_id,modo_anfitrion) VALUES($1,'activa',$2,'personal')`, communitySpace, communityHost); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.espacio(id,estado,propietario_id,modo_anfitrion,organizacion_id) VALUES($1,'activa',$2,'empresa',$3)`, communityCompanySpace, communityHost, communityOrganization); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.reserva_arriendo_comercial(id,anfitrion_id,arrendatario_id,espacio_id,estado,inicio,termino)
VALUES($1,$2,$3,$4,'pagada',$5,$6),($7,$2,$3,$4,'pagada',$8,$9),($10,$2,$3,$4,'pendiente_de_pago',$11,$12),($13,$2,$3,$14,'pagada',$5,$6)`, communityEnded, communityHost, communityRenter, communitySpace, now.Add(-3*time.Hour), now.Add(-time.Hour), communityFuture, now.Add(time.Hour), now.Add(2*time.Hour), communityUnpaid, now.Add(-3*time.Hour), now.Add(-time.Hour), communityCompanyEnded, communityCompanySpace); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return now }
	conversationService, err := conversation.NewService(conversationpg.New(pool), credentials.Generator{}, clock)
	if err != nil {
		t.Fatal(err)
	}
	reputationService, err := reputation.New(reputationpg.New(pool), credentials.Generator{}, clock)
	if err != nil {
		t.Fatal(err)
	}

	message, err := conversationService.SendCommercial(ctx, communityRenter, communityEnded, "send-message-1", "La puerta abre desde el costado.")
	if err != nil {
		t.Fatalf("send commercial message: %v", err)
	}
	replayed, err := conversationService.SendCommercial(ctx, communityRenter, communityEnded, "send-message-1", "La puerta abre desde el costado.")
	if err != nil || replayed.ID != message.ID {
		t.Fatalf("idempotent send replay=%+v err=%v first=%+v", replayed, err, message)
	}
	var notices int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.notificacion_usuario WHERE tipo='mensaje_reserva' AND usuario_id=$1`, communityHost).Scan(&notices); err != nil || notices != 1 {
		t.Fatalf("message notice count=%d err=%v; want one host notice", notices, err)
	}
	page, err := conversationService.ListCommercial(ctx, communityHost, communityEnded, nil, 30)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != message.ID || page.Items[0].AuthorID != communityRenter {
		t.Fatalf("host message list page=%+v err=%v", page, err)
	}
	if _, err = conversationService.ListCommercial(ctx, communityOther, communityEnded, nil, 30); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("third-party message list err=%v", err)
	}
	cursor, err := conversationService.MarkCommercialRead(ctx, communityHost, communityEnded, message.Sequence)
	if err != nil || cursor != message.Sequence {
		t.Fatalf("mark read cursor=%d err=%v", cursor, err)
	}
	if _, err = conversationService.MarkCommercialRead(ctx, communityOther, communityEnded, message.Sequence); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("third-party read cursor err=%v", err)
	}
	if _, err = conversationService.MarkCommercialRead(ctx, communityHost, communityEnded, message.Sequence+1); !errors.Is(err, conversation.ErrInvalid) {
		t.Fatalf("unknown read sequence err=%v", err)
	}

	companyMessage, err := conversationService.SendCommercial(ctx, communityCompanyMember, communityCompanyEnded, "company-message-1", "Coordino la entrega en nombre de la empresa.")
	if err != nil || companyMessage.AuthorID != communityCompanyMember {
		t.Fatalf("company message=%+v err=%v; actor must remain the author", companyMessage, err)
	}
	var companyNotice int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.notificacion_usuario WHERE tipo='mensaje_reserva' AND usuario_id=$1 AND referencia_id=$2`, communityRenter, communityCompanyEnded).Scan(&companyNotice); err != nil || companyNotice != 1 {
		t.Fatalf("company sender notice count=%d err=%v; expected renter recipient", companyNotice, err)
	}
	companyPage, err := conversationService.ListCommercial(ctx, communityCompanyMember, communityCompanyEnded, nil, 30)
	if err != nil || len(companyPage.Items) != 1 || companyPage.Items[0].AuthorID != communityCompanyMember {
		t.Fatalf("company member message page=%+v err=%v", companyPage, err)
	}
	companyCursor, err := conversationService.MarkCommercialRead(ctx, communityCompanyMember, communityCompanyEnded, companyMessage.Sequence)
	if err != nil || companyCursor != companyMessage.Sequence {
		t.Fatalf("company member read cursor=%d err=%v", companyCursor, err)
	}
	if _, err = conversationService.ListCommercial(ctx, communityRevokedMember, communityCompanyEnded, nil, 30); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("retired company member message list err=%v", err)
	}
	if _, err = conversationService.SendCommercial(ctx, communityRevokedMember, communityCompanyEnded, "revoked-company-message", "No debo acceder."); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("retired company member send err=%v", err)
	}

	futureGate, err := reputationService.ListCommercial(ctx, communityRenter, communityFuture)
	if err != nil || futureGate.CanReview || futureGate.ReviewUnavailableReason != "reservation_not_ended" {
		t.Fatalf("future review gate=%+v err=%v", futureGate, err)
	}
	unpaidGate, err := reputationService.ListCommercial(ctx, communityRenter, communityUnpaid)
	if err != nil || unpaidGate.CanReview || unpaidGate.ReviewUnavailableReason != "reservation_not_paid" {
		t.Fatalf("unpaid review gate=%+v err=%v", unpaidGate, err)
	}
	if _, err = reputationService.CreateCommercial(ctx, communityRenter, communityFuture, "future-review-1", reputation.ReviewInput{Rating: 5}); !errors.Is(err, reputation.ErrConflict) {
		t.Fatalf("future review creation err=%v", err)
	}
	renterReview, err := reputationService.CreateCommercial(ctx, communityRenter, communityEnded, "renter-review-1", reputation.ReviewInput{Rating: 5, Comment: "Muy buen espacio"})
	if err != nil || renterReview.TargetType != "espacio" {
		t.Fatalf("renter review=%+v err=%v", renterReview, err)
	}
	replayReview, err := reputationService.CreateCommercial(ctx, communityRenter, communityEnded, "renter-review-1", reputation.ReviewInput{Rating: 5, Comment: "Muy buen espacio"})
	if err != nil || !replayReview.Reused || replayReview.ID != renterReview.ID {
		t.Fatalf("review replay=%+v err=%v", replayReview, err)
	}
	gate, err := reputationService.ListCommercial(ctx, communityRenter, communityEnded)
	if err != nil || gate.CanReview || gate.ReviewUnavailableReason != "already_reviewed" {
		t.Fatalf("completed review gate=%+v err=%v", gate, err)
	}
	hostReview, err := reputationService.CreateCommercial(ctx, communityHost, communityEnded, "host-review-1", reputation.ReviewInput{Rating: 4, Comment: "Arrendatario puntual"})
	if err != nil || hostReview.TargetType != "arrendatario" {
		t.Fatalf("host review=%+v err=%v", hostReview, err)
	}
	publicReviews, err := reputationService.ListSpace(ctx, communitySpace)
	if err != nil || publicReviews.Count != 1 || publicReviews.Average != 5 || len(publicReviews.Items) != 1 {
		t.Fatalf("space reviews=%+v err=%v", publicReviews, err)
	}
	if _, err = reputationService.ListCommercial(ctx, communityOther, communityEnded); !errors.Is(err, reputation.ErrNotFound) {
		t.Fatalf("third-party review list err=%v", err)
	}
}
