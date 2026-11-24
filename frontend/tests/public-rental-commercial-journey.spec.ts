import { expect, test } from "@playwright/test";
import type { components } from "../src/lib/api/generated/openapi";
import { createBffFixtureState } from "./helpers/bff-fixture";

const spaceId = "00000000-0000-4000-8000-000000000001";
const quoteId = "00000000-0000-4000-8000-000000000101";
const accountId = "770e8400-e29b-41d4-a716-446655440000";

function nextScheduledRentalDate(): string {
  const parts = new Intl.DateTimeFormat("en", {
    timeZone: "America/Santiago",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const year = Number(parts.find((part) => part.type === "year")?.value);
  const month = Number(parts.find((part) => part.type === "month")?.value);
  const day = Number(parts.find((part) => part.type === "day")?.value);
  const date = new Date(Date.UTC(year, month - 1, day));
  const daysUntilTuesday = (2 - date.getUTCDay() + 7) % 7 || 7;
  date.setUTCDate(date.getUTCDate() + daysUntilTuesday);
  return date.toISOString().slice(0, 10);
}

test("public rental journey returns from login and requests an official quote", async ({
  page,
  context,
}) => {
  const hydrationWarnings: string[] = [];
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      /hydration|did not match|server rendered html/i.test(message.text())
    ) {
      hydrationWarnings.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    if (/hydration|did not match|server rendered html/i.test(error.message)) {
      hydrationWarnings.push(error.message);
    }
  });
  const bff = createBffFixtureState();
  const currentMonthParts = new Intl.DateTimeFormat("en", {
    timeZone: "America/Santiago",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(new Date());
  const currentMonth = `${currentMonthParts.find((part) => part.type === "year")?.value}-${currentMonthParts.find((part) => part.type === "month")?.value}`;
  const rentalDate = nextScheduledRentalDate();
  // The synthetic space is open Tuesdays from 14:15–16:45 America/Santiago.
  // 18:30Z falls inside that window in both Chilean DST offsets.
  const startAt = `${rentalDate}T18:30:00Z`;
  const endAt = `${rentalDate}T19:30:00Z`;
  const quoteRequests: Array<Record<string, unknown>> = [];
  const previewRequests: string[] = [];
  const browserAuthHeaders: Array<string | undefined> = [];
  const activeAccountId = accountId;

  await context.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const method = request.method();
    let result: {
      status: number;
      body: unknown;
      headers?: Record<string, string>;
    } = { status: 404, body: { error: { code: "not_found" } } };

    if (pathname === "/api/backend/rental-contract-capabilities") {
      result = {
        status: 200,
        body: { data: { manual_contracts_enabled: false } },
      };
    } else if (pathname === "/api/backend/auth/logout" && method === "POST") {
      result = { status: 204, body: undefined };
    } else if (pathname === "/api/backend/auth/login" && method === "POST") {
      result = {
        status: 200,
        body: {
          account_id: activeAccountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
          expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      };
    } else if (pathname === "/api/backend/auth/session" && method === "GET") {
      result = {
        status: 200,
        body: {
          account_id: activeAccountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
        },
      };
    } else if (
      pathname === `/api/backend/spaces/${spaceId}/rental-availability` &&
      method === "GET"
    ) {
      const selectedDate = url.searchParams.get("date");
      if (!selectedDate) throw new Error("Availability request omitted date");
      const availability = {
        space_id: spaceId,
        time_zone: "America/Santiago",
        rate_unit: "hora",
        checked_at: new Date().toISOString(),
        items: [{ start_at: startAt, end_at: endAt }],
      } satisfies components["schemas"]["PublicRentalAvailability"];
      result = { status: 200, body: { data: availability } };
    } else if (
      pathname === `/api/backend/public/spaces/${spaceId}/month` &&
      method === "GET"
    ) {
      const month = url.searchParams.get("month");
      if (!month) throw new Error("Month availability request omitted month");
      result = {
        status: 200,
        body: {
          month,
          time_zone: "America/Santiago",
          rate_unit: "hora",
          base_price_clp: 12000,
          days:
            rentalDate.slice(0, 7) === month
              ? [
                  {
                    date: rentalDate,
                    status: "AVAILABLE",
                    price_clp: 12000,
                    slots: [],
                  },
                ]
              : [],
        },
      };
    } else if (pathname === "/api/backend/rental-quotes" && method === "POST") {
      browserAuthHeaders.push(request.headers()["authorization"]);
      const body = request.postDataJSON() as {
        space_id: string;
        start_at: string;
        end_at: string;
      };
      quoteRequests.push(body);
      const quote = {
        id: quoteId,
        space_id: spaceId,
        space_title: "Espacio sintético",
        rate_unit: "hora",
        unit_price_clp: 12000,
        currency: "CLP",
        units: 1,
        subtotal_clp: 12000,
        start_at: body.start_at,
        end_at: body.end_at,
        time_zone: "America/Santiago",
        created_at: new Date().toISOString(),
        expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
      } satisfies components["schemas"]["RentalQuote"];
      result = { status: 201, body: { data: quote } };
    } else if (
      pathname === `/api/backend/rental-quotes/${quoteId}/contract-preview` &&
      method === "POST"
    ) {
      browserAuthHeaders.push(request.headers()["authorization"]);
      previewRequests.push(pathname);
      const preview = {
        state: "preview",
        quote_id: quoteId,
        space_id: spaceId,
        space_title: "Espacio sintético",
        rate_unit: "hora",
        unit_price_clp: 12000,
        units: 1,
        subtotal_clp: 12000,
        currency: "CLP",
        start_at: startAt,
        end_at: endAt,
        time_zone: "America/Santiago",
        parties: [
          {
            role: "arrendador",
            self: false,
          },
          {
            role: "arrendatario",
            self: true,
            legal_name: "Arrendataria Sintética",
            rut: "••••4321",
          },
        ],
        contact_notice:
          "Los datos personales y contactos se habilitan después de confirmar el pago.",
        generated_at: new Date().toISOString(),
      } satisfies components["schemas"]["RentalContractPreview"];
      result = { status: 200, body: { data: preview } };
    }

    const adapted = bff.adapt(request, result);
    if (!adapted)
      throw new Error(`No controlled response for ${method} ${pathname}`);
    await route.fulfill({
      status: adapted.status,
      headers: {
        "cache-control": "no-store",
        ...(adapted.contentType
          ? { "content-type": adapted.contentType }
          : adapted.status !== 204
            ? { "content-type": "application/json; charset=utf-8" }
            : {}),
        ...(adapted.headers ?? {}),
      },
      ...(adapted.status === 204
        ? {}
        : { body: JSON.stringify(adapted.body ?? {}) }),
    });
  });

  await page.goto(`/espacios/${spaceId}`);
  await expect(
    page.getByRole("heading", { name: "Espacio sintético", level: 1 }),
  ).toBeVisible();

  const availability = page
    .getByRole("heading", { name: "Disponibilidad para arriendo" })
    .locator("xpath=ancestor::section[1]");
  const dayNumber = Number(rentalDate.slice(-2));
  if (rentalDate.slice(0, 7) !== currentMonth) {
    await availability.getByRole("button", { name: "Mes siguiente" }).click();
  }
  await availability.getByRole("button", { name: "1 hora" }).click();
  await availability
    .getByRole("button", {
      name: new RegExp(`^${dayNumber}\\b`),
    })
    .click();
  await expect(
    availability.getByRole("button", {
      name: /(?:\d{2}:\d{2}\s*(?:a|–)\s*\d{2}:\d{2})/,
    }),
  ).toBeVisible();
  await availability
    .getByRole("button", {
      name: /(?:\d{2}:\d{2}\s*(?:a|–)\s*\d{2}:\d{2})/,
    })
    .click();
  await page.getByRole("link", { name: /^Solicitar arriendo/ }).click();

  await expect(page).toHaveURL(/\/reservas\/nueva\?.*source=marketplace/);
  await page
    .getByRole("navigation", { name: "Opciones para esta selección" })
    .getByRole("link", { name: "Iniciar sesión", exact: true })
    .click();

  await expect(page).toHaveURL(/\/acceso\?return_to=/);
  const accessUrl = new URL(page.url());
  const returnUrl = new URL(
    accessUrl.searchParams.get("return_to") ?? "",
    accessUrl.origin,
  );
  expect(returnUrl.pathname).toBe("/reservas/nueva");
  expect(returnUrl.searchParams.get("source")).toBe("marketplace");
  expect(returnUrl.searchParams.get("date")).toBe(rentalDate);
  expect(returnUrl.searchParams.get("duration")).toBe("1");
  expect(returnUrl.searchParams.get("start")).toBe(startAt);
  expect(returnUrl.searchParams.get("end")).toBe(endAt);
  await page.getByLabel("Correo electrónico").fill("renter@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticPass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/reservas/nueva");
  // Authenticated marketplace checkout requests the quote automatically.
  await expect(
    page.getByText("Cotización oficial", { exact: true }),
  ).toBeVisible();
  const payButton = page.getByRole("button", {
    name: "Pagar con Mercado Pago",
  });
  const payLink = page.getByRole("link", { name: "Pagar con Mercado Pago" });
  const payAction = payButton.or(payLink);
  await expect(payAction).toBeVisible();
  await expect(payAction).toHaveCSS("background-color", "rgb(164, 71, 49)");
  await expect(payAction).toHaveCSS("color", "rgb(255, 255, 255)");
  if (await payLink.isVisible()) {
    await expect(payLink).toHaveAttribute(
      "href",
      `/reservas/checkout/tarjeta/${quoteId}`,
    );
  }
  expect(quoteRequests).toHaveLength(1);
  expect(quoteRequests[0]).toEqual({
    space_id: spaceId,
    start_at: startAt,
    end_at: endAt,
  });
  expect(browserAuthHeaders).toEqual([undefined]);
  expect(hydrationWarnings).toEqual([]);
});

test("Checkout API card page is clear and hydration-stable before login", async ({
  page,
}) => {
  const hydrationWarnings: string[] = [];
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      /hydration|did not match|server rendered html/i.test(message.text())
    ) {
      hydrationWarnings.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    if (/hydration|did not match|server rendered html/i.test(error.message)) {
      hydrationWarnings.push(error.message);
    }
  });
  const sdkRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith("https://sdk.mercadopago.com/")) {
      sdkRequests.push(request.url());
    }
  });
  await page.route("**/api/backend/**", async (route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "unauthenticated" } }),
    });
  });

  await page.goto(
    "/reservas/checkout/tarjeta/00000000-0000-4000-8000-000000000101",
  );

  await expect(
    page.getByRole("heading", { name: "Pagar con tarjeta", level: 1 }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("alert")
      .getByText(
        "Inicia sesión para continuar con el pago de esta cotización.",
      ),
  ).toBeVisible();
  await expect(page.getByLabel("El número de tarjeta")).toHaveCount(0);
  expect(sdkRequests).toEqual([]);
  expect(hydrationWarnings).toEqual([]);
});

