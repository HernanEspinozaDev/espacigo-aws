package adminglobal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	privacyIncidentCreateAuditAction     = "admin.global.privacy_incidents.create"
	privacyIncidentTransitionAuditAction = "admin.global.privacy_incidents.transition"
	privacyLegalHoldCreateAuditAction    = "admin.global.privacy_legal_holds.create"
	privacyLegalHoldReleaseAuditAction   = "admin.global.privacy_legal_holds.release"
)

type PrivacyIncidentFilter struct {
	Status   string `json:"status"`
	Severity string `json:"severity"`
	PageSize int    `json:"-"`
	Cursor   string `json:"-"`
}

type PrivacyIncidentItem struct {
	ID                    string     `json:"id"`
	Folio                 string     `json:"folio"`
	Title                 string     `json:"title"`
	Severity              string     `json:"severity"`
	Status                string     `json:"status"`
	DataCategories        []string   `json:"data_categories"`
	AffectedSystems       []string   `json:"affected_systems"`
	EstimatedSubjects     int        `json:"estimated_subjects"`
	Summary               string     `json:"summary"`
	ContainmentMeasures   *string    `json:"containment_measures,omitempty"`
	RequiresAgencyNotice  bool       `json:"requires_agency_notice"`
	AgencyNotifiedAt      *time.Time `json:"agency_notified_at,omitempty"`
	AgencyReference       *string    `json:"agency_reference,omitempty"`
	RequiresSubjectNotice bool       `json:"requires_subject_notice"`
	SubjectsNotifiedAt    *time.Time `json:"subjects_notified_at,omitempty"`
	LeaderID              string     `json:"leader_id"`
	DeputyID              *string    `json:"deputy_id,omitempty"`
	DetectedAt            time.Time  `json:"detected_at"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ClosedAt              *time.Time `json:"closed_at,omitempty"`
	Version               int        `json:"version"`
}

type PrivacyIncidentEvent struct {
	ID        string    `json:"id"`
	Sequence  int       `json:"sequence"`
	EventType string    `json:"event_type"`
	ActorID   string    `json:"actor_id"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type PrivacyIncidentDetail struct {
	PrivacyIncidentItem
	Events []PrivacyIncidentEvent `json:"events"`
}

type CreatePrivacyIncidentInput struct {
	ActorID               string    `json:"-"`
	CorrelationID         string    `json:"-"`
	Title                 string    `json:"title"`
	Severity              string    `json:"severity"`
	DataCategories        []string  `json:"data_categories"`
	AffectedSystems       []string  `json:"affected_systems"`
	EstimatedSubjects     int       `json:"estimated_subjects"`
	Summary               string    `json:"summary"`
	ContainmentMeasures   string    `json:"containment_measures,omitempty"`
	RequiresAgencyNotice  bool      `json:"requires_agency_notice"`
	RequiresSubjectNotice bool      `json:"requires_subject_notice"`
	LeaderID              string    `json:"leader_id"`
	DeputyID              string    `json:"deputy_id,omitempty"`
	DetectedAt            time.Time `json:"detected_at"`
}

type TransitionPrivacyIncidentInput struct {
	ActorID               string `json:"-"`
	CorrelationID         string `json:"-"`
	IncidentID            string `json:"-"`
	Action                string `json:"action"`
	ExpectedVersion       int    `json:"expected_version"`
	Detail                string `json:"detail"`
	ContainmentMeasures   string `json:"containment_measures,omitempty"`
	RequiresAgencyNotice  *bool  `json:"requires_agency_notice,omitempty"`
	RequiresSubjectNotice *bool  `json:"requires_subject_notice,omitempty"`
	AgencyReference       string `json:"agency_reference,omitempty"`
}

type PrivacyLegalHoldFilter struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
	Scope     string `json:"scope"`
	PageSize  int    `json:"-"`
	Cursor    string `json:"-"`
}

