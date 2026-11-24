import {
  validContractSections,
  type ContractSection,
} from "../domain/contract-template";
import type { ContractTemplatePort } from "./contract-template.port";

export class ContractTemplateService {
  constructor(private readonly port: ContractTemplatePort) {}
  read(spaceId: string, signal?: AbortSignal) {
    return this.port.read(spaceId, signal);
  }
  async load(spaceId: string, signal?: AbortSignal) {
    const [read, versions, sources] = await Promise.all([
      this.port.read(spaceId, signal),
      this.port.versions(spaceId, signal),
      this.port.sources(signal),
    ]);
    return {
      ...read,
      versions,
      sources: sources.filter((source) => source.id !== spaceId),
    };
  }
  save(spaceId: string, sections: ContractSection[], etag: string) {
    if (!validContractSections(sections))
      throw new TypeError("Invalid clauses");
    return this.port.save(
      spaceId,
      sections.map(({ key, title, body }) => ({ key, title, body })),
      etag,
    );
  }
  publish(spaceId: string, etag: string) {
    return this.port.publish(spaceId, etag);
  }
  versions(spaceId: string, signal?: AbortSignal) {
    return this.port.versions(spaceId, signal);
  }
  copy(
    spaceId: string,
    etag: string,
    sourceSpaceId: string,
    sourceVersion: number,
  ) {
    return this.port.copy(spaceId, etag, sourceSpaceId, sourceVersion);
  }
}
