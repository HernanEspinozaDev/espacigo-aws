import { expect, test, type Page } from "@playwright/test";

const reservationId = "20000000-0000-4000-8000-000000000001";
const accountId = "20000000-0000-4000-8000-000000000002";
const pdf = Buffer.from(
  "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Count 0/Kids[]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
);

async function installPrivateReservationFixture(
  page: Page,
  { paid, role }: { paid: boolean; role: "arrendatario" | "anfitrion" },
) {
  const calls: string[] = [];
  let taxAttached = false;
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    calls.push(`${request.method()} ${path}`);
    if (path.endsWith("/auth/session")) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "cache-control": "no-store" },
        body: JSON.stringify({
          account_id: accountId,
          roles: [role],
          capabilities: [],
          restricted_mode: false,
          expires_at: new Date(Date.now() + 3600000).toISOString(),
        }),
      });
      return;
    }
    if (
      path === `/api/backend/commercial-reservations/${reservationId}/documents`
    ) {
      if (!paid) {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          headers: { "cache-control": "no-store" },
          body: JSON.stringify({ error: { code: "payment_required" } }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "cache-control": "no-store" },
        body: JSON.stringify({
          reservation_id: reservationId,
          participant_role: role,
          contract: {
            state: "ready",
            version: 1,
            sha256: "a".repeat(64),
            size_bytes: pdf.byteLength,
            available_at: new Date().toISOString(),
            download_path: `/api/v1/commercial-reservations/${reservationId}/contract-document`,
          },
          receipt: {
            kind: "internal_payment_receipt",
            status: "approved",
            amount_clp: 12000,
            currency: "CLP",
            provider: "mercadopago",
            provider_reference: "fixture-payment-123",
            verified_at: "2026-11-22T15:00:00Z",
          },
          counterpart_contact: {
            role: role === "anfitrion" ? "arrendatario" : "anfitrion",
            display_name: "Contraparte Sintética",
            email: "contraparte@example.test",
            phone: "+56912345678",
          },
          rental_tax_document: {
            state: taxAttached ? "ready" : "not_attached",
            notice: taxAttached
              ? "Documento adjunto por el anfitrión. EspaciGo no emite ni valida documentos tributarios."
              : "EspaciGo no emite ni valida documentos tributarios del arriendo.",
          },
          access_instructions: null,
        }),
      });
      return;
    }
    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/rental-tax-document`
    ) {
      if (!paid) {
        await route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify({ error: { code: "not_found" } }),
        });
        return;
      }
      if (request.method() === "PUT" && role === "anfitrion") {
        expect(request.headers()["content-type"]).toBe("application/pdf");
        expect(request.postDataBuffer()?.subarray(0, 5).toString()).toBe(
          "%PDF-",
        );
        taxAttached = true;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          headers: { "cache-control": "no-store" },
          body: JSON.stringify({
            reservation_id: reservationId,
            state: "ready",
            sha256: "b".repeat(64),
            size_bytes: pdf.byteLength,
            uploaded_at: new Date().toISOString(),
          }),
        });
        return;
      }
      if (request.method() === "GET" && taxAttached) {
        await route.fulfill({
          status: 200,
          contentType: "application/pdf",
          headers: { "cache-control": "no-store" },
          body: pdf,
        });
        return;
      }
      await route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "not_found" } }),
      });
      return;
    }
    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/contract-document`
    ) {
      if (!paid) {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ error: { code: "payment_required" } }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/pdf",
        headers: {
          "cache-control": "no-store",
          "content-disposition": `attachment; filename="contrato-reserva-${reservationId}.pdf"`,
        },
        body: pdf,
      });
      return;
    }
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "not_found" } }),
    });
  });
  return calls;
}

test("host uploads and downloads a scanned private rental tax PDF", async ({
  page,
}) => {
  const calls = await installPrivateReservationFixture(page, {
    paid: true,
    role: "anfitrion",
  });
  await page.goto(`/anfitrion/reservas/${reservationId}`);
  await page
    .getByLabel("Adjuntar boleta o factura del arriendo (PDF)")
    .setInputFiles({
      name: "boleta-sintetica.pdf",
      mimeType: "application/pdf",
      buffer: pdf,
    });
  await page.getByRole("button", { name: "Adjuntar PDF" }).click();
  await expect(
    page.getByRole("button", { name: "Descargar PDF tributario" }),
  ).toBeVisible();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Descargar PDF tributario" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(
    `documento-tributario-arriendo-${reservationId}.pdf`,
  );
  expect(calls).toContain(
    `PUT /api/backend/commercial-reservations/${reservationId}/rental-tax-document`,
  );
  expect(calls).toContain(
    `GET /api/backend/commercial-reservations/${reservationId}/rental-tax-document`,
  );
});

for (const participant of [
  { role: "arrendatario" as const, path: `/reservas/${reservationId}` },
  {
    role: "anfitrion" as const,
    path: `/anfitrion/reservas/${reservationId}`,
  },
]) {
  test(`${participant.role} sees paid reservation receipt and downloads its private PDF`, async ({
    page,
  }) => {
    const calls = await installPrivateReservationFixture(page, {
      paid: true,
      role: participant.role,
    });
    await page.goto(participant.path);
    await expect(
      page.getByRole("heading", { name: "Tu reserva pagada" }),
    ).toBeVisible();
    await expect(page.getByText("Contraparte Sintética")).toBeVisible();
    await expect(page.getByText("$12.000 CLP")).toBeVisible();
    await expect(page.getByText("fixture-payment-123")).toBeVisible();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: "Descargar contrato PDF" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe(
      `contrato-reserva-${reservationId}.pdf`,
    );
    expect(calls).toContain(
      `GET /api/backend/commercial-reservations/${reservationId}/contract-document`,
    );
  });

  test(`${participant.role} does not disclose documents for an unpaid reservation`, async ({
    page,
  }) => {
    const calls = await installPrivateReservationFixture(page, {
      paid: false,
      role: participant.role,
    });
    await page.goto(participant.path);
    await expect(
      page.getByRole("heading", { name: "Tu reserva pagada" }),
    ).toHaveCount(0);
    await expect(page.getByText("Contraparte Sintética")).toHaveCount(0);
    await expect(page.getByText("fixture-payment-123")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Descargar contrato PDF" }),
    ).toHaveCount(0);
    expect(calls).toContain(
      `GET /api/backend/commercial-reservations/${reservationId}/documents`,
    );
    expect(calls).not.toContain(
      `GET /api/backend/commercial-reservations/${reservationId}/contract-document`,
    );
  });
}
