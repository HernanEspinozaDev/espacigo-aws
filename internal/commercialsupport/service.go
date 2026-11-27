package commercialsupport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid  = errors.New("invalid support request")
	ErrNotFound = errors.New("support case not found")
	ErrConflict = errors.New("support action is not available")
)

const maxEvidence = 5 << 20
const defenseWindow = 48 * time.Hour

type FileStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type Service struct {
	db    *pgxpool.Pool
	files FileStore
	now   func() time.Time
}

func New(db *pgxpool.Pool, files FileStore, now func() time.Time) *Service {
	return &Service{db: db, files: files, now: now}
}

type Case struct {
	ID              string     `json:"id"`
	ReservationID   string     `json:"reservation_id"`
	Category        string     `json:"category"`
	InitiatorRole   string     `json:"initiator_role"`
	RoutedTo        string     `json:"routed_to"`
	Status          string     `json:"status"`
	Description     string     `json:"description"`
	DefenseDeadline *time.Time `json:"defense_deadline,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	Actions         []string   `json:"actions"`
	Events          []Event    `json:"events"`
	Evidence        []Evidence `json:"evidence"`

	initiator string
}

type Event struct {
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Evidence struct {
	ID   string `json:"id"`
	MIME string `json:"mime"`
	Size int    `json:"size"`
}

type ReservationCases struct {
	Items          []Case `json:"items"`
	CanClaimDamage bool   `json:"can_claim_damage"`
}

type reservation struct {
	host, renter, status   string
	start, end             time.Time
	actingUser, actingRole string
}

func (r reservation) role(actor string) string {
	if r.host == r.renter {
		return ""
	}
	switch actor {
	case r.host:
		return "anfitrion"
	case r.renter:
		return "arrendatario"
	default:
		if actor == r.actingUser {
			return r.actingRole
		}
		return ""
	}
}

func (r reservation) mayClaimDamage(actor string, now time.Time) bool {
	return r.role(actor) == "anfitrion" && r.status == "pagada" && !now.Before(r.end)
}

func readReservation(ctx context.Context, tx pgx.Tx, id, actor string, admin bool) (reservation, error) {
	var r reservation
	if !rentalflow.ValidUUID(id) {
		return r, ErrInvalid
	}
	err := tx.QueryRow(ctx, `SELECT r.anfitrion_id::text,r.arrendatario_id::text,r.estado,r.inicio,r.termino,
		CASE WHEN r.anfitrion_id=$2::uuid THEN 'anfitrion'
		     WHEN r.arrendatario_id=$2::uuid THEN 'arrendatario'
		     WHEN e.modo_anfitrion='empresa' AND e.propietario_id=r.anfitrion_id AND EXISTS(
		       SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=e.organizacion_id
		        AND m.usuario_id=$2::uuid AND m.estado='activo'
		        AND m.rol IN ('representante_legal','administrador','operador'))
		     THEN 'anfitrion' ELSE '' END
		FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id
		WHERE r.id=$1 FOR SHARE OF r`, id, actor).
		Scan(&r.host, &r.renter, &r.status, &r.start, &r.end, &r.actingRole)
	r.actingUser = actor
	if err == nil && r.actingRole == "anfitrion" && actor != r.host {
		var memberID string
		err = tx.QueryRow(ctx, `SELECT m.id::text FROM public.reserva_arriendo_comercial r
			JOIN public.espacio e ON e.id=r.espacio_id JOIN public.organizacion_miembro m ON m.organizacion_id=e.organizacion_id
			WHERE r.id=$1 AND e.modo_anfitrion='empresa' AND e.propietario_id=r.anfitrion_id
			 AND m.usuario_id=$2 AND m.estado='activo'
			 AND m.rol IN ('representante_legal','administrador','operador') FOR SHARE OF m`, id, actor).Scan(&memberID)
	}
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !admin && r.role(actor) == "" {
		return r, ErrNotFound
	}
	return r, err
}

const caseColumns = `id::text,reserva_id::text,iniciador_id::text,rol_iniciador,categoria,enrutado_a,estado,descripcion,plazo_descargo,creada_en`

func scanCase(row interface{ Scan(...any) error }) (Case, error) {
	var c Case
	c.Actions = []string{}
	c.Events = []Event{}
	c.Evidence = []Evidence{}
	err := row.Scan(&c.ID, &c.ReservationID, &c.initiator, &c.InitiatorRole, &c.Category, &c.RoutedTo, &c.Status, &c.Description, &c.DefenseDeadline, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}

func readCase(ctx context.Context, tx pgx.Tx, id, reservationID string, admin, lock bool) (Case, error) {
	if !rentalflow.ValidUUID(id) {
		return Case{}, ErrInvalid
	}
	q := `SELECT ` + caseColumns + ` FROM public.caso_soporte_reserva_comercial WHERE id=$1 AND
		(($2::boolean AND enrutado_a='espacigo') OR (NOT $2::boolean AND reserva_id=NULLIF($3,'')::uuid))`
	if lock {
		q += " FOR NO KEY UPDATE"
	}
	return scanCase(tx.QueryRow(ctx, q, id, admin, reservationID))
}

func disposition(category string) string {
	switch category {
	case "problemas_acceso", "condiciones_diferentes", "incumplimiento_normas":
		return "contraparte"
	case "espacio_no_disponible", "problema_pago_reembolso", "mediacion_directa", "dano_inmueble":
		return "espacigo"
	default:
		return ""
	}
}

func (c Case) availableActions(r reservation, actor string, admin bool, now time.Time) []string {
	actions := []string{}
	if admin {
		switch c.Status {
		case "en_mediacion_espacigo":
			if c.DefenseDeadline == nil || !now.Before(*c.DefenseDeadline) || c.hasDefense() {
				actions = append(actions, "resolver")
			}
		case "resuelto":
			actions = append(actions, "cerrar")
		}
		return actions
	}
	switch c.Status {
	case "esperando_contraparte":
		actions = append(actions, "responder", "escalar", "adjuntar")
		if actor == c.initiator || c.InitiatorRole == "anfitrion" && r.role(actor) == "anfitrion" {
			actions = append(actions, "cerrar")
		}
	case "en_mediacion_espacigo":
		if c.DefenseDeadline == nil {
			actions = append(actions, "responder", "adjuntar")
		}
		if c.DefenseDeadline != nil && (r.role(actor) == "anfitrion" || now.Before(*c.DefenseDeadline)) {
			actions = append(actions, "adjuntar")
		}
		if c.DefenseDeadline != nil && r.role(actor) == "arrendatario" && now.Before(*c.DefenseDeadline) && !c.hasDefense() {
			actions = append(actions, "descargo")
		}
	}
	return actions
}

func (c Case) hasDefense() bool {
	for _, event := range c.Events {
		if event.Action == "descargo" {
			return true
		}
	}
	return false
}

func caseDetails(ctx context.Context, tx pgx.Tx, c *Case, r reservation, actor string, admin bool, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT accion,descripcion,creada_en FROM public.evento_caso_soporte_comercial WHERE caso_id=$1
		ORDER BY creada_en,id`, c.ID)
	if err != nil {
		return err
	}
	c.Events, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
	if err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT id::text,mime,tamano_bytes FROM public.evidencia_caso_soporte_comercial WHERE caso_id=$1 ORDER BY creada_en,id`, c.ID)
	if err != nil {
		return err
	}
	c.Evidence, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Evidence])
	c.Actions = c.availableActions(r, actor, admin, now)
	return err
}

func (s *Service) List(ctx context.Context, reservationID, actor string, admin bool) (ReservationCases, error) {
	var result ReservationCases
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var r reservation
	q := `SELECT ` + caseColumns + ` FROM public.caso_soporte_reserva_comercial WHERE reserva_id=$1 ORDER BY creada_en DESC,id`
	var args []any
	now := s.now().UTC()
	if admin {
		q = `SELECT ` + caseColumns + ` FROM public.caso_soporte_reserva_comercial WHERE enrutado_a='espacigo'
			ORDER BY (estado='en_mediacion_espacigo') DESC,creada_en DESC,id LIMIT 100`
	} else {
		r, err = readReservation(ctx, tx, reservationID, actor, false)
		if err != nil {
			return result, err
		}
		args = append(args, reservationID)
		result.CanClaimDamage = r.mayClaimDamage(actor, now)
	}
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return result, err
	}
	result.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Case, error) { return scanCase(row) })
	if err != nil {
		return result, err
	}
	for i := range result.Items {
		c := &result.Items[i]
		if err = caseDetails(ctx, tx, c, r, actor, admin, now); err != nil {
			return result, err
		}
		if c.Category == "dano_inmueble" {
			result.CanClaimDamage = false
		}
	}
	return result, tx.Commit(ctx)
}

func (s *Service) Open(ctx context.Context, reservationID, actor, category, description, key string) (Case, error) {
	category, description = strings.TrimSpace(category), strings.TrimSpace(description)
	route := disposition(category)
	if route == "" || !validText(description, true) || !validKey(key) {
		return Case{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Case{}, err
	}
	defer tx.Rollback(ctx)
	r, err := readReservation(ctx, tx, reservationID, actor, false)
	if err != nil {
		return Case{}, err
	}
	now := s.now().UTC()
	status := "esperando_contraparte"
	var deadline *time.Time
	if route == "espacigo" {
		status = "en_mediacion_espacigo"
	}
	if category == "dano_inmueble" {
		if !r.mayClaimDamage(actor, now) {
			return Case{}, ErrConflict
		}
		at := now.Add(defenseWindow)
		deadline = &at
	}
	c, err := scanCase(tx.QueryRow(ctx, `INSERT INTO public.caso_soporte_reserva_comercial
		(reserva_id,iniciador_id,rol_iniciador,categoria,enrutado_a,estado,descripcion,clave_idempotencia,plazo_descargo,creada_en,actualizada_en)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		ON CONFLICT (reserva_id,iniciador_id,clave_idempotencia) DO NOTHING RETURNING `+caseColumns,
		reservationID, actor, r.role(actor), category, route, status, description, key, deadline, now))
	if errors.Is(err, ErrNotFound) {
		c, err = scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM public.caso_soporte_reserva_comercial
			WHERE reserva_id=$1 AND iniciador_id=$2 AND clave_idempotencia=$3`, reservationID, actor, key))
		if err == nil && (c.Category != category || c.Description != description) {
			return Case{}, ErrConflict
		}
	} else if err == nil {
		if err = recordEvent(ctx, tx, c.ID, actor, "abrir", description, key, now); err == nil {
			err = notify(ctx, tx, c, r, actor, route == "espacigo", now)
		}
	}
	if err != nil {
		return Case{}, mapConflict(err)
	}
	if err = caseDetails(ctx, tx, &c, r, actor, false, now); err != nil {
		return Case{}, err
	}
	return c, tx.Commit(ctx)
}

