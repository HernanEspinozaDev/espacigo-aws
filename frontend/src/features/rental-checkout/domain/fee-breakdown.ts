export interface BuyerFeeBreakdown {
  readonly baseClp: number;
  readonly commissionClp: number;
  readonly vatClp: number;
  readonly serviceFeeClp: number;
  readonly totalClp: number;
}

/**
 * Calculates the buyer-side EspaciGo service fee breakdown in CLP:
 * 3% commission plus 19% VAT (IVA) over that commission (3.57% total).
 */
export function calculateBuyerFeeBreakdown(baseClp: number): BuyerFeeBreakdown {
  const safeBase = Number.isFinite(baseClp)
    ? Math.max(0, Math.round(baseClp))
    : 0;
  const commissionClp = Math.floor((safeBase * 3 + 50) / 100);
  const vatClp = Math.floor((commissionClp * 19 + 50) / 100);
  const serviceFeeClp = commissionClp + vatClp;

  return {
    baseClp: safeBase,
    commissionClp,
    vatClp,
    serviceFeeClp,
    totalClp: safeBase + serviceFeeClp,
  };
}