test("Checkout API requires host reconnection when seller tokenization key is missing", async ({
  page,
}) => {
  const quoteId = "00000000-0000-4000-8000-000000000101";
  const sdkRequests: string[] = [];
  let paymentPosts = 0;
  page.on("request", (request) => {
    if (request.url().startsWith("https://sdk.mercadopago.com/")) {
      sdkRequests.push(request.url());
    }
  });
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/backend/auth/session") {
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
    if (pathname === "/api/backend/profile") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ display_name: "Arrendataria Sintética" }),
      });
      return;
    }
    if (
      pathname === "/api/backend/profile/photo" ||
      pathname === "/api/backend/local/early-rentals/capabilities"
    ) {
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (pathname === `/api/backend/rental-quotes/${quoteId}/checkout-api`) {
      if (request.method() === "POST") paymentPosts += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            amount_clp: 12000,
            status: "not_started",
            card_tokenization: {
              mode: "seller_public_key",
              reconnect_required: true,
            },
          },
        }),
      });
      return;
    }
    throw new Error(`Unexpected BFF request: ${request.method()} ${pathname}`);
  });

  await page.goto(`/reservas/checkout/tarjeta/${quoteId}`);
  if (
    await page
      .getByText("Checkout API está desactivado en este entorno.")
      .isVisible()
  ) {
    test.skip(true, "El feature Checkout API no está activo en este entorno.");
  }
  await expect(
    page.getByText("Vuelve a vincular Mercado Pago", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(/El anfitrión debe volver a autorizar su cuenta/),
  ).toBeVisible();
  await expect(page.getByLabel("El número de tarjeta")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Pagar con tarjeta" }),
  ).toHaveCount(0);
  expect(sdkRequests).toEqual([]);
  expect(paymentPosts).toBe(0);
});

