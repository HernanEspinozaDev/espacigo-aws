import {
  expect,
  test,
  type Page,
  type Request,
  type Route,
} from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";
import type {
  AdminCapability,
  AdminRole,
  GlobalAdminAccount,
  GlobalAdminOverview,
  GlobalAdminPrivacyIncidentDetail,
  GlobalAdminPrivacyLegalHold,
  GlobalAdminPrivacyRequest,
  GlobalAdminPrivacyRequestDetail,
} from "@/features/admin/domain/types";

test.use({ trace: "off", screenshot: "off", video: "off" });

interface MockResult {
  status?: number;
  body?: unknown;
  contentType?: string;
  headers?: Record<string, string>;
}

type MockHandler = (
  route: Route,
  request: Request,
) => Promise<MockResult | null | undefined> | MockResult | null | undefined;

const canonicalAdminId = "990e8400-e29b-41d4-a716-446655440000";
const canonicalUserAccountId = "110e8400-e29b-41d4-a716-446655440000";
const portabilityRequestId = "440e8400-e29b-41d4-a716-446655440001";
const createdIncidentId = "550e8400-e29b-41d4-a716-446655440001";
const createdLegalHoldId = "660e8400-e29b-41d4-a716-446655440001";
const sampleSha256 =
  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855";

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

function pageEnvelope<T>(items: readonly T[]) {
  return {
    items,
    matching_total: items.length,
    generated_at: "2026-11-27T12:00:00Z",
  };
}

interface PrivacyGovernanceStore {
  accounts: GlobalAdminAccount[];
  privacyRequests: GlobalAdminPrivacyRequest[];
  privacyDetails: Record<string, GlobalAdminPrivacyRequestDetail>;
  incidents: GlobalAdminPrivacyIncidentDetail[];
  legalHolds: GlobalAdminPrivacyLegalHold[];
  handler: MockHandler;
}

