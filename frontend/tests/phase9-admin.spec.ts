import {
  expect,
  test,
  type Page,
  type Request,
  type Route,
} from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createBffFixtureState } from "./helpers/bff-fixture";
import type {
  AdminCapability,
  AdminDamageClaim,
  AdminLocalDispute,
  AdminLocalNotice,
  AdminReviewReport,
  AdminRole,
  AdminVerificationCase,
  AuditEvent,
  GlobalAdminAccount,
  GlobalAdminAccountState,
  GlobalAdminKPIs,
  GlobalAdminMe,
  GlobalAdminOverview,
  GlobalAdminPublication,
  GlobalAdminPublicationState,
} from "@/features/admin/domain/types";

test.use({ trace: "off", screenshot: "off", video: "off" });

interface MockResult {
  status?: number;
  body?: unknown;
  contentType?: string;
}

type MockHandler = (
  route: Route,
  request: Request,
) => Promise<MockResult | null | undefined> | MockResult | null | undefined;

const canonicalAdminId = "990e8400-e29b-41d4-a716-446655440000";
const canonicalUserAccountId = "110e8400-e29b-41d4-a716-446655440000";
const canonicalPublicationId = "pub-test-01";
const canonicalVerificationId = "verif-test-01";
const canonicalDisputeId = "disp-test-01";

const ADMIN_CAPABILITIES: AdminCapability[] = [
  "accounts.list",
  "accounts.read",
  "accounts.email.reveal",
  "accounts.update",
  "accounts.block",
  "accounts.roles",
  "publications.list",
  "publications.read",
  "publications.update",
  "publications.moderate",
  "publications.retire",
  "admin_access.manage",
  "kpi.read",
  "audit.read",
  "audit.export",
  "legacy.administration",
  "payments.read",
  "payments.reconcile",
  "payments.refund.request",
  "payments.refund.approve",
  "reservations.read",
  "contracts.read",
  "kyc.case.read",
  "kyc.review",
  "privacy.requests.manage",
  "privacy.export.approve",
  "support.tickets.read",
  "support.tickets.manage",
  "approvals.read",
  "approvals.request",
  "approvals.decide",
];

interface TestAdminStore {
  me: GlobalAdminMe;
  overview: GlobalAdminOverview;
  kpis: GlobalAdminKPIs;
  accounts: GlobalAdminAccount[];
  publications: GlobalAdminPublication[];
  verifications: AdminVerificationCase[];
  disputes: AdminLocalDispute[];
  auditEvents: AuditEvent[];
  damageClaims: AdminDamageClaim[];
  reviewReports: AdminReviewReport[];
  notices: AdminLocalNotice[];
  simulate428OnPath: string | null;
  simulate403OnPath: string | null;
  simulate409OnSpaceModeration: boolean;
  handler: MockHandler;
}

