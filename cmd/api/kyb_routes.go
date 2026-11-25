package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	kybpg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/kyb"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	kybhttp "github.com/HernanEspinozaDev/espaciGo/internal/kyb/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountKYBRoutes(mux *http.ServeMux, pool *pgxpool.Pool, auth kybhttp.Authenticator, cfg config, getenv func(string) string, required bool) error {
	store, err := newRuntimeKYBDocumentStore(getenv)
	if err != nil {
		return err
	}
	if store == nil {
		if required {
			return errors.New("KYB private document storage is required")
		}
		return nil
	}
	declarationKeys, err := loadKYBDeclarationKeyring(getenv)
	if err != nil || declarationKeys == nil {
		return errors.New("KYB RUT encryption keyring is unavailable")
	}
	rutKeys, err := loadKYBRutHMACKeyring(getenv)
	if err != nil || rutKeys == nil {
		return errors.New("KYB RUT HMAC keyring is unavailable")
	}
	service, err := kyb.NewService(kybpg.New(pool), store, declarationKeys, rutKeys, credentials.Generator{}, time.Now)
	if err != nil {
		return errors.New("KYB service initialization failed")
	}
	handler := kybhttp.NewHandler(auth, service, cfg.allowedOrigins)
	mux.Handle("/api/v1/kyb/", handler)
	mux.Handle("/api/v1/admin/kyb/", handler)
	return nil
}

var _ kybhttp.Authenticator = (*identity.AuthenticationService)(nil)

func loadKYBDeclarationKeyring(getenv func(string) string) (*declarationcrypto.Keyring, error) {
	if strings.TrimSpace(getenv("IDENTITY_DECLARATION_KEYRING_FILE")) != "" || strings.TrimSpace(getenv("LOCAL_IDENTITY_DECLARATION_KEYRING_FILE")) != "" {
		return loadIdentityDeclarationKeyring(getenv)
	}
	encoded := []byte(strings.TrimSpace(getenv("KYB_RUT_ENCRYPTION_KEYRING_JSON")))
	if len(encoded) == 0 {
		return nil, nil
	}
	defer clear(encoded)
	current, keys, err := loadRuntimeDocumentKeyringJSON(encoded)
	if err != nil {
		return nil, err
	}
	defer clearRuntimeDocumentKeys(keys)
	return declarationcrypto.New(current, keys)
}

func loadKYBRutHMACKeyring(getenv func(string) string) (*verification.RutHMACKeyring, error) {
	encoded := []byte(strings.TrimSpace(getenv("KYB_RUT_HMAC_KEYRING_JSON")))
	if len(encoded) == 0 {
		return loadRuntimeRutHMACKeyring(getenv)
	}
	defer clear(encoded)
	var config struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || config.Active == "" || len(config.Keys) == 0 {
		return nil, errors.New("invalid KYB HMAC keyring")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, errors.New("KYB HMAC keyring has trailing data")
	}
	keys := make(map[string][]byte, len(config.Keys))
	for version, material := range config.Keys {
		key, err := hex.DecodeString(material)
		if err != nil {
			clearRuntimeDocumentKeys(keys)
			return nil, errors.New("invalid KYB HMAC key material")
		}
		keys[version] = key
	}
	defer clearRuntimeDocumentKeys(keys)
	return verification.NewRutHMACKeyring(config.Active, keys)
}
