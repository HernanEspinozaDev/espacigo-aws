import "client-only";
import { CommercialAuthorizationService } from "./application/commercial-authorization.service";
import { CommercialAuthorizationOpenApiAdapter } from "./infrastructure/commercial-authorization-openapi-adapter";

export const commercialAuthorizationService =
  new CommercialAuthorizationService(
    new CommercialAuthorizationOpenApiAdapter(),
  );
