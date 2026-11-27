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
	version, err := os.ReadFile("../../db/migrations/V000015__local_terms_v2_commercial_documentation.sql")
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
	prodTermsVersion, err := os.ReadFile("../../db/migrations/V000019__production_terms_v2_commercial_documentation.sql")
	if err != nil {
		t.Fatal(err)
	}
	termsDoc, privacyDoc := prodDocs[0], prodDocs[1]
	if termsDoc.ID != "00000000-0000-4000-8000-000000000104" ||
		termsDoc.Code != "espacigo-terminos-20261126-v2" ||
		termsDoc.Type != "terminos" ||
		len(termsDoc.Sections) < 5 ||
		termsDoc.Status == "" ||
		!strings.Contains(string(prodTermsVersion), termsDoc.ID) ||
		!strings.Contains(string(prodTermsVersion), termsDoc.Code) ||
		!strings.Contains(string(prodTermsVersion), termsDoc.SHA256) {
		t.Fatal("production terms v2 publication/content integrity mismatch")
	}
	if !strings.Contains(string(migration), privacyDoc.ID) || !strings.Contains(string(migration), privacyDoc.SHA256) || len(privacyDoc.Sections) < 5 || privacyDoc.Status == "" {
		t.Fatal("production privacy document publication/content integrity mismatch")
	}
}
