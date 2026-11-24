package adminglobal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/booking"
	"github.com/HernanEspinozaDev/espaciGo/internal/gallery"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid                = errors.New("adminglobal: invalid input")
	ErrSensitiveAccessInvalid = errors.New("adminglobal: invalid sensitive access input")
	ErrAssistedViewInvalid    = errors.New("adminglobal: invalid assisted view input")
	ErrForbidden              = errors.New("adminglobal: forbidden")
	ErrNotFound               = errors.New("adminglobal: not found")
)

const collectionID = "00000000-0000-0000-0000-000000000000"

var canonicalUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Service struct {
	pool               *pgxpool.Pool
	now                func() time.Time
	ids                credentials.Generator
	publicationRetirer interface {
		DeleteAsAdministrator(context.Context, string, string, string, string, time.Time, string) (spaces.Deletion, error)
	}
	weeklyHoursManager interface {
		WeeklyHours(context.Context, string, string) (booking.WeeklyHours, error)
		WeeklyHoursAsAdministrator(context.Context, string, string, string, string) (booking.WeeklyHours, error)
		SaveWeeklyHoursAsAdministrator(context.Context, string, string, string, booking.WeeklyHours, time.Time, string, string) (booking.WeeklyHours, error)
	}
	publicationGallery interface {
		ListAsAdministrator(context.Context, string, string, string, string) ([]gallery.Photo, error)
		SetPublicInCatalogAsAdministrator(context.Context, string, string, string, string, bool, time.Time, string, string) (gallery.Photo, error)
		SelectCoverAsAdministrator(context.Context, string, string, string, string, time.Time, string, string) (gallery.Cover, error)
		RemoveAsAdministrator(context.Context, string, string, string, string, time.Time, string, string) (gallery.Removal, error)
	}
	sensitiveAccessStore SensitiveAccessStore
	assistedViewStore    AssistedViewStore
	specializedOpsStore  SpecializedOpsStore
}

// SensitiveAccessStore persists audited sensitive-access grants (used by tests or custom adapters).
type SensitiveAccessStore interface {
	GrantSensitiveAccess(context.Context, SensitiveAccessRecord) (SensitiveAccessGrant, error)
}

func (s *Service) SetSensitiveAccessStore(store SensitiveAccessStore) {
	s.sensitiveAccessStore = store
}

func (s *Service) SetPublicationGallery(manager interface {
	ListAsAdministrator(context.Context, string, string, string, string) ([]gallery.Photo, error)
	SetPublicInCatalogAsAdministrator(context.Context, string, string, string, string, bool, time.Time, string, string) (gallery.Photo, error)
	SelectCoverAsAdministrator(context.Context, string, string, string, string, time.Time, string, string) (gallery.Cover, error)
	RemoveAsAdministrator(context.Context, string, string, string, string, time.Time, string, string) (gallery.Removal, error)
}) {
	s.publicationGallery = manager
}

func New(pool *pgxpool.Pool, now func() time.Time) (*Service, error) {
	if pool == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Service{pool: pool, now: now, ids: credentials.Generator{}}, nil
}

func (s *Service) SetPublicationRetirer(retirer interface {
	DeleteAsAdministrator(context.Context, string, string, string, string, time.Time, string) (spaces.Deletion, error)
}) {
	s.publicationRetirer = retirer
}

func (s *Service) SetWeeklyHoursManager(manager interface {
	WeeklyHours(context.Context, string, string) (booking.WeeklyHours, error)
	WeeklyHoursAsAdministrator(context.Context, string, string, string, string) (booking.WeeklyHours, error)
	SaveWeeklyHoursAsAdministrator(context.Context, string, string, string, booking.WeeklyHours, time.Time, string, string) (booking.WeeklyHours, error)
}) {
	s.weeklyHoursManager = manager
}

type AccountFilter struct {
	AccountID             string
	EmailPrefix           string
	NamePrefix            string
	Role                  identity.Role
	State                 identity.AccountState
	AdministrativeBlocked *bool
	CreatedFrom           *time.Time
	CreatedUntil          *time.Time
	PageSize              int
	Cursor                string
}

