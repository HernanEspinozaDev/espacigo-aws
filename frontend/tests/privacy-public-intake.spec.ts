import { expect, test, type Page, type Route } from "@playwright/test";

test.use({ trace: "off", screenshot: "off", video: "off" });

const requestsOutsideLoopback = new WeakMap<Page, boolean>();

test.beforeEach(async ({ page }) => {
  requestsOutsideLoopback.set(page, false);
  page.on("request", (request) => {
    const url = request.url();
    if (!url.startsWith("http://127.0.0.1:")) {
      requestsOutsideLoopback.set(page, true);
    }
  });
});

test.afterEach(async ({ page }) => {
  expect(requestsOutsideLoopback.get(page)).toBe(false);
});

async function mockAnonymousSession(page: Page): Promise<void> {
  await page.route("**/api/backend/auth/session", async (route: Route) => {
    await route.fulfill({
      status: 401,
      headers: {
        "cache-control": "no-store",
        "content-type": "application/json; charset=utf-8",
      },
      body: JSON.stringify({
        error: {
          code: "unauthenticated",
          message: "Sin sesión activa.",
        },
      }),
    });
  });
}

test("@controlled /privacidad/solicitudes: ingresa bloqueo como representante legal, muestra acuse único y consulta estado y línea de tiempo sin datos internos", async ({
  page,
}) => {
  await mockAnonymousSession(page);

  let capturedSubmitBody: Record<string, unknown> | null = null;
  let capturedSubmitAuthHeader: string | undefined;
  let capturedLookupBody: Record<string, unknown> | null = null;
  let capturedLookupAuthHeader: string | undefined;

  const expectedFolio = "PRIV-2026-AB12CD34";
  const expectedTrackingCode = "trk_unico_2026_9876543210abcdef";

  await page.route(
    "**/api/backend/privacy/public-requests",
    async (route: Route) => {
      const request = route.request();
      expect(request.method()).toBe("POST");
      capturedSubmitAuthHeader = request.headers().authorization;
      capturedSubmitBody = JSON.parse(request.postData() ?? "{}") as Record<
        string,
        unknown
      >;

      await route.fulfill({
        status: 201,
        headers: {
          "cache-control": "no-store",
          "content-type": "application/json; charset=utf-8",
        },
        body: JSON.stringify({
          data: {
            id: "660e8400-e29b-41d4-a716-446655440010",
            folio: expectedFolio,
            tracking_code: expectedTrackingCode,
            right_type: "bloqueo",
            requester_type: "representante",
            state: "recibida",
            channel: "publico",
            due_at: "2026-12-01T18:00:00Z",
            created_at: "2026-11-27T17:30:00Z",
            notice:
              "Solicitud registrada de forma neutral. Este acuse no confirma ni descarta la existencia de una cuenta en EspaciGo.",
          },
        }),
      });
    },
  );

  await page.route(
    "**/api/backend/privacy/public-requests/lookup",
    async (route: Route) => {
      const request = route.request();
      expect(request.method()).toBe("POST");
      capturedLookupAuthHeader = request.headers().authorization;
      capturedLookupBody = JSON.parse(request.postData() ?? "{}") as Record<
        string,
        unknown
      >;

      await route.fulfill({
        status: 200,
        headers: {
          "cache-control": "no-store",
          "content-type": "application/json; charset=utf-8",
        },
        body: JSON.stringify({
          data: {
            folio: expectedFolio,
            right_type: "bloqueo",
            requester_type: "representante",
            state: "en_revision",
            created_at: "2026-11-27T17:30:00Z",
            due_at: "2026-12-01T18:00:00Z",
            extended_due_at: "2026-12-03T18:00:00Z",
            extension_reason:
              "Validación adicional de mandato de representación legal.",
            response:
              "Se dispuso el bloqueo temporal preventivo mientras concluye la revisión de antecedentes.",
            events: [
              {
                id: "11111111-1111-4111-8111-111111111111",
                sequence: 1,
                event_type: "recepcion",
                state_to: "recibida",
                detail: "Ingreso público registrado.",
                created_at: "2026-11-27T17:30:00Z",
              },
              {
                id: "22222222-2222-4222-8222-222222222222",
                sequence: 2,
                event_type: "en_revision",
                state_from: "recibida",
                state_to: "en_revision",
                detail: "Expediente asignado a evaluación.",
                created_at: "2026-11-27T18:00:00Z",
              },
              {
                id: "33333333-3333-4333-8333-333333333333",
                sequence: 3,
                event_type: "prorroga",
                new_due_at: "2026-12-03T18:00:00Z",
                created_at: "2026-11-27T18:15:00Z",
              },
            ],
          },
        }),
      });
    },
  );

  await page.goto("/privacidad/solicitudes");

  await expect(
    page.getByRole("heading", {
      name: "Solicitudes de privacidad y derechos ARCO+",
    }),
  ).toBeVisible();
  await expect(page.getByText(/Ley N° 19\.628/)).toBeVisible();
  await expect(page.getByText(/Ley N° 21\.719/)).toBeVisible();
  await expect(page.getByText(/15 de octubre de 2027/)).toBeVisible();
  await expect(
    page.getByText(/no confirma ni revela la existencia de cuentas/i),
  ).toBeVisible();

  await expect(
    page.getByRole("link", { name: "Solicitudes ARCO+ (sin sesión)" }),
  ).toHaveAttribute("href", "/privacidad/solicitudes");

  // Seleccionar derecho Bloqueo temporal (2 días hábiles)
  await page.getByRole("radio", { name: /Bloqueo temporal/i }).click();
  await expect(page.getByText("2 días hábiles").first()).toBeVisible();

  // Seleccionar representante legal
  await page.getByRole("radio", { name: "Representante" }).click();
  await page.getByLabel("Nombre del representante").fill("Camila Rojas Soto");
  await page
    .getByLabel("Relación de representación")
    .selectOption("representante_legal");

  await page
    .getByLabel("Nombre completo del solicitante (titular)")
    .fill("Mateo Valdés Pérez");
  await page
    .getByLabel("Correo electrónico de contacto")
    .fill("representante@ejemplo.invalid");
  await page
    .getByLabel("Referencia del titular (opcional)")
    .fill("Contrato de arriendo folio 4501");
  await page
    .getByLabel("Detalle del requerimiento")
    .fill(
      "Solicito bloqueo temporal preventivo de los datos personales del titular representado.",
    );

  await page.getByRole("button", { name: "Enviar solicitud ARCO+" }).click();

  // Verificar payload y ausencia de Authorization
  expect(capturedSubmitAuthHeader).toBeUndefined();
  expect(capturedSubmitBody).toEqual({
    right_type: "bloqueo",
    requester_type: "representante",
    requester_name: "Mateo Valdés Pérez",
    requester_email: "representante@ejemplo.invalid",
    representative_name: "Camila Rojas Soto",
    representative_relation: "representante_legal",
    subject_reference: "Contrato de arriendo folio 4501",
    detail:
      "Solicito bloqueo temporal preventivo de los datos personales del titular representado.",
  });

  // Verificar acuse de recepción
  await expect(
    page.getByRole("heading", { name: "Acuse de recepción registrado" }),
  ).toBeVisible();
  await expect(page.getByTestId("receipt-folio")).toHaveText(expectedFolio);
  await expect(page.getByTestId("receipt-tracking-code")).toHaveText(
    expectedTrackingCode,
  );
  await expect(page.getByText(/se muestra una sola vez/i)).toBeVisible();
  await expect(
    page.getByText(/no confirma ni descarta la existencia de una cuenta/i),
  ).toBeVisible();
  await expect(page.getByText(/2 días hábiles/).last()).toBeVisible();

  // Pasar a pestaña de seguimiento con folio y código pre-rellenados
  await page
    .getByRole("button", { name: "Consultar este folio en seguimiento" })
    .click();

  await expect(page.getByRole("textbox", { name: "Folio" })).toHaveValue(
    expectedFolio,
  );
  await expect(
    page.getByRole("textbox", { name: "Código de seguimiento" }),
  ).toHaveValue(expectedTrackingCode);

  await page.getByRole("button", { name: "Consultar estado" }).click();

  expect(capturedLookupAuthHeader).toBeUndefined();
  expect(capturedLookupBody).toEqual({
    folio: expectedFolio,
    tracking_code: expectedTrackingCode,
  });

  // Verificar estado, plazo, prórroga, respuesta y línea de tiempo visible
  await expect(
    page.getByText("En revisión", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Validación adicional de mandato de representación legal."),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Se dispuso el bloqueo temporal preventivo mientras concluye la revisión de antecedentes.",
    ),
  ).toBeVisible();
  await expect(
    page.getByText("Solicitud registrada", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Ingreso público registrado.")).toBeVisible();
  await expect(page.getByText("Prórroga operativa registrada")).toBeVisible();
  await expect(page.getByText(/actor_id/i)).toHaveCount(0);
  await expect(page.getByText(/nota interna/i)).toHaveCount(0);
});

test("@controlled /privacidad/solicitudes: consulta con folio o código inválido muestra error 404 uniforme sin revelar cuentas ni folios", async ({
  page,
}) => {
  await mockAnonymousSession(page);

  await page.route(
    "**/api/backend/privacy/public-requests/lookup",
    async (route: Route) => {
      await route.fulfill({
        status: 404,
        headers: {
          "cache-control": "no-store",
          "content-type": "application/json; charset=utf-8",
        },
        body: JSON.stringify({
          error: {
            code: "privacy_request_not_found",
            message: "No encontrado",
          },
        }),
      });
    },
  );

  await page.goto("/privacidad/solicitudes");

  await page
    .getByRole("tab", { name: "Consultar solicitud por folio y código" })
    .click();

  await page.getByRole("textbox", { name: "Folio" }).fill("PRIV-2026-00000000");
  await page
    .getByRole("textbox", { name: "Código de seguimiento" })
    .fill("codigo_invalido_1234567890");
  await page.getByRole("button", { name: "Consultar estado" }).click();

  const alert = page
    .getByRole("tabpanel", { name: "Consultar solicitud por folio y código" })
    .getByRole("alert");
  await expect(alert).toBeVisible();
  await expect(alert).toContainText(
    "No encontramos una solicitud que coincida con el folio y código de seguimiento ingresados. Verifica los datos e intenta nuevamente.",
  );
});
