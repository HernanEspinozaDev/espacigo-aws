import type {
  AuthorizationActivation,
  CommercialAuthorization,
  CommercialAuthorizationEvent,
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
  activate(
    spaceId: string,
    input: AuthorizationActivation,
  ): Promise<CommercialAuthorization>;
  revoke(
    spaceId: string,
    authorizationId: string,
    reason: string,
  ): Promise<CommercialAuthorization>;
}
