import type { ActivateCommercialAuthorizationInput } from "../domain/commercial-authorization";
import type { CommercialAuthorizationPort } from "./commercial-authorization.port";

export class CommercialAuthorizationService {
  constructor(private readonly port: CommercialAuthorizationPort) {}
  current(spaceId: string, signal?: AbortSignal) {
    return this.port.current(spaceId, signal);
  }
  history(spaceId: string, signal?: AbortSignal) {
    return this.port.history(spaceId, signal);
  }
  requestOtpChallenge(spaceId: string, templateVersion: number) {
    return this.port.requestOtpChallenge(spaceId, templateVersion);
  }
  activate(spaceId: string, input: ActivateCommercialAuthorizationInput) {
    return this.port.activate(spaceId, input);
  }
  revoke(spaceId: string, authorizationId: string, reason: string) {
    return this.port.revoke(spaceId, authorizationId, reason);
  }
}
