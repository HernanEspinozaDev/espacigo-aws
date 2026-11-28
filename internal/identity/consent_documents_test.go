package identity

import (
	"os"
	"strings"
	"testing"
)

func TestConsentDocumentsMatchAdditivePublishedVersions(t *testing.T) {
	t.Setenv("APP_PROFILE", "")
	t.Setenv("KYB_LOCAL_TERMS_V2", "")
	docs, err := PublicConsentDocuments()
	if err != nil || len(docs) != 2 {
		t.Fatal("read consent documents")
	}
	migration, err := os.ReadFile("../../db/migrations/V000001__baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if !strings.Contains(string(migration), d.ID) || !strings.Contains(string(migration), d.SHA256) || len(d.Sections) < 5 || d.Status == "" {
			t.Fatal("document publication/content integrity mismatch")
		}
	}

	t.Setenv("KYB_LOCAL_TERMS_V2", "1")
	localDocs, err := PublicConsentDocuments()
	if err != nil || len(localDocs) != 2 {
		t.Fatal("read v2 evaluation terms")
	}
	version, err := os.ReadFile("../../db/legacy-migrations/v29/V000015__local_terms_v2_commercial_documentation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if localDocs[0].ID == docs[0].ID || localDocs[1].ID != docs[1].ID ||
		!strings.Contains(string(version), localDocs[0].ID) ||
		!strings.Contains(string(version), localDocs[0].SHA256) {
		t.Fatal("v2 must match its published hash and preserve the privacy document")
	}

	t.Setenv("APP_PROFILE", "production")
	prodDocs, err := PublicConsentDocuments()
	if err != nil || len(prodDocs) != 2 {
		t.Fatal("read production consent documents")
	}
	prodTermsVersion, err := os.ReadFile("../../db/legacy-migrations/v29/V000021__hotusoft_legal_documents.sql")
	if err != nil {
		t.Fatal(err)
	}
	termsDoc, privacyDoc := prodDocs[0], prodDocs[1]
	if termsDoc.Type != "terminos" || privacyDoc.Type != "privacidad" {
		t.Fatal("production consent document types in the wrong order")
	}
	for _, document := range prodDocs {
		if len(document.Sections) < 5 || document.Status == "" ||
			!strings.Contains(string(prodTermsVersion), document.ID) ||
			!strings.Contains(string(prodTermsVersion), document.Code) ||
			!strings.Contains(string(prodTermsVersion), document.SHA256) {
			t.Fatalf("production publication/content integrity mismatch for %s", document.Code)
		}
	}
}
