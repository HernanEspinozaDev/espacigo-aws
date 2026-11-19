package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"
)

var (
	ErrQRTicketNotFound   = errors.New("ticket de verificación qr no encontrado")
	ErrQRTicketExpired    = errors.New("el ticket de verificación qr ha expirado")
	ErrQRTicketResolved   = errors.New("el ticket de verificación qr ya fue resuelto")
	ErrQRTicketInvalid    = errors.New("token de verificación qr inválido")
	ErrQRDeviceNotEnabled = errors.New("el dispositivo qr no se encuentra activado")
)

const (
	qrTicketTTLSeconds = 120
)

// AccountQRStatusResponse is the payload returned by GET /api/v1/account/security/qr.
type AccountQRStatusResponse struct {
	Enabled            bool       `json:"enabled"`
	EnabledAt          *time.Time `json:"enabled_at,omitempty"`
	TrustedDeviceLabel *string    `json:"trusted_device_label,omitempty"`
}

// AccountQRDisableResponse is returned when the QR trusted device is disabled.
type AccountQRDisableResponse struct {
	Enabled bool `json:"enabled"`
}

// CreateQRTicketResponse is the payload returned by POST /api/v1/account/security/qr/ticket.
type CreateQRTicketResponse struct {
	TicketID  string    `json:"ticket_id"`
	QRPayload string    `json:"qr_payload"`
	ExpiresAt time.Time `json:"expires_at"`
}