func (s *Service) Act(ctx context.Context, reservationID, id, actor, action, description, key string, admin bool) (Case, error) {
	description = strings.TrimSpace(description)
	required := action == "responder" || action == "descargo" || action == "resolver"
	if !validKey(key) || !validText(description, required) || action == "adjuntar" {
		return Case{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Case{}, err
	}
	defer tx.Rollback(ctx)
	if !admin {
		if _, err = readReservation(ctx, tx, reservationID, actor, false); err != nil {
			return Case{}, err
		}
	}
	c, err := readCase(ctx, tx, id, reservationID, admin, true)
	if err != nil {
		return Case{}, err
	}
	r, err := readReservation(ctx, tx, c.ReservationID, actor, admin)
	if err != nil {
		return Case{}, err
	}
	now := s.now().UTC()
	if err = caseDetails(ctx, tx, &c, r, actor, admin, now); err != nil {
		return Case{}, err
	}
	var priorAction, priorDescription string
	err = tx.QueryRow(ctx, `SELECT accion,descripcion FROM public.evento_caso_soporte_comercial WHERE caso_id=$1 AND actor_id=$2 AND clave_idempotencia=$3`, id, actor, key).Scan(&priorAction, &priorDescription)
	if err == nil {
		if priorAction != action || priorDescription != description {
			return Case{}, ErrConflict
		}
		return c, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Case{}, err
	}
	if !contains(c.Actions, action) {
		return Case{}, ErrConflict
	}
	switch action {
	case "escalar":
		c.RoutedTo, c.Status = "espacigo", "en_mediacion_espacigo"
	case "resolver":
		c.Status = "resuelto"
	case "cerrar":
		c.Status = "cerrado"
	}
	_, err = tx.Exec(ctx, `UPDATE public.caso_soporte_reserva_comercial SET enrutado_a=$2,estado=$3,actualizada_en=$4 WHERE id=$1`, id, c.RoutedTo, c.Status, now)
	if err != nil {
		return Case{}, err
	}
	if err = recordEvent(ctx, tx, id, actor, action, description, key, now); err != nil {
		return Case{}, mapConflict(err)
	}
	if err = notify(ctx, tx, c, r, actor, admin || action == "escalar", now); err != nil {
		return Case{}, err
	}
	if err = caseDetails(ctx, tx, &c, r, actor, admin, now); err != nil {
		return Case{}, err
	}
	return c, tx.Commit(ctx)
}

func recordEvent(ctx context.Context, tx pgx.Tx, caseID, actor, action, description, key string, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.evento_caso_soporte_comercial(caso_id,actor_id,accion,descripcion,clave_idempotencia,creada_en) VALUES($1,$2,$3,$4,$5,$6)`,
		caseID, actor, action, description, key, now)
	return err
}

func notify(ctx context.Context, tx pgx.Tx, c Case, r reservation, actor string, both bool, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.notificacion_usuario (id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en)
		SELECT gen_random_uuid(),usuario,'caso_soporte_reserva','Actualización del caso de la reserva',
		'Abre la reserva para consultar la actualización.',ruta || $1::uuid::text,$2,$3
		FROM (VALUES ($4::uuid,'/anfitrion/reservas/'),($5::uuid,'/reservas/')) AS destinatario(usuario,ruta)
		WHERE $6::boolean OR ($7='anfitrion' AND usuario=$5::uuid) OR ($7='arrendatario' AND usuario=$4::uuid)`, c.ReservationID, c.ID, now, r.host, r.renter, both, r.role(actor))
	return err
}

