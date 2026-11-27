import { expect, test, type Page } from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";

const accountId = "770e8400-e29b-41d4-a716-446655440000";
const adminId = "990e8400-e29b-41d4-a716-446655440000";
const orgId = "880e8400-e29b-41d4-a716-446655440001";
const newOrgId = "880e8400-e29b-41d4-a716-446655440002";
const repMemberId = "880e8400-e29b-41d4-a716-446655440011";
const opMemberId = "880e8400-e29b-41d4-a716-446655440012";
const joinRequestId = "880e8400-e29b-41d4-a716-446655440021";
const incomingJoinRequestId = "880e8400-e29b-41d4-a716-446655440022";
const invitationId = "880e8400-e29b-41d4-a716-446655440031";
const spaceAId = "880e8400-e29b-41d4-a716-446655440041";
const spaceBId = "880e8400-e29b-41d4-a716-446655440042";
const verificationId = "880e8400-e29b-41d4-a716-446655440051";
const docErutId = "880e8400-e29b-41d4-a716-446655440061";
const docSiiId = "880e8400-e29b-41d4-a716-446655440062";
const docPowerId = "880e8400-e29b-41d4-a716-446655440063";
const docStorageRef = "880e8400-e29b-41d4-a716-446655440071";

const syntheticPdf = {
  name: "documento-sintetico.pdf",
  mimeType: "application/pdf",
  buffer: Buffer.from("%PDF-1.4\n% synthetic compliance fixture\n", "utf8"),
};

const futureIso = (daysAhead = 45) =>
  new Date(Date.now() + daysAhead * 86_400_000).toISOString();

async function setupHostSession(
  page: Page,
  extraHandler: (
    pathname: string,
    method: string,
    url: URL,
    postData: unknown,
  ) => { status?: number; body?: unknown; contentType?: string } | null,
) {
  const bff = createBffFixtureState();
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const method = request.method();

    if (pathname === "/api/backend/auth/session" && method === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "cache-control": "no-store" },
        body: JSON.stringify({
          account_id: accountId,
          roles: ["arrendador", "arrendatario"],
          capabilities: [],
          restricted_mode: false,
          expires_at: futureIso(1),
        }),
      });
      return;
    }
    if (pathname === "/api/backend/early-rentals" && method === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: [] }),
      });
      return;
    }

    let postData: unknown = null;
    if (
      method !== "GET" &&
      method !== "HEAD" &&
      request.headers()["content-type"]?.includes("application/json")
    ) {
      postData = request.postDataJSON();
    }

    const custom = extraHandler(pathname, method, url, postData);
    if (custom) {
      const adapted = bff.adapt(request, custom) ?? custom;
      await route.fulfill({
        status: adapted.status ?? 200,
        contentType: adapted.contentType ?? "application/json",
        headers: { "cache-control": "no-store" },
        body:
          adapted.body === undefined
            ? ""
            : typeof adapted.body === "string"
              ? adapted.body
              : JSON.stringify(adapted.body),
      });
      return;
    }

    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "not_found" } }),
    });
  });
}