type PrivacyLegalHoldItem struct {
	ID            string     `json:"id"`
	Folio         string     `json:"folio"`
	AccountID     *string    `json:"account_id,omitempty"`
	RequestID     *string    `json:"request_id,omitempty"`
	IncidentID    *string    `json:"incident_id,omitempty"`
	Scope         string     `json:"scope"`
	Authority     string     `json:"authority"`
	LegalBasis    string     `json:"legal_basis"`
	Status        string     `json:"status"`
	ReviewAt      *time.Time `json:"review_at,omitempty"`
	AppliedBy     string     `json:"applied_by"`
	AppliedAt     time.Time  `json:"applied_at"`
	ReleasedBy    *string    `json:"released_by,omitempty"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	ReleaseReason *string    `json:"release_reason,omitempty"`
}

type CreatePrivacyLegalHoldInput struct {
	ActorID       string     `json:"-"`
	CorrelationID string     `json:"-"`
	AccountID     string     `json:"account_id,omitempty"`
	RequestID     string     `json:"request_id,omitempty"`
	IncidentID    string     `json:"incident_id,omitempty"`
	Scope         string     `json:"scope"`
	Authority     string     `json:"authority"`
	LegalBasis    string     `json:"legal_basis"`
	ReviewAt      *time.Time `json:"review_at,omitempty"`
}

type ReleasePrivacyLegalHoldInput struct {
	ActorID       string `json:"-"`
	CorrelationID string `json:"-"`
	HoldID        string `json:"-"`
	ReleaseReason string `json:"release_reason"`
}

func (s *Service) ListPrivacyIncidents(ctx context.Context, actor, correlation string, filter PrivacyIncidentFilter) (OpsPage[PrivacyIncidentItem], error) {
	if (filter.Status != "" && !validIncidentStatus(filter.Status)) ||
		(filter.Severity != "" && !validIncidentSeverity(filter.Severity)) {
		return OpsPage[PrivacyIncidentItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[PrivacyIncidentItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListPrivacyIncidents(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[PrivacyIncidentItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildPrivacyIncidentWhere(filter)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.privacidad_incidente_datos i`+where, args...).Scan(&total); err != nil {
		return OpsPage[PrivacyIncidentItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "i.creado_en", "i.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT i.id::text, i.folio_incidente, i.titulo, i.severidad, i.estado,
		       i.categorias_datos, i.sistemas_afectados, i.titulares_estimados, i.resumen_hechos,
		       i.medidas_contencion, i.requiere_notificacion_agencia, i.notificado_agencia_en, i.referencia_agencia,
		       i.requiere_notificacion_titulares, i.notificado_titulares_en,
		       i.lider_id::text, i.suplente_id::text, i.detectado_en, i.creado_por::text,
		       i.creado_en, i.actualizado_en, i.cerrado_en, i.version
		FROM public.privacidad_incidente_datos i`+cursorClause+`
		ORDER BY i.creado_en DESC, i.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[PrivacyIncidentItem]{}, err
	}
	defer rows.Close()

	items := make([]PrivacyIncidentItem, 0, pageSize+1)
	for rows.Next() {
		item, err := scanPrivacyIncidentItem(rows)
		if err != nil {
			return OpsPage[PrivacyIncidentItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[PrivacyIncidentItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.currentTime(), func(it PrivacyIncidentItem) (time.Time, string) { return it.CreatedAt, it.ID })
}

func (s *Service) GetPrivacyIncident(ctx context.Context, actor, correlation, incidentID string) (PrivacyIncidentDetail, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(correlation) || !canonicalUUID.MatchString(incidentID) {
		return PrivacyIncidentDetail{}, ErrInvalid
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.GetPrivacyIncident(ctx, actor, correlation, incidentID)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	return loadPrivacyIncidentDetailTx(ctx, tx, incidentID)
}

func (s *Service) CreatePrivacyIncident(ctx context.Context, input CreatePrivacyIncidentInput) (PrivacyIncidentDetail, error) {
	now := s.currentTime().Truncate(time.Microsecond)
	title, okTitle := validTrimmedText(input.Title, 5, 200)
	summary, okSummary := validTrimmedText(input.Summary, 10, 2000)
	categories, okCats := normalizeStringSlice(input.DataCategories, 2, 80)
	systems, okSys := normalizeStringSlice(input.AffectedSystems, 2, 80)
	containment, okContainment := optionalTrimmedText(input.ContainmentMeasures, 10, 2000)
	deputy := strings.TrimSpace(input.DeputyID)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.LeaderID) || (deputy != "" && !canonicalUUID.MatchString(deputy)) ||
		!okTitle || !validIncidentSeverity(input.Severity) || !okCats || !okSys ||
		input.EstimatedSubjects < 0 || !okSummary || !okContainment ||
		input.DetectedAt.IsZero() || input.DetectedAt.After(now.Add(time.Minute)) {
		return PrivacyIncidentDetail{}, ErrInvalid
	}
	input.Title = title
	input.Summary = summary
	input.DataCategories = categories
	input.AffectedSystems = systems
	input.DeputyID = deputy
	if containment != nil {
		input.ContainmentMeasures = *containment
	} else {
		input.ContainmentMeasures = ""
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.CreatePrivacyIncident(ctx, input)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := requireExistingUserTx(ctx, tx, input.LeaderID); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if deputy != "" {
		if err := requireExistingUserTx(ctx, tx, deputy); err != nil {
			return PrivacyIncidentDetail{}, err
		}
	}

	incidentID, err := s.ids.ID()
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	folio, err := generatePrefixedFolio("INC", now)
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	detectedAt := input.DetectedAt.UTC().Truncate(time.Microsecond)
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.privacidad_incidente_datos (
			id, folio_incidente, titulo, severidad, estado,
			categorias_datos, sistemas_afectados, titulares_estimados, resumen_hechos,
			medidas_contencion, requiere_notificacion_agencia, requiere_notificacion_titulares,
			lider_id, suplente_id, detectado_en, creado_por, creado_en, actualizado_en, version
		) VALUES (
			$1, $2, $3, $4, 'abierto',
			$5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15, $16, $16, 1
		)
	`, incidentID, folio, title, input.Severity,
		categories, systems, input.EstimatedSubjects, summary,
		containment, input.RequiresAgencyNotice, input.RequiresSubjectNotice,
		input.LeaderID, nullableUUID(deputy), detectedAt, input.ActorID, now); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := s.appendPrivacyIncidentEventTx(ctx, tx, incidentID, "apertura", input.ActorID, summary, now); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := s.audit(ctx, tx, input.ActorID, incidentID, privacyIncidentCreateAuditAction, "exito", input.Severity, input.CorrelationID, now); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	detail, err := loadPrivacyIncidentDetailTx(ctx, tx, incidentID)
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	return detail, nil
}

func (s *Service) TransitionPrivacyIncident(ctx context.Context, input TransitionPrivacyIncidentInput) (PrivacyIncidentDetail, error) {
	detailText, okDetail := validTrimmedText(input.Detail, 5, 2000)
	containment, okContainment := optionalTrimmedText(input.ContainmentMeasures, 10, 2000)
	agencyRef, okAgencyRef := optionalTrimmedText(input.AgencyReference, 2, 160)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.IncidentID) || input.ExpectedVersion < 1 || !okDetail {
		return PrivacyIncidentDetail{}, ErrInvalid
	}
	switch input.Action {
	case "contain":
		if utf8.RuneCountInString(detailText) < 10 || !okContainment {
			return PrivacyIncidentDetail{}, ErrInvalid
		}
	case "evaluate":
	case "notify_agency":
		if !okAgencyRef {
			return PrivacyIncidentDetail{}, ErrInvalid
		}
	case "notify_subjects", "add_note", "close":
	default:
		return PrivacyIncidentDetail{}, ErrInvalid
	}
	input.Detail = detailText
	if containment != nil {
		input.ContainmentMeasures = *containment
	} else {
		input.ContainmentMeasures = ""
	}
	if agencyRef != nil {
		input.AgencyReference = *agencyRef
	} else {
		input.AgencyReference = ""
	}
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.TransitionPrivacyIncident(ctx, input)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyIncidentDetail{}, err
	}

	var status string
	var version int
	var requiresAgency, requiresSubjects bool
	var agencyNotifiedAt, subjectsNotifiedAt *time.Time
	var existingAgencyRef *string
	err = tx.QueryRow(ctx, `
		SELECT estado, version, requiere_notificacion_agencia, notificado_agencia_en,
		       referencia_agencia, requiere_notificacion_titulares, notificado_titulares_en
		FROM public.privacidad_incidente_datos
		WHERE id = $1
		FOR UPDATE
	`, input.IncidentID).Scan(
		&status, &version, &requiresAgency, &agencyNotifiedAt,
		&existingAgencyRef, &requiresSubjects, &subjectsNotifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyIncidentDetail{}, ErrNotFound
	}
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if version != input.ExpectedVersion || status == "cerrado" {
		return PrivacyIncidentDetail{}, ErrConflict
	}

	now := s.currentTime().Truncate(time.Microsecond)
	var eventType string
	switch input.Action {
	case "contain":
		eventType = "contencion"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET estado = CASE WHEN estado = 'abierto' THEN 'contenido' ELSE estado END,
			    medidas_contencion = COALESCE(NULLIF($2, ''), $3),
			    actualizado_en = $4,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, input.ContainmentMeasures, detailText, now); err != nil {
			return PrivacyIncidentDetail{}, err
		}

	case "evaluate":
		if input.RequiresAgencyNotice != nil && !*input.RequiresAgencyNotice && agencyNotifiedAt != nil {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		if input.RequiresSubjectNotice != nil && !*input.RequiresSubjectNotice && subjectsNotifiedAt != nil {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		nextReqAgency := requiresAgency
		if input.RequiresAgencyNotice != nil {
			nextReqAgency = *input.RequiresAgencyNotice
		}
		nextReqSubjects := requiresSubjects
		if input.RequiresSubjectNotice != nil {
			nextReqSubjects = *input.RequiresSubjectNotice
		}
		eventType = "evaluacion"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET estado = CASE WHEN estado IN ('abierto', 'contenido') THEN 'evaluado' ELSE estado END,
			    requiere_notificacion_agencia = $2,
			    requiere_notificacion_titulares = $3,
			    actualizado_en = $4,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, nextReqAgency, nextReqSubjects, now); err != nil {
			return PrivacyIncidentDetail{}, err
		}

	case "notify_agency":
		if input.RequiresAgencyNotice != nil && !*input.RequiresAgencyNotice {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		if !requiresAgency && (input.RequiresAgencyNotice == nil || !*input.RequiresAgencyNotice) {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		if input.AgencyReference == "" && (existingAgencyRef == nil || strings.TrimSpace(*existingAgencyRef) == "") {
			return PrivacyIncidentDetail{}, ErrInvalid
		}
		eventType = "notificacion_agencia"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET requiere_notificacion_agencia = true,
			    notificado_agencia_en = COALESCE(notificado_agencia_en, $2),
			    referencia_agencia = COALESCE(NULLIF($3, ''), referencia_agencia),
			    estado = CASE WHEN estado <> 'cerrado' THEN 'notificado' ELSE estado END,
			    actualizado_en = $2,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, now, input.AgencyReference); err != nil {
			return PrivacyIncidentDetail{}, err
		}

	case "notify_subjects":
		if input.RequiresSubjectNotice != nil && !*input.RequiresSubjectNotice {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		eventType = "notificacion_titulares"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET requiere_notificacion_titulares = true,
			    notificado_titulares_en = COALESCE(notificado_titulares_en, $2),
			    estado = CASE WHEN estado <> 'cerrado' THEN 'notificado' ELSE estado END,
			    actualizado_en = $2,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, now); err != nil {
			return PrivacyIncidentDetail{}, err
		}

	case "add_note":
		eventType = "nota"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET actualizado_en = $2,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, now); err != nil {
			return PrivacyIncidentDetail{}, err
		}

	case "close":
		if requiresAgency && agencyNotifiedAt == nil {
			return PrivacyIncidentDetail{}, ErrConflict
		}
		eventType = "cierre"
		if _, err := tx.Exec(ctx, `
			UPDATE public.privacidad_incidente_datos
			SET estado = 'cerrado',
			    cerrado_en = $2,
			    actualizado_en = $2,
			    version = version + 1
			WHERE id = $1
		`, input.IncidentID, now); err != nil {
			return PrivacyIncidentDetail{}, err
		}
	}

	if err := s.appendPrivacyIncidentEventTx(ctx, tx, input.IncidentID, eventType, input.ActorID, detailText, now); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := s.audit(ctx, tx, input.ActorID, input.IncidentID, privacyIncidentTransitionAuditAction, "exito", input.Action, input.CorrelationID, now); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	detail, err := loadPrivacyIncidentDetailTx(ctx, tx, input.IncidentID)
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	return detail, nil
}

func (s *Service) ListPrivacyLegalHolds(ctx context.Context, actor, correlation string, filter PrivacyLegalHoldFilter) (OpsPage[PrivacyLegalHoldItem], error) {
	if !validateOptionalUUID(filter.AccountID) ||
		(filter.Status != "" && !validLegalHoldStatus(filter.Status)) ||
		(filter.Scope != "" && !validLegalHoldScope(filter.Scope)) {
		return OpsPage[PrivacyLegalHoldItem]{}, ErrInvalid
	}
	pageSize, filterHash, cursor, err := validateOpsListArgs(actor, correlation, filter.Cursor, filter.PageSize, filter)
	if err != nil {
		return OpsPage[PrivacyLegalHoldItem]{}, err
	}
	if s.specializedOpsStore != nil {
		filter.PageSize = pageSize
		return s.specializedOpsStore.ListPrivacyLegalHolds(ctx, actor, correlation, filter)
	}
	tx, err := s.beginGlobalReadTx(ctx, actor)
	if err != nil {
		return OpsPage[PrivacyLegalHoldItem]{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	where, args := buildPrivacyLegalHoldWhere(filter)
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.privacidad_retencion_legal h`+where, args...).Scan(&total); err != nil {
		return OpsPage[PrivacyLegalHoldItem]{}, err
	}
	cursorClause, args := appendCursorClause(where, args, "h.aplicada_en", "h.id", cursor, pageSize)
	rows, err := tx.Query(ctx, `
		SELECT h.id::text, h.folio_retencion, h.cuenta_id::text, h.solicitud_id::text, h.incidente_id::text,
		       h.ambito, h.autoridad, h.fundamento, h.estado, h.revisar_en,
		       h.aplicada_por::text, h.aplicada_en, h.liberada_por::text, h.liberada_en, h.motivo_liberacion
		FROM public.privacidad_retencion_legal h`+cursorClause+`
		ORDER BY h.aplicada_en DESC, h.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return OpsPage[PrivacyLegalHoldItem]{}, err
	}
	defer rows.Close()

	items := make([]PrivacyLegalHoldItem, 0, pageSize+1)
	for rows.Next() {
		item, err := scanPrivacyLegalHoldItem(rows)
		if err != nil {
			return OpsPage[PrivacyLegalHoldItem]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return OpsPage[PrivacyLegalHoldItem]{}, err
	}
	return finalizeOpsPage(items, pageSize, total, filterHash, s.currentTime(), func(it PrivacyLegalHoldItem) (time.Time, string) { return it.AppliedAt, it.ID })
}

func (s *Service) CreatePrivacyLegalHold(ctx context.Context, input CreatePrivacyLegalHoldInput) (PrivacyLegalHoldItem, error) {
	authority, okAuth := validTrimmedText(input.Authority, 3, 200)
	legalBasis, okBasis := validTrimmedText(input.LegalBasis, 10, 1000)
	accountID := strings.TrimSpace(input.AccountID)
	requestID := strings.TrimSpace(input.RequestID)
	incidentID := strings.TrimSpace(input.IncidentID)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!validateOptionalUUID(accountID) || !validateOptionalUUID(requestID) || !validateOptionalUUID(incidentID) ||
		!validLegalHoldScope(input.Scope) || !okAuth || !okBasis ||
		(input.ReviewAt != nil && input.ReviewAt.IsZero()) {
		return PrivacyLegalHoldItem{}, ErrInvalid
	}
	input.Authority = authority
	input.LegalBasis = legalBasis
	input.AccountID = accountID
	input.RequestID = requestID
	input.IncidentID = incidentID
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.CreatePrivacyLegalHold(ctx, input)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	if accountID != "" {
		if err := requireExistingUserTx(ctx, tx, accountID); err != nil {
			return PrivacyLegalHoldItem{}, err
		}
	}
	if requestID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.solicitud_titular WHERE id=$1)`, requestID).Scan(&exists); err != nil {
			return PrivacyLegalHoldItem{}, err
		}
		if !exists {
			return PrivacyLegalHoldItem{}, ErrNotFound
		}
	}
	if incidentID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.privacidad_incidente_datos WHERE id=$1)`, incidentID).Scan(&exists); err != nil {
			return PrivacyLegalHoldItem{}, err
		}
		if !exists {
			return PrivacyLegalHoldItem{}, ErrNotFound
		}
	}

	now := s.currentTime().Truncate(time.Microsecond)
	id, err := s.ids.ID()
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	folio, err := generatePrefixedFolio("HOLD", now)
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	var reviewAt *time.Time
	if input.ReviewAt != nil {
		r := input.ReviewAt.UTC().Truncate(time.Microsecond)
		reviewAt = &r
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO public.privacidad_retencion_legal (
			id, folio_retencion, cuenta_id, solicitud_id, incidente_id,
			ambito, autoridad, fundamento, estado, revisar_en, aplicada_por, aplicada_en
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, 'activa', $9, $10, $11
		)
		RETURNING id::text, folio_retencion, cuenta_id::text, solicitud_id::text, incidente_id::text,
		          ambito, autoridad, fundamento, estado, revisar_en,
		          aplicada_por::text, aplicada_en, liberada_por::text, liberada_en, motivo_liberacion
	`, id, folio, nullableUUID(accountID), nullableUUID(requestID), nullableUUID(incidentID),
		input.Scope, authority, legalBasis, reviewAt, input.ActorID, now)
	item, err := scanPrivacyLegalHoldItem(row)
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, privacyLegalHoldCreateAuditAction, "exito", input.Scope, input.CorrelationID, now, accountID); err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	return item, nil
}

