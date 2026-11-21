package checkoutapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMPCLICatalogContractMatchesCheckoutAPIValidation(t *testing.T) {
	mlcCardMethods := []string{
		"visa",
		"master",
		"amex",
		"debvisa",
		"debmaster",
		"presto",
		"magna",
		"diners",
	}
	for _, methodID := range mlcCardMethods {
		t.Run("method_"+methodID, func(t *testing.T) {
			in := Input{
				Token:                 "a1b2c3d4e5f60718293a4b5c6d7e8f90",
				PaymentMethodID:       methodID,
				IssuerID:              "12345",
				TokenizationPublicKey: "TEST-11111111-2222-3333-4444-555555555555",
				Payer: Payer{
					Email: "comprador.prueba@example.cl",
					Identification: &Identification{
						Type:   "RUT",
						Number: "123456785",
					},
				},
			}
			if !validInput(in) {
				t.Fatalf("expected MLC payment method %q with RUT identification to be accepted by validInput", methodID)
			}
		})
	}

	if validInput(Input{
		Token:           "a1b2c3d4e5f60718293a4b5c6d7e8f90",
		PaymentMethodID: "account_money",
		Payer:           Payer{Email: "comprador.prueba@example.cl"},
	}) {
		t.Fatal("account_money must remain rejected on Checkout API card endpoint")
	}
}

