import { expect, test, type Page } from "@playwright/test";

async function rejectOptionalStorage(page: Page) {
  await page
    .getByRole("dialog", { name: "Privacidad y cookies" })
    .getByRole("button", { name: "Rechazar opcionales" })
    .click();
}

test.beforeEach(async ({ page }) => {
  await page.route("**/__catalog-map-style.json", (route) =>
    route.fulfill({
      json: {
        version: 8,
        sources: {},
        layers: [
          {
            id: "test-background",
            type: "background",
            paint: { "background-color": "#e8efe6" },
          },
        ],
      },
    }),
  );
});
for (const viewport of [
  { width: 390, height: 844 },
  { width: 820, height: 1180 },
  { width: 1440, height: 1000 },
]) {
  test(`mapa y cuadrícula ${viewport.width}px`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const external: string[] = [];
    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      if (url.hostname !== "127.0.0.1" && url.hostname !== "localhost") {
        external.push(url.hostname);
        await route.abort();
      } else await route.fallback();
    });
    await page.goto("/explorar?q=Sala");
    await rejectOptionalStorage(page);
    await expect(
      page.getByRole("heading", { name: "Explorar arriendos" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Sala con ubicación confirmada/ }).first(),
    ).toBeVisible();
    await expect(
      page.getByLabel("Mapa de espacios", { exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Buscar en esta zona" }),
    ).toHaveCount(0);

    await page.getByRole("link", { name: /ver mapa/i }).click();
    await expect(page).toHaveURL(/\/explorar\/mapa\?q=Sala/);
    await expect(
      page.getByRole("heading", { name: "Mapa de arriendos" }),
    ).toBeVisible();
    await expect(page.locator("[data-space-id]")).toHaveCount(0);
    const map = page.getByLabel("Mapa de espacios", { exact: true });
    await expect(map.locator("canvas")).toBeVisible();
    await expect(
      map.getByRole("button", { name: "Acercar", exact: true }),
    ).toBeVisible();
    await map
      .getByRole("button", {
        name: "Sala fuera de la página de cuadrícula",
        exact: true,
      })
      .click();
    await expect(
      page.getByRole("heading", {
        name: "Sala fuera de la página de cuadrícula",
      }),
    ).toBeVisible();
    await expect(page.getByText("Calle Sintética 123").first()).toBeVisible();
    if (viewport.width < 1024) {
      const details = page.getByRole("heading", {
        name: "Espacios en el mapa",
      });
      await details
        .locator("..")
        .getByRole("button", { name: "Ocultar resultados" })
        .click();
    }
    await map.scrollIntoViewIfNeeded();
    const box = (await map.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(
      box.x + box.width / 2 - 50,
      box.y + box.height / 2 + 25,
      { steps: 5 },
    );
    await page.mouse.up();
    const search = page.getByRole("button", { name: "Buscar en esta zona" });
    await expect(search).toBeEnabled();
    await search.click();
    await expect(page).toHaveURL(/bbox=/);
    await expect(page).toHaveURL(/\/explorar\/mapa\?/);
    await page.getByRole("link", { name: /ver cuadrícula/i }).click();
    await expect(page).toHaveURL(
      (url) =>
        url.pathname === "/explorar" &&
        url.searchParams.get("q") === "Sala" &&
        url.searchParams.has("bbox"),
    );
    await expect(
      page.getByRole("link", { name: /Sala con ubicación confirmada/ }).first(),
    ).toBeVisible();
    await expect(
      page.getByLabel("Mapa de espacios", { exact: true }),
    ).toHaveCount(0);
    expect(external).toEqual([]);
  });
}
test("ubicación aproximada solo tras acción y destino manual disponible", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["geolocation"]);
  await context.setGeolocation({
    latitude: -33.456789,
    longitude: -70.654321,
    accuracy: 20,
  });
  await page.goto("/explorar/mapa");
  await rejectOptionalStorage(page);
  await expect(page).not.toHaveURL(/latitude=/);
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(page).toHaveURL(/latitude=-33.46/);
  await expect(page).toHaveURL(/longitude=-70.65/);
  await expect(page).toHaveURL(/radius_km=10/);
  await expect(
    page.getByRole("button", { name: /radio 10 kil[óo]metros/ }).first(),
  ).toBeVisible();
  await context.clearPermissions();
  await page.goto("/explorar/mapa");
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(page.getByText(/No pudimos usar tu ubicación/)).toBeVisible();
});
test("sin GPS, la comuna y el radio se conservan entre el feed y el mapa", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/explorar");
  await rejectOptionalStorage(page);
  await expect(page.locator("canvas")).toHaveCount(0);
  await page.getByLabel("Categoría de espacios").selectOption("oficina");
  await expect(page).toHaveURL(/category_code=oficina/);
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByLabel("Comuna de referencia").selectOption("05603");
  await page.getByLabel("Radio de búsqueda").selectOption("2");
  await page.getByRole("button", { name: "Aplicar cercanía" }).click();
  await expect(page).toHaveURL(/radius_km=2/);
  await expect(
    page.getByRole("link", { name: "Ampliar a 25 km" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /radio 2 kil[óo]metros/ })
    .first()
    .click();
  await page.getByLabel("Comuna de referencia").selectOption("13101");
  await page.getByRole("button", { name: "Aplicar cercanía" }).click();
  await expect(page.locator("[data-space-id]")).toHaveCount(2);
  await expect(page.getByText(/km/).first()).toBeVisible();
  await page.getByRole("link", { name: /ver mapa/i }).click();
  await expect(page).toHaveURL(
    (url) =>
      url.pathname === "/explorar/mapa" &&
      url.searchParams.get("radius_km") === "2" &&
      url.searchParams.get("category_code") === "oficina" &&
      url.searchParams.get("sort") === "distance_asc",
  );
  await expect(
    page.getByRole("button", { name: /Ver 2 resultados/ }),
  ).toBeVisible();
  await page.getByRole("link", { name: /ver cuadrícula/i }).click();
  await expect(page.locator("[data-space-id]")).toHaveCount(2);
  await expect(page.getByLabel("Categoría de espacios")).toHaveValue("oficina");
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
});