func (s *Service) ReleasePrivacyLegalHold(ctx context.Context, input ReleasePrivacyLegalHoldInput) (PrivacyLegalHoldItem, error) {
	reason, okReason := validTrimmedText(input.ReleaseReason, 10, 1000)
	if !canonicalUUID.MatchString(input.ActorID) || !canonicalUUID.MatchString(input.CorrelationID) ||
		!canonicalUUID.MatchString(input.HoldID) || !okReason {
		return PrivacyLegalHoldItem{}, ErrInvalid
	}
	input.ReleaseReason = reason
	if s.specializedOpsStore != nil {
		return s.specializedOpsStore.ReleasePrivacyLegalHold(ctx, input)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := requireGlobalActor(ctx, tx, input.ActorID); err != nil {
		return PrivacyLegalHoldItem{}, err
	}

	now := s.currentTime().Truncate(time.Microsecond)
	row := tx.QueryRow(ctx, `
		UPDATE public.privacidad_retencion_legal
		SET estado='liberada', liberada_por=$2, liberada_en=$3, motivo_liberacion=$4
		WHERE id=$1 AND estado='activa'
		RETURNING id::text, folio_retencion, cuenta_id::text, solicitud_id::text, incidente_id::text,
		          ambito, autoridad, fundamento, estado, revisar_en,
		          aplicada_por::text, aplicada_en, liberada_por::text, liberada_en, motivo_liberacion
	`, input.HoldID, input.ActorID, now, reason)
	item, err := scanPrivacyLegalHoldItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if checkErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.privacidad_retencion_legal WHERE id=$1)`, input.HoldID).Scan(&exists); checkErr != nil {
			return PrivacyLegalHoldItem{}, checkErr
		}
		if exists {
			return PrivacyLegalHoldItem{}, ErrConflict
		}
		return PrivacyLegalHoldItem{}, ErrNotFound
	}
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	targetAccount := ""
	if item.AccountID != nil {
		targetAccount = *item.AccountID
	}
	if err := s.auditWithTarget(ctx, tx, input.ActorID, item.ID, privacyLegalHoldReleaseAuditAction, "exito", "retencion_liberada", input.CorrelationID, now, targetAccount); err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	return item, nil
}

func (s *Service) appendPrivacyIncidentEventTx(ctx context.Context, tx pgx.Tx, incidentID, eventType, actorID, detail string, at time.Time) error {
	eventID, err := s.ids.ID()
	if err != nil {
		return err
	}
	var nextSeq int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(secuencia), 0) + 1 FROM public.privacidad_incidente_evento WHERE incidente_id=$1`, incidentID).Scan(&nextSeq); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO public.privacidad_incidente_evento (
			id, incidente_id, secuencia, tipo_evento, actor_id, detalle, creado_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, eventID, incidentID, nextSeq, eventType, actorID, detail, at.UTC())
	return err
}

