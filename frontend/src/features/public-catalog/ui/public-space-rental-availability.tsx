"use client";

import { useId } from "react";
import { PublicBookingCalendar } from "./booking-calendar";
import { publicAvailabilityNotice } from "../domain/availability-notice";
import type { RentalReturnSelection } from "../domain/availability-return";
import type { MarketplaceBookingSelection } from "../domain/marketplace-booking-selection";

interface PublicSpaceRentalAvailabilityProps {
  spaceId: string;
  rateUnit: "hora" | "dia" | "mes";
  basePriceCLP: number | string | null;
  today: string;
  initialSelection?: RentalReturnSelection;
  onSelectionChange?: (selection: MarketplaceBookingSelection | null) => void;
  disponibleDesde?: string | null;
  estadiaMinimaMeses?: number | null;
}

export function PublicSpaceRentalAvailability({
  spaceId,
  rateUnit,
  basePriceCLP,
  today,
  initialSelection,
  onSelectionChange,
  disponibleDesde,
  estadiaMinimaMeses,
}: PublicSpaceRentalAvailabilityProps) {
  const headingId = useId();

  return (
    <section
      aria-labelledby={headingId}
      className="min-w-0 space-y-6 rounded-2xl border border-border p-6 sm:p-8"
    >
      <header className="space-y-1">
        <h2
          id={headingId}
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Disponibilidad para arriendo
        </h2>
        <p className="break-words text-sm leading-relaxed text-muted-foreground">
          {publicAvailabilityNotice}
        </p>
      </header>
      <div className="min-w-0 pt-2">
        <PublicBookingCalendar
          spaceId={spaceId}
          rateUnit={rateUnit}
          basePriceCLP={basePriceCLP}
          today={today}
          initialSelection={initialSelection}
          onSelectionChange={onSelectionChange}
          disponibleDesde={disponibleDesde}
          estadiaMinimaMeses={estadiaMinimaMeses}
        />
      </div>
    </section>
  );
}
