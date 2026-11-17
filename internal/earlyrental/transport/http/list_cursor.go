package earlyrentalhttp

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const earlyRentalCursorAAD = "early-rental-list-cursor-v1"

var (
	ErrInvalidEarlyRentalCursorKey = errors.New("invalid early-rental cursor key")
	ErrInvalidEarlyRentalCursor    = errors.New("invalid early-rental cursor")
)

type EarlyRentalCursorScope string

const (
	EarlyRentalCursorScopeOffers       EarlyRentalCursorScope = "offers"
	EarlyRentalCursorScopeTransactions EarlyRentalCursorScope = "transactions"
)

type EarlyRentalCursorBinding struct {
	Scope   EarlyRentalCursorScope
	ActorID string
	Role    string
	State   string
	Limit   int
}

type EarlyRentalCursorPosition struct {
	CreatedAt time.Time
	ID        string
}

type EarlyRentalCursorCodec struct {
	key [32]byte
}

type earlyRentalCursorPayload struct {
	Version   int                    `json:"v"`
	Scope     EarlyRentalCursorScope `json:"s"`
	ActorID   string                 `json:"a"`
	Role      string                 `json:"r,omitempty"`
	State     string                 `json:"t,omitempty"`
	Limit     int                    `json:"l"`
	CreatedAt string                 `json:"at"`
	ID        string                 `json:"id"`
}

// NewEarlyRentalCursorCodec requires a stable server key supplied by trusted
// local configuration; it deliberately does not generate an instance-local key.
func NewEarlyRentalCursorCodec(key []byte) (*EarlyRentalCursorCodec, error) {
	if len(key) != 32 || bytes.Equal(key, make([]byte, 32)) {
		return nil, ErrInvalidEarlyRentalCursorKey
	}
	codec := &EarlyRentalCursorCodec{}
	copy(codec.key[:], key)
	return codec, nil
}

func (c *EarlyRentalCursorCodec) Encode(binding EarlyRentalCursorBinding, position EarlyRentalCursorPosition) (string, error) {
	if c == nil || !validEarlyRentalCursorBinding(binding) || position.CreatedAt.IsZero() || !validEarlyRentalCursorUUID(position.ID) {
		return "", ErrInvalidEarlyRentalCursor
	}
	payload := earlyRentalCursorPayload{
		Version: 1, Scope: binding.Scope, ActorID: binding.ActorID, Role: binding.Role, State: binding.State,
		Limit: binding.Limit, CreatedAt: position.CreatedAt.UTC().Format(time.RFC3339Nano), ID: position.ID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", ErrInvalidEarlyRentalCursor
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", ErrInvalidEarlyRentalCursor
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrInvalidEarlyRentalCursor
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", ErrInvalidEarlyRentalCursor
	}
	sealed := gcm.Seal(nonce, nonce, raw, []byte(earlyRentalCursorAAD))
	token := "v1." + base64.RawURLEncoding.EncodeToString(sealed)
	if len(token) > maxEarlyRentalCursorRunes {
		return "", ErrInvalidEarlyRentalCursor
	}
	return token, nil
}

func (c *EarlyRentalCursorCodec) Decode(token string, binding EarlyRentalCursorBinding) (EarlyRentalCursorPosition, error) {
	if c == nil || !validEarlyRentalCursorBinding(binding) || len(token) < 4 || len(token) > maxEarlyRentalCursorRunes || !strings.HasPrefix(token, "v1.") {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	encoded := token[3:]
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(sealed) != encoded {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	nonce := sealed[:gcm.NonceSize()]
	raw, err := gcm.Open(nil, nonce, sealed[gcm.NonceSize():], []byte(earlyRentalCursorAAD))
	if err != nil {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload earlyRentalCursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	if payload.Version != 1 || payload.Scope != binding.Scope || payload.ActorID != binding.ActorID || payload.Role != binding.Role ||
		payload.State != binding.State || payload.Limit != binding.Limit || !validEarlyRentalCursorUUID(payload.ID) {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil || createdAt.IsZero() || createdAt.Format(time.RFC3339Nano) != payload.CreatedAt {
		return EarlyRentalCursorPosition{}, ErrInvalidEarlyRentalCursor
	}
	return EarlyRentalCursorPosition{CreatedAt: createdAt.UTC(), ID: payload.ID}, nil
}

func validEarlyRentalCursorBinding(binding EarlyRentalCursorBinding) bool {
	if !validEarlyRentalCursorUUID(binding.ActorID) || binding.Limit < 1 || binding.Limit > maxEarlyRentalListLimit {
		return false
	}
	if binding.Role != "" && binding.Role != earlyRentalRoleRenter && binding.Role != earlyRentalRoleHost {
		return false
	}
	switch binding.Scope {
	case EarlyRentalCursorScopeOffers:
		return binding.State == ""
	case EarlyRentalCursorScopeTransactions:
		return binding.State == "" || validEarlyRentalTransactionState(binding.State)
	default:
		return false
	}
}

func validEarlyRentalCursorUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
