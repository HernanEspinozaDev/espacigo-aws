package earlyrentalhttp

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestEarlyRentalCursorCodecRoundTripsAndUsesFreshNonces(t *testing.T) {
	codec := newTestEarlyRentalCursorCodec(t)
	binding := EarlyRentalCursorBinding{
		Scope: EarlyRentalCursorScopeOffers, ActorID: earlyRentalTestRenterID,
		Role: earlyRentalRoleRenter, Limit: 25,
	}
	position := EarlyRentalCursorPosition{
		CreatedAt: time.Date(2026, 11, 17, 9, 30, 0, 123, time.FixedZone("CLST", -3*60*60)),
		ID:        earlyRentalTestOfferID,
	}
	first, err := codec.Encode(binding, position)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	second, err := codec.Encode(binding, position)
	if err != nil {
		t.Fatalf("Encode() second error = %v", err)
	}
	if first == second {
		t.Fatal("cursor token reused a nonce and produced identical ciphertext")
	}
	if len(first) > maxEarlyRentalCursorRunes {
		t.Fatalf("cursor length = %d, exceeds OpenAPI max", len(first))
	}
	got, err := codec.Decode(first, binding)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.ID != position.ID || !got.CreatedAt.Equal(position.CreatedAt) || got.CreatedAt.Location() != time.UTC {
		t.Fatalf("decoded position = %+v, want %+v normalized to UTC", got, position)
	}
	reloadedCodec := newTestEarlyRentalCursorCodec(t)
	if _, err := reloadedCodec.Decode(first, binding); err != nil {
		t.Fatalf("stable configured key did not survive codec reconstruction: %v", err)
	}
	wrongKeyCodec, err := NewEarlyRentalCursorCodec(bytes.Repeat([]byte{0x43}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongKeyCodec.Decode(first, binding); err == nil {
		t.Fatal("cursor decoded under a different key")
	}
}

func TestDecodedListQueriesBuildCursorBindingsFromNormalizedFilters(t *testing.T) {
	offers, err := DecodeEarlyRentalListQuery(map[string][]string{"limit": {"25"}})
	if err != nil {
		t.Fatal(err)
	}
	offerBinding := offers.CursorBinding(earlyRentalTestRenterID)
	if offerBinding.Scope != EarlyRentalCursorScopeOffers || offerBinding.ActorID != earlyRentalTestRenterID ||
		offerBinding.Role != "" || offerBinding.State != "" || offerBinding.Limit != 25 {
		t.Fatalf("offer cursor binding = %+v", offerBinding)
	}

	transactions, err := DecodeEarlyRentalTransactionListQuery(map[string][]string{
		"role": {earlyRentalRoleHost}, "state": {"vencido"}, "limit": {"7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	transactionBinding := transactions.CursorBinding(earlyRentalTestHostID)
	if transactionBinding.Scope != EarlyRentalCursorScopeTransactions || transactionBinding.ActorID != earlyRentalTestHostID ||
		transactionBinding.Role != earlyRentalRoleHost || transactionBinding.State != "vencido" || transactionBinding.Limit != 7 {
		t.Fatalf("transaction cursor binding = %+v", transactionBinding)
	}
}

func TestEarlyRentalCursorCodecBindsTokenToActorScopeFiltersAndLimit(t *testing.T) {
	codec := newTestEarlyRentalCursorCodec(t)
	binding := EarlyRentalCursorBinding{
		Scope: EarlyRentalCursorScopeTransactions, ActorID: earlyRentalTestRenterID,
		Role: earlyRentalRoleRenter, State: "enviado", Limit: 20,
	}
	token, err := codec.Encode(binding, EarlyRentalCursorPosition{
		CreatedAt: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC),
		ID:        "550e8400-e29b-41d4-a716-446655440090",
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	for _, tc := range []struct {
		name    string
		binding EarlyRentalCursorBinding
	}{
		{name: "different actor", binding: EarlyRentalCursorBinding{Scope: binding.Scope, ActorID: earlyRentalTestHostID, Role: binding.Role, State: binding.State, Limit: binding.Limit}},
		{name: "different scope", binding: EarlyRentalCursorBinding{Scope: EarlyRentalCursorScopeOffers, ActorID: binding.ActorID, Role: binding.Role, State: binding.State, Limit: binding.Limit}},
		{name: "different role", binding: EarlyRentalCursorBinding{Scope: binding.Scope, ActorID: binding.ActorID, Role: earlyRentalRoleHost, State: binding.State, Limit: binding.Limit}},
		{name: "different state", binding: EarlyRentalCursorBinding{Scope: binding.Scope, ActorID: binding.ActorID, Role: binding.Role, State: "recibido", Limit: binding.Limit}},
		{name: "different limit", binding: EarlyRentalCursorBinding{Scope: binding.Scope, ActorID: binding.ActorID, Role: binding.Role, State: binding.State, Limit: 21}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := codec.Decode(token, tc.binding); err == nil {
				t.Fatal("Decode() error = nil, want binding mismatch")
			}
		})
	}
}

func TestEarlyRentalCursorCodecRejectsInvalidKeysBindingsPositionsAndTokens(t *testing.T) {
	if _, err := NewEarlyRentalCursorCodec([]byte("short")); err == nil {
		t.Fatal("short cursor key unexpectedly accepted")
	}
	if _, err := NewEarlyRentalCursorCodec(make([]byte, 32)); err == nil {
		t.Fatal("all-zero cursor key unexpectedly accepted")
	}
	codec := newTestEarlyRentalCursorCodec(t)
	binding := EarlyRentalCursorBinding{Scope: EarlyRentalCursorScopeOffers, ActorID: earlyRentalTestRenterID, Limit: 25}
	position := EarlyRentalCursorPosition{CreatedAt: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC), ID: earlyRentalTestOfferID}
	invalidBindings := []EarlyRentalCursorBinding{
		{Scope: "unknown", ActorID: binding.ActorID, Limit: 25},
		{Scope: binding.Scope, ActorID: "not-a-uuid", Limit: 25},
		{Scope: binding.Scope, ActorID: binding.ActorID, Role: "admin", Limit: 25},
		{Scope: binding.Scope, ActorID: binding.ActorID, State: "enviado", Limit: 25},
		{Scope: binding.Scope, ActorID: binding.ActorID, Limit: 0},
		{Scope: EarlyRentalCursorScopeTransactions, ActorID: binding.ActorID, State: "pagado", Limit: 25},
	}
	for i, invalid := range invalidBindings {
		if _, err := codec.Encode(invalid, position); err == nil {
			t.Fatalf("Encode() accepted invalid binding %d: %+v", i, invalid)
		}
	}
	for _, invalidPosition := range []EarlyRentalCursorPosition{
		{CreatedAt: position.CreatedAt, ID: "00000000-0000-0000-0000-000000000000"},
		{CreatedAt: position.CreatedAt, ID: strings.ToUpper("550e8400-e29b-41d4-a716-446655440099")},
		{CreatedAt: time.Time{}, ID: position.ID},
	} {
		if _, err := codec.Encode(binding, invalidPosition); err == nil {
			t.Fatalf("Encode() accepted invalid position: %+v", invalidPosition)
		}
	}
	token, err := codec.Encode(binding, position)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	tamperedBytes := []byte(token)
	tamperedBytes[len(tamperedBytes)-2] ^= 1
	tampered := string(tamperedBytes)
	for _, invalidToken := range []string{"", "v2.invalid", strings.Repeat("x", maxEarlyRentalCursorRunes+1), tampered} {
		if _, err := codec.Decode(invalidToken, binding); err == nil {
			t.Fatalf("Decode() accepted invalid token %q", invalidToken)
		}
	}
}

func newTestEarlyRentalCursorCodec(t *testing.T) *EarlyRentalCursorCodec {
	t.Helper()
	key := bytes.Repeat([]byte{0x42}, 32)
	codec, err := NewEarlyRentalCursorCodec(key)
	if err != nil {
		t.Fatalf("NewEarlyRentalCursorCodec() error = %v", err)
	}
	return codec
}
