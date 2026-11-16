package publiccatalog

import (
	"encoding/json"
	"testing"
)

func TestSpaceCapacityUsesPublicWireKey(t *testing.T) {
	value, err := json.Marshal(Space{CapacityMax: 12})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(value, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["capacity_max"] != float64(12) {
		t.Fatalf("capacity_max=%v", wire["capacity_max"])
	}
}

func TestSpaceCapacityIsOmittedWhenZero(t *testing.T) {
	value, err := json.Marshal(Space{})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(value, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["capacity_max"]; ok {
		t.Fatalf("list wire unexpectedly includes capacity_max: %s", value)
	}
}
