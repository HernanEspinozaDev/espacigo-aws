package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/platform/safelog"
)

var (
	ErrInvalidExtraEmail        = errors.New("formato de correo secundario inválido")
	ErrExtraEmailMatchesPrimary = errors.New("el correo secundario no puede ser igual al principal")
)

// SecurityAlertPreferences represents the user's notification channel toggles and masked contacts.
type SecurityAlertPreferences struct {
	AppNotificationsEnabled bool   `json:"app_notifications_enabled"`
	EmailAlertsEnabled      bool   `json:"email_alerts_enabled"`
	WhatsAppAlertsEnabled   bool   `json:"whatsapp_alerts_enabled"`
	MaskedEmail             string `json:"masked_email"`
	MaskedExtraEmail        string `json:"masked_extra_email"`
	ExtraEmailConfigured    bool   `json:"extra_email_configured"`
	MaskedPhone             string `json:"masked_phone"`
}

// SecurityAlertEvent represents an audit event in the user's security feed.
// Full IP addresses and sensitive tokens are strictly excluded.
type SecurityAlertEvent struct {
	ID          string    `json:"id"`
	EventCode   string    `json:"event_code"`
	Title       string    `json:"title"`
	DeviceLabel string    `json:"device_label"`
	IPMasked    string    `json:"ip_masked"`
	CreatedAt   time.Time `json:"created_at"`
}

// SecurityAlertsResponse is the payload for GET /api/v1/account/security/alerts.
type SecurityAlertsResponse struct {
	Preferences SecurityAlertPreferences `json:"preferences"`
	Alerts      []SecurityAlertEvent     `json:"alerts"`
}

// UpdateSecurityAlertChannelsInput is the input for PUT /api/v1/account/security/alerts/channels.
type UpdateSecurityAlertChannelsInput struct {
	AppNotificationsEnabled bool    `json:"app_notifications_enabled"`
	EmailAlertsEnabled      bool    `json:"email_alerts_enabled"`
	WhatsAppAlertsEnabled   bool    `json:"whatsapp_alerts_enabled"`
	ExtraEmail              *string `json:"extra_email,omitempty"`
	DeviceLabel             string  `json:"-"`
	IPMasked                string  `json:"-"`
}

// SecurityAlertPreferencesRecord is the storage model for cuenta_alerta_preferencia.
type SecurityAlertPreferencesRecord struct {
	AccountID               string
	AppNotificationsEnabled bool
	EmailAlertsEnabled      bool
	WhatsAppAlertsEnabled   bool
	ExtraEmail              *string
	ExtraEmailVerifiedAt    *time.Time
	UpdatedAt               time.Time
}

// SecurityAlertEventRecord is the storage model for cuenta_alerta_evento.
type SecurityAlertEventRecord struct {
	ID          string
	AccountID   string
	EventCode   string
	Title       string
	DeviceLabel string
	IPMasked    string
	CreatedAt   time.Time
}

// SecurityAlertsRepository defines persistence for security alerts and channel preferences.
type SecurityAlertsRepository interface {
	GetSecurityAlertPreferences(ctx context.Context, accountID string) (*SecurityAlertPreferencesRecord, error)
	UpsertSecurityAlertPreferences(ctx context.Context, record SecurityAlertPreferencesRecord) error
	ListSecurityAlertEvents(ctx context.Context, accountID string, limit int) ([]SecurityAlertEventRecord, error)
	RecordSecurityAlertEvent(ctx context.Context, event SecurityAlertEventRecord) error
	GetPhoneVerification(ctx context.Context, accountID string) (*AccountPhoneVerification, error)
}

// GetSecurityAlerts returns channel preferences and the last 20 security events for the account.
func (s *AuthenticationService) GetSecurityAlerts(ctx context.Context, accountID string) (SecurityAlertsResponse, error) {
	if accountID == "" {
		return SecurityAlertsResponse{}, ErrNotFound
	}

	primaryEmail, err := s.repo.ActiveAccountEmail(ctx, accountID)
	if err != nil {
		return SecurityAlertsResponse{}, err
	}

	prefRec, err := s.repo.GetSecurityAlertPreferences(ctx, accountID)
	if err != nil {
		return SecurityAlertsResponse{}, err
	}

	appNotifications := true
	emailAlerts := true
	whatsappAlerts := false
	var extraEmail *string
	if prefRec != nil {
		appNotifications = prefRec.AppNotificationsEnabled
		emailAlerts = prefRec.EmailAlertsEnabled
		whatsappAlerts = prefRec.WhatsAppAlertsEnabled
		extraEmail = prefRec.ExtraEmail
	}

	maskedExtraEmail := ""
	extraEmailConfigured := false
	if extraEmail != nil && strings.TrimSpace(*extraEmail) != "" {
		extraEmailConfigured = true
		maskedExtraEmail = MaskEmail(*extraEmail)
	}

	maskedPhone := ""
	phoneRec, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err == nil && phoneRec != nil && phoneRec.VerifiedAt != nil && phoneRec.PhoneE164 != "" {
		maskedPhone = MaskPhone(phoneRec.PhoneE164)
	}

	prefs := SecurityAlertPreferences{
		AppNotificationsEnabled: appNotifications,
		EmailAlertsEnabled:      emailAlerts,
		WhatsAppAlertsEnabled:   whatsappAlerts,
		MaskedEmail:             MaskEmail(primaryEmail),
		MaskedExtraEmail:        maskedExtraEmail,
		ExtraEmailConfigured:    extraEmailConfigured,
		MaskedPhone:             maskedPhone,
	}

	eventsRec, err := s.repo.ListSecurityAlertEvents(ctx, accountID, 20)
	if err != nil {
		return SecurityAlertsResponse{}, err
	}

	alerts := make([]SecurityAlertEvent, 0, len(eventsRec))
	for _, ev := range eventsRec {
		label := ev.DeviceLabel
		if label == "" {
			label = "Dispositivo desconocido"
		}
		ip := ev.IPMasked
		if ip != "" && !strings.Contains(ip, "••") {
			ip = MaskIP(ip)
		}
		alerts = append(alerts, SecurityAlertEvent{
			ID:          ev.ID,
			EventCode:   ev.EventCode,
			Title:       ev.Title,
			DeviceLabel: label,
			IPMasked:    ip,
			CreatedAt:   ev.CreatedAt.UTC(),
		})
	}

	return SecurityAlertsResponse{
		Preferences: prefs,
		Alerts:      alerts,
	}, nil
}

