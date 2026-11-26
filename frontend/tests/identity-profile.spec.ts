import { expect, test, type Route } from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";

const accountId = "770e8400-e29b-41d4-a716-446655440000";
const localOrigin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? 4203}`;

test("perfil de identidad valida RUN, guarda, aparece en navbar e inicia verificación con estado recuperable", async ({
  page,
}) => {
  const bff = createBffFixtureState();
  const profileReturnTo = "/anfitrion/espacios/nuevo";
  const requestsOutsideLoopback: string[] = [];
  const apiCalls: Array<{ path: string; method: string; body?: unknown }> = [];
  let pinpassExpiresAt: string | null = null;
  let savedData: Record<string, unknown> = {};
  let draftData: Record<string, unknown> = {};
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
      body: `<main><h1>Verificación sintética</h1><a href="${localOrigin}/verificaciones/retorno?return_to=${encodeURIComponent(profileReturnTo)}">Volver a EspaciGo</a></main>`,
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
      savedData = body;
      profile = {
        ...profile,
        complete: true,
        draft_saved: false,
        state: profile.rut_locked ? "verified" : "ready",
        username: String(body.username),
        masked_rut: "•••••678-5",
      };
      result = { status: 200, body: profile };
    } else if (
      path === "/api/backend/account/identity-profile/draft" &&
      method === "PATCH"
    ) {
      draftData = request.postDataJSON();
      profile = { ...profile, draft_saved: true };
      result = { status: 200, body: { saved: true } };
    } else if (
      path === "/api/backend/account/identity-profile/draft/restore" &&
      method === "POST"
    ) {
      const proof = request.postDataJSON();
      apiCalls.push({ path, method, body: proof });
      result =
        proof.current_password === "SyntheticOnly123!"
          ? { status: 200, body: draftData }
          : {
              status: 403,
              body: { error: { code: "reauthentication_required" } },
            };
    } else if (
      path === "/api/backend/account/identity-profile/edit" &&
      method === "POST"
    ) {
      const proof = request.postDataJSON();
      apiCalls.push({ path, method, body: proof });
      result =
        proof.otp_code === "654321" || proof.pin === "123456"
          ? { status: 200, body: savedData }
          : {
              status: 403,
              body: { error: { code: "reauthentication_required" } },
            };
    } else if (path === "/api/backend/account/security/methods") {
      result = { status: 200, body: { methods: { totp: { enabled: true } } } };
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
      profile =
        profile.state === "in_review"
          ? {
              ...profile,
              state: "verified",
              rut_locked: true,
              verified_until: "2035-01-03T18:00:00Z",
            }
          : { ...profile, state: "in_review" };
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
      result = pinpassExpiresAt
        ? {
            status: 200,
            body: {
              field: "email",
              value: "identity@example.invalid",
              access_expires_at: pinpassExpiresAt,
            },
          }
        : { status: 403, body: { error: { code: "pinpass_access_required" } } };
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

  await page.goto("/acceso?return_to=%2Fmi-cuenta");
  await page.getByLabel("Correo electrónico").fill("identity@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticOnly123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/mi-cuenta");
  const sidebar = page.getByRole("navigation", {
    name: "Secciones de mi cuenta",
  });
  await expect(
    sidebar.getByRole("link", {
      name: /Verificar identidad|Revisión de identidad/,
    }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: "Ver verificaciones" }).click();
  await expect(page).toHaveURL(/\/mi-cuenta\/verificaciones$/);
  await expect(
    page.getByRole("heading", { name: "Verificaciones" }),
  ).toBeVisible();
  await expect(page.getByTestId("identity-profile-state")).toHaveText(
    "Perfil incompleto",
  );
  await expect(
    page.getByRole("button", { name: "Iniciar verificación" }),
  ).toHaveCount(0);
  await page.goto(
    `/mi-cuenta/verificaciones?return_to=${encodeURIComponent(profileReturnTo)}`,
  );
  await page.getByRole("link", { name: "Completar perfil" }).click();
  await expect(page).toHaveURL((url) => url.pathname === "/mi-cuenta/perfil");
  const documentType = page.getByRole("combobox", {
    name: "Tipo de documento",
  });
  await expect(
    documentType.getByText("Cédula de identidad chilena", { exact: true }),
  ).toBeVisible();
  await documentType.click();
  await expect(page.getByRole("option")).toHaveText([
    "Cédula de identidad chilena",
  ]);
  await page
    .getByRole("option", { name: "Cédula de identidad chilena" })
    .click();
  await expect(
    page.getByText("Documento admitido para esta primera etapa."),
  ).toHaveCount(0);
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
  ).toHaveCount(0);
  expect(apiCalls.filter((call) => call.method === "PUT")).toHaveLength(0);

  await page.locator("#identity-profile-rut").fill("12.345.678-0");
  await page.locator("#identity-profile-rut").blur();
  await expect(page.locator("#identity-profile-rut-error")).toContainText(
    "Revisa el RUN y su dígito verificador.",
  );

  await page.locator("#identity-profile-rut").fill("00.000.000-0");
  await page.locator("#identity-profile-rut").blur();
  await expect(page.locator("#identity-profile-rut-error")).toContainText(
    "Revisa el RUN y su dígito verificador.",
  );

  await page.locator("#identity-profile-username").fill("sintetica_ana");
  await page.locator("#identity-profile-first_name").fill("Ana María");
  await page.locator("#identity-profile-first_last_name").fill("González");
  await page.locator("#identity-profile-second_last_name").fill("Prueba");
  await page.locator("#identity-profile-rut").fill("12345678-5000000000000");
  await expect(page.locator("#identity-profile-rut")).toHaveValue(
    "12.345.678-5",
  );
  await page.locator("#identity-profile-date_of_birth").fill("1990-01-02");
  await page.locator("#identity-profile-phone").fill("+56 9 1234 5678 1234");
  await expect(page.locator("#identity-profile-phone")).toHaveValue("12345678");
  await page.getByRole("button", { name: "Guardar borrador" }).click();
  await page.getByRole("button", { name: "Continuar perfil" }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Confirma tu identidad",
  });
  await expect(confirmation).toBeVisible();
  await expect(
    page.getByText("Continuar un borrador", { exact: true }),
  ).toHaveCount(0);
  await confirmation.getByLabel("Contraseña actual").fill("SyntheticOnly123!");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
  await expect(page.locator("#identity-profile-first_name")).toHaveValue(
    "Ana María",
  );
  await page.locator("#identity-profile-consent").check();
  await page.getByRole("button", { name: "Guardar perfil" }).click();

  await expect(page.getByText("Datos guardados.")).toBeVisible();
  await expect(
    page.getByRole("form", { name: "Editar datos del perfil" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: /Ir a revisión de identidad/ }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: "Ir a verificaciones" }).click();
  await expect(
    page.getByText("Listo para verificar", { exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Editar datos del perfil" }).click();
  await expect(
    confirmation.getByRole("button", { name: "Pinpass", exact: true }),
  ).toBeEnabled();
  await confirmation.getByRole("button", { name: "Autenticador" }).click();
  await confirmation.getByLabel("Código del autenticador").fill("654321");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
  await expect(
    page.getByRole("form", { name: "Editar datos del perfil" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Cancelar", exact: true }).click();
  await expect(
    page.getByRole("form", { name: "Editar datos del perfil" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Editar datos" }).click();
  await confirmation
    .getByRole("button", { name: "Pinpass", exact: true })
    .click();
  await confirmation.getByLabel("Ingresa tu Pinpass").fill("123456");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
  await expect(page.locator("#identity-profile-phone")).toHaveValue("12345678");
  await page.getByRole("button", { name: "Cancelar", exact: true }).click();
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

  await accountMenuButton.click();
  await page.getByRole("link", { name: "Ir a verificaciones" }).click();
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
  await expect(page).toHaveURL(
    (url) => url.pathname === "/mi-cuenta/verificaciones",
  );
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

  await expect(
    page.getByRole("link", { name: "Volver al borrador de publicación" }),
  ).toHaveAttribute("href", profileReturnTo);
  await page.getByRole("button", { name: "Actualizar estado" }).click();
  await expect(page.getByTestId("identity-profile-state")).toHaveText(
    "Verificado",
  );
  await expect(page.getByText("Vigente hasta", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Iniciar verificación" }),
  ).toHaveCount(0);
  await page.goto("/mi-cuenta/perfil");
  await expect(
    page
      .locator("#identity-verification")
      .getByText("Verificado", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Datos privados" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Mostrar Nombre legal completo" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Mostrar Fecha de nacimiento" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Editar teléfono" }).click();
  await confirmation
    .getByRole("button", { name: "Pinpass", exact: true })
    .click();
  await confirmation.getByLabel("Ingresa tu Pinpass").fill("123456");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
  await expect(page.locator("#identity-profile-phone")).toBeVisible();
  await expect(page.locator("#identity-profile-first_name")).toHaveCount(0);
  await expect(page.locator("#identity-profile-date_of_birth")).toHaveCount(0);
  await page.getByRole("button", { name: "Cancelar", exact: true }).click();
  const showEmail = page.getByRole("button", {
    name: "Mostrar Correo electrónico",
  });
  await expect(showEmail).toBeVisible();
  await showEmail.click();
  const pin = page.getByLabel("Ingresa tu Pinpass");
  await expect(pin).toBeVisible();
  await pin.fill("111111");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
  await expect(confirmation.getByRole("alert")).toContainText(
    "El Pinpass no coincide. Revisa los seis dígitos.",
  );
  await pin.fill("123456");
  await confirmation.getByRole("button", { name: "Confirmar" }).click();
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
      .getByText("i***@example.invalid", { exact: true }),
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

test("Pinpass es el primer método y se configura dentro de Seguridad", async ({
  page,
}) => {
  const bff = createBffFixtureState();
  const pinpassPath = "/api/backend/account/security/pinpass";
  const session = {
    account_id: accountId,
    roles: ["arrendatario"],
    capabilities: [],
    expires_at: new Date(Date.now() + 3_600_000).toISOString(),
    restricted_mode: false,
  };
  let configured = false;
  let authenticatorEnabled = false;
  let available = true;
  let proof: unknown;

  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let result: { status: number; body: unknown } = {
      status: 404,
      body: { error: { code: "not_found" } },
    };
    if (
      path === "/api/backend/auth/login" ||
      path === "/api/backend/auth/session"
    ) {
      result = { status: 200, body: session };
    } else if (path === "/api/backend/account/security/methods") {
      result = {
        status: 200,
        body: {
          protected: false,
          active_count: 2 + Number(configured) + Number(authenticatorEnabled),
          total_count: 7,
          methods: {
            phone: { enabled: false },
            password: { enabled: true },
            email: { enabled: true, masked_email: "i***@example.invalid" },
            passkeys: { enabled: false, count: 0 },
            totp: { enabled: authenticatorEnabled },
            pinpass: { enabled: configured },
            qr: { enabled: false },
            facial_kyc: { enabled: false, status: "unavailable" },
          },
        },
      };
    } else if (
      path === pinpassPath + "/email-code" &&
      request.method() === "POST"
    ) {
      result = {
        status: 201,
        body: {
          masked_email: "i***@example.invalid",
          expires_at: session.expires_at,
        },
      };
    } else if (path === pinpassPath && request.method() === "PUT") {
      proof = request.postDataJSON();
      configured = true;
      result = { status: 200, body: { configured } };
    } else if (path === pinpassPath && request.method() === "GET") {
      result = available
        ? {
            status: 200,
            body: { configured, blocked_until: null, access_expires_at: null },
          }
        : { status: 503, body: { error: { code: "unavailable" } } };
    }
    const adapted = bff.adapt(request, result);
    await route.fulfill({
      status: adapted?.status ?? 500,
      json: adapted?.body ?? {},
    });
  });

  const security = "/mi-cuenta/seguridad";
  await page.goto(`/acceso?return_to=${encodeURIComponent(security)}`);
  await page.getByLabel("Correo electrónico").fill("identity@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticOnly123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await expect(page).toHaveURL(new RegExp(`${security}$`));
  await expect(
    page.getByRole("link", { name: /EspaciGo Pinpass/i }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: /Métodos de verificación/ }).click();

  const methods = page
    .getByRole("list", { name: "Métodos de verificación de la cuenta" })
    .getByRole("heading");
  await expect(methods).toHaveText([
    "Pinpass",
    "Contraseña",
    "Correo electrónico",
    "Teléfono",
    "Llaves de acceso",
    "Aplicación de autenticación",
    "Código QR",
  ]);
  await expect(
    page.getByText(
      "Configura tu Pinpass para navegar y desbloquear opciones dentro de EspaciGo.",
    ),
  ).toBeVisible();
  available = false;
  await page.getByRole("link", { name: /Configurar Pinpass/ }).click();
  await expect(page).toHaveURL(new RegExp(`${security}/metodos/pinpass$`));
  await expect(
    page
      .getByRole("alert")
      .filter({ hasText: "Pinpass no está disponible en este momento." }),
  ).toBeVisible();
  available = true;
  await page.getByRole("button", { name: "Reintentar" }).click();
  await page.getByLabel("Nuevo Pinpass").fill("134679");
  await page.getByLabel("Confirma tu Pinpass").fill("134679");
  await page.getByRole("button", { name: "Crear Pinpass" }).click();
  const dialog = page.getByRole("dialog", { name: "Confirma tu identidad" });
  await dialog.getByLabel("Contraseña actual").fill("SyntheticOnly123!");
  await expect(dialog.getByLabel("Código del autenticador")).toHaveCount(0);
  await expect(
    dialog.getByRole("button", { name: "Confirmar" }),
  ).toBeDisabled();
  await dialog.getByRole("button", { name: "Enviar código" }).click();
  await expect(dialog.getByText(/Enviamos un código/)).toBeVisible();
  await dialog.getByLabel("Código de correo electrónico").fill("987654");
  await dialog.getByRole("button", { name: "Confirmar" }).click();
  await expect(page.getByText("Tu Pinpass quedó configurado.")).toBeVisible();
  expect(proof).toEqual({
    pin: "134679",
    confirmation: "134679",
    current_password: "SyntheticOnly123!",
    email_code: "987654",
  });
  await page.getByRole("link", { name: "Métodos de verificación" }).click();
  await expect(
    page.getByRole("link", { name: /Administrar Pinpass/ }),
  ).toBeVisible();
  await expect(
    page.getByText("Configurar Pinpass", { exact: true }),
  ).toHaveCount(0);

  await expect(page.getByText("3 de 7 métodos configurados")).toBeVisible();
  await expect(
    page
      .getByRole("link", { name: "Administrar Pinpass" })
      .getByText("Configurado", { exact: true }),
  ).toBeVisible();
  authenticatorEnabled = true;
  await page.getByRole("link", { name: "Administrar Pinpass" }).click();
  await page.getByRole("button", { name: "Cambiar Pinpass" }).click();
  await page.getByLabel("Nuevo Pinpass").fill("134680");
  await page.getByLabel("Confirma tu Pinpass").fill("134680");
  await page.getByRole("button", { name: "Actualizar Pinpass" }).click();
  await expect(dialog.getByLabel("Contraseña actual")).toHaveCount(0);
  await dialog.getByLabel("Código del autenticador").fill("123456");
  await dialog.getByRole("button", { name: "Enviar código" }).click();
  await dialog.getByLabel("Código de correo electrónico").fill("987654");
  await dialog.getByRole("button", { name: "Confirmar" }).click();
  await expect(page.getByText("Tu Pinpass quedó actualizado.")).toBeVisible();
  expect(proof).toEqual({
    pin: "134680",
    confirmation: "134680",
    otp_code: "123456",
    email_code: "987654",
  });

  await page.goto(`${security}/metodos/reconocimiento-facial`);
  await expect(page).toHaveURL(new RegExp(`${security}/metodos$`));
  await expect(
    page.getByRole("heading", { name: "Reconocimiento facial" }),
  ).toHaveCount(0);
});
