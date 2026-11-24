import "client-only";
import { HostAgreementsService } from "./application/host-agreements-service";
import { HostAgreementsOpenApiAdapter } from "./infrastructure/host-agreements-openapi-adapter";
export const hostAgreements = new HostAgreementsService(
  new HostAgreementsOpenApiAdapter(),
);