function createPrivacyGovernanceStore(options?: {
  privacyRequests?: GlobalAdminPrivacyRequest[];
  privacyDetails?: Record<string, GlobalAdminPrivacyRequestDetail>;
  incidents?: GlobalAdminPrivacyIncidentDetail[];
  legalHolds?: GlobalAdminPrivacyLegalHold[];
}): PrivacyGovernanceStore {
  let authenticated = false;
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

  const overview: GlobalAdminOverview = {
    total_accounts: 12,
    connected_accounts: 4,
    administrative_blocked: 0,
    by_state: { activo: 12 },
    by_role: { arrendatario: 8, arrendador: 3, administrador: 1 },
    generated_at: "2026-11-27T12:00:00Z",
    definition: "Resumen global de métricas del sistema",
  };

  const accounts: GlobalAdminAccount[] = [
    {
      account_id: canonicalUserAccountId,
      email: "u••••l@espacigo.cl",
      display_name: "Usuario Prueba",
      phone: "+56912345678",
      profile_updated_at: "2026-11-20T10:00:00Z",
      state: "activo",
      created_at: "2026-10-01T10:00:00Z",
      administrative_blocked: false,
      roles: ["arrendatario"],
      publication_count: 1,
      reservation_count: 2,
      incident_count: 0,
    },
  ];

  const store: PrivacyGovernanceStore = {
    accounts,
    privacyRequests: options?.privacyRequests ?? [],
    privacyDetails: options?.privacyDetails ?? {},
    incidents: options?.incidents ?? [],
    legalHolds: options?.legalHolds ?? [],
    handler: (_route, request) => {
      const url = new URL(request.url());
      const { pathname, searchParams } = url;
      const method = request.method();

      if (pathname === "/api/backend/auth/login" && method === "POST") {
        authenticated = true;
        return { status: 200, body: session };
      }

      if (pathname === "/api/backend/auth/session" && method === "GET") {
        return authenticated
          ? { status: 200, body: session }
          : { status: 401, body: { code: "unauthenticated" } };
      }

      if (pathname === "/api/backend/admin/session" && method === "GET") {
        return { status: 200, body: { data: adminSession } };
      }

      if (pathname === "/api/backend/admin/global/me" && method === "GET") {
        return {
          status: 200,
          body: {
            data: {
              account_id: canonicalAdminId,
              roles: ["administrador_global"],
              capabilities: ADMIN_CAPABILITIES,
              generated_at: "2026-11-27T12:00:00Z",
            },
          },
        };
      }

      if (
        pathname === "/api/backend/admin/global/overview" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: overview } };
      }

      if (
        pathname === "/api/backend/admin/global/kyc-cases" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope([]) } };
      }

      const accountMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/accounts\/([^/]+)$/,
      );
      if (accountMatch && method === "GET") {
        const accountId = accountMatch[1];
        const found = store.accounts.find((a) => a.account_id === accountId);
        return found
          ? { status: 200, body: { data: found } }
          : { status: 404, body: { error: { code: "not_found" } } };
      }

      if (
        pathname === "/api/backend/admin/global/privacy-requests" &&
        method === "GET"
      ) {
        const accountFilter = searchParams.get("account_id");
        const items = accountFilter
          ? store.privacyRequests.filter(
              (item) => item.account_id === accountFilter,
            )
          : store.privacyRequests;
        return { status: 200, body: { data: pageEnvelope(items) } };
      }

      const privacyDetailMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-requests\/([^/]+)$/,
      );
      if (privacyDetailMatch && method === "GET") {
        const requestId = privacyDetailMatch[1];
        const detail = store.privacyDetails[requestId];
        return detail
          ? { status: 200, body: { data: detail } }
          : { status: 404, body: { error: { code: "not_found" } } };
      }

      const retryMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-requests\/([^/]+)\/notifications\/retry$/,
      );
      if (retryMatch && method === "POST") {
        const requestId = retryMatch[1];
        const detail = store.privacyDetails[requestId];
        if (!detail) {
          return { status: 404, body: { error: { code: "not_found" } } };
        }
        const updated: GlobalAdminPrivacyRequestDetail = {
          ...detail,
          notification_summary: {
            pending: 1,
            delivered: detail.notification_summary?.delivered ?? 0,
            terminal: 0,
            last_error_code: null,
            last_delivered_at: detail.notification_summary?.last_delivered_at,
          },
          updated_at: new Date().toISOString(),
        };
        store.privacyDetails[requestId] = updated;
        store.privacyRequests = store.privacyRequests.map((item) =>
          item.id === requestId ? { ...item, updated_at: updated.updated_at } : item,
        );
        return { status: 200, body: { data: updated } };
      }

      const executionMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-requests\/([^/]+)\/executions$/,
      );
      if (executionMatch && method === "POST") {
        const requestId = executionMatch[1];
        const detail = store.privacyDetails[requestId];
        if (!detail) {
          return { status: 404, body: { error: { code: "not_found" } } };
        }
        const payload = JSON.parse(request.postData() ?? "{}") as {
          action: string;
          summary: string;
          target_reference?: string;
        };
        const now = new Date().toISOString();
        const newExecution = {
          id: "exec-port-01",
          request_id: requestId,
          account_id: detail.account_id,
          execution_type: "portabilidad_emitida" as const,
          status: "ejecutada" as const,
          scope: "tratamiento_general",
          summary: payload.summary,
          target_reference: payload.target_reference,
          artifact_sha256: sampleSha256,
          executed_by: canonicalAdminId,
          executed_at: now,
        };
        const updated: GlobalAdminPrivacyRequestDetail = {
          ...detail,
          version: (detail.version ?? 1) + 1,
          updated_at: now,
          executions: [newExecution, ...(detail.executions ?? [])],
          events: [
            ...detail.events,
            {
              id: `evt-${detail.events.length + 1}`,
              sequence: detail.events.length + 1,
              event_type: "ejecucion_derecho",
              visibility: "titular",
              detail: payload.summary,
              created_at: now,
            },
          ],
        };
        store.privacyDetails[requestId] = updated;
        store.privacyRequests = store.privacyRequests.map((item) =>
          item.id === requestId
            ? { ...item, version: updated.version, updated_at: now }
            : item,
        );
        return { status: 200, body: { data: updated } };
      }

      if (
        pathname === "/api/backend/admin/global/privacy-incidents" &&
        method === "GET"
      ) {
        return { status: 200, body: { data: pageEnvelope(store.incidents) } };
      }

      if (
        pathname === "/api/backend/admin/global/privacy-incidents" &&
        method === "POST"
      ) {
        const payload = JSON.parse(request.postData() ?? "{}") as {
          title: string;
          severity: GlobalAdminPrivacyIncidentDetail["severity"];
          data_categories: string[];
          affected_systems: string[];
          estimated_subjects?: number;
          summary: string;
          requires_agency_notice?: boolean;
          requires_subject_notice?: boolean;
          leader_id?: string;
        };
        const now = new Date().toISOString();
        const created: GlobalAdminPrivacyIncidentDetail = {
          id: createdIncidentId,
          folio: "INC-2026-0001",
          title: payload.title,
          severity: payload.severity,
          status: "abierto",
          data_categories: payload.data_categories,
          affected_systems: payload.affected_systems,
          estimated_subjects: payload.estimated_subjects ?? 0,
          summary: payload.summary,
          requires_agency_notice: Boolean(payload.requires_agency_notice),
          requires_subject_notice: Boolean(payload.requires_subject_notice),
          leader_id: payload.leader_id ?? canonicalAdminId,
          detected_at: now,
          created_by: canonicalAdminId,
          created_at: now,
          updated_at: now,
          version: 1,
          events: [
            {
              id: "inc-evt-1",
              sequence: 1,
              event_type: "apertura",
              actor_id: canonicalAdminId,
              detail: payload.summary,
              created_at: now,
            },
          ],
        };
        store.incidents = [created, ...store.incidents];
        return { status: 201, body: { data: created } };
      }

      const incidentDetailMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-incidents\/([^/]+)$/,
      );
      if (incidentDetailMatch && method === "GET") {
        const incidentId = incidentDetailMatch[1];
        const found = store.incidents.find((item) => item.id === incidentId);
        return found
          ? { status: 200, body: { data: found } }
          : { status: 404, body: { error: { code: "not_found" } } };
      }

      const incidentTransitionMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-incidents\/([^/]+)\/transitions$/,
      );
      if (incidentTransitionMatch && method === "POST") {
        const incidentId = incidentTransitionMatch[1];
        const idx = store.incidents.findIndex((item) => item.id === incidentId);
        if (idx < 0) {
          return { status: 404, body: { error: { code: "not_found" } } };
        }
        const payload = JSON.parse(request.postData() ?? "{}") as {
          action: string;
          expected_version: number;
          detail?: string;
          agency_reference?: string;
        };
        const current = store.incidents[idx];
        const now = new Date().toISOString();
        const updated: GlobalAdminPrivacyIncidentDetail = {
          ...current,
          status: payload.action === "notify_agency" ? "notificado" : current.status,
          agency_notified_at:
            payload.action === "notify_agency" ? now : current.agency_notified_at,
          agency_reference:
            payload.agency_reference ?? current.agency_reference,
          updated_at: now,
          version: current.version + 1,
          events: [
            ...current.events,
            {
              id: `inc-evt-${current.events.length + 1}`,
              sequence: current.events.length + 1,
              event_type:
                payload.action === "notify_agency"
                  ? "notificacion_agencia"
                  : "nota",
              actor_id: canonicalAdminId,
              detail:
                payload.detail ??
                `Notificación cursada (${payload.agency_reference ?? ""})`,
              created_at: now,
            },
          ],
        };
        store.incidents[idx] = updated;
        return { status: 200, body: { data: updated } };
      }

      if (
        pathname === "/api/backend/admin/global/privacy-legal-holds" &&
        method === "GET"
      ) {
        const accountFilter = searchParams.get("account_id");
        const items = accountFilter
          ? store.legalHolds.filter((item) => item.account_id === accountFilter)
          : store.legalHolds;
        return { status: 200, body: { data: pageEnvelope(items) } };
      }

      if (
        pathname === "/api/backend/admin/global/privacy-legal-holds" &&
        method === "POST"
      ) {
        const payload = JSON.parse(request.postData() ?? "{}") as {
          scope: GlobalAdminPrivacyLegalHold["scope"];
          authority: string;
          legal_basis: string;
          account_id?: string;
          request_id?: string;
          incident_id?: string;
        };
        const now = new Date().toISOString();
        const created: GlobalAdminPrivacyLegalHold = {
          id: createdLegalHoldId,
          folio: "LH-2026-0001",
          scope: payload.scope,
          authority: payload.authority,
          legal_basis: payload.legal_basis,
          account_id: payload.account_id ?? null,
          request_id: payload.request_id ?? null,
          incident_id: payload.incident_id ?? null,
          status: "activa",
          applied_by: canonicalAdminId,
          applied_at: now,
        };
        store.legalHolds = [created, ...store.legalHolds];
        return { status: 201, body: { data: created } };
      }

      const releaseHoldMatch = pathname.match(
        /^\/api\/backend\/admin\/global\/privacy-legal-holds\/([^/]+)\/release$/,
      );
      if (releaseHoldMatch && method === "POST") {
        const holdId = releaseHoldMatch[1];
        const idx = store.legalHolds.findIndex((item) => item.id === holdId);
        if (idx < 0) {
          return { status: 404, body: { error: { code: "not_found" } } };
        }
        const payload = JSON.parse(request.postData() ?? "{}") as {
          release_reason: string;
        };
        const now = new Date().toISOString();
        const updated: GlobalAdminPrivacyLegalHold = {
          ...store.legalHolds[idx],
          status: "liberada",
          released_by: canonicalAdminId,
          released_at: now,
          release_reason: payload.release_reason,
        };
        store.legalHolds[idx] = updated;
        return { status: 200, body: { data: updated } };
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

async function loginAndNavigate(page: Page, destination: string) {
  const [pathname] = destination.split("?");
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
    await page.waitForURL((url) =>
      url.pathname.startsWith("/administracion"),
    );
    if (new URL(page.url()).pathname !== pathname) {
      await page.goto(destination);
    }
  }
}