func loadPrivacyIncidentDetailTx(ctx context.Context, tx pgx.Tx, incidentID string) (PrivacyIncidentDetail, error) {
	row := tx.QueryRow(ctx, `
		SELECT i.id::text, i.folio_incidente, i.titulo, i.severidad, i.estado,
		       i.categorias_datos, i.sistemas_afectados, i.titulares_estimados, i.resumen_hechos,
		       i.medidas_contencion, i.requiere_notificacion_agencia, i.notificado_agencia_en, i.referencia_agencia,
		       i.requiere_notificacion_titulares, i.notificado_titulares_en,
		       i.lider_id::text, i.suplente_id::text, i.detectado_en, i.creado_por::text,
		       i.creado_en, i.actualizado_en, i.cerrado_en, i.version
		FROM public.privacidad_incidente_datos i
		WHERE i.id = $1
	`, incidentID)
	item, err := scanPrivacyIncidentItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyIncidentDetail{}, ErrNotFound
	}
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, secuencia, tipo_evento, actor_id::text, detalle, creado_en
		FROM public.privacidad_incidente_evento
		WHERE incidente_id = $1
		ORDER BY secuencia ASC, id ASC
	`, incidentID)
	if err != nil {
		return PrivacyIncidentDetail{}, err
	}
	defer rows.Close()
	events := []PrivacyIncidentEvent{}
	for rows.Next() {
		var ev PrivacyIncidentEvent
		if err := rows.Scan(&ev.ID, &ev.Sequence, &ev.EventType, &ev.ActorID, &ev.Detail, &ev.CreatedAt); err != nil {
			return PrivacyIncidentDetail{}, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return PrivacyIncidentDetail{}, err
	}
	return PrivacyIncidentDetail{
		PrivacyIncidentItem: item,
		Events:              events,
	}, nil
}

type privacyRowScanner interface {
	Scan(dest ...any) error
}

func scanPrivacyIncidentItem(row privacyRowScanner) (PrivacyIncidentItem, error) {
	var item PrivacyIncidentItem
	err := row.Scan(
		&item.ID, &item.Folio, &item.Title, &item.Severity, &item.Status,
		&item.DataCategories, &item.AffectedSystems, &item.EstimatedSubjects, &item.Summary,
		&item.ContainmentMeasures, &item.RequiresAgencyNotice, &item.AgencyNotifiedAt, &item.AgencyReference,
		&item.RequiresSubjectNotice, &item.SubjectsNotifiedAt,
		&item.LeaderID, &item.DeputyID, &item.DetectedAt, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.ClosedAt, &item.Version,
	)
	if err != nil {
		return PrivacyIncidentItem{}, err
	}
	if item.DataCategories == nil {
		item.DataCategories = []string{}
	}
	if item.AffectedSystems == nil {
		item.AffectedSystems = []string{}
	}
	return item, nil
}

func scanPrivacyLegalHoldItem(row privacyRowScanner) (PrivacyLegalHoldItem, error) {
	var item PrivacyLegalHoldItem
	err := row.Scan(
		&item.ID, &item.Folio, &item.AccountID, &item.RequestID, &item.IncidentID,
		&item.Scope, &item.Authority, &item.LegalBasis, &item.Status, &item.ReviewAt,
		&item.AppliedBy, &item.AppliedAt, &item.ReleasedBy, &item.ReleasedAt, &item.ReleaseReason,
	)
	if err != nil {
		return PrivacyLegalHoldItem{}, err
	}
	return item, nil
}

func requireExistingUserTx(ctx context.Context, tx pgx.Tx, userID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id=$1)`, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func buildPrivacyIncidentWhere(f PrivacyIncidentFilter) (string, []any) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "i.estado = $"+strconv.Itoa(len(args)))
	}
	if f.Severity != "" {
		args = append(args, f.Severity)
		conds = append(conds, "i.severidad = $"+strconv.Itoa(len(args)))
	}
	return joinWhere(conds), args
}