type AccountSummary struct {
	AccountID             string     `json:"account_id"`
	Email                 string     `json:"email"`
	DisplayName           *string    `json:"display_name"`
	Phone                 *string    `json:"phone"`
	ProfileUpdatedAt      *time.Time `json:"profile_updated_at"`
	State                 string     `json:"state"`
	CreatedAt             time.Time  `json:"created_at"`
	AdministrativeBlocked bool       `json:"administrative_blocked"`
	Roles                 []string   `json:"roles"`
	PublicationCount      int64      `json:"publication_count"`
	ReservationCount      int64      `json:"reservation_count"`
	IncidentCount         int64      `json:"incident_count"`
}

type AccountPage struct {
	Items         []AccountSummary `json:"items"`
	NextCursor    string           `json:"next_cursor"`
	MatchingTotal int64            `json:"matching_total"`
	GeneratedAt   time.Time        `json:"generated_at"`
}

type AccountOverview struct {
	TotalAccounts         int64            `json:"total_accounts"`
	ConnectedAccounts     int64            `json:"connected_accounts"`
	AdministrativeBlocked int64            `json:"administrative_blocked"`
	ByState               map[string]int64 `json:"by_state"`
	ByRole                map[string]int64 `json:"by_role"`
	GeneratedAt           time.Time        `json:"generated_at"`
	Definition            string           `json:"definition"`
}

type GlobalAdminMe struct {
	AccountID    string                `json:"account_id"`
	Roles        []identity.Role       `json:"roles"`
	Capabilities []identity.Capability `json:"capabilities"`
	GeneratedAt  time.Time             `json:"generated_at"`
}

type accountCursor struct {
	Version       int       `json:"v"`
	FilterHash    string    `json:"f"`
	SnapshotUntil time.Time `json:"s"`
	CreatedAt     time.Time `json:"c"`
	AccountID     string    `json:"i"`
}

type sqlFilter struct {
	where []string
	args  []any
}

func (q *sqlFilter) add(clause string, value any) string {
	q.args = append(q.args, value)
	return fmt.Sprintf(clause, len(q.args))
}

func (s *Service) Me(ctx context.Context, principal identity.Principal) (GlobalAdminMe, error) {
	if principal.AccountID == "" || !identity.HasCapability(principal.Roles, identity.CapabilityAccountsList) || principal.RestrictedMode {
		return GlobalAdminMe{}, ErrForbidden
	}
	return GlobalAdminMe{
		AccountID:    principal.AccountID,
		Roles:        append([]identity.Role(nil), principal.Roles...),
		Capabilities: append([]identity.Capability(nil), principal.Capabilities...),
		GeneratedAt:  s.now().UTC(),
	}, nil
}