function createAdminStore(options?: {
  adminAuthenticated?: boolean;
}): TestAdminStore {
  let authenticated = false;
  let adminAuthenticated = options?.adminAuthenticated ?? true;
  const sessionExpiresAt = new Date(Date.now() + 30 * 60_000).toISOString();
  const session = {
    account_id: canonicalAdminId,
    roles: ["administrador_global"],
    capabilities: ADMIN_CAPABILITIES,
    restricted_mode: false,
    expires_at: sessionExpiresAt,
  };
  const adminSession = {
    account_id: canonicalAdminId,
    audience: "admin",
    roles: ["administrador_global"] as AdminRole[],
    capabilities: ADMIN_CAPABILITIES,
    mfa_verified_at: new Date().toISOString(),
    recent_auth_expires_at: sessionExpiresAt,
    expires_at: sessionExpiresAt,
  };
  const me: GlobalAdminMe = {
    account_id: canonicalAdminId,
    roles: ["administrador_global"],
    capabilities: ADMIN_CAPABILITIES,
    generated_at: "2026-10-25T12:00:00Z",
  };

  const overview: GlobalAdminOverview = {
    total_accounts: 42,
    connected_accounts: 8,
    administrative_blocked: 3,
    by_state: { activo: 35, correo_pendiente: 4, bloqueado: 3 },
    by_role: { arrendatario: 30, arrendador: 10, administrador: 2 },
    generated_at: "2026-10-25T12:00:00Z",
    definition: "Resumen global de métricas del sistema",
  };

  const kpis: GlobalAdminKPIs = {
    period: {
      from: "2026-09-25T00:00:00Z",
      until: "2026-10-25T00:00:00Z",
      time_zone: "America/Santiago",
    },
    generated_at: "2026-10-25T12:00:00Z",
    definition: "KPIs operativos consolidados",
    metrics: {
      total_revenue_clp: 15000000,
      active_reservations: 18,
      occupancy_rate: 0.74,
      dispute_rate: 0.02,
    },
  };

  const accounts: GlobalAdminAccount[] = [
    {
      account_id: canonicalUserAccountId,
      email: "usuario.prueba@espacigo.cl",
      display_name: "Usuario Prueba",
      phone: "+56912345678",
      profile_updated_at: "2026-10-25T10:00:00Z",
      state: "activo",
      created_at: "2026-10-01T10:00:00Z",
      administrative_blocked: false,
      roles: ["arrendatario"],
      publication_count: 0,
      reservation_count: 2,
      incident_count: 0,
    },
    {
      account_id: "220e8400-e29b-41d4-a716-446655440000",
      email: "anfitrion.bloqueado@espacigo.cl",
      display_name: "Anfitrión Bloqueado",
      phone: "+56987654321",
      profile_updated_at: "2026-10-24T10:00:00Z",
      state: "bloqueado",
      created_at: "2026-10-02T10:00:00Z",
      administrative_blocked: true,
      roles: ["arrendador"],
      publication_count: 1,
      reservation_count: 0,
      incident_count: 1,
    },
  ];

  const publications: GlobalAdminPublication[] = [
    {
      publication_id: canonicalPublicationId,
      owner_id: canonicalUserAccountId,
      owner_name: "Usuario Prueba",
      owner_email: "usuario.prueba@espacigo.cl",
      category_code: "bodega",
      category_name: "Bodega Segura",
      title: "Bodega Centro Santiago 25m2",
      description: "Espacio amplio y seguro para almacenamiento.",
      area_m2: "25",
      capacity: 5,
      usage_rules: "Sin sustancias peligrosas.",
      address: "Av. Libertador Bernardo O'Higgins 1234, Santiago",
      state: "activa",
      rate_unit: "mes",
      base_price_clp: "120000",
      updated_at: "2026-10-25T10:00:00Z",
      publicly_visible: true,
      public_cover_available: false,
      attribute_schema_version: 1,
      attributes: {},
      photos: [],
    },
  ];

  const verifications: AdminVerificationCase[] = [
    {
      id: canonicalVerificationId,
      type: "kyc",
      applicant_id: canonicalUserAccountId,
      state: "en_revision",
      created_at: "2026-10-25T09:00:00Z",
      updated_at: "2026-10-25T09:00:00Z",
      notes: "Documentación enviada para revisión",
    },
  ];

  const disputes: AdminLocalDispute[] = [
    {
      dispute_id: canonicalDisputeId,
      reservation_id: "res-test-99",
      renter_id: canonicalUserAccountId,
      host_id: "220e8400-e29b-41d4-a716-446655440000",
      state: "abierta",
      reason: "Discrepancia en devolución de depósito de garantía.",
      created_at: "2026-10-24T15:00:00Z",
    },
  ];

  const auditEvents: AuditEvent[] = [
    {
      event_id: "audit-evt-01",
      actor_id: canonicalAdminId,
      action: "publication.state_update",
      resource_type: "publication",
      resource_id: canonicalPublicationId,
      timestamp: "2026-10-25T12:00:00Z",
      result: "exito",
      details: { reason: "revision_administrativa" },
    },
  ];

  const damageClaims: AdminDamageClaim[] = [];
  const reviewReports: AdminReviewReport[] = [];
  const notices: AdminLocalNotice[] = [];

  const store: TestAdminStore = {
    me,
    overview,
    kpis,
    accounts,
    publications,
    verifications,
    disputes,
    auditEvents,
    damageClaims,
    reviewReports,
    notices,
    simulate428OnPath: null,
    simulate403OnPath: null,
    simulate409OnSpaceModeration: false,
    handler: (route, request) => {
      const url = new URL(request.url());
      const pathname = url.pathname;
      const method = request.method();

      // Security simulation gates
      if (
        store.simulate428OnPath &&
        pathname.includes(store.simulate428OnPath)
      ) {
        return {
          status: 428,
          body: {
            code: "precondition_required",
            message:
              "Se requiere autenticación reciente para realizar esta operación administrativa sensible.",
          },
        };
      }

      if (
        store.simulate403OnPath &&
        pathname.includes(store.simulate403OnPath)
      ) {
        return {
          status: 403,
          body: {
            code: "forbidden",
            message: "Acceso denegado o sesión revocada.",
          },
        };
      }

      // Auth Session Profile & Login
      if (pathname === "/api/backend/auth/login" && method === "POST") {
        authenticated = true;
        return {
          status: 200,
          body: {
            account_id: canonicalAdminId,
            roles: ["administrador_global"],
            capabilities: store.me.capabilities,
            expires_at: new Date(Date.now() + 3600_000).toISOString(),
            restricted_mode: false,
          },
        };
      }

      if (pathname === "/api/backend/auth/session" && method === "GET") {
        return authenticated
          ? { status: 200, body: session }
          : { status: 401, body: { code: "unauthenticated" } };
      }

      if (pathname === "/api/backend/auth/logout" && method === "POST") {
        authenticated = false;
        return { status: 204, body: "" };
      }

      // Dedicated Admin Staff Session
      if (pathname === "/api/backend/admin/session" && method === "GET") {
        return adminAuthenticated
          ? { status: 200, body: { data: adminSession } }
          : { status: 401, body: { code: "unauthenticated" } };
      }

      if (
        pathname === "/api/backend/admin/session/start" &&
        method === "POST"
      ) {
        return {
          status: 200,
          body: {
            data: {
              status: "mfa_required",
              challenge_id: "mfa-chal-01",
              methods: ["totp"],
              expires_at: sessionExpiresAt,
            },
          },
        };
      }

      if (pathname === "/api/backend/admin/session/mfa" && method === "POST") {
        adminAuthenticated = true;
        return { status: 200, body: { data: adminSession } };
      }

      if (pathname === "/api/backend/admin/session" && method === "DELETE") {
        adminAuthenticated = false;
        return { status: 204, body: "" };
      }

      // Global Admin Me & Overview & KPIs
      if (pathname === "/api/backend/admin/global/me" && method === "GET") {
        return { status: 200, body: { data: store.me } };
      }

      if (
        pathname === "/api/backend/admin/global/overview" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: store.overview } };
      }

      if (pathname === "/api/backend/admin/global/kpis" && method === "GET") {
        return { status: 200, body: store.kpis };
      }

      // Audit events export & list
      if (
        pathname === "/api/backend/admin/global/audit-events/export" &&
        method === "GET"
      ) {
        return {
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            exported_events: store.auditEvents.length,
            items: store.auditEvents,
            generated_at: new Date().toISOString(),
          }),
        };
      }

      if (
        pathname === "/api/backend/admin/global/audit-events" &&
        method === "GET"
      ) {
        return {
          status: 200,
          body: { data: { items: store.auditEvents } },
        };
      }

      // Global Accounts list & mutations
      if (
        pathname === "/api/backend/admin/global/accounts" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: { items: store.accounts } } };
      }

      const rolesMatch = pathname.match(
        /\/api\/backend\/admin\/global\/accounts\/([^/]+)\/roles\/([^/]+)/,
      );
      if (rolesMatch && method === "PUT") {
        const [, accountId, role] = rolesMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const targetIdx = store.accounts.findIndex(
          (a) => a.account_id === accountId,
        );
        if (targetIdx >= 0) {
          const target = store.accounts[targetIdx];
          const newRoles = json.granted
            ? [...new Set([...target.roles, role as AdminRole])]
            : target.roles.filter((r) => r !== role);
          const updated: GlobalAdminAccount = {
            ...target,
            roles: newRoles,
          };
          store.accounts[targetIdx] = updated;
          return { status: 200, body: { data: updated } };
        }
        return { status: 404, body: { error: "account_not_found" } };
      }

      const blockMatch = pathname.match(
        /\/api\/backend\/admin\/global\/accounts\/([^/]+)\/block/,
      );
      if (blockMatch && method === "POST") {
        const [, accountId] = blockMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const targetIdx = store.accounts.findIndex(
          (a) => a.account_id === accountId,
        );
        if (targetIdx >= 0) {
          const target = store.accounts[targetIdx];
          const updated: GlobalAdminAccount = {
            ...target,
            administrative_blocked: !!json.blocked,
            state: (json.blocked
              ? "bloqueado"
              : "activo") as GlobalAdminAccountState,
          };
          store.accounts[targetIdx] = updated;
          return { status: 200, body: { data: updated } };
        }
        return { status: 404, body: { error: "account_not_found" } };
      }

      const revokeMatch = pathname.match(
        /\/api\/backend\/admin\/global\/accounts\/([^/]+)\/sessions\/revoke/,
      );
      if (revokeMatch && method === "POST") {
        return { status: 200, body: { data: { revoked: true } } };
      }

      const sensitiveAccessMatch = pathname.match(
        /\/api\/backend\/admin\/global\/accounts\/([^/]+)\/sensitive-access/,
      );
      if (sensitiveAccessMatch && method === "POST") {
        const [, accountId] = sensitiveAccessMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const target = store.accounts.find((a) => a.account_id === accountId);
        if (!target) {
          return { status: 404, body: { error: "account_not_found" } };
        }
        return {
          status: 200,
          body: {
            data: {
              account_id: accountId,
              field: json.field ?? "email",
              value: target.email,
              masked_value: "u••••l",
              scope: json.scope ?? "account_email",
              reason: json.reason,
              case_id: json.case_id,
              expires_at: new Date(Date.now() + 60_000).toISOString(),
              audit_event_id: "audit-sensitive-01",
            },
          },
        };
      }

      // Publications list & state change
      if (
        pathname === "/api/backend/admin/global/publications" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: { items: store.publications } } };
      }

      const pubStateMatch = pathname.match(
        /\/api\/backend\/admin\/global\/publications\/([^/]+)\/state/,
      );
      if (pubStateMatch && method === "POST") {
        if (store.simulate409OnSpaceModeration) {
          return {
            status: 409,
            body: {
              code: "conflict",
              message:
                "Conflicto de concurrencia: el recurso fue modificado por otra sesión administrativa.",
            },
          };
        }
        const [, pubId] = pubStateMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const targetIdx = store.publications.findIndex(
          (p) => p.publication_id === pubId,
        );
        if (targetIdx >= 0) {
          const target = store.publications[targetIdx];
          const updated: GlobalAdminPublication = {
            ...target,
            state: json.state as GlobalAdminPublicationState,
            updated_at: new Date().toISOString(),
          };
          store.publications[targetIdx] = updated;
          return { status: 200, body: { data: updated } };
        }
        return { status: 404, body: { error: "publication_not_found" } };
      }

      // Verifications list & review
      if (pathname === "/api/backend/admin/verifications" && method === "GET") {
        return { status: 200, body: { items: store.verifications } };
      }

      if (
        pathname === "/api/backend/admin/verifications/failed" &&
        method === "GET"
      ) {
        return { status: 200, body: { items: [] } };
      }

      const reviewMatch = pathname.match(
        /\/api\/backend\/admin\/verifications\/([^/]+)\/review/,
      );
      if (reviewMatch && method === "POST") {
        const [, verifId] = reviewMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const targetIdx = store.verifications.findIndex(
          (v) => v.id === verifId,
        );
        if (targetIdx >= 0) {
          const target = store.verifications[targetIdx];
          const updated: AdminVerificationCase = {
            ...target,
            state: json.decision === "aprobar" ? "aprobada" : "rechazada",
            notes: json.reason ?? target.notes,
            updated_at: new Date().toISOString(),
          };
          store.verifications[targetIdx] = updated;
          return { status: 200, body: updated };
        }
        return { status: 404, body: { error: "verification_not_found" } };
      }

      // Disputes list & close
      if (pathname === "/api/backend/admin/disputes" && method === "GET") {
        return { status: 200, body: { items: store.disputes } };
      }

      const closeMatch = pathname.match(
        /\/api\/backend\/admin\/disputes\/([^/]+)\/close/,
      );
      if (closeMatch && method === "POST") {
        const [, disputeId] = closeMatch;
        const json = JSON.parse(request.postData() ?? "{}");
        const targetIdx = store.disputes.findIndex(
          (d) => d.dispute_id === disputeId,
        );
        if (targetIdx >= 0) {
          const target = store.disputes[targetIdx];
          const updated: AdminLocalDispute = {
            ...target,
            state: "cerrada",
            close_reason: json.reason_code,
            closed_at: new Date().toISOString(),
          };
          store.disputes[targetIdx] = updated;
          return { status: 200, body: updated };
        }
        return { status: 404, body: { error: "dispute_not_found" } };
      }

      // Fallbacks for secondary endpoints
      if (
        pathname === "/api/backend/admin/local/damage-claims" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: { items: store.damageClaims } } };
      }

      if (
        pathname === "/api/backend/admin/local/review-reports" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: { items: store.reviewReports } } };
      }

      if (pathname === "/api/backend/admin/local/notices" && method === "GET") {
        return { status: 200, body: { data: { items: store.notices } } };
      }

      if (
        (pathname === "/api/backend/admin/local/published-spaces" ||
          pathname === "/api/backend/admin/spaces") &&
        method === "GET"
      ) {
        return { status: 200, body: { data: { items: [] } } };
      }

      return null;
    },
  };

  return store;
}