test("1. /anfitrion/empresas: creación, colisión de RUT con adhesión y gestión de equipo", async ({
  page,
}) => {
  const organizations: Array<Record<string, unknown>> = [
    {
      id: orgId,
      legal_name: "Espacios Base SpA",
      trade_name: "Espacios Base",
      created_at: "2026-10-01T12:00:00Z",
      my_role: "representante_legal",
      compliance_status: "grace_active",
      grace_due_at: futureIso(40),
    },
  ];
  const membersByOrg: Record<string, Array<Record<string, unknown>>> = {
    [orgId]: [
      {
        id: repMemberId,
        usuario_id: accountId,
        rol: "representante_legal",
        estado: "activo",
      },
      {
        id: opMemberId,
        usuario_id: "770e8400-e29b-41d4-a716-446655440099",
        rol: "operador",
        estado: "activo",
      },
    ],
    [newOrgId]: [
      {
        id: "880e8400-e29b-41d4-a716-446655440019",
        usuario_id: accountId,
        rol: "representante_legal",
        estado: "activo",
      },
    ],
  };
  const joinRequestsByOrg: Record<string, Array<Record<string, unknown>>> = {
    [orgId]: [
      {
        id: incomingJoinRequestId,
        usuario_id: "770e8400-e29b-41d4-a716-446655440088",
        rol_propuesto: "administrador",
        estado: "pendiente",
        created_at: "2026-11-20T10:00:00Z",
        expires_at: futureIso(7),
      },
    ],
    [newOrgId]: [],
  };

  let createdPayload: Record<string, unknown> | null = null;
  let joinPayload: Record<string, unknown> | null = null;
  let invitePayload: Record<string, unknown> | null = null;
  let rolePatchPayload: Record<string, unknown> | null = null;

  await setupHostSession(page, (pathname, method, _url, postData) => {
    if (pathname === "/api/backend/host/organizations" && method === "GET") {
      return { body: { items: organizations } };
    }
    if (pathname === "/api/backend/host/organizations" && method === "POST") {
      const payload = postData as Record<string, string>;
      if (payload.company_rut === "77123456-9") {
        return {
          status: 409,
          body: { error: { code: "already_exists" } },
        };
      }
      createdPayload = payload;
      const created = {
        id: newOrgId,
        legal_name: payload.legal_name,
        trade_name: payload.trade_name,
        created_at: new Date().toISOString(),
        my_role: "representante_legal",
        compliance_status: "grace_active",
      };
      organizations.push(created);
      return { status: 201, body: { organization: created } };
    }
    const orgMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)$/,
    );
    if (orgMatch && method === "GET") {
      const org = organizations.find((item) => item.id === orgMatch[1]);
      return org ? { body: { organization: org } } : { status: 404, body: {} };
    }
    const membersMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)\/members$/,
    );
    if (membersMatch && method === "GET") {
      return { body: { items: membersByOrg[membersMatch[1]] ?? [] } };
    }
    const memberPatchMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)\/members\/([^/]+)$/,
    );
    if (memberPatchMatch && method === "PATCH") {
      const payload = postData as { rol: string };
      if (memberPatchMatch[2] === repMemberId) {
        return { status: 409, body: { error: { code: "conflict" } } };
      }
      rolePatchPayload = payload;
      const list = membersByOrg[memberPatchMatch[1]] ?? [];
      const target = list.find((m) => m.id === memberPatchMatch[2]);
      if (target) target.rol = payload.rol;
      return { status: 204, body: "" };
    }
    if (memberPatchMatch && method === "DELETE") {
      if (memberPatchMatch[2] === repMemberId) {
        return { status: 409, body: { error: { code: "conflict" } } };
      }
      membersByOrg[memberPatchMatch[1]] = (
        membersByOrg[memberPatchMatch[1]] ?? []
      ).filter((m) => m.id !== memberPatchMatch[2]);
      return { status: 204, body: "" };
    }
    const joinListMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)\/join-requests$/,
    );
    if (joinListMatch && method === "GET") {
      return { body: { items: joinRequestsByOrg[joinListMatch[1]] ?? [] } };
    }
    const joinApproveMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)\/join-requests\/([^/]+)\/approve$/,
    );
    if (joinApproveMatch && method === "POST") {
      const target = (joinRequestsByOrg[joinApproveMatch[1]] ?? []).find(
        (j) => j.id === joinApproveMatch[2],
      );
      if (target) target.estado = "aceptada";
      return {
        status: 200,
        body: {
          invitation: {
            id: invitationId,
            organizacion_id: joinApproveMatch[1],
            rol_propuesto: "administrador",
            estado: "pendiente",
            expires_at: futureIso(7),
            created_at: new Date().toISOString(),
          },
          invitation_token: "synthetic-join-approval-token-32-chars",
        },
      };
    }
    const inviteMatch = pathname.match(
      /^\/api\/backend\/host\/organizations\/([^/]+)\/invitations$/,
    );
    if (inviteMatch && method === "POST") {
      invitePayload = postData as Record<string, unknown>;
      return {
        status: 201,
        body: {
          invitation: {
            id: invitationId,
            organizacion_id: inviteMatch[1],
            target_email: invitePayload.email,
            rol_propuesto: invitePayload.rol,
            estado: "pendiente",
            expires_at: futureIso(7),
            created_at: new Date().toISOString(),
          },
          invitation_token: "synthetic-direct-invite-token-32-chars",
        },
      };
    }
    if (
      pathname === "/api/backend/host/organization-join-requests" &&
      method === "POST"
    ) {
      joinPayload = postData as Record<string, unknown>;
      return {
        status: 202,
        body: {
          request: {
            id: joinRequestId,
            rol_propuesto: joinPayload.rol_propuesto,
            estado: "pendiente",
            expires_at: futureIso(7),
            created_at: new Date().toISOString(),
          },
        },
      };
    }
    return null;
  });

  await page.goto("/anfitrion/empresas");
  await expect(page.getByRole("heading", { name: "Empresas" })).toBeVisible();

  // Gestión de equipo y representantes legales en empresa base
  await page
    .getByRole("button", { name: "Aprobar y preparar invitación" })
    .click();
  await expect(
    page.getByLabel("Enlace de invitación de un solo uso"),
  ).toHaveValue(/synthetic-join-approval-token-32-chars/);

  // Protección del último representante legal activo (409)
  await page.getByLabel("Rol de integrante 1").selectOption("operador");
  await expect(
    page.getByText(
      "La empresa debe conservar al menos un representante legal activo.",
    ),
  ).toBeVisible();

  // Cambio de rol y retiro de acceso de otro integrante
  await page.getByLabel("Rol de integrante 2").selectOption("administrador");
  expect(rolePatchPayload).toEqual({ rol: "administrador" });
  await expect(page.getByText("Rol actualizado.")).toBeVisible();

  await page.getByRole("button", { name: "Retirar acceso" }).nth(1).click();
  await expect(page.getByText("Acceso retirado.")).toBeVisible();

  // Invitación directa
  await page.getByLabel("Correo de la persona").fill("nuevo@empresa.test");
  await page.locator("#invite-role").selectOption("operador");
  await page.getByRole("button", { name: "Invitar" }).click();
  expect(invitePayload).toEqual({
    email: "nuevo@empresa.test",
    rol: "operador",
  });
  await expect(
    page.getByLabel("Enlace de invitación de un solo uso"),
  ).toHaveValue(/synthetic-direct-invite-token-32-chars/);

  // Colisión de RUT -> anti-duplicación y solicitud privada de adhesión
  await page.locator("#new-legal").fill("Empresa Ya Registrada SpA");
  await page.locator("#new-trade").fill("Empresa Duplicada");
  await page.locator("#new-rut").fill("77.123.456-9");
  await page.getByRole("button", { name: "Crear empresa" }).click();
  await expect(
    page.getByText(
      "Esta empresa ya se encuentra registrada. Puedes solicitar acceso con el mismo RUT; no mostraremos información de sus integrantes.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Solicitar adhesión a una empresa existente",
    }),
  ).toBeVisible();
  await page.locator("#join-role").selectOption("administrador");
  await page.getByRole("button", { name: "Enviar solicitud" }).click();
  expect(joinPayload).toEqual({
    company_rut: "77123456-9",
    rol_propuesto: "administrador",
  });
  await expect(page.getByText("Pendiente de revisión")).toBeVisible();

  // Creación exitosa de empresa nueva
  await page.locator("#new-legal").fill("Espacios Cordillera SpA");
  await page.locator("#new-trade").fill("Cordillera Cowork");
  await page.locator("#new-rut").fill("76.543.210-3");
  await page.getByRole("button", { name: "Crear empresa" }).click();
  expect(createdPayload).toEqual({
    legal_name: "Espacios Cordillera SpA",
    trade_name: "Cordillera Cowork",
    company_rut: "76543210-3",
  });
  await expect(
    page.getByText("Empresa creada. Completa su carpeta en Verificación KYB."),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Cordillera Cowork" }),
  ).toBeVisible();
});