func (s *Service) Attach(ctx context.Context, reservationID, id, actor string, data []byte) (Evidence, error) {
	var e Evidence
	if len(data) == 0 || len(data) > maxEvidence || s.files == nil {
		return e, ErrInvalid
	}
	e.MIME = http.DetectContentType(data)
	switch e.MIME {
	case "image/jpeg", "image/png", "application/pdf":
	default:
		return e, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return e, err
	}
	defer tx.Rollback(ctx)
	r, err := readReservation(ctx, tx, reservationID, actor, false)
	if err != nil {
		return e, err
	}
	c, err := readCase(ctx, tx, id, reservationID, false, true)
	if err != nil {
		return e, err
	}
	now := s.now().UTC()
	if !contains(c.availableActions(r, actor, false, now), "adjuntar") {
		return e, ErrConflict
	}
	sum := sha256.Sum256(data)
	err = tx.QueryRow(ctx, `SELECT id::text,mime,tamano_bytes FROM public.evidencia_caso_soporte_comercial WHERE caso_id=$1 AND autor_id=$2 AND sha256=$3`, id, actor, sum[:]).Scan(&e.ID, &e.MIME, &e.Size)
	if err == nil {
		return e, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return e, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.evidencia_caso_soporte_comercial WHERE caso_id=$1`, id).Scan(&count); err != nil {
		return e, err
	}
	if count >= 12 {
		return e, ErrConflict
	}
	e.ID = uuid.NewSHA1(uuid.NameSpaceOID, append([]byte("espacigo:commercial-support:"+id+":"+actor+":"), sum[:]...)).String()
	e.Size = len(data)
	_, err = tx.Exec(ctx, `INSERT INTO public.evidencia_caso_soporte_comercial(id,caso_id,autor_id,mime,tamano_bytes,sha256,creada_en) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		e.ID, id, actor, e.MIME, e.Size, sum[:], now)
	if err != nil {
		return e, mapConflict(err)
	}
	if err = notify(ctx, tx, c, r, actor, false, now); err != nil {
		return e, err
	}
	if err = s.files.Put(ctx, e.ID, data); err != nil {
		// A previous attempt can have stored the blob before its transaction
		// was lost. Reuse it only if the complete contents are identical.
		saved, readErr := s.files.Get(ctx, e.ID)
		if readErr != nil || !bytes.Equal(saved, data) {
			return e, err
		}
	}
	return e, tx.Commit(ctx)
}

