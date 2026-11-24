import type {
  AdminAssistedViewGrant,
  AdminAssistedViewProjection,
  AdminAssistedViewStartInput,
  AdminDamageClaim,
  AdminDamageClaimResolutionInput,
  AdminDisputesReport,
  AdminFinanceReport,
  AdminLocalAccount,
  AdminLocalDispute,
  AdminLocalNotice,
  AdminPublishedSpace,
  AdminReservationDetail,
  AdminReservationSummary,
  AdminReservationsReport,
  AdminReviewReport,
  AdminSensitiveAccessInput,
  AdminSensitiveAccessResult,
  AdminSessionData,
  AdminSessionStartResponse,
  AdminVerificationCase,
  AuditEvent,
  AuditEventsQuery,
  GlobalAdminAccount,
  GlobalAdminAccountQuery,
  GlobalAdminAccountResourceKind,
  GlobalAdminAccountResourcePage,
  GlobalAdminAccountResourceQuery,
  GlobalAdminKPIs,
  GlobalAdminMe,
  GlobalAdminOverview,
  GlobalAdminPublication,
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
  GlobalAdminPrivacyRequestQuery,
  GlobalAdminPrivacyRequestCreateInput,
  GlobalAdminSupportTicket,
  GlobalAdminSupportTicketQuery,
  GlobalAdminSupportTicketCreateInput,
  GlobalAdminApproval,
  GlobalAdminApprovalQuery,
  GlobalAdminApprovalCreateInput,
  GlobalAdminApprovalDecisionInput,
} from "../domain/types";

export interface AdminPort {
  // Dedicated Admin Session
  getAdminSession(signal?: AbortSignal): Promise<AdminSessionData | null>;

  startAdminSession(
    input: { email: string; password: string },
    signal?: AbortSignal,
  ): Promise<AdminSessionStartResponse>;

  verifyAdminSessionMfa(
    input: { challenge_id?: string; code: string },
    signal?: AbortSignal,
  ): Promise<AdminSessionData>;

  deleteAdminSession(signal?: AbortSignal): Promise<void>;

  requestGlobalAccountSensitiveAccess(
    accountId: string,
    input: AdminSensitiveAccessInput,
    signal?: AbortSignal,
  ): Promise<AdminSensitiveAccessResult>;

