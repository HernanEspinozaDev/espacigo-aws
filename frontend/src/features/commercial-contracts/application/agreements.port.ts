import type {
  AgreementRead,
  CommercialAgreement,
  ContractProposal,
} from "../domain/agreements";
export interface AgreementsPort {
  capabilities(signal?: AbortSignal): Promise<boolean>;
  proposal(quoteId: string, signal?: AbortSignal): Promise<ContractProposal>;
  readProposal(
    proposalId: string,
    signal?: AbortSignal,
  ): Promise<ContractProposal>;
  list(
    role: "renter" | "landlord",
    signal?: AbortSignal,
  ): Promise<CommercialAgreement[]>;
  get(id: string, signal?: AbortSignal): Promise<AgreementRead>;
  send(
    proposal: ContractProposal,
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<AgreementRead>;
  withdraw(read: AgreementRead, signal?: AbortSignal): Promise<AgreementRead>;
}
