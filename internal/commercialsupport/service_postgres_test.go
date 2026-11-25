package commercialsupport

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	supportHost     = "11111111-1111-4111-8111-111111111111"
	supportRenter   = "22222222-2222-4222-8222-222222222222"
	supportOther    = "33333333-3333-4333-8333-333333333333"
	supportAdmin    = "44444444-4444-4444-8444-444444444444"
	supportRentalID = "66666666-6666-4666-8666-666666666666"
)

type memoryEvidence struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *memoryEvidence) Put(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte(nil), value...)
	return nil
}
func (s *memoryEvidence) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), value...), nil
}
func (s *memoryEvidence) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func TestCommercialSupportPostgresWorkflow(t *testing.T) {
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
	dbName := fmt.Sprintf("commercial_support_%x", uint64(time.Now().UnixNano()))
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
CREATE TABLE public.reserva_arriendo_comercial(
 id uuid PRIMARY KEY, anfitrion_id uuid NOT NULL REFERENCES public.usuario(id),
 arrendatario_id uuid NOT NULL REFERENCES public.usuario(id), estado text NOT NULL,
 inicio timestamptz NOT NULL, termino timestamptz NOT NULL,
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
	migration := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations", "V000005__disputas_reclamos_comerciales.sql"))
	sql, err := os.ReadFile(migration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply commercial support schema: %v", err)
	}

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	ended := now.Add(-time.Hour)
	if _, err = pool.Exec(ctx, `INSERT INTO public.usuario(id) VALUES($1),($2),($3),($4)`, supportHost, supportRenter, supportOther, supportAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO public.reserva_arriendo_comercial(id,anfitrion_id,arrendatario_id,estado,inicio,termino) VALUES($1,$2,$3,'pagada',$4,$5)`, supportRentalID, supportHost, supportRenter, ended.Add(-4*time.Hour), ended); err != nil {
		t.Fatal(err)
	}
	files := &memoryEvidence{}
	service := New(pool, files, func() time.Time { return now })

	// Operational cases first go to the other participant and can later be
	// escalated to EspaciGo. Opening and escalation retries must not notify twice.
	caseItem, err := service.Open(ctx, supportRentalID, supportHost, "problemas_acceso", "No recibí instrucciones de acceso.", "open-operational-1")
	if err != nil {
		t.Fatalf("open operational case: %v", err)
	}
	if caseItem.InitiatorRole != "anfitrion" || caseItem.RoutedTo != "contraparte" || caseItem.Status != "esperando_contraparte" {
		t.Fatalf("unexpected operational route: %+v", caseItem)
	}
	if _, err = service.Open(ctx, supportRentalID, supportHost, "problemas_acceso", "No recibí instrucciones de acceso.", "open-operational-1"); err != nil {
		t.Fatalf("idempotent open: %v", err)
	}
	assertNotificationCount(t, ctx, pool, 1)
	if _, err = service.List(ctx, supportRentalID, supportOther, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("third party listing err=%v, want not found", err)
	}
	if _, err = service.List(ctx, supportRentalID, supportRenter, false); err != nil {
		t.Fatalf("counterparty should see case: %v", err)
	}
	if _, err = service.Act(ctx, supportRentalID, caseItem.ID, supportRenter, "responder", "Ya envié el código de acceso.", "reply-1", false); err != nil {
		t.Fatalf("counterparty response: %v", err)
	}
	caseItem, err = service.Act(ctx, supportRentalID, caseItem.ID, supportRenter, "escalar", "No logramos resolverlo.", "escalate-1", false)
	if err != nil {
		t.Fatalf("escalate case: %v", err)
	}
	if caseItem.RoutedTo != "espacigo" || caseItem.Status != "en_mediacion_espacigo" {
		t.Fatalf("case did not escalate: %+v", caseItem)
	}
	if _, err = service.Act(ctx, supportRentalID, caseItem.ID, supportRenter, "escalar", "No logramos resolverlo.", "escalate-1", false); err != nil {
		t.Fatalf("idempotent escalation: %v", err)
	}
	assertNotificationCount(t, ctx, pool, 4)
	adminQueue, err := service.List(ctx, "", supportAdmin, true)
	if err != nil || !containsCase(adminQueue.Items, caseItem.ID) {
		t.Fatalf("admin queue omitted escalated case: items=%+v err=%v", adminQueue.Items, err)
	}

	// Evidence metadata and bytes are readable only to the participants/admin;
	// duplicate uploads return the same evidence without another notification.
	png := []byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10, 0, 0, 0, 0}
	evidence, err := service.Attach(ctx, supportRentalID, caseItem.ID, supportRenter, png)
	if err != nil {
		t.Fatalf("attach evidence: %v", err)
	}
	again, err := service.Attach(ctx, supportRentalID, caseItem.ID, supportRenter, png)
	if err != nil || again.ID != evidence.ID {
		t.Fatalf("duplicate evidence id=%s err=%v; original=%s", again.ID, err, evidence.ID)
	}
	metadata, content, err := service.Evidence(ctx, supportRentalID, caseItem.ID, evidence.ID, supportHost, false)
	if err != nil || metadata.ID != evidence.ID || string(content) != string(png) {
		t.Fatalf("participant evidence read metadata=%+v content=%v err=%v", metadata, content, err)
	}
	if _, _, err = service.Evidence(ctx, supportRentalID, caseItem.ID, evidence.ID, supportOther, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("third-party evidence err=%v, want not found", err)
	}
	assertNotificationCount(t, ctx, pool, 5)

	// A damage claim is host-only, direct to EspaciGo after the reservation ends,
	// permits one renter defense during the deadline, and then admin resolution.
	if _, err = service.Open(ctx, supportRentalID, supportRenter, "dano_inmueble", "Intento de reclamo no autorizado.", "renter-damage-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("renter damage claim err=%v, want conflict", err)
	}
	damage, err := service.Open(ctx, supportRentalID, supportHost, "dano_inmueble", "Se detectó un daño en el inmueble.", "host-damage-1")
	if err != nil {
		t.Fatalf("host damage claim: %v", err)
	}
	if damage.RoutedTo != "espacigo" || damage.Status != "en_mediacion_espacigo" || damage.DefenseDeadline == nil {
		t.Fatalf("damage case route/deadline: %+v", damage)
	}
	if _, err = service.Act(ctx, supportRentalID, damage.ID, supportRenter, "descargo", "El daño ya estaba antes de ingresar.", "defense-1", false); err != nil {
		t.Fatalf("renter defense: %v", err)
	}
	resolved, err := service.Act(ctx, supportRentalID, damage.ID, supportAdmin, "resolver", "Se revisó evidencia de ambas partes.", "resolve-1", true)
	if err != nil || resolved.Status != "resuelto" {
		t.Fatalf("admin resolution status=%q err=%v", resolved.Status, err)
	}
	closed, err := service.Act(ctx, supportRentalID, damage.ID, supportAdmin, "cerrar", "Caso cerrado.", "close-1", true)
	if err != nil || closed.Status != "cerrado" {
		t.Fatalf("admin close status=%q err=%v", closed.Status, err)
	}
	if _, err = service.Act(ctx, supportRentalID, damage.ID, supportOther, "descargo", "No soy parte.", "third-defense-1", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("third-party action err=%v, want not found", err)
	}
}

func assertNotificationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.notificacion_usuario WHERE tipo='caso_soporte_reserva'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("notification count=%d, want %d", got, want)
	}
}

func containsCase(cases []Case, id string) bool {
	for _, item := range cases {
		if item.ID == id {
			return true
		}
	}
	return false
}
