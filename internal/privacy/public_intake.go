package privacy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const publicIntakeNeutralNotice = "Solicitud registrada para evaluación de privacidad. Este acuse no confirma la existencia de una cuenta ni implica la procedencia del requerimiento. Conserva tu folio y código de seguimiento para consultar el estado."

var publicFolioPattern = regexp.MustCompile(`^PRIV-[0-9A-Z-]{6,32}$`)

type PublicRightsRequestInput struct {
	RightType              string `json:"right_type"`
	RequesterType          string `json:"requester_type"`
	RequesterName          string `json:"requester_name"`
	RequesterEmail         string `json:"requester_email"`
	RepresentativeName     string `json:"representative_name,omitempty"`
	RepresentativeRelation string `json:"representative_relation,omitempty"`
	SubjectReference       string `json:"subject_reference,omitempty"`
	Detail                 string `json:"detail"`
}

type PublicRightsReceipt struct {
	ID            string    `json:"id"`
	Folio         string    `json:"folio"`
	TrackingCode  string    `json:"tracking_code"`
	RightType     string    `json:"right_type"`
	RequesterType string    `json:"requester_type"`
	State         string    `json:"state"`
	Channel       string    `json:"channel"`
	DueAt         time.Time `json:"due_at"`
	CreatedAt     time.Time `json:"created_at"`
	Notice        string    `json:"notice"`
}

type PublicRightsLookupInput struct {
	Folio        string `json:"folio"`
	TrackingCode string `json:"tracking_code"`
}

type PublicRightsStatus struct {
	Folio           string               `json:"folio"`
	RightType       string               `json:"right_type"`
	RequesterType   string               `json:"requester_type"`
	State           string               `json:"state"`
	CreatedAt       time.Time            `json:"created_at"`
	DueAt           *time.Time           `json:"due_at,omitempty"`
	ExtendedDueAt   *time.Time           `json:"extended_due_at,omitempty"`
	ExtensionReason *string              `json:"extension_reason,omitempty"`
	ResolvedAt      *time.Time           `json:"resolved_at,omitempty"`
	Response        *string              `json:"response,omitempty"`
	Events          []RightsRequestEvent `json:"events,omitempty"`
}

// PublicIntakeRepository persists and queries unauthenticated ARCO+ requests
// using an opaque public folio and a SHA-256 hash of the tracking code.
type PublicIntakeRepository interface {
	CreatePublicRightsRequest(ctx context.Context, in PublicRightsRequestInput, folio string, trackingHash []byte, now, dueAt time.Time) (PublicRightsReceipt, error)
	LookupPublicRightsRequest(ctx context.Context, folio string, trackingHash []byte) (PublicRightsStatus, error)
}

func (s *Service) clockNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

// GeneratePublicFolioAt produces a unique public folio formatted as PRIV-YYYY-XXXXXXXX.
func GeneratePublicFolioAt(now time.Time) (string, error) {
	var raw [4]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("PRIV-%04d-%s", now.UTC().Year(), strings.ToUpper(hex.EncodeToString(raw[:]))), nil
}

// FormatAcuseRecepcionNotice builds a neutral receipt email body and subject
// without confirming account existence.
func FormatAcuseRecepcionNotice(folio, rightType string, dueAt time.Time) (string, string) {
	dueLocal := dueAt.In(SantiagoLocation()).Format("02/01/2006 15:04")
	subject := fmt.Sprintf("Acuse de recepción de solicitud de privacidad (%s)", folio)
	body := fmt.Sprintf(
		"Registramos tu solicitud de privacidad con folio %s para el derecho de %s.\n"+
			"Plazo estimado de respuesta: %s (America/Santiago).\n"+
			"Este acuse no confirma la existencia de una cuenta ni implica la procedencia del requerimiento.\n"+
			"Puedes consultar el seguimiento de tu caso en /privacidad/solicitudes.",
		folio, rightType, dueLocal,
	)
	return subject, body
}

func generatePublicTrackingCode() (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw[:])), nil
}

func isValidRightType(kind string) bool {
	switch kind {
	case "acceso", "rectificacion", "supresion", "oposicion", "portabilidad", "bloqueo":
		return true
	default:
		return false
	}
}

func validateBoundedText(value string, minRunes, maxRunes int) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, 0) {
		return "", false
	}
	n := utf8.RuneCountInString(trimmed)
	if n < minRunes || n > maxRunes {
		return "", false
	}
	return trimmed, true
}

