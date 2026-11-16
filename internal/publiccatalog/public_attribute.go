package publiccatalog

import "encoding/json"

// PublicAttribute is the minimal typed projection of an explicitly published profile value.
type PublicAttribute struct {
	Code  string          `json:"code"`
	Label string          `json:"label"`
	Type  string          `json:"type"`
	Order int             `json:"order"`
	Unit  string          `json:"unit,omitempty"`
	Value json.RawMessage `json:"value"`
}