async function installMockGateway(page: Page, handler: MockHandler) {
  const bff = createBffFixtureState();
  await page.route("**/api/backend/**", async (route) => {
    const request = route.request();
    const result = bff.adapt(request, await handler(route, request));
    if (result) {
      await route.fulfill({
        status: result.status ?? 200,
        contentType: result.contentType ?? "application/json",
        headers: result.headers,
        body:
          typeof result.body === "string"
            ? result.body
            : JSON.stringify(result.body),
      });
      return;
    }
    await route.continue();
  });
}

async function loginAndNavigate(page: Page, destination = "/administracion") {
  const [pathname] = destination.split("?");

  const header = page.locator("header");
  const isConfirmed =
    (await header
      .getByRole("button", { name: "Cerrar sesión", exact: true })
      .isVisible()
      .catch(() => false)) ||
    (await header
      .getByRole("link", { name: "Mi cuenta", exact: true })
      .isVisible()
      .catch(() => false));

  if (isConfirmed) {
    await page.goto(destination);
    await page.waitForURL((url) => url.pathname === pathname);
    await header
      .getByRole("button", { name: "Cerrar sesión", exact: true })
      .waitFor({ state: "visible" });
    return;
  }

  await page.goto(`/acceso?return_to=${encodeURIComponent(pathname)}`);

  const emailInput = page.getByLabel("Correo electrónico");
  const authOrGuest = await Promise.race([
    page
      .waitForURL((url) => url.pathname === pathname)
      .then(() => "authenticated" as const),
    emailInput.waitFor({ state: "visible" }).then(() => "guest" as const),
  ]);

  if (authOrGuest === "guest") {
    await emailInput.fill("administrador.global@example.invalid");
    await page
      .getByLabel("Contraseña", { exact: true })
      .fill("SecureAdmin123!");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await page.waitForURL((url) => url.pathname === pathname);
  }
}

