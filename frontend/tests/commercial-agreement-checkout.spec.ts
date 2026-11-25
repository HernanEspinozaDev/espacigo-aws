import { expect, test, type Page } from "@playwright/test";

const spaceId = "10000000-0000-4000-8000-000000000001";
const quoteId = "10000000-0000-4000-8000-000000000002";
const proposalId = "10000000-0000-4000-8000-000000000003";
const agreementId = "10000000-0000-4000-8000-000000000004";
const accountId = "10000000-0000-4000-8000-000000000005";
const reservationId = "10000000-0000-4000-8000-000000000007";
const otpChallengeId = "10000000-0000-4000-8000-000000000008";
const otpCode = "123456";
const snapshot = "a".repeat(64);
const cancellationPolicy = {
  version: "flexible_v1",
  text: "Flexible: reembolso total hasta 24 horas antes del inicio del arriendo.",
};
const future = () => new Date(Date.now() + 3600000).toISOString();
const rentalDate = () =>
  new Date(Date.now() + 3 * 86400000).toISOString().slice(0, 10);
const proposal = () => ({
  id: proposalId,
  version: 1,
  sha256: snapshot,
  quote_id: quoteId,
  space_id: spaceId,
  space_title: "Sala de contrato sintética",
  template_id: "10000000-0000-4000-8000-000000000006",
  template_version: 1,
  location: {
    address: "Dirección sintética 123",
    latitude: -33.4,
    longitude: -70.6,
    revision: 1,
    region_code: "13",
    commune_code: "13101",
  },
  sections: [
    {
      key: "uso",
      title: "Uso permitido",
      body: "Respetar los horarios y devolver el espacio limpio.",
    },
  ],
  rate_unit: "hora",
  rate_version: 1,
  unit_price_clp: 12000,
  units: 1,
  subtotal_clp: 12000,
  currency: "CLP",
  start_at: `${rentalDate()}T18:30:00Z`,
  end_at: `${rentalDate()}T19:30:00Z`,
  time_zone: "America/Santiago",
  cancellation_policy: cancellationPolicy,
  parties: [
    { role: "arrendador", self: false },
    {
      role: "arrendatario",
      self: true,
      legal_name: "Arrendataria Sintética",
      rut: "••••2222",
    },
  ],
  contact_notice:
    "Los datos personales y contactos se habilitan después de confirmar el pago.",
  generated_at: new Date().toISOString(),
});
function agreement(ready = false) {
  return {
    id: agreementId,
    proposal_id: proposalId,
    quote_id: quoteId,
    space_id: spaceId,
    proposal_version: 1,
    snapshot_sha256: snapshot,
    state: ready ? "ready_for_payment" : "pending_landlord",
    expires_at: future(),
    payment_expires_at: ready ? future() : null,
    revision: ready ? 2 : 1,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    signatures: (ready ? ["arrendatario", "arrendador"] : ["arrendatario"]).map(
      (role) => ({
        role,
        signed_at: new Date().toISOString(),
        snapshot_sha256: snapshot,
        proposal_version: 1,
        consent_version: "commercial-agreement-v1",
      }),
    ),
  };
}