test("Checkout API submits only the tokenized payment data to the BFF", async ({
  page,
}) => {
  const hydrationWarnings: string[] = [];
  const submittedPayloads: unknown[] = [];
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      /hydration|did not match|server rendered html/i.test(message.text())
    ) {
      hydrationWarnings.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    if (/hydration|did not match|server rendered html/i.test(error.message)) {
      hydrationWarnings.push(error.message);
    }
  });
  const quoteId = "00000000-0000-4000-8000-000000000101";
  await page.route("https://sdk.mercadopago.com/js/v2", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: `class MercadoPagoMock {
        constructor(publicKey) { document.body.dataset.mpKeyUsed = publicKey; }
        cardForm(options) {
          for (const id of ["form-checkout-api__cardNumber", "form-checkout-api__expirationDate", "form-checkout-api__securityCode"]) {
            const host = document.getElementById(id);
            const iframe = document.createElement("iframe");
            iframe.title = "Campo seguro simulado";
            host.appendChild(iframe);
          }
          const identificationType = document.getElementById("form-checkout-api__identificationType");
          const otherOption = document.createElement("option");
          otherOption.value = "OTHER";
          otherOption.textContent = "Otro";
          identificationType.appendChild(otherOption);
          identificationType.value = "OTHER";
          document.getElementById(options.form.id).addEventListener("submit", (event) => options.callbacks.onSubmit(event));
          window.setTimeout(() => options.callbacks.onFormMounted(null), 0);
          return { getCardFormData: () => ({ token: "synthetic-single-use-token", paymentMethodId: "visa", cardholderEmail: "buyer@example.invalid", identificationType: "OTHER", identificationNumber: "123456789" }) };
        }
      }
      window.MercadoPago = MercadoPagoMock;`,
    });
  });
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
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
          display_name: "Arrendataria Sintética",
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
      pathname === "/api/backend/rental-contract-capabilities" &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { manual_contracts_enabled: false } }),
      });
      return;
    }
    if (
      pathname === `/api/backend/rental-quotes/${quoteId}/checkout-api` &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            amount_clp: 12000,
            status: "not_started",
            space_title: "Espacio de prueba horario Mercado Pago",
            start_at: "2026-11-24T18:30:00Z",
            end_at: "2026-11-24T19:30:00Z",
            time_zone: "America/Santiago",
            expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
            units: 1,
            rate_unit: "hora",
            card_tokenization: {
              mode: "seller_public_key",
              public_key: "public-key-test-fixture",
            },
          },
        }),
      });
      return;
    }
    if (
      pathname === `/api/backend/rental-quotes/${quoteId}/checkout-api` &&
      request.method() === "POST"
    ) {
      const payload = request.postDataJSON();
      submittedPayloads.push(payload);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            amount_clp: 12000,
            status: "provider_error",
            provider_http_status: 400,
            provider_code: "2067",
          },
        }),
      });
      return;
    }
    throw new Error(`Unexpected BFF request: ${request.method()} ${pathname}`);
  });

  await page.goto(`/reservas/checkout/tarjeta/${quoteId}`);
  if (
    await page
      .getByText("Checkout API está desactivado en este entorno.")
      .isVisible()
  ) {
    test.skip(true, "El feature Checkout API no está activo en el Next local.");
  }
  await expect(page.getByText("Modo de prueba", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Pagar con tarjeta", level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByText("Espacio de prueba horario Mercado Pago", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("body")).toHaveAttribute(
    "data-mp-key-used",
    "public-key-test-fixture",
  );
  await expect(
    page.getByText("America/Santiago", { exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Titular de la tarjeta")).toBeVisible();
  await expect(page.getByLabel("Tipo de documento")).toHaveValue("OTHER");
  await expect(page.getByLabel("Correo del pagador")).toHaveAttribute(
    "autocomplete",
    "off",
  );
  await page.getByLabel("Titular de la tarjeta").fill("Comprador Sintético");
  await page.getByLabel("Número de documento").fill("12.345.678-9");
  await page.getByLabel("Correo del pagador").fill("buyer@example.invalid");
  const payButton = page.getByRole("button", { name: "Pagar con tarjeta" });
  await expect(payButton).toBeEnabled();
  await payButton.click();
  await expect(
    page.getByText(
      "Mercado Pago no pudo validar el medio de pago o los datos del pagador. Revisa la tarjeta y el documento antes de iniciar otra cotización.",
    ),
  ).toBeVisible();
  await expect(payButton).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Consultar estado" }),
  ).toBeVisible();
  expect(submittedPayloads).toEqual([
    {
      token: "synthetic-single-use-token",
      payment_method_id: "visa",
      tokenization_public_key: "public-key-test-fixture",
      payer: {
        email: "buyer@example.invalid",
        identification: { type: "OTHER", number: "123456789" },
      },
    },
  ]);
  expect(hydrationWarnings).toEqual([]);
});

test("Checkout API warns against resubmitting an uncertain result", async ({
  page,
}) => {
  const quoteId = "00000000-0000-4000-8000-000000000101";
  const sdkRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith("https://sdk.mercadopago.com/")) {
      sdkRequests.push(request.url());
    }
  });
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/backend/auth/session") {
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
    if (pathname === "/api/backend/profile") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          display_name: "Arrendataria Sintética",
          phone: null,
          updated_at: new Date().toISOString(),
        }),
      });
      return;
    }
    if (
      pathname === "/api/backend/profile/photo" ||
      pathname === "/api/backend/local/early-rentals/capabilities"
    ) {
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (pathname === `/api/backend/rental-quotes/${quoteId}/checkout-api`) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            amount_clp: 12000,
            status: "reconciliation_required",
          },
        }),
      });
      return;
    }
    throw new Error(`Unexpected BFF request: ${request.method()} ${pathname}`);
  });

  await page.goto(`/reservas/checkout/tarjeta/${quoteId}`);
  if (
    await page
      .getByText("Checkout API está desactivado en este entorno.")
      .isVisible()
  ) {
    test.skip(true, "El feature Checkout API no está activo en el Next local.");
  }
  await expect(
    page.getByText(
      "EspaciGo está verificando el resultado con Mercado Pago. No vuelvas a enviar el pago.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Consultar estado" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Pagar con tarjeta" }),
  ).toHaveCount(0);
  expect(sdkRequests).toEqual([]);
});

