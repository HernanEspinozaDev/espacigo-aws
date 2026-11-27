package conversationpg

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/booking/expiry"
	contractspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/contracts"
	"github.com/HernanEspinozaDev/espaciGo/internal/contract"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const messageColumns = `id::text,reserva_id::text,autor_id::text,secuencia,cuerpo,creada_en`

func scanMessage(row pgx.Row) (conversation.Message, error) {
	var v conversation.Message
	err := row.Scan(&v.ID, &v.ReservationID, &v.AuthorID, &v.Sequence, &v.Body, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, conversation.ErrNotFound
	}
	return v, err
}

func (r *Repository) List(ctx context.Context, actor, reservationID string, before *int64, limit int) (conversation.Page, error) {
	var participant bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_ensayo_local WHERE id=$1 AND (anfitrion_id=$2 OR arrendatario_id=$2))`, reservationID, actor).Scan(&participant)
	if err != nil {
		return conversation.Page{}, err
	}
	if !participant {
		return conversation.Page{}, conversation.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messageColumns+` FROM public.mensaje_reserva_ensayo
WHERE reserva_id=$1 AND ($2::bigint IS NULL OR secuencia<$2)
ORDER BY secuencia DESC LIMIT $3`, reservationID, before, limit+1)
	if err != nil {
		return conversation.Page{}, err
	}
	defer rows.Close()
	descending := make([]conversation.Message, 0, limit+1)
	for rows.Next() {
		var item conversation.Message
		if err = rows.Scan(&item.ID, &item.ReservationID, &item.AuthorID, &item.Sequence, &item.Body, &item.CreatedAt); err != nil {
			return conversation.Page{}, err
		}
		descending = append(descending, item)
	}
	if err = rows.Err(); err != nil {
		return conversation.Page{}, err
	}
	hasOlder := len(descending) > limit
	if hasOlder {
		descending = descending[:limit]
	}
	items := make([]conversation.Message, len(descending))
	for i := range descending {
		items[len(descending)-1-i] = descending[i]
	}
	page := conversation.Page{Items: items}
	if hasOlder && len(items) > 0 {
		cursor := items[0].Sequence
		page.OlderCursor = &cursor
	}
	return page, nil
}

func (r *Repository) MarkRead(ctx context.Context, actor, reservationID string, throughSequence int64) (int64, error) {
	var participant bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_ensayo_local
WHERE id=$1 AND (anfitrion_id=$2 OR arrendatario_id=$2))`, reservationID, actor).Scan(&participant); err != nil {
		return 0, err
	}
	if !participant {
		return 0, conversation.ErrNotFound
	}
	var cursor int64
	err := r.pool.QueryRow(ctx, `INSERT INTO public.reserva_mensaje_lectura(reserva_id,participante_id,ultima_secuencia_leida,actualizada_en)
SELECT $1,$2,$3,clock_timestamp()
WHERE EXISTS(SELECT 1 FROM public.mensaje_reserva_ensayo WHERE reserva_id=$1 AND secuencia=$3)
ON CONFLICT (reserva_id,participante_id) DO UPDATE SET
ultima_secuencia_leida=GREATEST(reserva_mensaje_lectura.ultima_secuencia_leida,EXCLUDED.ultima_secuencia_leida),
actualizada_en=CASE WHEN EXCLUDED.ultima_secuencia_leida>reserva_mensaje_lectura.ultima_secuencia_leida THEN EXCLUDED.actualizada_en ELSE reserva_mensaje_lectura.actualizada_en END
RETURNING ultima_secuencia_leida`, reservationID, actor, throughSequence).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, conversation.ErrInvalid
	}
	return cursor, err
}