func (s *Service) Overview(ctx context.Context, actor, correlation string) (AccountOverview, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) {
		return AccountOverview{}, ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite})
	if err != nil {
		return AccountOverview{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = requireGlobalActor(ctx, tx, actor); err != nil {
		return AccountOverview{}, err
	}
	out := AccountOverview{ByState: map[string]int64{}, ByRole: map[string]int64{}, GeneratedAt: s.now().UTC(), Definition: "Cuentas registradas y cuentas con al menos una sesión vigente sin revocar. Los conteos por rol pueden solaparse."}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.usuario`).Scan(&out.TotalAccounts); err != nil {
		return AccountOverview{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.usuario u
		WHERE u.estado='activo' AND (u.bloqueado_hasta IS NULL OR u.bloqueado_hasta <= $1)
		  AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id)
		  AND EXISTS(SELECT 1 FROM public.sesion s WHERE s.usuario_id=u.id
			AND s.revocada_en IS NULL AND s.expira_en > $1)`, out.GeneratedAt).Scan(&out.ConnectedAccounts); err != nil {
		return AccountOverview{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.bloqueo_cuenta_administrativo_local`).Scan(&out.AdministrativeBlocked); err != nil {
		return AccountOverview{}, err
	}
	rows, err := tx.Query(ctx, `SELECT estado,count(*) FROM public.usuario GROUP BY estado ORDER BY estado`)
	if err != nil {
		return AccountOverview{}, err
	}
	for rows.Next() {
		var state string
		var count int64
		if err = rows.Scan(&state, &count); err != nil {
			rows.Close()
			return AccountOverview{}, err
		}
		out.ByState[state] = count
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return AccountOverview{}, err
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT role,count(*) FROM (
		SELECT rol AS role,usuario_id FROM public.rol_usuario
		UNION ALL SELECT 'administrador_global'::text,usuario_id FROM public.administrador_global
	) roles GROUP BY role ORDER BY role`)
	if err != nil {
		return AccountOverview{}, err
	}
	for rows.Next() {
		var role string
		var count int64
		if err = rows.Scan(&role, &count); err != nil {
			rows.Close()
			return AccountOverview{}, err
		}
		out.ByRole[role] = count
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return AccountOverview{}, err
	}
	rows.Close()
	if err = s.audit(ctx, tx, actor, collectionID, "admin.global.accounts.overview", "exito", "consulta_administrativa", correlation, out.GeneratedAt); err != nil {
		return AccountOverview{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountOverview{}, err
	}
	return out, nil
}

func (s *Service) ListAccounts(ctx context.Context, actor, correlation string, filter AccountFilter) (AccountPage, error) {
	return s.queryAccounts(ctx, actor, correlation, filter, "admin.global.accounts.list")
}

func (s *Service) queryAccounts(ctx context.Context, actor, correlation string, filter AccountFilter, action string) (AccountPage, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) {
		return AccountPage{}, ErrInvalid
	}
	if filter.PageSize == 0 {
		filter.PageSize = 25
	}
	if err := filter.validate(); err != nil {
		return AccountPage{}, err
	}
	filter.EmailPrefix = identity.NormalizeEmail(filter.EmailPrefix)
	filter.NamePrefix = strings.ToLower(strings.TrimSpace(filter.NamePrefix))
	filter.CreatedFrom = utcPointer(filter.CreatedFrom)
	filter.CreatedUntil = utcPointer(filter.CreatedUntil)
	filterHash, err := filterFingerprint(filter)
	if err != nil {
		return AccountPage{}, err
	}
	now := s.now().UTC()
	cursor := accountCursor{Version: 1, FilterHash: filterHash, SnapshotUntil: now}
	if filter.Cursor != "" {
		cursor, err = decodeCursor(filter.Cursor)
		if err != nil || cursor.FilterHash != filterHash || cursor.SnapshotUntil.IsZero() {
			return AccountPage{}, ErrInvalid
		}
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite})
	if err != nil {
		return AccountPage{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = requireGlobalActor(ctx, tx, actor); err != nil {
		return AccountPage{}, err
	}
	pageFilter, err := buildSQLFilter(filter, cursor.SnapshotUntil, &cursor)
	if err != nil {
		return AccountPage{}, err
	}
	countFilter, err := buildSQLFilter(filter, cursor.SnapshotUntil, nil)
	if err != nil {
		return AccountPage{}, err
	}
	countQuery := `SELECT count(*) FROM public.usuario u LEFT JOIN public.perfil_usuario p ON p.usuario_id=u.id WHERE ` + strings.Join(countFilter.where, " AND ")
	out := AccountPage{Items: []AccountSummary{}, GeneratedAt: now}
	if err = tx.QueryRow(ctx, countQuery, countFilter.args...).Scan(&out.MatchingTotal); err != nil {
		return AccountPage{}, err
	}
	pageFilter.args = append(pageFilter.args, filter.PageSize+1)
	limitArg := len(pageFilter.args)
	query := accountPageQuery(strings.Join(pageFilter.where, " AND "), limitArg)
	rows, err := tx.Query(ctx, query, pageFilter.args...)
	if err != nil {
		return AccountPage{}, err
	}
	for rows.Next() {
		var item AccountSummary
		if err = rows.Scan(&item.AccountID, &item.Email, &item.DisplayName, &item.Phone, &item.ProfileUpdatedAt, &item.State, &item.CreatedAt, &item.AdministrativeBlocked, &item.Roles, &item.PublicationCount, &item.ReservationCount, &item.IncidentCount); err != nil {
			rows.Close()
			return AccountPage{}, err
		}
		if item.Roles == nil {
			item.Roles = []string{}
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return AccountPage{}, err
	}
	rows.Close()
	if len(out.Items) > filter.PageSize {
		last := out.Items[filter.PageSize-1]
		out.Items = out.Items[:filter.PageSize]
		out.NextCursor, err = encodeCursor(accountCursor{Version: 1, FilterHash: filterHash, SnapshotUntil: cursor.SnapshotUntil, CreatedAt: last.CreatedAt.UTC(), AccountID: last.AccountID})
		if err != nil {
			return AccountPage{}, err
		}
	}
	result, reason, resource := "exito", "consulta_administrativa", collectionID
	individualRead := action == accountDetailRead || action == accountEmailReveal
	if individualRead {
		resource = filter.AccountID
		if len(out.Items) == 0 {
			result, reason = "rechazo", "cuenta_no_encontrada"
		} else if action == accountEmailReveal && out.Items[0].State == string(identity.AccountDeidentified) {
			result, reason = "rechazo", "cuenta_desidentificada"
		}
	}
	if individualRead {
		err = s.auditWithTarget(ctx, tx, actor, resource, action, result, reason, correlation, now, filter.AccountID)
	} else {
		err = s.audit(ctx, tx, actor, resource, action, result, reason, correlation, now)
	}
	if err != nil {
		return AccountPage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountPage{}, err
	}
	return out, nil
}

const (
	accountDetailRead  = "admin.global.accounts.read"
	accountEmailReveal = "admin.global.accounts.email.reveal"
)

func (s *Service) LookupAccount(ctx context.Context, actor, target, correlation string) (AccountSummary, error) {
	return s.lookupAccount(ctx, actor, target, correlation, accountDetailRead)
}

func (s *Service) RevealAccountEmail(ctx context.Context, actor, target, correlation string) (AccountSummary, error) {
	return s.lookupAccount(ctx, actor, target, correlation, accountEmailReveal)
}

func (s *Service) lookupAccount(ctx context.Context, actor, target, correlation, action string) (AccountSummary, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(target) || !canonicalUUID.MatchString(correlation) {
		return AccountSummary{}, ErrInvalid
	}
	page, err := s.queryAccounts(ctx, actor, correlation, AccountFilter{AccountID: target, PageSize: 1}, action)
	if err != nil {
		return AccountSummary{}, err
	}
	if len(page.Items) == 0 {
		return AccountSummary{}, ErrNotFound
	}
	item := page.Items[0]
	if action == accountDetailRead || item.State == string(identity.AccountDeidentified) {
		item.Email = identity.MaskEmail(item.Email)
	}
	return item, nil
}

func (f AccountFilter) validate() error {
	if f.PageSize < 1 || f.PageSize > 50 || len(f.Cursor) > 2048 || len(f.EmailPrefix) > 120 || len(f.NamePrefix) > 120 {
		return ErrInvalid
	}
	if f.AccountID != "" && !canonicalUUID.MatchString(f.AccountID) {
		return ErrInvalid
	}
	if f.Role != "" && f.Role != identity.RoleTenant && f.Role != identity.RoleLandlord && f.Role != identity.RoleAdministrator && f.Role != identity.RoleGlobalAdministrator {
		return ErrInvalid
	}
	if f.State != "" && !f.State.Valid() {
		return ErrInvalid
	}
	if f.CreatedFrom != nil && f.CreatedUntil != nil && (!f.CreatedUntil.After(*f.CreatedFrom) || f.CreatedUntil.After(f.CreatedFrom.AddDate(1, 0, 0))) {
		return ErrInvalid
	}
	return nil
}

func buildSQLFilter(f AccountFilter, snapshot time.Time, cursor *accountCursor) (sqlFilter, error) {
	q := sqlFilter{args: []any{snapshot}}
	q.where = append(q.where, `u.creado_en <= $1`)
	if f.AccountID != "" {
		q.where = append(q.where, q.add(`u.id = $%d::uuid`, f.AccountID))
	}
	if f.EmailPrefix != "" {
		q.where = append(q.where, q.add(`u.correo_normalizado LIKE $%d ESCAPE E'\\'`, escapeLikePrefix(f.EmailPrefix)+"%"))
	}
	if f.NamePrefix != "" {
		q.where = append(q.where, q.add(`lower(p.nombre_visible) LIKE $%d ESCAPE E'\\'`, escapeLikePrefix(f.NamePrefix)+"%"))
	}
	if f.Role != "" {
		q.args = append(q.args, string(f.Role))
		position := len(q.args)
		q.where = append(q.where, fmt.Sprintf(`(EXISTS(SELECT 1 FROM public.rol_usuario r WHERE r.usuario_id=u.id AND r.rol=$%d)
			OR ($%d='administrador_global' AND EXISTS(SELECT 1 FROM public.administrador_global g WHERE g.usuario_id=u.id)))`, position, position))
	}
	if f.State != "" {
		q.where = append(q.where, q.add(`u.estado=$%d`, string(f.State)))
	}
	if f.AdministrativeBlocked != nil {
		q.where = append(q.where, q.add(`EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id)=$%d`, *f.AdministrativeBlocked))
	}
	if f.CreatedFrom != nil {
		q.where = append(q.where, q.add(`u.creado_en >= $%d`, f.CreatedFrom.UTC()))
	}
	if f.CreatedUntil != nil {
		q.where = append(q.where, q.add(`u.creado_en < $%d`, f.CreatedUntil.UTC()))
	}
	if cursor != nil && cursor.AccountID != "" {
		if cursor.Version != 1 || !canonicalUUID.MatchString(cursor.AccountID) || cursor.CreatedAt.IsZero() || cursor.CreatedAt.After(cursor.SnapshotUntil) {
			return sqlFilter{}, ErrInvalid
		}
		q.args = append(q.args, cursor.CreatedAt.UTC(), cursor.AccountID)
		createdPosition, idPosition := len(q.args)-1, len(q.args)
		q.where = append(q.where, fmt.Sprintf(`(u.creado_en,u.id) < ($%d::timestamptz,$%d::uuid)`, createdPosition, idPosition))
	}
	return q, nil
}

func accountPageQuery(where string, limitArg int) string {
	return fmt.Sprintf(`WITH page AS (
		SELECT u.id,u.correo_original,u.estado,u.creado_en,
			EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id) AS administrative_blocked,
			p.nombre_visible,p.telefono_normalizado,p.actualizado_en AS profile_updated_at
		FROM public.usuario u LEFT JOIN public.perfil_usuario p ON p.usuario_id=u.id
		WHERE %s ORDER BY u.creado_en DESC,u.id DESC LIMIT $%d
	), role_rows AS (
		SELECT r.usuario_id AS account_id,r.rol FROM public.rol_usuario r JOIN page p ON p.id=r.usuario_id
		UNION ALL SELECT g.usuario_id,'administrador_global'::text FROM public.administrador_global g JOIN page p ON p.id=g.usuario_id
	), roles AS (SELECT account_id,array_agg(rol ORDER BY rol) AS values FROM role_rows GROUP BY account_id),
	spaces AS (SELECT e.propietario_id account_id,count(*)::bigint total FROM public.espacio e JOIN page p ON p.id=e.propietario_id GROUP BY e.propietario_id),
	reservation_parties AS (
		SELECT r.id,r.anfitrion_id account_id FROM public.reserva_ensayo_local r JOIN page p ON p.id=r.anfitrion_id
		UNION ALL SELECT r.id,r.arrendatario_id FROM public.reserva_ensayo_local r JOIN page p ON p.id=r.arrendatario_id
	), reservations AS (SELECT account_id,count(DISTINCT id)::bigint total FROM reservation_parties GROUP BY account_id),
	claim_parties AS (
		SELECT c.id,c.anfitrion_id account_id FROM public.reclamo_dano_ensayo_local c JOIN page p ON p.id=c.anfitrion_id
		UNION ALL SELECT c.id,c.arrendatario_id FROM public.reclamo_dano_ensayo_local c JOIN page p ON p.id=c.arrendatario_id
	), claims AS (SELECT account_id,count(DISTINCT id)::bigint total FROM claim_parties GROUP BY account_id)
	SELECT p.id::text,p.correo_original,p.nombre_visible,p.telefono_normalizado,p.profile_updated_at,p.estado,p.creado_en,p.administrative_blocked,
		COALESCE(r.values,'{}'::text[]),COALESCE(s.total,0),COALESCE(b.total,0),COALESCE(c.total,0)
	FROM page p LEFT JOIN roles r ON r.account_id=p.id LEFT JOIN spaces s ON s.account_id=p.id
	LEFT JOIN reservations b ON b.account_id=p.id LEFT JOIN claims c ON c.account_id=p.id
	ORDER BY p.creado_en DESC,p.id DESC`, where, limitArg)
}

func requireGlobalActor(ctx context.Context, tx pgx.Tx, actor string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario u JOIN public.administrador_global g ON g.usuario_id=u.id
		WHERE u.id=$1 AND u.estado='activo' AND (u.bloqueado_hasta IS NULL OR u.bloqueado_hasta <= now())
		AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=u.id)
		FOR UPDATE OF u)`, actor).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, actor, resourceID, action, result, reason, correlation string, at time.Time) error {
	return s.auditWithTarget(ctx, tx, actor, resourceID, action, result, reason, correlation, at, "")
}

func (s *Service) auditWithTarget(ctx context.Context, tx pgx.Tx, actor, resourceID, action, result, reason, correlation string, at time.Time, targetID string) error {
	id, err := s.ids.ID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.evento_auditoria_local(id,actor_id,cuenta_objetivo_id,recurso_tipo,recurso_id,accion,resultado,motivo_codigo,correlacion_id,ocurrido_en,registrado_en,retirar_en)
		VALUES($1,$2,CASE WHEN EXISTS(SELECT 1 FROM public.usuario WHERE id=NULLIF($3,'')::uuid) THEN NULLIF($3,'')::uuid END,'cuenta_global',$4,$5,$6,$7,$8,$9,clock_timestamp(),$9::timestamptz+interval '5 years')`, id, actor, targetID, resourceID, action, result, reason, correlation, at)
	return err
}