async function fixture(
  page: Page,
  {
    capabilitiesError = false,
    failFirstSend = false,
    actor = "arrendatario",
  }: {
    capabilitiesError?: boolean;
    failFirstSend?: boolean;
    actor?: "arrendatario" | "arrendador";
  } = {},
) {
  let submitted = false;
  let ready = false;
  const keys: string[] = [];
  const settlementRequests: string[] = [];
  await page.route("**/api/backend/**", async (route) => {
    const r = route.request(),
      path = new URL(r.url()).pathname;
    let status = 200;
    let body: unknown;
    const headers: Record<string, string> = { "cache-control": "no-store" };
    if (path.endsWith("/auth/session"))
      body = {
        account_id: accountId,
        roles: [actor],
        capabilities: [],
        restricted_mode: false,
        expires_at: future(),
      };
    else if (path === "/api/backend/profile")
      body = {
        display_name: "Arrendataria Sintética",
        phone: null,
        updated_at: new Date().toISOString(),
      };
    else if (path === "/api/backend/rental-contract-capabilities") {
      status = capabilitiesError ? 503 : 200;
      body = capabilitiesError
        ? { error: { code: "service_unavailable" } }
        : { data: { manual_contracts_enabled: true } };
    } else if (path === "/api/backend/host/settlements") {
      const url = new URL(r.url());
      const mode = url.searchParams.get("mode") ?? "live";
      settlementRequests.push(`${mode}:${url.searchParams.get("month") ?? ""}`);
      expect(r.headers()["authorization"]).toBeUndefined();
      const values =
        mode === "test"
          ? {
              gross: "12000",
              base: "12000",
              commission: "0",
              vat: "0",
              fee: "0",
              afterFee: "12000",
              title: "Reserva de prueba",
            }
          : {
              gross: "100000",
              base: "100000",
              commission: "3000",
              vat: "570",
              fee: "3570",
              afterFee: "96430",
              title: "Reserva real",
            };
      const amountFields = {
        gross_clp: values.gross,
        base_clp: values.base,
        commission_clp: values.commission,
        commission_vat_clp: values.vat,
        marketplace_fee_clp: values.fee,
        after_marketplace_fee_clp: values.afterFee,
        processor_fee_clp: null,
        adjustments_clp: null,
        reconciled_net_clp: null,
      };
      body = {
        data: {
          summary: {
            month: url.searchParams.get("month"),
            time_zone: "America/Santiago",
            mode,
            currency: "CLP",
            payment_count: 1,
            ...amountFields,
          },
          items: [
            {
              reservation_id: reservationId,
              space_id: spaceId,
              space_title: values.title,
              provider_reference: "123456",
              verified_at: new Date().toISOString(),
              currency: "CLP",
              test_mode: mode === "test",
              ...amountFields,
              fee_policy:
                mode === "test"
                  ? "no_commission_applied"
                  : "three_percent_plus_vat",
              commission_rate_percent: mode === "test" ? null : 3,
              commission_vat_rate_percent: mode === "test" ? null : 19,
            },
          ],
          next_cursor: null,
        },
      };
    } else if (path === "/api/backend/rental-quotes") {
      status = 201;
      body = {
        data: {
          ...proposal(),
          id: quoteId,
          cancellation_policy: cancellationPolicy,
          created_at: new Date().toISOString(),
          expires_at: future(),
        },
      };
    } else if (path.endsWith("/contract-proposal")) body = { data: proposal() };
    else if (path === `/api/backend/rental-contract-proposals/${proposalId}`)
      body = { data: proposal() };
    else if (
      path === "/api/backend/commercial-agreements/otp-challenge" &&
      r.method() === "POST"
    ) {
      expect(r.postDataJSON()).toEqual({
        proposal_id: proposalId,
        snapshot_sha256: snapshot,
        role: actor,
        ...(actor === "arrendador" ? { agreement_id: agreementId } : {}),
      });
      status = 201;
      body = {
        challenge_id: otpChallengeId,
        channel: "email",
        masked_email: "s***@example.test",
        expires_at: future(),
      };
    } else if (
      path === "/api/backend/commercial-agreements" &&
      r.method() === "GET"
    )
      body =
        actor === "arrendador"
          ? { items: [agreement(ready)] }
          : { items: submitted ? [agreement(ready)] : [] };
    else if (
      path === "/api/backend/commercial-agreements" &&
      r.method() === "POST"
    ) {
      expect(r.postDataJSON()).toEqual({
        proposal_id: proposalId,
        snapshot_sha256: snapshot,
        consent: true,
        consent_version: "commercial-agreement-v1",
        otp_challenge_id: otpChallengeId,
        otp_code: otpCode,
      });
      keys.push(r.headers()["idempotency-key"]);
      expect(r.headers()["authorization"]).toBeUndefined();
      if (failFirstSend && keys.length === 1) {
        status = 503;
        body = { error: { code: "service_unavailable" } };
      } else {
        submitted = true;
        status = 201;
        body = agreement();
        headers.etag = `"commercial-agreement-${agreementId}-1"`;
      }
    } else if (path === `/api/backend/commercial-agreements/${agreementId}`) {
      body = agreement(ready);
      headers.etag = `"commercial-agreement-${agreementId}-${ready ? 2 : 1}"`;
    } else if (
      actor === "arrendador" &&
      path ===
        `/api/backend/commercial-agreements/${agreementId}/landlord-signature` &&
      r.method() === "PUT"
    ) {
      expect(r.postDataJSON()).toEqual({
        snapshot_sha256: snapshot,
        consent: true,
        consent_version: "commercial-agreement-v1",
        otp_challenge_id: otpChallengeId,
        otp_code: otpCode,
      });
      expect(r.headers()["if-match"]).toBe(
        `"commercial-agreement-${agreementId}-1"`,
      );
      expect(r.headers()["authorization"]).toBeUndefined();
      ready = true;
      body = agreement(true);
      headers.etag = `"commercial-agreement-${agreementId}-2"`;
    } else {
      status = 404;
      body = { error: { code: "not_found" } };
    }
    await route.fulfill({
      status,
      headers,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return {
    keys,
    settlementRequests,
    accept() {
      ready = true;
    },
  };
}
function selection() {
  return new URLSearchParams({
    source: "marketplace",
    spaceId,
    date: rentalDate(),
    duration: "1",
    start: `${rentalDate()}T18:30:00Z`,
    end: `${rentalDate()}T19:30:00Z`,
    rate_unit: "hora",
    time_zone: "America/Santiago",
  });
}

test("renter reviews the immutable proposal, explicitly signs, and resumes only after the second signature", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.goto(`/reservas/nueva?${selection()}`);
  await expect(
    page.getByText(cancellationPolicy.text, { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Revisar y firmar contrato", exact: true })
    .click();
  await expect(
    page.getByText(/ubicado en Dirección sintética 123/),
  ).toBeVisible();
  await expect(
    page.getByText("SEGUNDO. OBJETO Y USO PERMITIDO:", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText(cancellationPolicy.text)).toHaveCount(2);
  const requestCode = page.getByRole("button", {
    name: "Enviar código de firma al correo",
  });
  await expect(requestCode).toBeDisabled();
  await page.getByRole("checkbox").check();
  await requestCode.click();
  await expect(page.getByText("Código enviado a")).toBeVisible();
  const send = page.getByRole("button", {
    name: "Confirmar y firmar (FES)",
  });
  await expect(send).toBeDisabled();
  await page.getByPlaceholder("Código de 6 dígitos").fill(otpCode);
  await expect(send).toBeEnabled();
  await send.click();
  await expect(page.getByRole("status")).toHaveText(
    "Esperando la firma del anfitrión",
  );
  await expect(
    page.getByRole("link", { name: "Ir a pagar", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Pagar con Mercado Pago", exact: true }),
  ).toHaveCount(0);
  f.accept();
  await page
    .getByRole("button", { name: "Actualizar estado", exact: true })
    .click();
  await expect(
    page.getByText("Firmado por ambas partes", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.getByText(/Plazo para pagar:/)).toBeVisible();
  const pay = page
    .getByRole("button", { name: "Pagar con Mercado Pago", exact: true })
    .or(page.getByRole("link", { name: "Ir a pagar", exact: true }));
  await expect(pay).toBeVisible();
  await page.getByRole("link", { name: "Volver a esta solicitud" }).click();
  await expect(page).toHaveURL(
    new RegExp(`/reservas/contratos/${agreementId}$`),
  );
  await expect(
    page.getByText("Firmado por ambas partes", { exact: true }).first(),
  ).toBeVisible();
});

test("capability lookup failure blocks the legacy pay button", async ({
  page,
}) => {
  await fixture(page, { capabilitiesError: true });
  await page.goto(`/reservas/nueva?${selection()}`);
  await expect(
    page.getByText(/No pudimos comprobar los requisitos del contrato/),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Ir a pagar", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Pagar con Mercado Pago", exact: true }),
  ).toHaveCount(0);
});

test("an uncertain send retries the same signed payload and idempotency key", async ({
  page,
}) => {
  const f = await fixture(page, { failFirstSend: true });
  await page.goto(`/reservas/nueva?${selection()}`);
  await page
    .getByRole("button", { name: "Revisar y firmar contrato", exact: true })
    .click();
  await page.getByRole("checkbox").check();
  await page
    .getByRole("button", { name: "Enviar código de firma al correo" })
    .click();
  await page.getByPlaceholder("Código de 6 dígitos").fill(otpCode);
  await page.getByRole("button", { name: "Confirmar y firmar (FES)" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "No pudimos confirmar" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Confirmar y firmar (FES)" }).click();
  await expect(page.getByRole("status")).toHaveText(
    "Esperando la firma del anfitrión",
  );
  expect(f.keys).toHaveLength(2);
  expect(f.keys[0]).toBe(f.keys[1]);
});

test("landlord reviews the private proposal and signs it before both signatures unlock payment", async ({
  page,
}) => {
  await fixture(page, { actor: "arrendador" });
  await page.goto("/anfitrion/reservas");
  await page
    .getByRole("button", { name: "Revisar y firmar", exact: true })
    .click();

  await expect(
    page.getByRole("heading", { name: "Revisión del contrato (Arrendador)" }),
  ).toBeVisible();
  await expect(page.getByText("Dirección sintética 123")).toBeVisible();
  await expect(page.getByText("••••2222", { exact: true })).toBeVisible();
  await expect(
    page.getByText(/datos completos se completan al confirmar el pago/),
  ).toBeVisible();
  await expect(page.getByText(/@|\+56\s?9/)).toHaveCount(0);

  const requestCode = page.getByRole("button", {
    name: "Enviar código de firma al correo",
  });
  await expect(requestCode).toBeDisabled();
  await page.getByRole("checkbox").check();
  await requestCode.click();
  await expect(page.getByText("Código enviado a")).toBeVisible();
  const sign = page.getByRole("button", { name: "Aceptar y firmar (FES)" });
  await expect(sign).toBeDisabled();
  await page.getByPlaceholder("Código de 6 dígitos").fill(otpCode);
  await expect(sign).toBeEnabled();
  await sign.click();

  await expect(
    page
      .getByRole("status")
      .filter({
        hasText:
          "Firma registrada. El horario quedó retenido hasta el plazo de pago indicado.",
      })
      .first(),
  ).toHaveText(
    "Firma registrada. El horario quedó retenido hasta el plazo de pago indicado.",
  );
  await expect(
    page.getByText("Firmado por ambas partes", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.getByText(/Firmado el .*· v1/)).toHaveCount(2);
  await expect(page.getByText(/Plazo de pago:/)).toBeVisible();
});

test("landlord monthly settlements preserve unknown amounts and separate live from test", async ({
  page,
}) => {
  const f = await fixture(page, { actor: "arrendador" });
  await page.goto("/anfitrion/liquidaciones");

  await expect(
    page.getByRole("heading", { name: "Liquidaciones" }),
  ).toBeVisible();
  await expect(page.getByText("$100.000 CLP").first()).toBeVisible();
  await expect(page.getByText("$3.570 CLP").first()).toBeVisible();
  await expect(page.getByText("$3.000 CLP").first()).toBeVisible();
  await expect(page.getByText("$570 CLP").first()).toBeVisible();
  await expect(page.getByText("$96.430 CLP").first()).toBeVisible();
  await expect(page.getByText("No disponible").first()).toBeVisible();
  const liveReservation = page.getByRole("link", { name: "Reserva real" });
  await expect(liveReservation).toHaveAttribute(
    "href",
    `/anfitrion/reservas/${reservationId}`,
  );

  await page.getByLabel("Entorno de pago").selectOption("test");
  await expect(
    page.getByRole("link", { name: "Reserva de prueba" }),
  ).toBeVisible();
  await expect(page.getByText("$12.000 CLP").first()).toBeVisible();
  await expect(page.getByText("$0 CLP").first()).toBeVisible();
  expect(
    f.settlementRequests.some((request) => request.startsWith("live:")),
  ).toBe(true);
  expect(
    f.settlementRequests.some((request) => request.startsWith("test:")),
  ).toBe(true);
});

test("isolated application serves access, static assets and readiness", async ({
  page,
  request,
}) => {
  const ready = await request.get("/api/ready");
  expect(ready.status()).toBe(200);
  expect(await ready.json()).toEqual({ status: "ready" });
  const assets: number[] = [];
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.startsWith("/_next/static/")) {
      assets.push(response.status());
    }
  });
  await page.goto("/acceso", { waitUntil: "load" });
  await expect(page).toHaveURL(/\/acceso$/);
  await expect(page.getByRole("main")).toBeVisible();
  expect(assets.length).toBeGreaterThan(0);
  expect(assets.every((status) => status >= 200 && status < 400)).toBe(true);
});
