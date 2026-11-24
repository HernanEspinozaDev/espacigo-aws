import { expect, test, type Route } from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";
import categoryProfiles from "./fixtures/category-profiles-v1.json";

const mapName = "Mapa: selecciona el punto del espacio";
const santiagoPin = {
  address: "Av. Geocodificada 200, Santiago",
  region_code: "13",
  comuna_code: "13101",
};
const valparaisoPin = {
  address: "Av. de prueba 500, Valparaíso",
  region_code: "05",
  comuna_code: "05101",
};
const mapStyle = {
  version: 8,
  sources: {},
  layers: [
    {
      id: "wizard-test-background",
      type: "background",
      paint: { "background-color": "#e8efe6" },
    },
  ],
};

test.beforeEach(async ({ page }) => {
  await page.route(/(?:\/__[^/]+-map-style\.json|\/v2\/styles\/)/, (route) =>
    route.fulfill({ json: mapStyle }),
  );
});

test.use({ trace: "off", screenshot: "off", video: "off" });

test("@phase7 @controlled wizard recupera el borrador cuando falla weekly-hours después de crearlo", async ({
  page,
  context,
}) => {
  await context.setGeolocation({
    latitude: -33.45,
    longitude: -70.66,
    accuracy: 20,
  });
  const bff = createBffFixtureState();
  const spaceId = "00000000-0000-4000-8000-000000000121";
  const profile = categoryProfiles.profiles.inmuebles;
  let createCount = 0;
  let locationCount = 0;
  let availabilityCount = 0;
  let weeklyHoursCount = 0;
  let spaceDetailGetCount = 0;
  let createBody: Record<string, unknown> = {};
  let location: Record<string, unknown> = {};

  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let result: {
      status: number;
      body?: unknown;
      headers?: Record<string, string>;
    };
    if (path === "/api/backend/auth/login" && request.method() === "POST") {
      result = {
        status: 200,
        body: {
          account_id: "770e8400-e29b-41d4-a716-446655440000",
          roles: ["arrendador"],
          capabilities: [],
          expires_at: new Date(Date.now() + 3600000).toISOString(),
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/auth/session" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: {
          account_id: "770e8400-e29b-41d4-a716-446655440000",
          roles: ["arrendador"],
          capabilities: [],
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/spaces/categories" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: { items: [{ code: "inmuebles", name: "Inmuebles" }] },
      };
    } else if (
      /^\/api\/backend\/spaces\/categories\/inmuebles\/attributes(?:\/1)?$/.test(
        path,
      ) &&
      request.method() === "GET"
    ) {
      result = { status: 200, body: profile };
    } else if (
      path === "/api/backend/spaces/location/reverse-geocode" &&
      request.method() === "POST"
    ) {
      result = { status: 200, body: santiagoPin };
    } else if (path === "/api/backend/spaces" && request.method() === "POST") {
      createCount += 1;
      createBody = JSON.parse(request.postData() ?? "{}") as Record<
        string,
        unknown
      >;
      location = createBody.location as Record<string, unknown>;
      result = {
        status: 201,
        headers: { etag: '"draft-v1"' },
        body: {
          id: spaceId,
          category_code: "inmuebles",
          category_name: "Inmuebles",
          state: "borrador",
          ...createBody,
          ...location,
        },
      };
    } else if (
      path === `/api/backend/spaces/${spaceId}/location` &&
      request.method() === "PUT"
    ) {
      locationCount += 1;
      result = {
        status: 200,
        headers: { etag: '"location-v2"' },
        body: {
          space_id: spaceId,
          address: "Dirección sintética 100, Santiago",
          region_code: "13",
          comuna_code: "13101",
          latitude: null,
          longitude: null,
          public_confirmed: false,
          confirmed_at: null,
          revision: 1,
          updated_at: new Date().toISOString(),
        },
      };
    } else if (
      path === `/api/backend/spaces/${spaceId}/availability` &&
      request.method() === "PUT"
    ) {
      availabilityCount += 1;
      result = {
        status: 200,
        headers: { etag: '"availability-v2"' },
        body: { space_id: spaceId, time_zone: "America/Santiago" },
      };
    } else if (
      path === `/api/backend/spaces/${spaceId}/weekly-hours` &&
      request.method() === "PUT"
    ) {
      weeklyHoursCount += 1;
      result = {
        status: 503,
        body: {
          error: {
            code: "fixture_unavailable",
            message: "Controlled weekly-hours failure",
          },
        },
      };
    } else if (
      path === `/api/backend/spaces/${spaceId}` &&
      request.method() === "GET"
    ) {
      spaceDetailGetCount += 1;
      result = {
        status: 200,
        headers: { etag: '"draft-v2"' },
        body: {
          id: spaceId,
          category_code: "inmuebles",
          category_name: "Inmuebles",
          title: createBody.title,
          description: createBody.description,
          state: "borrador",
          time_zone: "America/Santiago",
          region_code: location.region_code,
          comuna_code: location.comuna_code,
          address: location.address,
          area_m2: createBody.area_m2,
          capacity: createBody.capacity,
          rate_unit: createBody.rate_unit,
          base_price_clp: createBody.base_price_clp,
          usage_rules: createBody.usage_rules,
          attribute_schema_version: 1,
          attributes: createBody.attributes,
        },
      };
    } else {
      result = {
        status: 404,
        body: { error: { code: "not_found", message: "Not found" } },
      };
    }
    const adapted = bff.adapt(request, result);
    if (!adapted)
      throw new Error(
        `Missing controlled response for ${request.method()} ${path}`,
      );
    await route.fulfill({
      status: adapted.status,
      headers: {
        "content-type": "application/json; charset=utf-8",
        ...(adapted.headers ?? {}),
      },
      body: JSON.stringify(adapted.body ?? {}),
    });
  });

  await page.goto(
    `/acceso?return_to=${encodeURIComponent("/anfitrion/espacios/nuevo")}`,
  );
  await page
    .getByLabel("Correo electrónico")
    .fill("wizard-test@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticPass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/anfitrion/espacios/nuevo");
  await page.locator("#wizard-category").selectOption("inmuebles");
  await page.locator("#wizard-title").fill("Borrador sintético recuperable");
  await page
    .locator("#wizard-description")
    .fill(
      "Descripción sintética suficientemente extensa para el anuncio del espacio y su prueba controlada, con detalles de uso, acceso y características principales.",
    );
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(page.getByText(/No pudimos obtener tu ubicación/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Siguiente", exact: true }),
  ).toBeDisabled();
  await context.grantPermissions(["geolocation"]);
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(page.locator("#wizard-address")).toHaveValue(
    santiagoPin.address,
  );
  await page
    .locator("#wizard-address")
    .fill("Dirección sintética 100, Santiago");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await page.locator("#wizard-area").fill("40");
  await page.locator("#wizard-capacity").fill("2");
  for (const attribute of profile.attributes.filter((item) => item.required)) {
    const control = page.locator(`#attr-${attribute.code}`);
    if (attribute.type === "enum")
      await control.selectOption(attribute.options![0]!);
    else if (attribute.type === "boolean") await control.check();
    else await control.fill("1");
  }
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await page.locator("#wizard-price").fill("5001");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(page.locator("#wizard-timezone")).toBeVisible();
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(page.getByRole("heading", { name: /Paso 6:/ })).toBeVisible();
  await page
    .getByRole("button", { name: "Crear espacio y continuar a edición" })
    .click();

  await expect.poll(() => weeklyHoursCount).toBe(1);
  expect(createCount).toBe(1);
  expect(locationCount).toBe(0);
  expect(location).toMatchObject({
    address: "Dirección sintética 100, Santiago",
    region_code: "13",
    comuna_code: "13101",
    latitude: expect.any(Number),
    longitude: expect.any(Number),
  });
  expect(createBody).not.toHaveProperty("address");
  expect(availabilityCount).toBe(1);
  await expect(
    page
      .getByRole("region", { name: "Contenido de la sección de cuenta" })
      .getByRole("alert"),
  ).toContainText(/borrador.*creado/i);
  const editorLink = page.getByRole("link", {
    name: /editar.*borrador|ir al editor/i,
  });
  await expect(editorLink).toHaveAttribute(
    "href",
    `/anfitrion/espacios/${spaceId}/editar`,
  );
  const createButton = page.getByRole("button", {
    name: "Crear espacio y continuar a edición",
  });
  await expect(createButton).toBeDisabled();
  const stepper = page.getByRole("navigation", {
    name: "Pasos del asistente de publicación",
  });
  await stepper.getByRole("button", { name: /Tipo y Resumen/ }).click();
  await expect(editorLink).toBeVisible();
  await stepper.getByRole("button", { name: /Confirmación/ }).click();
  await expect(createButton).toBeDisabled();
  await editorLink.click();
  await page.waitForURL(`/anfitrion/espacios/${spaceId}/editar`);
  await expect(
    page.getByRole("heading", { name: "Borrador sintético recuperable" }),
  ).toBeVisible();
  const readsBeforeReload = spaceDetailGetCount;
  await page.reload();
  await expect(page).toHaveURL(`/anfitrion/espacios/${spaceId}/editar`);
  await expect
    .poll(() => spaceDetailGetCount)
    .toBeGreaterThan(readsBeforeReload);
  await expect(
    page.getByRole("heading", { name: "Borrador sintético recuperable" }),
  ).toBeVisible();
  expect(createCount).toBe(1);
  expect(availabilityCount).toBe(1);
  expect(weeklyHoursCount).toBe(1);
});

