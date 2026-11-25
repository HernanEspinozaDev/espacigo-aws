import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";

const reservationId = "30000000-0000-4000-8000-000000000001";
const accountId = "30000000-0000-4000-8000-000000000002";
const caseId = "30000000-0000-4000-8000-000000000003";
const messageId = "30000000-0000-4000-8000-000000000004";
const reviewId = "30000000-0000-4000-8000-000000000005";
const policyText = "Cancelación fácil: devolución total antes del inicio.";
const startsAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString();
const endsAt = new Date(Date.now() + 8 * 24 * 60 * 60 * 1000).toISOString();

type Participant = {
  role: "arrendatario" | "anfitrion";
  path: string;
};

const participants: Participant[] = [
  { role: "arrendatario", path: `/reservas/${reservationId}` },
  { role: "anfitrion", path: `/anfitrion/reservas/${reservationId}` },
];

function json(data: unknown) {
  return {
    status: 200,
    contentType: "application/json",
    headers: { "cache-control": "no-store" },
    body: JSON.stringify({ data }),
  };
}

async function installAftercareFixture(
  page: Page,
  {
    role,
    reviewEligible,
    invoiceFixture,
  }: {
    role: Participant["role"];
    reviewEligible: boolean;
    invoiceFixture?: boolean;
  },
) {
  const calls: string[] = [];
  let reservationStatus = "pagada";
  let cancellationPosted = false;
  let messages: Array<Record<string, unknown>> = [
    {
      id: "30000000-0000-4000-8000-000000000006",
      reservation_id: reservationId,
      author_id: "30000000-0000-4000-8000-000000000007",
      sequence: 1,
      body: "Confirmo el horario de la reserva.",
      created_at: new Date().toISOString(),
    },
  ];
  let supportCases: Array<Record<string, unknown>> = [];

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
          expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        }),
      });
      return;
    }

    if (path === `/api/backend/commercial-reservations/${reservationId}`) {
      await route.fulfill(
        json({
          id: reservationId,
          quote_id: "30000000-0000-4000-8000-000000000010",
          space_id: "30000000-0000-4000-8000-000000000011",
          space_title: "Sala de ensayo sintética",
          participant_role: role,
          status: reservationStatus,
          cancellation_policy_version: "facil_v1",
          cancellation_policy_text: policyText,
          checkout_status: "approved",
          checkout_method: "checkout_pro",
          provider_payment_id: "synthetic-payment-reference",
          unit_price_clp: 12000,
          units: 1,
          subtotal_clp: 12000,
          total_charged_clp: 12000,
          currency: "CLP",
          rate_unit: "hora",
          start_at: startsAt,
          end_at: endsAt,
          time_zone: "America/Santiago",
          payment_expires_at: startsAt,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          ...(invoiceFixture
            ? {
                unit_price_clp: 1000,
                subtotal_clp: 1000,
                commission_clp: 30,
                vat_clp: 6,
                buyer_commission_clp: 30,
                buyer_iva_clp: 6,
                buyer_fee_clp: 36,
                marketplace_fee_clp: 72,
                net_host_amount_clp: 964,
                total_charged_clp: 1036,
                provider_settlement: {
                  net_clp: 925,
                  marketplace_fee_clp: 72,
                  processing_fee_clp: 39,
                  release_status: "released",
                  release_at: startsAt,
                  provider_updated_at: startsAt,
                  reconciliation: "reconciled",
                },
              }
            : {}),
        }),
      );
      return;
    }

    if (
      path === `/api/backend/commercial-reservations/${reservationId}/documents`
    ) {
      await route.fulfill(
        json({
          reservation_id: reservationId,
          participant_role: role,
          contract: { state: "unavailable" },
          receipt: {
            kind: "internal_payment_receipt",
            status: "approved",
            amount_clp: 12000,
            currency: "CLP",
            provider: "mercadopago",
            provider_reference: "synthetic-payment-reference",
            verified_at: new Date().toISOString(),
          },
          counterpart_contact: null,
          rental_tax_document: {
            state: "not_attached",
            notice: "Fixture local de posventa.",
          },
          access_instructions: null,
        }),
      );
      return;
    }

    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/cancellation-preview`
    ) {
      await route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: !cancellationPosted,
          reason: "La reserva comienza en el futuro.",
          cancellation_policy_version: "facil_v1",
          cancellation_policy_text: policyText,
          refund_percent: 100,
          total_charged_clp: 12000,
          refund_amount_clp: 12000,
          currency: "CLP",
          starts_at: startsAt,
        }),
      );
      return;
    }

    if (
      path === `/api/backend/commercial-reservations/${reservationId}/cancel` &&
      request.method() === "POST"
    ) {
      expect(request.postDataJSON()).toEqual({
        reason: "Cambio de planes",
        expected_refund_amount_clp: 12000,
      });
      cancellationPosted = true;
      reservationStatus = "reembolsada";
      await route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: false,
          reason: "La reserva ya fue cancelada.",
          cancellation_policy_version: "facil_v1",
          cancellation_policy_text: policyText,
          refund_percent: 100,
          total_charged_clp: 12000,
          refund_amount_clp: 12000,
          currency: "CLP",
          starts_at: startsAt,
          status: "reembolsada",
          refund_status: "aprobado",
        }),
      );
      return;
    }

    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/post-sale-summary`
    ) {
      await route.fulfill(
        json({
          unread_messages: messages.length,
          reviews: reviewEligible ? 0 : 1,
          open_cases: supportCases.length,
        }),
      );
      return;
    }

    if (
      path === `/api/backend/commercial-reservations/${reservationId}/messages`
    ) {
      if (request.method() === "POST") {
        const body = request.postDataJSON() as { body: string };
        const item = {
          id: messageId,
          reservation_id: reservationId,
          author_id: accountId,
          sequence: messages.length + 1,
          body: body.body,
          created_at: new Date().toISOString(),
        };
        messages = [item];
        await route.fulfill(json(item));
      } else {
        await route.fulfill(json({ items: messages, older_cursor: null }));
      }
      return;
    }

    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/messages/read`
    ) {
      await route.fulfill(json({ read_through_sequence: 1 }));
      return;
    }

    if (
      path === `/api/backend/commercial-reservations/${reservationId}/reviews`
    ) {
      if (request.method() === "POST") {
        const item = {
          id: reviewId,
          target_type: role === "anfitrion" ? "arrendatario" : "espacio",
          rating: 5,
          comment: "Experiencia sintética positiva.",
          created_at: new Date().toISOString(),
        };
        await route.fulfill(json(item));
      } else {
        await route.fulfill(
          json({
            items: [],
            can_review: reviewEligible,
            review_unavailable_reason: reviewEligible
              ? undefined
              : "reservation_not_ended",
            reservation_ends_at: reviewEligible
              ? new Date(Date.now() - 60_000).toISOString()
              : endsAt,
          }),
        );
      }
      return;
    }

    if (
      path ===
      `/api/backend/commercial-reservations/${reservationId}/support-cases`
    ) {
      if (request.method() === "POST") {
        const body = request.postDataJSON() as {
          category: string;
          description: string;
        };
        const routedTo =
          body.category === "mediacion_directa" ? "espacigo" : "contraparte";
        const status =
          routedTo === "espacigo"
            ? "en_mediacion_espacigo"
            : "esperando_contraparte";
        const item = {
          id: caseId,
          reservation_id: reservationId,
          initiator_role: role,
          category: body.category,
          routed_to: routedTo,
          status,
          description: body.description,
          actions: [],
          events: [
            {
              action: "abrir",
              description: body.description,
              created_at: new Date().toISOString(),
            },
          ],
          evidence: [],
        };
        supportCases = [item];
        await route.fulfill(json(item));
      } else {
        await route.fulfill(
          json({ items: supportCases, can_claim_damage: false }),
        );
      }
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

for (const participant of participants) {
  test(`${participant.role} can cancel, message, see review eligibility and route a support case`, async ({
    page,
  }) => {
    const calls = await installAftercareFixture(page, {
      role: participant.role,
      reviewEligible: false,
    });
    await page.goto(participant.path);

    await expect(
      page.getByRole("heading", { name: "Política de cancelación" }),
    ).toBeVisible();
    await expect(page.getByText(policyText)).toBeVisible();
    await page.getByRole("button", { name: "Ver monto de devolución" }).click();
    await expect(page.getByText("Devolución estimada")).toBeVisible();
    await expect(
      page
        .getByText("Devolución estimada")
        .locator("..")
        .getByText("$12.000 CLP"),
    ).toBeVisible();
    await page.getByLabel("Motivo de la cancelación").fill("Cambio de planes");
    const confirm = page.getByLabel(
      "Confirmo que quiero cancelar esta reserva y solicitar la devolución indicada.",
    );
    await expect(
      page.getByRole("button", { name: "Confirmar cancelación" }),
    ).toBeDisabled();
    await confirm.check();
    await page.getByRole("button", { name: "Confirmar cancelación" }).click();
    await expect(
      page.getByText(
        "La cancelación y el reembolso se registraron correctamente.",
      ),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Mensajería privada" }).click();
    await expect(
      page.getByText("Confirmo el horario de la reserva."),
    ).toBeVisible();
    await expect(
      page.getByText(
        participant.role === "anfitrion" ? "Arrendatario" : "Anfitrión",
        { exact: true },
      ),
    ).toBeVisible();
    await page
      .getByLabel("Mensaje para la contraparte")
      .fill("Hola, confirmo el horario.");
    await page.getByRole("button", { name: "Enviar mensaje" }).click();
    await expect(page.getByText("Hola, confirmo el horario.")).toBeVisible();
    await expect(page.getByText("Tú", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "Reseñas" }).click();
    await expect(
      page.getByText(/Podrás dejar tu reseña cuando finalice el horario/),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Disputas y reclamos" }).click();
    const caseCategory =
      participant.role === "anfitrion"
        ? "mediacion_directa"
        : "problemas_acceso";
    await page.getByLabel("Motivo del caso").selectOption(caseCategory);
    await page
      .getByLabel("Describe lo ocurrido")
      .fill("Necesito apoyo directo de EspaciGo.");
    await page.getByRole("button", { name: "Abrir caso" }).click();
    await expect(
      page.getByText(
        participant.role === "anfitrion"
          ? "En mediación de EspaciGo"
          : "En conversación con la contraparte",
      ),
    ).toBeVisible();
    expect(calls).toContain(
      `POST /api/backend/commercial-reservations/${reservationId}/cancel`,
    );
    expect(calls).toContain(
      `POST /api/backend/commercial-reservations/${reservationId}/messages`,
    );
    expect(calls).toContain(
      `GET /api/backend/commercial-reservations/${reservationId}/reviews`,
    );
    expect(calls).toContain(
      `POST /api/backend/commercial-reservations/${reservationId}/support-cases`,
    );
    expect(calls.some((call) => call.includes("/local/booking-trial/"))).toBe(
      false,
    );
  });

  test(`${participant.role} gets a role-specific review form after the reservation ends`, async ({
    page,
  }) => {
    await installAftercareFixture(page, {
      role: participant.role,
      reviewEligible: true,
    });
    await page.goto(participant.path);
    await page.getByRole("tab", { name: "Reseñas" }).click();
    const reviewLabel =
      participant.role === "anfitrion"
        ? "Calificar al arrendatario"
        : "Calificar el espacio";
    await expect(page.getByLabel(reviewLabel)).toBeVisible();
    await page
      .getByLabel("Comentario")
      .fill("Reserva finalizada correctamente.");
    await page.getByRole("button", { name: "Publicar reseña" }).click();
    await expect(
      page.getByText("Experiencia sintética positiva."),
    ).toBeVisible();
  });
}

test("host invoice discloses both EspaciGo charges and the net read from Mercado Pago", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
    invoiceFixture: true,
  });
  await page.goto(`/anfitrion/reservas/${reservationId}`);
  await page.getByRole("button", { name: "Ver Invoice Anfitrión" }).click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByText("Cargo EspaciGo al arrendatario (3% + IVA)"),
  ).toBeVisible();
  await expect(
    dialog.getByText("Cargo EspaciGo al anfitrión (3% + IVA)"),
  ).toBeVisible();
  await expect(
    dialog.getByText(/72 CLP en total.*36 CLP arrendatario.*36 CLP anfitrión/),
  ).toBeVisible();
  await expect(dialog.getByText("−$39 CLP", { exact: true })).toBeVisible();
  await expect(dialog.getByText("$925 CLP", { exact: true })).toBeVisible();
  await expect(
    dialog.getByText(/Fondos liberados en Mercado Pago/),
  ).toBeVisible();
  await expect(dialog.getByText("$964 CLP", { exact: true })).toHaveCount(0);
  const downloadReady = page.waitForEvent("download");
  await dialog.getByRole("button", { name: "Descargar (.txt)" }).click();
  const download = await downloadReady;
  const receipt = await readFile(await download.path(), "utf8");
  expect(receipt).toContain("Neto informado por Mercado Pago: $925 CLP");
  expect(receipt).toContain(
    "Comisión de procesamiento de Mercado Pago: −$39 CLP",
  );
  expect(receipt).toContain("$36 CLP arrendatario + $36 CLP anfitrión");
});

test("host finance calculator estimates the same amounts by fee or percentage", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
  });
  await page.route(/\/api\/backend\/commercial-reservations(?:\?|$)/, (route) =>
    route.fulfill(
      json({
        items: [
          {
            id: reservationId,
            quote_id: caseId,
            space_id: messageId,
            space_title: "Sala sintética",
            participant_role: "anfitrion",
            status: "pagada",
            checkout_status: "approved",
            checkout_method: "checkout_pro",
            rate_unit: "hora",
            unit_price_clp: 1000,
            units: 1,
            subtotal_clp: 1000,
            currency: "CLP",
            start_at: startsAt,
            end_at: endsAt,
            time_zone: "America/Santiago",
            payment_expires_at: startsAt,
            created_at: startsAt,
            updated_at: startsAt,
            provider_settlement: {
              net_clp: 925,
              processing_fee_clp: 39,
              marketplace_fee_clp: 72,
              payment_type: "debit_card",
              payment_status: "approved",
              approved_at: startsAt,
              synced_at: startsAt,
              provider_updated_at: startsAt,
              reconciliation: "reconciled",
            },
          },
        ],
      }),
    ),
  );
  await page.goto("/anfitrion/reservas");
  await page.getByRole("tab", { name: "Finanzas" }).click();
  const netSummary = page
    .getByText("Neto informado para el anfitrión")
    .locator("..");
  await expect(netSummary.getByText("$925 CLP", { exact: true })).toBeVisible();

  const calculator = page
    .locator("details")
    .filter({ hasText: "Calculadora de comisiones" });
  await calculator
    .getByText("Calculadora de comisiones", { exact: true })
    .click();
  await expect(
    calculator.getByText(/Cargo confirmado.*39 CLP.*Débito/),
  ).toBeVisible();
  await calculator
    .getByRole("button", { name: "Usar como referencia" })
    .click();
  await expect(calculator.getByLabel("Valor del arriendo en CLP")).toHaveValue(
    "1000",
  );
  await expect(
    calculator.getByLabel("Costo de Mercado Pago en CLP"),
  ).toHaveValue("39");
  await calculator.getByLabel("Valor del arriendo en CLP").fill("1000");
  await calculator.getByLabel("Costo de Mercado Pago en CLP").fill("39");
  await expect(
    calculator.getByText("$1.036 CLP", { exact: true }),
  ).toBeVisible();
  await expect(calculator.getByText("$925 CLP", { exact: true })).toBeVisible();
  await calculator
    .getByLabel("Forma de ingresar el costo de Mercado Pago")
    .selectOption("rate");
  await calculator.getByLabel("Tasa de Mercado Pago (%)").fill("3,76");
  await expect(calculator.getByText("−$39 CLP", { exact: true })).toBeVisible();
  await expect(calculator.getByText("$925 CLP", { exact: true })).toBeVisible();
});
