import type {
  CreateMultiRentalQuoteRequest,
  RentalQuoteRequest,
  RentalQuoteResult,
} from "../domain/rental-quote";

export interface RentalQuotePort {
  createQuote(
    request: RentalQuoteRequest,
    signal?: AbortSignal,
  ): Promise<RentalQuoteResult>;
  createMultiQuote?(
    request: CreateMultiRentalQuoteRequest,
    signal?: AbortSignal,
  ): Promise<RentalQuoteResult>;
}
