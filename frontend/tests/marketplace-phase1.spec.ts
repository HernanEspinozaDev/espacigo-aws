import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const idFor = (n: number) =>
  `10000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const views = [
  { name: "desktop", width: 1440, height: 1100 },
  { name: "mobile", width: 390, height: 844 },
];
for (const viewport of views) {
  test(`three-column marketplace, complete public data and no overflow — ${viewport.name}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    const external: string[] = [];
    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      if (
        ["blob:", "data:"].includes(url.protocol) ||
        ["127.0.0.1", "localhost"].includes(url.hostname)
      )
        return route.continue();
      external.push(url.origin);
      return route.abort("blockedbyclient");
    });
    const response = await page.goto(`/espacios/${idFor(1)}`);
    expect(response?.status()).toBe(200);
    const html = await response!.text();
    for (const marker of [
      "PRIVATE_OWNER_MARKER",
      "PRIVATE_EMAIL_MARKER",
      "PRIVATE_ADDRESS_MARKER",
      "PRIVATE_DOCUMENT_MARKER",
    ])
      expect(html).not.toContain(marker);
    await expect(
      page.getByRole("heading", {
        name: "Espacio de prueba del marketplace",
        exact: true,
      }),
    ).toBeVisible();
    const disclosure = page.getByRole("button", {
      name: "Ver más",
      exact: true,
    });
    await disclosure.click();
    await expect(
      page.getByRole("button", { name: "Ver menos", exact: true }),
    ).toHaveAttribute("aria-expanded", "true");
    await page.getByRole("button", { name: "Ver menos", exact: true }).click();
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");
    await expect(
      page.getByText("Anfitrión Público", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("Puestos de trabajo", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText(/Identidad Verificada por EspaciGo/i),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Hoy", exact: true }),
    ).toBeVisible();
    const specifications = page.getByRole("region", {
      name: "Especificaciones y reglas",
      exact: true,
    });
    await expect(
      specifications.getByText(/Política de cancelación no informada/i),
    ).toBeVisible();
    await expect(
      specifications.getByText(/Garantía no informada/i),
    ).toBeVisible();
    if (viewport.name === "desktop") {
      const summary = await page
        .getByRole("heading", { level: 1 })
        .boundingBox();
      const calendar = await page
        .getByRole("region", {
          name: "Disponibilidad para arriendo",
          exact: true,
        })
        .boundingBox();
      const gallery = await page
        .getByRole("region", { name: "Galería de imágenes", exact: true })
        .boundingBox();
      expect(summary && calendar && gallery).toBeTruthy();
      expect(summary!.x).toBeLessThan(calendar!.x);
      expect(calendar!.x).toBeLessThan(gallery!.x);
      expect(Math.abs(calendar!.width - gallery!.width)).toBeLessThan(2);
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({
      path: testInfo.outputPath(`${viewport.name}.png`),
      fullPage: true,
    });
    const audit = await new AxeBuilder({ page }).analyze();
    expect(audit.violations).toEqual([]);
    expect(external).toEqual([]);
  });
}

test("mosaic and lightbox keep keyboard focus and navigate all images", async ({
  page,
}) => {
  await page.goto(`/espacios/${idFor(1)}`);
  const gallery = page.getByRole("region", {
    name: "Galería de imágenes",
    exact: true,
  });
  const opener = gallery.getByRole("button", {
    name: "Ver imagen 1 de 5",
    exact: true,
  });
  await expect(opener.locator("img")).toBeVisible();
  await expect
    .poll(() =>
      opener
        .locator("img")
        .evaluate(
          (image: HTMLImageElement) => image.complete && image.naturalWidth > 0,
        ),
    )
    .toBe(true);
  await opener.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect
    .poll(() =>
      dialog
        .locator("img")
        .evaluate(
          (image: HTMLImageElement) => image.complete && image.naturalWidth > 0,
        ),
    )
    .toBe(true);
  const close = dialog.getByRole("button", {
    name: "Cerrar galería",
    exact: true,
  });
  await expect(close).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  expect(
    await dialog.evaluate((element) =>
      element.contains(document.activeElement),
    ),
  ).toBe(true);
  await page.keyboard.press("Tab");
  expect(
    await dialog.evaluate((element) =>
      element.contains(document.activeElement),
    ),
  ).toBe(true);
  await dialog
    .getByRole("button", { name: "Imagen siguiente", exact: true })
    .click();
  await expect(dialog).toHaveAttribute("aria-label", /Imagen 2 de 5/);
  await page.keyboard.press("ArrowRight");
  await expect(dialog).toHaveAttribute("aria-label", /Imagen 3 de 5/);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
});

