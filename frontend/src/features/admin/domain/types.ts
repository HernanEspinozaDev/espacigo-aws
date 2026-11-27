export type AdminRole =
  "arrendatario" | "arrendador" | "administrador" | "administrador_global";

export type AdminCapability =
  | "accounts.list"
  | "accounts.read"
  | "accounts.email.reveal"
  | "accounts.update"
  | "accounts.block"
  | "accounts.roles"
  | "publications.list"
  | "publications.read"
  | "publications.update"
  | "publications.moderate"
  | "publications.retire"
  | "admin_access.manage"
  | "kpi.read"
  | "audit.read"
  | "audit.export"
  | "legacy.administration"
  | "payments.read"
  | "payments.reconcile"
  | "payments.refund.request"
  | "payments.refund.approve"
  | "reservations.read"
  | "contracts.read"
  | "kyc.case.read"
  | "kyc.review"
  | "privacy.requests.manage"
  | "privacy.export.approve"
  | "support.tickets.read"
  | "support.tickets.manage"
  | "approvals.read"
  | "approvals.request"
  | "approvals.decide";

export type AdminSection =
  | "overview"
  | "accounts"
  | "publications"
  | "kpi"
  | "audit"
  | "verifications"
  | "disputes"
  | "review_reports"
  | "notices"
  | "spaces"
  | "reports"
  | "reservations"
  | "damage_claims"
  | "finance"
  | "privacy"
  | "approvals"
  | "support";

export interface GlobalAdminMe {
  readonly account_id: string;
  readonly roles: readonly AdminRole[];
  readonly capabilities: readonly AdminCapability[];
  readonly generated_at: string;
}

export interface GlobalAdminOverview {
  readonly total_accounts: number | string;
  readonly connected_accounts?: number | string;
  readonly administrative_blocked: number | string;
  readonly by_state: Readonly<Record<string, number | string>>;
  readonly by_role: Readonly<Record<string, number | string>>;
  readonly generated_at: string;
  readonly definition: string;
}

export interface GlobalAdminKPIs {
  readonly period: {
    readonly from: string;
    readonly until: string;
    readonly time_zone: string;
  };
  readonly generated_at: string;
  readonly definition: string;
  readonly metrics: Readonly<Record<string, unknown>>;
}

export type GlobalAdminAccountState =
  | "correo_pendiente"
  | "activo"
  | "bloqueado"
  | "baja_solicitada"
  | "desidentificado";

export interface GlobalAdminAccount {
  readonly account_id: string;
  readonly email: string;
  readonly display_name?: string | null;
  readonly phone?: string | null;
  readonly profile_updated_at?: string | null;
  readonly state: GlobalAdminAccountState;
  readonly created_at: string;
  readonly administrative_blocked: boolean;
  readonly roles: readonly AdminRole[];
  readonly publication_count: number | string;
  readonly reservation_count: number | string;
  readonly incident_count: number | string;
}

export interface GlobalAdminAccountQuery {
  readonly account_id?: string;
  readonly email_prefix?: string;
  readonly name_prefix?: string;
  readonly role?: AdminRole;
  readonly state?: GlobalAdminAccountState;
  readonly cursor?: string;
  readonly page_size?: number;
}

export type GlobalAdminAccountResourceKind =
  | "reservations"
  | "incidents"
  | "commercial_reservations"
  | "commercial_contracts"
  | "commercial_payments";

export interface GlobalAdminAccountResource {
  readonly id: string;
  readonly kind: GlobalAdminAccountResourceKind;
  readonly state: string;
  readonly occurred_at: string;
  readonly reservation_id?: string;
  readonly participant_role?: string;
  readonly data_origin?: "commercial" | "trial";
  readonly test_mode?: boolean | null;
  readonly role_in_record?: string;
  readonly amount_clp?: number | null;
  readonly total_charged_clp?: number | null;
  readonly currency?: string;
  readonly space_id?: string;
  readonly space_title?: string;
  readonly provider_payment_id?: string;
  readonly provider_status?: string;
  readonly document_status?: string;
  readonly related_ids?: Readonly<Record<string, string>>;
}

export interface GlobalAdminAccountResourceQuery {
  readonly page_size?: number;
  readonly cursor?: string;
}

export interface GlobalAdminAccountResourcePage {
  readonly items: readonly GlobalAdminAccountResource[];
  readonly next_cursor: string;
  readonly matching_total: number;
  readonly generated_at: string;
}

