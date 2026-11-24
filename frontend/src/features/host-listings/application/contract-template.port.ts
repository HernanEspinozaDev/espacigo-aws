import type {
  ContractSection,
  ContractTemplateRead,
  ContractTemplateSource,
  ContractTemplateVersion,
} from "../domain/contract-template";
export interface ContractTemplatePort {
  read(spaceId: string, signal?: AbortSignal): Promise<ContractTemplateRead>;
  save(
    spaceId: string,
    sections: ContractSection[],
    etag: string,
  ): Promise<ContractTemplateRead>;
  publish(
    spaceId: string,
    etag: string,
  ): Promise<{ version: ContractTemplateVersion; etag: string }>;
  versions(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<ContractTemplateVersion[]>;
  sources(signal?: AbortSignal): Promise<ContractTemplateSource[]>;
  copy(
    spaceId: string,
    etag: string,
    sourceSpaceId: string,
    sourceVersion: number,
  ): Promise<ContractTemplateRead>;
}