func filterFingerprint(filter AccountFilter) (string, error) {
	filter.Cursor = ""
	value, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:]), nil
}

func encodeCursor(cursor accountCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeCursor(value string) (accountCursor, error) {
	var cursor accountCursor
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) > 1024 {
		return accountCursor{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cursor); err != nil || cursor.Version != 1 || !canonicalUUID.MatchString(cursor.AccountID) || cursor.CreatedAt.IsZero() || cursor.SnapshotUntil.IsZero() {
		return accountCursor{}, ErrInvalid
	}
	return cursor, nil
}

func escapeLikePrefix(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func utcPointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

const (
	accountSensitiveAccessAction = "admin.global.accounts.sensitive_access"
	sensitiveAccessTTL           = 10 * time.Minute
)

type SensitiveAccessInput struct {
	ActorID        string
	TargetID       string
	AdminSessionID string
	CorrelationID  string
	Field          string
	Reason         string
	CaseID         string
	Scope          string
}

type SensitiveAccessRecord struct {
	ID             string
	ActorID        string
	TargetID       string
	AdminSessionID string
	CorrelationID  string
	Field          string
	Scope          string
	Reason         string
	CaseID         string
	GrantedAt      time.Time
	ExpiresAt      time.Time
	AuditEventID   string
}

type SensitiveAccessGrant struct {
	Value        string
	MaskedValue  string
	AuditEventID string
}

type SensitiveAccessResult struct {
	AccountID    string    `json:"account_id"`
	Field        string    `json:"field"`
	Value        string    `json:"value"`
	MaskedValue  string    `json:"masked_value"`
	Scope        string    `json:"scope"`
	Reason       string    `json:"reason"`
	CaseID       string    `json:"case_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	AuditEventID string    `json:"audit_event_id"`
}

// GrantSensitiveAccess validates, audits, and records an explicit sensitive-field reveal.
func (s *Service) GrantSensitiveAccess(ctx context.Context, input SensitiveAccessInput) (SensitiveAccessResult, error) {
	field := strings.TrimSpace(input.Field)
	if field != "email" {
		return SensitiveAccessResult{}, ErrSensitiveAccessInvalid
	}
	reason := strings.TrimSpace(input.Reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 500 {
		return SensitiveAccessResult{}, ErrSensitiveAccessInvalid
	}
	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = "account_email"
	}
	if scope != "account_email" && scope != "account_summary" {
		return SensitiveAccessResult{}, ErrSensitiveAccessInvalid
	}
	caseID := strings.TrimSpace(input.CaseID)
	if utf8.RuneCountInString(caseID) > 100 {
		return SensitiveAccessResult{}, ErrSensitiveAccessInvalid
	}
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.TargetID) || !canonicalUUID.MatchString(input.CorrelationID) {
		return SensitiveAccessResult{}, ErrInvalid
	}

	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	expiresAt := now.Add(sensitiveAccessTTL)

	accessID, err := s.ids.ID()
	if err != nil {
		return SensitiveAccessResult{}, err
	}
	auditEventID, err := s.ids.ID()
	if err != nil {
		return SensitiveAccessResult{}, err
	}

	rec := SensitiveAccessRecord{
		ID:             accessID,
		ActorID:        input.ActorID,
		TargetID:       input.TargetID,
		AdminSessionID: input.AdminSessionID,
		CorrelationID:  input.CorrelationID,
		Field:          field,
		Scope:          scope,
		Reason:         reason,
		CaseID:         caseID,
		GrantedAt:      now,
		ExpiresAt:      expiresAt,
		AuditEventID:   auditEventID,
	}

	if s.sensitiveAccessStore != nil {
		grant, err := s.sensitiveAccessStore.GrantSensitiveAccess(ctx, rec)
		if err != nil {
			return SensitiveAccessResult{}, err
		}
		if grant.AuditEventID != "" {
			auditEventID = grant.AuditEventID
		}
		return SensitiveAccessResult{
			AccountID:    input.TargetID,
			Field:        field,
			Value:        grant.Value,
			MaskedValue:  grant.MaskedValue,
			Scope:        scope,
			Reason:       reason,
			CaseID:       caseID,
			ExpiresAt:    expiresAt,
			AuditEventID: auditEventID,
		}, nil
	}

	if s.pool == nil {
		return SensitiveAccessResult{}, ErrInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SensitiveAccessResult{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return SensitiveAccessResult{}, err
	}

	var email, state string
	err = tx.QueryRow(ctx, `SELECT correo_original, estado FROM public.usuario WHERE id = $1 FOR SHARE`, input.TargetID).Scan(&email, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return SensitiveAccessResult{}, ErrNotFound
	}
	if err != nil {
		return SensitiveAccessResult{}, err
	}

	masked := identity.MaskEmail(email)
	value := email
	if state == string(identity.AccountDeidentified) {
		value = masked
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO public.evento_auditoria_local(
			id, actor_id, cuenta_objetivo_id, recurso_tipo, recurso_id,
			accion, resultado, motivo_codigo, correlacion_id, ocurrido_en, registrado_en, retirar_en
		) VALUES ($1, $2, $3, 'cuenta_global', $4, $5, 'exito', $6, $7, $8, clock_timestamp(), $8::timestamptz + interval '5 years')
	`, auditEventID, input.ActorID, input.TargetID, input.TargetID, accountSensitiveAccessAction, reason, input.CorrelationID, now)
	if err != nil {
		return SensitiveAccessResult{}, err
	}

	var sessionID any
	if canonicalUUID.MatchString(input.AdminSessionID) {
		sessionID = input.AdminSessionID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.acceso_sensible_administrativo(
			id, actor_cuenta_id, objetivo_cuenta_id, sesion_administrativa_id,
			campo, alcance, motivo, caso_id, otorgado_en, expira_en, evento_auditoria_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, accessID, input.ActorID, input.TargetID, sessionID, field, scope, reason, caseID, now, expiresAt, auditEventID)
	if err != nil {
		return SensitiveAccessResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SensitiveAccessResult{}, err
	}

	return SensitiveAccessResult{
		AccountID:    input.TargetID,
		Field:        field,
		Value:        value,
		MaskedValue:  masked,
		Scope:        scope,
		Reason:       reason,
		CaseID:       caseID,
		ExpiresAt:    expiresAt,
		AuditEventID: auditEventID,
	}, nil
}
