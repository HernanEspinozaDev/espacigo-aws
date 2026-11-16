package publiccatalog

import (
	"encoding/json"
	"testing"
)

func TestPublicAttributeJSONContract(t *testing.T) {
	var a PublicAttribute
	if err := json.Unmarshal([]byte(`{"code":"metros","label":"Metros","type":"number","order":2,"unit":"m","value":0.00}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.Code != "metros" || a.Type != "number" || string(a.Value) != "0.00" {
		t.Fatalf("unexpected: %#v value=%s", a, a.Value)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"code":"metros","label":"Metros","type":"number","order":2,"unit":"m","value":0.00}` {
		t.Fatalf("marshal=%s", b)
	}
}
