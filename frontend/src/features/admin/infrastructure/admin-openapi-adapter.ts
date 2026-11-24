import "client-only";

import { apiClient } from "@/lib/api/client";
import {
  ApiError,
  ApiProtocolError,
  normalizeApiError,
} from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { AdminPort } from "../application/admin.port";
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

export class OpenApiAdminAdapter implements AdminPort {
  // -----------------------------------------------------
  // Dedicated Admin Session
  // -----------------------------------------------------

  async getAdminSession(signal?: AbortSignal): Promise<AdminSessionData | null> {
    const result = await apiClient.GET("/admin/session", { signal });
    if (result.response?.status === 401) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminSessionData;
  }

  async startAdminSession(
    input: { email: string; password: string },
    signal?: AbortSignal,
  ): Promise<AdminSessionStartResponse> {
    const result = await apiClient.POST("/admin/session/start", {
      body: input,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminSessionStartResponse;
  }

  async verifyAdminSessionMfa(
    input: { challenge_id?: string; code: string },
    signal?: AbortSignal,
  ): Promise<AdminSessionData> {
    const result = await apiClient.POST("/admin/session/mfa", {
      body: input,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminSessionData;
  }

  async deleteAdminSession(signal?: AbortSignal): Promise<void> {
    const result = await apiClient.DELETE("/admin/session", { signal });
    if (!result.response.ok && result.response.status !== 204) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async requestGlobalAccountSensitiveAccess(
    accountId: string,
    input: AdminSensitiveAccessInput,
    signal?: AbortSignal,
  ): Promise<AdminSensitiveAccessResult> {
    const result = await apiClient.POST(
      "/admin/global/accounts/{accountID}/sensitive-access",
      {
        params: { path: { accountID: accountId } },
        body: input,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminSensitiveAccessResult;
  }

  async startAssistedView(
    accountId: string,
    input: AdminAssistedViewStartInput,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewGrant> {
    const result = await apiClient.POST(
      "/admin/global/accounts/{accountID}/assisted-view",
      {
        params: { path: { accountID: accountId } },
        body: input,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminAssistedViewGrant;
  }

  async getAssistedViewProjection(
    grantId: string,
    signal?: AbortSignal,
  ): Promise<AdminAssistedViewProjection> {
    const result = await apiClient.GET(
      "/admin/global/assisted-view/{grantID}/projection",
      {
        params: { path: { grantID: grantId } },
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as AdminAssistedViewProjection;
  }

  async closeAssistedView(
    grantId: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.DELETE(
      "/admin/global/assisted-view/{grantID}",
      {
        params: { path: { grantID: grantId } },
        signal,
      },
    );
    if (!result.response.ok && result.response.status !== 204) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  // -----------------------------------------------------
  // Global Admin
  // -----------------------------------------------------

  async getGlobalAdminMe(signal?: AbortSignal): Promise<GlobalAdminMe> {
    const result = await apiClient.GET("/admin/global/me", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as GlobalAdminMe;
  }

  async getGlobalAdminOverview(
    signal?: AbortSignal,
  ): Promise<GlobalAdminOverview> {
    const result = await apiClient.GET("/admin/global/overview", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: GlobalAdminOverview } | undefined;
    const data =
      envelope?.data ?? (result.data as unknown as GlobalAdminOverview);
    if (!data) throw new ApiProtocolError();
    return data;
  }

  async getGlobalAdminKPIs(
    query: { from: string; until: string; time_zone: string },
    signal?: AbortSignal,
  ): Promise<GlobalAdminKPIs> {
    const result = await apiClient.GET("/admin/global/kpis", {
      params: { query },
      signal,
    });
    if (result.response?.status === 404) {
      return {
        period: {
          from: query.from,
          until: query.until,
          time_zone: query.time_zone,
        },
        generated_at: new Date().toISOString(),
        definition: "kpis_fallback",
        metrics: {},
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: GlobalAdminKPIs } | undefined;
    const data = envelope?.data ?? (result.data as unknown as GlobalAdminKPIs);
    if (!data) throw new ApiProtocolError();
    return data;
  }

  async listGlobalAuditEvents(
    query: AuditEventsQuery,
    signal?: AbortSignal,
  ): Promise<AuditEvent[]> {
    const result = await apiClient.GET("/admin/global/audit-events", {
      params: {
        query: query as never,
      },
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const payload = result.data as Record<string, unknown> | undefined;
    const dataObj = payload?.data as Record<string, unknown> | undefined;
    return (
      (dataObj?.items as unknown as AuditEvent[]) ??
      (payload?.items as unknown as AuditEvent[]) ??
      []
    );
  }

  async exportGlobalAuditEvents(
    query: AuditEventsQuery,
    signal?: AbortSignal,
  ): Promise<Blob> {
    const result = await apiClient.GET("/admin/global/audit-events/export", {
      params: {
        query: {
          ...query,
          snapshot_cutoff: query.snapshot_cutoff ?? new Date().toISOString(),
        } as never,
      },
      parseAs: "blob",
      signal,
    });
    if (result.response?.status === 404) {
      return new Blob(["[]"], { type: "application/json" });
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async listGlobalAccounts(
    query?: GlobalAdminAccountQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminAccount[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/accounts", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: GlobalAdminAccount[]; next_cursor?: string };
          items?: GlobalAdminAccount[];
          next_cursor?: string;
        }
      | undefined;
    return {
      items:
        (envelope?.data?.items as unknown as GlobalAdminAccount[]) ??
        (envelope?.items as unknown as GlobalAdminAccount[]) ??
        [],
      next_cursor:
        envelope?.data?.next_cursor ?? envelope?.next_cursor ?? undefined,
    };
  }

  async getGlobalAccount(
    accountId: string,
    signal?: AbortSignal,
    revealEmail?: true,
  ): Promise<GlobalAdminAccount | null> {
    const result = await apiClient.GET("/admin/global/accounts/{accountID}", {
      params: {
        path: { accountID: accountId },
        query: { reveal_email: revealEmail },
      },
      signal,
    });
    if (result.response?.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: GlobalAdminAccount } | undefined;
    return (
      (envelope?.data as unknown as GlobalAdminAccount) ??
      (result.data as unknown as GlobalAdminAccount) ??
      null
    );
  }

  async listGlobalAccountResources(
    accountId: string,
    kind: GlobalAdminAccountResourceKind,
    query?: GlobalAdminAccountResourceQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminAccountResourcePage> {
    const result = await apiClient.GET(
      "/admin/global/accounts/{accountID}/resources/{resourceKind}",
      {
        params: {
          path: { accountID: accountId, resourceKind: kind },
          query: query as never,
        },
        signal,
      },
    );
    if (result.response?.status === 404) {
      return {
        items: [],
        next_cursor: "",
        matching_total: 0,
        generated_at: new Date().toISOString(),
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | { data?: GlobalAdminAccountResourcePage }
      | GlobalAdminAccountResourcePage
      | undefined;
    const page =
      envelope && "data" in envelope && envelope.data
        ? envelope.data
        : (envelope as GlobalAdminAccountResourcePage | undefined);
    return {
      items: page?.items ?? [],
      next_cursor: page?.next_cursor ?? "",
      matching_total: page?.matching_total ?? 0,
      generated_at: page?.generated_at ?? new Date().toISOString(),
    };
  }

  async updateGlobalAccountProfile(
    accountId: string,
    input: {
      display_name: string;
      phone?: string | null;
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.PATCH(
      "/admin/global/accounts/{accountID}/profile",
      {
        params: { path: { accountID: accountId } },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async setGlobalAccountRole(
    accountId: string,
    role: "arrendatario" | "arrendador",
    granted: boolean,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.PUT(
      "/admin/global/accounts/{accountID}/roles/{role}",
      {
        params: { path: { accountID: accountId, role } },
        body: { granted, reason },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async setGlobalLegacyAdminRole(
    accountId: string,
    granted: boolean,
    reason:
      | "aprovisionamiento_sintetico"
      | "revision_administrativa"
      | "retirada_acceso",
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.PUT(
      "/admin/global/accounts/{accountID}/legacy-administrator",
      {
        params: { path: { accountID: accountId } },
        body: { granted, reason },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async revokeGlobalAccountSessions(
    accountId: string,
    reason:
      "correccion_perfil" | "revision_administrativa" | "gestion_operativa",
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.POST(
      "/admin/global/accounts/{accountID}/sessions/revoke",
      {
        params: { path: { accountID: accountId } },
        body: { reason },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async setGlobalAccountBlocked(
    accountId: string,
    blocked: boolean,
    reason: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.POST(
      "/admin/global/accounts/{accountID}/block",
      {
        params: { path: { accountID: accountId } },
        body: {
          blocked,
          reason: reason as never,
        },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async listGlobalPublications(
    query?: GlobalAdminPublicationQuery,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPublication[]> {
    const page = await this.listGlobalPublicationPage(query, signal).catch(
      (error: unknown) => {
        if (error instanceof ApiError && error.status === 404) {
          return { items: [], nextCursor: "" };
        }
        throw error;
      },
    );
    return page.items;
  }

  async listGlobalPublicationPage(
    query?: GlobalAdminPublicationQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminPublication[]; nextCursor: string }> {
    const result = await apiClient.GET("/admin/global/publications", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: GlobalAdminPublication[]; next_cursor?: string };
          items?: GlobalAdminPublication[];
          next_cursor?: string;
        }
      | undefined;
    const page = envelope?.data ?? envelope;
    return { items: page?.items ?? [], nextCursor: page?.next_cursor ?? "" };
  }

  async setGlobalPublicationState(
    publicationId: string,
    input: {
      state: "activa" | "oculta";
      expected_updated_at: string;
      reason:
        "correccion_perfil" | "revision_administrativa" | "gestion_operativa";
    },
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.POST(
      "/admin/global/publications/{publicationID}/state",
      {
        params: { path: { publicationID: publicationId } },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  async retireGlobalPublication(
    publicationId: string,
    input: {
      owner_id: string;
      expected_updated_at: string;
      reason: string;
    },
    signal?: AbortSignal,
  ): Promise<void> {
    const result = await apiClient.POST(
      "/admin/global/publications/{publicationID}/retire",
      {
        params: { path: { publicationID: publicationId } },
        body: input as never,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
  }

  // -----------------------------------------------------
  // Local / Operativa Admin
  // -----------------------------------------------------

  async listVerifications(
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase[]> {
    const result = await apiClient.GET("/admin/verifications", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: AdminVerificationCase[] };
          items?: AdminVerificationCase[];
        }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async listRejectedVerifications(
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase[]> {
    const result = await apiClient.GET("/admin/verifications/failed", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: AdminVerificationCase[] };
          items?: AdminVerificationCase[];
        }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async reviewVerification(
    id: string,
    decision: "aprobar" | "rechazar",
    reason?: string,
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase> {
    const result = await apiClient.POST("/admin/verifications/{id}/review", {
      params: { path: { id } },
      body: { decision, reason } as never,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data as unknown as AdminVerificationCase;
  }

  async revokeVerification(
    id: string,
    idempotencyKey: string,
    reason_code:
      "aprobacion_fixture_incorrecta" | "revision_fixture_actualizada",
    signal?: AbortSignal,
  ): Promise<AdminVerificationCase> {
    const result = await apiClient.POST("/admin/verifications/{id}/revoke", {
      params: {
        path: { id },
        header: { "Idempotency-Key": idempotencyKey },
      },
      body: { reason_code },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data as unknown as AdminVerificationCase;
  }

  async listDisputes(signal?: AbortSignal): Promise<AdminLocalDispute[]> {
    const result = await apiClient.GET("/admin/disputes", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | { data?: { items?: AdminLocalDispute[] }; items?: AdminLocalDispute[] }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async closeDispute(
    disputeId: string,
    reason_code: "ensayo_finalizado" | "registro_erroneo" | "duplicada",
    signal?: AbortSignal,
  ): Promise<AdminLocalDispute> {
    const result = await apiClient.POST("/admin/disputes/{dispute_id}/close", {
      params: { path: { dispute_id: disputeId } },
      body: { reason_code },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data as unknown as AdminLocalDispute;
  }

  async listReviewReports(signal?: AbortSignal): Promise<AdminReviewReport[]> {
    const result = await apiClient.GET("/admin/local/review-reports", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | { data?: { items?: AdminReviewReport[] }; items?: AdminReviewReport[] }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async moderateReviewReport(
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
  ): Promise<AdminReviewReport> {
    const result = await apiClient.POST(
      "/admin/local/review-reports/{reportID}/{action}",
      {
        params: {
          path: { reportID: reportId, action },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.data as unknown as AdminReviewReport;
  }

  async listNotices(signal?: AbortSignal): Promise<AdminLocalNotice[]> {
    const result = await apiClient.GET("/admin/local/notices", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | { data?: { items?: AdminLocalNotice[] }; items?: AdminLocalNotice[] }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async reopenNotice(
    noticeId: string,
    idempotencyKey: string,
    input: {
      reason_code: "smtp_restaurado" | "reintento_operativo";
      correlation_id: string;
    },
    signal?: AbortSignal,
  ): Promise<AdminLocalNotice> {
    const result = await apiClient.POST(
      "/admin/local/notices/{noticeID}/reopen",
      {
        params: {
          path: { noticeID: noticeId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.data as unknown as AdminLocalNotice;
  }

  async listLocalAuditEvents(
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
  ): Promise<AuditEvent[]> {
    const result = await apiClient.GET("/admin/local/audit-events", {
      params: { query: query as never },
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      { data?: { items?: AuditEvent[] }; items?: AuditEvent[] } | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async exportLocalAuditEvents(
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
  ): Promise<Blob> {
    const result = await apiClient.GET("/admin/local/audit-events/export", {
      params: { query: query as never },
      parseAs: "blob",
      signal,
    });
    if (result.response?.status === 404) {
      return new Blob(["[]"], { type: "application/json" });
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async getLocalAccount(
    accountId: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount | null> {
    const result = await apiClient.GET("/admin/local/accounts/{accountID}", {
      params: { path: { accountID: accountId } },
      signal,
    });
    if (result.response?.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: AdminLocalAccount } | undefined;
    return (
      envelope?.data ?? (result.data as unknown as AdminLocalAccount) ?? null
    );
  }

  async blockLocalAccount(
    accountId: string,
    reason: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount> {
    const result = await apiClient.POST(
      "/admin/local/accounts/{accountID}/block",
      {
        params: { path: { accountID: accountId } },
        body: { reason } as never,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.data as unknown as AdminLocalAccount;
  }

  async unblockLocalAccount(
    accountId: string,
    reason: string,
    signal?: AbortSignal,
  ): Promise<AdminLocalAccount> {
    const result = await apiClient.POST(
      "/admin/local/accounts/{accountID}/unblock",
      {
        params: { path: { accountID: accountId } },
        body: { reason } as never,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.data as unknown as AdminLocalAccount;
  }

  async listPublishedSpaces(
    signal?: AbortSignal,
  ): Promise<AdminPublishedSpace[]> {
    const result = await apiClient.GET("/admin/local/published-spaces", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: AdminPublishedSpace[] };
          items?: AdminPublishedSpace[];
        }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async getPublishedSpace(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<AdminPublishedSpace | null> {
    const result = await apiClient.GET(
      "/admin/local/published-spaces/{spaceID}",
      {
        params: { path: { spaceID: spaceId } },
        signal,
      },
    );
    if (result.response?.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: AdminPublishedSpace } | undefined;
    return (
      envelope?.data ?? (result.data as unknown as AdminPublishedSpace) ?? null
    );
  }

  async getReservationsReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminReservationsReport> {
    const result = await apiClient.GET("/admin/local/reports/reservations", {
      params: { query },
      signal,
    });
    if (result.response?.status === 404) {
      return {
        generated_at: new Date().toISOString(),
        from: query.from,
        until: query.until,
        timezone: query.timezone,
        total_reservations: 0,
        by_state: {},
        test_notice: "Sin registros en este período",
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      { data?: AdminReservationsReport } | undefined;
    const data =
      envelope?.data ?? (result.data as unknown as AdminReservationsReport);
    if (!data) throw new ApiProtocolError();
    return data;
  }

  async getFinanceReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminFinanceReport> {
    const result = await apiClient.GET("/admin/local/reports/finance", {
      params: { query },
      signal,
    });
    if (result.response?.status === 404) {
      return {
        generated_at: new Date().toISOString(),
        from: query.from,
        until: query.until,
        timezone: query.timezone,
        confirmed_movements_clp: 0,
        pending_obligations_clp: 0,
        fee_volume_clp: 0,
        currency: "CLP",
        test_notice: "Sin registros en este período",
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: AdminFinanceReport } | undefined;
    const data =
      envelope?.data ?? (result.data as unknown as AdminFinanceReport);
    if (!data) throw new ApiProtocolError();
    return data;
  }

  async getDisputesReport(
    query: { from: string; until: string; timezone: string },
    signal?: AbortSignal,
  ): Promise<AdminDisputesReport> {
    const result = await apiClient.GET("/admin/local/reports/disputes", {
      params: { query },
      signal,
    });
    if (result.response?.status === 404) {
      return {
        generated_at: new Date().toISOString(),
        from: query.from,
        until: query.until,
        timezone: query.timezone,
        total_disputes: 0,
        open_disputes: 0,
        closed_disputes: 0,
        test_notice: "Sin registros en este período",
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: AdminDisputesReport } | undefined;
    const data =
      envelope?.data ?? (result.data as unknown as AdminDisputesReport);
    if (!data) throw new ApiProtocolError();
    return data;
  }

  async listLocalReservations(
    query?: {
      reservation_id?: string;
      state?: string;
      created_from?: string;
      created_to?: string;
      page_size?: number;
      cursor?: string;
    },
    signal?: AbortSignal,
  ): Promise<AdminReservationSummary[]> {
    const result = await apiClient.GET("/admin/local/reservations", {
      params: { query: query as never },
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | {
          data?: { items?: AdminReservationSummary[] };
          items?: AdminReservationSummary[];
        }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async getLocalReservation(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<AdminReservationDetail | null> {
    const result = await apiClient.GET(
      "/admin/local/reservations/{reservationID}",
      {
        params: { path: { reservationID: reservationId } },
        signal,
      },
    );
    if (result.response?.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      { data?: AdminReservationDetail } | undefined;
    return (
      envelope?.data ??
      (result.data as unknown as AdminReservationDetail) ??
      null
    );
  }

  async listDamageClaims(signal?: AbortSignal): Promise<AdminDamageClaim[]> {
    const result = await apiClient.GET("/admin/local/damage-claims", {
      signal,
    });
    if (result.response?.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as
      | { data?: { items?: AdminDamageClaim[] }; items?: AdminDamageClaim[] }
      | undefined;
    return envelope?.data?.items ?? envelope?.items ?? [];
  }

  async getDamageClaim(
    claimId: string,
    signal?: AbortSignal,
  ): Promise<AdminDamageClaim | null> {
    const result = await apiClient.GET("/admin/local/damage-claims/{claimID}", {
      params: { path: { claimID: claimId } },
      signal,
    });
    if (result.response?.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const envelope = result.data as { data?: AdminDamageClaim } | undefined;
    return (
      envelope?.data ?? (result.data as unknown as AdminDamageClaim) ?? null
    );
  }

  async resolveDamageClaim(
    claimId: string,
    idempotencyKey: string,
    input: AdminDamageClaimResolutionInput,
    signal?: AbortSignal,
  ): Promise<AdminDamageClaim> {
    const result = await apiClient.POST(
      "/admin/local/damage-claims/{claimID}/resolution",
      {
        params: {
          path: { claimID: claimId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input as never,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data?.data as unknown as AdminDamageClaim;
  }

  private unwrapPage<T>(data: unknown): {
    items: T[];
    next_cursor?: string;
    matching_total?: number;
    generated_at?: string;
  } {
    const envelope = data as
      | {
          data?: {
            items?: T[];
            next_cursor?: string;
            matching_total?: number;
            generated_at?: string;
          };
          items?: T[];
          next_cursor?: string;
          matching_total?: number;
          generated_at?: string;
        }
      | undefined;
    return {
      items: envelope?.data?.items ?? envelope?.items ?? [],
      next_cursor: envelope?.data?.next_cursor ?? envelope?.next_cursor,
      matching_total:
        envelope?.data?.matching_total ?? envelope?.matching_total,
      generated_at: envelope?.data?.generated_at ?? envelope?.generated_at,
    };
  }

  async listGlobalPayments(
    query?: GlobalAdminPaymentQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminPayment[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/payments", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminPayment>(result.data);
  }

  async listGlobalReservations(
    query?: GlobalAdminReservationQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminReservation[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/reservations", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminReservation>(result.data);
  }

  async listGlobalContracts(
    query?: GlobalAdminContractQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminContract[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/contracts", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminContract>(result.data);
  }

  async listGlobalKycCases(
    query?: GlobalAdminKycCaseQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminKycCase[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/kyc-cases", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminKycCase>(result.data);
  }

  async listGlobalPrivacyRequests(
    query?: GlobalAdminPrivacyRequestQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminPrivacyRequest[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/privacy-requests", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminPrivacyRequest>(result.data);
  }

  async createGlobalPrivacyRequest(
    idempotencyKey: string,
    input: GlobalAdminPrivacyRequestCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminPrivacyRequest> {
    const result = await apiClient.POST("/admin/global/privacy-requests", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: input as never,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as GlobalAdminPrivacyRequest;
  }

  async listGlobalSupportTickets(
    query?: GlobalAdminSupportTicketQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminSupportTicket[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/support-tickets", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminSupportTicket>(result.data);
  }

  async createGlobalSupportTicket(
    idempotencyKey: string,
    input: GlobalAdminSupportTicketCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminSupportTicket> {
    const result = await apiClient.POST("/admin/global/support-tickets", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: input as never,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as GlobalAdminSupportTicket;
  }

  async listGlobalApprovals(
    query?: GlobalAdminApprovalQuery,
    signal?: AbortSignal,
  ): Promise<{ items: GlobalAdminApproval[]; next_cursor?: string }> {
    const result = await apiClient.GET("/admin/global/approvals", {
      params: { query: query as never },
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return this.unwrapPage<GlobalAdminApproval>(result.data);
  }

  async createGlobalApproval(
    idempotencyKey: string,
    input: GlobalAdminApprovalCreateInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval> {
    const body = {
      ...input,
      idempotency_key: input.idempotency_key ?? idempotencyKey,
    };
    const result = await apiClient.POST("/admin/global/approvals", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: body as never,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as GlobalAdminApproval;
  }

  async decideGlobalApproval(
    approvalId: string,
    idempotencyKey: string,
    input: GlobalAdminApprovalDecisionInput,
    signal?: AbortSignal,
  ): Promise<GlobalAdminApproval> {
    const result = await apiClient.POST(
      "/admin/global/approvals/{approvalID}/decisions",
      {
        params: {
          path: { approvalID: approvalId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input as never,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as GlobalAdminApproval;
  }
}