test("2. /anfitrion/kyb: Persona Natural en gracia de 2 meses y estado disabled_missing_docs", async ({
  page,
}) => {
  let complianceStatus: "grace_active" | "disabled_missing_docs" =
    "grace_active";
  let siiStatus = "PENDING";
  let spaceBStatus = "PENDING";
  let siiSubmitted = false;
  let spaceBSubmitted = false;

  await setupHostSession(page, (pathname, method) => {
    if (pathname === "/api/backend/host/organizations" && method === "GET") {
      return { body: { items: [] } };
    }
    if (
      pathname === "/api/backend/kyb/personal/compliance" &&
      method === "GET"
    ) {
      return {
        body: {
          owner_usuario_id: accountId,
          compliance_status: complianceStatus,
          sii_giro_code: siiSubmitted ? "620100" : undefined,
          sii_status: siiStatus,
          grace_started_at: "2026-11-01T12:00:00Z",
          grace_due_at: futureIso(58),
          ...(complianceStatus === "disabled_missing_docs"
            ? {
                disabled_at: new Date().toISOString(),
                disabled_reason: "Plazo vencido sin documentos obligatorios.",
              }
            : {}),
        },
      };
    }
    if (pathname === "/api/backend/spaces" && method === "GET") {
      return {
        body: {
          items: [
            { id: spaceAId, title: "Espacio A Aprobado" },
            { id: spaceBId, title: "Espacio B Pendiente" },
          ],
        },
      };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceAId}/eligibility` &&
      method === "GET"
    ) {
      return {
        body: {
          can_publish: complianceStatus !== "disabled_missing_docs",
          can_book: complianceStatus !== "disabled_missing_docs",
          space_verified: true,
          compliance_status: complianceStatus,
          verification_status: "APPROVED",
          grace_due_at: futureIso(58),
        },
      };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceBId}/eligibility` &&
      method === "GET"
    ) {
      return {
        body: {
          can_publish: complianceStatus !== "disabled_missing_docs",
          can_book: complianceStatus !== "disabled_missing_docs",
          space_verified: false,
          compliance_status: complianceStatus,
          verification_status: spaceBStatus,
          grace_due_at: futureIso(58),
        },
      };
    }
    if (pathname === "/api/backend/kyb/personal/sii" && method === "POST") {
      siiSubmitted = true;
      siiStatus = "IN_REVIEW";
      return {
        status: 201,
        body: {
          id: "880e8400-e29b-41d4-a716-446655440091",
          owner_usuario_id: accountId,
          document_kind: "sii_start_activities",
          status: "PENDING",
          mime_type: "application/pdf",
          size_bytes: syntheticPdf.buffer.length,
          sii_giro_code: "620100",
          submitted_at: new Date().toISOString(),
        },
      };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceBId}/verification` &&
      method === "POST"
    ) {
      spaceBSubmitted = true;
      spaceBStatus = "IN_REVIEW";
      return {
        status: 201,
        body: {
          id: "880e8400-e29b-41d4-a716-446655440092",
          owner_usuario_id: accountId,
          space_id: spaceBId,
          document_kind: "property_or_sublease_authorization",
          status: "PENDING",
          mime_type: "application/pdf",
          size_bytes: syntheticPdf.buffer.length,
          submitted_at: new Date().toISOString(),
        },
      };
    }
    return null;
  });

  await page.goto("/anfitrion/kyb");
  await expect(
    page.getByRole("heading", { name: "Cumplimiento Persona Natural" }),
  ).toBeVisible();
  await expect(page.getByText("En plazo de gracia")).toBeVisible();
  await expect(page.getByText(/Plazo de gracia: quedan/)).toBeVisible();

  // Espacio A verificado vs Espacio B pendiente
  await expect(page.getByText("Espacio A Aprobado")).toBeVisible();
  await expect(
    page.getByText("Espacio verificado", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Espacio B Pendiente")).toBeVisible();
  await expect(
    page.getByText("Pendiente de revisión", { exact: true }),
  ).toBeVisible();

  // Envío SII y respaldo de Espacio B
  await page.locator("#personal-sii-giro").fill("620100");
  await page.locator("#personal-sii-file").setInputFiles(syntheticPdf);
  await page.getByRole("button", { name: "Enviar SII" }).click();
  expect(siiSubmitted).toBe(true);
  await expect(
    page.getByText("Certificado SII enviado para revisión."),
  ).toBeVisible();

  await page.locator(`#space-doc-${spaceBId}`).setInputFiles(syntheticPdf);
  await page.getByRole("button", { name: "Enviar respaldo" }).nth(1).click();
  expect(spaceBSubmitted).toBe(true);
  await expect(
    page.getByText("Respaldo del espacio enviado para revisión."),
  ).toBeVisible();

  // Transición a estado disabled_missing_docs
  complianceStatus = "disabled_missing_docs";
  await page.reload();
  await expect(
    page.getByText("Inhabilitada por falta de documentación", { exact: true }),
  ).toBeVisible();
  const alert = page
    .getByRole("alert")
    .filter({ hasText: "Tu perfil está inhabilitado" });
  await expect(alert).toContainText(
    "Tu perfil está inhabilitado por falta de documentación",
  );
  await expect(alert).toContainText(
    "Plazo vencido sin documentos obligatorios.",
  );
  await expect(
    alert.getByRole("link", {
      name: "Seguir usando EspaciGo como arrendatario",
    }),
  ).toHaveAttribute("href", "/reservas");
  await expect(
    alert.getByRole("link", { name: "Privacidad y cierre de cuenta" }),
  ).toHaveAttribute("href", "/mi-cuenta/privacidad");
  await expect(page.getByRole("button", { name: "Enviar SII" })).toBeEnabled();
});

test("3. /administracion/kyb: filtros, revisión por documento, blob preview e inhabilitar/rehabilitar", async ({
  page,
}) => {
  const complianceProfiles: Array<Record<string, unknown>> = [
    {
      owner_usuario_id: accountId,
      compliance_status: "compliant",
      sii_giro_code: "620100",
      sii_status: "APPROVED",
    },
    {
      owner_usuario_id: "770e8400-e29b-41d4-a716-446655440002",
      organization_id: orgId,
      legal_name: "Espacios Base SpA",
      compliance_status: "grace_active",
      sii_giro_code: "681010",
      sii_status: "PENDING",
      grace_due_at: futureIso(30),
    },
    {
      owner_usuario_id: "770e8400-e29b-41d4-a716-446655440003",
      compliance_status: "disabled_missing_docs",
      sii_status: "PENDING",
      disabled_reason: "Plazo vencido.",
    },
  ];

  let verificationState: Record<string, unknown> = {
    id: verificationId,
    organization_id: orgId,
    legal_name: "Espacios Base SpA",
    company_rut: "76123456-6",
    representative_name: "Representante Legal Sintético",
    representative_rut: "12345678-5",
    sii_giro_code: "681010",
    status: "PENDING",
    submitted_at: "2026-11-24T12:00:00Z",
    created_at: "2026-11-24T12:00:00Z",
    updated_at: "2026-11-24T12:00:00Z",
    documents: [
      {
        id: docErutId,
        verification_id: verificationId,
        kind: "erut",
        mime_type: "application/pdf",
        size_bytes: 1024,
        review_status: "PENDING",
      },
      {
        id: docSiiId,
        verification_id: verificationId,
        kind: "sii_start_activities",
        mime_type: "application/pdf",
        size_bytes: 2048,
        review_status: "PENDING",
      },
      {
        id: docPowerId,
        verification_id: verificationId,
        kind: "power_certificate",
        mime_type: "application/pdf",
        size_bytes: 3072,
        review_status: "PENDING",
      },
    ],
  };

  const filterRequests: string[] = [];
  let disablePayload: Record<string, unknown> | null = null;
  let enableCalled = false;
  let decisionPayload: Record<string, unknown> | null = null;

  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const method = request.method();

    if (pathname === "/api/backend/auth/session") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          account_id: adminId,
          roles: ["administrador_global"],
          capabilities: ["kyc.case.read", "kyc.review"],
          restricted_mode: false,
          expires_at: futureIso(1),
        }),
      });
      return;
    }
    if (pathname === "/api/backend/admin/session") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            account_id: adminId,
            audience: "admin",
            roles: ["administrador_global"],
            capabilities: ["kyc.case.read", "kyc.review"],
            mfa_verified_at: new Date().toISOString(),
            recent_auth_expires_at: futureIso(1),
            expires_at: futureIso(1),
          },
        }),
      });
      return;
    }
    if (pathname === "/api/backend/admin/global/me") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            account_id: adminId,
            roles: ["administrador_global"],
            capabilities: ["kyc.case.read", "kyc.review"],
            generated_at: new Date().toISOString(),
          },
        }),
      });
      return;
    }
    if (pathname === "/api/backend/admin/kyb/compliance" && method === "GET") {
      const statusFilter = url.searchParams.get("status") ?? "";
      filterRequests.push(statusFilter);
      const items = statusFilter
        ? complianceProfiles.filter(
            (item) => item.compliance_status === statusFilter,
          )
        : complianceProfiles;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items }),
      });
      return;
    }
    if (
      pathname === `/api/backend/admin/kyb/compliance/${accountId}/disable` &&
      method === "POST"
    ) {
      disablePayload = request.postDataJSON() as Record<string, unknown>;
      complianceProfiles[0].compliance_status = "disabled_missing_docs";
      complianceProfiles[0].disabled_reason = disablePayload.reason;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ updated: true }),
      });
      return;
    }
    if (
      pathname === `/api/backend/admin/kyb/compliance/${accountId}/enable` &&
      method === "POST"
    ) {
      enableCalled = true;
      complianceProfiles[0].compliance_status = "compliant";
      delete complianceProfiles[0].disabled_reason;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ updated: true }),
      });
      return;
    }
    if (
      pathname === "/api/backend/admin/kyb/compliance/documents" &&
      method === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: [] }),
      });
      return;
    }
    if (
      pathname === "/api/backend/admin/kyb/verifications" &&
      method === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          items:
            verificationState.status === "PENDING" ||
            verificationState.status === "IN_REVIEW"
              ? [verificationState]
              : [],
        }),
      });
      return;
    }
    if (
      pathname === `/api/backend/admin/kyb/verifications/${verificationId}` &&
      method === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(verificationState),
      });
      return;
    }
    if (
      pathname ===
        `/api/backend/admin/kyb/verifications/${verificationId}/review` &&
      method === "POST"
    ) {
      verificationState = {
        ...verificationState,
        status: "IN_REVIEW",
        documents: (
          verificationState.documents as Array<Record<string, unknown>>
        ).map((doc) => ({ ...doc, review_status: "IN_REVIEW" })),
      };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(verificationState),
      });
      return;
    }
    if (
      pathname ===
        `/api/backend/admin/kyb/verifications/${verificationId}/documents/${docErutId}/url` &&
      method === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          download_url: `/api/v1/kyb/documents/${docStorageRef}?expires=1999999999&key_version=v1&signature=${"a".repeat(64)}`,
          expires_at: futureIso(1),
        }),
      });
      return;
    }
    if (
      pathname === `/api/backend/kyb/documents/${docStorageRef}` &&
      method === "GET"
    ) {
      await route.fulfill({
        status: 200,
        contentType: "application/pdf",
        body: syntheticPdf.buffer,
      });
      return;
    }
    if (
      pathname ===
        `/api/backend/admin/kyb/verifications/${verificationId}/decision` &&
      method === "POST"
    ) {
      decisionPayload = request.postDataJSON() as Record<string, unknown>;
      const docs = (
        decisionPayload.documents as Array<{
          document_id: string;
          decision: string;
          rejection_reason?: string;
        }>
      ).reduce<Record<string, { decision: string; rejection_reason?: string }>>(
        (acc, item) => {
          acc[item.document_id] = item;
          return acc;
        },
        {},
      );
      verificationState = {
        ...verificationState,
        status:
          decisionPayload.decision === "APPROVE" ? "APPROVED" : "REJECTED",
        rejection_reason:
          decisionPayload.decision === "REJECT"
            ? "power_certificate: Vigencia vencida del poder"
            : undefined,
        documents: (
          verificationState.documents as Array<Record<string, unknown>>
        ).map((doc) => {
          const item = docs[doc.id as string];
          return {
            ...doc,
            review_status:
              item?.decision === "APPROVE" ? "APPROVED" : "REJECTED",
            ...(item?.rejection_reason
              ? { rejection_reason: item.rejection_reason }
              : {}),
          };
        }),
      };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(verificationState),
      });
      return;
    }

    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "not_found" } }),
    });
  });

  await page.goto("/administracion/kyb");
  await expect(
    page.getByRole("heading", { name: "Solicitudes KYB" }),
  ).toBeVisible();

  // Filtros de cumplimiento
  const filterSelect = page.getByLabel("Filtrar cumplimiento");
  await filterSelect.selectOption("compliant");
  await expect(page.getByText("Habilitada · SII APPROVED")).toBeVisible();

  await filterSelect.selectOption("grace_active");
  await expect(page.getByText(/En gracia \/ por vencer · SII/)).toBeVisible();

  await filterSelect.selectOption("disabled_missing_docs");
  await expect(
    page.getByText("Inhabilitada por falta de documentación · SII PENDING"),
  ).toBeVisible();

  await filterSelect.selectOption("");
  expect(filterRequests).toEqual([
    "",
    "compliant",
    "grace_active",
    "disabled_missing_docs",
    "",
  ]);

  // Botones Inhabilitar / Rehabilitar con prompt
  page.once("dialog", async (dialog) => {
    await dialog.accept("Documentación vencida en auditoría");
  });
  await page.getByRole("button", { name: "Inhabilitar" }).first().click();
  expect(disablePayload).toEqual({
    reason: "Documentación vencida en auditoría",
  });
  await expect(
    page.getByText("Motivo: Documentación vencida en auditoría"),
  ).toBeVisible();

  await page.getByRole("button", { name: "Rehabilitar" }).first().click();
  expect(enableCalled).toBe(true);
  await expect(page.getByText("Habilitada · SII APPROVED")).toBeVisible();

  // Cola pendiente -> revisión por documento individual + visor seguro vía blob
  await page.getByRole("button", { name: /Espacios Base SpA/ }).click();
  await page.getByRole("button", { name: "Pasar a revisión" }).click();

  await page.getByRole("button", { name: "Abrir documento" }).first().click();
  const previewFrame = page.locator('iframe[title="e-RUT"]');
  await expect(previewFrame).toBeVisible();
  await expect(previewFrame).toHaveAttribute("src", /^blob:/);

  // Modo por defecto: Decidir por documento
  const approveButtons = page.getByRole("button", {
    name: "Aprobar este documento",
  });
  const rejectButtons = page.getByRole("button", {
    name: "Rechazar este documento",
  });
  await approveButtons.nth(0).click();
  await approveButtons.nth(1).click();
  await rejectButtons.nth(2).click();
  await page
    .locator(`#document-reason-${docPowerId}`)
    .fill("Vigencia vencida del poder");
  await page
    .getByRole("button", { name: "Confirmar decisiones por documento" })
    .click();

  expect(decisionPayload).toEqual({
    decision: "REJECT",
    documents: [
      { document_id: docErutId, decision: "APPROVE" },
      { document_id: docSiiId, decision: "APPROVE" },
      {
        document_id: docPowerId,
        decision: "REJECT",
        rejection_reason: "Vigencia vencida del poder",
      },
    ],
  });
  await expect(
    page.getByText("Motivo anterior: Vigencia vencida del poder"),
  ).toBeVisible();
});

