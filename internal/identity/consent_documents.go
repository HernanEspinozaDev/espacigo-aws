package identity

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
)

//go:embed consentdocs/*.json
var consentFiles embed.FS

type ConsentSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}
type ConsentDocument struct {
	ID       string           `json:"id"`
	Code     string           `json:"code"`
	Type     string           `json:"type"`
	Title    string           `json:"title"`
	Scope    string           `json:"scope"`
	Status   string           `json:"status"`
	Sections []ConsentSection `json:"sections"`
	SHA256   string           `json:"-"`
}

// PublicConsentDocuments returns the active readable consent documents. The HTTP
// boundary checks their exact bytes against published DB versions before use.
func PublicConsentDocuments() ([]ConsentDocument, error) {
	names := []string{"terms", "privacy"}
	if os.Getenv("APP_PROFILE") == "production" {
		names = []string{"production_terms_v3", "production_privacy_v2"}
	} else if os.Getenv("KYB_LOCAL_TERMS_V2") == "1" {
		names[0] = "evaluation_terms_v2"
	}
	result := []ConsentDocument{}
	for _, name := range names {
		data, err := consentFiles.ReadFile("consentdocs/" + name + ".json")
		if err != nil {
			return nil, err
		}
		var document ConsentDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		document.SHA256 = hex.EncodeToString(digest[:])
		result = append(result, document)
	}
	return result, nil
}
