"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { adminFacade } from "../admin-runtime";
import type { AdminSnapshot } from "../application/admin-facade";
import { canAccessSection, hasCapability } from "../domain/admin-rules";
import type {
  AdminAssistedViewGrant,
  AdminAssistedViewProjection,
  AdminAssistedViewStartInput,
  AdminCapability,
  AdminDamageClaimResolutionInput,
  AdminRole,
  AdminSection,
  AdminSensitiveAccessInput,
  AdminSensitiveAccessResult,
  AdminSessionStartResponse,
  AuditEventsQuery,
  GlobalAdminAccountQuery,
  GlobalAdminAccountResourceKind,
  GlobalAdminAccountResourcePage,
  GlobalAdminAccountResourceQuery,
  GlobalAdminPublicationQuery,
  GlobalAdminPayment,
  GlobalAdminPaymentQuery,
  GlobalAdminReservation,
  GlobalAdminReservationQuery,
  GlobalAdminContract,
  GlobalAdminContractQuery,
  GlobalAdminKycCase,
  GlobalAdminKycCaseQuery,
  GlobalAdminPrivacyRequest,
  GlobalAdminPrivacyRequestDetail,
  GlobalAdminPrivacyRequestQuery,
  GlobalAdminPrivacyRequestCreateInput,
  GlobalAdminPrivacyWorkflowInput,
  GlobalAdminPrivacyExecutionInput,
  GlobalAdminPrivacyRequestResolutionInput,
  GlobalAdminPrivacyDispatchReceiptInput,
  GlobalAdminSupportTicket,
  GlobalAdminSupportTicketQuery,
  GlobalAdminSupportTicketCreateInput,
  GlobalAdminApproval,
  GlobalAdminApprovalQuery,
  GlobalAdminApprovalCreateInput,
  GlobalAdminApprovalDecisionInput,
  GlobalAdminPrivacyIncidentDetail,
  GlobalAdminPrivacyIncidentPage,
  GlobalAdminPrivacyIncidentQuery,
  CreateGlobalAdminPrivacyIncidentInput,
  TransitionGlobalAdminPrivacyIncidentInput,
  GlobalAdminPrivacyLegalHold,
  GlobalAdminPrivacyLegalHoldPage,
  GlobalAdminPrivacyLegalHoldQuery,
  CreateGlobalAdminPrivacyLegalHoldInput,
  ReleaseGlobalAdminPrivacyLegalHoldInput,
} from "../domain/types";

export interface AdminView {
  snapshot: AdminSnapshot;
  canAccess(section: AdminSection): boolean;
  hasAdminCapability(capability: AdminCapability): boolean;
  activeRole: AdminRole | null;

