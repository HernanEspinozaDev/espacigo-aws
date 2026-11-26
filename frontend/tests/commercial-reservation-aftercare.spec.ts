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
    pendingPaymentResults,
    externalRefund,
  }: {
    role: Participant["role"];
    reviewEligible: boolean;
    invoiceFixture?: boolean;
    pendingPaymentResults?: Array<"approved" | "cancelled" | "pending">;
    externalRefund?: boolean;
  },
) {
  const calls: string[] = [];
  let reservationStatus = pendingPaymentResults
    ? "pendiente_de_pago"
    : externalRefund
      ? "reconciliation_required"
      : "pagada";
  let cancellationPosted = false;
  let pendingCancellation = false;
  let externalRefundApproved = false;
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
          checkout_status:
            reservationStatus === "pendiente_de_pago"
              ? "pending"
              : reservationStatus === "cancelada_por_pago"
                ? "cancelled"
                : "approved",
          pending_payment_cancellation: pendingCancellation,
          refund_status: externalRefundApproved ? "aprobado" : undefined,
          checkout_method: "checkout_pro",
          provider_payment_id: pendingPaymentResults
            ? undefined
            : "synthetic-payment-reference",
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
      path ===
        `/api/backend/commercial-reservations/${reservationId}/cancel-pending-payment` &&
      request.method() === "POST"
    ) {
      expect(request.postData()).toBeNull();
      const status = pendingPaymentResults?.shift();
      expect(status).toBeDefined();
      pendingCancellation = status === "pending";
      if (status === "cancelled") reservationStatus = "cancelada_por_pago";
      if (status === "approved") reservationStatus = "pagada";
      await route.fulfill({
        ...json({ payment_status: status }),
        status: pendingCancellation ? 202 : 200,
      });
      return;
    }

    if (
      path ===
        `/api/backend/commercial-reservations/${reservationId}/reconcile-refund` &&
      request.method() === "POST"
    ) {
      expect(request.postDataJSON()).toEqual({ refund_id: "9007199254740993" });
      expect(role).toBe("anfitrion");
      expect(externalRefund).toBe(true);
      externalRefundApproved = true;
      reservationStatus = "reembolsada";
      await route.fulfill(
        json({
          reservation_id: reservationId,
          status: "reembolsada",
          refund_status: "aprobado",
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

test("an unlinked Mercado Pago refund directs the participant to support", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "arrendatario",
    reviewEligible: false,
  });
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancel`,
    (route) =>
      route.fulfill({
        status: 409,
        json: { error: { code: "refund_requires_reconciliation" } },
      }),
  );
  await page.goto(`/reservas/${reservationId}`);
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await page.getByLabel("Motivo de la cancelación").fill("Cambio de planes");
  await page
    .getByLabel(
      "Confirmo que quiero cancelar esta reserva y solicitar la devolución indicada.",
    )
    .check();
  await page.getByRole("button", { name: "Confirmar cancelación" }).click();
  await expect(
    page.getByText(
      "Mercado Pago informa una devolución que requiere revisión. Contacta a soporte desde esta reserva antes de reintentar.",
    ),
  ).toBeVisible();
  await expect(
    page.getByText(
      "La cancelación y el reembolso se registraron correctamente.",
    ),
  ).toHaveCount(0);
});

test("insufficient funds 409 shows dedicated guidance and keeps reservation active", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
  });
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancellation-preview`,
    (route) =>
      route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: true,
          reason: "host_cancellation",
          cancellation_policy_version: "facil_v1",
          cancellation_policy_text: policyText,
          refund_percent: 100,
          total_charged_clp: 12000,
          refund_amount_clp: 10000,
          refunded_amount_clp: 2000,
          refundable_balance_clp: 10000,
          currency: "CLP",
          starts_at: startsAt,
        }),
      ),
  );
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancel`,
    (route) =>
      route.fulfill({
        status: 409,
        json: { error: { code: "refund_insufficient_funds" } },
      }),
  );
  await page.goto(`/anfitrion/reservas/${reservationId}`);
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await expect(page.getByText("Ya reembolsado")).toBeVisible();
  await expect(
    page.getByText("Ya reembolsado").locator("..").getByText("$2.000 CLP"),
  ).toBeVisible();
  await expect(page.getByText("Saldo reembolsable")).toBeVisible();
  await expect(
    page.getByText("Saldo reembolsable").locator("..").getByText("$10.000 CLP"),
  ).toBeVisible();
  await expect(
    page
      .getByText("Devolución estimada")
      .locator("..")
      .getByText("$10.000 CLP"),
  ).toBeVisible();
  await page.getByLabel("Motivo de la cancelación").fill("Falta de saldo");
  await page
    .getByLabel(
      "Confirmo que quiero cancelar esta reserva y solicitar la devolución indicada.",
    )
    .check();
  await page.getByRole("button", { name: "Confirmar cancelación" }).click();
  await expect(
    page.getByText(
      "Mercado Pago confirmó falta de saldo para realizar el reembolso. El anfitrión debe reponer fondos y reintentar desde la reserva.",
    ),
  ).toBeVisible();
  await expect(
    page.getByText("Pago Aprobado · Reserva Guardada"),
  ).toBeVisible();
  await expect(
    page.getByText(
      "La cancelación y el reembolso se registraron correctamente.",
    ),
  ).toHaveCount(0);
});

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
            provider_payment_id: "123456789",
            total_charged_clp: 1036,
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
          {
            id: messageId,
            quote_id: caseId,
            space_id: messageId,
            space_title: "Sala sin conciliar",
            participant_role: "anfitrion",
            status: "pagada",
            checkout_status: "approved",
            checkout_method: "checkout_pro",
            provider_payment_id: "123456790",
            total_charged_clp: 5179,
            rate_unit: "hora",
            unit_price_clp: 5000,
            units: 1,
            subtotal_clp: 5000,
            currency: "CLP",
            start_at: startsAt,
            end_at: endsAt,
            time_zone: "America/Santiago",
            payment_expires_at: startsAt,
            created_at: startsAt,
            updated_at: startsAt,
          },
        ],
      }),
    ),
  );
  await page.goto("/anfitrion/reservas");
  await page.getByRole("tab", { name: "Finanzas" }).click();
  const providerSummary = page
    .getByText("Comisión Mercado Pago", { exact: true })
    .filter({ visible: true })
    .first()
    .locator("..");
  await expect(
    providerSummary.getByText("$39 CLP", { exact: true }),
  ).toBeVisible();
  await expect(
    providerSummary.getByText(/3,76\s*%.*1 de 2 pagos/),
  ).toBeVisible();
  const payment = page.getByRole("row").filter({ hasText: "#123456789" });
  await expect(payment.getByText(/3,76\s*%/)).toBeVisible();
  await expect(
    page
      .getByRole("row")
      .filter({ hasText: "#123456790" })
      .getByText("Sin datos"),
  ).toBeVisible();

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

test("el arrendatario consulta la cancelación pendiente después de recargar", async ({
  page,
}) => {
  const calls = await installAftercareFixture(page, {
    role: "arrendatario",
    reviewEligible: false,
    pendingPaymentResults: ["pending", "cancelled"],
  });
  await page.goto(participants[0].path);
  const checkout = page.getByRole("link", {
    name: "Ir al Checkout de Mercado Pago",
  });
  await expect(checkout).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Ver monto de devolución" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Cancelar pago pendiente" }).click();
  await page.getByRole("button", { name: "Volver", exact: true }).click();
  expect(calls).not.toContain(
    `POST /api/backend/commercial-reservations/${reservationId}/cancel-pending-payment`,
  );
  await page.getByRole("button", { name: "Cancelar pago pendiente" }).click();
  await page
    .getByRole("button", { name: "Confirmar cancelación del pago" })
    .click();
  await expect(
    page.getByText(
      "Solicitud registrada. Mercado Pago aún está confirmando la cancelación.",
    ),
  ).toBeVisible();
  await expect(checkout).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByText(
      "Solicitud registrada. Mercado Pago aún está confirmando la cancelación.",
    ),
  ).toBeVisible();
  await expect(checkout).toHaveCount(0);
  await page
    .getByRole("button", { name: "Consultar estado de la cancelación" })
    .click();
  await expect(
    page.getByText("Mercado Pago confirmó la cancelación del pago."),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Ver monto de devolución" }),
  ).toHaveCount(0);
  expect(calls).not.toContain(
    `POST /api/backend/commercial-reservations/${reservationId}/cancel`,
  );
});

test("el anfitrión distingue un pago aprobado durante la cancelación", async ({
  page,
}) => {
  const calls = await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
    pendingPaymentResults: ["approved"],
  });
  await page.goto(participants[1].path);
  await page.getByRole("button", { name: "Cancelar pago pendiente" }).click();
  await page
    .getByRole("button", { name: "Confirmar cancelación del pago" })
    .click();
  await expect(
    page.getByText(/Mercado Pago ya aprobó este pago/),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Ver monto de devolución" }),
  ).toBeVisible();
  expect(calls).not.toContain(
    `POST /api/backend/commercial-reservations/${reservationId}/cancel`,
  );
});

test("el anfitrión vincula un reembolso externo con el identificador exacto", async ({
  page,
}) => {
  const calls = await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
    externalRefund: true,
  });
  await page.goto(participants[1].path);
  await page.getByText("Vincular un reembolso ya realizado").click();
  const id = page.getByLabel("ID del reembolso");
  const submit = page.getByRole("button", { name: "Verificar y vincular" });
  await id.fill("900a");
  await expect(submit).toBeDisabled();
  await id.fill("9007199254740993");
  await submit.click();
  await expect(
    page.getByText(
      "Mercado Pago confirmó el reembolso y quedó vinculado a la reserva.",
    ),
  ).toBeVisible();
  expect(calls).not.toContain(
    `POST /api/backend/commercial-reservations/${reservationId}/cancel`,
  );
});

test("el arrendatario no puede vincular un reembolso externo", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "arrendatario",
    reviewEligible: false,
    externalRefund: true,
  });
  await page.goto(participants[0].path);
  await expect(
    page.getByRole("heading", { name: "Política de cancelación" }),
  ).toBeVisible();
  await expect(
    page.getByText("Vincular un reembolso ya realizado"),
  ).toHaveCount(0);
});

test("finanzas isolates chargebacks and confirmed refunds without showing unreconciled net", async ({
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
            space_title: "Sala con reembolso",
            participant_role: "anfitrion",
            status: "reembolsada",
            checkout_status: "approved",
            refund_status: "aprobado",
            checkout_method: "checkout_pro",
            provider_payment_id: "555001",
            commission_clp: 30,
            vat_clp: 6,
            buyer_commission_clp: 30,
            buyer_iva_clp: 6,
            buyer_fee_clp: 36,
            marketplace_fee_clp: 72,
            net_host_amount_clp: 964,
            total_charged_clp: 1036,
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
              refunded_clp: 1036,
              payment_type: "debit_card",
              payment_status: "refunded",
              approved_at: startsAt,
              synced_at: startsAt,
              provider_updated_at: startsAt,
              reconciliation: "discrepancy",
            },
          },
          {
            id: messageId,
            quote_id: caseId,
            space_id: messageId,
            space_title: "Sala en contracargo",
            participant_role: "anfitrion",
            status: "pagada",
            checkout_status: "approved",
            checkout_method: "checkout_pro",
            provider_payment_id: "555002",
            commission_clp: 60,
            vat_clp: 11,
            buyer_commission_clp: 60,
            buyer_iva_clp: 11,
            buyer_fee_clp: 71,
            marketplace_fee_clp: 142,
            net_host_amount_clp: 1929,
            total_charged_clp: 2071,
            rate_unit: "hora",
            unit_price_clp: 2000,
            units: 1,
            subtotal_clp: 2000,
            currency: "CLP",
            start_at: startsAt,
            end_at: endsAt,
            time_zone: "America/Santiago",
            payment_expires_at: startsAt,
            created_at: startsAt,
            updated_at: startsAt,
            provider_settlement: {
              net_clp: 1850,
              processing_fee_clp: 79,
              marketplace_fee_clp: 142,
              payment_type: "credit_card",
              payment_status: "charged_back",
              approved_at: startsAt,
              synced_at: startsAt,
              provider_updated_at: startsAt,
              reconciliation: "discrepancy",
            },
          },
        ],
      }),
    ),
  );
  await page.goto("/anfitrion/reservas");
  await page.getByRole("tab", { name: "Finanzas" }).click();

  await expect(
    page.getByText("Contracargo en curso (1 operación)"),
  ).toBeVisible();
  const refundSummary = page
    .getByText("Reembolsos confirmados", { exact: true })
    .locator("..");
  await expect(refundSummary.getByText("$1.036 CLP")).toBeVisible();

  const netSummary = page
    .getByText("Neto informado para el anfitrión")
    .locator("..");
  await expect(netSummary.getByText("Por confirmar")).toBeVisible();

  const refundedRow = page.getByRole("row").filter({ hasText: "#555001" });
  await expect(refundedRow.getByText("Split $72")).toBeVisible();
  await expect(refundedRow.getByText("$1.036", { exact: true })).toHaveCount(2);
  await expect(refundedRow.getByText("Reembolso confirmado")).toBeVisible();
  await expect(refundedRow.getByText("Pendiente de conciliar")).toBeVisible();
  await expect(refundedRow.getByText("$925")).toHaveCount(0);

  const chargebackRow = page.getByRole("row").filter({ hasText: "#555002" });
  await expect(
    chargebackRow.getByText("Contracargo", { exact: true }),
  ).toBeVisible();
  await expect(chargebackRow.getByText("Pendiente de conciliar")).toBeVisible();
  await expect(chargebackRow.getByText("$1.850")).toHaveCount(0);
});

test("host can issue a post-start refund with Reembolsar igualmente", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
  });
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancellation-preview`,
    (route) =>
      route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: true,
          reason: "host_post_start_refund",
          cancellation_policy_version: "flexible_v1",
          cancellation_policy_text: policyText,
          refund_percent: 100,
          total_charged_clp: 12000,
          refunded_amount_clp: 0,
          refundable_balance_clp: 12000,
          refund_amount_clp: 12000,
          currency: "CLP",
          starts_at: startsAt,
          refund_status: "fallido",
        }),
      ),
  );
  await page.goto(`/anfitrion/reservas/${reservationId}`);
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await expect(
    page.getByText(
      "La hora de inicio de esta reserva ya pasó. Como anfitrión, puedes reembolsar igualmente dentro del plazo de 180 días de Mercado Pago.",
    ),
  ).toBeVisible();
  await page.getByLabel("Motivo de la cancelación").fill("Cambio de planes");
  await page
    .getByLabel(
      "Confirmo que, aunque la hora de inicio ya pasó, quiero reembolsar esta reserva igualmente.",
    )
    .check();
  await page.getByRole("button", { name: "Reembolsar igualmente" }).click();
  await expect(
    page.getByText(
      "La cancelación y el reembolso se registraron correctamente.",
    ),
  ).toBeVisible();
});

