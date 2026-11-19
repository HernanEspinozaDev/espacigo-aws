package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/jackc/pgx/v5"
)

var _ identity.SecurityAlertsRepository = (*IdentityRepository)(nil)

func (r *IdentityRepository) GetSecurityAlertPreferences(ctx context.Context, accountID string) (*identity.SecurityAlertPreferencesRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	const query = `
SELECT
    account_id::text,
    app_notifications_enabled,
    email_alerts_enabled,
    whatsapp_alerts_enabled,
    extra_email,
    extra_email_verified_at,
    updated_at
FROM public.cuenta_alerta_preferencia
WHERE account_id = $1
`
	row := dbtx.QueryRow(ctx, query, accountID)
	var (
		rec                  identity.SecurityAlertPreferencesRecord
		extraEmail           *string
		extraEmailVerifiedAt *time.Time
	)
	if err := row.Scan(
		&rec.AccountID,
		&rec.AppNotificationsEnabled,
		&rec.EmailAlertsEnabled,
		&rec.WhatsAppAlertsEnabled,
		&extraEmail,
		&extraEmailVerifiedAt,
		&rec.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapError(err)
	}
	rec.ExtraEmail = extraEmail
	if extraEmailVerifiedAt != nil {
		t := extraEmailVerifiedAt.UTC()
		rec.ExtraEmailVerifiedAt = &t
	}
	rec.UpdatedAt = rec.UpdatedAt.UTC()
	return &rec, nil
}

func (r *IdentityRepository) UpsertSecurityAlertPreferences(ctx context.Context, record identity.SecurityAlertPreferencesRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}
	const query = `
INSERT INTO public.cuenta_alerta_preferencia (
    account_id,
    app_notifications_enabled,
    email_alerts_enabled,
    whatsapp_alerts_enabled,
    extra_email,
    extra_email_verified_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id) DO UPDATE SET
    app_notifications_enabled = EXCLUDED.app_notifications_enabled,
    email_alerts_enabled = EXCLUDED.email_alerts_enabled,
    whatsapp_alerts_enabled = EXCLUDED.whatsapp_alerts_enabled,
    extra_email = EXCLUDED.extra_email,
    extra_email_verified_at = EXCLUDED.extra_email_verified_at,
    updated_at = EXCLUDED.updated_at
`
	_, err := dbtx.Exec(ctx, query,
		record.AccountID,
		record.AppNotificationsEnabled,
		record.EmailAlertsEnabled,
		record.WhatsAppAlertsEnabled,
		record.ExtraEmail,
		record.ExtraEmailVerifiedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (r *IdentityRepository) ListSecurityAlertEvents(ctx context.Context, accountID string, limit int) ([]identity.SecurityAlertEventRecord, error) {
	dbtx := r.dbtx()
	if dbtx == nil {
		return nil, errors.New("repository uninitialized")
	}
	if limit <= 0 {
		limit = 20
	}
	const query = `
SELECT
    id::text,
    account_id::text,
    event_code,
    title,
    COALESCE(device_label, ''),
    COALESCE(ip_masked, ''),
    created_at
FROM public.cuenta_alerta_evento
WHERE account_id = $1
ORDER BY created_at DESC
LIMIT $2
`
	rows, err := dbtx.Query(ctx, query, accountID, limit)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	var events []identity.SecurityAlertEventRecord
	for rows.Next() {
		var ev identity.SecurityAlertEventRecord
		if err := rows.Scan(
			&ev.ID,
			&ev.AccountID,
			&ev.EventCode,
			&ev.Title,
			&ev.DeviceLabel,
			&ev.IPMasked,
			&ev.CreatedAt,
		); err != nil {
			return nil, mapError(err)
		}
		ev.CreatedAt = ev.CreatedAt.UTC()
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err)
	}
	if events == nil {
		events = []identity.SecurityAlertEventRecord{}
	}
	return events, nil
}

func (r *IdentityRepository) RecordSecurityAlertEvent(ctx context.Context, event identity.SecurityAlertEventRecord) error {
	dbtx := r.dbtx()
	if dbtx == nil {
		return errors.New("repository uninitialized")
	}
	const query = `
INSERT INTO public.cuenta_alerta_evento (
    id,
    account_id,
    event_code,
    title,
    device_label,
    ip_masked,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
`
	_, err := dbtx.Exec(ctx, query,
		event.ID,
		event.AccountID,
		event.EventCode,
		event.Title,
		event.DeviceLabel,
		event.IPMasked,
		event.CreatedAt,
	)
	if err != nil {
		return mapError(err)
	}
	return nil
}
