import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import {
  agreementConsentVersion,
  agreementEtag,
  parseAgreement,
  parseProposal,
  type AgreementRead,
} from "@/features/commercial-contracts/domain/agreements";
import type { HostAgreementsPort } from "../application/host-agreements-service";
export class HostAgreementsOpenApiAdapter implements HostAgreementsPort {
  constructor(
    private readonly client: Pick<typeof apiClient, "GET" | "PUT"> = apiClient,
  ) {}
  async capabilities(signal?: AbortSignal) {
    const result = await this.client.GET("/rental-contract-capabilities", {
      signal,
    });
    if (result.response.status === 404) return false;
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    if (typeof result.data?.data.manual_contracts_enabled !== "boolean")
      throw new ApiProtocolError();
    return result.data.data.manual_contracts_enabled;
  }
  async list(signal?: AbortSignal) {
    const result = await this.client.GET("/commercial-agreements", {
      params: { query: { role: "landlord" } },
      signal,
    });
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    if (!Array.isArray(result.data?.items)) throw new ApiProtocolError();
    return result.data.items.map((item) => parseAgreement(item));
  }
  async get(id: string, signal?: AbortSignal) {
    const result = await this.client.GET(
      "/commercial-agreements/{agreementID}",
      { params: { path: { agreementID: id } }, signal },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return this.read(result.data, result.response, id);
  }
  async proposal(id: string, signal?: AbortSignal) {
    const result = await this.client.GET(
      "/rental-contract-proposals/{proposalID}",
      { params: { path: { proposalID: id } }, signal },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return parseProposal(result.data?.data, {
      proposalId: id,
      allowRevealedParties: true,
    });
  }
  async accept(
    read: AgreementRead,
    signal?: AbortSignal,
    otp?: { challengeId: string; code: string },
  ) {
    const result = await this.client.PUT(
      "/commercial-agreements/{agreementID}/landlord-signature",
      {
        params: {
          path: { agreementID: read.agreement.id },
          header: { "If-Match": read.etag },
        },
        body: {
          snapshot_sha256: read.agreement.snapshot_sha256,
          consent: true,
          consent_version: agreementConsentVersion,
          ...(otp
            ? { otp_challenge_id: otp.challengeId, otp_code: otp.code }
            : {}),
        },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return this.read(result.data, result.response, read.agreement.id);
  }
  async reject(read: AgreementRead, signal?: AbortSignal) {
    const result = await this.client.PUT(
      "/commercial-agreements/{agreementID}/decision",
      {
        params: {
          path: { agreementID: read.agreement.id },
          header: { "If-Match": read.etag },
        },
        body: {
          snapshot_sha256: read.agreement.snapshot_sha256,
          decision: "reject",
        },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    return this.read(result.data, result.response, read.agreement.id);
  }
  private read(data: unknown, response: Response, id: string): AgreementRead {
    const agreement = parseAgreement(data, id);
    const etag = response.headers.get("ETag");
    if (etag !== agreementEtag(agreement)) throw new ApiProtocolError();
    return { agreement, etag };
  }
}