test("Checkout API exposes a safe retry when CardForm initialization throws", async ({
  page,
}) => {
  const quoteId = "00000000-0000-4000-8000-000000000101";
  let paymentPosts = 0;
  await page.route("https://sdk.mercadopago.com/js/v2", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: `window.MercadoPago = class MercadoPagoMock {
        constructor(publicKey) { document.body.dataset.mpKeyUsed = publicKey; }
        cardForm() { throw new TypeError("blocked secure fields"); }
      };`,
    });
  });
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/api/backend/auth/session") {
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
    if (pathname === "/api/backend/profile") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          display_name: "Arrendataria Sintética",
          phone: null,
          updated_at: new Date().toISOString(),
        }),
      });
      return;
    }
    if (
      pathname === "/api/backend/profile/photo" ||
      pathname === "/api/backend/local/early-rentals/capabilities"
    ) {
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (pathname === `/api/backend/rental-quotes/${quoteId}/checkout-api`) {
      if (request.method() === "POST") paymentPosts += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            quote_id: quoteId,
            amount_clp: 12000,
            status: "not_started",
            space_title: "Espacio de prueba",
            start_at: "2026-11-24T18:30:00Z",
            end_at: "2026-11-24T19:30:00Z",
            time_zone: "America/Santiago",
            expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
            units: 1,
            rate_unit: "hora",
          },
        }),
      });
      return;
    }
    throw new Error(`Unexpected BFF request: ${request.method()} ${pathname}`);
  });

  await page.goto(`/reservas/checkout/tarjeta/${quoteId}`);
  if (
    await page
      .getByText("Checkout API está desactivado en este entorno.")
      .isVisible()
  ) {
    test.skip(true, "El feature Checkout API no está activo en este entorno.");
  }
  await expect(page.locator("body")).toHaveAttribute(
    "data-mp-key-used",
    /^TEST-/,
  );
  await expect(
    page.getByText(
      "Mercado Pago no pudo inicializar los campos seguros. Recarga el checkout; si continúa, permite sdk.mercadopago.com y sus campos seguros en tu navegador.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Volver a abrir el checkout" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Pagar con tarjeta" }),
  ).toBeDisabled();
  expect(paymentPosts).toBe(0);

  const reload = page.waitForLoadState("domcontentloaded");
  await page
    .getByRole("button", { name: "Volver a abrir el checkout" })
    .click();
  await reload;
  await expect(
    page.getByRole("button", { name: "Volver a abrir el checkout" }),
  ).toBeVisible();
  expect(paymentPosts).toBe(0);
});

