import {
  expect,
  test,
  type Page,
  type Request,
  type Route,
} from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";
import { isAllowedLocalResource } from "./helpers/local-resource-origin";
import type { components } from "../src/lib/api/generated/openapi";
import { localDateLabel } from "../src/features/rental-demo/domain/calendar";

test.use({ trace: "off", screenshot: "off", video: "off" });
const accountId = "770e8400-e29b-41d4-a716-446655440000";
const spaceId = "00000000-0000-4000-8000-000000000221";
const path = `/api/backend/spaces/${spaceId}/availability/configuration`;
type Reply = {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
};
type Respond = (pathname: string, method: string, request: Request) => Reply;
async function mock(page: Page, respond: Respond) {
  const fixture = createBffFixtureState();
  let outside = false;
  page.on("request", (r) => {
    if (
      !isAllowedLocalResource(r.url(), String(test.info().project.use.baseURL))
    )
      outside = true;
  });
  page.on("requestfailed", (r) => {
    if (
      !isAllowedLocalResource(r.url(), String(test.info().project.use.baseURL))
    )
      outside = true;
  });
  page.on("close", () => expect(outside).toBe(false));
  await page.route("**/api/backend/**", async (route: Route) => {
    const req = route.request();
    const url = new URL(req.url());
    if (!url.hostname.match(/^(127\.0\.0\.1|localhost)$/))
      throw new Error("Non-loopback fixture request");
    const reply = fixture.adapt(req, respond(url.pathname, req.method(), req));
    if (!reply)
      throw new Error(
        `Unmatched controlled request: ${req.method()} ${url.pathname}`,
      );
    await route.fulfill({
      status: reply.status,
      headers: {
        "content-type": "application/json",
        "cache-control": "no-store",
        ...(reply.headers ?? {}),
      },
      body: reply.status === 204 ? "" : JSON.stringify(reply.body ?? {}),
    });
  });
}
function loginBody(expires: string) {
  return {
    account_id: accountId,
    roles: ["arrendatario", "anfitrion"],
    capabilities: [],
    restricted_mode: false,
    expires_at: expires,
  };
}
async function enter(page: Page) {
  await page.goto(
    `/acceso?return_to=${encodeURIComponent(`/anfitrion/espacios/${spaceId}/editar`)}`,
  );
  await page
    .getByLabel("Correo electrónico")
    .fill("calendar-tester@example.invalid");
  await page.getByLabel("Contraseña", { exact: true }).fill("SecurePass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL(
    (u) => u.pathname === `/anfitrion/espacios/${spaceId}/editar`,
  );
  await page
    .getByRole("tab", { name: /Bloqueos|Disponibilidad por fecha/ })
    .click();
}
const overrideDate = `${new Date().toISOString().slice(0, 7)}-17`;
const initial: components["schemas"]["OwnCalendarConfiguration"] = {
  space_id: spaceId,
  time_zone: "America/Santiago",
  updated_at: "2026-11-16T00:00:00Z",
  rule: {
    space_id: spaceId,
    dias_operacion: [2, 4],
    hora_apertura: "10:00",
    hora_cierre: "18:00",
    check_in_hora: "15:00",
    check_out_hora: "12:00",
    tarifa_base_dia: 35000,
    estadia_minima: 1,
    estadia_maxima: 30,
    actualizado_en: "2026-11-16T00:00:00Z",
  },
  overrides: [
    {
      fecha: overrideDate,
      estado: "bloqueado",
      motivo: "Mantenimiento de prueba",
      tarifa_personalizada_clp: 45000,
    },
  ],
};
for (const width of [1440, 390]) {
  test(`@controlled guarda configuración de calendario y confirma por GET ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 });
    const expires = new Date(Date.now() + 3600_000).toISOString();
    let current = initial;
    let etag = '"v1"';
    let gets = 0;
    let puts = 0;
    let saved: unknown;
    await mock(page, (p, m, req) => {
      if (p === "/api/backend/auth/login" && m === "POST")
        return { status: 200, body: loginBody(expires) };
      if (p === "/api/backend/auth/session" && m === "GET")
        return { status: 200, body: loginBody(expires) };
      if (p === `/api/backend/spaces/${spaceId}` && m === "GET")
        return {
          status: 200,
          headers: { etag },
          body: {
            id: spaceId,
            category_code: "oficina",
            category_name: "Oficinas",
            title: "Espacio calendario",
            description: "Descripción",
            area_m2: 40,
            capacity: 4,
            usage_rules: "Reglas",
            rate_unit: "hora",
            base_price_clp: 35000,
            address: "Santiago",
            state: "borrador",
            time_zone: "America/Santiago",
            region_code: "13",
            comuna_code: "Santiago",
            attributes: {},
            attribute_schema_version: 1,
          },
        };
      if (p === path && m === "GET") {
        gets++;
        return { status: 200, headers: { etag }, body: current };
      }
      if (p === path && m === "PUT") {
        puts++;
        saved = JSON.parse(req.postData() ?? "{}");
        if (req.headers()["if-match"] !== '"v1"')
          throw new Error("Incorrect If-Match");
        const input = saved as components["schemas"]["CalendarInventoryUpdate"];
        const { time_zone, overrides, ...rule } = input;
        current = {
          ...current,
          time_zone: time_zone ?? current.time_zone,
          updated_at: "2026-11-16T00:00:01Z",
          rule: {
            ...current.rule,
            ...rule,
            actualizado_en: "2026-11-16T00:00:01Z",
          },
          overrides: overrides ?? current.overrides,
        };
        etag = '"v2"';
        return {
          status: 200,
          headers: { etag },
          body: { space_id: spaceId, time_zone: current.time_zone },
        };
      }
      return {
        status: 404,
        body: { error: { code: "not_found", message: "No disponible" } },
      };
    });
    await enter(page);

    await expect(page.getByLabel("Hora apertura", { exact: true })).toHaveValue(
      "10:00",
    );
    await expect(page.getByLabel("Hora cierre", { exact: true })).toHaveValue(
      "18:00",
    );
    await expect(
      page.getByLabel("Tarifa base del calendario (CLP)"),
    ).toHaveValue("35000");
    const base = page.getByRole("region", {
      name: "Configurador rápido de horario base",
    });
    await expect(base.getByTitle("Jueves", { exact: true })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await base.getByTitle("Viernes", { exact: true }).click();
    await page.getByLabel("Hora apertura", { exact: true }).fill("11:00");
    await page
      .getByRole("button", {
        name: `${localDateLabel(overrideDate)}, bloqueada`,
        exact: true,
      })
      .click();
    await page
      .getByRole("button", { name: "Abrir fechas bloqueadas", exact: true })
      .click();
    await page
      .getByRole("button", {
        name: "Guardar disponibilidad en servidor",
        exact: true,
      })
      .click();
    await expect.poll(() => puts).toBe(1);
    await expect(
      page.getByText("Configuración guardada y verificada.", { exact: true }),
    ).toBeVisible();
    expect(saved).toEqual({
      time_zone: initial.time_zone,
      dias_operacion: [2, 4, 5],
      hora_apertura: "11:00",
      hora_cierre: "18:00",
      check_in_hora: "15:00",
      check_out_hora: "12:00",
      tarifa_base_dia: 35000,
      estadia_minima: 1,
      estadia_maxima: 30,
      overrides: [{ ...initial.overrides[0], estado: "disponible" }],
    });
    expect(gets).toBeGreaterThanOrEqual(2);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page
      .getByRole("tab", { name: "Disponibilidad por fecha", exact: true })
      .click();
    await expect(page.getByLabel("Hora apertura", { exact: true })).toHaveValue(
      "11:00",
    );
    await expect(base.getByTitle("Viernes", { exact: true })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(puts).toBe(1);
    expect(gets).toBeGreaterThanOrEqual(3);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`host-calendar-${width}.png`),
      fullPage: true,
    });
  });
}

test("@controlled conflicto 412 no reintenta y permite refrescar", async ({
  page,
}) => {
  const expires = new Date(Date.now() + 3600_000).toISOString();
  let puts = 0;
  let gets = 0;
  await mock(page, (p, m) => {
    if (p === "/api/backend/auth/login" && m === "POST")
      return { status: 200, body: loginBody(expires) };
    if (p === "/api/backend/auth/session" && m === "GET")
      return { status: 200, body: loginBody(expires) };
    if (p === `/api/backend/spaces/${spaceId}` && m === "GET")
      return {
        status: 200,
        headers: { etag: '"v1"' },
        body: {
          id: spaceId,
          category_code: "oficina",
          category_name: "Oficinas",
          title: "Espacio calendario",
          description: "Descripción",
          area_m2: 40,
          capacity: 4,
          usage_rules: "Reglas",
          rate_unit: "hora",
          base_price_clp: 35000,
          address: "Santiago",
          state: "borrador",
          time_zone: "America/Santiago",
          region_code: "13",
          comuna_code: "Santiago",
          attributes: {},
          attribute_schema_version: 1,
        },
      };
    if (p === path && m === "GET") {
      gets++;
      return { status: 200, headers: { etag: '"v1"' }, body: initial };
    }
    if (p === path && m === "PUT") {
      puts++;
      return {
        status: 412,
        body: {
          error: {
            code: "precondition_failed",
            message: "No mostrar este error",
          },
        },
      };
    }
    return {
      status: 404,
      body: { error: { code: "not_found", message: "No disponible" } },
    };
  });
  await enter(page);
  await page
    .getByRole("button", {
      name: "Guardar disponibilidad en servidor",
      exact: true,
    })
    .click();
  await expect.poll(() => puts).toBe(1);
  await expect(page.getByText(/No mostrar este error/)).toHaveCount(0);
  await expect(
    page.getByRole("button", {
      name: "Guardar disponibilidad en servidor",
      exact: true,
    }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Actualizar para verificar", exact: true })
    .click();
  await expect.poll(() => gets).toBeGreaterThan(1);
  await expect(
    page.getByRole("button", {
      name: "Guardar disponibilidad en servidor",
      exact: true,
    }),
  ).toBeEnabled();
  expect(puts).toBe(1);
});
