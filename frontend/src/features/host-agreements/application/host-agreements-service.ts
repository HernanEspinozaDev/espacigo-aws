import type {
  AgreementRead,
  CommercialAgreement,
  ContractProposal,
} from "@/features/commercial-contracts/domain/agreements";
export type HostAgreementReview = {
  read: AgreementRead;
  proposal: ContractProposal;
};
export interface HostAgreementsPort {
  capabilities(signal?: AbortSignal): Promise<boolean>;
  list(signal?: AbortSignal): Promise<CommercialAgreement[]>;
  get(id: string, signal?: AbortSignal): Promise<AgreementRead>;
  proposal(id: string, signal?: AbortSignal): Promise<ContractProposal>;
  accept(
    read: AgreementRead,
    signal?: AbortSignal,
    otp?: { challengeId: string; code: string },
  ): Promise<AgreementRead>;
  reject(read: AgreementRead, signal?: AbortSignal): Promise<AgreementRead>;
}
export class HostAgreementsService {
  constructor(private readonly port: HostAgreementsPort) {}
  available(signal?: AbortSignal) {
    return this.port.capabilities(signal);
  }
  list(signal?: AbortSignal) {
    return this.port.list(signal);
  }
  async review(id: string, signal?: AbortSignal): Promise<HostAgreementReview> {
    const read = await this.port.get(id, signal);
    const proposal = await this.port.proposal(
      read.agreement.proposal_id,
      signal,
    );
    if (
      proposal.id !== read.agreement.proposal_id ||
      proposal.sha256 !== read.agreement.snapshot_sha256 ||
      proposal.version !== read.agreement.proposal_version ||
      proposal.quote_id !== read.agreement.quote_id ||
      proposal.space_id !== read.agreement.space_id
    )
      throw new Error(
        "La propuesta no coincide con la solicitud. Recarga antes de decidir.",
      );
    return { read, proposal };
  }
  accept(
    review: HostAgreementReview,
    consent: boolean,
    signal?: AbortSignal,
    otp?: { challengeId: string; code: string },
  ) {
    this.pending(review);
    if (!consent)
      throw new Error(
        "Confirma que has revisado y aceptas esta versión del contrato.",
      );
    return this.port.accept(review.read, signal, otp);
  }
  reject(review: HostAgreementReview, signal?: AbortSignal) {
    this.pending(review);
    return this.port.reject(review.read, signal);
  }
  private pending(review: HostAgreementReview) {
    const a = review.read.agreement;
    if (
      a.state !== "pending_landlord" ||
      review.proposal.id !== a.proposal_id ||
      review.proposal.sha256 !== a.snapshot_sha256 ||
      review.proposal.version !== a.proposal_version
    )
      throw new Error(
        "La solicitud cambió o ya tiene una decisión. Recarga para revisar su estado.",
      );
  }
}
