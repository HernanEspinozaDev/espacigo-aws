import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { CommercialAuthorizationPort } from "../application/commercial-authorization.port";
import {
  isCommercialAuthorization,
  isCommercialAuthorizationEvent,
  isCommercialAuthorizationOTPChallenge,
  type ActivateCommercialAuthorizationInput,
  type CommercialAuthorization,
  type CommercialAuthorizationEvent,
  type CommercialAuthorizationOTPChallenge,
} from "../domain/commercial-authorization";

export class CommercialAuthorizationOpenApiAdapter implements CommercialAuthorizationPort {
  constructor(
    private readonly client: Pick<typeof apiClient, "GET" | "POST"> = apiClient,
  ) {}

  async current(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<CommercialAuthorization> {
    const result = await this.client.GET(
      "/spaces/{spaceId}/commercial-authorization/current",
      { params: { path: { spaceId } }, signal },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const item = result.data?.data.authorization;
    if (!isCommercialAuthorization(item)) throw new ApiProtocolError();
    return item;
  }

  async history(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<CommercialAuthorizationEvent[]> {
    const result = await this.client.GET(
      "/spaces/{spaceId}/commercial-authorization/history",
      { params: { path: { spaceId } }, signal },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const items = result.data?.data.items;
    if (
      !Array.isArray(items) ||
      items.some((item) => !isCommercialAuthorizationEvent(item))
    ) {
      throw new ApiProtocolError();
    }
    return items;
  }

  async requestOtpChallenge(
    spaceId: string,
    templateVersion: number,
  ): Promise<CommercialAuthorizationOTPChallenge> {
    const result = await this.client.POST(
      "/spaces/{spaceId}/commercial-authorization/otp-challenge",
      {
        params: { path: { spaceId } },
        body: { template_version: templateVersion },
        bodySerializer: serializeExactJson,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const item = result.data?.data;
    if (!isCommercialAuthorizationOTPChallenge(item))
      throw new ApiProtocolError();
    return item;
  }

  async activate(
    spaceId: string,
    input: ActivateCommercialAuthorizationInput,
  ): Promise<CommercialAuthorization> {
    const result = await this.client.POST(
      "/spaces/{spaceId}/commercial-authorization/activate",
      {
        params: { path: { spaceId } },
        body: {
          template_version: input.template_version,
          consent: input.consent,
          otp_challenge_id: input.otp_challenge_id,
          otp_code: input.otp_code,
        },
        bodySerializer: serializeExactJson,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const item = result.data?.data.authorization;
    if (!isCommercialAuthorization(item)) throw new ApiProtocolError();
    return item;
  }

  async revoke(
    spaceId: string,
    authorizationId: string,
    reason: string,
  ): Promise<CommercialAuthorization> {
    const result = await this.client.POST(
      "/spaces/{spaceId}/commercial-authorization/revoke",
      {
        params: { path: { spaceId } },
        body: { authorization_id: authorizationId, reason },
        bodySerializer: serializeExactJson,
      },
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const item = result.data?.data.authorization;
    if (!isCommercialAuthorization(item)) throw new ApiProtocolError();
    return item;
  }
}