func (s *Service) Evidence(ctx context.Context, reservationID, id, evidenceID, actor string, admin bool) (Evidence, []byte, error) {
	var e Evidence
	if !rentalflow.ValidUUID(evidenceID) || s.files == nil {
		return e, nil, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return e, nil, err
	}
	defer tx.Rollback(ctx)
	c, err := readCase(ctx, tx, id, reservationID, admin, false)
	if err != nil {
		return e, nil, err
	}
	if _, err = readReservation(ctx, tx, c.ReservationID, actor, admin); err != nil {
		return e, nil, err
	}
	var digest []byte
	err = tx.QueryRow(ctx, `SELECT id::text,mime,tamano_bytes,sha256 FROM public.evidencia_caso_soporte_comercial WHERE caso_id=$1 AND id=$2`, id, evidenceID).Scan(&e.ID, &e.MIME, &e.Size, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, nil, ErrNotFound
	}
	if err != nil {
		return e, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return e, nil, err
	}
	data, err := s.files.Get(ctx, e.ID)
	if err != nil {
		return e, nil, err
	}
	hash := sha256.Sum256(data)
	if len(data) != e.Size || !bytes.Equal(hash[:], digest) {
		return e, nil, errors.New("support evidence integrity failed")
	}
	return e, data, nil
}

