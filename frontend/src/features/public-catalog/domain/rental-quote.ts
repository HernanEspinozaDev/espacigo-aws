import type { components } from "@/lib/api/generated/openapi";

export type MonthlyBillingMode = "full" | "monthly_installments";

export type CreateRentalQuoteRequest =
  components["schemas"]["CreateRentalQuoteRequest"] & {
    billing_mode?: MonthlyBillingMode;
  };
export type RentalQuoteRequest = CreateRentalQuoteRequest;
export type RentalQuote = components["schemas"]["RentalQuote"] & {
  billing_mode?: MonthlyBillingMode;
};

export type CreateMultiRentalQuoteRequest =
  components["schemas"]["CreateMultiRentalQuoteRequest"] & {
    billing_mode?: MonthlyBillingMode;
  };
export type RentalQuoteInterval = components["schemas"]["RentalQuoteInterval"];
export type RentalMultiQuote = components["schemas"]["RentalMultiQuote"];

export type RentalQuoteError =
  | { kind: "invalid"; status: 400 | 422 }
  | { kind: "unauthenticated"; status: 401 }
  | { kind: "forbidden"; status: 403 }
  | { kind: "not_found"; status: 404 }
  | { kind: "conflict"; status: 409 }
  | { kind: "unavailable"; status: 503 }
  | { kind: "other"; status: number };

export type RentalQuoteResult =
  | { ok: true; value: RentalQuote & { intervals?: RentalQuoteInterval[] } }
  | { ok: false; error: RentalQuoteError };
