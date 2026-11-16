import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test.skip(
  process.env.CALENDAR_NATIVE_READONLY !== "1",
  "Opt-in: reads the retained local API and published inventory; creates no accounts or bookings.",
);

const apiOrigin =
  process.env.CALENDAR_NATIVE_API_ORIGIN ?? "http://127.0.0.1:8080";
const parsedAPI = new URL(apiOrigin);
if (parsedAPI.protocol !== "http:" || parsedAPI.hostname !== "127.0.0.1") {
  throw new Error("Calendar native reads must target the explicit local API.");
}

interface MonthSlot {
  name: string;
  ticket?: string;
  start?: string;
  end?: string;
  available: boolean;
}
interface MonthDay {
  date: string;
  status: "AVAILABLE" | "PARTIAL" | "BLOCKED" | "BOOKED" | "PAST";
  price_clp: number;
  slots: MonthSlot[];
}
interface MonthRead {
  month: string;
  time_zone: string;
  rate_unit: "hora" | "dia" | "mes";
  base_price_clp: number;
  days: MonthDay[];
}

function civilDate(timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone,
  }).formatToParts(new Date());
  const value = (type: string) =>
    parts.find((part) => part.type === type)?.value;
  return `${value("year")}-${value("month")}-${value("day")}`;
}

for (const viewport of [
  { width: 1440, height: 1100 },
  { width: 390, height: 844 },
]) {
  test(`native monthly BFF and published day ${viewport.width}px`, async ({
    page,
    request,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    const catalogResponse = await request.get(
      `${apiOrigin}/api/v1/public/spaces?page_size=48`,
    );
    expect(catalogResponse.status()).toBe(200);
    const catalog = (await catalogResponse.json()) as {
      items: { space_id: string; time_zone: string }[];
    };
    const item = catalog.items[0];
    expect(
      item,
      "The retained local database must contain a published listing.",
    ).toBeTruthy();
    const today = civilDate(item.time_zone ?? "America/Santiago");
    const month = today.slice(0, 7);
    // The legacy GET is a native read baseline only, not the browser route under test.
    const nativeResponse = await request.get(
      `${apiOrigin}/api/v1/spaces/${item.space_id}/availability?month=${month}`,
    );
    expect(nativeResponse.status()).toBe(200);
    const native = (await nativeResponse.json()) as MonthRead;
    const expectedDay = native.days.find(
      (day) => day.date > today && day.status === "AVAILABLE",
    );
    expect(
      expectedDay,
      "A native future available day is required; do not synthesize it.",
    ).toBeTruthy();
    if (!expectedDay) throw new Error("No native future available day.");

    const writes: string[] = [];
    page.on("request", (r) => {
      if (!["GET", "HEAD", "OPTIONS"].includes(r.method()))
        writes.push(`${r.method()} ${new URL(r.url()).pathname}`);
    });
    const monthRead = page.waitForResponse((r) => {
      const url = new URL(r.url());
      return (
        url.origin === testInfo.project.use.baseURL &&
        url.pathname.includes(item.space_id) &&
        url.searchParams.get("month") === month
      );
    });
    await page.goto(`/espacios/${item.space_id}`, {
      waitUntil: "domcontentloaded",
    });
    const response = await monthRead;
    expect(
      response.status(),
      "The browser month request must traverse the real BFF successfully.",
    ).toBe(200);
    const observed = (await response.json()) as MonthRead;
    expect(observed.time_zone).toBe(native.time_zone);
    expect(observed.days.find((day) => day.date === expectedDay.date)).toEqual(
      expectedDay,
    );

    const longDate = new Intl.DateTimeFormat("es-CL", {
      weekday: "long",
      day: "numeric",
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    }).format(new Date(`${expectedDay.date}T12:00:00Z`));
    const day = page.getByRole("button").filter({ hasText: longDate });
    await expect(day).toBeEnabled();
    await expect(day).not.toContainText("Sin información");
    await expect(day).toContainText("$");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(writes).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath("native-month.png"),
      fullPage: true,
    });
  });
}

