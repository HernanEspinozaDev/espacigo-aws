import type { ContractProposal, AgreementRead } from "../domain/agreements";
import type { AgreementsPort } from "./agreements.port";
type SessionRunner = <T>(
  operation: (signal: AbortSignal) => Promise<T>,
) => Promise<T>;
export class AgreementsService {
  constructor(
    private readonly port: AgreementsPort,
    private readonly authenticated: SessionRunner,
  ) {}
  capabilities(signal?: AbortSignal) {
    return this.port.capabilities(signal);
  }
  proposal(quoteId: string) {
    return this.authenticated((signal) => this.port.proposal(quoteId, signal));
  }
  list(role: "renter" | "landlord") {
    return this.authenticated((signal) => this.port.list(role, signal));
  }
  async resume(id: string) {
    return this.authenticated(async (signal) => {
      const read = await this.port.get(id, signal);
      const proposal = await this.port.readProposal(
        read.agreement.proposal_id,
        signal,
      );
      if (
        proposal.sha256 !== read.agreement.snapshot_sha256 ||
        proposal.version !== read.agreement.proposal_version ||
        proposal.quote_id !== read.agreement.quote_id
      )
        throw new TypeError("Agreement snapshot mismatch");
      return { ...read, proposal };
    });
  }
  send(proposal: ContractProposal, idempotencyKey: string) {
    return this.authenticated((signal) =>
      this.port.send(proposal, idempotencyKey, signal),
    );
  }
  withdraw(read: AgreementRead) {
    return this.authenticated((signal) => this.port.withdraw(read, signal));
  }
}
