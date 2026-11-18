package main

import "testing"

func TestValidateProductionRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
	}{
		{name: "production settings"},
		{name: "local payment flow", key: "LOCAL_BOOKING_TRIAL", value: "1"},
		{name: "synthetic identity", key: "LOCAL_AUTH_PROTOTYPE", value: "1"},
		{name: "emulator", key: "AWS_ENDPOINT_URL_S3", value: "https://127.0.0.1:9297"},
		{name: "static credentials", key: "AWS_ACCESS_KEY_ID", value: "test"},
		{name: "database TLS fallback", key: "DATABASE_SSLMODE", value: "require"},
		{name: "database admin", key: "DATABASE_USER", value: "espacigo_admin"},
		{name: "public API listener", key: "HTTP_ADDR", value: "0.0.0.0:8080"},
		{name: "insecure frontend origin", key: "FRONTEND_PUBLIC_ORIGIN", value: "http://espacigo.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				"HTTP_ADDR":              "127.0.0.1:8080",
				"DATABASE_SSLMODE":       "verify-full",
				"DATABASE_SSLROOTCERT":   "/certs/rds.pem",
				"DATABASE_USER":          "espacigo_api_login",
				"DATABASE_SET_ROLE":      "espacigo_runtime",
				"FRONTEND_PUBLIC_ORIGIN": "https://espacigo.example",
				"BLOB_STORE_BACKEND":     "s3",
				"AWS_REGION":             "us-east-2",
			}
			if tc.key != "" {
				values[tc.key] = tc.value
			}
			err := validateProductionRuntime(func(key string) string { return values[key] }, runtimeProfileStandard)
			if (err != nil) != (tc.key != "") {
				t.Fatalf("unexpected production configuration result: %v", err)
			}
		})
	}
}
