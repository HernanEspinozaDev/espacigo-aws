package main

import (
	"net"
	"strings"
)

// localSyntheticKYCEnabled permits the quick KYC fixture only for the local
// PostgreSQL/API target used by scripts/local-dev.sh. The fixture remains a
// separate verification assertion and is never documentary identity proof.
func localSyntheticKYCEnabled(getenv func(string) string, cfg config) bool {
	if getenv == nil || cfg.production || getenv("ENVIRONMENT") != "development" ||
		getenv("APP_PROFILE") != "" || getenv("LOCAL_AUTH_PROTOTYPE") != "1" ||
		getenv("DATABASE_CONNECTION_MODE") != "direct" ||
		cfg.dbName != "espacigo_aws_local" || cfg.dbUser != "espacigo_api_login" ||
		cfg.dbSetRole != "espacigo_runtime" || cfg.dbPort != 5432 {
		return false
	}

	apiHost, _, err := net.SplitHostPort(cfg.httpAddr)
	if err != nil {
		return false
	}
	apiIP := net.ParseIP(strings.Trim(apiHost, "[]"))
	if strings.ToLower(apiHost) != "localhost" && (apiIP == nil || !apiIP.IsLoopback()) {
		return false
	}

	// local-dev.sh connects directly to the PostgreSQL container via its
	// private Docker bridge address, or through a loopback port mapping.
	dbIP := net.ParseIP(strings.Trim(strings.ToLower(cfg.dbHost), "[]"))
	if dbIP == nil {
		return false
	}
	if dbIP.IsLoopback() {
		return true
	}
	return dbIP.To4() != nil && dbIP.To4()[0] == 172 && dbIP.To4()[1]&0xf0 == 16
}