test("marketplace quote handoff shows a high-contrast payment action", async ({
  page,
}) => {
  const hydrationWarnings: string[] = [];
  page.on("console", (message) => {
    if (
      message.type() === "error" &&
      /hydration|did not match|server rendered html/i.test(message.text())
    ) {
      hydrationWarnings.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    if (/hydration|did not match|server rendered html/i.test(error.message)) {
      hydrationWarnings.push(error.message);
    }
  });
  const spaceId = "26fc9622-c4a1-4429-9f68-33fb017d21ce";
  const quoteId = "00000000-0000-4000-8000-000000000101";
  const rentalDate = nextScheduledRentalDate();
  const startAt = `${rentalDate}T18:30:00Z`;
  const endAt = `${rentalDate}T19:30:00Z`;
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
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
          display_name: "Arrendataria Sintética",
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
      pathname === "/api/backend/rental-contract-capabilities" &&
      request.method() === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { manual_contracts_enabled: false } }),
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
            space_title: "Espacio de prueba horario Mercado Pago",
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

  const query = new URLSearchParams({
    spaceId,
    date: rentalDate,
    duration: "1",
    start: startAt,
    end: endAt,
    rate_unit: "hora",
    time_zone: "America/Santiago",
    source: "marketplace",
  });
  await page.goto(`/reservas/nueva?${query.toString()}`);
  await expect(
    page.getByText("Cotización oficial", { exact: true }),
  ).toBeVisible();
  const payButton = page.getByRole("button", {
    name: "Pagar con Mercado Pago",
  });
  const payLink = page.getByRole("link", { name: "Pagar con Mercado Pago" });
  const payAction = payButton.or(payLink);
  await expect(payAction).toBeVisible();
  await expect(payAction).toHaveCSS("background-color", "rgb(164, 71, 49)");
  await expect(payAction).toHaveCSS("color", "rgb(255, 255, 255)");
  if (await payLink.isVisible()) {
    await expect(payLink).toHaveAttribute(
      "href",
      `/reservas/checkout/tarjeta/${quoteId}`,
    );
    await expect(
      page.getByText(
        "Completarás el pago en EspaciGo. Mercado Pago procesa la transacción.",
      ),
    ).toBeVisible();
  }
  expect(hydrationWarnings).toEqual([]);
});

