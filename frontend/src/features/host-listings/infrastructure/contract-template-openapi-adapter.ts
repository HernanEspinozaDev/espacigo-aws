import "client-only";
import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { ContractTemplatePort } from "../application/contract-template.port";
import {
  contractTemplateEtag,
  validTemplateDraft,
  validTemplateVersion,
  type ContractSection,
} from "../domain/contract-template";

export class OpenApiContractTemplateAdapter implements ContractTemplatePort {
  constructor(
    private readonly client: Pick<
      typeof apiClient,
      "GET" | "PUT" | "POST"
    > = apiClient,
  ) {}
  async read(spaceId: string, signal?: AbortSignal) {
    const r = await this.client.GET(
      "/spaces/{spaceId}/contract-template/draft",
      { params: { path: { spaceId } }, signal },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return this.draft(r.data?.data, r.response, spaceId);
  }
  async save(spaceId: string, sections: ContractSection[], etag: string) {
    const r = await this.client.PUT(
      "/spaces/{spaceId}/contract-template/draft",
      {
        params: { path: { spaceId }, header: { "If-Match": etag } },
        body: { sections },
        bodySerializer: serializeExactJson,
      },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return this.draft(r.data?.data, r.response, spaceId);
  }
  async publish(spaceId: string, etag: string) {
    const r = await this.client.POST(
      "/spaces/{spaceId}/contract-template/publish",
      { params: { path: { spaceId }, header: { "If-Match": etag } } },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    const version = r.data?.data;
    const next = r.response.headers.get("ETag");
    if (
      !validTemplateVersion(version, spaceId) ||
      !next ||
      !/^"contract-template-(0|[1-9][0-9]*)"$/.test(next)
    )
      throw new ApiProtocolError();
    return { version, etag: next };
  }
  async versions(spaceId: string, signal?: AbortSignal) {
    const r = await this.client.GET(
      "/spaces/{spaceId}/contract-template/versions",
      { params: { path: { spaceId } }, signal },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    const items = r.data?.data.items;
    if (
      !Array.isArray(items) ||
      !items.every((v) => validTemplateVersion(v, spaceId))
    )
      throw new ApiProtocolError();
    return items;
  }
  async sources(signal?: AbortSignal) {
    const r = await this.client.GET("/spaces", { signal });
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    if (!r.data?.items) throw new ApiProtocolError();
    return r.data.items.map((v) => ({ id: v.id, title: v.title }));
  }
  async copy(
    spaceId: string,
    etag: string,
    sourceSpaceId: string,
    sourceVersion: number,
  ) {
    const r = await this.client.POST(
      "/spaces/{spaceId}/contract-template/copy",
      {
        params: { path: { spaceId }, header: { "If-Match": etag } },
        body: { source_space_id: sourceSpaceId, source_version: sourceVersion },
        bodySerializer: serializeExactJson,
      },
    );
    if (!r.response.ok) throw normalizeApiError(r.response, r.error);
    return this.draft(r.data?.data, r.response, spaceId);
  }
  private draft(value: unknown, response: Response, spaceId: string) {
    if (
      !validTemplateDraft(value, spaceId) ||
      response.headers.get("ETag") !== contractTemplateEtag(value.revision)
    )
      throw new ApiProtocolError();
    return { draft: value, etag: contractTemplateEtag(value.revision) };
  }
}
