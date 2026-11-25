import { expect, test, type Route } from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";

const accountId = "770e8400-e29b-41d4-a716-446655440000";
const localOrigin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? 4203}`;

test("perfil de identidad valida RUN, guarda, aparece en navbar e inicia verificación con estado recuperable", async ({
  page,
}) => {
  const bff = createBffFixtureState();
  const requestsOutsideLoopback: string[] = [];
  const apiCalls: Array<{ path: string; method: string; body?: unknown }> = [];
  let pinpassExpiresAt: string | null = null;
  let profile: Record<string, unknown> = {
    complete: false,
    state: "incomplete",
    rut_locked: false,
    draft_saved: false,
    masked_email: "i***@example.invalid",
    masked_rut: "•••••678-5",
    masked_phone: "••••1234",
  };

  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.hostname !== "127.0.0.1" && url.hostname !== "verify.didit.me")
      requestsOutsideLoopback.push(request.url());
  });

  await page.route("https://verify.didit.me/**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "text/html; charset=utf-8",
      body: `<main><h1>Verificación sintética</h1><a href="${localOrigin}/verificaciones/retorno">Volver a EspaciGo</a></main>`,
    });
  });

  await page.route("**/api/backend/**", async (route: Route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    let result: { status: number; body?: unknown };

    if (path === "/api/backend/auth/login" && method === "POST") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          expires_at: new Date(Date.now() + 3_600_000).toISOString(),
          restricted_mode: false,
        },
      };
    } else if (path === "/api/backend/auth/session" && method === "GET") {
      result = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          expires_at: new Date(Date.now() + 3_600_000).toISOString(),
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/account/identity-profile" &&
      method === "GET"
    ) {
      result = { status: 200, body: profile };
    } else if (
      path === "/api/backend/account/identity-profile" &&
      method === "PUT"
    ) {
      const body = request.postDataJSON() as Record<string, unknown>;
      apiCalls.push({ path, method, body });
      profile = {
        ...profile,
        complete: true,
        state: "ready",
        username: String(body.username),
        masked_rut: "•••••678-5",
      };
      result = { status: 200, body: profile };
    } else if (
      path === "/api/backend/account/identity-profile/verification" &&
      method === "POST"
    ) {
      apiCalls.push({ path, method, body: request.postDataJSON() });
      profile = { ...profile, state: "in_progress" };
      result = {
        status: 201,
        body: { url: "https://verify.didit.me/session/synthetic-session" },
      };
    } else if (
      path === "/api/backend/account/identity-profile/verification/refresh" &&
      method === "POST"
    ) {
      apiCalls.push({ path, method });
      profile = { ...profile, state: "in_review" };
      result = { status: 200, body: profile };
    } else if (
      path === "/api/backend/account/security/pinpass" &&
      method === "GET"
    ) {
      result = {
        status: 200,
        body: {
          configured: true,
          blocked_until: null,
          access_expires_at: null,
        },
      };
    } else if (
      path === "/api/backend/account/security/pinpass/unlock" &&
      method === "POST"
    ) {
      const body = request.postDataJSON() as { pin?: string };
      apiCalls.push({ path, method, body });
      pinpassExpiresAt =
        body.pin === "123456"
          ? new Date(Date.now() + 15 * 60_000).toISOString()
          : null;
      result = pinpassExpiresAt
        ? { status: 200, body: { access_expires_at: pinpassExpiresAt } }
        : { status: 403, body: { error: { code: "invalid_pin" } } };
    } else if (
      path === "/api/backend/account/security/pinpass/reveal/email" &&
      method === "GET"
    ) {
      apiCalls.push({ path, method });
      result = {
        status: 200,
        body: {
          field: "email",
          value: "identity@example.invalid",
          access_expires_at: pinpassExpiresAt,
        },
      };
    } else {
      apiCalls.push({ path, method });
      result = { status: 404, body: { error: { code: "not_found" } } };
    }

    const adapted = bff.adapt(request, result);
    await route.fulfill({
      status: adapted?.status ?? 500,
      headers: {
        "content-type": "application/json; charset=utf-8",
        "cache-control": "private, no-store",
      },
      body: JSON.stringify(adapted?.body ?? {}),
    });
  });

  const profileReturnTo = "/anfitrion/espacios/nuevo";
  await page.goto(
    `/acceso?return_to=${encodeURIComponent("/mi-cuenta/perfil")}`,
  );
  await page.getByLabel("Correo electrónico").fill("identity@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticOnly123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/mi-cuenta/perfil");
  await page.goto(
    `/mi-cuenta/perfil?return_to=${encodeURIComponent(profileReturnTo)}`,
  );

  await expect(
    page.locator('[aria-labelledby="identity-profile-title"] [role="status"]'),
  ).toHaveText("Perfil incompleto");
  await page.getByRole("button", { name: "Guardar perfil" }).click();
  await expect(
    page.locator('[aria-labelledby="identity-profile-title"] [role="alert"]'),
  ).toContainText("Revisa los campos marcados antes de guardar tu perfil.");
  await expect(page.locator("#identity-profile-username-error")).toContainText(
    "Elige un nombre de usuario.",
  );
  await expect(page.locator("#identity-profile-rut-error")).toContainText(
    "Ingresa tu RUN.",
  );
  await expect(
    page.getByRole("button", { name: "Iniciar verificación" }),
  ).toBeDisabled();
  expect(apiCalls.filter((call) => call.method === "PUT")).toHaveLength(0);

  await page.locator("#identity-profile-rut").fill("12.345.678-0");
  await page.locator("#identity-profile-rut").blur();
  await expect(page.locator("#identity-profile-rut-error")).toContainText(
    "Revisa el RUN y su dígito verificador.",
  );

  await page.locator("#identity-profile-username").fill("sintetica_ana");
  await page.locator("#identity-profile-first_name").fill("Ana María");
  await page.locator("#identity-profile-first_last_name").fill("González");
  await page.locator("#identity-profile-second_last_name").fill("Prueba");
  await page.locator("#identity-profile-rut").fill("12345678-5");
  await expect(page.locator("#identity-profile-rut")).toHaveValue(
    "12.345.678-5",
  );
  await page.locator("#identity-profile-date_of_birth").fill("1990-01-02");
  await page.locator("#identity-profile-phone").fill("+56 9 1234 5678");
  await page.locator("#identity-profile-consent").check();
  await page.getByRole("button", { name: "Guardar perfil" }).click();

  await expect(
    page.getByText("Listo para verificar", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Datos guardados.")).toBeVisible();
  const put = apiCalls.find((call) => call.method === "PUT");
  expect(put?.body).toMatchObject({
    username: "sintetica_ana",
    first_name: "Ana María",
    first_last_name: "González",
    second_last_name: "Prueba",
    rut: "12.345.678-5",
    date_of_birth: "1990-01-02",
    document_type: "cl_id",
    phone: "+56912345678",
    consent: true,
  });

  const accountMenuButton = page.getByRole("button", {
    name: "sintetica_ana",
  });
  await expect(accountMenuButton).toBeVisible();
  await accountMenuButton.click();
  await expect(
    page.getByRole("menu", { name: "Menú de cuenta" }),
  ).toContainText("sintetica_ana");

  await page.getByRole("button", { name: "Iniciar verificación" }).click();
  await expect(page).toHaveURL(
    "https://verify.didit.me/session/synthetic-session",
  );
  const startCall = apiCalls.find(
    (call) =>
      call.path === "/api/backend/account/identity-profile/verification",
  );
  expect(startCall?.method).toBe("POST");
  expect(startCall?.body).toEqual({ return_to: profileReturnTo });
  await page.getByRole("link", { name: "Volver a EspaciGo" }).click();
  await expect(page).toHaveURL(/\/mi-cuenta\/perfil(?:\?|#|$)/);
  await expect(
    page.getByText("Identidad en revisión", { exact: true }),
  ).toBeVisible();
  await expect(page).not.toHaveURL(/refresh_identity=1/);
  expect(
    apiCalls.filter(
      (call) =>
        call.path ===
          "/api/backend/account/identity-profile/verification/refresh" &&
        call.method === "POST",
    ),
  ).toHaveLength(1);

  const showEmail = page.getByRole("button", {
    name: "Mostrar Correo electrónico",
  });
  await expect(showEmail).toBeVisible();
  await showEmail.click();
  const pin = page.getByLabel("Ingresa tu Pinpass");
  await expect(pin).toBeVisible();
  await pin.fill("111111");
  await page.getByRole("button", { name: "Verificar Pinpass" }).click();
  await expect(page.locator("#pinpass [role=alert]")).toContainText(
    "El Pinpass no coincide. Revisa los seis dígitos.",
  );
  await expect(
    page.getByRole("button", { name: "Mostrar Correo electrónico" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "sintetica_ana" }),
  ).toBeVisible();

  await pin.fill("123456");
  await page.getByRole("button", { name: "Verificar Pinpass" }).click();
  await expect(
    page.getByText("identity@example.invalid", { exact: true }),
  ).toBeVisible();
  const hideEmail = page.getByRole("button", {
    name: "Ocultar Correo electrónico",
  });
  await hideEmail.click();
  const showEmailAgain = page.getByRole("button", {
    name: "Mostrar Correo electrónico",
  });
  await expect(showEmailAgain).toBeVisible();
  await expect(
    showEmailAgain
      .locator("xpath=..")
      .getByText("••••••••••••", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "sintetica_ana" }),
  ).toBeVisible();
  expect(
    apiCalls.filter(
      (call) =>
        call.path === "/api/backend/account/security/pinpass/unlock" &&
        call.method === "POST",
    ),
  ).toHaveLength(2);
  expect(requestsOutsideLoopback).toEqual([]);
});