test("elegir un punto manualmente carga el mapa solo al abrirlo", async ({
  page,
}) => {
  await page.goto("/explorar");
  await rejectOptionalStorage(page);
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await expect(page.locator("canvas")).toHaveCount(0);
  await page.getByRole("button", { name: "Elegir en el mapa" }).click();
  const canvas = page.getByRole("dialog").locator("canvas").first();
  await expect(canvas).toBeVisible();
  await canvas.click({ position: { x: 90, y: 85 } });
  await expect(page.getByText(/Punto seleccionado en el mapa/)).toBeVisible();
  await page.getByLabel("Radio de búsqueda").selectOption("5");
  await page.getByRole("button", { name: "Aplicar cercanía" }).click();
  await expect(page).toHaveURL(
    (url) =>
      url.searchParams.get("radius_km") === "5" &&
      /^-?\d+\.\d{5}$/.test(url.searchParams.get("latitude") ?? ""),
  );
  await expect(page.locator("canvas")).toHaveCount(0);
});

test("ofrece alternativas si el GPS es impreciso", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["geolocation"]);
  await context.setGeolocation({
    latitude: -33.45,
    longitude: -70.66,
    accuracy: 8000,
  });
  await page.goto("/explorar");
  await rejectOptionalStorage(page);
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(
    page.getByText(/ubicación es demasiado imprecisa/),
  ).toBeVisible();
  await expect(page.getByLabel("Comuna de referencia")).toBeEnabled();
  await expect(page).not.toHaveURL(/latitude=/);
});