test("4. Tarjeta/ficha pública muestra Espacio verificado solo en Espacio A y Términos v2", async ({
  page,
}) => {
  await page.goto("/explorar");
  const cardA = page.getByRole("link", { name: /Espacio A Verificado/ });
  const cardB = page.getByRole("link", { name: /Espacio B Sin Respaldo/ });
  await expect(cardA).toBeVisible();
  await expect(cardB).toBeVisible();
  await expect(cardA.getByText("Espacio verificado")).toBeVisible();
  await expect(cardB.getByText("Espacio verificado")).toHaveCount(0);

  await page.goto(`/espacios/${spaceAId}`);
  await expect(
    page.getByRole("heading", { name: "Espacio A Verificado" }),
  ).toBeVisible();
  await expect(page.getByText("Espacio verificado")).toBeVisible();

  await page.goto(`/espacios/${spaceBId}`);
  await expect(
    page.getByRole("heading", { name: "Espacio B Sin Respaldo" }),
  ).toBeVisible();
  await expect(page.getByText("Espacio verificado")).toHaveCount(0);

  await page.goto("/terminos-y-condiciones");
  await expect(
    page.getByRole("heading", { name: "Términos y condiciones", level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByText("Versión informada por Go: evaluacion-terminos-20261126-v2"),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Documentación comercial y plazo para publicar",
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Documentos del titular y de cada espacio",
    }),
  ).toBeVisible();
});