test("Checkout Pro return trusts backend status instead of Mercado Pago query", async ({
  page,
}) => {
  const bff = createBffFixtureState();
  let status = "pending";
  let reads = 0;
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    let result: { status: number; body: unknown } = {
      status: 404,
      body: { error: { code: "not_found" } },
    };
    if (pathname === "/api/backend/auth/session") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
        },
      };
    } else if (pathname === "/api/backend/auth/login") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
          expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      };
    } else if (
      pathname === "/api/backend/rental-contract-capabilities" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: { data: { manual_contracts_enabled: false } },
      };
    } else if (
      pathname === `/api/backend/rental-quotes/${quoteId}/checkout` &&
      request.method() === "GET"
    ) {
      reads += 1;
      result = {
        status: 200,
        body: {
          data: {
            quote_id: quoteId,
            operation_id: "00000000-0000-4000-8000-000000000201",
            status,
            ...(status === "pending"
              ? {
                  init_point:
                    "https://sandbox.mercadopago.cl/checkout/v1/redirect",
                }
              : {}),
            updated_at: new Date().toISOString(),
          },
        },
      };
    }
    const adapted = bff.adapt(request, result);
    if (!adapted) throw new Error(`Uncontrolled request: ${request.url()}`);
    await route.fulfill({
      status: adapted.status,
      headers: {
        "cache-control": "no-store",
        "content-type": "application/json; charset=utf-8",
        ...(adapted.headers ?? {}),
      },
      body: JSON.stringify(adapted.body ?? {}),
    });
  });

  await page.goto(
    `/acceso?return_to=${encodeURIComponent(`/reservas/checkout/${quoteId}?collection_status=approved&status=approved`)}`,
  );
  await page.getByLabel("Correo electrónico").fill("renter@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticPass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/mi-cuenta");
  await page.goto(
    `/reservas/checkout/${quoteId}?collection_status=approved&status=approved`,
  );
  await expect(page.getByText("Pago pendiente de confirmación")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Volver a Mercado Pago" }),
  ).toBeVisible();
  expect(reads).toBe(1);

  status = "approved";
  await page.getByRole("button", { name: "Actualizar estado" }).click();
  await expect(page.getByText("Pago confirmado")).toBeVisible();
  expect(reads).toBe(2);
});