// QRTicketStatusResponse is the payload returned by GET /api/v1/account/security/qr/ticket/{ticket_id}.
type QRTicketStatusResponse struct {
	TicketID   string     `json:"ticket_id"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// ApproveQRTicketInput is the payload sent to POST /api/v1/account/security/qr/approve.
type ApproveQRTicketInput struct {
	Token       string `json:"token"`
	DeviceLabel string `json:"device_label,omitempty"`
}

// ApproveQRTicketResponse is returned when a QR ticket is successfully approved.
type ApproveQRTicketResponse struct {
	Status     string    `json:"status"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// AccountQRDeviceRecord represents the storage entity in public.cuenta_qr_dispositivo.
type AccountQRDeviceRecord struct {
	AccountID          string
	EnabledAt          *time.Time
	TrustedDeviceLabel *string
	UpdatedAt          time.Time
}

// AccountQRTicketRecord represents the storage entity in public.cuenta_qr_ticket.
type AccountQRTicketRecord struct {
	ID          string
	AccountID   string
	TokenHash   string
	Status      string
	DeviceLabel string
	IPMasked    string
	ExpiresAt   time.Time
	ResolvedAt  *time.Time
	CreatedAt   time.Time
}

// QRRepository defines persistence operations for QR device verification and tickets.
type QRRepository interface {
	GetQRDevice(ctx context.Context, accountID string) (*AccountQRDeviceRecord, error)
	UpsertQRDevice(ctx context.Context, accountID string, enabledAt time.Time, trustedDeviceLabel string) error
	DisableQRDevice(ctx context.Context, accountID string, updatedAt time.Time) error
	SaveQRTicket(ctx context.Context, ticket AccountQRTicketRecord) error
	GetQRTicketByID(ctx context.Context, ticketID string) (*AccountQRTicketRecord, error)
	GetQRTicketByTokenHash(ctx context.Context, tokenHash string) (*AccountQRTicketRecord, error)
	ApproveQRTicket(ctx context.Context, accountID, ticketID, deviceLabel string, resolvedAt time.Time) error
	ExpireQRTicket(ctx context.Context, ticketID string) error
}

// GetQRStatus returns the current QR trusted device configuration status for the account.
func (s *AuthenticationService) GetQRStatus(ctx context.Context, accountID string) (AccountQRStatusResponse, error) {
	if accountID == "" {
		return AccountQRStatusResponse{}, ErrNotFound
	}

	rec, err := s.repo.GetQRDevice(ctx, accountID)
	if err != nil {
		return AccountQRStatusResponse{}, err
	}
	if rec == nil || rec.EnabledAt == nil {
		return AccountQRStatusResponse{
			Enabled: false,
		}, nil
	}

	return AccountQRStatusResponse{
		Enabled:            true,
		EnabledAt:          rec.EnabledAt,
		TrustedDeviceLabel: rec.TrustedDeviceLabel,
	}, nil
}

// CreateQRTicket creates an ephemeral QR verification challenge with a 120-second TTL.
func (s *AuthenticationService) CreateQRTicket(ctx context.Context, accountID, deviceLabel, ipMasked string) (CreateQRTicketResponse, error) {
	if accountID == "" {
		return CreateQRTicketResponse{}, ErrNotFound
	}

	var ticketID string
	var err error
	if s.credentials != nil {
		ticketID, err = s.credentials.ID()
	}
	if ticketID == "" || err != nil {
		ticketID, err = newUUIDFallback()
	}
	if err != nil {
		return CreateQRTicketResponse{}, err
	}

	var tokenBytes [32]byte
	if _, err := io.ReadFull(rand.Reader, tokenBytes[:]); err != nil {
		return CreateQRTicketResponse{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	h := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(h[:])

	now := s.now()
	expiresAt := now.Add(qrTicketTTLSeconds * time.Second)

	ticket := AccountQRTicketRecord{
		ID:          ticketID,
		AccountID:   accountID,
		TokenHash:   tokenHash,
		Status:      "pending",
		DeviceLabel: deviceLabel,
		IPMasked:    ipMasked,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}

	if err := s.repo.SaveQRTicket(ctx, ticket); err != nil {
		return CreateQRTicketResponse{}, err
	}

	return CreateQRTicketResponse{
		TicketID:  ticketID,
		QRPayload: token,
		ExpiresAt: expiresAt,
	}, nil
}

// GetQRTicketStatus checks the resolution state of a QR verification ticket.
func (s *AuthenticationService) GetQRTicketStatus(ctx context.Context, accountID, ticketID string) (QRTicketStatusResponse, error) {
	if accountID == "" || ticketID == "" {
		return QRTicketStatusResponse{}, ErrNotFound
	}

	ticket, err := s.repo.GetQRTicketByID(ctx, ticketID)
	if err != nil {
		return QRTicketStatusResponse{}, err
	}
	if ticket == nil || ticket.AccountID != accountID {
		return QRTicketStatusResponse{}, ErrQRTicketNotFound
	}

	status := ticket.Status
	if status == "pending" && s.now().After(ticket.ExpiresAt) {
		status = "expired"
		_ = s.repo.ExpireQRTicket(ctx, ticket.ID)
	}

	return QRTicketStatusResponse{
		TicketID:   ticket.ID,
		Status:     status,
		ExpiresAt:  ticket.ExpiresAt,
		ResolvedAt: ticket.ResolvedAt,
	}, nil
}

// ApproveQRTicket validates the token hash against pending tickets and marks the ticket approved.
func (s *AuthenticationService) ApproveQRTicket(ctx context.Context, accountID, token, deviceLabel, ipMasked string) (ApproveQRTicketResponse, error) {
	if accountID == "" {
		return ApproveQRTicketResponse{}, ErrNotFound
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ApproveQRTicketResponse{}, ErrQRTicketInvalid
	}

	h := sha256.Sum256([]byte(trimmed))
	tokenHash := hex.EncodeToString(h[:])

	ticket, err := s.repo.GetQRTicketByTokenHash(ctx, tokenHash)
	if err != nil {
		return ApproveQRTicketResponse{}, err
	}
	if ticket == nil || ticket.AccountID != accountID {
		return ApproveQRTicketResponse{}, ErrQRTicketNotFound
	}

	now := s.now()
	if ticket.Status == "expired" || now.After(ticket.ExpiresAt) {
		_ = s.repo.ExpireQRTicket(ctx, ticket.ID)
		return ApproveQRTicketResponse{}, ErrQRTicketExpired
	}

	if ticket.Status != "pending" || ticket.ResolvedAt != nil {
		return ApproveQRTicketResponse{}, ErrQRTicketResolved
	}

	if err := s.repo.ApproveQRTicket(ctx, accountID, ticket.ID, deviceLabel, now); err != nil {
		return ApproveQRTicketResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "qr_device_verified", "Dispositivo QR verificado", deviceLabel, ipMasked)

	return ApproveQRTicketResponse{
		Status:     "approved",
		ResolvedAt: now,
	}, nil
}

// DisableQR disables the QR trusted device for the account.
func (s *AuthenticationService) DisableQR(ctx context.Context, accountID, deviceLabel, ipMasked string) (AccountQRDisableResponse, error) {
	if accountID == "" {
		return AccountQRDisableResponse{}, ErrNotFound
	}

	rec, err := s.repo.GetQRDevice(ctx, accountID)
	if err != nil {
		return AccountQRDisableResponse{}, err
	}
	if rec == nil || rec.EnabledAt == nil {
		return AccountQRDisableResponse{}, ErrQRDeviceNotEnabled
	}

	now := s.now()
	if err := s.repo.DisableQRDevice(ctx, accountID, now); err != nil {
		return AccountQRDisableResponse{}, err
	}

	_ = s.RecordSecurityAlertEvent(ctx, accountID, "qr_device_disabled", "Dispositivo QR desvinculado", deviceLabel, ipMasked)

	return AccountQRDisableResponse{
		Enabled: false,
	}, nil
}

func newUUIDFallback() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16]), nil
}
