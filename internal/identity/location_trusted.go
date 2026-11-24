package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrTrustedContactLimit       = errors.New("se ha alcanzado el límite máximo de 3 personas de confianza")
	ErrInvalidTrustedContactName = errors.New("el nombre debe tener entre 2 y 80 caracteres")
	ErrInvalidRelationship       = errors.New("el parentesco debe tener entre 2 y 40 caracteres")
	ErrInvalidRegionLabel        = errors.New("la etiqueta de región no debe exceder 80 caracteres")
	ErrCoordinatesNotAllowed     = errors.New("no se permiten coordenadas GPS exactas; use una etiqueta de región")
	ErrTrustedContactNotFound    = errors.New("persona de confianza no encontrada")
	ErrInvalidEmail              = errors.New("formato de correo electrónico inválido")
)

var (
	gpsCoordRegex   = regexp.MustCompile(`(?i)^\s*[-+]?([0-9]{1,3}\.[0-9]+)\s*,\s*[-+]?([0-9]{1,3}\.[0-9]+)\s*$`)
	gpsKeywordRegex = regexp.MustCompile(`(?i)(lat(itude)?|lon(gitude)?|gps)\s*[:=]\s*[-+]?[0-9]`)
)

// AccountLocationPermissionResponse is returned by GET and PUT /api/v1/account/security/location.
type AccountLocationPermissionResponse struct {
	Enabled          bool       `json:"enabled"`
	ConsentUpdatedAt *time.Time `json:"consent_updated_at,omitempty"`
	LastRegionLabel  *string    `json:"last_region_label,omitempty"`
}

// UpdateLocationPermissionInput is the request body for PUT /api/v1/account/security/location.
type UpdateLocationPermissionInput struct {
	Enabled     bool    `json:"enabled"`
	RegionLabel *string `json:"region_label,omitempty"`
}

// LocationPermissionRecord is the storage model for public.cuenta_permiso_ubicacion.
type LocationPermissionRecord struct {
	AccountID        string
	Enabled          bool
	ConsentUpdatedAt time.Time
	LastRegionLabel  *string
}

// TrustedContact represents an active trusted contact in API responses.
type TrustedContact struct {
	ID           string    `json:"id"`
	FullName     string    `json:"full_name"`
	MaskedEmail  string    `json:"masked_email"`
	MaskedPhone  string    `json:"masked_phone,omitempty"`
	Relationship string    `json:"relationship"`
	CreatedAt    time.Time `json:"created_at"`
}

// TrustedContactsListResponse is returned by GET /api/v1/account/security/trusted-contacts.
type TrustedContactsListResponse struct {
	Contacts []TrustedContact `json:"contacts"`
}

// AddTrustedContactInput is the request body for POST /api/v1/account/security/trusted-contacts.
type AddTrustedContactInput struct {
	FullName     string  `json:"full_name"`
	Email        string  `json:"email"`
	Phone        *string `json:"phone,omitempty"`
	Relationship string  `json:"relationship"`
}

// RemoveTrustedContactResponse is returned by DELETE /api/v1/account/security/trusted-contacts/{contact_id}.
type RemoveTrustedContactResponse struct {
	Status    string `json:"status"`
	ContactID string `json:"contact_id"`
}