test("el mapa agrupa pines cercanos y mantiene la lista accesible", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/explorar/mapa?q=Grupo");
  await rejectOptionalStorage(page);
  const map = page.getByLabel("Mapa de espacios", { exact: true });
  await expect(
    map.getByRole("button", {
      name: "12 espacios en este punto. Abrir lista.",
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^Sala del grupo / }),
  ).toHaveCount(12);
});

test("la ficha del espacio conserva su mapa y dirección confirmada", async ({
  page,
}) => {
  await page.goto("/espacios/00000000-0000-4000-8000-000000000007");
  await rejectOptionalStorage(page);
  await expect(
    page.getByRole("heading", { name: "Ubicación", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Calle Sintética 123")).toBeVisible();
  await expect(
    page.getByLabel("Mapa de espacios", { exact: true }).locator("canvas"),
  ).toBeVisible();
});

test("en móvil busca por comuna, horario y capacidad sin GPS y cambia el área", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/explorar");
  await rejectOptionalStorage(page);
  await page.getByLabel("Categoría de espacios").selectOption("oficina");
  await expect(page).toHaveURL(/category_code=oficina/);
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByRole("button", { name: "Por comuna o región" }).click();
  await page
    .getByRole("combobox", { name: "Comuna", exact: true })
    .selectOption("13101");
  await page.getByRole("button", { name: "Buscar en esta comuna" }).click();
  await expect(page).toHaveURL(/areaMode=admin/);

  await page.getByRole("button", { name: "Filtrar" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Capacidad mínima").fill("4");
  await dialog.locator('input[name="start_local"]').fill("2027-07-02T09:00");
  await dialog.locator('input[name="end_local"]').fill("2027-07-02T11:00");
  await dialog.getByRole("button", { name: "Aplicar filtros" }).click();
  await expect(page).toHaveURL(
    (url) =>
      url.searchParams.get("start_at") === "2027-07-02T09:00:00-04:00" &&
      url.searchParams.get("end_at") === "2027-07-02T11:00:00-04:00" &&
      url.searchParams.get("comuna_code") === "13101" &&
      url.searchParams.get("min_capacity") === "4",
  );
  await expect(page.locator("[data-space-id]")).toHaveCount(1);
  await page
    .getByRole("combobox", { name: "Ordenar publicaciones" })
    .selectOption("estimated_total_asc");
  await expect(page).toHaveURL(/sort=estimated_total_asc/);
  await page.getByRole("link", { name: "Ver mapa" }).click();
  await expect(
    page.getByRole("button", { name: "Ver 1 resultado" }),
  ).toBeVisible();
  await page.getByRole("button", { name: /Ubicación y distancia/ }).click();
  await page.getByRole("button", { name: "Por cercanía" }).click();
  await page.getByRole("button", { name: "Aplicar cercanía" }).click();
  await expect(page).toHaveURL(
    (url) =>
      url.searchParams.get("areaMode") === "radius" &&
      !url.searchParams.has("region_code") &&
      !url.searchParams.has("comuna_code") &&
      url.searchParams.get("category_code") === "oficina" &&
      url.searchParams.get("min_capacity") === "4" &&
      url.searchParams.get("sort") === "estimated_total_asc" &&
      url.searchParams.has("start_at"),
  );
  await page
    .getByRole("link", { name: "Quitar filtro: Desde 4 personas" })
    .click();
  await expect(page).not.toHaveURL(/min_capacity/);
  await expect(
    page.getByRole("button", { name: "Ver 2 resultados" }),
  ).toBeVisible();
});

test("el orden por precio preselecciona una tarifa comparable", async ({
  page,
}) => {
  await page.goto("/explorar");
  await rejectOptionalStorage(page);
  await page
    .getByRole("combobox", { name: "Ordenar publicaciones" })
    .selectOption("unit_price_asc");
  await expect(page).toHaveURL(
    (url) =>
      url.searchParams.get("sort") === "unit_price_asc" &&
      url.searchParams.get("rate_unit") === "hora",
  );
  await page.getByRole("link", { name: "Quitar filtro: Por hora" }).click();
  await expect(page).toHaveURL(
    (url) =>
      !url.searchParams.has("sort") && !url.searchParams.has("rate_unit"),
  );
});

for (const viewport of [
  { width: 390, height: 844 },
  { width: 1440, height: 1000 },
]) {
  test(`volver de una ficha conserva scroll, filtros, cámara y selección (${viewport.width}px)`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await page.goto("/explorar?q=Grupo&min_capacity=4");
    const initialNotice = page.getByRole("dialog", {
      name: "Privacidad y cookies",
    });
    await expect(initialNotice).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(initialNotice).toBeVisible();
    await rejectOptionalStorage(page);
    const cards = page.locator("[data-space-id]");
    await expect(cards).toHaveCount(12);
    const card = cards.last();
    await card.scrollIntoViewIfNeeded();
    await card.getByRole("link").first().click();
    await expect(page).toHaveURL(/\/espacios\//);
    await page.goBack();
    await expect(page).toHaveURL(/min_capacity=4/);
    expect(
      await page.evaluate(() =>
        sessionStorage.getItem("espacigo.explore.visited.v1"),
      ),
    ).toBeNull();
    await expect
      .poll(() => page.evaluate(() => window.scrollY))
      .toBeGreaterThan(300);

    await page.goto("/explorar/mapa?q=Sala&min_capacity=4");
    const configure = page.getByRole("button", {
      name: "Preferencias de cookies",
      exact: true,
    });
    await configure.focus();
    await configure.press("Enter");
    const preferences = page.getByRole("dialog", {
      name: "Preferencias de cookies",
    });
    await expect(preferences).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(preferences).toBeHidden();
    await expect(configure).toBeFocused();
    await configure.press("Enter");
    await preferences
      .getByRole("checkbox", {
        name: /Recordar espacios visitados/,
      })
      .check();
    await preferences
      .getByRole("button", { name: "Aceptar opcionales" })
      .click();
    const map = page.getByLabel("Mapa de espacios", { exact: true });
    const pin = map.getByRole("button", {
      name: "Sala con ubicación confirmada",
      exact: true,
    });
    await expect(pin).toBeVisible();
    await map.getByRole("button", { name: "Acercar", exact: true }).click();
    await expect(
      page.getByRole("status").filter({ hasText: "Área pendiente" }),
    ).toBeVisible();
    await expect(page).not.toHaveURL(/bbox=/);
    const getSaved = () =>
      page.evaluate(() => {
        const sessions = JSON.parse(
          sessionStorage.getItem("espacigo.explore.continuity.v1") ?? "{}",
        );
        return sessions["q=Sala&min_capacity=4"];
      });
    const before = await getSaved();
    expect(before.camera.zoom).toBeGreaterThan(13);
    await pin.click();
    await page.getByRole("link", { name: "Ver espacio", exact: true }).click();
    await expect(page).toHaveURL(/\/espacios\//);
    await page.goBack();
    await expect(page).toHaveURL(/min_capacity=4/);
    await expect(
      page.getByRole("heading", {
        name: "Sala con ubicación confirmada",
        exact: true,
      }),
    ).toBeVisible();
    await expect
      .poll(async () => (await getSaved()).camera)
      .toEqual(before.camera);
    const restored = await getSaved();
    const visited = await page.evaluate(() =>
      JSON.parse(sessionStorage.getItem("espacigo.explore.visited.v1") ?? "[]"),
    );
    expect(visited).toContain(restored.selectedSpaceId);
    await page.getByRole("button", { name: "Preferencias de cookies" }).click();
    await preferences
      .getByRole("button", { name: "Rechazar opcionales" })
      .click();
    expect(
      await page.evaluate(() =>
        sessionStorage.getItem("espacigo.explore.visited.v1"),
      ),
    ).toBeNull();
    await page.reload();
    await expect(
      page.getByRole("dialog", {
        name: "Privacidad y cookies",
      }),
    ).toHaveCount(0);
  });
}