test("@phase7 @controlled wizard valida atributos requeridos opcionales sin serializar undefined", async ({
  page,
}) => {
  const bff = createBffFixtureState();
  let createBody: Record<string, unknown> | null = null;
  let locationBody: Record<string, unknown> | null = null;
  let createAttempts = 0;
  const locationWrites: Array<{
    location: Record<string, unknown>;
    etag: string | null;
  }> = [];
  let editedLocation: Record<string, unknown> | null = null;
  let reverseBody: unknown;
  let reverseCount = 0;
  let forwardQuery: unknown;
  let failSavedLookup = false;
  const profile = categoryProfiles.profiles.inmuebles;
  const required = profile.attributes.filter((attribute) => attribute.required);
  const duration = profile.attributes.find(
    (attribute) => attribute.code === "duracion_minima_unidades",
  );
  if (!duration || required.length === 0)
    throw new Error("Versioned inmuebles fixture is incomplete");

  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let result: {
      status: number;
      body?: unknown;
      headers?: Record<string, string>;
    };
    if (path === "/api/backend/auth/login" && request.method() === "POST") {
      result = {
        status: 200,
        body: {
          account_id: "770e8400-e29b-41d4-a716-446655440000",
          roles: ["arrendatario", "arrendador"],
          capabilities: [],
          expires_at: new Date(Date.now() + 3600000).toISOString(),
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/auth/session" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: {
          account_id: "770e8400-e29b-41d4-a716-446655440000",
          roles: ["arrendatario", "arrendador"],
          capabilities: [],
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/spaces/categories" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        body: { items: [{ code: "inmuebles", name: "Inmuebles" }] },
      };
    } else if (
      /^\/api\/backend\/spaces\/categories\/inmuebles\/attributes(?:\/1)?$/.test(
        path,
      ) &&
      request.method() === "GET"
    ) {
      result = { status: 200, body: profile };
    } else if (path === "/api/backend/spaces" && request.method() === "POST") {
      createBody = JSON.parse(request.postData() ?? "{}") as Record<
        string,
        unknown
      >;
      locationBody = createBody.location as Record<string, unknown>;
      createAttempts++;
      const locationValid = createAttempts > 1;
      if (!locationValid) {
        await route.fulfill({
          status: 422,
          json: {
            error: {
              code: "validation_error",
              message: "Revisa la posición del pin.",
              details: [
                {
                  field: "location.latitude",
                  code: "outside_selected_commune",
                },
              ],
            },
          },
        });
        return;
      }
      result = {
        status: 201,
        headers: { etag: '"wizard-create-v1"' },
        body: {
          id: "00000000-0000-4000-8000-000000000121",
          category_code: "inmuebles",
          category_name: "Inmuebles",
          title: createBody.title,
          description: createBody.description,
          state: "borrador",
          ...createBody,
          ...locationBody,
        },
      };
    } else if (
      path === "/api/backend/spaces/00000000-0000-4000-8000-000000000121" &&
      request.method() === "GET"
    ) {
      result = {
        status: 200,
        headers: { etag: '"wizard-editor-v1"' },
        body: {
          id: "00000000-0000-4000-8000-000000000121",
          category_code: "inmuebles",
          category_name: "Inmuebles",
          title: createBody?.title,
          description: createBody?.description,
          area_m2: createBody?.area_m2,
          capacity: createBody?.capacity,
          usage_rules: createBody?.usage_rules,
          rate_unit: createBody?.rate_unit,
          base_price_clp: createBody?.base_price_clp,
          address: locationBody?.address,
          state: "borrador",
          time_zone: "America/Santiago",
          region_code: locationBody?.region_code,
          comuna_code: locationBody?.comuna_code,
          attribute_schema_version: createBody?.attribute_schema_version,
          attributes: createBody?.attributes,
        },
      };
    } else if (
      path === "/api/backend/spaces/location/geocode" &&
      request.method() === "POST"
    ) {
      forwardQuery = request.postDataJSON();
      result = {
        status: 200,
        body: {
          candidates: [
            {
              label: "Centro de Valparaíso",
              latitude: -33.11533,
              longitude: -71.56463,
              precision: "approximate",
            },
          ],
        },
      };
    } else if (
      path === "/api/backend/spaces/location/reverse-geocode" &&
      request.method() === "POST"
    ) {
      reverseBody = request.postDataJSON();
      reverseCount++;
      const lookup = reverseBody as { latitude: number; longitude: number };
      const fromCreatedPin =
        locationBody?.latitude === lookup.latitude &&
        locationBody?.longitude === lookup.longitude;
      result = failSavedLookup
        ? {
            status: 503,
            body: {
              error: {
                code: "boundary_unavailable",
                message: "No se pudo consultar la comuna",
              },
            },
          }
        : reverseCount === 2
          ? {
              status: 422,
              body: {
                error: {
                  code: "pin_outside_coverage",
                  message: "Ubica el pin dentro de una zona válida en Chile.",
                },
              },
            }
          : {
              status: 200,
              body:
                reverseCount === 3 || fromCreatedPin
                  ? valparaisoPin
                  : santiagoPin,
            };
    } else if (
      path ===
      "/api/backend/spaces/00000000-0000-4000-8000-000000000121/location"
    ) {
      if (request.method() === "PUT") {
        editedLocation = request.postDataJSON();
        locationWrites.push({
          location: editedLocation!,
          etag: request.headers()["if-match"] ?? null,
        });
      }
      result = {
        status: 200,
        headers: {
          etag: editedLocation
            ? '"wizard-location-v2"'
            : '"wizard-location-v1"',
        },
        body: {
          ...locationBody,
          ...editedLocation,
          space_id: "00000000-0000-4000-8000-000000000121",
          confirmed_at: "2026-11-23T20:00:00.000Z",
          updated_at: "2026-11-23T20:00:00.000Z",
          revision: editedLocation ? 2 : 1,
        },
      };
    } else if (
      path ===
        "/api/backend/spaces/00000000-0000-4000-8000-000000000121/availability" &&
      request.method() === "PUT"
    ) {
      result = {
        status: 200,
        headers: { etag: '"wizard-time-v2"' },
        body: {
          space_id: "00000000-0000-4000-8000-000000000121",
          time_zone: "America/Santiago",
        },
      };
    } else if (
      path ===
        "/api/backend/spaces/00000000-0000-4000-8000-000000000121/weekly-hours" &&
      request.method() === "PUT"
    ) {
      result = {
        status: 200,
        headers: { etag: '"wizard-hours-v3"' },
        body: {
          data: {
            space_id: "00000000-0000-4000-8000-000000000121",
            enabled: false,
            time_zone: "America/Santiago",
            days: [],
          },
        },
      };
    } else {
      result = {
        status: 404,
        body: { error: { code: "not_found", message: "Not found" } },
      };
    }
    const adapted = bff.adapt(request, result);
    if (!adapted)
      throw new Error(
        `Missing controlled response for ${request.method()} ${path}`,
      );
    await route.fulfill({
      status: adapted.status,
      headers: {
        "content-type": "application/json; charset=utf-8",
        ...(adapted.headers ?? {}),
      },
      body: JSON.stringify(adapted.body ?? {}),
    });
  });

  await page.goto(
    `/acceso?return_to=${encodeURIComponent("/anfitrion/espacios/nuevo")}`,
  );
  await page
    .getByLabel("Correo electrónico")
    .fill("wizard-test@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticPass123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/anfitrion/espacios/nuevo");

  await page.locator("#wizard-category").selectOption("inmuebles");
  await page.locator("#wizard-title").fill("Casa de prueba sintética");
  await page
    .locator("#wizard-description")
    .fill(
      "Descripción sintética suficientemente extensa para completar la creación del inmueble, con información clara sobre su uso y características principales.",
    );
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  const map = page.getByLabel(mapName).locator("canvas");
  await map.click({ position: { x: 140, y: 100 } });
  await expect(page.locator("#wizard-address")).toHaveValue(
    santiagoPin.address,
  );
  await map.click({ position: { x: 250, y: 140 } });
  await expect(page.getByText(/zona válida en Chile/i).first()).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Siguiente", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Buscar dirección o lugar").fill("Valparaíso");
  await page.getByRole("button", { name: "Buscar", exact: true }).click();
  await page.getByRole("button", { name: /Centro de Valparaíso/ }).click();
  expect(forwardQuery).toEqual({ address: "Valparaíso" });
  await expect
    .poll(() => reverseBody)
    .toEqual({ latitude: -33.11533, longitude: -71.56463 });
  await expect(page.locator("#wizard-address")).toHaveValue(
    valparaisoPin.address,
  );
  await expect(
    page.getByText("Valparaíso", { exact: true }).last(),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Deshacer cambio de dirección" })
    .click();
  await expect(page.locator("#wizard-address")).toHaveValue(
    santiagoPin.address,
  );
  await expect(page.locator("#wizard-comuna")).toHaveText("Valparaíso");
  await expect(
    page.locator("select#wizard-region, select#wizard-comuna"),
  ).toHaveCount(0);
  const reversePath = "**/api/backend/spaces/location/reverse-geocode";
  const pending: Array<{
    point: { latitude: number; longitude: number };
    deliver: (answer: typeof santiagoPin) => void;
    done: Promise<void>;
  }> = [];
  const delayReverse = async (route: Route) => {
    let deliver!: (answer: typeof santiagoPin) => void;
    const reply = new Promise<typeof santiagoPin>((resolve) => {
      deliver = resolve;
    });
    const done = reply.then(async (answer) => {
      // The browser can close the superseded request before Playwright delivers it.
      await route.fulfill({ json: answer }).catch(() => undefined);
    });
    pending.push({ point: route.request().postDataJSON(), deliver, done });
    await done;
  };
  await page.route(reversePath, delayReverse);
  let resolvedPoint: { latitude: number; longitude: number };
  try {
    await map.click({ position: { x: 180, y: 132 } });
    await expect.poll(() => pending.length).toBe(1);
    const pin = await page
      .getByLabel(mapName)
      .locator(".maplibregl-marker")
      .first()
      .boundingBox();
    if (!pin) throw new Error("No se pudo ubicar el marcador para arrastrarlo");
    await page.mouse.move(pin.x + pin.width / 2, pin.y + pin.height / 2);
    await page.mouse.down();
    await page.mouse.move(
      pin.x + pin.width / 2 + 58,
      pin.y + pin.height / 2 + 24,
      { steps: 5 },
    );
    await page.mouse.up();
    await expect.poll(() => pending.length).toBe(2);
    await expect(page.locator("#wizard-comuna")).toContainText(/Detectando/);
    await page.locator("#wizard-address").fill("Av. corregida 125, Valparaíso");
    pending[1]!.deliver(valparaisoPin);
    await pending[1]!.done;
    await expect(page.locator("#wizard-comuna")).toHaveText("Valparaíso");
    pending[0]!.deliver(santiagoPin);
    await pending[0]!.done;
    await expect(page.locator("#wizard-comuna")).toHaveText("Valparaíso");
    await expect(page.locator("#wizard-address")).toHaveValue(
      "Av. corregida 125, Valparaíso",
    );
    resolvedPoint = pending[1]!.point;
  } finally {
    for (const request of pending) request.deliver(santiagoPin);
    await page.unroute(reversePath, delayReverse);
  }
  await page
    .getByRole("checkbox", { name: /Confirmo que esta dirección/ })
    .check();
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await page.locator("#wizard-area").fill("40");
  await page.locator("#wizard-capacity").fill("2");

  const durationField = page.locator("#attr-duracion_minima_unidades");
  await durationField.fill("2");
  await durationField.fill("");
  const requiredField = page.locator(`#attr-${required[0]!.code}`);
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(requiredField).toHaveAttribute("aria-invalid", "true");
  await expect(
    page
      .getByRole("region", { name: "Contenido de la sección de cuenta" })
      .getByRole("alert"),
  ).toContainText(required[0]!.label);
  await expect(
    page.getByRole("heading", { name: /Paso 3: Atributos/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Crear espacio y continuar a edición" }),
  ).toHaveCount(0);
  expect(createBody).toBeNull();

  const requiredEnum = required.find((attribute) => attribute.type === "enum");
  if (!requiredEnum) throw new Error("Required enum missing from fixture");
  const enumControl = page.locator(`#attr-${requiredEnum.code}`);
  await enumControl.selectOption("");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(enumControl).toHaveAttribute("aria-invalid", "true");
  await expect(enumControl).toHaveAttribute(
    "aria-describedby",
    new RegExp(`wizard-attribute-${requiredEnum.code}-error`),
  );
  await expect(
    page.locator(`#wizard-attribute-${requiredEnum.code}-error`),
  ).toContainText(requiredEnum.label);
  await expect(
    page.getByRole("heading", { name: /Paso 3: Atributos/ }),
  ).toBeVisible();
  expect(createBody).toBeNull();
  await enumControl.selectOption(requiredEnum.options![0]!);
  await expect(enumControl).not.toHaveAttribute("aria-invalid", "true");

  // Complete actual required fields from the checked-in category fixture.
  for (const attribute of required) {
    const control = page.locator(`#attr-${attribute.code}`);
    if (attribute.type === "enum")
      await control.selectOption(attribute.options![0]!);
    else if (attribute.type === "boolean") await control.check();
    else await control.fill("1");
  }
  await page.locator("#attr-superficie_construida_m2").fill("40");
  await page.locator("#attr-amoblado").check();
  await page.locator("#attr-amoblado").uncheck();
  await page.locator("#attr-dormitorios").fill("0");
  await page.locator("#attr-duracion_minima_unidades").fill("2");
  await page.locator("#attr-duracion_minima_unidades").fill("");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: /Paso 4: Tarifas iniciales y condiciones/,
    }),
  ).toBeVisible();

  await page.locator("#wizard-price").fill("5001");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(page.locator("#wizard-timezone")).toBeVisible();
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: /Paso 6: Resumen y confirmación de alta/,
    }),
  ).toBeVisible();

  const stepper = page.getByRole("navigation", {
    name: "Pasos del asistente de publicación",
  });
  await stepper.getByRole("button", { name: /Tipo y Resumen/ }).click();
  await expect(page.locator("#wizard-title")).toHaveValue(
    "Casa de prueba sintética",
  );
  await page.locator("#wizard-title").fill("Título actualizado por teclado");
  await page.keyboard.press("Tab");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await stepper.getByRole("button", { name: /Confirmación/ }).click();
  await expect(page.getByRole("heading", { name: /Paso 6:/ })).toBeVisible();
  await expect(page.locator("main")).toContainText(
    "Título actualizado por teclado",
  );

  // Invalidate only the title, without changing category/profile state.
  await stepper.getByRole("button", { name: /Tipo y Resumen/ }).click();
  await page.locator("#wizard-title").fill("");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await stepper.getByRole("button", { name: /Confirmación/ }).click();
  await expect(page.getByRole("heading", { name: /Paso 1:/ })).toBeVisible();
  await expect(page.locator("#wizard-title")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await expect(page.locator("#wizard-title")).toHaveAttribute(
    "aria-describedby",
    /wizard-title-error/,
  );
  await expect(page.locator("#wizard-title-error")).toContainText(
    "obligatorio",
  );
  expect(createBody).toBeNull();
  await page.locator("#wizard-title").fill("Título actualizado por teclado");
  await page.getByRole("button", { name: "Siguiente", exact: true }).click();
  await expect(page.getByRole("heading", { name: /Paso 2:/ })).toBeVisible();
  // Restore the final step using only visited checkpoints.
  for (const label of [/Tarifas y Reglas/, /Disponibilidad/, /Confirmación/]) {
    await stepper.getByRole("button", { name: label }).click();
  }
  await expect(page.locator("main")).toContainText(
    "Título actualizado por teclado",
  );

  const summary = await page.locator("main").innerText();
  expect(summary).toContain("Tipo de inmueble");
  expect(summary).toContain("Dormitorios: 0");
  expect(summary).not.toMatch(/duracion_minima_unidades|undefined|true|false/);
  const createButton = page.getByRole("button", {
    name: "Crear espacio y continuar a edición",
  });
  await expect(createButton).toBeEnabled();
  await createButton.click();
  await expect.poll(() => createAttempts).toBe(1);
  await expect(
    page.getByRole("link", { name: /ir al editor del borrador/i }),
  ).toHaveCount(0);
  await expect(createButton).toBeEnabled();
  await createButton.click();
  await page.waitForURL(
    /\/anfitrion\/espacios\/00000000-0000-4000-8000-000000000121\/editar/,
  );
  const updatedTitle = "Título actualizado por teclado";
  await expect(
    page.getByRole("heading", { name: updatedTitle, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("tablist", { name: "Secciones de edición del espacio" }),
  ).toBeVisible();
  expect(createAttempts).toBe(2);
  expect(locationWrites).toHaveLength(0);
  await page.getByRole("tab", { name: "Detalles" }).click();
  await expect(
    page.getByRole("textbox", { name: "Título del anuncio *" }),
  ).toHaveValue(updatedTitle);

  expect(createBody).not.toBeNull();
  expect(locationBody).toMatchObject({
    address: "Av. corregida 125, Valparaíso",
    region_code: "05",
    comuna_code: "05101",
    ...resolvedPoint,
    public_confirmed: true,
    source: "manual",
  });
  expect(createBody!.title).toBe(updatedTitle);
  expect(createBody).toMatchObject({
    category_code: "inmuebles",
    attribute_schema_version: 1,
    attributes: {
      tipo_inmueble: "Casa",
      tipo_de_espacio: "Inmueble completo",
      superficie_construida_m2: 40,
      dormitorios: 0,
      amoblado: false,
    },
  });
  expect(createBody!.attributes).not.toHaveProperty("duracion_minima_unidades");
  await page.getByRole("tab", { name: "Ubicación", exact: true }).click();
  const locationPanel = page.getByRole("tabpanel", { name: "Ubicación" });
  await expect(
    locationPanel.getByRole("textbox", { name: /^Dirección/ }),
  ).toHaveValue("Av. corregida 125, Valparaíso");
  await expect(locationPanel.locator("select")).toHaveCount(0);
  await locationPanel
    .getByLabel(mapName)
    .locator("canvas")
    .click({ position: { x: 140, y: 100 } });
  await expect(
    locationPanel.getByText("Santiago", { exact: true }).last(),
  ).toBeVisible();
  await locationPanel
    .getByRole("textbox", { name: /^Dirección/ })
    .fill("Av. editada 22, Santiago");
  await locationPanel
    .getByRole("checkbox", { name: /Confirmo que esta dirección/ })
    .check();
  await locationPanel
    .getByRole("button", { name: "Guardar ubicación" })
    .click();
  await expect.poll(() => locationWrites.length).toBe(1);
  expect(locationWrites[0]).toMatchObject({
    etag: '"wizard-location-v1"',
    location: {
      address: "Av. editada 22, Santiago",
      region_code: "13",
      comuna_code: "13101",
      latitude: expect.any(Number),
      longitude: expect.any(Number),
      public_confirmed: true,
    },
  });
  await page.reload();
  await page.getByRole("tab", { name: "Ubicación", exact: true }).click();
  await expect(
    locationPanel.getByRole("textbox", { name: /^Dirección/ }),
  ).toHaveValue("Av. editada 22, Santiago");
  await expect(
    locationPanel.getByRole("checkbox", {
      name: /Confirmo que esta dirección/,
    }),
  ).toBeChecked();
  failSavedLookup = true;
  await page.reload();
  await page.getByRole("tab", { name: "Ubicación", exact: true }).click();
  await expect(
    locationPanel.getByText(/No se pudo consultar la comuna/),
  ).toBeVisible();
  await expect(locationPanel.locator("#location-comuna")).toHaveText(
    "Pendiente de resolución",
  );
  await expect(
    locationPanel.getByRole("button", { name: "Guardar ubicación" }),
  ).toBeDisabled();
});