  startAssistedView(
    accountId: string,
    input: AdminAssistedViewStartInput,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewGrant>;

  getAssistedViewProjection(
    grantId: string,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewProjection>;

  closeAssistedView(grantId: string, signal?: AbortSignal): Promise<void>;

  // Global Admin
  getGlobalAdminMe(signal?: AbortSignal): Promise<GlobalAdminMe>;

  getGlobalAdminOverview(signal?: AbortSignal): Promise<GlobalAdminOverview>;

  getGlobalAdminKPIs(
    query: { from: string; until: string; time_zone: string },
    signal?: AbortSignal,
  ): Promise<GlobalAdminKPIs>;

  listGlobalAuditEvents(
    query: AuditEventsQuery,
    signal?: AbortSignal,
  ): Promise<AuditEvent[]>;

  exportGlobalAuditEvents(
    query: AuditEventsQuery,
    signal?: AbortSignal,
  ): Promise<Blob>;

  listGlobalAccounts(
    query?: GlobalAdminAccountQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminAccount[]; next_cursor?: string }>;

  getGlobalAccount(
    accountId: string,
    signal?: AbortSignal,
    revealEmail?: true,
  ): Promise<GlobalAdminAccount | null>;

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
    signal?: AbortSignal,
  ): Promise<void>;

  setGlobalAccountRole(
    accountId: string,
    role: "arrendatario" | "arrendador",
    granted: boolean,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    signal?: AbortSignal,
  ): Promise<void>;

  setGlobalLegacyAdminRole(
    accountId: string,
    granted: boolean,
    reason:
      | "aprovisionamiento_sintetico"
      | "revision_administrativa"
      | "retirada_acceso",
    signal?: AbortSignal,
  ): Promise<void>;

  revokeGlobalAccountSessions(
    accountId: string,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    signal?: AbortSignal,
  ): Promise<void>;

  setGlobalAccountBlocked(
    accountId: string,
    blocked: boolean,
    reason: string,
    signal?: AbortSignal,
  ): Promise<void>;

  listGlobalPublications(
    query?: GlobalAdminPublicationQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPublication[]>;

  setGlobalPublicationState(
    publicationId: string,
    input: {
      state: "activa" | "oculta";
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
    signal?: AbortSignal,
  ): Promise<void>;

  retireGlobalPublication(
    publicationId: string,
    input: {
      owner_id: string;
      expected_updated_at: string;
      reason: string;
    },
    signal?: AbortSignal,
  ): Promise<void>;

  // Local / Operativa Admin
  listVerifications(signal?: AbortSignal): Promise<AdminVerificationCase[]>;

  listRejectedVerifications(
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase[]>;

  reviewVerification(
    id: string,
    decision: "aprobar" | "rechazar",
    reason?: string,
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase>;

  revokeVerification(
    id: string,
    idempotencyKey: string,
    reason_code:
      "aprobacion_fixture_incorrecta" | "revision_fixture_actualizada",
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase>;

  listDisputes(signal?: AbortSignal): Promise<AdminLocalDispute[]>;

  closeDispute(
    disputeId: string,
    reason_code: "ensayo_finalizado" | "registro_erroneo" | "duplicada",
    signal?: AbortSignal,
  ): Promise<AdminLocalDispute>;

  listReviewReports(signal?: AbortSignal): Promise<AdminReviewReport[]>;

  moderateReviewReport(
    reportId: string,
    action: "dismiss" | "hide",
    idempotencyKey: string,
    input: {
      reason_code:
        | "sin_infraccion"
        | "contenido_inadecuado"
        | "duplicado"
        | "error_registro";
      correlation_id: string;
    },
    signal?: AbortSignal,
  ): Promise<AdminReviewReport>;

  listNotices(signal?: AbortSignal): Promise<AdminLocalNotice[]>;

  reopenNotice(
    noticeId: string,
    idempotencyKey: string,
    input: {
      reason_code: "smtp_restaurado" | "reintento_operativo";
      correlation_id: string;
    },
    signal?: AbortSignal,
  ): Promise<AdminLocalNotice>;

  listLocalAuditEvents(
    query: {
      from: string;
      until: string;
      actor_id?: string;
      resource_type?: string;
      resource_id?: string;
      action?: string;
      result?: "exito" | "rechazo";
    },
    signal?: AbortSignal,
  ): Promise<AuditEvent[]>;

  exportLocalAuditEvents(
    query: {
      from: string;
      until: string;
      actor_id?: string;
      resource_type?: string;
      resource_id?: string;
      action?: string;
      result?: "exito" | "rechazo";
    },
    signal?: AbortSignal,
  ): Promise<Blob>;

  getLocalAccount(
    accountId: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount | null>;

  blockLocalAccount(
    accountId: string,
    reason: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount>;

  unblockLocalAccount(
    accountId: string,
    reason: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount>;

  listPublishedSpaces(signal?: AbortSignal): Promise<AdminPublishedSpace[]>;

  getPublishedSpace(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<AdminPublishedSpace | null>;

  getReservationsReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminReservationsReport>;

  getFinanceReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminFinanceReport>;

  getDisputesReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminDisputesReport>;

  listLocalReservations(
    query?: {
      reservation_id?: string;
      state?: string;
      created_from?: string;
      created_to?: string;
      page_size?: number;
      cursor?: string;
    },
    signal?: AbortSignal,
  ): Promise<AdminReservationSummary[]>;

  getLocalReservation(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<AdminReservationDetail | null>;

  listDamageClaims(signal?: AbortSignal): Promise<AdminDamageClaim[]>;

  getDamageClaim(
    claimId: string,
    signal?: AbortSignal,
  ): Promise<AdminDamageClaim | null>;

  resolveDamageClaim(
    claimId: string,
    idempotencyKey: string,
    input: AdminDamageClaimResolutionInput,
    signal?: AbortSignal,
  ): Promise<AdminDamageClaim>;

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

  createGlobalPrivacyRequest(
    idempotencyKey: string,
    input: GlobalAdminPrivacyRequestCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequest>;

  listGlobalSupportTickets(
    query?: GlobalAdminSupportTicketQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminSupportTicket[]; next_cursor?: string }>;

  createGlobalSupportTicket(
    idempotencyKey: string,
    input: GlobalAdminSupportTicketCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminSupportTicket>;

  listGlobalApprovals(
    query?: GlobalAdminApprovalQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminApproval[]; next_cursor?: string }>;

  createGlobalApproval(
    idempotencyKey: string,
    input: GlobalAdminApprovalCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval>;

  decideGlobalApproval(
    approvalId: string,
    idempotencyKey: string,
    input: GlobalAdminApprovalDecisionInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval>;
}
