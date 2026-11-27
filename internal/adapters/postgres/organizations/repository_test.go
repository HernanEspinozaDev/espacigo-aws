package organizationspg

import (
	"errors"
	"testing"

	"github.com/HernanEspinozaDev/espaciGo/internal/organizations"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRutFingerprintUniqueViolationIsConflict(t *testing.T) {
	err := mapErr(&pgconn.PgError{Code: "23505", ConstraintName: "organizacion_rut_fingerprint_fingerprint_key"})
	if !errors.Is(err, organizations.ErrConflict) {
		t.Fatalf("expected RUT collision conflict, got %v", err)
	}
}
func TestCannotRemoveLastActiveLegalRepresentative(t *testing.T) {
	if !lastRepresentativeConflict(string(organizations.LegalRepresentative), 1) {
		t.Fatal("last active representative must be protected")
	}
	if lastRepresentativeConflict(string(organizations.LegalRepresentative), 2) {
		t.Fatal("multiple active representatives may change roles")
	}
	if lastRepresentativeConflict(string(organizations.Administrator), 1) {
		t.Fatal("other roles are not the representative invariant")
	}
}
