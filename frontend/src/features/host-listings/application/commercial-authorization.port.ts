import type {
  ActivateCommercialAuthorizationInput,
  CommercialAuthorization,
  CommercialAuthorizationEvent,
  CommercialAuthorizationOTPChallenge,
} from "../domain/commercial-authorization";

export interface CommercialAuthorizationPort {
  current(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<CommercialAuthorization>;
  history(
    spaceId: string,
    signal?: AbortSignal,
  ): Promise<CommercialAuthorizationEvent[]>;
  requestOtpChallenge(
    spaceId: string,
    templateVersion: number,
  ): Promise<CommercialAuthorizationOTPChallenge>;
  activate(
    spaceId: string,
    input: ActivateCommercialAuthorizationInput,
  ): Promise<CommercialAuthorization>;
  revoke(
    spaceId: string,
    authorizationId: string,
    reason: string,
  ): Promise<CommercialAuthorization>;
}