test("5. /mi-cuenta: menú lateral limpio, tarjetas actualizadas y pestañas de Privacidad", async ({
  page,
}) => {
  await setupHostSession(page, (pathname, method) => {
    if (pathname === "/api/backend/profile" && method === "GET") {
      return {
        body: {
          display_name: "Anfitrión Sintético",
          phone: "912345678",
          updated_at: "2026-11-01T12:00:00Z",
        },
      };
    }
    if (pathname === "/api/backend/identity/profile" && method === "GET") {
      return {
        body: {
          state: "verified",
          complete: true,
          username: "Anfitrión Sintético",
          masked_email: "a***@example.invalid",
          masked_phone: "+56 9 **** 5678",
        },
      };
    }
    if (pathname === "/api/backend/rights-requests" && method === "GET") {
      return { body: { items: [] } };
    }
    if (
      pathname === "/api/backend/document-verification/cases" &&
      method === "GET"
    ) {
      return { body: { items: [] } };
    }
    if (pathname === "/api/backend/spaces" && method === "GET") {
      return { body: { items: [] } };
    }
    if (pathname === "/api/backend/document-review/cases" && method === "GET") {
      return { body: { items: [] } };
    }
    return null;
  });

  await page.goto("/mi-cuenta");

  const nav = page.getByRole("navigation", { name: "Secciones de mi cuenta" });
  for (const visibleTitle of [
    /^General/,
    /^Mis espacios/,
    /^Mis reservas/,
    /^Mis contratos/,
    /^Seguridad/,
  ]) {
    await expect(nav.getByRole("link", { name: visibleTitle })).toBeVisible();
  }
  for (const removedTitle of [
    /^Verificación KYB/,
    /^Privacidad/,
    /^Documentos/,
    /^Revisión documental/,
  ]) {
    await expect(nav.getByRole("link", { name: removedTitle })).toHaveCount(0);
  }

  // Tarjetas actualizadas en General
  await expect(
    page.getByText(
      "Privacidad, derechos ARCO, documentos privados y revisión documental.",
    ),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Verificación de identidad y verificaciones de espacio (KYB).",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Perfil empresas" }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Registra tus empresas (razón social, nombre comercial y RUT), gestiona representantes y verifica tu empresa.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Gestionar perfil empresas" }),
  ).toHaveAttribute("href", "/mi-cuenta/empresas");
  await expect(page.getByRole("link", { name: "Ver documentos" })).toHaveCount(
    0,
  );

  // Navegación a Privacidad con pestañas accesibles
  await page.getByRole("link", { name: "Gestionar privacidad" }).click();
  await expect(page).toHaveURL(/\/mi-cuenta\/privacidad$/);
  await expect(nav.getByRole("link", { name: /^General/ })).toHaveAttribute(
    "aria-current",
    "page",
  );

  const tablist = page.getByRole("tablist", {
    name: "Secciones de privacidad",
  });
  const privacyTab = tablist.getByRole("tab", { name: "Privacidad" });
  const docsTab = tablist.getByRole("tab", { name: "Documentos" });
  const reviewTab = tablist.getByRole("tab", { name: "Revisión documental" });

  await expect(privacyTab).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByText("Derechos ARCO y Privacidad de Datos"),
  ).toBeVisible();

  await docsTab.click();
  await expect(docsTab).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("heading", { name: "Documentos de verificación" }),
  ).toBeVisible();

  await reviewTab.click();
  await expect(reviewTab).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("heading", { name: "Revisión documental" }),
  ).toBeVisible();

  // Query param ?tab=documentos y ?tab=revision
  await page.goto("/mi-cuenta/privacidad?tab=documentos");
  await expect(page.getByRole("tab", { name: "Documentos" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.goto("/mi-cuenta/privacidad?tab=revision");
  await expect(
    page.getByRole("tab", { name: "Revisión documental" }),
  ).toHaveAttribute("aria-selected", "true");
});

test("6. /mi-cuenta/empresas: registro de empresa y sección Verificar empresa (KYB)", async ({
  page,
}) => {
  let kybRecord: Record<string, unknown> | null = null;
  let submittedCompanyName: string | null = null;

  await setupHostSession(page, (pathname, method) => {
    if (pathname === "/api/backend/host/organizations" && method === "GET") {
      return {
        body: {
          items: [
            {
              id: orgId,
              legal_name: "Espacios Base SpA",
              trade_name: "Espacios Base",
              my_role: "representante_legal",
              compliance_status: "grace_active",
              grace_due_at: futureIso(40),
            },
          ],
        },
      };
    }
    if (
      pathname === `/api/backend/host/organizations/${orgId}` &&
      method === "GET"
    ) {
      return {
        body: {
          organization: {
            id: orgId,
            legal_name: "Espacios Base SpA",
            trade_name: "Espacios Base",
            my_role: "representante_legal",
          },
        },
      };
    }
    if (
      pathname === `/api/backend/host/organizations/${orgId}/members` &&
      method === "GET"
    ) {
      return {
        body: {
          items: [
            {
              id: repMemberId,
              usuario_id: accountId,
              rol: "representante_legal",
              estado: "activo",
            },
          ],
        },
      };
    }
    if (
      pathname === `/api/backend/host/organizations/${orgId}/join-requests` &&
      method === "GET"
    ) {
      return { body: { items: [] } };
    }
    if (pathname === "/api/backend/kyb/verifications" && method === "GET") {
      return kybRecord ? { body: kybRecord } : { status: 404, body: {} };
    }
    if (pathname === "/api/backend/kyb/verifications" && method === "POST") {
      submittedCompanyName = "Espacios Base SpA";
      kybRecord = {
        id: verificationId,
        organization_id: orgId,
        legal_name: "Espacios Base SpA",
        company_rut: "76123456-0",
        representative_name: "Representante Legal Sintético",
        representative_rut: "12345678-5",
        sii_giro_code: "620100",
        status: "PENDING",
        submitted_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        documents: [],
      };
      return { status: 201, body: kybRecord };
    }
    return null;
  });

  await page.goto("/mi-cuenta/empresas");
  const breadcrumb = page.getByRole("navigation", {
    name: "Ruta de navegación",
  });
  await expect(
    breadcrumb.getByRole("link", { name: "General" }),
  ).toHaveAttribute("href", "/mi-cuenta");
  await expect(breadcrumb.getByText("Perfil empresas")).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(
    page.getByRole("heading", { name: "Perfil empresas", level: 1 }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Secciones de mi cuenta" })
      .getByRole("link", { name: /^General/ }),
  ).toHaveAttribute("aria-current", "page");

  await expect(
    page.getByRole("heading", { name: "Registrar empresa" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Verificar empresa" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Ir a Verificaciones de espacio" }),
  ).toHaveAttribute("href", "/mi-cuenta/verificaciones/espacios");

  // Razón social prellenada con legal_name en CompanyKYBCard
  const legalNameInputs = page.getByLabel("Razón social");
  await expect(legalNameInputs.last()).toHaveValue("Espacios Base SpA");

  await page.getByLabel("RUT empresa").fill("76.123.456-0");
  await page
    .getByLabel("Representante legal")
    .fill("Representante Legal Sintético");
  await page.getByLabel("RUT representante").fill("12.345.678-5");
  await page.getByLabel("Giro SII (6 dígitos)").fill("620100");
  await page.getByLabel("e-RUT").setInputFiles(syntheticPdf);
  await page
    .getByLabel("Certificado de Inicio de Actividades del SII")
    .setInputFiles(syntheticPdf);
  await page.getByLabel("Vigencia de Poderes").setInputFiles(syntheticPdf);
  await page.getByRole("button", { name: "Enviar verificación KYB" }).click();

  expect(submittedCompanyName).toBe("Espacios Base SpA");
  await expect(
    page.getByText("Solicitud de verificación empresarial KYB enviada."),
  ).toBeVisible();
});

test("7. /mi-cuenta/verificaciones -> Verificaciones de espacio con 0 espacios", async ({
  page,
}) => {
  await setupHostSession(page, (pathname, method) => {
    if (pathname === "/api/backend/identity/profile" && method === "GET") {
      return {
        body: {
          state: "verified",
          complete: true,
          username: "Anfitrión Sintético",
        },
      };
    }
    if (pathname === "/api/backend/reviewer/cases" && method === "GET") {
      return { body: { items: [] } };
    }
    if (pathname === "/api/backend/spaces" && method === "GET") {
      return { body: { items: [] } };
    }
    if (pathname === "/api/backend/host/organizations" && method === "GET") {
      return {
        body: {
          items: [
            {
              id: orgId,
              legal_name: "Espacios Base SpA",
              trade_name: "Espacios Base",
              my_role: "representante_legal",
              compliance_status: "grace_active",
            },
          ],
        },
      };
    }
    if (
      pathname === "/api/backend/kyb/personal/compliance" &&
      method === "GET"
    ) {
      return { status: 404, body: {} };
    }
    return null;
  });

  await page.goto("/mi-cuenta/verificaciones");
  await expect(
    page.getByRole("heading", { name: "Verificaciones de espacio" }),
  ).toBeVisible();
  await expect(page.getByText("Verificaciones KYB")).toBeVisible();

  await page
    .getByRole("link", { name: "Ver verificaciones de espacio" })
    .click();
  await expect(page).toHaveURL(/\/mi-cuenta\/verificaciones\/espacios$/);

  const breadcrumb = page.getByRole("navigation", {
    name: "Ruta de navegación",
  });
  await expect(
    breadcrumb.getByRole("link", { name: "General" }),
  ).toHaveAttribute("href", "/mi-cuenta");
  await expect(
    breadcrumb.getByRole("link", { name: "Verificaciones" }),
  ).toHaveAttribute("href", "/mi-cuenta/verificaciones");
  await expect(breadcrumb.getByText("Verificaciones KYB")).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(
    page.getByRole("heading", { name: "Verificaciones de espacio", level: 1 }),
  ).toBeVisible();

  // Estado vacío sin formularios de empresa ni de espacio ni SII
  await expect(
    page.getByText("Aún no tienes espacios publicados"),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Publicar un espacio" }),
  ).toHaveAttribute("href", "/anfitrion/espacios/nuevo");
  await expect(
    page.getByRole("link", { name: "Ir a Perfil empresas" }),
  ).toHaveAttribute("href", "/mi-cuenta/empresas");
  await expect(page.getByText("Verificación empresarial (KYB)")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Enviar respaldo" }),
  ).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Enviar SII" })).toHaveCount(0);
});

test("8. /mi-cuenta/verificaciones/espacios con 2 espacios (1 Persona Natural + 1 Empresa)", async ({
  page,
}) => {
  let personalSiiSubmitted = false;
  let personalSpaceSubmitted = false;

  await setupHostSession(page, (pathname, method) => {
    if (pathname === "/api/backend/spaces" && method === "GET") {
      return {
        body: {
          items: [
            {
              id: spaceAId,
              title: "Oficina Providencia Personal",
              state: "activa",
            },
            {
              id: spaceBId,
              title: "Bodega Corporativa",
              state: "borrador",
              organization_id: orgId,
            },
          ],
        },
      };
    }
    if (pathname === "/api/backend/host/organizations" && method === "GET") {
      return {
        body: {
          items: [
            {
              id: orgId,
              legal_name: "Espacios Base SpA",
              trade_name: "Espacios Base",
              my_role: "representante_legal",
              compliance_status: "grace_active",
              grace_due_at: futureIso(42),
            },
          ],
        },
      };
    }
    if (
      pathname === "/api/backend/kyb/personal/compliance" &&
      method === "GET"
    ) {
      return {
        body: {
          owner_usuario_id: accountId,
          compliance_status: "grace_active",
          sii_status: personalSiiSubmitted ? "IN_REVIEW" : "PENDING",
          ...(personalSiiSubmitted ? { sii_giro_code: "620100" } : {}),
          grace_due_at: futureIso(45),
        },
      };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceAId}/eligibility` &&
      method === "GET"
    ) {
      return {
        body: {
          can_publish: true,
          can_book: true,
          space_verified: false,
          compliance_status: "grace_active",
          verification_status: personalSpaceSubmitted ? "IN_REVIEW" : "PENDING",
          grace_due_at: futureIso(45),
        },
      };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceBId}/eligibility` &&
      method === "GET"
    ) {
      return {
        body: {
          can_publish: true,
          can_book: true,
          space_verified: true,
          compliance_status: "compliant",
          verification_status: "APPROVED",
        },
      };
    }
    if (pathname === "/api/backend/kyb/verifications" && method === "GET") {
      return { status: 404, body: {} };
    }
    if (pathname === "/api/backend/kyb/personal/sii" && method === "POST") {
      personalSiiSubmitted = true;
      return { status: 201, body: { id: "sii-1", status: "PENDING" } };
    }
    if (
      pathname === `/api/backend/kyb/spaces/${spaceAId}/verification` &&
      method === "POST"
    ) {
      personalSpaceSubmitted = true;
      return { status: 201, body: { id: "doc-a-1", status: "PENDING" } };
    }
    return null;
  });

  await page.goto("/mi-cuenta/verificaciones/espacios");

  const spaceCards = page.locator("[data-space-verification-card]");
  await expect(spaceCards).toHaveCount(2);

  // Tarjeta 1: Persona Natural
  const personalCard = page.locator(
    `[data-space-verification-card="${spaceAId}"]`,
  );
  await expect(personalCard).toContainText("Oficina Providencia Personal");
  await expect(personalCard).toContainText("Publicada");
  await expect(personalCard).toContainText("Persona Natural");
  await expect(
    personalCard.getByRole("heading", {
      name: "Certificado de Inicio de Actividades del SII",
    }),
  ).toBeVisible();

  await personalCard
    .locator(`#space-doc-${spaceAId}`)
    .setInputFiles(syntheticPdf);
  await personalCard.getByRole("button", { name: "Enviar respaldo" }).click();
  expect(personalSpaceSubmitted).toBe(true);
  await expect(
    page.getByText("Respaldo del espacio enviado para revisión."),
  ).toBeVisible();

  await personalCard.locator(`#sii-giro-${spaceAId}`).fill("620100");
  await personalCard
    .locator(`#sii-file-${spaceAId}`)
    .setInputFiles(syntheticPdf);
  await personalCard.getByRole("button", { name: "Enviar SII" }).click();
  expect(personalSiiSubmitted).toBe(true);
  await expect(
    page.getByText("Certificado SII enviado para revisión."),
  ).toBeVisible();

  // Tarjeta 2: Empresa
  const companyCard = page.locator(
    `[data-space-verification-card="${spaceBId}"]`,
  );
  await expect(companyCard).toContainText("Bodega Corporativa");
  await expect(companyCard).toContainText("Borrador");
  await expect(companyCard).toContainText("Empresa: Espacios Base");
  await expect(
    companyCard.getByText("Espacio verificado", { exact: true }),
  ).toBeVisible();
  await expect(
    companyCard.getByRole("heading", {
      name: "Verificación empresarial (KYB)",
    }),
  ).toBeVisible();
  await expect(companyCard.getByLabel("Razón social")).toHaveValue(
    "Espacios Base SpA",
  );
});
