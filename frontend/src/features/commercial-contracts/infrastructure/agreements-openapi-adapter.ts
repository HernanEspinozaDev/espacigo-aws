import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { AgreementsPort } from "../application/agreements.port";
import {
  agreementConsentVersion,
  agreementEtag,
  parseAgreement,
  parseProposal,
  type AgreementRead,
  type ContractProposal,
} from "../domain/agreements";
export class OpenApiAgreementsAdapter implements AgreementsPort {
  constructor(
    private readonly client: Pick<
      typeof apiClient,
      "GET" | "POST" | "PUT"
    > = apiClient,
  ) {}
  async capabilities(signal?: AbortSignal) {
    const r = await this.client.GET("/rental-contract-capabilities", {
      signal,
    });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    if (typeof r.data?.data.manual_contracts_enabled !== "boolean")
      throw new ApiProtocolError();
    return r.data.data.manual_contracts_enabled;
  }
  async proposal(quoteId: string, signal?: AbortSignal) {
    const r = await this.client.POST(
      "/rental-quotes/{quoteID}/contract-proposal",
      { params: { path: { quoteID: quoteId } }, signal },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return parseProposal(r.data?.data, { quoteId });
  }
  async readProposal(proposalId: string, signal?: AbortSignal) {
    const r = await this.client.GET("/rental-contract-proposals/{proposalID}", {
      params: { path: { proposalID: proposalId } },
      signal,
    });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return parseProposal(r.data?.data, {
      proposalId,
      allowRevealedParties: true,
    });
  }
  async list(role: "renter" | "landlord", signal?: AbortSignal) {
    const r = await this.client.GET("/commercial-agreements", {
      params: { query: { role } },
      signal,
    });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    if (!Array.isArray(r.data?.items)) throw new ApiProtocolError();
    return r.data.items.map((v) => parseAgreement(v));
  }
  async get(id: string, signal?: AbortSignal) {
    const r = await this.client.GET("/commercial-agreements/{agreementID}", {
      params: { path: { agreementID: id } },
      signal,
    });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return this.read(r.data, r.response, id);
  }
  async send(
    proposal: ContractProposal,
    idempotencyKey: string,
    signal?: AbortSignal,
    otp?: { challengeId: string; code: string },
  ) {
    const r = await this.client.POST("/commercial-agreements", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: {
        proposal_id: proposal.id,
        snapshot_sha256: proposal.sha256,
        consent: true,
        consent_version: agreementConsentVersion,
        ...(otp ? { otp_challenge_id: otp.challengeId, otp_code: otp.code } : {}),
      },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    const read = this.read(r.data, r.response);
    if (
      read.agreement.proposal_id !== proposal.id ||
      read.agreement.proposal_version !== proposal.version ||
      read.agreement.snapshot_sha256 !== proposal.sha256 ||
      read.agreement.quote_id !== proposal.quote_id
    )
      throw new ApiProtocolError();
    return read;
  }
  async withdraw(read: AgreementRead, signal?: AbortSignal) {
    const r = await this.client.PUT(
      "/commercial-agreements/{agreementID}/decision",
      {
        params: {
          path: { agreementID: read.agreement.id },
          header: { "If-Match": read.etag },
        },
        body: {
          snapshot_sha256: read.agreement.snapshot_sha256,
          decision: "withdraw",
        },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return this.read(r.data, r.response, read.agreement.id);
  }
  private read(
    value: unknown,
    response: Response,
    expectedId?: string,
  ): AgreementRead {
    const agreement = parseAgreement(value, expectedId);
    const etag = response.headers.get("ETag");
    if (etag !== agreementEtag(agreement)) throw new ApiProtocolError();
    return { agreement, etag };
  }
}