test("public month empty state is honest and non-actionable (mocked empty response)", async ({
  page,
  request,
}, testInfo) => {
  const viewport = { width: 390, height: 844 };
  await page.setViewportSize(viewport);
  const catalogResponse = await request.get(
    `${apiOrigin}/api/v1/public/spaces?page_size=48`,
  );
  expect(catalogResponse.status()).toBe(200);
  const catalog = (await catalogResponse.json()) as {
    items: { space_id: string; time_zone: string }[];
  };
  const item = catalog.items[0];
  expect(item).toBeTruthy();
  if (!item) throw new Error("No published real space for controlled test.");
  const month = civilDate(item.time_zone).slice(0, 7);
  const path = `/api/backend/public/spaces/${item.space_id}/month`;
  let intercepted = 0;
  const writes: string[] = [];
  page.on("request", (r) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(r.method()))
      writes.push(`${r.method()} ${new URL(r.url()).pathname}`);
  });
  await page.route(`**${path}*`, async (route) => {
    const routedURL = new URL(route.request().url());
    expect(routedURL.pathname).toBe(path);
    expect(routedURL.searchParams.get("month")).toBe(month);
    intercepted += 1;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        month,
        time_zone: item.time_zone,
        rate_unit: "hora",
        base_price_clp: 10000,
        days: [],
      } satisfies MonthRead),
    });
  });
  await page.goto(`/espacios/${item.space_id}`, {
    waitUntil: "domcontentloaded",
  });
  await expect.poll(() => intercepted).toBe(1);
  const calendar = page
    .getByRole("region")
    .filter({ has: page.getByRole("button", { name: "Mes siguiente" }) });
  await expect(
    calendar.getByRole("button", { name: /disponibilidad no confirmada/i }),
  ).toHaveCount(
    new Date(
      Date.UTC(
        Number(month.slice(0, 4)),
        Number.parseInt(month.slice(5), 10),
        0,
      ),
    ).getUTCDate(),
  );
  await expect
    .soft(page.getByRole("heading", { name: /horarios para/i }))
    .toHaveCount(0);
  await expect
    .soft(
      page
        .getByRole("button", { name: "Selecciona fechas en el calendario" })
        .first(),
    )
    .toBeDisabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(writes).toEqual([]);
  const axe = await new AxeBuilder({ page }).analyze();
  expect(axe.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("empty-mobile.png"),
    fullPage: true,
  });
});

test("public month API failure stays non-actionable (mocked failure)", async ({
  page,
  request,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const catalogResponse = await request.get(
    `${apiOrigin}/api/v1/public/spaces?page_size=48`,
  );
  expect(catalogResponse.status()).toBe(200);
  const catalog = (await catalogResponse.json()) as {
    items: { space_id: string; time_zone: string }[];
  };
  const item = catalog.items[0];
  expect(item).toBeTruthy();
  if (!item) throw new Error("No published real space for controlled test.");
  const month = civilDate(item.time_zone).slice(0, 7);
  const path = `/api/backend/public/spaces/${item.space_id}/month`;
  let intercepted = 0;
  const writes: string[] = [];
  page.on("request", (r) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(r.method()))
      writes.push(`${r.method()} ${new URL(r.url()).pathname}`);
  });
  await page.route(`**${path}*`, async (route) => {
    const routedURL = new URL(route.request().url());
    expect(routedURL.pathname).toBe(path);
    expect(routedURL.searchParams.get("month")).toBe(month);
    intercepted += 1;
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: "{}",
    });
  });
  await page.goto(`/espacios/${item.space_id}`, {
    waitUntil: "domcontentloaded",
  });
  await expect
    .soft(
      page.getByText(
        "No se pudo cargar la disponibilidad de este mes. Inténtalo de nuevo.",
      ),
    )
    .toBeVisible();
  await expect.poll(() => intercepted).toBe(1);
  await expect
    .soft(page.getByRole("heading", { name: /horarios para/i }))
    .toHaveCount(0);
  await expect
    .soft(
      page
        .getByRole("button", { name: "Selecciona fechas en el calendario" })
        .first(),
    )
    .toBeDisabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(writes).toEqual([]);
  const axe = await new AxeBuilder({ page }).analyze();
  expect(axe.violations).toEqual([]);
});