  // Global admin
  loadGlobalMe(): Promise<void>;
  loadMe(): Promise<void>;
  loadOverview(): Promise<void>;
  loadKPIs(query: {
    from: string;
    until: string;
    time_zone: string;
  }): Promise<void>;
  loadGlobalAccounts(query?: GlobalAdminAccountQuery): Promise<void>;
  revealGlobalAccountEmail(
    accountId: string,
    signal: AbortSignal,
  ): Promise<string>;
  loadAccounts(query?: GlobalAdminAccountQuery): Promise<void>;
  selectGlobalAccount(accountId: string): Promise<void>;
  listGlobalAccountResources(
    accountId: string,
    kind: GlobalAdminAccountResourceKind,
    query?: GlobalAdminAccountResourceQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminAccountResourcePage>;
  updateGlobalAccountProfile(
    accountId: string,
    input: {
      display_name: string;
      phone?: string | null;
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
  ): Promise<boolean>;
  setGlobalAccountRole(
    accountId: string,
    role: "arrendatario" | "arrendador",
    granted: boolean,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
  ): Promise<boolean>;
  updateAccountRole(
    accountId: string,
    role: "arrendatario" | "arrendador",
    granted: boolean,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
  ): Promise<boolean>;
  setGlobalLegacyAdminRole(
    accountId: string,
    granted: boolean,
    reason:
      | "aprovisionamiento_sintetico"
      | "revision_administrativa"
      | "retirada_acceso",
  ): Promise<boolean>;
  revokeGlobalAccountSessions(
    accountId: string,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
  ): Promise<boolean>;
  revokeAccountSessions(
    accountId: string,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
  ): Promise<boolean>;
  setGlobalAccountBlocked(
    accountId: string,
    blocked: boolean,
    reason: string,
  ): Promise<boolean>;
  blockAccount(accountId: string, reason: string): Promise<boolean>;
  unblockAccount(accountId: string, reason: string): Promise<boolean>;
  loadGlobalPublications(query?: GlobalAdminPublicationQuery): Promise<void>;
  loadPublications(query?: GlobalAdminPublicationQuery): Promise<void>;
  setGlobalPublicationState(
    publicationId: string,
    input: {
      state: "activa" | "oculta";
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
  ): Promise<boolean>;
  updatePublicationState(
    publicationId: string,
    input: {
      state: "activa" | "oculta";
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
  ): Promise<boolean>;
  retireGlobalPublication(
    publicationId: string,
    input: {
      owner_id: string;
      expected_updated_at: string;
      reason: string;
    },
  ): Promise<boolean>;
  retirePublication(
    publicationId: string,
    input: {
      owner_id: string;
      expected_updated_at: string;
      reason: string;
    },
  ): Promise<boolean>;
  loadGlobalAuditEvents(query: AuditEventsQuery): Promise<void>;
  loadAuditEvents(query: AuditEventsQuery): Promise<void>;
  exportGlobalAuditEvents(query: AuditEventsQuery): Promise<Blob | null>;
  exportAuditEvents(query: AuditEventsQuery): Promise<Blob | null>;

  // Local admin
  loadVerifications(): Promise<void>;
  loadRejectedVerifications(): Promise<void>;
  reviewVerification(
    id: string,
    decision: "aprobar" | "rechazar",
    reason?: string,
  ): Promise<boolean>;
  revokeVerification(
    id: string,
    reason_code:
      "aprobacion_fixture_incorrecta" | "revision_fixture_actualizada",
  ): Promise<boolean>;
  loadDisputes(): Promise<void>;
  closeDispute(
    disputeId: string,
    reason_code: "ensayo_finalizado" | "registro_erroneo" | "duplicada",
  ): Promise<boolean>;
  loadReviewReports(): Promise<void>;
  moderateReviewReport(
    reportId: string,
    action: "dismiss" | "hide",
    reason_code:
      | "sin_infraccion"
      | "contenido_inadecuado"
      | "duplicado"
      | "error_registro",
    correlation_id?: string,
  ): Promise<boolean>;
  handleReviewReport(
    reportId: string,
    action: "dismiss" | "hide",
    reason_code:
      | "sin_infraccion"
      | "contenido_inadecuado"
      | "duplicado"
      | "error_registro",
    correlation_id?: string,
  ): Promise<boolean>;
  loadNotices(): Promise<void>;
  reopenNotice(
    noticeId: string,
    reason_code: "smtp_restaurado" | "reintento_operativo",
    correlation_id?: string,
  ): Promise<boolean>;
  loadLocalAccount(accountId: string): Promise<void>;
  blockLocalAccount(accountId: string, reason: string): Promise<boolean>;
  unblockLocalAccount(accountId: string, reason: string): Promise<boolean>;
  loadPublishedSpaces(): Promise<void>;
  loadReports(query: {
    from: string;
    until: string;
    timezone: string;
  }): Promise<void>;
  loadLocalReservations(query?: {
    reservation_id?: string;
    state?: string;
    created_from?: string;
    created_to?: string;
    page_size?: number;
    cursor?: string;
  }): Promise<void>;
  loadLocalReservation(reservationId: string): Promise<void>;
  loadDamageClaims(): Promise<void>;
  loadLocalDamageClaims(): Promise<void>;
  loadDamageClaim(claimId: string): Promise<void>;
  resolveDamageClaim(
    claimId: string,
    input: AdminDamageClaimResolutionInput,
  ): Promise<boolean>;

  // Dedicated admin session
  checkAdminSession(): Promise<boolean>;
  startAdminSession(
    email: string,
    password: string,
  ): Promise<AdminSessionStartResponse | null>;
  verifyAdminSessionMfa(code: string, challengeId?: string): Promise<boolean>;
  logoutAdminSession(): Promise<void>;
  openMfaStepUp(): void;
  closeMfaStepUp(): void;
  requestGlobalAccountSensitiveAccess(
    accountId: string,
    input: AdminSensitiveAccessInput,
    signal?: AbortSignal,
  ): Promise<AdminSensitiveAccessResult | null>;
  startAssistedView(
    accountId: string,
    input: AdminAssistedViewStartInput,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewGrant | null>;
  getAssistedViewProjection(
    grantId: string,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewProjection | null>;
  closeAssistedView(grantId?: string, signal?: AbortSignal): Promise<void>;

  // Phase 4: Specialized Operations
  listGlobalPayments(
    query?: GlobalAdminPaymentQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminPayment[]; next_cursor?: string }>;
  listGlobalReservations(
    query?: GlobalAdminReservationQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminReservation[]; next_cursor?: string }>;
  listGlobalContracts(
    query?: GlobalAdminContractQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminContract[]; next_cursor?: string }>;
  listGlobalKycCases(
    query?: GlobalAdminKycCaseQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminKycCase[]; next_cursor?: string }>;
  listGlobalPrivacyRequests(
    query?: GlobalAdminPrivacyRequestQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminPrivacyRequest[]; next_cursor?: string }>;
  getGlobalPrivacyRequest(
    requestId: string,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequestDetail>;
  createGlobalPrivacyRequest(
    input: GlobalAdminPrivacyRequestCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequest>;
  transitionGlobalPrivacyRequest(
    requestId: string,
    input: GlobalAdminPrivacyWorkflowInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequestDetail>;
  executeGlobalPrivacyRight(
    requestId: string,
    input: GlobalAdminPrivacyExecutionInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequestDetail>;
  resolveGlobalPrivacyRequest(
    requestId: string,
    input: GlobalAdminPrivacyRequestResolutionInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequest>;
  recordGlobalPrivacyDispatchReceipt(
    requestId: string,
    input: GlobalAdminPrivacyDispatchReceiptInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequest>;
  retryGlobalPrivacyNotifications(
    requestId: string,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequestDetail>;
  listGlobalSupportTickets(
    query?: GlobalAdminSupportTicketQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminSupportTicket[]; next_cursor?: string }>;
  createGlobalSupportTicket(
    input: GlobalAdminSupportTicketCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminSupportTicket>;
  listGlobalApprovals(
    query?: GlobalAdminApprovalQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminApproval[]; next_cursor?: string }>;
  createGlobalApproval(
    input: GlobalAdminApprovalCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval>;
  decideGlobalApproval(
    approvalId: string,
    input: GlobalAdminApprovalDecisionInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval>;
  listGlobalPrivacyIncidents(
    query?: GlobalAdminPrivacyIncidentQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyIncidentPage>;
  getGlobalPrivacyIncident(
    incidentId: string,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyIncidentDetail>;
  createGlobalPrivacyIncident(
    input: CreateGlobalAdminPrivacyIncidentInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyIncidentDetail>;
  transitionGlobalPrivacyIncident(
    incidentId: string,
    input: TransitionGlobalAdminPrivacyIncidentInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyIncidentDetail>;
  listGlobalPrivacyLegalHolds(
    query?: GlobalAdminPrivacyLegalHoldQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyLegalHoldPage>;
  createGlobalPrivacyLegalHold(
    input: CreateGlobalAdminPrivacyLegalHoldInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyLegalHold>;
  releaseGlobalPrivacyLegalHold(
    holdId: string,
    input: ReleaseGlobalAdminPrivacyLegalHoldInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyLegalHold>;

  // Memory & error gates
  clearError(): void;
  clearFeedback(): void;
  purgePrivateData(): void;
}

export function useAdmin(): AdminView {
  const snapshot = useSyncExternalStore(
    adminFacade.subscribe,
    adminFacade.getSnapshot,
    adminFacade.getServerSnapshot,
  );

  useEffect(() => {
    if (snapshot.adminSessionStatus === "unknown") {
      void adminFacade.checkAdminSession();
    }
  }, [snapshot.adminSessionStatus]);

  // Only one role shown in UI per session
  const activeRole: AdminRole | null = ((): AdminRole | null => {
    if (
      snapshot.adminSessionStatus !== "authenticated" ||
      !snapshot.adminSession?.account_id
    ) {
      return null;
    }
    const me =
      snapshot.me && snapshot.me.account_id === snapshot.adminSession.account_id
        ? snapshot.me
        : null;
    const roles = (me?.roles ??
      snapshot.adminSession.roles) as readonly AdminRole[];
    if (roles.includes("administrador_global")) return "administrador_global";
    if (roles.includes("administrador")) return "administrador";
    if (roles.includes("arrendador")) return "arrendador";
    if (roles.includes("arrendatario")) return "arrendatario";
    return null;
  })();

  const canAccess = useCallback(
    (section: AdminSection): boolean => {
      if (
        snapshot.adminSessionStatus !== "authenticated" ||
        !snapshot.adminSession?.account_id
      ) {
        return false;
      }
      const me =
        snapshot.me &&
        snapshot.me.account_id === snapshot.adminSession.account_id
          ? snapshot.me
          : null;
      const capabilities = (me?.capabilities ??
        snapshot.adminSession.capabilities) as readonly AdminCapability[];
      const roles = (me?.roles ??
        snapshot.adminSession.roles) as readonly AdminRole[];
      return canAccessSection(capabilities, section, roles);
    },
    [snapshot.me, snapshot.adminSessionStatus, snapshot.adminSession],
  );

  const hasAdminCapability = useCallback(
    (capability: AdminCapability): boolean => {
      if (
        snapshot.adminSessionStatus !== "authenticated" ||
        !snapshot.adminSession?.account_id
      ) {
        return false;
      }
      const me =
        snapshot.me &&
        snapshot.me.account_id === snapshot.adminSession.account_id
          ? snapshot.me
          : null;
      const capabilities = (me?.capabilities ??
        snapshot.adminSession.capabilities) as readonly AdminCapability[];
      return hasCapability(capabilities, capability);
    },
    [snapshot.me, snapshot.adminSessionStatus, snapshot.adminSession],
  );

  const loadGlobalMe = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadGlobalMe();
  }, []);

  const loadOverview = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadOverview();
  }, []);

  const loadKPIs = useCallback(
    async (query: { from: string; until: string; time_zone: string }) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return;
      await adminFacade.loadKPIs(query);
    },
    [],
  );

  const loadGlobalAccounts = useCallback(
    async (query?: GlobalAdminAccountQuery) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return;
      await adminFacade.loadGlobalAccounts(query);
    },
    [],
  );

  const selectGlobalAccount = useCallback(async (accountId: string) => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.selectGlobalAccount(accountId);
  }, []);

  const listGlobalAccountResources = useCallback(
    async (
      accountId: string,
      kind: GlobalAdminAccountResourceKind,
      query?: GlobalAdminAccountResourceQuery,
      signal?: AbortSignal,
    ) => adminFacade.listGlobalAccountResources(accountId, kind, query, signal),
    [],
  );

  const updateGlobalAccountProfile = useCallback(
    async (
      accountId: string,
      input: {
        display_name: string;
        phone?: string | null;
        expected_updated_at: string;
        reason:
          "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
      },
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.updateGlobalAccountProfile(accountId, input);
    },
    [],
  );

  const setGlobalAccountRole = useCallback(
    async (
      accountId: string,
      role: "arrendatario" | "arrendador",
      granted: boolean,
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalAccountRole(accountId, role, granted, reason);
    },
    [],
  );

  const setGlobalLegacyAdminRole = useCallback(
    async (
      accountId: string,
      granted: boolean,
      reason:
        | "aprovisionamiento_sintetico"
        | "revision_administrativa"
        | "retirada_acceso",
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalLegacyAdminRole(accountId, granted, reason);
    },
    [],
  );

  const revokeGlobalAccountSessions = useCallback(
    async (
      accountId: string,
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.revokeGlobalAccountSessions(accountId, reason);
    },
    [],
  );

  const setGlobalAccountBlocked = useCallback(
    async (accountId: string, blocked: boolean, reason: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalAccountBlocked(accountId, blocked, reason);
    },
    [],
  );

  const blockAccount = useCallback(
    async (accountId: string, reason: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalAccountBlocked(accountId, true, reason);
    },
    [],
  );

  const unblockAccount = useCallback(
    async (accountId: string, reason: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalAccountBlocked(accountId, false, reason);
    },
    [],
  );

  const loadGlobalPublications = useCallback(
    async (query?: GlobalAdminPublicationQuery) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return;
      await adminFacade.loadGlobalPublications(query);
    },
    [],
  );

  const setGlobalPublicationState = useCallback(
    async (
      publicationId: string,
      input: {
        state: "activa" | "oculta";
        expected_updated_at: string;
        reason:
          "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
      },
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.setGlobalPublicationState(publicationId, input);
    },
    [],
  );

  const retireGlobalPublication = useCallback(
    async (
      publicationId: string,
      input: {
        owner_id: string;
        expected_updated_at: string;
        reason: string;
      },
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.retireGlobalPublication(publicationId, input);
    },
    [],
  );

  const loadGlobalAuditEvents = useCallback(async (query: AuditEventsQuery) => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadGlobalAuditEvents(query);
  }, []);

  const exportGlobalAuditEvents = useCallback(
    async (query: AuditEventsQuery) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return null;
      return adminFacade.exportGlobalAuditEvents(query);
    },
    [],
  );

  const loadVerifications = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadVerifications();
  }, []);

  const loadRejectedVerifications = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadRejectedVerifications();
  }, []);

  const reviewVerification = useCallback(
    async (id: string, decision: "aprobar" | "rechazar", reason?: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.reviewVerification(id, decision, reason);
    },
    [],
  );

  const revokeVerification = useCallback(
    async (
      id: string,
      reason_code:
        "aprobacion_fixture_incorrecta" | "revision_fixture_actualizada",
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.revokeVerification(id, reason_code);
    },
    [],
  );

  const loadDisputes = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadDisputes();
  }, []);

  const closeDispute = useCallback(
    async (
      disputeId: string,
      reason_code: "ensayo_finalizado" | "registro_erroneo" | "duplicada",
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.closeDispute(disputeId, reason_code);
    },
    [],
  );

  const loadReviewReports = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadReviewReports();
  }, []);

  const moderateReviewReport = useCallback(
    async (
      reportId: string,
      action: "dismiss" | "hide",
      reason_code:
        | "sin_infraccion"
        | "contenido_inadecuado"
        | "duplicado"
        | "error_registro",
      correlation_id?: string,
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.moderateReviewReport(
        reportId,
        action,
        reason_code,
        correlation_id,
      );
    },
    [],
  );

  const loadNotices = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadNotices();
  }, []);

  const reopenNotice = useCallback(
    async (
      noticeId: string,
      reason_code: "smtp_restaurado" | "reintento_operativo",
      correlation_id?: string,
    ) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.reopenNotice(noticeId, reason_code, correlation_id);
    },
    [],
  );

  const loadLocalAccount = useCallback(async (accountId: string) => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadLocalAccount(accountId);
  }, []);

  const blockLocalAccount = useCallback(
    async (accountId: string, reason: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.blockLocalAccount(accountId, reason);
    },
    [],
  );

  const unblockLocalAccount = useCallback(
    async (accountId: string, reason: string) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.unblockLocalAccount(accountId, reason);
    },
    [],
  );

