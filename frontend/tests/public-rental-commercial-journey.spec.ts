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

    if (pathname === "/api/backend/auth/logout" && method === "POST") {
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
            legal_name: "Propietario Sintético",
            rut: "12.345.678-9",
          },
          {
            role: "arrendatario",
            legal_name: "Arrendataria Sintética",
            rut: "98.765.432-1",
          },
        ],
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
  await page
    .getByRole("button", { name: "Calcular cotización oficial" })
    .click();
  await expect(
    page.getByText("Cotización oficial", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Pagar con Mercado Pago" }),
  ).toBeVisible();

  expect(quoteRequests).toHaveLength(1);
  expect(quoteRequests[0]).toEqual({
    space_id: spaceId,
    start_at: startAt,
    end_at: endAt,
  });
  expect(browserAuthHeaders).toEqual([undefined]);
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
            expires_at: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
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
  await context.route("https://sandbox.mercadopago.cl/**", (route) =>
    route.abort("blockedbyclient"),
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
  await page
    .getByRole("button", { name: "Calcular cotización oficial" })
    .click();
  await expect(page.getByText("$25.000 CLP")).toBeVisible();
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await expect(
    page.getByText(
      "El pago con Mercado Pago no está habilitado en este entorno.",
    ),
  ).toBeVisible();
  const providerRequest = page.waitForRequest(
    (request) =>
      request.url() === "https://sandbox.mercadopago.cl/checkout/v1/redirect",
  );
  await page.getByRole("button", { name: "Pagar con Mercado Pago" }).click();
  await providerRequest;
  await expect.poll(() => starts).toBe(2);
  await expect.poll(() => startKeys[1]).toBe(startKeys[0]);
  expect(startKeys[0]).toMatch(/^[0-9a-f-]{36}$/i);
});
