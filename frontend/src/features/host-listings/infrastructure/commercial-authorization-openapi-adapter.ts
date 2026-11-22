import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { CommercialAuthorizationPort } from "../application/commercial-authorization.port";
import {
  isCommercialAuthorization,
  type AuthorizationActivation,
  type CommercialAuthorization,
  type CommercialAuthorizationEvent,
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
      items.some(
        (item) =>
          typeof item.grant_id !== "string" || typeof item.action !== "string",
      )
    ) {
      throw new ApiProtocolError();
    }
    return items;
  }

  async activate(
    spaceId: string,
    input: AuthorizationActivation,
  ): Promise<CommercialAuthorization> {
    const result = await this.client.POST(
      "/spaces/{spaceId}/commercial-authorization/activate",
      {
        params: { path: { spaceId } },
        body: input,
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
