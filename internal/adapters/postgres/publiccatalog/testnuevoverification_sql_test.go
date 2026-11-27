package publiccatalogpg

import (
	"os"
	"strings"
	"testing"
)

func TestPublicVerificationSQLDerivesCurrentOwnerBadges(t *testing.T) {
	source, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, fragment := range []string{
		"account_profile_identity_status(owner.id,now())",
		"SELECT p.username FROM public.account_identity_profile",
		"JOIN LATERAL public.kyb_space_eligibility(e.id) commercial ON commercial.can_book",
		"commercial.space_verified",
		"company.trade_name",
		"AS identity_verified",
		"verification.identity_verified,verification.space_verified",
		"&v.Verification.IdentityVerified, &v.Verification.SpaceVerified",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("repository SQL/scan missing %q", fragment)
		}
	}
	if strings.Contains(text, "elegibilidad_verificacion_local") || strings.Contains(text, "tipo='kyc'") {
		t.Fatal("legacy KYC eligibility must not gate the public catalog")
	}
	for _, guard := range []string{
		"e.estado='activa'",
		"bloqueo_cuenta_administrativo_local",
		"reserva_ensayo_local_fixture",
		"owner.estado='activo'",
	} {
		if !strings.Contains(text, guard) {
			t.Errorf("required public eligibility guard removed: %s", guard)
		}
	}
}
