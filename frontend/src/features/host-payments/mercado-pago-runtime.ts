import "client-only";
import { MercadoPagoFacade } from "./application/mercado-pago.facade";
import { MercadoPagoOpenApiAdapter } from "./infrastructure/mercado-pago-openapi-adapter";

export const mercadoPagoFacade = new MercadoPagoFacade(
  new MercadoPagoOpenApiAdapter(),
);
