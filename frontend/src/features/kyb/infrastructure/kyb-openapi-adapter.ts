import "client-only";

import type { AccessToken } from "@/features/auth/application/auth.port";
import { apiClient } from "@/lib/api/client";
import { normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type {
  KYBDocumentDecision,
  KYBSubmissionInput,
  KYBVerification,
} from "../domain/kyb";

export interface KYBComplianceProfile {
  owner_usuario_id: string;
  organization_id?: string;
  legal_name?: string;
  compliance_status:
    "grace_active" | "in_review" | "compliant" | "disabled_missing_docs";
  sii_giro_code?: string;
  sii_status?: string;
  grace_started_at?: string;
  grace_due_at?: string;
  disabled_at?: string;
  disabled_reason?: string;
}

export interface KYBComplianceDocument {
  id: string;
  owner_usuario_id: string;
  organization_id?: string;
  submitted_by?: string;
  space_id?: string;
  document_kind: "sii_start_activities" | "property_or_sublease_authorization";
  status: "PENDING" | "IN_REVIEW" | "APPROVED" | "REJECTED";
  mime_type: string;
  size_bytes: number;
  sii_giro_code?: string;
  category_code?: string;
  submitted_at: string;
  rejection_reason?: string;
}

function auth(token: AccessToken) {
  return { Authorization: ["Bearer", token].join(" ") };
}

export class KYBOpenApiAdapter {
  async listCompliance(
    token: AccessToken,
    status: string,
    signal: AbortSignal,
  ): Promise<KYBComplianceProfile[]> {
    if (!token) return [];
    const query = status ? `?status=${encodeURIComponent(status)}` : "";
    const response = await fetch(`/api/backend/admin/kyb/compliance${query}`, {
      signal,
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!response.ok) throw normalizeApiError(response, undefined);
    const body = (await response.json()) as { items?: KYBComplianceProfile[] };
    return body.items ?? [];
  }

  async setCompliance(
    token: AccessToken,
    item: KYBComplianceProfile,
    disabled: boolean,
    reason: string,
    signal: AbortSignal,
  ): Promise<void> {
    if (!token) throw new Error("Inicia sesión para continuar.");
    const path = item.organization_id
      ? `/api/backend/admin/kyb/compliance/organizations/${item.organization_id}/${disabled ? "disable" : "enable"}`
      : `/api/backend/admin/kyb/compliance/${item.owner_usuario_id}/${disabled ? "disable" : "enable"}`;
    const response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: disabled ? JSON.stringify({ reason }) : "{}",
      signal,
    });
    if (!response.ok) throw normalizeApiError(response, undefined);
  }

  async listPendingComplianceDocuments(
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<KYBComplianceDocument[]> {
    if (!token) throw new Error("Inicia sesión para continuar.");
    const response = await fetch(
      "/api/backend/admin/kyb/compliance/documents",
      { signal, cache: "no-store", credentials: "same-origin" },
    );
    if (!response.ok) throw normalizeApiError(response, undefined);
    return (
      ((await response.json()) as { items?: KYBComplianceDocument[] }).items ??
      []
    );
  }

  async startComplianceDocumentReview(
    token: AccessToken,
    id: string,
    signal: AbortSignal,
  ): Promise<void> {
    if (!token) throw new Error("Inicia sesión para continuar.");
    const response = await fetch(
      `/api/backend/admin/kyb/compliance/documents/${id}/review`,
      { method: "POST", signal, credentials: "same-origin" },
    );
    if (!response.ok) throw normalizeApiError(response, undefined);
  }

  async complianceDocumentURL(
    token: AccessToken,
    id: string,
    signal: AbortSignal,
  ): Promise<{ download_url: string; expires_at: string }> {
    if (!token) throw new Error("Inicia sesión para continuar.");
    const response = await fetch(
      `/api/backend/admin/kyb/compliance/documents/${id}/url`,
      { signal, credentials: "same-origin" },
    );
    if (!response.ok) throw normalizeApiError(response, undefined);
    return (await response.json()) as {
      download_url: string;
      expires_at: string;
    };
  }

  async decideComplianceDocument(
    token: AccessToken,
    id: string,
    decision: "APPROVE" | "REJECT",
    rejectionReason: string,
    signal: AbortSignal,
  ): Promise<void> {
    if (!token) throw new Error("Inicia sesión para continuar.");
    const response = await fetch(
      `/api/backend/admin/kyb/compliance/documents/${id}/decision`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(
          decision === "REJECT"
            ? { decision, rejection_reason: rejectionReason }
            : { decision },
        ),
        signal,
      },
    );
    if (!response.ok) throw normalizeApiError(response, undefined);
  }

  async getOwn(
    token: AccessToken,
    signal: AbortSignal,
    organizationId: string,
  ): Promise<KYBVerification | null> {
    const result = await apiClient.GET("/kyb/verifications", {
      headers: auth(token),
      params: { query: { organization_id: organizationId } },
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data ?? null;
  }

  async submit(
    token: AccessToken,
    input: KYBSubmissionInput,
    signal: AbortSignal,
  ): Promise<KYBVerification> {
    const form = new FormData();
    form.append("organization_id", input.organizationId);
    form.append("legal_name", input.legalName);
    form.append("company_rut", input.companyRut);
    form.append("representative_name", input.representativeName);
    form.append("representative_rut", input.representativeRut);
    form.append("sii_giro_code", input.siiGiroCode);
    form.append("erut", input.erut, input.erut.name);
    form.append(
      "power_certificate",
      input.powerCertificate,
      input.powerCertificate.name,
    );
    form.append(
      "sii_start_activities",
      input.siiStartActivities,
      input.siiStartActivities.name,
    );

    const result = await apiClient.POST("/kyb/verifications", {
      headers: auth(token),
      params: { header: { "Idempotency-Key": crypto.randomUUID() } },
      body: {
        organization_id: input.organizationId,
        sii_start_activities: input.siiStartActivities.name,
        legal_name: input.legalName,
        company_rut: input.companyRut,
        representative_name: input.representativeName,
        representative_rut: input.representativeRut,
        sii_giro_code: input.siiGiroCode,
        erut: input.erut.name,
        power_certificate: input.powerCertificate.name,
      },
      bodySerializer: () => form,
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async listPending(
    token: AccessToken,
    signal: AbortSignal,
  ): Promise<KYBVerification[]> {
    const result = await apiClient.GET("/admin/kyb/verifications", {
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data?.items ?? [];
  }

  async getForReview(
    token: AccessToken,
    id: string,
    signal: AbortSignal,
  ): Promise<KYBVerification> {
    const result = await apiClient.GET("/admin/kyb/verifications/{id}", {
      params: { path: { id } },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async startReview(
    token: AccessToken,
    id: string,
    signal: AbortSignal,
  ): Promise<KYBVerification> {
    const result = await apiClient.POST(
      "/admin/kyb/verifications/{id}/review",
      {
        params: { path: { id } },
        headers: auth(token),
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async decide(
    token: AccessToken,
    id: string,
    decision: "APPROVE" | "REJECT" | null,
    rejectionReason: string,
    signal: AbortSignal,
    documents?: KYBDocumentDecision[],
  ): Promise<KYBVerification> {
    if (!decision && !documents?.length) {
      throw new Error("Selecciona una decisión para cada documento.");
    }
    const aggregateDecision = documents?.length
      ? documents.every((item) => item.decision === "APPROVE")
        ? ("APPROVE" as const)
        : ("REJECT" as const)
      : (decision as "APPROVE" | "REJECT");
    const body = documents?.length
      ? { decision: aggregateDecision, documents }
      : aggregateDecision === "REJECT"
        ? { decision: aggregateDecision, rejection_reason: rejectionReason }
        : { decision: "APPROVE" as const };
    const result = await apiClient.POST(
      "/admin/kyb/verifications/{id}/decision",
      {
        params: { path: { id } },
        headers: auth(token),
        body,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async getDocumentReviewURL(
    token: AccessToken,
    verificationId: string,
    documentId: string,
    signal: AbortSignal,
  ): Promise<{ download_url: string; expires_at: string }> {
    const result = await apiClient.GET(
      "/admin/kyb/verifications/{id}/documents/{documentID}/url",
      {
        params: { path: { id: verificationId, documentID: documentId } },
        headers: auth(token),
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return result.data as { download_url: string; expires_at: string };
  }
}
