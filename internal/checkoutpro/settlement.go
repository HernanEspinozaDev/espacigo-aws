package checkoutpro

import (
	"encoding/json"
	"time"
)

// Settlement contains the financial facts returned for a payment by Mercado Pago.
type Settlement struct {
	NetCLP           *int64     `json:"net_clp,omitempty"`
	ProcessingFeeCLP *int64     `json:"processing_fee_clp,omitempty"`
	MarketplaceCLP   *int64     `json:"marketplace_fee_clp,omitempty"`
	RefundedCLP      *int64     `json:"refunded_clp,omitempty"`
	ReleaseStatus    string     `json:"release_status,omitempty"`
	ReleaseAt        *time.Time `json:"release_at,omitempty"`
	ProviderUpdated  time.Time  `json:"provider_updated_at"`
	Reconciliation   string     `json:"reconciliation"`
}

func (s Settlement) WithReconciliation(charged, expectedMarketplace int64) Settlement {
	s.Reconciliation = "pending"
	if s.NetCLP == nil || s.ProcessingFeeCLP == nil || s.MarketplaceCLP == nil {
		return s
	}
	s.Reconciliation = "discrepancy"
	// Refunded amounts can change the effective net. Keep that adjustment
	// explicit when the original sale equation no longer matches.
	marketplace, processing := *s.MarketplaceCLP, *s.ProcessingFeeCLP
	if marketplace == expectedMarketplace && marketplace <= charged &&
		processing <= charged-marketplace && *s.NetCLP == charged-marketplace-processing {
		s.Reconciliation = "reconciled"
	}
	return s
}

func optionalCLP(raw json.RawMessage) *int64 {
	var number json.Number
	if len(raw) == 0 || json.Unmarshal(raw, &number) != nil {
		return nil
	}
	clp, ok := parseCLPAmount(number, true)
	if !ok {
		return nil
	}
	return &clp
}

type paymentFee struct {
	Type  string          `json:"type"`
	Payer string          `json:"fee_payer"`
	Value json.RawMessage `json:"amount"`
}

type paymentCharge struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Amounts struct {
		Original json.RawMessage `json:"original"`
	} `json:"amounts"`
	Accounts struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"accounts"`
}

// The same deduction can appear in fee_details and charges_details.
func settlementFee(fees []paymentFee, charges []paymentCharge, feeName, chargeName, destination string) *int64 {
	var amount *int64
	for _, fee := range fees {
		if fee.Type != feeName || fee.Payer != "collector" {
			continue
		}
		if amount != nil {
			return nil
		}
		amount = optionalCLP(fee.Value)
		if amount == nil {
			return nil
		}
	}
	if amount != nil {
		return amount
	}
	for _, charge := range charges {
		if charge.Type != "fee" || charge.Name != chargeName || charge.Accounts.From != "collector" || charge.Accounts.To != destination {
			continue
		}
		if amount != nil {
			return nil
		}
		amount = optionalCLP(charge.Amounts.Original)
		if amount == nil {
			return nil
		}
	}
	return amount
}