test("zero and one image are honest and do not invent navigation", async ({
  page,
}) => {
  await page.goto(`/espacios/${idFor(2)}`);
  await expect(
    page.getByText("Este espacio no tiene imágenes públicas.", { exact: true }),
  ).toBeVisible();
  await page.goto(`/espacios/${idFor(3)}`);
  const gallery = page.getByRole("region", {
    name: "Galería de imágenes",
    exact: true,
  });
  await gallery
    .getByRole("button", { name: "Ver imagen 1 de 1", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Imagen siguiente", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
});

test("failed images preserve visible navigation and focus at both viewports", async ({
  page,
}) => {
  for (const viewport of views) {
    await page.setViewportSize(viewport);
    await page.goto(`/espacios/${idFor(6)}`);
    const gallery = page.getByRole("region", {
      name: "Galería de imágenes",
      exact: true,
    });
    const opener = gallery.getByRole("button", {
      name: "Ver imagen 1 de 5",
      exact: true,
    });
    await expect(
      opener.getByText("Imagen no disponible", { exact: true }),
    ).toBeAttached();
    await opener.click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByText("Imagen no disponible", { exact: true }),
    ).toBeAttached();
    await dialog
      .getByRole("button", { name: "Imagen siguiente", exact: true })
      .click();
    await expect(dialog).toHaveAttribute("aria-label", /Imagen 2 de 5/);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(opener).toBeFocused();
  }
});

test("missing optimized variants fall back to the original public image", async ({
  page,
}) => {
  await page.goto(`/espacios/${idFor(4)}`);
  const opener = page.getByRole("button", {
    name: "Ver imagen 1 de 1",
    exact: true,
  });
  const image = opener.locator("img");
  await expect
    .poll(() =>
      image.evaluate(
        (node: HTMLImageElement) => node.complete && node.naturalWidth > 0,
      ),
    )
    .toBe(true);
  expect(await image.getAttribute("src")).toMatch(/\/images\/[0-9a-f-]+$/);
});

test("server selection synchronizes desktop and mobile CTAs and survives the handoff", async ({
  page,
}, testInfo) => {
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method()))
      mutations.push(request.method() + " " + new URL(request.url()).pathname);
  });
  await page.goto(`/espacios/${idFor(1)}`);
  const conversion = page.getByRole("region", {
    name: "Solicitud de arriendo",
    exact: true,
  });
  await expect(
    conversion.getByRole("button", {
      name: "Selecciona fechas en el calendario",
      exact: true,
    }),
  ).toBeVisible();
  const calendar = page.getByRole("region", {
    name: "Disponibilidad para arriendo",
    exact: true,
  });
  await calendar
    .getByRole("button", { name: "Mes siguiente", exact: true })
    .click();
  const firstRead = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      response
        .url()
        .includes(`/api/backend/spaces/${idFor(1)}/rental-availability?`) &&
      new URL(response.url()).searchParams.get("duration") === "1",
  );
  await calendar
    .locator("button[aria-pressed]:not([disabled])")
    .first()
    .click();
  expect((await firstRead).status()).toBe(200);
  await calendar
    .getByRole("button", { name: /^\d{2}:\d{2} – \d{2}:\d{2}$/ })
    .first()
    .click();
  const action = conversion.getByRole("link", {
    name: /^Solicitar Arriendo — /,
  });
  await expect(action).toHaveText(
    `Solicitar Arriendo — $${BigInt(12000).toLocaleString("es-CL")} CLP`,
  );

  const secondRead = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      response
        .url()
        .includes(`/api/backend/spaces/${idFor(1)}/rental-availability?`) &&
      new URL(response.url()).searchParams.get("duration") === "2",
  );
  await calendar.getByRole("button", { name: "2 horas", exact: true }).click();
  await expect(action).toHaveCount(0);
  const authoritative = await secondRead;
  expect(authoritative.status()).toBe(200);
  const { data } = await authoritative.json();
  await calendar
    .getByRole("button", { name: /^\d{2}:\d{2} – \d{2}:\d{2}$/ })
    .first()
    .click();
  await expect(action).toHaveText(
    `Solicitar Arriendo — $${(BigInt(12000) * BigInt(2)).toLocaleString("es-CL")} CLP`,
  );
  const href = await action.getAttribute("href");
  expect(href).toBeTruthy();
  const target = new URL(href!, "http://127.0.0.1:4312");
  expect(target.pathname).toBe("/reservas/nueva");
  expect(target.searchParams.get("spaceId")).toBe(idFor(1));
  expect(target.searchParams.get("start")).toBe(data.items[0].start_at);
  expect(target.searchParams.get("end")).toBe(data.items[0].end_at);
  expect(target.searchParams.get("time_zone")).toBe(data.time_zone);
  expect(target.searchParams.get("duration")).toBe("2");
  expect(target.searchParams.get("source")).toBe("marketplace");
  expect(target.searchParams.has("price")).toBe(false);
  await page.setViewportSize({ width: 390, height: 844 });
  const mobile = page.getByRole("region", {
    name: "Solicitud de arriendo móvil",
    exact: true,
  });
  await expect(
    mobile.getByRole("link", { name: /^Solicitar Arriendo — / }),
  ).toHaveAttribute("href", href!);
  const box = await mobile.getByRole("link").boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("mobile-selected.png"),
    fullPage: true,
  });
  await mobile.getByRole("link").click();
  await expect(page).toHaveURL(target.href);
  await expect(
    page.getByRole("heading", {
      name: "Preparar solicitud de arriendo",
      exact: true,
    }),
  ).toBeVisible();
  expect(mutations).toEqual([]);
});

test("an availability error never enables a booking action", async ({
  page,
}) => {
  await page.goto(`/espacios/${idFor(7)}`);
  const conversion = page.getByRole("region", {
    name: "Solicitud de arriendo",
    exact: true,
  });
  const calendar = page.getByRole("region", {
    name: "Disponibilidad para arriendo",
    exact: true,
  });
  const denied = page.waitForResponse((response) =>
    response
      .url()
      .includes(`/api/backend/spaces/${idFor(7)}/rental-availability?`),
  );
  await calendar
    .locator("button[aria-pressed]:not([disabled])")
    .first()
    .click();
  expect((await denied).status()).toBe(503);
  await expect(
    calendar.getByText(
      "No se pudo consultar la disponibilidad en este momento.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    conversion.getByRole("link", { name: /^Solicitar Arriendo/ }),
  ).toHaveCount(0);
  await expect(
    conversion.getByRole("button", {
      name: "Selecciona fechas en el calendario",
      exact: true,
    }),
  ).toBeVisible();
});
