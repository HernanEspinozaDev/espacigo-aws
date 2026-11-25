import "client-only";

import type { AccessToken } from "@/features/auth/application/auth.port";
import { apiClient } from "@/lib/api/client";
import { normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { KYBSubmissionInput, KYBVerification } from "../domain/kyb";

function auth(token: AccessToken) {
  return { Authorization: ["Bearer", token].join(" ") };
}

export class KYBOpenApiAdapter {
  async getOwn(token: AccessToken, signal: AbortSignal): Promise<KYBVerification | null> {
    const result = await apiClient.GET("/kyb/verifications", {
      headers: auth(token),
      signal,
    });
    if (result.response.status === 404) return null;
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return (result.data as KYBVerification | undefined) ?? null;
  }

  async submit(token: AccessToken, input: KYBSubmissionInput, signal: AbortSignal): Promise<KYBVerification> {
    const form = new FormData();
    form.append("legal_name", input.legalName);
    form.append("company_rut", input.companyRut);
    form.append("representative_name", input.representativeName);
    form.append("representative_rut", input.representativeRut);
    form.append("sii_giro_code", input.siiGiroCode);
    form.append("erut", input.erut, input.erut.name);
    form.append("power_certificate", input.powerCertificate, input.powerCertificate.name);
    form.append("property_or_sublease_authorization", input.propertyOrSubleaseAuthorization, input.propertyOrSubleaseAuthorization.name);

    const result = await apiClient.POST("/kyb/verifications", {
      headers: auth(token),
      params: { header: { "Idempotency-Key": crypto.randomUUID() } },
      body: {
        legal_name: input.legalName,
        company_rut: input.companyRut,
        representative_name: input.representativeName,
        representative_rut: input.representativeRut,
        sii_giro_code: input.siiGiroCode,
        erut: input.erut.name,
        power_certificate: input.powerCertificate.name,
        property_or_sublease_authorization: input.propertyOrSubleaseAuthorization.name,
      },
      bodySerializer: () => form,
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async listPending(token: AccessToken, signal: AbortSignal): Promise<KYBVerification[]> {
    const result = await apiClient.GET("/admin/kyb/verifications", {
      headers: auth(token),
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data?.items ?? [];
  }

  async getForReview(token: AccessToken, id: string, signal: AbortSignal): Promise<KYBVerification> {
    const result = await apiClient.GET("/admin/kyb/verifications/{id}", {
      params: { path: { id } },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async startReview(token: AccessToken, id: string, signal: AbortSignal): Promise<KYBVerification> {
    const result = await apiClient.POST("/admin/kyb/verifications/{id}/review", {
      params: { path: { id } },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async decide(token: AccessToken, id: string, decision: "APPROVE" | "REJECT", rejectionReason: string, signal: AbortSignal): Promise<KYBVerification> {
    const result = await apiClient.POST("/admin/kyb/verifications/{id}/decision", {
      params: { path: { id } },
      headers: auth(token),
      body: decision === "REJECT"
        ? { decision, rejection_reason: rejectionReason }
        : { decision },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data as KYBVerification;
  }

  async getDocumentReviewURL(token: AccessToken, verificationId: string, documentId: string, signal: AbortSignal): Promise<{ download_url: string; expires_at: string }> {
    const result = await apiClient.GET("/admin/kyb/verifications/{id}/documents/{documentID}/url", {
      params: { path: { id: verificationId, documentID: documentId } },
      headers: auth(token),
      signal,
    });
    if (!result.response.ok) throw normalizeApiError(result.response, result.error);
    return result.data as { download_url: string; expires_at: string };
  }
}
