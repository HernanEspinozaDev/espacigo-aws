import "client-only";
import { createCheckoutService } from "../application/checkout.service";
import { CheckoutOpenApiAdapter } from "./checkout-openapi-adapter";

export const rentalCheckout = createCheckoutService(
  new CheckoutOpenApiAdapter(),
);