test("host sees inline error inside cancellation section when refund fails with 503", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "anfitrion",
    reviewEligible: false,
  });
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancellation-preview`,
    (route) =>
      route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: true,
          reason: "host_post_start_refund",
          cancellation_policy_version: "flexible_v1",
          cancellation_policy_text: policyText,
          refund_percent: 100,
          total_charged_clp: 1036,
          refunded_amount_clp: 0,
          refundable_balance_clp: 1036,
          refund_amount_clp: 1036,
          currency: "CLP",
          starts_at: startsAt,
          refund_status: "fallido",
        }),
      ),
  );
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancel`,
    (route) =>
      route.fulfill({
        status: 503,
        json: { error: { code: "service_unavailable" } },
      }),
  );
  await page.goto(`/anfitrion/reservas/${reservationId}`);
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await page.getByLabel("Motivo de la cancelación").fill("pago de prueba");
  await page
    .getByLabel(
      "Confirmo que, aunque la hora de inicio ya pasó, quiero reembolsar esta reserva igualmente.",
    )
    .check();
  await page.getByRole("button", { name: "Reembolsar igualmente" }).click();
  await expect(page.getByTestId("commercial-cancellation-error")).toBeVisible();
  await expect(
    page
      .getByTestId("commercial-cancellation-error")
      .getByText(/service_unavailable/),
  ).toBeVisible();
});

