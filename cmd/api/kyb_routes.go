package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func mountKYBRoutes(mux *http.ServeMux, pool *pgxpool.Pool, auth kybhttp.Authenticator, cfg config, getenv func(string) string, required, mountCompanies bool) (*kyb.Service, error) {
	store, err := newRuntimeKYBDocumentStore(getenv)
	if err != nil {
		return nil, err
	}
	if store == nil {
		if required {
			return nil, errors.New("KYB private document storage is required")
		}
		return nil, nil
	}
	declarationKeys, err := loadKYBDeclarationKeyring(getenv)
	if err != nil || declarationKeys == nil {
		return nil, errors.New("KYB RUT encryption keyring is unavailable")
	}
	rutKeys, err := loadKYBRutHMACKeyring(getenv)
	if err != nil || rutKeys == nil {
		return nil, errors.New("KYB RUT HMAC keyring is unavailable")
	}
	service, err := kyb.NewService(kybpg.New(pool), store, declarationKeys, rutKeys, credentials.Generator{}, time.Now)
	if err != nil {
		return nil, errors.New("KYB service initialization failed")
	}
	identityRutKeys, keyErr := loadRuntimeRutHMACKeyring(getenv)
	if keyErr != nil {
		return nil, errors.New("KYB representative identity fingerprinting is unavailable")
	}
	service.SetIdentityRutFingerprinter(identityRutKeys)
	if mountCompanies {
		companies, err := mountOrganizationRoutes(mux, pool, auth, rutKeys)
		if err != nil {
			return nil, err
		}
		service.SetOrganizationAuthorizer(companies)
	}
	handler := kybhttp.NewHandler(auth, service, cfg.allowedOrigins)
	mux.Handle("/api/v1/kyb/", handler)
	mux.Handle("/api/v1/admin/kyb/", handler)
	return service, nil
}

var _ kybhttp.Authenticator = (*identity.AuthenticationService)(nil)

func loadKYBDeclarationKeyring(getenv func(string) string) (*declarationcrypto.Keyring, error) {
	if path := strings.TrimSpace(getenv("KYB_RUT_ENCRYPTION_KEYRING_FILE")); path != "" {
		current, keys, err := loadRuntimeDocumentKeyring(path)
		if err != nil {
			return nil, err
		}
		defer clearRuntimeDocumentKeys(keys)
		return declarationcrypto.New(current, keys)
	}
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
	if path := strings.TrimSpace(getenv("KYB_RUT_HMAC_KEYRING_FILE")); path != "" {
		if len(encoded) > 0 {
			return nil, errors.New("KYB RUT HMAC keyring configuration is ambiguous")
		}
		contents, err := readRuntimeKeyringFile(path)
		if err != nil {
			return nil, err
		}
		defer clear(contents)
		return decodeKYBRutHMACKeyring(contents)
	}
	if len(encoded) == 0 {
		return loadRuntimeRutHMACKeyring(getenv)
	}
	defer clear(encoded)
	return decodeKYBRutHMACKeyring(encoded)
}

func decodeKYBRutHMACKeyring(encoded []byte) (*verification.RutHMACKeyring, error) {
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

func readRuntimeKeyringFile(path string) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("invalid keyring path")
	}
	canonicalPath, err := canonicalStoragePath(path)
	if err != nil || canonicalPath != path {
		return nil, errors.New("keyring path cannot contain symlinked ancestors")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("keyring must be a bounded readable regular file with an approved mode")
	}
	mode := info.Mode().Perm()
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		(mode != 0o400 && mode != 0o600 && mode != 0o444) || info.Size() <= 0 || info.Size() > 64<<10 {
		return nil, errors.New("keyring must be a bounded readable regular file with an approved mode")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("keyring cannot be read")
	}
	return contents, nil
}
