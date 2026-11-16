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
