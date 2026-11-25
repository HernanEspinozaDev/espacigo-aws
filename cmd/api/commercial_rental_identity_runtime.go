package main

import (
	"errors"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
)

func loadRentalContractDeclarationKeyring(getenv func(string) string) (*declarationcrypto.Keyring, error) {
	path := strings.TrimSpace(getenv("IDENTITY_DECLARATION_KEYRING_FILE"))
	if path == "" {
		path = strings.TrimSpace(getenv("LOCAL_IDENTITY_DECLARATION_KEYRING_FILE"))
	}
	if path == "" {
		if rawJSON := strings.TrimSpace(getenv("IDENTITY_DECLARATION_KEYRING_JSON")); rawJSON != "" {
			keyring, err := declarationcrypto.ParseJSON([]byte(rawJSON))
			if err != nil {
				return nil, errors.New("rental contract identity keyring configuration is invalid")
			}
			return keyring, nil
		}
		return nil, nil
	}
	keyring, err := declarationcrypto.LoadFile(path)
	if err != nil {
		return nil, errors.New("rental contract identity keyring configuration is invalid")
	}
	return keyring, nil
}
