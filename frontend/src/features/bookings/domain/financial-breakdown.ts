import { calculateBuyerFeeBreakdown } from "@/features/rental-checkout/domain/fee-breakdown";
import type { BookingReservation } from "./types";

export interface ReservationFinancialBreakdown {
  readonly baseClp: number;
  readonly hostCommissionClp: number;
  readonly hostVatClp: number;
  readonly hostFeeClp: number;
  readonly netHostClp: number;
  readonly confirmedNetHostClp?: number;
  readonly beforeProcessingClp: number;
  readonly processingFeeClp?: number;
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
  const beforeProcessingClp =
    reservation.net_host_amount_clp ?? baseClp - hostFeeClp;
  const settlement = reservation.provider_settlement;
  const confirmedNetHostClp = settlement?.net_clp;
  const netHostClp = confirmedNetHostClp ?? beforeProcessingClp;

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
    confirmedNetHostClp,
    beforeProcessingClp,
    processingFeeClp: settlement?.processing_fee_clp,
    buyerCommissionClp: resolvedBuyerCommission,
    buyerVatClp: resolvedBuyerVat,
    buyerFeeClp,
    totalChargedClp,
    marketplaceFeeClp,
  };
}

export function getRecordedProcessingFee(
  reservation: BookingReservation,
  financial: ReservationFinancialBreakdown,
): { amountClp: number; chargedClp: number } | undefined {
  const amountClp = financial.processingFeeClp;
  const chargedClp = reservation.total_charged_clp;
  if (
    !reservation.provider_payment_id ||
    amountClp === undefined ||
    chargedClp === undefined ||
    !Number.isSafeInteger(amountClp) ||
    !Number.isSafeInteger(chargedClp) ||
    amountClp < 0 ||
    chargedClp <= 0 ||
    amountClp > chargedClp
  ) {
    return undefined;
  }
  return { amountClp, chargedClp };
}

export type ReservationFinancialAdjustment =
  | "none"
  | "refund_pending"
  | "refund_confirmed"
  | "refund_failed"
  | "chargeback";

export interface ReservationFinancialMovement {
  readonly adjustment: ReservationFinancialAdjustment;
  readonly originalChargeClp?: number;
  readonly confirmedRefundClp?: number;
  readonly reconciledNetClp?: number;
}

export function getReservationFinancialMovement(
  reservation: BookingReservation,
  financial: ReservationFinancialBreakdown,
): ReservationFinancialMovement {
  if (!reservation.is_commercial)
    return { adjustment: "none", reconciledNetClp: financial.netHostClp };

  const total = reservation.total_charged_clp;
  const originalChargeClp =
    total !== undefined && Number.isSafeInteger(total) && total > 0
      ? total
      : undefined;
  const settlement = reservation.provider_settlement;
  const refunded = settlement?.refunded_clp;
  const confirmedRefundClp =
    reservation.provider_payment_id &&
    refunded !== undefined &&
    originalChargeClp !== undefined &&
    Number.isSafeInteger(refunded) &&
    refunded > 0 &&
    refunded <= originalChargeClp
      ? refunded
      : undefined;

  const refundStatus = reservation.commercial_refund_status;
  let adjustment: ReservationFinancialAdjustment = "none";
  if (settlement?.payment_status === "charged_back") {
    adjustment = "chargeback";
  } else if (
    confirmedRefundClp !== undefined ||
    settlement?.payment_status === "refunded" ||
    reservation.state === "reembolsada" ||
    refundStatus === "aprobado"
  ) {
    adjustment = "refund_confirmed";
  } else if (refundStatus === "pendiente") {
    adjustment = "refund_pending";
  } else if (refundStatus === "fallido") {
    adjustment = "refund_failed";
  }

  // The merged provider snapshot can still hold the original net after
  // recording a reversal. It cannot establish the final balance of that sale.
  const net = financial.confirmedNetHostClp;
  const reconciledNetClp =
    (adjustment === "none" || adjustment === "refund_failed") &&
    settlement?.reconciliation === "reconciled" &&
    net !== undefined &&
    Number.isSafeInteger(net) &&
    net >= 0
      ? net
      : undefined;

  return {
    adjustment,
    originalChargeClp,
    confirmedRefundClp,
    reconciledNetClp,
  };
}