test("Checkout Pro start reuses its idempotency key and redirects to backend URL", async ({
  page,
  context,
}) => {
  const bff = createBffFixtureState();
  const startKeys: Array<string | undefined> = [];
  let starts = 0;
  let quoteExpiryOverride: string | null = null;
  const startAt = "2030-01-16T12:00:00Z";
  const endAt = "2030-01-16T13:00:00Z";
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    let result: { status: number; body: unknown } = {
      status: 404,
      body: { error: { code: "not_found" } },
    };
    if (pathname === "/api/backend/auth/session") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
        },
      };
    } else if (pathname === "/api/backend/auth/login") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          restricted_mode: false,
          expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      };
    } else if (
      pathname === "/api/backend/rental-contract-capabilities" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: { data: { manual_contracts_enabled: false } },
      };
    } else if (
      pathname === "/api/backend/rental-quotes" &&
      request.method() === "POST"
    ) {
      result = {
        status: 201,
        body: {
          data: {
            id: quoteId,
            space_id: spaceId,
            space_title: "Espacio sintético",
            rate_unit: "hora",
            unit_price_clp: 25000,
            currency: "CLP",
            units: 1,
            subtotal_clp: 25000,
            start_at: startAt,
            end_at: endAt,
            time_zone: "America/Santiago",
            created_at: new Date().toISOString(),
            expires_at:
              quoteExpiryOverride ??
              new Date(Date.now() + 15 * 60 * 1000).toISOString(),
          },
        },
      };
    } else if (
      pathname === `/api/backend/rental-quotes/${quoteId}/checkout` &&
      request.method() === "POST"
    ) {
      starts += 1;
      startKeys.push(request.headers()["idempotency-key"]);
      result =
        starts === 1
          ? {
              status: 503,
              body: { error: { code: "checkout_not_configured" } },
            }
          : starts === 2
            ? {
                status: 503,
                body: { error: { code: "payment_method_unavailable" } },
              }
            : {
                status: 200,
                body: {
                  data: {
                    quote_id: quoteId,
                    operation_id: "00000000-0000-4000-8000-000000000201",
                    status: "pending",
                    init_point:
                      "https://sandbox.mercadopago.cl/checkout/v1/redirect",
                    expires_at: new Date(
                      Date.now() + 15 * 60 * 1000,
                    ).toISOString(),
                  },
                },
              };
    }
    const adapted = bff.adapt(request, result);
    if (!adapted) throw new Error(`Uncontrolled request: ${request.url()}`);
    await route.fulfill({
      status: adapted.status,
      headers: {
        "cache-control": "no-store",
        "content-type": "application/json; charset=utf-8",
        ...(adapted.headers ?? {}),
      },
      body: JSON.stringify(adapted.body ?? {}),
    });
  });
  const providerUrl = "https://sandbox.mercadopago.cl/checkout/v1/redirect";
  await context.route(providerUrl, (route) =>
    route.fulfill({
      contentType: "text/html",
      body: "<h1>Checkout de prueba</h1>",
    }),
  );

  await page.goto("/acceso");
  await page.getByLabel("Correo electrónico").fill("renter@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticPass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/mi-cuenta");
  const query = new URLSearchParams({
    spaceId,
    date: "2030-01-16",
    duration: "1",
    start: startAt,
    end: endAt,
    rate_unit: "hora",
    time_zone: "America/Santiago",
    source: "marketplace",
  });
  await page.goto(`/reservas/nueva?${query.toString()}`);
  // Authenticated marketplace checkout requests the quote automatically.
  await expect(page.getByText("$25.000 CLP")).toBeVisible();
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await expect(
    page.getByText(
      "El pago con Mercado Pago no está habilitado en este entorno.",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await expect(
    page.getByText(
      "Mercado Pago no mostró un medio de pago compatible para esta compra. No se inició ningún pago.",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await expect(page).toHaveURL(providerUrl);
  await expect(
    page.getByRole("heading", { name: "Checkout de prueba" }),
  ).toBeVisible();
  await expect.poll(() => starts).toBe(3);
  await expect.poll(() => startKeys.length).toBe(3);
  await expect.poll(() => startKeys[1]).toBe(startKeys[0]);
  await expect.poll(() => startKeys[2]).toBe(startKeys[0]);
  expect(startKeys[0]).toMatch(/^[0-9a-f-]{36}$/i);

  const now = new Date();
  quoteExpiryOverride = new Date(now.getTime() + 15 * 60 * 1000).toISOString();
  await page.clock.install({ time: now });
  await page.goto(`/reservas/nueva?${query.toString()}`);
  await expect(page.getByText("$25.000 CLP")).toBeVisible();
  await page.clock.fastForward("16:00");
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await expect(
    page.getByText(
      "La cotización venció. Solicita una cotización actualizada.",
    ),
  ).toBeVisible();
  expect(starts).toBe(3);
});
