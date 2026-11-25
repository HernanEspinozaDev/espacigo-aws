import "client-only";
import { HostSettlementsFacade } from "./application/settlements.facade";
import { HostSettlementsOpenApiAdapter } from "./infrastructure/settlements-openapi-adapter";

export const hostSettlementsFacade = new HostSettlementsFacade(
  new HostSettlementsOpenApiAdapter(),
);
