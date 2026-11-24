import { expect, test } from "@playwright/test";

const quoteId = "00000000-0000-4000-8000-000000000101";
const accountId = "770e8400-e29b-41d4-a716-446655440000";

test("marketplace quote offers Ir a pagar and opens the method chooser", async ({
  page,
}) => {
  const spaceId = "26fc9622-c4a1-4429-9f68-33fb017d21ce";
  const date = "2026-12-15";
  const startAt = "2026-12-15T18:30:00Z";
  const endAt = "2026-12-15T19:30:00Z";

  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/backend/rental-contract-capabilities") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { manual_contracts_enabled: false } }),
      });
      return;
    }
    if (
      pathname === "/api/backend/auth/session" &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
          expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        }),
      });
      return;
    }
    if (pathname === "/api/backend/profile" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          display_name: "Arrendataria de prueba",
          phone: null,
          updated_at: new Date().toISOString(),
        }),
      });
      return;
    }
    if (
      pathname === "/api/backend/profile/photo" &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (
      pathname === "/api/backend/local/early-rentals/capabilities" &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (
      pathname === "/api/backend/rental-quotes" &&
      request.method() === "POST"
    ) {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: quoteId,
            space_id: spaceId,
            space_title: "Espacio de prueba",
            rate_unit: "hora",
            unit_price_clp: 12000,
            currency: "CLP",
            units: 1,
            subtotal_clp: 12000,
            start_at: startAt,
            end_at: endAt,
            time_zone: "America/Santiago",
            created_at: new Date().toISOString(),
            expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
          },
        }),
      });
      return;
    }
    throw new Error(`Unexpected BFF request: ${request.method()} ${pathname}`);
  });

  const selection = new URLSearchParams({
    spaceId,
    date,
    duration: "1",
    start: startAt,
    end: endAt,
    rate_unit: "hora",
    time_zone: "America/Santiago",
    source: "marketplace",
  });
  await page.goto(`/reservas/nueva?${selection.toString()}`);
  await expect(
    page.getByText("Cotización oficial", { exact: true }),
  ).toBeVisible();
  const goToPay = page.getByRole("link", { name: "Ir a pagar" });
  await expect(goToPay).toHaveAttribute(
    "href",
    `/reservas/checkout/opciones/${quoteId}`,
  );
  await goToPay.click();
  await expect(page).toHaveURL(
    new RegExp(`/reservas/checkout/opciones/${quoteId}$`),
  );

  const disclosure = page.locator("details");
  await expect(disclosure).toHaveJSProperty("open", false);
  await disclosure.locator("summary").click();
  await expect(disclosure).toHaveJSProperty("open", true);
  await expect(
    page.getByRole("link", { name: "Pagar con tarjeta" }),
  ).toHaveAttribute("href", `/reservas/checkout/tarjeta/${quoteId}`);
  await expect(
    page.getByRole("button", { name: /Iniciar sesión con Mercado Pago/ }),
  ).toBeVisible();
});

test("wallet choice redirects through its dedicated hosted checkout endpoint", async ({
  page,
}) => {
  const requests: Array<{
    method: string;
    idempotencyKey: string | undefined;
  }> = [];

  await page.route("**/api/backend/auth/session", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        account_id: accountId,
        roles: ["arrendatario"],
        capabilities: [],
        restricted_mode: false,
        expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      }),
    });
  });

  await page.route(
    `**/api/backend/rental-quotes/${quoteId}/checkout-wallet`,
    async (route) => {
      requests.push({
        method: route.request().method(),
        idempotencyKey: route.request().headers()["idempotency-key"],
      });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            operation_id: "00000000-0000-4000-8000-000000000102",
            reservation_id: "00000000-0000-4000-8000-000000000102",
            status: "pending",
            init_point:
              "https://www.mercadopago.cl/checkout/v1/redirect/wallet-test",
            expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
          },
        }),
      });
    },
  );

  await page.route(
    "https://www.mercadopago.cl/checkout/v1/redirect/wallet-test",
    async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "text/html",
        body: "<main>Mercado Pago login mock</main>",
      });
    },
  );

  await page.goto(`/reservas/checkout/opciones/${quoteId}`);
  const disclosure = page.locator("details");
  await expect(disclosure).toHaveJSProperty("open", false);
  await expect(
    page.getByRole("heading", { name: "Elige cómo pagar" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Pagar con tarjeta" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /Iniciar sesión con Mercado Pago/ }),
  ).toHaveCount(0);
  await disclosure.locator("summary").click();
  await expect(disclosure).toHaveJSProperty("open", true);
  const walletButton = page.getByRole("button", {
    name: /Iniciar sesión con Mercado Pago/,
  });
  await expect(walletButton).toBeEnabled();
  await expect(
    page.getByRole("link", { name: "Pagar con tarjeta" }),
  ).toHaveAttribute("href", `/reservas/checkout/tarjeta/${quoteId}`);

  await Promise.all([
    page.waitForURL(
      "https://www.mercadopago.cl/checkout/v1/redirect/wallet-test",
    ),
    walletButton.click(),
  ]);

  expect(requests).toHaveLength(1);
  expect(requests[0].method).toBe("POST");
  expect(requests[0].idempotencyKey).toMatch(
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
  );
  await expect(page.getByText("Mercado Pago login mock")).toBeVisible();
});