func validatePublicEmail(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !utf8.ValidString(trimmed) || strings.ContainsRune(trimmed, 0) || strings.ContainsAny(trimmed, " \t\r\n<>") {
		return "", false
	}
	n := utf8.RuneCountInString(trimmed)
	if n < 5 || n > 254 {
		return "", false
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Name != "" || parsed.Address != trimmed || strings.Count(trimmed, "@") != 1 {
		return "", false
	}
	at := strings.IndexByte(trimmed, '@')
	domain := trimmed[at+1:]
	if at <= 0 || domain == "" || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return trimmed, true
}

func (s *Service) SubmitPublicRightsRequest(ctx context.Context, in PublicRightsRequestInput) (PublicRightsReceipt, error) {
	if s == nil || s.repo == nil || ctx == nil {
		return PublicRightsReceipt{}, ErrInvalid
	}
	if !isValidRightType(in.RightType) {
		return PublicRightsReceipt{}, ErrInvalid
	}
	if in.RequesterType != "titular" && in.RequesterType != "representante" {
		return PublicRightsReceipt{}, ErrInvalid
	}
	requesterName, ok := validateBoundedText(in.RequesterName, 2, 160)
	if !ok {
		return PublicRightsReceipt{}, ErrInvalid
	}
	requesterEmail, ok := validatePublicEmail(in.RequesterEmail)
	if !ok {
		return PublicRightsReceipt{}, ErrInvalid
	}
	detail, ok := validateBoundedText(in.Detail, 10, 1000)
	if !ok {
		return PublicRightsReceipt{}, ErrInvalid
	}
	repName := strings.TrimSpace(in.RepresentativeName)
	repRel := strings.TrimSpace(in.RepresentativeRelation)
	if in.RequesterType == "titular" {
		if repName != "" || repRel != "" {
			return PublicRightsReceipt{}, ErrInvalid
		}
	} else {
		var validRep bool
		repName, validRep = validateBoundedText(repName, 2, 160)
		if !validRep {
			return PublicRightsReceipt{}, ErrInvalid
		}
		if repRel != "mandatario" && repRel != "representante_legal" && repRel != "tutor_nna" {
			return PublicRightsReceipt{}, ErrInvalid
		}
	}
	subjectRef := strings.TrimSpace(in.SubjectReference)
	if subjectRef != "" {
		var validRef bool
		subjectRef, validRef = validateBoundedText(subjectRef, 2, 200)
		if !validRef {
			return PublicRightsReceipt{}, ErrInvalid
		}
	}

	repo, ok := s.repo.(PublicIntakeRepository)
	if !ok {
		return PublicRightsReceipt{}, errors.New("privacy: public intake repository unavailable")
	}

	now := s.clockNow().UTC().Truncate(time.Microsecond)
	folio, err := GeneratePublicFolioAt(now)
	if err != nil {
		return PublicRightsReceipt{}, err
	}
	trackingCode, err := generatePublicTrackingCode()
	if err != nil {
		return PublicRightsReceipt{}, err
	}
	hash := sha256.Sum256([]byte(trackingCode))
	dueAt := ComputeInitialDeadline(now, in.RightType)

	normalized := PublicRightsRequestInput{
		RightType:              in.RightType,
		RequesterType:          in.RequesterType,
		RequesterName:          requesterName,
		RequesterEmail:         requesterEmail,
		RepresentativeName:     repName,
		RepresentativeRelation: repRel,
		SubjectReference:       subjectRef,
		Detail:                 detail,
	}
	receipt, err := repo.CreatePublicRightsRequest(ctx, normalized, folio, hash[:], now, dueAt)
	if err != nil {
		return PublicRightsReceipt{}, err
	}
	if receipt.Folio == "" {
		receipt.Folio = folio
	}
	receipt.TrackingCode = trackingCode
	if receipt.Notice == "" {
		receipt.Notice = publicIntakeNeutralNotice
	}
	return receipt, nil
}

func (s *Service) LookupPublicRightsRequest(ctx context.Context, in PublicRightsLookupInput) (PublicRightsStatus, error) {
	if s == nil || s.repo == nil || ctx == nil {
		return PublicRightsStatus{}, ErrInvalid
	}
	folio := strings.ToUpper(strings.TrimSpace(in.Folio))
	if !publicFolioPattern.MatchString(folio) {
		return PublicRightsStatus{}, ErrInvalid
	}
	trackingCode := strings.TrimSpace(in.TrackingCode)
	if !utf8.ValidString(trackingCode) || len(trackingCode) < 16 || len(trackingCode) > 128 ||
		strings.ContainsRune(trackingCode, 0) || strings.ContainsAny(trackingCode, " \t\r\n") {
		return PublicRightsStatus{}, ErrInvalid
	}

	repo, ok := s.repo.(PublicIntakeRepository)
	if !ok {
		return PublicRightsStatus{}, errors.New("privacy: public intake repository unavailable")
	}
	hash := sha256.Sum256([]byte(trackingCode))
	return repo.LookupPublicRightsRequest(ctx, folio, hash[:])
}