test.describe("Fase 9 - Administración, Moderación y Gobernanza", () => {
  test("@phase9 acceso exclusivo de administración via /admin/session/start y verificación MFA", async ({
    page,
  }) => {
    const store = createAdminStore({ adminAuthenticated: false });
    let startPayload: Record<string, unknown> | null = null;
    let mfaPayload: Record<string, unknown> | null = null;

    await installMockGateway(page, (route, request) => {
      const url = new URL(request.url());
      if (
        url.pathname === "/api/backend/admin/session/start" &&
        request.method() === "POST"
      ) {
        startPayload = JSON.parse(request.postData() ?? "{}");
      }
      if (
        url.pathname === "/api/backend/admin/session/mfa" &&
        request.method() === "POST"
      ) {
        mfaPayload = JSON.parse(request.postData() ?? "{}");
      }
      return store.handler(route, request);
    });

    await loginAndNavigate(page, "/administracion");
    await expect(
      page.getByText("Acceso exclusivo de Administración"),
    ).toBeVisible();

    await page
      .getByLabel("Correo administrativo")
      .fill("staff.admin@espacigo.cl");
    await page.getByLabel("Contraseña", { exact: true }).fill("StaffPass123!");
    await page
      .getByRole("button", { name: "Iniciar sesión administrativa" })
      .click();

    await expect(
      page.getByText("Verificación MFA administrativa"),
    ).toBeVisible();
    expect(startPayload).toEqual({
      email: "staff.admin@espacigo.cl",
      password: "StaffPass123!",
    });

    await page.getByLabel("Código de verificación").fill("123456");
    await page.getByRole("button", { name: "Verificar MFA" }).click();

    expect(mfaPayload).toEqual({
      challenge_id: "mfa-chal-01",
      code: "123456",
    });
    await expect(page.getByText("Total Cuentas")).toBeVisible();
    await expect(page.getByText("42", { exact: true })).toBeVisible();
  });

  test("ficha 360 consulta una cuenta y limita los espacios a su propietario", async ({
    page,
  }) => {
    const store = createAdminStore();
    const account = store.accounts[0]!;
    let requestedOwner: string | null = null;
    let sensitiveAccessPayload: Record<string, unknown> | null = null;
    await installMockGateway(page, (route, request) => {
      const url = new URL(request.url());
      if (
        url.pathname ===
          `/api/backend/admin/global/accounts/${account.account_id}` &&
        request.method() === "GET"
      ) {
        return { body: { data: { ...account, email: "u••••l" } } };
      }
      if (
        url.pathname ===
          `/api/backend/admin/global/accounts/${account.account_id}/sensitive-access` &&
        request.method() === "POST"
      ) {
        sensitiveAccessPayload = JSON.parse(request.postData() ?? "{}");
      }
      if (
        url.pathname === "/api/backend/admin/global/publications" &&
        request.method() === "GET"
      ) {
        requestedOwner = url.searchParams.get("owner_id");
        return {
          body: {
            data: {
              items: [
                ...store.publications,
                {
                  ...store.publications[0],
                  publication_id: "other-space",
                  owner_id: canonicalAdminId,
                  title: "Espacio de otra cuenta",
                },
              ],
            },
          },
        };
      }
      return store.handler(route, request);
    });
    await loginAndNavigate(page, "/administracion/cuentas");
    await page.getByRole("link", { name: "Abrir ficha 360" }).first().click();
    await expect(
      page.getByRole("heading", { name: account.display_name! }),
    ).toBeVisible();
    await expect(page.getByText("u••••l", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Revelar correo" }).click();
    await expect(page.getByText("Acceso auditado a correo")).toBeVisible();
    await page
      .locator(`#sensitive-reason-${account.account_id}`)
      .fill("Verificación de titularidad solicitada en ticket de soporte");
    await page
      .locator(`#sensitive-case-${account.account_id}`)
      .fill("SUP-2026-1042");
    await page.locator(`#sensitive-scope-${account.account_id}`).click();
    await page
      .getByRole("option", { name: /Correo de cuenta \(account_email\)/i })
      .click();
    await page.getByRole("button", { name: "Confirmar y revelar" }).click();

    expect(sensitiveAccessPayload).toEqual({
      field: "email",
      reason: "Verificación de titularidad solicitada en ticket de soporte",
      case_id: "SUP-2026-1042",
      scope: "account_email",
    });
    await expect(page.getByText(account.email, { exact: true })).toBeVisible();
    await expect(page.getByText(/Expira:/i)).toBeVisible();
    await page.getByRole("button", { name: "Ocultar" }).click();
    await expect(page.getByText("u••••l", { exact: true })).toBeVisible();

    expect(requestedOwner).toBeNull();
    await page
      .getByRole("navigation", { name: "Secciones de la ficha" })
      .getByRole("button", { name: "Espacios" })
      .click();
    await expect(page.getByText(store.publications[0]!.title)).toBeVisible();
    await expect(page.getByText("Espacio de otra cuenta")).toHaveCount(0);
    expect(requestedOwner).toBe(account.account_id);

    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  });
  test("@phase9 flujo de capacidades, navegación, KPIs y exportación segura de bitácora Blob con revocación", async ({
    page,
  }) => {
    const store = createAdminStore();
    await installMockGateway(page, store.handler);

    // Monitor Blob ObjectURL creation and revocation
    await page.addInitScript(() => {
      const originalCreate = URL.createObjectURL;
      const originalRevoke = URL.revokeObjectURL;
      (window as unknown as Record<string, unknown>).__blobUrlsCreated = [];
      (window as unknown as Record<string, unknown>).__blobUrlsRevoked = [];
      URL.createObjectURL = function (blob: Blob) {
        const url = originalCreate.call(URL, blob);
        (
          (window as unknown as Record<string, unknown>)
            .__blobUrlsCreated as string[]
        ).push(url);
        return url;
      };
      URL.revokeObjectURL = function (url: string) {
        (
          (window as unknown as Record<string, unknown>)
            .__blobUrlsRevoked as string[]
        ).push(url);
        return originalRevoke.call(URL, url);
      };
    });

    // 1. Acceso autenticado al panel de administración
    await loginAndNavigate(page, "/administracion");
    await expect(page.locator("h1")).toHaveText("Panel de Administración");

    // 2. Un solo rol visible por sesión en Badge de navegación
    await expect(page.getByText("Administrador Global")).toBeVisible();

    // 3. Renderizado de navegación por capacidades
    await expect(
      page.getByRole("link", { name: "Resumen / KPIs" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Verificaciones KYC" }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "Espacios" })).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Disputas y Reclamos" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Cuentas y Roles" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Auditoría y Reportes" }),
    ).toBeVisible();

    // 4. Renderizado de KPIs y métricas consolidadas
    await expect(page.getByText("Total Cuentas")).toBeVisible();
    await expect(page.getByText("42", { exact: true })).toBeVisible();
    await expect(
      page.getByText(/3 bloqueadas administrativamente/i),
    ).toBeVisible();
    await expect(page.getByText("Cuentas conectadas")).toBeVisible();
    await expect(
      page.getByText("Distribución de Cuentas por Estado"),
    ).toBeVisible();
    await expect(
      page.getByText("Distribución de Cuentas por Rol"),
    ).toBeVisible();

    // 5. Exportación segura de bitácora Blob con revocación de URL
    await page.getByRole("link", { name: "Auditoría y Reportes" }).click();
    const exportBtn = page.getByRole("button", { name: "JSON", exact: true });
    await expect(exportBtn).toBeVisible();
    await exportBtn.click();

    await expect
      .poll(() =>
        page.evaluate(() => {
          const telemetry = window as unknown as Record<string, string[]>;
          const created = telemetry.__blobUrlsCreated?.[0];
          return !!created && telemetry.__blobUrlsRevoked?.includes(created);
        }),
      )
      .toBe(true);
  });

  test("@phase9 flujo moderación KYC (motivo obligatorio 5+ chars) y moderación de espacios con concurrencia optimista 409", async ({
    page,
  }) => {
    const store = createAdminStore();
    await installMockGateway(page, store.handler);

    // --- PARTE 1: Moderación KYC en /administracion/verificaciones ---
    await loginAndNavigate(page, "/administracion/verificaciones");
    await expect(
      page.getByRole("heading", {
        name: /Moderación de Verificaciones KYC/i,
      }),
    ).toBeVisible();
    await expect(page.getByText("En revisión", { exact: true })).toBeVisible();

    // 1.1 Rechazo con motivo obligatorio: validación < 5 caracteres
    await page.getByRole("button", { name: "Rechazar" }).click();
    await expect(page.getByText("Rechazar Verificación")).toBeVisible();

    const reasonInput = page.locator("#review-reason");
    await reasonInput.fill("abc");
    await page.getByRole("button", { name: "Confirmar Rechazo" }).click();

    // Error obligatorio visible
    await expect(
      page.getByText(
        /El motivo de rechazo es obligatorio y debe tener entre 5 y 500 caracteres/i,
      ),
    ).toBeVisible();

    // 1.2 Rechazo con motivo válido (5+ caracteres)
    await reasonInput.fill(
      "Documento ilegible con reflejos que impiden el cotejo.",
    );
    await page.getByRole("button", { name: "Confirmar Rechazo" }).click();

    // Caso pasa a estado rechazada
    await page.getByRole("button", { name: /Todos los Casos/i }).click();
    await expect(page.getByText("Rechazada")).toBeVisible();

    // --- PARTE 2: Moderación de Espacios con Concurrencia Optimista (409) ---
    await page.getByRole("link", { name: "Espacios" }).click();
    await page.waitForURL(/\/administracion\/espacios$/);
    await expect(
      page.getByRole("heading", {
        name: /Moderación de Espacios y Publicaciones/i,
      }),
    ).toBeVisible();
    await expect(page.getByText("Bodega Centro Santiago 25m2")).toBeVisible();
    await expect(page.getByText("Activa", { exact: true })).toBeVisible();

    // 2.1 Simular conflicto HTTP 409
    store.simulate409OnSpaceModeration = true;
    await page.getByRole("button", { name: "Ocultar / Suspender" }).click();
    await expect(
      page.getByText(/Ocultar \/ Suspender Publicación/i),
    ).toBeVisible();
    await page.getByRole("button", { name: "Confirmar Suspensión" }).click();

    // Banner 409 presente
    await expect(
      page.getByText(/Conflicto de concurrencia detectado \(HTTP 409\)/i),
    ).toBeVisible();

    // 2.2 Desactivar conflicto y suspender exitosamente
    store.simulate409OnSpaceModeration = false;
    await page.getByRole("button", { name: "Confirmar Suspensión" }).click();

    // Publicación pasa a Oculta
    await expect(page.getByText("Oculta", { exact: true })).toBeVisible();
  });

  test("@phase9 flujo disputas (cierre irreversible) y cuentas (roles RBAC, bloqueo/desbloqueo, revocación de sesiones)", async ({
    page,
  }) => {
    const store = createAdminStore();
    await installMockGateway(page, store.handler);

    // --- PARTE 1: Disputas en /administracion/disputas ---
    await loginAndNavigate(page, "/administracion/disputas");
    await expect(
      page.getByRole("heading", {
        name: /Disputas, Reclamos y Moderación Operativa/i,
      }),
    ).toBeVisible();
    await expect(page.getByText("Abierta", { exact: true })).toBeVisible();

    // Cierre de disputa con confirmación irreversible
    await page
      .getByRole("button", { name: "Cerrar Disputa (Irreversible)" })
      .click();
    await expect(
      page.getByText(/Cierre Irreversible de Disputa/i),
    ).toBeVisible();

    await page
      .locator("#dispute-close-reason")
      .selectOption("ensayo_finalizado");
    await page
      .getByRole("button", { name: "Confirmar Cierre Irreversible" })
      .click();

    // Disputa pasa a cerrada
    await expect(page.getByText("Cerrada", { exact: true })).toBeVisible();

    // --- PARTE 2: Cuentas y Roles en /administracion/cuentas ---
    await page.getByRole("link", { name: "Cuentas y Roles" }).click();
    await page.waitForURL(/\/administracion\/cuentas$/);
    await expect(
      page.getByRole("heading", {
        name: /Gestión de Cuentas y Asignación de Roles/i,
      }),
    ).toBeVisible();
    await expect(page.getByText("usuario.prueba@espacigo.cl")).toBeVisible();

    // 2.1 Asignación de rol RBAC
    await page.getByRole("button", { name: "Modificar Roles" }).first().click();
    await expect(page.getByText(/Asignación de Roles RBAC/i)).toBeVisible();
    await page.locator("#target-role").selectOption("arrendador");
    await page.locator("#grant-action").selectOption("true");
    await page.getByRole("button", { name: "Guardar Rol" }).click();

    // 2.2 Bloqueo de usuario con motivo
    await page.getByRole("button", { name: "Bloquear" }).first().click();
    await expect(page.getByText(/Bloquear Cuenta de Usuario/i)).toBeVisible();
    await page
      .locator("#block-reason")
      .fill("Bloqueo administrativo por incumplimiento de términos.");
    await page.getByRole("button", { name: "Confirmar Bloqueo" }).click();

    // Badge bloqueada visible
    await expect(page.getByText("Bloqueada").first()).toBeVisible();

    // 2.3 Desbloqueo
    await page.getByRole("button", { name: "Desbloquear" }).first().click();
    await expect(
      page.getByText(/Desbloquear Cuenta de Usuario/i),
    ).toBeVisible();
    await page
      .locator("#block-reason")
      .fill("Desbloqueo tras aclaración y verificación satisfactoria.");
    await page.getByRole("button", { name: "Confirmar Desbloqueo" }).click();

    // Badge activo visible
    await expect(page.getByText("Activo", { exact: true })).toBeVisible();

    // 2.4 Revocación de sesiones
    await page
      .getByRole("button", { name: "Revocar Sesiones" })
      .first()
      .click();
    await expect(page.getByText(/Revocar Sesiones Activas/i)).toBeVisible();
    await page
      .getByRole("button", { name: "Confirmar Revocación Irreversible" })
      .click();
  });

  test("@phase9 flujo gates de seguridad: manejo de HTTP 428 y HTTP 403 con purga inmediata en memoria", async ({
    page,
  }) => {
    const store = createAdminStore();
    let stepUpMfaPayload: Record<string, unknown> | null = null;
    await installMockGateway(page, (route, request) => {
      const url = new URL(request.url());
      if (
        url.pathname === "/api/backend/admin/session/mfa" &&
        request.method() === "POST"
      ) {
        stepUpMfaPayload = JSON.parse(request.postData() ?? "{}");
      }
      return store.handler(route, request);
    });

    // --- GATE 428: Precondition Required (Reautenticación requerida + AdminMfaStepUpModal) ---
    store.simulate428OnPath = "/accounts/";
    await loginAndNavigate(page, "/administracion/cuentas");

    await page.getByRole("button", { name: "Bloquear" }).first().click();
    await page
      .locator("#block-reason")
      .fill("Bloqueo administrativo sensible que requiere auth reciente.");
    await page.getByRole("button", { name: "Confirmar Bloqueo" }).click();

    // Banner 428 y AdminMfaStepUpModal visibles
    await expect(
      page.getByText(/Reautenticación requerida \(HTTP 428\)/i),
    ).toBeVisible();
    await expect(
      page.getByText("Confirmar autenticación reciente (MFA)"),
    ).toBeVisible();

    // Completar step-up MFA en el modal y verificar cierre
    store.simulate428OnPath = null;
    await page.locator("#admin-stepup-code").fill("654321");
    await page.getByRole("button", { name: "Confirmar código" }).click();

    expect(stepUpMfaPayload).toEqual({ code: "654321" });
    await expect(
      page.getByText("Confirmar autenticación reciente (MFA)"),
    ).not.toBeVisible();
    await expect(
      page.getByText(/Reautenticación requerida \(HTTP 428\)/i),
    ).not.toBeVisible();

    // Cerrar modal de bloqueo tras step-up
    await page.getByRole("button", { name: "Cancelar" }).click();

    // --- GATE 403: Forbidden con purga inmediata de datos privados en memoria ---
    store.simulate428OnPath = null;
    store.simulate403OnPath = "/admin/global/accounts";

    // Disparar recarga que retorna 403
    await page.getByRole("button", { name: "Actualizar" }).click();

    // Mensaje de purga inmediata en banner de error
    await expect(
      page.getByText(
        /Acceso denegado o sesión revocada. Los datos privados fueron purgados de la memoria./i,
      ),
    ).toBeVisible();

    // Directorio queda purgado y sin datos en memoria
    await expect(page.getByText(/No se encontraron cuentas\./i)).toBeVisible();
  });

  test("@phase9 @a11y todas las rutas de administración cumplen accesibilidad Axe y viewport sin overflow", async ({
    page,
  }) => {
    const store = createAdminStore();
    await installMockGateway(page, store.handler);

    await loginAndNavigate(page);

    const targetRoutes = [
      "/administracion",
      "/administracion/verificaciones",
      "/administracion/espacios",
      "/administracion/disputas",
      "/administracion/cuentas",
      "/administracion/auditoria",
    ];

    const viewports = [
      { name: "desktop", width: 1365, height: 900 },
      { name: "mobile", width: 390, height: 844 },
    ];

    for (const vp of viewports) {
      await page.setViewportSize({ width: vp.width, height: vp.height });

      for (const route of targetRoutes) {
        await page.goto(route);
        await expect(
          page.getByRole("region", { name: "Área operativa" }),
        ).toBeVisible();

        // Verificar que no hay desbordamiento horizontal
        const hasOverflow = await page.evaluate(() => {
          return (
            document.documentElement.scrollWidth >
            document.documentElement.clientWidth
          );
        });
        expect(
          hasOverflow,
          `Horizontal overflow detected at ${route} on ${vp.name} (${vp.width}x${vp.height})`,
        ).toBe(false);

        // Auditoría Axe WCAG 2 AA
        const results = await new AxeBuilder({ page })
          .exclude("nextjs-portal")
          .analyze();

        expect(
          results.violations,
          `Axe violations found at ${route} on ${vp.name}: ${JSON.stringify(
            results.violations.map((v) => ({
              id: v.id,
              impact: v.impact,
              description: v.description,
            })),
            null,
            2,
          )}`,
        ).toEqual([]);
      }
    }
  });

  test("@phase9 vista asistida de solo lectura: iniciar desde Ficha 360, banner persistente, proyección y cierre", async ({
    page,
  }) => {
    const store = createAdminStore();
    const account = store.accounts[0]!;
    const grantId = "88888888-8888-4888-8888-888888888888";
    const expiresAt = new Date(Date.now() + 15 * 60_000).toISOString();
    let startPayload: Record<string, unknown> | null = null;
    let closedGrantId: string | null = null;

    await installMockGateway(page, (_route, request) => {
      const url = new URL(request.url());
      if (
        url.pathname ===
          `/api/backend/admin/global/accounts/${account.account_id}` &&
        request.method() === "GET"
      ) {
        return { body: { data: { ...account, email: "u••••l" } } };
      }
      if (
        url.pathname ===
          `/api/backend/admin/global/accounts/${account.account_id}/assisted-view` &&
        request.method() === "POST"
      ) {
        startPayload = JSON.parse(request.postData() ?? "{}");
        return {
          body: {
            data: {
              grant_id: grantId,
              account_id: account.account_id,
              actor_account_id: canonicalAdminId,
              role: startPayload?.role ?? "arrendador",
              case_id: startPayload?.case_id ?? "SUP-2026-2040",
              reason:
                startPayload?.reason ??
                "Revisión de reservas y contratos reportados en soporte",
              read_only: true,
              expires_at: expiresAt,
            },
          },
        };
      }
      if (
        url.pathname ===
          `/api/backend/admin/global/assisted-view/${grantId}/projection` &&
        request.method() === "GET"
      ) {
        return {
          body: {
            data: {
              grant_id: grantId,
              account_id: account.account_id,
              actor_account_id: canonicalAdminId,
              role: "arrendador",
              case_id: "SUP-2026-2040",
              expires_at: expiresAt,
              read_only: true,
              account_summary: {
                account_id: account.account_id,
                state: "activa",
                email_verified: true,
                phone_verified: true,
                kyc_status: "aprobado",
                roles: ["arrendador"],
                admin_blocked: false,
              },
              diagnostics: [
                {
                  code: "SPACE_PUBLISH_BLOCKER",
                  severity: "warning",
                  area: "spaces",
                  title: "Espacio con bloqueo de publicación",
                  detail: "Falta confirmar ubicación exacta del espacio.",
                },
              ],
              publish_guide: {
                kyc_status: "completed",
                draft_status: "completed",
                mercadopago_status: "completed",
                contract_status: "completed",
                steps: [
                  {
                    step: 1,
                    key: "kyc",
                    title: "Verificación de identidad",
                    status: "completed",
                    detail: "KYC aprobado.",
                  },
                  {
                    step: 2,
                    key: "draft",
                    title: "Borrador del espacio",
                    status: "completed",
                    detail: "Borrador completo.",
                  },
                  {
                    step: 3,
                    key: "mercadopago",
                    title: "Cuenta Mercado Pago",
                    status: "completed",
                    detail: "Cuenta conectada.",
                  },
                  {
                    step: 4,
                    key: "contract",
                    title: "Contrato publicado",
                    status: "completed",
                    detail: "Versión 1 activa.",
                  },
                ],
              },
              mercadopago_connection: {
                connected: true,
                connection_state: "connected",
                live_mode: false,
                connected_at: "2026-11-20T12:00:00Z",
                updated_at: "2026-11-21T12:00:00Z",
              },
              spaces: [
                {
                  id: "99999999-9999-4999-8999-999999999991",
                  title: "Bodega Central Providencia",
                  status: "borrador",
                  modality: "diaria",
                  price_clp: 45000,
                  location_confirmed: false,
                  contract_published_version: 1,
                  publish_blockers: ["Falta confirmar ubicación en mapa"],
                },
              ],
              reservations: [
                {
                  id: "99999999-9999-4999-8999-999999999992",
                  space_id: "99999999-9999-4999-8999-999999999991",
                  space_title: "Bodega Central Providencia",
                  status: "confirmada",
                  subtotal_clp: 45000,
                  start_at: "2026-11-25T10:00:00Z",
                  end_at: "2026-11-25T18:00:00Z",
                  created_at: "2026-11-24T10:00:00Z",
                },
              ],
              contracts: [
                {
                  id: "99999999-9999-4999-8999-999999999993",
                  space_id: "99999999-9999-4999-8999-999999999991",
                  space_title: "Bodega Central Providencia",
                  status: "firmado",
                  document_status: "emitido",
                  subtotal_clp: 45000,
                  created_at: "2026-11-24T09:00:00Z",
                },
              ],
              payments: [
                {
                  id: "99999999-9999-4999-8999-999999999994",
                  reservation_id: "99999999-9999-4999-8999-999999999992",
                  space_id: "99999999-9999-4999-8999-999999999991",
                  space_title: "Bodega Central Providencia",
                  status: "approved",
                  checkout_method: "mercadopago",
                  test_mode: true,
                  total_charged_clp: 45000,
                  provider_payment_status: "approved",
                  created_at: "2026-11-24T10:05:00Z",
                },
              ],
            },
          },
        };
      }
      if (
        url.pathname === `/api/backend/admin/global/assisted-view/${grantId}` &&
        request.method() === "DELETE"
      ) {
        closedGrantId = grantId;
        return { status: 204 };
      }
      return store.handler(_route, request);
    });

    await loginAndNavigate(page, "/administracion/cuentas");
    await page.getByRole("link", { name: "Abrir ficha 360" }).first().click();
    await expect(
      page.getByRole("heading", { name: account.display_name! }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Abrir vista del cliente" }).click();
    await expect(
      page.getByRole("heading", { name: /Abrir vista asistida/i }),
    ).toBeVisible();

    await page.locator(`#assisted-case-${account.account_id}`).fill("SUP-2026-2040");
    await page
      .locator(`#assisted-reason-${account.account_id}`)
      .fill("Revisión de reservas y contratos reportados en soporte");
    await page.getByRole("button", { name: "Iniciar vista asistida" }).click();

    expect(startPayload).toEqual({
      role: "arrendador",
      case_id: "SUP-2026-2040",
      reason: "Revisión de reservas y contratos reportados en soporte",
    });

    await expect(page).toHaveURL(
      new RegExp(`/administracion/cuentas/${account.account_id}/vista-asistida$`),
    );

    const banner = page.getByRole("complementary", {
      name: "Vista asistida activa",
    });
    await expect(banner).toBeVisible();
    await expect(
      banner.getByText(
        "Vista asistida · Solo lectura (acciones comerciales bloqueadas)",
      ),
    ).toBeVisible();
    await expect(banner.getByText("SUP-2026-2040")).toBeVisible();
    await expect(banner.getByText("Arrendador")).toBeVisible();

    await expect(
      page.getByRole("heading", { name: "Diagnóstico operativo (1)" }),
    ).toBeVisible();
    await expect(
      page.getByText("Espacio con bloqueo de publicación"),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Resumen de cuenta" }),
    ).toBeVisible();

    const assistedAxe = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa"])
      .analyze();
    expect(assistedAxe.violations).toEqual([]);

    await page.getByRole("tab", { name: "Guía y Mercado Pago" }).click();
    await expect(
      page.getByRole("heading", { name: "Conexión Mercado Pago" }),
    ).toBeVisible();
    await expect(page.getByText("Conectado")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Guía de publicación (4 pasos)" }),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Mis Espacios (1)" }).click();
    await expect(
      page.getByRole("heading", { name: "Mis Espacios (1)" }),
    ).toBeVisible();
    await expect(
      page.getByText("Bodega Central Providencia").first(),
    ).toBeVisible();
    await expect(
      page.getByText("Falta confirmar ubicación en mapa"),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Contratos recibidos (1)" }).click();
    await expect(
      page.getByRole("heading", { name: "Contratos recibidos (1)" }),
    ).toBeVisible();
    await expect(page.getByText("firmado")).toBeVisible();

    await page.getByRole("tab", { name: "Reservas y cobros (1)" }).click();
    await expect(
      page.getByRole("heading", { name: "Reservas (1)" }),
    ).toBeVisible();
    await expect(page.getByText("confirmada")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Cobros asociados (1)" }),
    ).toBeVisible();
    await expect(page.getByText("mercadopago")).toBeVisible();

    // Ensure zero mutating buttons exist inside the read-only projection main area
    const mainArea = page.getByRole("region", { name: "Área operativa" });
    await expect(mainArea.getByRole("button")).toHaveCount(0);

    await banner
      .getByRole("button", { name: "Cerrar vista del cliente" })
      .click();
    expect(closedGrantId).toBe(grantId);
    await expect(banner).toHaveCount(0);
    await expect(
      page.getByText(/No hay una vista asistida activa para esta cuenta/i),
    ).toBeVisible();
  });

  test("Phase 4 specialized operations: finance, privacy/KYC, and dual-control approvals + support @a11y", async ({
    page,
  }) => {
    const store = createAdminStore();
    const otherAdminId = "770e8400-e29b-41d4-a716-446655440000";
    let stepUpRequired = true;

    const payments = [
      {
        id: "pay-1",
        quote_id: "quote-1",
        space_id: "space-1",
        space_title: "Bodega Central Providencia",
        renter_id: canonicalUserAccountId,
        host_id: otherAdminId,
        status: "conciliado",
        checkout_method: "mercadopago_checkout_pro",
        test_mode: true,
        base_clp: 100000,
        commission_clp: 10000,
        iva_clp: 1900,
        buyer_fee_clp: 3500,
        total_charged_clp: 115400,
        provider_payment_id: "mp-sandbox-9001",
        provider_payment_status: "approved",
        reconciliation_reason: "Conciliación automática sandbox",
        payment_verified_at: "2026-11-20T10:05:00Z",
        created_at: "2026-11-20T10:00:00Z",
        updated_at: "2026-11-20T10:05:00Z",
      },
      {
        id: "pay-2",
        quote_id: "quote-2",
        space_id: "space-1",
        space_title: "Bodega Central Providencia",
        renter_id: canonicalUserAccountId,
        host_id: otherAdminId,
        status: "aprobado",
        checkout_method: "mercadopago_checkout_pro",
        test_mode: false,
        base_clp: 250000,
        commission_clp: 25000,
        iva_clp: 4750,
        buyer_fee_clp: 8250,
        total_charged_clp: 288000,
        provider_payment_id: "mp-prod-7788",
        provider_payment_status: "approved",
        reconciliation_reason: "Liquidación verificada",
        payment_verified_at: "2026-11-21T10:05:00Z",
        created_at: "2026-11-21T10:00:00Z",
        updated_at: "2026-11-21T10:05:00Z",
      },
    ];

    const reservations = [
      {
        id: "res-1",
        checkout_operation_id: "chk-1",
        quote_id: "quote-1",
        space_id: "space-1",
        space_title: "Bodega Central Providencia",
        renter_id: canonicalUserAccountId,
        host_id: otherAdminId,
        status: "confirmada",
        modality: "mensual",
        unit_price_clp: 100000,
        units: 1,
        subtotal_clp: 115400,
        currency: "CLP",
        start_at: "2026-12-01T09:00:00Z",
        end_at: "2026-12-31T18:00:00Z",
        created_at: "2026-11-20T09:00:00Z",
        updated_at: "2026-11-20T09:00:00Z",
      },
    ];

    const contracts = [
      {
        id: "ctr-1",
        proposal_id: "prop-1",
        quote_id: "quote-1",
        space_id: "space-1",
        space_title: "Bodega Central Providencia",
        owner_id: otherAdminId,
        renter_id: canonicalUserAccountId,
        status: "firmado",
        signing_mode: "fes_otp",
        proposal_version: 1,
        subtotal_clp: 100000,
        currency: "CLP",
        document_status: "sellado",
        created_at: "2026-11-20T09:00:00Z",
        updated_at: "2026-11-20T09:30:00Z",
      },
    ];

    const kycCases = [
      {
        id: "kyc-1",
        account_id: canonicalUserAccountId,
        environment: "comercial",
        status: "en_revision",
        id_country: "CL",
        decision_status: "pendiente",
        rejection_reason: "",
        verified_at: null,
        created_at: "2026-11-22T12:00:00Z",
        updated_at: "2026-11-22T12:00:00Z",
      },
    ];

    const privacyRequests = [
      {
        id: "priv-1",
        account_id: canonicalUserAccountId,
        right_type: "acceso",
        status: "pendiente",
        reason: "Solicitud de copia de datos personales",
        case_ref: "ARCO-2026-01",
        requested_by: canonicalAdminId,
        created_at: "2026-11-22T10:00:00Z",
        updated_at: "2026-11-22T10:00:00Z",
        resolved_at: null,
      },
    ];

    const supportTickets = [
      {
        id: "tkt-1",
        account_id: canonicalUserAccountId,
        category: "pago",
        priority: "alta",
        status: "abierto",
        subject: "Duda sobre comprobante de pago",
        case_ref: "CASE-9001",
        created_at: "2026-11-22T11:00:00Z",
        updated_at: "2026-11-22T11:00:00Z",
      },
    ];

    const approvals: Array<{
      id: string;
      action_type: string;
      resource_type: string;
      resource_id: string;
      requester_id: string;
      approver_id: string | null;
      status: string;
      reason: string;
      decision_reason: string;
      payload?: Record<string, unknown>;
      idempotency_key: string;
      version: number;
      created_at: string;
      decided_at: string | null;
      expires_at: string;
    }> = [
      {
        id: "app-self",
        action_type: "refund_request",
        resource_type: "payment",
        resource_id: "pay-1",
        requester_id: canonicalAdminId,
        approver_id: null,
        status: "pending",
        reason: "Reembolso solicitado por el mismo operador",
        decision_reason: "",
        payload: { amount_clp: "115400" },
        idempotency_key: "idem-app-self",
        version: 1,
        created_at: "2026-11-23T10:00:00Z",
        decided_at: null,
        expires_at: "2026-11-24T10:00:00Z",
      },
      {
        id: "app-other",
        action_type: "privacy_export",
        resource_type: "privacy_request",
        resource_id: "priv-1",
        requester_id: otherAdminId,
        approver_id: null,
        status: "pending",
        reason: "Exportación de datos titular Ley 21.719",
        decision_reason: "",
        idempotency_key: "idem-app-other",
        version: 1,
        created_at: "2026-11-23T11:00:00Z",
        decided_at: null,
        expires_at: "2026-11-24T11:00:00Z",
      },
    ];

    const pageEnvelope = <T>(items: T[]) => ({
      items,
      matching_total: items.length,
      generated_at: new Date().toISOString(),
    });

    await installMockGateway(page, (_route, request) => {
      const url = new URL(request.url());
      if (
        url.pathname === "/api/backend/admin/global/payments" &&
        request.method() === "GET"
      ) {
        const testModeParam = url.searchParams.get("test_mode");
        const statusParam = url.searchParams.get("status");
        const items = payments.filter((p) => {
          if (testModeParam !== null && String(p.test_mode) !== testModeParam) {
            return false;
          }
          if (statusParam && p.status !== statusParam) {
            return false;
          }
          return true;
        });
        return { status: 200, body: { data: pageEnvelope(items) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/reservations" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(reservations) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/contracts" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(contracts) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/kyc-cases" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(kycCases) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/privacy-requests" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(privacyRequests) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/privacy-requests" &&
        request.method() === "POST"
      ) {
        const payload = request.postDataJSON() as Record<string, string>;
        const created = {
          id: "priv-2",
          account_id: payload.account_id,
          right_type: payload.right_type,
          status: "pendiente",
          reason: payload.reason,
          case_ref: payload.case_ref ?? "",
          requested_by: canonicalAdminId,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          resolved_at: null,
        };
        privacyRequests.unshift(created);
        return { status: 201, body: { data: created } };
      }
      if (
        url.pathname === "/api/backend/admin/global/support-tickets" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(supportTickets) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/support-tickets" &&
        request.method() === "POST"
      ) {
        const payload = request.postDataJSON() as Record<string, string>;
        const created = {
          id: "tkt-2",
          account_id: payload.account_id,
          category: payload.category,
          priority: payload.priority,
          status: "abierto",
          subject: payload.subject,
          case_ref: payload.case_ref ?? "",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        };
        supportTickets.unshift(created);
        return { status: 201, body: { data: created } };
      }
      if (
        url.pathname === "/api/backend/admin/global/approvals" &&
        request.method() === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(approvals) } };
      }
      if (
        url.pathname === "/api/backend/admin/global/approvals" &&
        request.method() === "POST"
      ) {
        const payload = request.postDataJSON() as {
          action_type: string;
          resource_type: string;
          resource_id: string;
          reason: string;
          idempotency_key: string;
          payload?: Record<string, unknown>;
        };
        const created = {
          id: "app-new",
          action_type: payload.action_type,
          resource_type: payload.resource_type,
          resource_id: payload.resource_id,
          requester_id: canonicalAdminId,
          approver_id: null,
          status: "pending",
          reason: payload.reason,
          decision_reason: "",
          payload: payload.payload,
          idempotency_key: payload.idempotency_key,
          version: 1,
          created_at: new Date().toISOString(),
          decided_at: null,
          expires_at: new Date(Date.now() + 86400000).toISOString(),
        };
        approvals.unshift(created);
        return { status: 201, body: { data: created } };
      }
      const decisionMatch = url.pathname.match(
        /^\/api\/backend\/admin\/global\/approvals\/([^/]+)\/decisions$/,
      );
      if (decisionMatch && request.method() === "POST") {
        if (stepUpRequired) {
          stepUpRequired = false;
          return {
            status: 428,
            body: {
              error: {
                code: "RECENT_AUTH_REQUIRED",
                message: "Se requiere autenticación MFA reciente",
              },
            },
          };
        }
        const approvalId = decisionMatch[1];
        const payload = request.postDataJSON() as {
          decision: "approved" | "rejected" | "cancelled";
          decision_reason: string;
          expected_version: number;
        };
        const target = approvals.find((a) => a.id === approvalId);
        if (!target) return { status: 404, body: {} };
        target.status = payload.decision;
        target.decision_reason = payload.decision_reason;
        target.approver_id = canonicalAdminId;
        target.decided_at = new Date().toISOString();
        target.version += 1;
        return { status: 200, body: { data: target } };
      }
      return store.handler(_route, request);
    });

    // 1. /administracion/finanzas
    await loginAndNavigate(page, "/administracion");
    await page.getByRole("link", { name: /Finanzas y Reservas/i }).click();
    await expect(page).toHaveURL(/\/administracion\/finanzas$/);
    await expect(
      page.getByRole("heading", { name: "Finanzas, Reservas y Contratos" }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="badge"]', { hasText: "Prueba MP" }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="badge"]', { hasText: "Comercial" }),
    ).toBeVisible();
    await expect(page.getByText("mp-sandbox-9001")).toBeVisible();
    await expect(page.getByText("Conciliación automática sandbox")).toBeVisible();

    const financeAxe = await new AxeBuilder({ page })
      .include("main")
      .analyze();
    expect(financeAxe.violations).toEqual([]);

    await page.getByRole("tab", { name: /Reservas/i }).click();
    await expect(page.getByText("Bodega Central Providencia")).toBeVisible();

    await page.getByRole("tab", { name: /Contratos/i }).click();
    await expect(page.getByText("ctr-1")).toBeVisible();

    // 2. /administracion/privacidad
    await page.getByRole("link", { name: /Privacidad y KYC/i }).click();
    await expect(page).toHaveURL(/\/administracion\/privacidad$/);
    await expect(
      page.getByRole("heading", {
        name: "Privacidad ARCO+ y Verificaciones KYC",
      }),
    ).toBeVisible();
    await expect(
      page.getByText("Solicitud de copia de datos personales"),
    ).toBeVisible();
    await expect(page.getByText("en_revision")).toBeVisible();

    const privacyAxe = await new AxeBuilder({ page })
      .include("main")
      .analyze();
    expect(privacyAxe.violations).toEqual([]);

    // 3. /administracion/aprobaciones
    await page.getByRole("link", { name: /Aprobaciones y Soporte/i }).click();
    await expect(page).toHaveURL(/\/administracion\/aprobaciones$/);
    await expect(
      page.getByRole("heading", {
        name: "Aprobaciones de Doble Control y Soporte",
      }),
    ).toBeVisible();
    await expect(
      page.getByText(
        "Auto-aprobación bloqueada: el solicitante no puede aprobar su propia solicitud.",
      ),
    ).toBeVisible();
    await expect(
      page.getByText("Duda sobre comprobante de pago"),
    ).toBeVisible();

    // Trigger 428 MFA step-up on decision for app-other
    await page
      .locator("#decision-reason-app-other")
      .fill("Aprobado tras validar respaldo de titularidad");
    await page.getByRole("button", { name: "Aprobar" }).first().click();

    await expect(
      page.getByRole("dialog", {
        name: /Confirmar autenticación reciente \(MFA\)/i,
      }),
    ).toBeVisible();
    await page.getByLabel("Código TOTP o de respaldo").fill("654321");
    await page.getByRole("button", { name: "Confirmar código" }).click();
    await expect(
      page.getByRole("dialog", {
        name: /Confirmar autenticación reciente \(MFA\)/i,
      }),
    ).toHaveCount(0);

    // Retry decision after MFA step-up
    await page.getByRole("button", { name: "Aprobar" }).first().click();
    await expect(page.getByText("approved")).toBeVisible();

    const approvalsAxe = await new AxeBuilder({ page })
      .include("main")
      .analyze();
    expect(approvalsAxe.violations).toEqual([]);
  });
});
