import { expect, test } from "@playwright/test";

test.skip(
  process.env.CALENDAR_NATIVE_READONLY !== "1",
  "Explicit retained-local read-only acceptance only.",
);
const apiOrigin =
  process.env.CALENDAR_NATIVE_API_ORIGIN ?? "http://127.0.0.1:8080";
if (
  new URL(apiOrigin).hostname !== "127.0.0.1" ||
  new URL(apiOrigin).protocol !== "http:"
) {
  throw new Error("Native calendar selection must remain on loopback.");
}

for (const viewport of [
  { width: 1440, height: 1100 },
  { width: 390, height: 844 },
]) {
  test(`native authoritative interval reaches CTA exactly ${viewport.width}px`, async ({
    page,
    request,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    const response = await request.get(
      `${apiOrigin}/api/v1/public/spaces?page_size=48`,
    );
    expect(response.status()).toBe(200);
    const catalog = (await response.json()) as {
      items: { space_id: string; rate_unit: string }[];
    };
    const listing = catalog.items.find((item) => item.rate_unit === "dia");
    expect(
      listing,
      "Need a real published daily listing, not a fabricated fixture.",
    ).toBeTruthy();
    if (!listing) throw new Error("No native daily listing.");
    const now = new Intl.DateTimeFormat("en-CA", {
      timeZone: "America/Santiago",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).format(new Date());
    const month = now.slice(0, 7);
    const monthRead = await request.get(
      `${apiOrigin}/api/v1/public/spaces/${listing.space_id}/month?month=${month}`,
    );
    expect(monthRead.status()).toBe(200);
    const nativeMonth = (await monthRead.json()) as {
      days: { date: string; status: string }[];
    };
    const date = nativeMonth.days.find(
      (day) => day.date > now && day.status === "AVAILABLE",
    )?.date;
    expect(
      date,
      "Need a future day from the actual Go month projection.",
    ).toBeTruthy();
    if (!date) throw new Error("No native future available day.");

    const reads: string[] = [];
    const writes: string[] = [];
    page.on("request", (r) => {
      if (
        new URL(r.url()).pathname.endsWith(
          `/${listing.space_id}/rental-availability`,
        )
      )
        reads.push(r.url());
      if (!["GET", "HEAD", "OPTIONS"].includes(r.method()))
        writes.push(r.method());
    });
    const loadedMonth = page.waitForResponse((r) =>
      new URL(r.url()).pathname.endsWith(`/${listing.space_id}/month`),
    );
    await page.goto(`/espacios/${listing.space_id}`, {
      waitUntil: "domcontentloaded",
    });
    expect((await loadedMonth).status()).toBe(200);
    const dayLabel = new Intl.DateTimeFormat("es-CL", {
      weekday: "long",
      day: "numeric",
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    }).format(new Date(`${date}T12:00:00Z`));
    const day = page.getByRole("button").filter({ hasText: dayLabel });
    await expect(day).toBeEnabled();
    const available = page.waitForResponse((r) => {
      const url = new URL(r.url());
      return (
        url.pathname.endsWith(`/${listing.space_id}/rental-availability`) &&
        url.searchParams.get("date") === date
      );
    });
    await day.click();
    const intervalResponse = await available;
    expect(
      intervalResponse.status(),
      "Public interval reads must be mounted in the selected local runtime.",
    ).toBe(200);
    const body = (await intervalResponse.json()) as {
      data: {
        space_id: string;
        time_zone: string;
        rate_unit: string;
        items: { start_at: string; end_at: string }[];
      };
    };
    expect(body.data.space_id).toBe(listing.space_id);
    const interval = body.data.items[0];
    expect(
      interval,
      "Native authority must provide an interval; empty is not a booking success.",
    ).toBeTruthy();
    if (!interval)
      throw new Error("Native interval authority returned no slots.");
    const links = page.getByRole("link", { name: /Solicitar arriendo/ });
    await expect(links.first()).toHaveAttribute("href", /source=marketplace/);
    for (const href of await links.evaluateAll((elements) =>
      elements.map((element) => element.getAttribute("href")),
    )) {
      const target = new URL(href ?? "", "http://127.0.0.1");
      expect(target.searchParams.get("start")).toBe(interval.start_at);
      expect(target.searchParams.get("end")).toBe(interval.end_at);
      expect(target.searchParams.get("time_zone")).toBe(body.data.time_zone);
      expect(target.searchParams.get("rate_unit")).toBe(body.data.rate_unit);
    }
    expect(reads).toHaveLength(1);
    expect(writes).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath("native-selection.png"),
      fullPage: true,
    });
    // Exercise the visible handoff, not just the href. This remains a read:
    // obtaining a quote or creating a reservation is a separate acceptance.
    await links.first().click();
    await expect(
      page.getByRole("heading", { name: "Preparar solicitud de arriendo" }),
    ).toBeVisible();
    await expect(page.locator("#quote-rate-unit")).toBeDisabled();
    await expect(page.locator("#quote-units")).toBeDisabled();
    await expect(page.locator("#quote-start-at")).toBeDisabled();
    await expect(page.getByText("Intervalo seleccionado:")).toContainText(
      body.data.time_zone,
    );
    const destination = new URL(page.url());
    expect(destination.searchParams.get("start")).toBe(interval.start_at);
    expect(destination.searchParams.get("end")).toBe(interval.end_at);
    expect(writes).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath("native-handoff.png"),
      fullPage: true,
    });
  });
}