func buildPrivacyLegalHoldWhere(f PrivacyLegalHoldFilter) (string, []any) {
	conds := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "h.estado = $"+strconv.Itoa(len(args)))
	}
	if f.AccountID != "" {
		args = append(args, f.AccountID)
		conds = append(conds, "h.cuenta_id = $"+strconv.Itoa(len(args)))
	}
	if f.Scope != "" {
		args = append(args, f.Scope)
		conds = append(conds, "h.ambito = $"+strconv.Itoa(len(args)))
	}
	return joinWhere(conds), args
}

func validTrimmedText(raw string, minRunes, maxRunes int) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, 0) {
		return "", false
	}
	n := utf8.RuneCountInString(trimmed)
	return trimmed, n >= minRunes && n <= maxRunes
}

func optionalTrimmedText(raw string, minRunes, maxRunes int) (*string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, true
	}
	if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, 0) {
		return nil, false
	}
	n := utf8.RuneCountInString(trimmed)
	if n < minRunes || n > maxRunes {
		return nil, false
	}
	return &trimmed, true
}

func normalizeStringSlice(items []string, minRunes, maxRunes int) ([]string, bool) {
	if len(items) < 1 {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		trimmed, ok := validTrimmedText(raw, minRunes, maxRunes)
		if !ok {
			return nil, false
		}
		out = append(out, trimmed)
	}
	return out, true
}

func generatePrefixedFolio(prefix string, now time.Time) (string, error) {
	var raw [4]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%04d-%s", prefix, now.UTC().Year(), strings.ToUpper(hex.EncodeToString(raw[:]))), nil
}

func validIncidentSeverity(v string) bool {
	switch v {
	case "baja", "media", "alta", "critica":
		return true
	}
	return false
}

func validIncidentStatus(v string) bool {
	switch v {
	case "abierto", "contenido", "evaluado", "notificado", "cerrado":
		return true
	}
	return false
}

func validLegalHoldScope(v string) bool {
	switch v {
	case "expediente_privacidad", "cuenta_completa", "evidencia_kyc", "contratos_y_pagos", "incidente_seguridad":
		return true
	}
	return false
}

func validLegalHoldStatus(v string) bool {
	switch v {
	case "activa", "liberada":
		return true
	}
	return false
}
