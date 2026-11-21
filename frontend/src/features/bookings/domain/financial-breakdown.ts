import { calculateBuyerFeeBreakdown } from "@/features/rental-checkout/domain/fee-breakdown";
import type { BookingReservation } from "./types";

export interface ReservationFinancialBreakdown {
  readonly baseClp: number;
  readonly hostCommissionClp: number;
  readonly hostVatClp: number;
  readonly hostFeeClp: number;
  readonly netHostClp: number;
  readonly buyerCommissionClp: number;
  readonly buyerVatClp: number;
  readonly buyerFeeClp: number;
  readonly totalChargedClp: number;
  readonly marketplaceFeeClp: number;
}

/**
 * Resolves the double-sided commission breakdown for a reservation,
 * preserving persisted backend values and deriving missing parts safely.
 */
export function getReservationFinancialBreakdown(
  reservation: BookingReservation,
): ReservationFinancialBreakdown {
  const baseClp = reservation.subtotal_clp;
  const hostCommissionClp = reservation.commission_clp ?? 0;
  const hostVatClp = reservation.vat_clp ?? 0;
  const hostFeeClp = hostCommissionClp + hostVatClp;
  const netHostClp = reservation.net_host_amount_clp ?? baseClp - hostFeeClp;

  const explicitBuyerFee = reservation.buyer_fee_clp;
  const explicitTotalCharged = reservation.total_charged_clp;
  let buyerCommissionClp = reservation.buyer_commission_clp;
  let buyerVatClp = reservation.buyer_vat_clp;

  const buyerFeeClp =
    explicitBuyerFee ??
    (buyerCommissionClp !== undefined || buyerVatClp !== undefined
      ? (buyerCommissionClp ?? 0) + (buyerVatClp ?? 0)
      : explicitTotalCharged !== undefined
        ? Math.max(0, explicitTotalCharged - baseClp)
        : 0);

  if (
    buyerCommissionClp === undefined &&
    buyerVatClp === undefined &&
    buyerFeeClp > 0
  ) {
    const derived = calculateBuyerFeeBreakdown(baseClp);
    if (derived.serviceFeeClp === buyerFeeClp) {
      buyerCommissionClp = derived.commissionClp;
      buyerVatClp = derived.vatClp;
    }
  }

  const resolvedBuyerCommission = buyerCommissionClp ?? 0;
  const resolvedBuyerVat = buyerVatClp ?? 0;
  const totalChargedClp = explicitTotalCharged ?? baseClp + buyerFeeClp;
  const marketplaceFeeClp =
    reservation.marketplace_fee_clp ?? hostFeeClp + buyerFeeClp;

  return {
    baseClp,
    hostCommissionClp,
    hostVatClp,
    hostFeeClp,
    netHostClp,
    buyerCommissionClp: resolvedBuyerCommission,
    buyerVatClp: resolvedBuyerVat,
    buyerFeeClp,
    totalChargedClp,
    marketplaceFeeClp,
  };
}