export type GlobalAdminPublicationState =
  "borrador" | "activa" | "oculta" | "eliminada";

export interface GlobalAdminPublicationPhoto {
  readonly photo_id: string;
  readonly url?: string;
  readonly is_cover?: boolean;
  readonly is_visible?: boolean;
}

export interface GlobalAdminPublication {
  readonly publication_id: string;
  readonly owner_id: string;
  readonly owner_name?: string | null;
  readonly owner_email: string;
  readonly category_code: string;
  readonly category_name: string;
  readonly title: string;
  readonly description: string;
  readonly area_m2: string;
  readonly capacity: number;
  readonly usage_rules: string;
  readonly address: string;
  readonly state: GlobalAdminPublicationState;
  readonly rate_unit: "hora" | "dia" | "mes";
  readonly base_price_clp: string;
  readonly updated_at: string;
  readonly publicly_visible: boolean;
  readonly public_cover_available: boolean;
  readonly attribute_schema_version: number;
  readonly attributes: Readonly<Record<string, unknown>>;
  readonly photos?: readonly GlobalAdminPublicationPhoto[];
}

export interface GlobalAdminPublicationQuery {
  readonly owner_id?: string;
  readonly category?: string;
  readonly state?: GlobalAdminPublicationState;
  readonly search?: string;
  readonly publicly_visible?: boolean;
  readonly page_size?: number;
  readonly cursor?: string;
}

export interface AuditEvent {
  readonly event_id: string;
  readonly actor_id: string;
  readonly target_account_id?: string | null;
  readonly resource_type?: string | null;
  readonly resource_id?: string | null;
  readonly action: string;
  readonly result: "exito" | "rechazo";
  readonly reason_code?: string | null;
  readonly correlation_id?: string | null;
  readonly timestamp: string;
  readonly details?: Readonly<Record<string, unknown>> | null;
}

export interface AuditEventsQuery {
  readonly from: string;
  readonly until: string;
  readonly actor_id?: string;
  readonly target_account_id?: string;
  readonly resource_type?: string;
  readonly resource_id?: string;
  readonly action?: string;
  readonly result?: "exito" | "rechazo";
  readonly reason_code?: string;
  readonly correlation_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
  readonly snapshot_cutoff?: string;
  readonly format?: "json" | "csv";
}

export interface AdminVerificationCase {
  readonly id: string;
  readonly type: "kyc" | "kyb";
  readonly state: "en_revision" | "aprobada" | "rechazada" | "revocada";
  readonly applicant_id: string;
  readonly correction_code?: string | null;
  readonly created_at: string;
  readonly updated_at: string;
  readonly notes?: string | null;
}

export interface AdminVerificationReviewInput {
  readonly decision: "aprobar" | "rechazar";
  readonly reason?: string;
}

export interface AdminLocalDispute {
  readonly dispute_id: string;
  readonly reservation_id: string;
  readonly renter_id: string;
  readonly host_id: string;
  readonly state: "abierta" | "en_revision" | "resuelta" | "cerrada";
  readonly reason: string;
  readonly created_at: string;
  readonly closed_at?: string | null;
  readonly close_reason?: string | null;
}

export interface AdminReviewReport {
  readonly report_id: string;
  readonly review_id: string;
  readonly reporter_id: string;
  readonly state: "pendiente" | "desestimado" | "oculto";
  readonly reason: string;
  readonly created_at: string;
  readonly action_reason?: string | null;
}

export interface AdminLocalNotice {
  readonly notice_id: string;
  readonly event_type: string;
  readonly recipient_email: string;
  readonly terminal_status: string;
  readonly created_at: string;
  readonly correlation_id?: string;
  readonly reopen_count?: number;
}

export interface AdminLocalAccount {
  readonly account_id: string;
  readonly email: string;
  readonly display_name?: string | null;
  readonly state: string;
  readonly is_blocked: boolean;
  readonly blocked_reason?: string | null;
  readonly created_at: string;
  readonly updated_at?: string | null;
}