func (r *Repository) ListCommercial(ctx context.Context, actor, reservationID string, before *int64, limit int) (conversation.Page, error) {
	if _, _, _, err := commercialParty(ctx, r.pool, actor, reservationID, false); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return conversation.Page{}, conversation.ErrNotFound
		}
		return conversation.Page{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text,reserva_id::text,COALESCE(autor_id::text,''),secuencia,cuerpo,creada_en FROM public.mensaje_reserva_comercial WHERE reserva_id=$1 AND ($2::bigint IS NULL OR secuencia<$2) ORDER BY secuencia DESC LIMIT $3`, reservationID, before, limit+1)
	if err != nil {
		return conversation.Page{}, err
	}
	defer rows.Close()
	descending := make([]conversation.Message, 0, limit+1)
	for rows.Next() {
		var item conversation.Message
		if err = rows.Scan(&item.ID, &item.ReservationID, &item.AuthorID, &item.Sequence, &item.Body, &item.CreatedAt); err != nil {
			return conversation.Page{}, err
		}
		descending = append(descending, item)
	}
	if err = rows.Err(); err != nil {
		return conversation.Page{}, err
	}
	return pageFromDescending(descending, limit), nil
}

func pageFromDescending(descending []conversation.Message, limit int) conversation.Page {
	hasOlder := len(descending) > limit
	if hasOlder {
		descending = descending[:limit]
	}
	items := make([]conversation.Message, len(descending))
	for i := range descending {
		items[len(descending)-1-i] = descending[i]
	}
	page := conversation.Page{Items: items}
	if hasOlder && len(items) > 0 {
		cursor := items[0].Sequence
		page.OlderCursor = &cursor
	}
	return page
}

func (r *Repository) MarkCommercialRead(ctx context.Context, actor, reservationID string, throughSequence int64) (int64, error) {
	var cursor int64
	err := r.pool.QueryRow(ctx, `INSERT INTO public.reserva_comercial_mensaje_lectura(reserva_id,usuario_id,ultima_secuencia_leida,actualizada_en)
SELECT $1,$2,$3,clock_timestamp()
WHERE EXISTS(SELECT 1 FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id
 WHERE r.id=$1 AND r.anfitrion_id<>r.arrendatario_id AND
 ($2::uuid=r.arrendatario_id OR $2::uuid=r.anfitrion_id OR EXISTS(
   SELECT 1 FROM public.organizacion_miembro m WHERE e.modo_anfitrion='empresa'
    AND e.propietario_id=r.anfitrion_id AND m.organizacion_id=e.organizacion_id AND m.usuario_id=$2
    AND m.estado='activo' AND m.rol IN ('representante_legal','administrador','operador'))))
AND EXISTS(SELECT 1 FROM public.mensaje_reserva_comercial WHERE reserva_id=$1 AND secuencia=$3)
ON CONFLICT(reserva_id,usuario_id) DO UPDATE SET ultima_secuencia_leida=GREATEST(reserva_comercial_mensaje_lectura.ultima_secuencia_leida,EXCLUDED.ultima_secuencia_leida),actualizada_en=CASE WHEN EXCLUDED.ultima_secuencia_leida>reserva_comercial_mensaje_lectura.ultima_secuencia_leida THEN EXCLUDED.actualizada_en ELSE reserva_comercial_mensaje_lectura.actualizada_en END
RETURNING ultima_secuencia_leida`, reservationID, actor, throughSequence).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		var participant bool
		if readErr := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id
 WHERE r.id=$1 AND r.anfitrion_id<>r.arrendatario_id AND
 ($2::uuid=r.arrendatario_id OR $2::uuid=r.anfitrion_id OR EXISTS(
   SELECT 1 FROM public.organizacion_miembro m WHERE e.modo_anfitrion='empresa'
    AND e.propietario_id=r.anfitrion_id AND m.organizacion_id=e.organizacion_id AND m.usuario_id=$2
    AND m.estado='activo' AND m.rol IN ('representante_legal','administrador','operador'))))`, reservationID, actor).Scan(&participant); readErr != nil {
			return 0, readErr
		}
		if !participant {
			return 0, conversation.ErrNotFound
		}
		return 0, conversation.ErrInvalid
	}
	return cursor, err
}

