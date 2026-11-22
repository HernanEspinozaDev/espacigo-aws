import { expect, test } from "@playwright/test";
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
      } else await route.continue();
    });
    await page.goto("/explorar?q=Sala");
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

    await page.getByRole("link", { name: "Ver mapa", exact: true }).click();
    await expect(page).toHaveURL(/\/explorar\/mapa\?q=Sala/);
    await expect(
      page.getByRole("heading", { name: "Mapa de arriendos" }),
    ).toBeVisible();
    await expect(page.locator("[data-space-id]")).toHaveCount(0);
    await expect(
      page.getByText("2 espacios con ubicación confirmada.", { exact: false }),
    ).toBeVisible();
    await expect(page.locator(".leaflet-interactive")).toHaveCount(2);
    await page.locator(".leaflet-interactive").last().click();
    await expect(
      page.getByRole("heading", {
        name: "Sala fuera de la página de cuadrícula",
      }),
    ).toBeVisible();
    await expect(page.getByText("Calle Sintética 123").first()).toBeVisible();
    const map = page.getByLabel("Mapa de espacios", { exact: true });
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
    await page
      .getByRole("link", { name: "Ver cuadrícula", exact: true })
      .click();
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
  await expect(page).not.toHaveURL(/latitude=/);
  await page.getByRole("button", { name: "Cerca de mí" }).click();
  await expect(page).toHaveURL(/latitude=-33.46/);
  await expect(page).toHaveURL(/longitude=-70.65/);
  await expect(page).toHaveURL(/radius_km=10/);
  await expect(page.getByText(/ubicación aproximada/)).toBeVisible();
  await context.clearPermissions();
  await page.goto("/explorar/mapa");
  await page.getByRole("button", { name: "Cerca de mí" }).click();
  await expect(page.getByText(/Puedes escribir un destino/)).toBeVisible();
});
test("la ficha del espacio conserva su mapa y dirección confirmada", async ({
  page,
}) => {
  await page.goto("/espacios/00000000-0000-4000-8000-000000000007");
  await expect(
    page.getByRole("heading", { name: "Ubicación", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Calle Sintética 123")).toBeVisible();
});