  const loadPublishedSpaces = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadPublishedSpaces();
  }, []);

  const loadReports = useCallback(
    async (query: { from: string; until: string; timezone: string }) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return;
      await adminFacade.loadReports(query);
    },
    [],
  );

  const loadLocalReservations = useCallback(
    async (query?: {
      reservation_id?: string;
      state?: string;
      created_from?: string;
      created_to?: string;
      page_size?: number;
      cursor?: string;
    }) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return;
      await adminFacade.loadLocalReservations(query);
    },
    [],
  );

  const loadLocalReservation = useCallback(async (reservationId: string) => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadLocalReservation(reservationId);
  }, []);

  const loadDamageClaims = useCallback(async () => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadDamageClaims();
  }, []);

  const loadDamageClaim = useCallback(async (claimId: string) => {
    const isConfirmedSession =
      adminFacade.getSnapshot().adminSessionStatus === "authenticated";
    if (!isConfirmedSession) return;
    await adminFacade.loadDamageClaim(claimId);
  }, []);

  const resolveDamageClaim = useCallback(
    async (claimId: string, input: AdminDamageClaimResolutionInput) => {
      const isConfirmedSession =
        adminFacade.getSnapshot().adminSessionStatus === "authenticated";
      if (!isConfirmedSession) return false;
      return adminFacade.resolveDamageClaim(claimId, input);
    },
    [],
  );

  const clearError = useCallback(() => {
    adminFacade.clearError();
  }, []);

  const purgePrivateData = useCallback(() => {
    adminFacade.purgePrivateData();
  }, []);

  const checkAdminSession = useCallback(
    () => adminFacade.checkAdminSession(),
    [],
  );

  const startAdminSession = useCallback(
    (email: string, password: string) =>
      adminFacade.startAdminSession(email, password),
    [],
  );

  const verifyAdminSessionMfa = useCallback(
    (code: string, challengeId?: string) =>
      adminFacade.verifyAdminSessionMfa(code, challengeId),
    [],
  );

  const logoutAdminSession = useCallback(
    () => adminFacade.logoutAdminSession(),
    [],
  );

  const openMfaStepUp = useCallback(() => {
    adminFacade.openMfaStepUp();
  }, []);

  const closeMfaStepUp = useCallback(() => {
    adminFacade.closeMfaStepUp();
  }, []);

  const requestGlobalAccountSensitiveAccess = useCallback(
    (
      accountId: string,
      input: AdminSensitiveAccessInput,
      signal?: AbortSignal,
    ) =>
      adminFacade.requestGlobalAccountSensitiveAccess(accountId, input, signal),
    [],
  );

  const startAssistedView = useCallback(
    (
      accountId: string,
      input: AdminAssistedViewStartInput,
      signal?: AbortSignal,
    ) => adminFacade.startAssistedView(accountId, input, signal),
    [],
  );

  const getAssistedViewProjection = useCallback(
    (grantId: string, signal?: AbortSignal) =>
      adminFacade.getAssistedViewProjection(grantId, signal),
    [],
  );

  const closeAssistedView = useCallback(
    (grantId?: string, signal?: AbortSignal) =>
      adminFacade.closeAssistedView(grantId, signal),
    [],
  );

  const listGlobalPayments = useCallback(
    (query?: GlobalAdminPaymentQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalPayments(query, signal),
    [],
  );

  const listGlobalReservations = useCallback(
    (query?: GlobalAdminReservationQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalReservations(query, signal),
    [],
  );

  const listGlobalContracts = useCallback(
    (query?: GlobalAdminContractQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalContracts(query, signal),
    [],
  );

  const listGlobalKycCases = useCallback(
    (query?: GlobalAdminKycCaseQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalKycCases(query, signal),
    [],
  );

  const listGlobalPrivacyRequests = useCallback(
    (query?: GlobalAdminPrivacyRequestQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalPrivacyRequests(query, signal),
    [],
  );
  const getGlobalPrivacyRequest = useCallback(
    (requestId: string, signal?: AbortSignal) =>
      adminFacade.getGlobalPrivacyRequest(requestId, signal),
    [],
  );

  const createGlobalPrivacyRequest = useCallback(
    (input: GlobalAdminPrivacyRequestCreateInput, signal?: AbortSignal) =>
      adminFacade.createGlobalPrivacyRequest(input, signal),
    [],
  );
  const transitionGlobalPrivacyRequest = useCallback(
    (
      requestId: string,
      input: GlobalAdminPrivacyWorkflowInput,
      signal?: AbortSignal,
    ) => adminFacade.transitionGlobalPrivacyRequest(requestId, input, signal),
    [],
  );
  const executeGlobalPrivacyRight = useCallback(
    (
      requestId: string,
      input: GlobalAdminPrivacyExecutionInput,
      signal?: AbortSignal,
    ) => adminFacade.executeGlobalPrivacyRight(requestId, input, signal),
    [],
  );
  const resolveGlobalPrivacyRequest = useCallback(
    (
      requestId: string,
      input: GlobalAdminPrivacyRequestResolutionInput,
      signal?: AbortSignal,
    ) => adminFacade.resolveGlobalPrivacyRequest(requestId, input, signal),
    [],
  );
  const recordGlobalPrivacyDispatchReceipt = useCallback(
    (
      requestId: string,
      input: GlobalAdminPrivacyDispatchReceiptInput,
      signal?: AbortSignal,
    ) =>
      adminFacade.recordGlobalPrivacyDispatchReceipt(requestId, input, signal),
    [],
  );
  const retryGlobalPrivacyNotifications = useCallback(
    (requestId: string, signal?: AbortSignal) =>
      adminFacade.retryGlobalPrivacyNotifications(requestId, signal),
    [],
  );

  const listGlobalSupportTickets = useCallback(
    (query?: GlobalAdminSupportTicketQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalSupportTickets(query, signal),
    [],
  );

  const createGlobalSupportTicket = useCallback(
    (input: GlobalAdminSupportTicketCreateInput, signal?: AbortSignal) =>
      adminFacade.createGlobalSupportTicket(input, signal),
    [],
  );

  const listGlobalApprovals = useCallback(
    (query?: GlobalAdminApprovalQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalApprovals(query, signal),
    [],
  );

  const createGlobalApproval = useCallback(
    (input: GlobalAdminApprovalCreateInput, signal?: AbortSignal) =>
      adminFacade.createGlobalApproval(input, signal),
    [],
  );

  const decideGlobalApproval = useCallback(
    (
      approvalId: string,
      input: GlobalAdminApprovalDecisionInput,
      signal?: AbortSignal,
    ) => adminFacade.decideGlobalApproval(approvalId, input, signal),
    [],
  );

  const listGlobalPrivacyIncidents = useCallback(
    (query?: GlobalAdminPrivacyIncidentQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalPrivacyIncidents(query, signal),
    [],
  );

  const getGlobalPrivacyIncident = useCallback(
    (incidentId: string, signal?: AbortSignal) =>
      adminFacade.getGlobalPrivacyIncident(incidentId, signal),
    [],
  );

  const createGlobalPrivacyIncident = useCallback(
    (input: CreateGlobalAdminPrivacyIncidentInput, signal?: AbortSignal) =>
      adminFacade.createGlobalPrivacyIncident(input, signal),
    [],
  );

  const transitionGlobalPrivacyIncident = useCallback(
    (
      incidentId: string,
      input: TransitionGlobalAdminPrivacyIncidentInput,
      signal?: AbortSignal,
    ) => adminFacade.transitionGlobalPrivacyIncident(incidentId, input, signal),
    [],
  );

  const listGlobalPrivacyLegalHolds = useCallback(
    (query?: GlobalAdminPrivacyLegalHoldQuery, signal?: AbortSignal) =>
      adminFacade.listGlobalPrivacyLegalHolds(query, signal),
    [],
  );

  const createGlobalPrivacyLegalHold = useCallback(
    (input: CreateGlobalAdminPrivacyLegalHoldInput, signal?: AbortSignal) =>
      adminFacade.createGlobalPrivacyLegalHold(input, signal),
    [],
  );

  const releaseGlobalPrivacyLegalHold = useCallback(
    (
      holdId: string,
      input: ReleaseGlobalAdminPrivacyLegalHoldInput,
      signal?: AbortSignal,
    ) => adminFacade.releaseGlobalPrivacyLegalHold(holdId, input, signal),
    [],
  );

  return {
    snapshot,
    canAccess,
    hasAdminCapability,
    activeRole,

    loadGlobalMe,
    loadMe: loadGlobalMe,
    loadOverview,
    loadKPIs,
    loadGlobalAccounts,
    revealGlobalAccountEmail: (accountId, signal) =>
      adminFacade.revealGlobalAccountEmail(accountId, signal),
    loadAccounts: loadGlobalAccounts,
    selectGlobalAccount,
    listGlobalAccountResources,
    updateGlobalAccountProfile,
    setGlobalAccountRole,
    updateAccountRole: setGlobalAccountRole,
    setGlobalLegacyAdminRole,
    revokeGlobalAccountSessions,
    revokeAccountSessions: revokeGlobalAccountSessions,
    setGlobalAccountBlocked,
    blockAccount,
    unblockAccount,
    loadGlobalPublications,
    loadPublications: loadGlobalPublications,
    setGlobalPublicationState,
    updatePublicationState: setGlobalPublicationState,
    retireGlobalPublication,
    retirePublication: retireGlobalPublication,
    loadGlobalAuditEvents,
    loadAuditEvents: loadGlobalAuditEvents,
    exportGlobalAuditEvents,
    exportAuditEvents: exportGlobalAuditEvents,

    loadVerifications,
    loadRejectedVerifications,
    reviewVerification,
    revokeVerification,
    loadDisputes,
    closeDispute,
    loadReviewReports,
    moderateReviewReport,
    handleReviewReport: moderateReviewReport,
    loadNotices,
    reopenNotice,
    loadLocalAccount,
    blockLocalAccount,
    unblockLocalAccount,
    loadPublishedSpaces,
    loadReports,
    loadLocalReservations,
    loadLocalReservation,
    loadDamageClaims,
    loadLocalDamageClaims: loadDamageClaims,
    loadDamageClaim,
    resolveDamageClaim,

    checkAdminSession,
    startAdminSession,
    verifyAdminSessionMfa,
    logoutAdminSession,
    openMfaStepUp,
    closeMfaStepUp,
    requestGlobalAccountSensitiveAccess,
    startAssistedView,
    getAssistedViewProjection,
    closeAssistedView,

    listGlobalPayments,
    listGlobalReservations,
    listGlobalContracts,
    listGlobalKycCases,
    listGlobalPrivacyRequests,
    getGlobalPrivacyRequest,
    createGlobalPrivacyRequest,
    transitionGlobalPrivacyRequest,
    executeGlobalPrivacyRight,
    resolveGlobalPrivacyRequest,
    recordGlobalPrivacyDispatchReceipt,
    retryGlobalPrivacyNotifications,
    listGlobalSupportTickets,
    createGlobalSupportTicket,
    listGlobalApprovals,
    createGlobalApproval,
    decideGlobalApproval,
    listGlobalPrivacyIncidents,
    getGlobalPrivacyIncident,
    createGlobalPrivacyIncident,
    transitionGlobalPrivacyIncident,
    listGlobalPrivacyLegalHolds,
    createGlobalPrivacyLegalHold,
    releaseGlobalPrivacyLegalHold,

    clearError,
    clearFeedback: clearError,
    purgePrivateData,
  };
}