// TrustedContactRecord is the storage model for public.cuenta_persona_confianza.
type TrustedContactRecord struct {
	ID           string
	AccountID    string
	FullName     string
	Email        string
	PhoneE164    *string
	Relationship string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SecurityMethodsSummaryResponse is returned by GET /api/v1/account/security/methods.
type SecurityMethodsSummaryResponse struct {
	Protected   bool                   `json:"protected"`
	ActiveCount int                    `json:"active_count"`
	TotalCount  int                    `json:"total_count"`
	Methods     SecurityMethodsDetails `json:"methods"`
}

// SecurityMethodsDetails holds the status of all 7 verification methods.
type SecurityMethodsDetails struct {
	Password  PasswordMethodStatus  `json:"password"`
	Email     EmailMethodStatus     `json:"email"`
	Phone     PhoneMethodStatus     `json:"phone"`
	TOTP      TOTPMethodStatus      `json:"totp"`
	Passkeys  PasskeysMethodStatus  `json:"passkeys"`
	QR        QRMethodStatus        `json:"qr"`
	FacialKYC FacialKYCMethodStatus `json:"facial_kyc"`
}

type PasswordMethodStatus struct {
	Enabled bool `json:"enabled"`
}

type EmailMethodStatus struct {
	Enabled     bool   `json:"enabled"`
	MaskedEmail string `json:"masked_email,omitempty"`
}

type PhoneMethodStatus struct {
	Enabled     bool   `json:"enabled"`
	MaskedPhone string `json:"masked_phone,omitempty"`
}

type TOTPMethodStatus struct {
	Enabled bool `json:"enabled"`
}

type PasskeysMethodStatus struct {
	Enabled bool `json:"enabled"`
	Count   int  `json:"count"`
}

type QRMethodStatus struct {
	Enabled bool `json:"enabled"`
}

type FacialKYCMethodStatus struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

// LocationTrustedRepository defines persistence operations for location consent and trusted contacts.
type LocationTrustedRepository interface {
	GetLocationPermission(ctx context.Context, accountID string) (*LocationPermissionRecord, error)
	UpsertLocationPermission(ctx context.Context, record LocationPermissionRecord) error
	ListActiveTrustedContacts(ctx context.Context, accountID string) ([]TrustedContactRecord, error)
	CountActiveTrustedContacts(ctx context.Context, accountID string) (int, error)
	GetTrustedContact(ctx context.Context, accountID, contactID string) (*TrustedContactRecord, error)
	InsertTrustedContact(ctx context.Context, contact TrustedContactRecord) error
	RevokeTrustedContact(ctx context.Context, accountID, contactID string, updatedAt time.Time) error
}

func sanitizeRegionLabel(raw string) (*string, error) {
	if gpsCoordRegex.MatchString(raw) || gpsKeywordRegex.MatchString(raw) {
		return nil, ErrCoordinatesNotAllowed
	}
	var sb strings.Builder
	for _, r := range raw {
		if r == '\r' || r == '\n' {
			continue
		}
		if r < 32 && r != '\t' {
			continue
		}
		sb.WriteRune(r)
	}
	cleaned := strings.TrimSpace(sb.String())
	if cleaned == "" {
		return nil, nil
	}
	runes := []rune(cleaned)
	if len(runes) > 80 {
		return nil, ErrInvalidRegionLabel
	}
	res := string(runes)
	return &res, nil
}

// GetLocationPermission returns location consent status for the given account.
func (s *AuthenticationService) GetLocationPermission(ctx context.Context, accountID string) (AccountLocationPermissionResponse, error) {
	if accountID == "" {
		return AccountLocationPermissionResponse{}, ErrNotFound
	}
	rec, err := s.repo.GetLocationPermission(ctx, accountID)
	if err != nil {
		return AccountLocationPermissionResponse{}, err
	}
	if rec == nil {
		return AccountLocationPermissionResponse{
			Enabled: false,
		}, nil
	}
	consentAt := rec.ConsentUpdatedAt.UTC()
	return AccountLocationPermissionResponse{
		Enabled:          rec.Enabled,
		ConsentUpdatedAt: &consentAt,
		LastRegionLabel:  rec.LastRegionLabel,
	}, nil
}

// UpdateLocationPermission updates location consent and records a security audit event.
func (s *AuthenticationService) UpdateLocationPermission(ctx context.Context, accountID string, enabled bool, regionLabel *string, deviceLabel, ipMasked string) (AccountLocationPermissionResponse, error) {
	if accountID == "" {
		return AccountLocationPermissionResponse{}, ErrNotFound
	}
	var sanitizedRegion *string
	if regionLabel != nil && strings.TrimSpace(*regionLabel) != "" {
		cleaned, err := sanitizeRegionLabel(*regionLabel)
		if err != nil {
			return AccountLocationPermissionResponse{}, err
		}
		sanitizedRegion = cleaned
	}

	now := s.now().UTC()
	rec := LocationPermissionRecord{
		AccountID:        accountID,
		Enabled:          enabled,
		ConsentUpdatedAt: now,
		LastRegionLabel:  sanitizedRegion,
	}
	if err := s.repo.UpsertLocationPermission(ctx, rec); err != nil {
		return AccountLocationPermissionResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "location_permission_updated", "Permiso de ubicación actualizado", deviceLabel, ipMasked)

	return AccountLocationPermissionResponse{
		Enabled:          enabled,
		ConsentUpdatedAt: &now,
		LastRegionLabel:  sanitizedRegion,
	}, nil
}

// ListTrustedContacts returns all active trusted contacts for the account with masked PII.
func (s *AuthenticationService) ListTrustedContacts(ctx context.Context, accountID string) (TrustedContactsListResponse, error) {
	if accountID == "" {
		return TrustedContactsListResponse{}, ErrNotFound
	}
	records, err := s.repo.ListActiveTrustedContacts(ctx, accountID)
	if err != nil {
		return TrustedContactsListResponse{}, err
	}
	contacts := make([]TrustedContact, 0, len(records))
	for _, rec := range records {
		maskedPhone := ""
		if rec.PhoneE164 != nil && *rec.PhoneE164 != "" {
			maskedPhone = MaskPhone(*rec.PhoneE164)
		}
		contacts = append(contacts, TrustedContact{
			ID:           rec.ID,
			FullName:     rec.FullName,
			MaskedEmail:  MaskEmail(rec.Email),
			MaskedPhone:  maskedPhone,
			Relationship: rec.Relationship,
			CreatedAt:    rec.CreatedAt.UTC(),
		})
	}
	return TrustedContactsListResponse{Contacts: contacts}, nil
}

// AddTrustedContact validates, inserts a new active trusted contact, and records a security audit event.
func (s *AuthenticationService) AddTrustedContact(ctx context.Context, accountID string, input AddTrustedContactInput, deviceLabel, ipMasked string) (TrustedContact, error) {
	if accountID == "" {
		return TrustedContact{}, ErrNotFound
	}
	fullName := strings.TrimSpace(input.FullName)
	nameRunes := []rune(fullName)
	if len(nameRunes) < 2 || len(nameRunes) > 80 {
		return TrustedContact{}, ErrInvalidTrustedContactName
	}

	email := NormalizeEmail(input.Email)
	if err := ValidateEmail(email); err != nil {
		return TrustedContact{}, ErrInvalidEmail
	}

	relationship := strings.TrimSpace(input.Relationship)
	relRunes := []rune(relationship)
	if len(relRunes) < 2 || len(relRunes) > 40 {
		return TrustedContact{}, ErrInvalidRelationship
	}

	var phoneE164 *string
	if input.Phone != nil && strings.TrimSpace(*input.Phone) != "" {
		normalized, err := ValidateAndNormalizeChileanPhone(*input.Phone)
		if err != nil {
			return TrustedContact{}, err
		}
		phoneE164 = &normalized
	}

	count, err := s.repo.CountActiveTrustedContacts(ctx, accountID)
	if err != nil {
		return TrustedContact{}, err
	}
	if count >= 3 {
		return TrustedContact{}, ErrTrustedContactLimit
	}

	contactID, err := s.credentials.ID()
	if err != nil {
		return TrustedContact{}, err
	}
	now := s.now().UTC()
	rec := TrustedContactRecord{
		ID:           contactID,
		AccountID:    accountID,
		FullName:     fullName,
		Email:        email,
		PhoneE164:    phoneE164,
		Relationship: relationship,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.InsertTrustedContact(ctx, rec); err != nil {
		return TrustedContact{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "trusted_contact_added", "Persona de confianza agregada", deviceLabel, ipMasked)

	maskedPhone := ""
	if phoneE164 != nil && *phoneE164 != "" {
		maskedPhone = MaskPhone(*phoneE164)
	}

	return TrustedContact{
		ID:           rec.ID,
		FullName:     rec.FullName,
		MaskedEmail:  MaskEmail(rec.Email),
		MaskedPhone:  maskedPhone,
		Relationship: rec.Relationship,
		CreatedAt:    rec.CreatedAt,
	}, nil
}

// RemoveTrustedContact marks an active trusted contact as revoked and records a security audit event.
func (s *AuthenticationService) RemoveTrustedContact(ctx context.Context, accountID, contactID, deviceLabel, ipMasked string) (RemoveTrustedContactResponse, error) {
	if accountID == "" || contactID == "" {
		return RemoveTrustedContactResponse{}, ErrNotFound
	}
	contact, err := s.repo.GetTrustedContact(ctx, accountID, contactID)
	if err != nil {
		return RemoveTrustedContactResponse{}, err
	}
	if contact == nil || contact.Status != "active" {
		return RemoveTrustedContactResponse{}, ErrNotFound
	}

	now := s.now().UTC()
	if err := s.repo.RevokeTrustedContact(ctx, accountID, contactID, now); err != nil {
		return RemoveTrustedContactResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "trusted_contact_removed", "Persona de confianza eliminada", deviceLabel, ipMasked)

	return RemoveTrustedContactResponse{
		Status:    "revoked",
		ContactID: contactID,
	}, nil
}

// GetSecurityMethodsSummary reports enrollment status for the available verification methods.
func (s *AuthenticationService) GetSecurityMethodsSummary(ctx context.Context, accountID string) (SecurityMethodsSummaryResponse, error) {
	if accountID == "" {
		return SecurityMethodsSummaryResponse{}, ErrNotFound
	}

	// 1. Password: authenticated users always have an active password
	passwordStatus := PasswordMethodStatus{Enabled: true}

	// 2. Email: verified if active account email exists
	email, err := s.repo.ActiveAccountEmail(ctx, accountID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return SecurityMethodsSummaryResponse{}, err
	}
	emailActive := err == nil && email != ""
	emailStatus := EmailMethodStatus{
		Enabled: emailActive,
	}
	if emailActive {
		emailStatus.MaskedEmail = MaskEmail(email)
	}

	// 3. Phone: verified from cuenta_verificacion_telefono
	phoneRec, err := s.repo.GetPhoneVerification(ctx, accountID)
	if err != nil {
		return SecurityMethodsSummaryResponse{}, err
	}
	phoneActive := phoneRec != nil && phoneRec.VerifiedAt != nil
	phoneStatus := PhoneMethodStatus{
		Enabled: phoneActive,
	}
	if phoneActive && phoneRec.PhoneE164 != "" {
		phoneStatus.MaskedPhone = MaskPhone(phoneRec.PhoneE164)
	}

	// 4. TOTP: enabled from cuenta_totp
	totpRec, _, err := s.repo.GetTOTP(ctx, accountID)
	if err != nil {
		return SecurityMethodsSummaryResponse{}, err
	}
	totpActive := totpRec != nil && totpRec.EnabledAt != nil
	totpStatus := TOTPMethodStatus{
		Enabled: totpActive,
	}

	// 5. Passkeys: registered credentials from cuenta_passkey
	passkeys, err := s.repo.ListPasskeys(ctx, accountID)
	if err != nil {
		return SecurityMethodsSummaryResponse{}, err
	}
	passkeysCount := len(passkeys)
	passkeysActive := passkeysCount > 0
	passkeysStatus := PasskeysMethodStatus{
		Enabled: passkeysActive,
		Count:   passkeysCount,
	}

	// 6. QR Device: paired device from cuenta_qr_dispositivo
	qrRec, err := s.repo.GetQRDevice(ctx, accountID)
	if err != nil {
		return SecurityMethodsSummaryResponse{}, err
	}
	qrActive := qrRec != nil && qrRec.EnabledAt != nil
	qrStatus := QRMethodStatus{
		Enabled: qrActive,
	}

	// 7. Facial KYC: pending module per project constraints
	facialActive := false
	facialStatus := FacialKYCMethodStatus{
		Enabled: facialActive,
		Status:  "pending_kyc_module",
	}

	// Count active methods
	activeCount := 1 // password
	if emailActive {
		activeCount++
	}
	if phoneActive {
		activeCount++
	}
	if totpActive {
		activeCount++
	}
	if passkeysActive {
		activeCount++
	}
	if qrActive {
		activeCount++
	}

	// Protected: password + email + at least 1 additional method
	hasAdditional := phoneActive || totpActive || passkeysActive || qrActive
	protected := emailActive && hasAdditional

	return SecurityMethodsSummaryResponse{
		Protected:   protected,
		ActiveCount: activeCount,
		TotalCount:  6,
		Methods: SecurityMethodsDetails{
			Password:  passwordStatus,
			Email:     emailStatus,
			Phone:     phoneStatus,
			TOTP:      totpStatus,
			Passkeys:  passkeysStatus,
			QR:        qrStatus,
			FacialKYC: facialStatus,
		},
	}, nil
}