func validText(text string, required bool) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0) && (!required || text != "") && utf8.RuneCountInString(text) <= 3000
}
func validKey(key string) bool {
	return key != "" && len(key) <= 128 && !strings.ContainsAny(key, " \t\r\n\x00")
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func mapConflict(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	return err
}

type Summary struct {
	UnreadMessages int `json:"unread_messages"`
	Reviews        int `json:"reviews"`
	OpenCases      int `json:"open_cases"`
}

func (s *Service) Summary(ctx context.Context, reservationID, actor string) (Summary, error) {
	var out Summary
	if !rentalflow.ValidUUID(reservationID) {
		return out, ErrInvalid
	}
	err := s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM public.mensaje_reserva_comercial m WHERE m.reserva_id=r.id
			AND m.autor_id IS DISTINCT FROM $2::uuid AND m.secuencia > COALESCE((SELECT l.ultima_secuencia_leida
			FROM public.reserva_comercial_mensaje_lectura l WHERE l.reserva_id=r.id AND l.usuario_id=$2),0)),
		(SELECT count(*) FROM public.resena_reserva_comercial v WHERE v.reserva_id=r.id AND v.estado <> 'oculta'),
		(SELECT count(*) FROM public.caso_soporte_reserva_comercial c WHERE c.reserva_id=r.id
			AND c.estado IN ('esperando_contraparte','en_mediacion_espacigo'))
		FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id WHERE r.id=$1
		AND ($2::uuid IN (r.anfitrion_id,r.arrendatario_id) OR EXISTS(
		 SELECT 1 FROM public.organizacion_miembro m WHERE e.modo_anfitrion='empresa'
		  AND e.propietario_id=r.anfitrion_id AND m.organizacion_id=e.organizacion_id
		  AND m.usuario_id=$2 AND m.estado='activo' AND m.rol IN ('representante_legal','administrador','operador')))
		AND r.anfitrion_id <> r.arrendatario_id`, reservationID, actor).Scan(&out.UnreadMessages, &out.Reviews, &out.OpenCases)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}