test("cancellation preview renders color-coded policy badge, partial retained amount, and zero-refund notice", async ({
  page,
}) => {
  await installAftercareFixture(page, {
    role: "arrendatario",
    reviewEligible: false,
  });
  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancellation-preview`,
    (route) =>
      route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: true,
          reason: "eligible",
          cancellation_policy_version: "moderada_v1",
          cancellation_policy_text:
            "Moderada: reembolso del 50 % hasta 48 horas antes del inicio del arriendo.",
          refund_percent: 50,
          total_charged_clp: 12000,
          refunded_amount_clp: 0,
          refundable_balance_clp: 12000,
          refund_amount_clp: 6000,
          currency: "CLP",
          starts_at: startsAt,
        }),
      ),
  );
  await page.goto(`/reservas/${reservationId}`);
  await expect(page.getByText("Incluida en el contrato")).toBeVisible();
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await expect(page.getByText("Monto retenido por política")).toBeVisible();
  await expect(
    page
      .getByText("Monto retenido por política")
      .locator("..")
      .getByText("$6.000 CLP"),
  ).toBeVisible();
  await expect(
    page.getByText(/Reembolso parcial del 50% según el contrato firmado/),
  ).toBeVisible();

  await page.route(
    `**/api/backend/commercial-reservations/${reservationId}/cancellation-preview`,
    (route) =>
      route.fulfill(
        json({
          reservation_id: reservationId,
          can_cancel: true,
          reason: "eligible_no_refund",
          cancellation_policy_version: "estricta_v1",
          cancellation_policy_text:
            "Estricta: sin reembolso después de confirmar la reserva.",
          refund_percent: 0,
          total_charged_clp: 12000,
          refunded_amount_clp: 0,
          refundable_balance_clp: 12000,
          refund_amount_clp: 0,
          currency: "CLP",
          starts_at: startsAt,
        }),
      ),
  );
  await page.getByRole("button", { name: "Ver monto de devolución" }).click();
  await expect(
    page.getByText(
      /Cancelación sin reembolso según la política del contrato firmado/,
    ),
  ).toBeVisible();
});