// UpdateSecurityAlertChannels updates preferences and optionally extra_email, validating and auditing.
func (s *AuthenticationService) UpdateSecurityAlertChannels(ctx context.Context, accountID string, input UpdateSecurityAlertChannelsInput) (SecurityAlertPreferences, error) {
	if accountID == "" {
		return SecurityAlertPreferences{}, ErrNotFound
	}

	primaryEmail, err := s.repo.ActiveAccountEmail(ctx, accountID)
	if err != nil {
		return SecurityAlertPreferences{}, err
	}

	var extraEmailToSave *string
	var extraEmailVerifiedAt *time.Time

	if input.ExtraEmail != nil {
		trimmed := strings.TrimSpace(*input.ExtraEmail)
		if trimmed == "" {
			extraEmailToSave = nil
			extraEmailVerifiedAt = nil
		} else {
			if err := ValidateEmail(trimmed); err != nil {
				return SecurityAlertPreferences{}, ErrInvalidExtraEmail
			}
			if strings.EqualFold(trimmed, strings.TrimSpace(primaryEmail)) {
				return SecurityAlertPreferences{}, ErrExtraEmailMatchesPrimary
			}
			extraEmailToSave = &trimmed
		}
	} else {
		existing, err := s.repo.GetSecurityAlertPreferences(ctx, accountID)
		if err != nil {
			return SecurityAlertPreferences{}, err
		}
		if existing != nil {
			extraEmailToSave = existing.ExtraEmail
			extraEmailVerifiedAt = existing.ExtraEmailVerifiedAt
		}
	}

	now := s.now().UTC()
	record := SecurityAlertPreferencesRecord{
		AccountID:               accountID,
		AppNotificationsEnabled: input.AppNotificationsEnabled,
		EmailAlertsEnabled:      input.EmailAlertsEnabled,
		WhatsAppAlertsEnabled:   input.WhatsAppAlertsEnabled,
		ExtraEmail:              extraEmailToSave,
		ExtraEmailVerifiedAt:    extraEmailVerifiedAt,
		UpdatedAt:               now,
	}

	if err := s.repo.UpsertSecurityAlertPreferences(ctx, record); err != nil {
		return SecurityAlertPreferences{}, err
	}

	deviceLabel := input.DeviceLabel
	if deviceLabel == "" {
		deviceLabel = "Dispositivo desconocido"
	}
	ipMasked := input.IPMasked
	if ipMasked != "" && !strings.Contains(ipMasked, "••") {
		ipMasked = MaskIP(ipMasked)
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "alert_channel_updated", "Canales de alerta actualizados", deviceLabel, ipMasked)

	maskedExtraEmail := ""
	extraEmailConfigured := false
	if extraEmailToSave != nil && *extraEmailToSave != "" {
		extraEmailConfigured = true
		maskedExtraEmail = MaskEmail(*extraEmailToSave)
	}

	maskedPhone := ""
	phoneRec, _ := s.repo.GetPhoneVerification(ctx, accountID)
	if phoneRec != nil && phoneRec.VerifiedAt != nil && phoneRec.PhoneE164 != "" {
		maskedPhone = MaskPhone(phoneRec.PhoneE164)
	}

	safelog.Info(ctx, "security alert channels updated",
		safelog.F("account_id", accountID),
		safelog.F("app_notifications", input.AppNotificationsEnabled),
		safelog.F("email_alerts", input.EmailAlertsEnabled),
		safelog.F("whatsapp_alerts", input.WhatsAppAlertsEnabled),
		safelog.F("extra_email_configured", extraEmailConfigured),
	)

	return SecurityAlertPreferences{
		AppNotificationsEnabled: input.AppNotificationsEnabled,
		EmailAlertsEnabled:      input.EmailAlertsEnabled,
		WhatsAppAlertsEnabled:   input.WhatsAppAlertsEnabled,
		MaskedEmail:             MaskEmail(primaryEmail),
		MaskedExtraEmail:        maskedExtraEmail,
		ExtraEmailConfigured:    extraEmailConfigured,
		MaskedPhone:             maskedPhone,
	}, nil
}

// RecordSecurityAlertEvent records an audit event in cuenta_alerta_evento.
func (s *AuthenticationService) RecordSecurityAlertEvent(ctx context.Context, accountID, eventCode, title, deviceLabel, ipMasked string) error {
	if accountID == "" || eventCode == "" || title == "" {
		return ErrInvalid
	}
	eventID, err := s.credentials.ID()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if deviceLabel == "" {
		deviceLabel = "Dispositivo desconocido"
	}
	if ipMasked != "" && !strings.Contains(ipMasked, "••") {
		ipMasked = MaskIP(ipMasked)
	}
	rec := SecurityAlertEventRecord{
		ID:          eventID,
		AccountID:   accountID,
		EventCode:   eventCode,
		Title:       title,
		DeviceLabel: deviceLabel,
		IPMasked:    ipMasked,
		CreatedAt:   now,
	}
	return s.repo.RecordSecurityAlertEvent(ctx, rec)
}