func TestMPCLISandboxConnectionAndCatalog(t *testing.T) {
	if os.Getenv("MPCLI_SANDBOX_VERIFY") != "1" {
		t.Skip("set MPCLI_SANDBOX_VERIFY=1 to run live mpcli sandbox connection checks")
	}
	mpcliBin, err := exec.LookPath("mpcli")
	if err != nil {
		t.Fatalf("mpcli not found in PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("whoami_sandbox_mlc", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, mpcliBin, "whoami", "--silent", "--no-color").Output()
		if err != nil {
			t.Fatalf("mpcli whoami failed: %v", err)
		}
		var payload struct {
			Status string `json:"status"`
			Data   struct {
				SiteID    string `json:"site_id"`
				CountryID string `json:"country_id"`
				AppID     string `json:"app_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("decode mpcli whoami JSON: %v", err)
		}
		if payload.Status != "success" {
			t.Fatalf("mpcli whoami status=%q, want success", payload.Status)
		}
		if payload.Data.SiteID != "MLC" || payload.Data.CountryID != "CL" {
			t.Fatalf("mpcli whoami site_id=%q country_id=%q, want MLC/CL", payload.Data.SiteID, payload.Data.CountryID)
		}
		if strings.TrimSpace(payload.Data.AppID) == "" {
			t.Fatal("mpcli whoami returned empty app_id")
		}
	})

	t.Run("identification_types_include_rut", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, mpcliBin, "identification-types", "list", "--silent", "--no-color").Output()
		if err != nil {
			t.Fatalf("mpcli identification-types list failed: %v", err)
		}
		var payload struct {
			Data struct {
				Results []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("decode mpcli identification-types JSON: %v", err)
		}
		foundRUT := false
		for _, item := range payload.Data.Results {
			if item.ID == "RUT" {
				foundRUT = true
			}
			if !identifierPattern.MatchString(item.ID) {
				t.Fatalf("identification type %q rejected by checkoutapi identifierPattern", item.ID)
			}
		}
		if !foundRUT {
			t.Fatal("MLC identification types did not include RUT")
		}
	})

	t.Run("payment_methods_compatible_with_checkoutapi", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, mpcliBin, "payment-methods", "list", "--silent", "--no-color").Output()
		if err != nil {
			t.Fatalf("mpcli payment-methods list failed: %v", err)
		}
		var payload struct {
			Data struct {
				Results []struct {
					ID            string `json:"id"`
					PaymentTypeID string `json:"payment_type_id"`
					Status        string `json:"status"`
				} `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("decode mpcli payment-methods JSON: %v", err)
		}
		cardCount := 0
		for _, pm := range payload.Data.Results {
			if pm.Status != "" && pm.Status != "active" {
				continue
			}
			if pm.PaymentTypeID != "credit_card" && pm.PaymentTypeID != "debit_card" && pm.PaymentTypeID != "prepaid_card" {
				continue
			}
			cardCount++
			sample := Input{
				Token:           "a1b2c3d4e5f60718293a4b5c6d7e8f90",
				PaymentMethodID: pm.ID,
				Payer: Payer{
					Email:          "comprador.prueba@example.cl",
					Identification: &Identification{Type: "RUT", Number: "123456785"},
				},
			}
			if !validInput(sample) {
				t.Fatalf("active MLC card payment_method_id=%q (type=%q) rejected by validInput", pm.ID, pm.PaymentTypeID)
			}
		}
		if cardCount == 0 {
			t.Fatal("mpcli payment-methods list returned zero active card methods for MLC")
		}
	})
}

func TestLiveCheckoutAPIFullPaymentFlow(t *testing.T) {
	if os.Getenv("MPCLI_LIVE_PAYMENT_FLOW_TEST") != "1" {
		t.Skip("set MPCLI_LIVE_PAYMENT_FLOW_TEST=1 to run the full end-to-end Checkout API payment flow against :8081 and Mercado Pago Sandbox")
	}
	rootDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	pwBytes, err := os.ReadFile(filepath.Join(rootDir, ".local", "secrets", "runtime_password"))
	if err != nil {
		t.Fatalf("read runtime_password: %v", err)
	}
	runtimePW := strings.TrimSpace(string(pwBytes))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	const (
		apiBaseURL = "http://127.0.0.1:8081"
		spaceID    = "26fc9622-c4a1-4429-9f68-33fb017d21ce"
		ownerID    = "bd4674cd-e0f5-4246-856f-acddf1731aed"
		renterID   = "8b240d82-1107-4e51-bb16-85692407f16f"
		amountCLP  = 24000
	)

	sessionID := uuid.NewString()
	rawBearer := "live-flow-test-token-" + uuid.NewString()
	sum := sha256.Sum256([]byte(rawBearer))
	tokenHash := hex.EncodeToString(sum[:])
	quoteID := uuid.NewString()
	idempotencyKey := uuid.NewString()

	runSQL := func(sqlText string) string {
		cmd := exec.CommandContext(ctx, "psql", "-h", "127.0.0.1", "-p", "15433", "-U", "espacigo_api_login", "-d", "espacigo_payments_local", "-t", "-A", "-v", "ON_ERROR_STOP=1", "-c", "SET ROLE espacigo_runtime; "+sqlText)
		cmd.Env = append(os.Environ(), "PGPASSFILE=/dev/null", "PGPASSWORD="+runtimePW)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("psql failed: %v\noutput: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	dayOffset := 15 + int(time.Now().UnixNano()%90)
	futureDay := time.Now().UTC().AddDate(0, 0, dayOffset)
	startSlot := time.Date(futureDay.Year(), futureDay.Month(), futureDay.Day(), 14, 0, 0, 0, time.UTC)
	endSlot := startSlot.Add(2 * time.Hour)

	runSQL(fmt.Sprintf(`
INSERT INTO public.sesion(id, usuario_id, token_hash, creada_en, ultima_actividad_en, expira_en, cliente_resumen, device_label, ip_masked, user_agent)
VALUES ('%s', '%s', '%s', NOW(), NOW(), NOW() + INTERVAL '30 minutes', '', 'Test E2E Checkout API', '127.0.•••.•••', 'go-test');
INSERT INTO public.cotizacion_arriendo_comercial(id, espacio_id, propietario_id, arrendatario_id, titulo_espacio, tarifa_version, modalidad, precio_unitario_clp, moneda, unidades, subtotal_clp, inicio, termino, zona_horaria, creada_en, vence_en)
VALUES ('%s', '%s', '%s', '%s', 'Espacio de prueba horario Mercado Pago', 1, 'hora', 12000, 'CLP', 2, %d, '%s', '%s', 'America/Santiago', NOW(), NOW() + INTERVAL '15 minutes');
`, sessionID, renterID, tokenHash, quoteID, spaceID, ownerID, renterID, amountCLP, startSlot.Format(time.RFC3339), endSlot.Format(time.RFC3339)))

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		cleanupSQL := fmt.Sprintf(`
SET ROLE espacigo_runtime;
UPDATE public.cotizacion_arriendo_comercial SET vence_en = NOW() - INTERVAL '1 minute' WHERE id = '%s';
UPDATE public.checkout_preferencia_comercial SET vence_en = NOW() - INTERVAL '1 minute', estado = CASE WHEN estado = 'pending' THEN 'expired' ELSE estado END WHERE cotizacion_id = '%s';
DELETE FROM public.sesion WHERE id = '%s';
`, quoteID, quoteID, sessionID)
		cmd := exec.CommandContext(cleanCtx, "psql", "-h", "127.0.0.1", "-p", "15433", "-U", "espacigo_api_login", "-d", "espacigo_payments_local", "-c", cleanupSQL)
		cmd.Env = append(os.Environ(), "PGPASSFILE=/dev/null", "PGPASSWORD="+runtimePW)
		_ = cmd.Run()
	})

	client := &http.Client{Timeout: 20 * time.Second}

	// Step 1: GET /api/v1/rental-quotes/{quoteID}/checkout-api
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/api/v1/rental-quotes/"+quoteID+"/checkout-api", nil)
	if err != nil {
		t.Fatal(err)
	}
	getReq.Header.Set("Authorization", "Bearer "+rawBearer)
	getReq.Header.Set("Accept", "application/json")
	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatalf("GET checkout-api failed: %v", err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET checkout-api status=%d body=%s", getResp.StatusCode, getBody)
	}
	var snapshot struct {
		Data Result `json:"data"`
	}
	if err := json.Unmarshal(getBody, &snapshot); err != nil {
		t.Fatalf("decode GET checkout-api: %v", err)
	}
	if snapshot.Data.Status != "not_started" || snapshot.Data.AmountCLP != 24857 || snapshot.Data.BaseAmountCLP != amountCLP {
		t.Fatalf("unexpected initial checkout snapshot: %+v", snapshot.Data)
	}
	if snapshot.Data.CardTokenization == nil || snapshot.Data.CardTokenization.Mode != "seller_public_key" || snapshot.Data.CardTokenization.PublicKey == "" {
		t.Fatalf("expected seller_public_key tokenization in snapshot, got: %+v", snapshot.Data.CardTokenization)
	}
	sellerPublicKey := snapshot.Data.CardTokenization.PublicKey
	t.Logf("[Step 1 OK] Snapshot received: quote_id=%s amount_clp=%d status=%s tokenization_mode=%s seller_public_key_prefix=%s...",
		snapshot.Data.QuoteID, snapshot.Data.AmountCLP, snapshot.Data.Status, snapshot.Data.CardTokenization.Mode, sellerPublicKey[:12])

	// Step 2: Tokenize official Mercado Pago MLC test Visa card against https://api.mercadopago.com/v1/card_tokens
	cardNumber := strings.Join([]string{"4509", "9535", "6623", "3704"}, "")
	tokenPayload := map[string]any{
		"card_number":      cardNumber,
		"expiration_month": 11,
		"expiration_year":  2030,
		"security_code":    "123",
		"cardholder": map[string]any{
			"name": "APRO",
			"identification": map[string]any{
				"type":   "RUT",
				"number": "123456785",
			},
		},
	}
	tokenReqBytes, _ := json.Marshal(tokenPayload)
	mpTokenURL := "https://api.mercadopago.com/v1/card_tokens?public_key=" + url.QueryEscape(sellerPublicKey)
	mpTokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mpTokenURL, bytes.NewReader(tokenReqBytes))
	if err != nil {
		t.Fatal(err)
	}
	mpTokenReq.Header.Set("Content-Type", "application/json")
	mpTokenReq.Header.Set("Accept", "application/json")
	mpTokenResp, err := client.Do(mpTokenReq)
	if err != nil {
		t.Fatalf("POST /v1/card_tokens failed: %v", err)
	}
	mpTokenBody, _ := io.ReadAll(mpTokenResp.Body)
	mpTokenResp.Body.Close()
	if mpTokenResp.StatusCode != http.StatusCreated && mpTokenResp.StatusCode != http.StatusOK {
		t.Fatalf("Mercado Pago /v1/card_tokens returned HTTP %d: %s", mpTokenResp.StatusCode, mpTokenBody)
	}
	var mpCardToken struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(mpTokenBody, &mpCardToken); err != nil || mpCardToken.ID == "" {
		t.Fatalf("decode /v1/card_tokens response: err=%v body=%s", err, mpTokenBody)
	}
	t.Logf("[Step 2 OK] Card tokenized with seller public_key: token_id=%s status=%s", mpCardToken.ID, mpCardToken.Status)

	// Step 3: POST /api/v1/rental-quotes/{quoteID}/checkout-api
	payInput := Input{
		Token:                 mpCardToken.ID,
		PaymentMethodID:       "visa",
		TokenizationPublicKey: sellerPublicKey,
		Payer: Payer{
			Email: "buyer@example.com",
			Identification: &Identification{
				Type:   "RUT",
				Number: "123456785",
			},
		},
	}
	payBytes, _ := json.Marshal(payInput)
	payReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/api/v1/rental-quotes/"+quoteID+"/checkout-api", bytes.NewReader(payBytes))
	if err != nil {
		t.Fatal(err)
	}
	payReq.Header.Set("Authorization", "Bearer "+rawBearer)
	payReq.Header.Set("Content-Type", "application/json")
	payReq.Header.Set("Accept", "application/json")
	payReq.Header.Set("Idempotency-Key", idempotencyKey)
	payResp, err := client.Do(payReq)
	if err != nil {
		t.Fatalf("POST checkout-api failed: %v", err)
	}
	payRespBody, _ := io.ReadAll(payResp.Body)
	payResp.Body.Close()
	if payResp.StatusCode != http.StatusOK {
		t.Fatalf("POST checkout-api returned HTTP %d: %s", payResp.StatusCode, payRespBody)
	}
	var payResult struct {
		Data Result `json:"data"`
	}
	if err := json.Unmarshal(payRespBody, &payResult); err != nil {
		t.Fatalf("decode POST checkout-api response: %v", err)
	}
	t.Logf("[Step 3 OK] status=%s payment_id=%s status_detail=%s provider_http_status=%d provider_code=%s provider_request_id=%s",
		payResult.Data.Status, payResult.Data.PaymentID, payResult.Data.StatusDetail,
		payResult.Data.ProviderHTTPStatus, payResult.Data.ProviderCode, payResult.Data.ProviderRequestID)
	if payResult.Data.Status != "approved" || payResult.Data.PaymentID == "" {
		t.Fatalf("expected approved payment with non-empty payment_id, got: %+v", payResult.Data)
	}

	dbRow := runSQL(fmt.Sprintf(`
SELECT c.id, c.estado, COALESCE(c.provider_payment_id, ''), COALESCE(c.provider_payment_status, ''), r.estado
FROM public.checkout_preferencia_comercial c
JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id = c.id
WHERE c.cotizacion_id = '%s';
`, quoteID))
	t.Logf("[Step 4 OK] DB State (checkout_preferencia_comercial + reserva_arriendo_comercial): %s", dbRow)
	if !strings.Contains(dbRow, "|approved|"+payResult.Data.PaymentID+"|approved|pagada") {
		t.Fatalf("expected DB state approved + pagada, got: %s", dbRow)
	}

	// Step 5: If a payment_id was generated, also test the signed webhook receiver POST /api/v1/webhooks/mercadopago
	if payResult.Data.PaymentID != "" {
		webhookSecretBytes, err := os.ReadFile(filepath.Join(rootDir, ".local", "secrets", "mercadopago-webhook-secret"))
		if err != nil {
			t.Fatalf("read mercadopago-webhook-secret: %v", err)
		}
		webhookSecret := strings.TrimSpace(string(webhookSecretBytes))
		reqID := "e2e-webhook-" + uuid.NewString()
		ts := fmt.Sprint(time.Now().Unix())
		manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", payResult.Data.PaymentID, reqID, ts)
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		_, _ = mac.Write([]byte(manifest))
		signature := fmt.Sprintf("ts=%s,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))

		whBody := fmt.Sprintf(`{"type":"payment","action":"payment.updated","user_id":"3452018328","data":{"id":"%s"}}`, payResult.Data.PaymentID)
		whURL := apiBaseURL + "/api/v1/webhooks/mercadopago?data.id=" + url.QueryEscape(payResult.Data.PaymentID) + "&type=payment"
		whReq, err := http.NewRequestWithContext(ctx, http.MethodPost, whURL, strings.NewReader(whBody))
		if err != nil {
			t.Fatal(err)
		}
		whReq.Header.Set("Content-Type", "application/json")
		whReq.Header.Set("x-request-id", reqID)
		whReq.Header.Set("x-signature", signature)
		whResp, err := client.Do(whReq)
		if err != nil {
			t.Fatalf("POST /api/v1/webhooks/mercadopago failed: %v", err)
		}
		whRespBody, _ := io.ReadAll(whResp.Body)
		whResp.Body.Close()
		if whResp.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/v1/webhooks/mercadopago status=%d body=%s", whResp.StatusCode, whRespBody)
		}
		t.Logf("[Step 5 OK] Signed webhook accepted (HTTP %d) for payment_id=%s request_id=%s", whResp.StatusCode, payResult.Data.PaymentID, reqID)
	}
}