test.describe("Gobernanza ARCO+ Fase 4, Fase 6.2 y Fase 7", () => {
  test("Fase 4 + Outbox en /administracion/privacidad: reintentar notificación y emitir portabilidad con SHA-256", async ({
    page,
  }) => {
    const baseRequest: GlobalAdminPrivacyRequest = {
      id: portabilityRequestId,
      account_id: canonicalUserAccountId,
      folio: "PRIV-2026-PORT01",
      right_type: "portabilidad",
      status: "en_revision",
      reason: "Solicitud de copia estructurada del historial de reservas",
      case_ref: "ARCO-PORT-01",
      channel: "web",
      requester_type: "titular",
      requester_name: "Usuario Prueba",
      version: 1,
      can_publish_response: true,
      created_at: "2026-11-20T10:00:00Z",
      updated_at: "2026-11-20T10:00:00Z",
      resolved_at: null,
    };

    const baseDetail: GlobalAdminPrivacyRequestDetail = {
      ...baseRequest,
      notification_summary: {
        pending: 0,
        delivered: 0,
        terminal: 1,
        last_error_code: "smtp_delivery_failed",
        last_delivered_at: null,
      },
      events: [
        {
          id: "evt-port-1",
          sequence: 1,
          event_type: "recepcion",
          visibility: "titular",
          state_to: "pendiente",
          detail: "Solicitud ingresada por canal web",
          created_at: "2026-11-20T10:00:00Z",
        },
      ],
      executions: [],
      treatment_blocks: [],
      purpose_preferences: [],
    };

    const store = createPrivacyGovernanceStore({
      privacyRequests: [baseRequest],
      privacyDetails: { [portabilityRequestId]: baseDetail },
    });

    await installMockGateway(page, (route, request) =>
      store.handler(route, request),
    );

    await loginAndNavigate(page, "/administracion/privacidad");

    await expect(
      page.getByText("PRIV-2026-PORT01", { exact: true }).first(),
    ).toBeVisible();

    await page.getByRole("button", { name: "Gestionar expediente" }).click();

    await expect(page.getByText("Estado de notificaciones")).toBeVisible();
    await expect(page.getByText("smtp_delivery_failed")).toBeVisible();

    const retryResponsePromise = page.waitForResponse(
      (res) =>
        res
          .url()
          .includes(
            `/api/backend/admin/global/privacy-requests/${portabilityRequestId}/notifications/retry`,
          ) && res.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Reintentar notificación" }).click();
    const retryResponse = await retryResponsePromise;
    expect(retryResponse.status()).toBe(200);

    await expect(
      page.getByText("Pendientes: 1", { exact: false }),
    ).toBeVisible();
    await expect(
      page.getByText("Fallidas/Terminales: 0", { exact: false }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Reintentar notificación" }),
    ).toHaveCount(0);

    await expect(
      page.getByText("Ejecución efectiva de derechos (Fase 4)"),
    ).toBeVisible();
    await expect(
      page.locator(`#privacy-exec-action-${portabilityRequestId}`),
    ).toHaveValue("issue_portability");

    await page
      .locator(`#privacy-exec-target-ref-${portabilityRequestId}`)
      .fill("portal_titular_descarga");
    await page
      .locator(`#privacy-exec-summary-${portabilityRequestId}`)
      .fill("Paquete de portabilidad emitido y verificado con SHA-256");

    let capturedExecutionPayload: Record<string, unknown> | null = null;
    const executionRequestPromise = page.waitForRequest((req) => {
      if (
        req
          .url()
          .includes(
            `/api/backend/admin/global/privacy-requests/${portabilityRequestId}/executions`,
          ) &&
        req.method() === "POST"
      ) {
        capturedExecutionPayload = JSON.parse(
          req.postData() ?? "{}",
        ) as Record<string, unknown>;
        return true;
      }
      return false;
    });
    const executionResponsePromise = page.waitForResponse(
      (res) =>
        res
          .url()
          .includes(
            `/api/backend/admin/global/privacy-requests/${portabilityRequestId}/executions`,
          ) && res.request().method() === "POST",
    );

    await page.getByRole("button", { name: "Ejecutar derecho" }).click();

    await executionRequestPromise;
    const executionResponse = await executionResponsePromise;
    expect(executionResponse.status()).toBe(200);
    expect(capturedExecutionPayload).toMatchObject({
      action: "issue_portability",
      expected_version: 1,
      summary: "Paquete de portabilidad emitido y verificado con SHA-256",
      target_reference: "portal_titular_descarga",
    });

    await expect(page.getByText("Ejecuciones registradas")).toBeVisible();
    await expect(page.getByText("portabilidad_emitida")).toBeVisible();
    await expect(
      page.getByText(`${sampleSha256.slice(0, 16)}…`),
    ).toBeVisible();
  });

  test("Fase 7 Incidentes y Retención Legal en /administracion/privacidad", async ({
    page,
  }) => {
    const store = createPrivacyGovernanceStore();

    await installMockGateway(page, (route, request) =>
      store.handler(route, request),
    );

    await loginAndNavigate(page, "/administracion/privacidad");

    await page
      .getByRole("button", { name: /Incidentes y Retención Legal \(Fase 7\)/i })
      .click();
    await expect(page.locator("#privacy-phase7-content")).toBeVisible();

    // 1. Registrar un incidente
    await page.getByRole("button", { name: "Registrar incidente" }).click();
    await page
      .locator("#incident-create-title")
      .fill("Exposición temporal de registros KYC");
    await page.locator("#incident-create-severity").selectOption("alta");
    await page
      .locator("#incident-create-categories")
      .fill("identificacion, contacto");
    await page.locator("#incident-create-systems").fill("api-core, s3-kyc");
    await page.locator("#incident-create-subjects").fill("14");
    await page
      .locator("#incident-create-summary")
      .fill("Acceso anómalo contenido tras rotación inmediata de llaves.");
    await page.locator("#incident-create-agency-notice").check();

    const createIncidentPromise = page.waitForResponse(
      (res) =>
        res.url().endsWith("/api/backend/admin/global/privacy-incidents") &&
        res.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Confirmar registro de incidente" })
      .click();
    const createIncidentRes = await createIncidentPromise;
    expect(createIncidentRes.status()).toBe(201);

    await expect(page.getByText("INC-2026-0001")).toBeVisible();
    await expect(
      page.getByText("Exposición temporal de registros KYC"),
    ).toBeVisible();

    // 2. Expandir detalle y ejecutar transición notify_agency
    await page.getByRole("button", { name: "Ver detalle" }).click();
    await expect(page.getByText("Transicionar incidente")).toBeVisible();

    await page
      .locator(`#incident-transition-action-${createdIncidentId}`)
      .selectOption("notify_agency");
    await page
      .locator(`#incident-agency-ref-${createdIncidentId}`)
      .fill("AGPD-2026-0091");
    await page
      .locator(`#incident-transition-detail-${createdIncidentId}`)
      .fill("Notificación formal remitida a la Agencia dentro de plazo.");

    let capturedTransitionPayload: Record<string, unknown> | null = null;
    const transitionReqPromise = page.waitForRequest((req) => {
      if (
        req
          .url()
          .includes(
            `/api/backend/admin/global/privacy-incidents/${createdIncidentId}/transitions`,
          ) &&
        req.method() === "POST"
      ) {
        capturedTransitionPayload = JSON.parse(
          req.postData() ?? "{}",
        ) as Record<string, unknown>;
        return true;
      }
      return false;
    });
    const transitionResPromise = page.waitForResponse(
      (res) =>
        res
          .url()
          .includes(
            `/api/backend/admin/global/privacy-incidents/${createdIncidentId}/transitions`,
          ) && res.request().method() === "POST",
    );

    await page.getByRole("button", { name: "Ejecutar transición" }).click();
    await transitionReqPromise;
    const transitionRes = await transitionResPromise;
    expect(transitionRes.status()).toBe(200);
    expect(capturedTransitionPayload).toMatchObject({
      action: "notify_agency",
      expected_version: 1,
      agency_reference: "AGPD-2026-0091",
    });

    await expect(page.getByText("notificado", { exact: true })).toBeVisible();
    await expect(page.getByText(/Notificada \(AGPD-2026-0091\)/)).toBeVisible();

    // 3. Aplicar retención legal y luego liberarla
    await page.getByRole("button", { name: "Aplicar retención legal" }).click();
    await page.locator("#legal-hold-scope").selectOption("cuenta_completa");
    await page
      .locator("#legal-hold-authority")
      .fill("Juzgado Civil de Santiago");
    await page
      .locator("#legal-hold-basis")
      .fill("Resolución Rol C-1024-2026 ordena conservación de antecedentes");
    await page.locator("#legal-hold-account-id").fill(canonicalUserAccountId);

    const createHoldResPromise = page.waitForResponse(
      (res) =>
        res.url().endsWith("/api/backend/admin/global/privacy-legal-holds") &&
        res.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Confirmar retención legal" })
      .click();
    const createHoldRes = await createHoldResPromise;
    expect(createHoldRes.status()).toBe(201);

    await expect(page.getByText("LH-2026-0001")).toBeVisible();
    await expect(page.getByText("activa", { exact: true })).toBeVisible();

    await page
      .locator(`#legal-hold-release-reason-${createdLegalHoldId}`)
      .fill("Resolución judicial firme ordena levantar la retención");

    const releaseHoldResPromise = page.waitForResponse(
      (res) =>
        res
          .url()
          .includes(
            `/api/backend/admin/global/privacy-legal-holds/${createdLegalHoldId}/release`,
          ) && res.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Liberar retención" }).click();
    const releaseHoldRes = await releaseHoldResPromise;
    expect(releaseHoldRes.status()).toBe(200);

    await expect(page.getByText(/Liberada:/)).toBeVisible();
    await expect(page.getByText("liberada", { exact: true })).toBeVisible();
  });

  test("Fase 6.2 Ficha 360: pestaña Privacidad y ARCO+ carga solicitudes y retenciones legales de la cuenta", async ({
    page,
  }) => {
    const accountRequest: GlobalAdminPrivacyRequest = {
      id: "440e8400-e29b-41d4-a716-446655440099",
      account_id: canonicalUserAccountId,
      folio: "PRIV-2026-F360",
      right_type: "portabilidad",
      status: "en_revision",
      reason: "Solicitud vinculada desde Ficha 360",
      case_ref: "ARCO-360-01",
      channel: "web",
      due_at: "2026-12-10T12:00:00Z",
      is_overdue: false,
      created_at: "2026-11-22T10:00:00Z",
      updated_at: "2026-11-22T10:00:00Z",
    };

    const accountHold: GlobalAdminPrivacyLegalHold = {
      id: "660e8400-e29b-41d4-a716-446655440099",
      folio: "LH-2026-F360",
      account_id: canonicalUserAccountId,
      scope: "cuenta_completa",
      authority: "Tribunal de Garantía de Santiago",
      legal_basis: "Medida cautelar vigente en investigación administrativa",
      status: "activa",
      applied_by: canonicalAdminId,
      applied_at: "2026-11-23T09:30:00Z",
    };

    const store = createPrivacyGovernanceStore({
      privacyRequests: [accountRequest],
      legalHolds: [accountHold],
    });

    await installMockGateway(page, (route, request) =>
      store.handler(route, request),
    );

    await loginAndNavigate(
      page,
      `/administracion/cuentas/${canonicalUserAccountId}`,
    );

    await expect(
      page.getByRole("heading", { name: "Usuario Prueba" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Privacidad y ARCO+" }).click();

    await expect(
      page.getByRole("heading", { name: "Privacidad y ARCO+ (Ley 21.719)" }),
    ).toBeVisible();

    const privacyLink = page.getByRole("link", {
      name: /Ir a gestión completa de privacidad y retenciones/i,
    });
    await expect(privacyLink).toBeVisible();
    await expect(privacyLink).toHaveAttribute(
      "href",
      "/administracion/privacidad",
    );

    await expect(
      page.getByRole("heading", {
        name: "Solicitudes ARCO+ vinculadas a la cuenta",
      }),
    ).toBeVisible();
    await expect(page.getByText("PRIV-2026-F360")).toBeVisible();
    await expect(page.getByText("Derecho: Portabilidad")).toBeVisible();
    await expect(page.getByText("En revisión")).toBeVisible();

    await expect(
      page.getByRole("heading", {
        name: "Retenciones legales (Legal Hold) de la cuenta",
      }),
    ).toBeVisible();
    await expect(page.getByText("LH-2026-F360")).toBeVisible();
    await expect(page.getByText("Ámbito: Cuenta completa")).toBeVisible();
    await expect(
      page.locator('[data-slot="badge"]', { hasText: /^Activa$/ }),
    ).toBeVisible();
    await expect(
      page.getByText("Tribunal de Garantía de Santiago"),
    ).toBeVisible();
  });
});
