package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	commercialproposalspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialproposals"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	commercialproposalhttp "github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal/transport/http"
	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification/declarationcrypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mountCommercialContractProposals(mux *http.ServeMux, pool *pgxpool.Pool, auth commercialproposalhttp.Authenticator, quotes rentalflow.QuoteRepository, preview commercialproposal.Previewer, gate rentalflow.IdentityGate, identities rentalflow.ContractIdentityReader, origins []string, getenv func(string) string, businessIdentity *kyb.Service) (commercialproposal.Cipher, error) {
	var cipher commercialproposal.Cipher
	if path := strings.TrimSpace(getenv("CONTRACT_PROPOSAL_KEYRING_FILE")); path != "" {
		keys, err := declarationcrypto.LoadFile(path)
		if err != nil {
			return nil, errors.New("commercial contract proposal keyring unavailable")
		}
		cipher = keys
	} else if rawJSON := strings.TrimSpace(getenv("CONTRACT_PROPOSAL_KEYRING_JSON")); rawJSON != "" {
		keys, err := declarationcrypto.ParseJSON([]byte(rawJSON))
		if err != nil {
			return nil, errors.New("commercial contract proposal keyring unavailable")
		}
		cipher = keys
	}
	policy := commercialproposal.CancellationPolicy{Version: getenv("COMMERCIAL_CONTRACT_CANCELLATION_VERSION"), Text: getenv("COMMERCIAL_CONTRACT_CANCELLATION_TEXT")}
	service := commercialproposal.New(commercialproposalspg.New(pool), quotes, preview, gate, identities, cipher, policy, time.Now)
	if businessIdentity != nil {
		service.WithOrganizationContractIdentityReader(businessIdentity)
	}
	h := commercialproposalhttp.New(auth, service, origins)
	mux.Handle("/api/v1/rental-quotes/{quoteID}/contract-proposal", h)
	mux.Handle("/api/v1/rental-contract-proposals/{proposalID}", h)
	return cipher, nil
}
