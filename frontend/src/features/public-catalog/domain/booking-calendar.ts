import type { PublicRentalAvailability } from "./rental-availability";
import type { MarketplaceBookingSelection } from "./marketplace-booking-selection";

export function authoritativeIntervals(
  result: { ok: true; value: PublicRentalAvailability } | { ok: false },
) {
  return result.ok ? result.value.items : [];
}

export function availabilitySelection(
  items: PublicRentalAvailability["items"],
  context: Omit<MarketplaceBookingSelection, "startAt" | "endAt" | "timeZone">,
  timeZone: string | undefined,
): MarketplaceBookingSelection | null {
  const item = items[0];
  if (!item || !timeZone) return null;
  return { ...context, startAt: item.start_at, endAt: item.end_at, timeZone };
}

export function createGenerationFence() {
  let generation = 0;
  return {
    next: () => ++generation,
    isCurrent: (value: number) => generation === value,
  };
}

export interface SuggestedStayOption {
  readonly label: string;
  readonly duration: number;
}

export const SUGGESTED_STAYS: Record<
  "hora" | "dia" | "mes",
  readonly SuggestedStayOption[]
> = {
  hora: [
    { label: "1 hora", duration: 1 },
    { label: "2 horas", duration: 2 },
    { label: "4 horas", duration: 4 },
  ],
  dia: [
    { label: "1 día", duration: 1 },
    { label: "2 días", duration: 2 },
    { label: "3 días", duration: 3 },
  ],
  mes: [{ label: "1 mes", duration: 1 }],
} as const;

export function suggestedStaysForUnit(
  rateUnit: "hora" | "dia" | "mes" | string,
): readonly SuggestedStayOption[] {
  if (rateUnit === "hora") return SUGGESTED_STAYS.hora;
  if (rateUnit === "dia") return SUGGESTED_STAYS.dia;
  if (rateUnit === "mes") return SUGGESTED_STAYS.mes;
  return [];
}