func (r *Repository) SendCommercial(ctx context.Context, actor, reservationID, key, body string, fingerprint []byte, id string, now func() time.Time) (conversation.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return conversation.Message{}, err
	}
	defer tx.Rollback(ctx)
	host, renter, hostSide, err := commercialParty(ctx, tx, actor, reservationID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, conversation.ErrNotFound
	}
	if err != nil {
		return conversation.Message{}, err
	}
	if hostSide && actor != host {
		if err = lockCommercialHostMember(ctx, tx, actor, reservationID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return conversation.Message{}, conversation.ErrNotFound
			}
			return conversation.Message{}, err
		}
	}
	var item conversation.Message
	var priorHash []byte
	err = tx.QueryRow(ctx, `SELECT id::text,reserva_id::text,COALESCE(autor_id::text,''),secuencia,cuerpo,creada_en,huella_solicitud FROM public.mensaje_reserva_comercial WHERE reserva_id=$1 AND autor_id=$2 AND clave_idempotencia=$3`, reservationID, actor, key).Scan(&item.ID, &item.ReservationID, &item.AuthorID, &item.Sequence, &item.Body, &item.CreatedAt, &priorHash)
	if err == nil {
		if !equalHash(priorHash, fingerprint) {
			return conversation.Message{}, conversation.ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return conversation.Message{}, err
		}
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, err
	}
	createdAt := now().UTC()
	err = tx.QueryRow(ctx, `INSERT INTO public.mensaje_reserva_comercial(id,reserva_id,autor_id,clave_idempotencia,huella_solicitud,cuerpo,creada_en) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,reserva_id::text,COALESCE(autor_id::text,''),secuencia,cuerpo,creada_en`, id, reservationID, actor, key, fingerprint, body, createdAt).Scan(&item.ID, &item.ReservationID, &item.AuthorID, &item.Sequence, &item.Body, &item.CreatedAt)
	if err != nil {
		return conversation.Message{}, err
	}
	recipient, href, recipientLabel := host, "/anfitrion/reservas/"+reservationID, "Arrendatario"
	if hostSide {
		recipient, href, recipientLabel = renter, "/reservas/"+reservationID, "Anfitrión"
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.notificacion_usuario(id,usuario_id,tipo,titulo,mensaje,href,referencia_id,creada_en) VALUES(gen_random_uuid(),$1,'mensaje_reserva','Nuevo mensaje de ' || $2,'Tienes un nuevo mensaje sobre tu reserva.',$3,$4,$5)`, recipient, recipientLabel, href, reservationID, createdAt)
	if err != nil {
		return conversation.Message{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return conversation.Message{}, err
	}
	return item, nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func commercialParty(ctx context.Context, q rowQuerier, actor, reservationID string, lockReservation bool) (host, renter string, hostSide bool, err error) {
	query := `SELECT r.anfitrion_id::text,r.arrendatario_id::text,
 (r.anfitrion_id=$2::uuid OR EXISTS(SELECT 1 FROM public.organizacion_miembro m
   WHERE e.modo_anfitrion='empresa' AND e.propietario_id=r.anfitrion_id
    AND m.organizacion_id=e.organizacion_id AND m.usuario_id=$2::uuid AND m.estado='activo'
    AND m.rol IN ('representante_legal','administrador','operador')))
 FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id
 WHERE r.id=$1 AND r.anfitrion_id<>r.arrendatario_id
   AND (r.arrendatario_id=$2::uuid OR r.anfitrion_id=$2::uuid OR EXISTS(
     SELECT 1 FROM public.organizacion_miembro m WHERE e.modo_anfitrion='empresa'
      AND e.propietario_id=r.anfitrion_id AND m.organizacion_id=e.organizacion_id
      AND m.usuario_id=$2::uuid AND m.estado='activo'
      AND m.rol IN ('representante_legal','administrador','operador')))`
	if lockReservation {
		query += ` FOR UPDATE OF r`
	}
	err = q.QueryRow(ctx, query, reservationID, actor).Scan(&host, &renter, &hostSide)
	return host, renter, hostSide, err
}

func lockCommercialHostMember(ctx context.Context, tx pgx.Tx, actor, reservationID string) error {
	var memberID string
	return tx.QueryRow(ctx, `SELECT m.id::text FROM public.reserva_arriendo_comercial r
 JOIN public.espacio e ON e.id=r.espacio_id JOIN public.organizacion_miembro m ON m.organizacion_id=e.organizacion_id
 WHERE r.id=$1 AND e.modo_anfitrion='empresa' AND e.propietario_id=r.anfitrion_id
  AND m.usuario_id=$2 AND m.estado='activo' AND m.rol IN ('representante_legal','administrador','operador')
 FOR SHARE OF m`, reservationID, actor).Scan(&memberID)
}

func (r *Repository) Send(ctx context.Context, actor, reservationID, key, body string, fingerprint []byte, id string, now func() time.Time) (conversation.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return conversation.Message{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = contractspg.LockReservationAccounts(ctx, tx, reservationID); err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return conversation.Message{}, conversation.ErrNotFound
		}
		return conversation.Message{}, err
	}
	var state string
	err = tx.QueryRow(ctx, `SELECT estado FROM public.reserva_ensayo_local
WHERE id=$1 AND (anfitrion_id=$2 OR arrendatario_id=$2) FOR UPDATE`, reservationID, actor).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, conversation.ErrNotFound
	}
	if err != nil {
		return conversation.Message{}, err
	}
	var prior conversation.Message
	var priorHash []byte
	err = tx.QueryRow(ctx, `SELECT `+messageColumns+`,huella_solicitud FROM public.mensaje_reserva_ensayo
WHERE reserva_id=$1 AND autor_id=$2 AND clave_idempotencia=$3`, reservationID, actor, key).Scan(&prior.ID, &prior.ReservationID, &prior.AuthorID, &prior.Sequence, &prior.Body, &prior.CreatedAt, &priorHash)
	if err == nil {
		if !equalHash(priorHash, fingerprint) {
			return conversation.Message{}, conversation.ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return conversation.Message{}, err
		}
		return prior, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, err
	}
	// Sample the injected backend clock only after acquiring the reservation
	// lock and resolving idempotency. A send that waited on this row must be
	// judged at the time it actually obtains the lock, not when it began.
	createdAt := now().UTC()
	expired, err := expiry.LockedReservation(ctx, tx, reservationID, createdAt)
	if err != nil {
		return conversation.Message{}, err
	}
	if expired {
		if err = tx.Commit(ctx); err != nil {
			return conversation.Message{}, err
		}
		return conversation.Message{}, conversation.ErrConflict
	}
	contractExpired, err := contractspg.ExpireForReservation(ctx, tx, reservationID, createdAt)
	if err != nil {
		if errors.Is(err, contract.ErrConflict) {
			return conversation.Message{}, conversation.ErrConflict
		}
		if errors.Is(err, contract.ErrNotFound) {
			return conversation.Message{}, conversation.ErrNotFound
		}
		return conversation.Message{}, err
	}
	if contractExpired {
		if err = tx.Commit(ctx); err != nil {
			return conversation.Message{}, err
		}
		return conversation.Message{}, conversation.ErrConflict
	}
	if state != "pendiente_de_pago" && state != "pagada" && state != "aprobada_host" && state != "firma_parcial" && state != "lista_para_checkin" && state != "en_curso" && state != "en_disputa" {
		return conversation.Message{}, conversation.ErrConflict
	}
	item, err := scanMessage(tx.QueryRow(ctx, `INSERT INTO public.mensaje_reserva_ensayo
(id,reserva_id,autor_id,clave_idempotencia,huella_solicitud,cuerpo,creada_en)
	VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+messageColumns, id, reservationID, actor, key, fingerprint, body, createdAt))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return conversation.Message{}, conversation.ErrConflict
		}
		return conversation.Message{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return conversation.Message{}, err
	}
	return item, nil
}

func equalHash(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
