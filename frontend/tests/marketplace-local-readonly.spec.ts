import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

// Opt-in only: requires the separately owned worker-free API harness.
// No database credentials, writes, booking operations or account provisioning.
test.skip(
  process.env.MARKETPLACE_NATIVE_READONLY !== "1",
  "Explicit read-only native acceptance required",
);

for (const viewport of [
  { name: "desktop", width: 1440, height: 1100 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`real PostgreSQL public detail reaches the visible UI — ${viewport.name}`, async ({
    page,
    request,
  }, testInfo) => {
    const ready = await request.get("http://127.0.0.1:18122/health/ready");
    expect(ready.status()).toBe(200);
    expect(await ready.json()).toEqual({ status: "ready", read_only: true });
    const listed = await request.get(
      "http://127.0.0.1:18122/api/v1/public/spaces?page_size=1",
    );
    expect(listed.status()).toBe(200);
    const inventory = await listed.json();
    expect(inventory.items.length).toBe(1);
    const id = inventory.items[0].space_id;
    const detail = await request.get(
      `http://127.0.0.1:18122/api/v1/public/spaces/${id}`,
    );
    expect(detail.status()).toBe(200);
    const { data } = await detail.json();
    expect(Object.keys(data.host).sort()).toEqual([
      "identity_verified",
      "joined_at",
      "name",
      "spaces_count",
    ]);
    expect(data.capacity_max).toBeGreaterThan(0);
    expect(data.cancellation_policy).toBeNull();
    expect(data.guarantee).toBeNull();

    const external: string[] = [];
    await page.route("**/*", async (route) => {
      const url = new URL(route.request().url());
      if (
        url.origin === "http://127.0.0.1:4313" ||
        ["data:", "blob:"].includes(url.protocol)
      )
        return route.continue();
      external.push(url.origin);
      return route.abort("blockedbyclient");
    });
    await page.setViewportSize(viewport);
    const response = await page.goto(`/espacios/${id}`, {
      waitUntil: "domcontentloaded",
    });
    expect(response?.status()).toBe(200);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(
      page.getByText(`Hasta ${data.capacity_max} personas`, { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "Especificaciones y reglas",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Quién publica", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "Reseñas y recomendaciones",
        exact: true,
      }),
    ).toBeVisible();
    const host = page.getByRole("region", {
      name: "Quién publica",
      exact: true,
    });
    await expect(
      host.getByText(
        `${data.host.spaces_count} ${data.host.spaces_count === 1 ? "espacio publicado" : "espacios publicados"}`,
        { exact: true },
      ),
    ).toBeVisible();
    expect(
      await host
        .getByText("Identidad Verificada por EspaciGo", { exact: true })
        .count(),
    ).toBe(data.host.identity_verified ? 1 : 0);
    await expect(
      page.getByRole("button", { name: "Hoy", exact: true }),
    ).toBeVisible();
    const photo = page
      .getByRole("region", { name: "Galería de imágenes", exact: true })
      .getByRole("button", { name: /^Ver imagen 1 de / })
      .getByRole("img");
    await expect(photo).toBeVisible();
    await expect
      .poll(() =>
        photo.evaluate(
          (image: HTMLImageElement) => image.complete && image.naturalWidth > 0,
        ),
      )
      .toBe(true);
    await page.evaluate(async () => {
      await document.fonts.ready;
      await Promise.all(
        Array.from(
          document.querySelectorAll<HTMLImageElement>("article img"),
        ).map((image) => image.decode().catch(() => undefined)),
      );
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
    expect(external).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`native-${viewport.name}.png`),
      fullPage: true,
    });
  });
}