func TestLiveCheckoutAPIMultiCardRetryFlow(t *testing.T) {
	if os.Getenv("MPCLI_LIVE_PAYMENT_FLOW_TEST") != "1" {
		t.Skip("set MPCLI_LIVE_PAYMENT_FLOW_TEST=1 to run the live multi-card retry Checkout API flow against :8081 and Mercado Pago Sandbox")
	}
	rootDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	pwBytes, err := os.ReadFile(filepath.Join(rootDir, ".local", "secrets", "runtime_password"))
	if err != nil {
		t.Fatalf("read runtime_password: %v", err)
	}
	runtimePW := strings.TrimSpace(string(pwBytes))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	const (
		apiBaseURL = "http://127.0.0.1:8081"
		spaceID    = "26fc9622-c4a1-4429-9f68-33fb017d21ce"
		ownerID    = "bd4674cd-e0f5-4246-856f-acddf1731aed"
		renterID   = "8b240d82-1107-4e51-bb16-85692407f16f"
		amountCLP  = 24000
	)

	sessionID := uuid.NewString()
	rawBearer := "live-retry-test-token-" + uuid.NewString()
	sum := sha256.Sum256([]byte(rawBearer))
	tokenHash := hex.EncodeToString(sum[:])
	quoteID := uuid.NewString()

	runSQL := func(sqlText string) string {
		cmd := exec.CommandContext(ctx, "psql", "-h", "127.0.0.1", "-p", "15433", "-U", "espacigo_api_login", "-d", "espacigo_payments_local", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1", "-c", "SET ROLE espacigo_runtime; "+sqlText)
		cmd.Env = append(os.Environ(), "PGPASSFILE=/dev/null", "PGPASSWORD="+runtimePW)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("psql failed: %v\noutput: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	dayOffset := 120 + int(time.Now().UnixNano()%60)
	futureDay := time.Now().UTC().AddDate(0, 0, dayOffset)
	startSlot := time.Date(futureDay.Year(), futureDay.Month(), futureDay.Day(), 14, 0, 0, 0, time.UTC)
	endSlot := startSlot.Add(2 * time.Hour)

	runSQL(fmt.Sprintf(`
INSERT INTO public.sesion(id, usuario_id, token_hash, creada_en, ultima_actividad_en, expira_en, cliente_resumen, device_label, ip_masked, user_agent)
VALUES ('%s', '%s', '%s', NOW(), NOW(), NOW() + INTERVAL '30 minutes', '', 'Test Multi-Card Retry', '127.0.•••.•••', 'go-test');
INSERT INTO public.cotizacion_arriendo_comercial(id, espacio_id, propietario_id, arrendatario_id, titulo_espacio, tarifa_version, modalidad, precio_unitario_clp, moneda, unidades, subtotal_clp, inicio, termino, zona_horaria, creada_en, vence_en)
VALUES ('%s', '%s', '%s', '%s', 'Espacio de prueba horario Mercado Pago', 1, 'hora', 12000, 'CLP', 2, %d, '%s', '%s', 'America/Santiago', NOW(), NOW() + INTERVAL '15 minutes');
`, sessionID, renterID, tokenHash, quoteID, spaceID, ownerID, renterID, amountCLP, startSlot.Format(time.RFC3339), endSlot.Format(time.RFC3339)))

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		cleanupSQL := fmt.Sprintf(`
SET ROLE espacigo_runtime;
UPDATE public.cotizacion_arriendo_comercial SET vence_en = NOW() - INTERVAL '1 minute' WHERE id = '%s';
UPDATE public.checkout_preferencia_comercial SET vence_en = NOW() - INTERVAL '1 minute', estado = CASE WHEN estado = 'pending' THEN 'expired' ELSE estado END WHERE cotizacion_id = '%s';
DELETE FROM public.sesion WHERE id = '%s';
`, quoteID, quoteID, sessionID)
		cmd := exec.CommandContext(cleanCtx, "psql", "-h", "127.0.0.1", "-p", "15433", "-U", "espacigo_api_login", "-d", "espacigo_payments_local", "-c", cleanupSQL)
		cmd.Env = append(os.Environ(), "PGPASSFILE=/dev/null", "PGPASSWORD="+runtimePW)
		_ = cmd.Run()
	})

	client := &http.Client{Timeout: 20 * time.Second}

	// 1. Read seller public key
	getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/api/v1/rental-quotes/"+quoteID+"/checkout-api", nil)
	getReq.Header.Set("Authorization", "Bearer "+rawBearer)
	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	var snapshot struct {
		Data Result `json:"data"`
	}
	_ = json.Unmarshal(getBody, &snapshot)
	sellerPublicKey := snapshot.Data.CardTokenization.PublicKey

	tokenizeCard := func(holderName string) string {
		cardNumber := strings.Join([]string{"4509", "9535", "6623", "3704"}, "")
		tokenPayload := map[string]any{
			"card_number":      cardNumber,
			"expiration_month": 11,
			"expiration_year":  2030,
			"security_code":    "123",
			"cardholder": map[string]any{
				"name": holderName,
				"identification": map[string]any{
					"type":   "RUT",
					"number": "123456785",
				},
			},
		}
		b, _ := json.Marshal(tokenPayload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mercadopago.com/v1/card_tokens?public_key="+url.QueryEscape(sellerPublicKey), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var tok struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(rb, &tok)
		if tok.ID == "" {
			t.Fatalf("failed to tokenize card (%s): %s", holderName, rb)
		}
		return tok.ID
	}

	// 2. First Card Attempt: OTHE (rejected by Mercado Pago)
	firstToken := tokenizeCard("OTHE")
	firstPayInput := Input{
		Token:                 firstToken,
		PaymentMethodID:       "visa",
		TokenizationPublicKey: sellerPublicKey,
		Payer: Payer{
			Email:          "buyer@example.com",
			Identification: &Identification{Type: "RUT", Number: "123456785"},
		},
	}
	firstBytes, _ := json.Marshal(firstPayInput)
	firstReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/api/v1/rental-quotes/"+quoteID+"/checkout-api", bytes.NewReader(firstBytes))
	firstReq.Header.Set("Authorization", "Bearer "+rawBearer)
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Idempotency-Key", uuid.NewString())
	firstResp, err := client.Do(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	firstBody, _ := io.ReadAll(firstResp.Body)
	firstResp.Body.Close()
	var firstResult struct {
		Data Result `json:"data"`
	}
	_ = json.Unmarshal(firstBody, &firstResult)
	t.Logf("[Card 1 Result] status=%s status_detail=%s", firstResult.Data.Status, firstResult.Data.StatusDetail)
	if firstResult.Data.Status != "rejected" && firstResult.Data.Status != "provider_error" {
		t.Fatalf("expected Card 1 (OTHE) to be rejected/provider_error, got: %+v", firstResult.Data)
	}

	// 3. Second Card Attempt on the SAME quote: APRO (approved by Mercado Pago)
	secondToken := tokenizeCard("APRO")
	secondPayInput := Input{
		Token:                 secondToken,
		PaymentMethodID:       "visa",
		TokenizationPublicKey: sellerPublicKey,
		Payer: Payer{
			Email:          "buyer@example.com",
			Identification: &Identification{Type: "RUT", Number: "123456785"},
		},
	}
	secondBytes, _ := json.Marshal(secondPayInput)
	secondReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/api/v1/rental-quotes/"+quoteID+"/checkout-api", bytes.NewReader(secondBytes))
	secondReq.Header.Set("Authorization", "Bearer "+rawBearer)
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Idempotency-Key", uuid.NewString())
	secondResp, err := client.Do(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, _ := io.ReadAll(secondResp.Body)
	secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("Card 2 retry returned HTTP %d: %s", secondResp.StatusCode, secondBody)
	}
	var secondResult struct {
		Data Result `json:"data"`
	}
	_ = json.Unmarshal(secondBody, &secondResult)
	t.Logf("[Card 2 Result] status=%s payment_id=%s status_detail=%s", secondResult.Data.Status, secondResult.Data.PaymentID, secondResult.Data.StatusDetail)
	if secondResult.Data.Status != "approved" || secondResult.Data.PaymentID == "" {
		t.Fatalf("expected Card 2 (APRO) retry on same quote to be approved, got: %+v", secondResult.Data)
	}

	// 4. Verify GET /api/v1/commercial-reservations returns status=pagada and checkout_status=approved
	resReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/api/v1/commercial-reservations", nil)
	resReq.Header.Set("Authorization", "Bearer "+rawBearer)
	resResp, err := client.Do(resReq)
	if err != nil {
		t.Fatal(err)
	}
	resBody, _ := io.ReadAll(resResp.Body)
	resResp.Body.Close()
	if !strings.Contains(string(resBody), `"quote_id":"`+quoteID+`"`) || !strings.Contains(string(resBody), `"status":"pagada"`) {
		t.Fatalf("expected GET /api/v1/commercial-reservations to show quote %s as pagada, got: %s", quoteID, resBody)
	}
	t.Logf("[Card 2 Reservation Verified] GET /api/v1/commercial-reservations confirmed status=pagada for quote_id=%s", quoteID)
}
