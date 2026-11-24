package identity

import (
	"os"
	"strings"
	"testing"
)

func TestConsentDocumentsMatchAdditivePublishedVersions(t *testing.T) {
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

	t.Setenv("APP_PROFILE", "production")
	prodDocs, err := PublicConsentDocuments()
	if err != nil || len(prodDocs) != 2 {
		t.Fatal("read production consent documents")
	}
	for _, d := range prodDocs {
		if !strings.Contains(string(migration), d.ID) || !strings.Contains(string(migration), d.SHA256) || len(d.Sections) < 5 || d.Status == "" {
			t.Fatal("production document publication/content integrity mismatch")
		}
	}
}
