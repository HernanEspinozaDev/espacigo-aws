import type {
  CreateMultiRentalQuoteRequest,
  RentalQuoteRequest,
  RentalQuoteResult,
} from "../domain/rental-quote";
import type { RentalQuotePort } from "./rental-quote.port";

export function createRentalQuoteService(port: RentalQuotePort) {
  return {
    createQuote(
      request: RentalQuoteRequest,
      signal?: AbortSignal,
    ): Promise<RentalQuoteResult> {
      return port.createQuote(request, signal);
    },
    createMultiQuote(
      request: CreateMultiRentalQuoteRequest,
      signal?: AbortSignal,
    ): Promise<RentalQuoteResult> {
      if (port.createMultiQuote) {
        return port.createMultiQuote(request, signal);
      }
      const first = request.intervals[0];
      if (!first) {
        return Promise.resolve({
          ok: false,
          error: { kind: "invalid", status: 400 },
        });
      }
      return port.createQuote(
        {
          space_id: request.space_id,
          start_at: first.start_at,
          end_at: first.end_at,
        },
        signal,
      );
    },
  };
}