export interface AdminPublishedSpace {
  readonly space_id: string;
  readonly host_id: string;
  readonly title: string;
  readonly category_code: string;
  readonly state: string;
  readonly base_price_clp: number | string;
  readonly area_m2: number;
  readonly capacity: number;
  readonly address?: string;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface AdminReservationsReport {
  readonly generated_at: string;
  readonly from: string;
  readonly until: string;
  readonly timezone: string;
  readonly total_reservations: number;
  readonly by_state: Readonly<Record<string, number>>;
  readonly test_notice: string;
}

export interface AdminFinanceReport {
  readonly generated_at: string;
  readonly from: string;
  readonly until: string;
  readonly timezone: string;
  readonly confirmed_movements_clp: number | string;
  readonly pending_obligations_clp: number | string;
  readonly fee_volume_clp: number | string;
  readonly currency: "CLP";
  readonly test_notice: string;
}

export interface AdminDisputesReport {
  readonly generated_at: string;
  readonly from: string;
  readonly until: string;
  readonly timezone: string;
  readonly total_disputes: number;
  readonly open_disputes: number;
  readonly closed_disputes: number;
  readonly test_notice: string;
}

export interface AdminReservationSummary {
  readonly reservation_id: string;
  readonly quote_id: string;
  readonly space_id: string;
  readonly host_id: string;
  readonly renter_id: string;
  readonly state: string;
  readonly created_at: string;
  readonly start_at: string;
  readonly end_at: string;
  readonly total_clp: number | string;
}

export interface AdminReservationDetail extends AdminReservationSummary {
  readonly history: readonly {
    readonly state: string;
    readonly at: string;
    readonly reason?: string;
  }[];
}

export interface AdminDamageClaim {
  readonly claim_id: string;
  readonly reservation_id: string;
  readonly host_id: string;
  readonly renter_id: string;
  readonly state: "abierto" | "en_evaluacion" | "resuelto" | "rechazado";
  readonly description: string;
  readonly created_at: string;
  readonly evidence_ids?: readonly string[];
  readonly resolution?: {
    readonly decision: string;
    readonly reason: string;
    readonly at: string;
  };
}

export interface AdminDamageClaimResolutionInput {
  readonly decision: "aceptado" | "rechazado" | "parcial";
  readonly resolution_notes: string;
  readonly approved_amount_clp?: number;
}

export type AdminSessionStatus =
  "unknown" | "checking" | "anonymous" | "mfa_required" | "authenticated";

export interface AdminSessionData {
  readonly account_id: string;
  readonly audience: string;
  readonly roles: readonly AdminRole[];
  readonly capabilities: readonly AdminCapability[];
  readonly mfa_verified_at: string | null;
  readonly recent_auth_expires_at: string | null;
  readonly expires_at: string;
}

export interface AdminSessionStartResponse {
  readonly status: "mfa_required" | "authenticated";
  readonly challenge_id?: string;
  readonly methods?: readonly string[];
  readonly expires_at: string;
  readonly account_id?: string;
  readonly audience?: string;
  readonly roles?: readonly AdminRole[];
  readonly capabilities?: readonly AdminCapability[];
  readonly mfa_verified_at?: string | null;
  readonly recent_auth_expires_at?: string | null;
}

export interface AdminMfaChallenge {
  readonly challenge_id: string;
  readonly methods: readonly string[];
  readonly expires_at: string;
}

export type AdminSensitiveAccessScope = "account_email" | "account_summary";

export interface AdminSensitiveAccessInput {
  readonly field: "email";
  readonly reason: string;
  readonly case_id?: string;
  readonly scope?: AdminSensitiveAccessScope;
}

export interface AdminSensitiveAccessResult {
  readonly account_id: string;
  readonly field: "email";
  readonly value: string;
  readonly masked_value: string;
  readonly scope: AdminSensitiveAccessScope;
  readonly reason: string;
  readonly case_id?: string;
  readonly expires_at: string;
  readonly audit_event_id?: string;
}

export type AdminAssistedViewRole = "arrendador" | "arrendatario";

export interface AdminAssistedViewStartInput {
  readonly role: AdminAssistedViewRole;
  readonly case_id: string;
  readonly reason: string;
}

export interface AdminAssistedViewGrant {
  readonly grant_id: string;
  readonly account_id: string;
  readonly actor_account_id: string;
  readonly role: AdminAssistedViewRole;
  readonly case_id: string;
  readonly reason: string;
  readonly read_only: true;
  readonly expires_at: string;
  readonly created_at?: string;
  readonly audit_event_id?: string;
}

export interface AdminAssistedViewAccountSummary {
  readonly account_id: string;
  readonly state: string;
  readonly email_verified: boolean;
  readonly phone_verified: boolean;
  readonly kyc_status: string;
  readonly kyc_rejection_reason?: string;
  readonly roles: readonly string[];
  readonly admin_blocked: boolean;
}

export type AdminAssistedViewDiagnosticSeverity = "error" | "warning" | "info";

export interface AdminAssistedViewDiagnostic {
  readonly code: string;
  readonly severity: AdminAssistedViewDiagnosticSeverity;
  readonly area: string;
  readonly title: string;
  readonly detail: string;
}

export interface AdminAssistedViewPublishStep {
  readonly step: number;
  readonly key: string;
  readonly title: string;
  readonly status: string;
  readonly detail: string;
}

export interface AdminAssistedViewPublishGuide {
  readonly kyc_status: string;
  readonly draft_status: string;
  readonly mercadopago_status: string;
  readonly contract_status: string;
  readonly steps: readonly AdminAssistedViewPublishStep[];
}

export interface AdminAssistedViewMercadoPagoConnection {
  readonly connected: boolean;
  readonly connection_state?: string;
  readonly live_mode?: boolean;
  readonly connected_at?: string;
  readonly updated_at?: string;
}

export interface AdminAssistedViewSpaceItem {
  readonly id: string;
  readonly title: string;
  readonly status: string;
  readonly modality?: string;
  readonly price_clp?: number | null;
  readonly max_capacity?: number | null;
  readonly area_m2?: number | null;
  readonly time_zone?: string;
  readonly location_confirmed: boolean;
  readonly contract_published_version: number;
  readonly publish_blockers: readonly string[];
}

export interface AdminAssistedViewReservationItem {
  readonly id: string;
  readonly space_id: string;
  readonly space_title: string;
  readonly status: string;
  readonly modality?: string;
  readonly subtotal_clp?: number | null;
  readonly start_at: string;
  readonly end_at: string;
  readonly payment_expires_at?: string | null;
  readonly created_at: string;
}

export interface AdminAssistedViewContractItem {
  readonly id: string;
  readonly proposal_id?: string | null;
  readonly space_id: string;
  readonly space_title: string;
  readonly status: string;
  readonly signing_mode?: string;
  readonly document_status?: string;
  readonly subtotal_clp?: number | null;
  readonly created_at: string;
}

export interface AdminAssistedViewPaymentItem {
  readonly id: string;
  readonly reservation_id?: string | null;
  readonly space_id: string;
  readonly space_title: string;
  readonly status: string;
  readonly checkout_method: string;
  readonly test_mode: boolean;
  readonly total_charged_clp: number;
  readonly provider_payment_status?: string;
  readonly reconciliation_reason?: string;
  readonly created_at: string;
}

export interface AdminAssistedViewProjection {
  readonly grant_id: string;
  readonly account_id: string;
  readonly actor_account_id: string;
  readonly role: AdminAssistedViewRole;
  readonly case_id: string;
  readonly expires_at: string;
  readonly read_only: true;
  readonly account_summary: AdminAssistedViewAccountSummary;
  readonly diagnostics: readonly AdminAssistedViewDiagnostic[];
  readonly publish_guide?: AdminAssistedViewPublishGuide | null;
  readonly mercadopago_connection?: AdminAssistedViewMercadoPagoConnection | null;
  readonly spaces: readonly AdminAssistedViewSpaceItem[];
  readonly reservations: readonly AdminAssistedViewReservationItem[];
  readonly contracts: readonly AdminAssistedViewContractItem[];
  readonly payments: readonly AdminAssistedViewPaymentItem[];
}

// Phase 4: Specialized Operations

export interface GlobalAdminPayment {
  readonly id: string;
  readonly quote_id: string;
  readonly space_id: string;
  readonly space_title: string;
  readonly renter_id: string;
  readonly host_id: string;
  readonly status: string;
  readonly checkout_method: string;
  readonly test_mode: boolean;
  readonly base_clp: number;
  readonly commission_clp: number;
  readonly iva_clp: number;
  readonly buyer_fee_clp: number;
  readonly total_charged_clp: number;
  readonly provider_payment_id: string;
  readonly provider_payment_status: string;
  readonly reconciliation_reason: string;
  readonly payment_verified_at?: string | null;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface GlobalAdminPaymentQuery {
  readonly status?: string;
  readonly test_mode?: boolean;
  readonly account_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
}

export interface GlobalAdminReservation {
  readonly id: string;
  readonly checkout_operation_id: string;
  readonly quote_id: string;
  readonly space_id: string;
  readonly space_title: string;
  readonly renter_id: string;
  readonly host_id: string;
  readonly status: string;
  readonly modality: string;
  readonly unit_price_clp: number;
  readonly units: number;
  readonly subtotal_clp: number;
  readonly currency: string;
  readonly start_at: string;
  readonly end_at: string;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface GlobalAdminReservationQuery {
  readonly status?: string;
  readonly account_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
}

export interface GlobalAdminContract {
  readonly id: string;
  readonly proposal_id: string;
  readonly quote_id: string;
  readonly space_id: string;
  readonly space_title: string;
  readonly owner_id: string;
  readonly renter_id: string;
  readonly status: string;
  readonly signing_mode: string;
  readonly proposal_version: number;
  readonly subtotal_clp: number;
  readonly currency: string;
  readonly document_status: string;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface GlobalAdminContractQuery {
  readonly status?: string;
  readonly account_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
}

export interface GlobalAdminKycCase {
  readonly id: string;
  readonly account_id: string;
  readonly environment: string;
  readonly status: string;
  readonly id_country: string;
  readonly decision_status: string;
  readonly rejection_reason: string;
  readonly verified_at?: string | null;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface GlobalAdminKycCaseQuery {
  readonly status?: string;
  readonly account_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
}

export type GlobalAdminPrivacyRequestType =
  | "acceso"
  | "rectificacion"
  | "supresion"
  | "oposicion"
  | "portabilidad"
  | "bloqueo";

export type GlobalAdminPrivacyRequestStatus =
  "pendiente" | "en_revision" | "aprobada" | "rechazada" | "completada";

export type GlobalAdminPrivacyChannel =
  | "web"
  | "api"
  | "publico"
  | "correo";

export type GlobalAdminPrivacyRequesterType = "titular" | "representante";

export type GlobalAdminPrivacyRepresentativeRelation =
  | "mandatario"
  | "representante_legal"
  | "tutor_nna";

export interface AdminPrivacyNotificationSummary {
  readonly pending: number;
  readonly delivered: number;
  readonly terminal: number;
  readonly last_error_code?: string | null;
  readonly last_delivered_at?: string | null;
}

export interface GlobalAdminPrivacyRequest {
  readonly id: string;
  readonly account_id: string;
  readonly folio?: string;
  readonly right_type: GlobalAdminPrivacyRequestType;
  readonly status: GlobalAdminPrivacyRequestStatus;
  readonly reason: string;
  readonly case_ref: string;
  readonly requested_by?: string;
  readonly channel?: GlobalAdminPrivacyChannel;
  readonly requester_type?: GlobalAdminPrivacyRequesterType;
  readonly requester_name?: string;
  readonly requester_email_masked?: string;
  readonly representative_name?: string;
  readonly representative_relation?: GlobalAdminPrivacyRepresentativeRelation;
  readonly subject_reference?: string;
  readonly assigned_to_id?: string;
  readonly due_at?: string | null;
  readonly extended_due_at?: string | null;
  readonly extension_reason?: string | null;
  readonly version?: number;
  readonly is_overdue?: boolean;
  readonly created_at: string;
  readonly updated_at: string;
  readonly resolved_at?: string | null;
  readonly can_publish_response?: boolean;
  readonly can_record_dispatch?: boolean;
  readonly dispatch_recorded_at?: string | null;
  readonly detail?: string;
}

export interface GlobalAdminPrivacyRequestEvent {
  readonly id: string;
  readonly sequence: number;
  readonly event_type: string;
  readonly visibility: "titular" | "interna";
  readonly state_from?: string | null;
  readonly state_to?: string | null;
  readonly actor_id?: string | null;
  readonly detail?: string | null;
  readonly new_due_at?: string | null;
  readonly created_at: string;
}

export type GlobalAdminPrivacyExecutionAction =
  | "rectify_profile"
  | "apply_opposition"
  | "apply_block"
  | "lift_block"
  | "issue_portability";

export type AdminPrivacyExecutionType =
  | "rectificacion_perfil"
  | "oposicion_finalidad"
  | "bloqueo_aplicado"
  | "bloqueo_levantado"
  | "portabilidad_emitida"
  | "transferencia_portabilidad"
  | "propagacion_encargado";

export type AdminPrivacyTreatmentScope =
  | "comunicaciones_comerciales"
  | "analitica_y_perfilado"
  | "publicaciones_y_visibilidad"
  | "exportaciones_y_terceros"
  | "tratamiento_general";

export type AdminPrivacyPurpose =
  | "comunicaciones_comerciales"
  | "analitica_producto"
  | "personalizacion_recomendaciones"
  | "comparticion_encargados_opcionales";

export type AdminPrivacyPurposeState = "permitido" | "opuesto" | "retirado";

export interface AdminPrivacyRightExecution {
  readonly id: string;
  readonly request_id: string;
  readonly account_id?: string;
  readonly execution_type: AdminPrivacyExecutionType;
  readonly status: "ejecutada" | "parcial" | "denegada";
  readonly scope: string;
  readonly summary: string;
  readonly target_reference?: string;
  readonly artifact_sha256?: string;
  readonly executed_by: string;
  readonly executed_at: string;
}

export interface AdminPrivacyTreatmentBlock {
  readonly id: string;
  readonly request_id: string;
  readonly account_id?: string;
  readonly scope: AdminPrivacyTreatmentScope;
  readonly state: "activo" | "levantado";
  readonly reason: string;
  readonly applied_by?: string;
  readonly applied_at: string;
  readonly review_at?: string;
  readonly lifted_by?: string;
  readonly lifted_at?: string;
  readonly lift_reason?: string;
}

export interface AdminPrivacyPurposePreference {
  readonly purpose: AdminPrivacyPurpose;
  readonly state: AdminPrivacyPurposeState;
  readonly reason?: string;
  readonly request_id?: string;
  readonly updated_at?: string;
}

export interface GlobalAdminPrivacyExecutionInput {
  readonly action: GlobalAdminPrivacyExecutionAction;
  readonly expected_version: number;
  readonly summary: string;
  readonly display_name?: string;
  readonly phone?: string;
  readonly purpose?: AdminPrivacyPurpose;
  readonly purpose_state?: AdminPrivacyPurposeState;
  readonly scope?: AdminPrivacyTreatmentScope;
  readonly block_id?: string;
  readonly review_at?: string;
  readonly target_reference?: string;
}

export interface GlobalAdminPrivacyRequestDetail extends GlobalAdminPrivacyRequest {
  readonly response?: string | null;
  readonly notification_summary?: AdminPrivacyNotificationSummary;
  readonly events: readonly GlobalAdminPrivacyRequestEvent[];
  readonly executions?: AdminPrivacyRightExecution[];
  readonly treatment_blocks?: AdminPrivacyTreatmentBlock[];
  readonly purpose_preferences?: AdminPrivacyPurposePreference[];
}

export type GlobalAdminPrivacyWorkflowAction =
  | "assign"
  | "verify_identity"
  | "start_review"
  | "request_clarification"
  | "add_internal_note"
  | "extend_deadline";

export interface GlobalAdminPrivacyWorkflowInput {
  readonly action: GlobalAdminPrivacyWorkflowAction;
  readonly expected_version: number;
  readonly assigned_to_id?: string;
  readonly account_id?: string;
  readonly note?: string;
  readonly extra_days?: number;
}

export interface GlobalAdminPrivacyDispatchReceiptInput {
  readonly channel: "email" | "postal";
  readonly destination: string;
  readonly sent_at: string;
  readonly evidence_ref: string;
}

export interface GlobalAdminPrivacyRequestResolutionInput {
  readonly decision: "resuelta" | "denegada_fundada";
  readonly response: string;
}

export interface GlobalAdminPrivacyRequestQuery {
  readonly status?: GlobalAdminPrivacyRequestStatus;
  readonly right_type?: GlobalAdminPrivacyRequestType;
  readonly account_id?: string;
  readonly assigned_to_id?: string;
  readonly channel?: GlobalAdminPrivacyChannel;
  readonly overdue?: boolean;
  readonly cursor?: string;
  readonly page_size?: number;
}

export type AdminPrivacyRequestItem = GlobalAdminPrivacyRequest;
export type AdminPrivacyRequestDetail = GlobalAdminPrivacyRequestDetail;
export type TransitionAdminPrivacyRequestInput = GlobalAdminPrivacyWorkflowInput;

export interface GlobalAdminPrivacyRequestCreateInput {
  readonly account_id: string;
  readonly right_type: GlobalAdminPrivacyRequestType;
  readonly reason: string;
  readonly case_ref?: string;
}

export type GlobalAdminSupportTicketCategory =
  | "pago"
  | "reserva"
  | "contrato"
  | "kyc"
  | "privacidad"
  | "cuenta"
  | (string & {});

export type GlobalAdminSupportTicketPriority =
  "baja" | "media" | "alta" | "critica";

export type GlobalAdminSupportTicketStatus =
  "abierto" | "en_progreso" | "resuelto" | "cerrado";

export interface GlobalAdminSupportTicket {
  readonly id: string;
  readonly account_id?: string;
  readonly assigned_to_id?: string;
  readonly category: GlobalAdminSupportTicketCategory;
  readonly priority: GlobalAdminSupportTicketPriority;
  readonly status: GlobalAdminSupportTicketStatus;
  readonly subject: string;
  readonly case_ref: string;
  readonly created_at: string;
  readonly updated_at: string;
}

export interface GlobalAdminSupportTicketQuery {
  readonly status?: GlobalAdminSupportTicketStatus;
  readonly priority?: GlobalAdminSupportTicketPriority;
  readonly account_id?: string;
  readonly cursor?: string;
  readonly page_size?: number;
}

export interface GlobalAdminSupportTicketCreateInput {
  readonly account_id?: string;
  readonly assigned_to_id?: string;
  readonly category: GlobalAdminSupportTicketCategory;
  readonly priority: GlobalAdminSupportTicketPriority;
  readonly subject: string;
  readonly case_ref?: string;
}

export type GlobalAdminApprovalActionType =
  "refund_request" | "privacy_export" | "staff_role_grant";

export type GlobalAdminApprovalStatus =
  "pending" | "approved" | "rejected" | "cancelled";

export type GlobalAdminApprovalDecision = "approved" | "rejected" | "cancelled";

export interface GlobalAdminApproval {
  readonly id: string;
  readonly action_type: GlobalAdminApprovalActionType;
  readonly resource_type: string;
  readonly resource_id: string;
  readonly requester_id: string;
  readonly approver_id?: string | null;
  readonly status: GlobalAdminApprovalStatus;
  readonly reason: string;
  readonly decision_reason: string;
  readonly payload?: Readonly<Record<string, unknown>>;
  readonly idempotency_key: string;
  readonly version: number;
  readonly created_at: string;
  readonly decided_at?: string | null;
  readonly expires_at: string;
}

export interface GlobalAdminApprovalQuery {
  readonly status?: GlobalAdminApprovalStatus;
  readonly action_type?: GlobalAdminApprovalActionType;
  readonly cursor?: string;
  readonly page_size?: number;
}

export interface GlobalAdminApprovalCreateInput {
  readonly action_type: GlobalAdminApprovalActionType;
  readonly resource_type: string;
  readonly resource_id: string;
  readonly reason: string;
  readonly idempotency_key?: string;
  readonly payload?: Readonly<Record<string, unknown>>;
}

export interface GlobalAdminApprovalDecisionInput {
  readonly decision: GlobalAdminApprovalDecision;
  readonly decision_reason: string;
  readonly expected_version: number;
}

// Phase 7: Privacy Incidents & Legal Holds

export type GlobalAdminPrivacyIncidentSeverity =
  | "baja"
  | "media"
  | "alta"
  | "critica";

export type GlobalAdminPrivacyIncidentStatus =
  | "abierto"
  | "contenido"
  | "evaluado"
  | "notificado"
  | "cerrado";

export type GlobalAdminPrivacyIncidentAction =
  | "contain"
  | "evaluate"
  | "notify_agency"
  | "notify_subjects"
  | "add_note"
  | "close";

export type GlobalAdminPrivacyIncidentEventType =
  | "apertura"
  | "contencion"
  | "evaluacion"
  | "notificacion_agencia"
  | "notificacion_titulares"
  | "nota"
  | "cierre";

export interface GlobalAdminPrivacyIncident {
  readonly id: string;
  readonly folio: string;
  readonly title: string;
  readonly severity: GlobalAdminPrivacyIncidentSeverity;
  readonly status: GlobalAdminPrivacyIncidentStatus;
  readonly data_categories: readonly string[];
  readonly affected_systems: readonly string[];
  readonly estimated_subjects: number;
  readonly summary: string;
  readonly containment_measures?: string | null;
  readonly requires_agency_notice: boolean;
  readonly agency_notified_at?: string | null;
  readonly agency_reference?: string | null;
  readonly requires_subject_notice: boolean;
  readonly subjects_notified_at?: string | null;
  readonly leader_id: string;
  readonly deputy_id?: string | null;
  readonly detected_at: string;
  readonly created_by: string;
  readonly created_at: string;
  readonly updated_at: string;
  readonly closed_at?: string | null;
  readonly version: number;
}

export interface GlobalAdminPrivacyIncidentEvent {
  readonly id: string;
  readonly sequence: number;
  readonly event_type: GlobalAdminPrivacyIncidentEventType;
  readonly actor_id: string;
  readonly detail: string;
  readonly created_at: string;
}

export interface GlobalAdminPrivacyIncidentDetail extends GlobalAdminPrivacyIncident {
  readonly events: readonly GlobalAdminPrivacyIncidentEvent[];
}

export interface GlobalAdminPrivacyIncidentPage {
  readonly items: readonly GlobalAdminPrivacyIncident[];
  readonly next_cursor?: string;
  readonly matching_total: number;
  readonly generated_at: string;
}

export interface GlobalAdminPrivacyIncidentQuery {
  readonly status?: GlobalAdminPrivacyIncidentStatus;
  readonly severity?: GlobalAdminPrivacyIncidentSeverity;
  readonly page_size?: number;
  readonly cursor?: string;
}

export interface CreateGlobalAdminPrivacyIncidentInput {
  readonly title: string;
  readonly severity: GlobalAdminPrivacyIncidentSeverity;
  readonly data_categories: readonly string[];
  readonly affected_systems: readonly string[];
  readonly estimated_subjects?: number;
  readonly summary: string;
  readonly containment_measures?: string;
  readonly requires_agency_notice?: boolean;
  readonly requires_subject_notice?: boolean;
  readonly leader_id?: string;
  readonly deputy_id?: string;
  readonly detected_at?: string;
}

export interface TransitionGlobalAdminPrivacyIncidentInput {
  readonly action: GlobalAdminPrivacyIncidentAction;
  readonly expected_version: number;
  readonly detail?: string;
  readonly containment_measures?: string;
  readonly requires_agency_notice?: boolean;
  readonly requires_subject_notice?: boolean;
  readonly agency_reference?: string;
}

export type GlobalAdminPrivacyLegalHoldScope =
  | "expediente_privacidad"
  | "cuenta_completa"
  | "evidencia_kyc"
  | "contratos_y_pagos"
  | "incidente_seguridad";

export type GlobalAdminPrivacyLegalHoldStatus = "activa" | "liberada";

export interface GlobalAdminPrivacyLegalHold {
  readonly id: string;
  readonly folio: string;
  readonly account_id?: string | null;
  readonly request_id?: string | null;
  readonly incident_id?: string | null;
  readonly scope: GlobalAdminPrivacyLegalHoldScope;
  readonly authority: string;
  readonly legal_basis: string;
  readonly status: GlobalAdminPrivacyLegalHoldStatus;
  readonly review_at?: string | null;
  readonly applied_by: string;
  readonly applied_at: string;
  readonly released_by?: string | null;
  readonly released_at?: string | null;
  readonly release_reason?: string | null;
}

export interface GlobalAdminPrivacyLegalHoldPage {
  readonly items: readonly GlobalAdminPrivacyLegalHold[];
  readonly next_cursor?: string;
  readonly matching_total: number;
  readonly generated_at: string;
}

export interface GlobalAdminPrivacyLegalHoldQuery {
  readonly status?: GlobalAdminPrivacyLegalHoldStatus;
  readonly account_id?: string;
  readonly scope?: GlobalAdminPrivacyLegalHoldScope;
  readonly page_size?: number;
  readonly cursor?: string;
}

export interface CreateGlobalAdminPrivacyLegalHoldInput {
  readonly account_id?: string;
  readonly request_id?: string;
  readonly incident_id?: string;
  readonly scope: GlobalAdminPrivacyLegalHoldScope;
  readonly authority: string;
  readonly legal_basis: string;
  readonly review_at?: string;
}

export interface ReleaseGlobalAdminPrivacyLegalHoldInput {
  readonly release_reason: string;
}
