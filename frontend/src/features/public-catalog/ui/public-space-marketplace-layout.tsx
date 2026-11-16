"use client";

import {
  cloneElement,
  isValidElement,
  useEffect,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import type { MarketplaceBookingSelection } from "../domain/marketplace-booking-selection";
import { PublicSpaceBookingCta } from "./public-space-booking-cta";

export function PublicSpaceMarketplaceLayout({
  summary,
  availability,
  gallery,
  summaryAside,
  basePriceCLP,
  rateUnit = "hora",
}: {
  summary: ReactNode;
  availability: ReactNode;
  gallery: ReactNode;
  summaryAside?: ReactNode;
  basePriceCLP?: number | string | null;
  rateUnit?: "hora" | "dia" | "mes";
}) {
  const [selection, setSelection] =
    useState<MarketplaceBookingSelection | null>(null);
  const calendarRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleSelection = (e: Event) => {
      const customEvent = e as CustomEvent<MarketplaceBookingSelection | null>;
      setSelection(customEvent.detail ?? null);
    };
    window.addEventListener("espacigo:marketplace-selection", handleSelection);
    return () => {
      window.removeEventListener(
        "espacigo:marketplace-selection",
        handleSelection,
      );
    };
  }, []);

  const handleChooseDates = () => {
    calendarRef.current?.scrollIntoView({ behavior: "smooth" });
    const button = calendarRef.current?.querySelector<HTMLButtonElement>(
      "button[aria-pressed]:not([disabled])",
    );
    button?.focus();
  };

  const availabilityElement = isValidElement(availability)
    ? cloneElement(availability as ReactElement<any>, {
        onSelectionChange: (sel: MarketplaceBookingSelection | null) => {
          setSelection(sel);
        },
      })
    : availability;

  return (
    <>
      <div className="lg:grid lg:grid-cols-12 lg:items-start lg:gap-6 xl:gap-8">
        <div className="min-w-0 lg:col-span-4 space-y-6">
          {summary}
          {summaryAside ?? (
            <PublicSpaceBookingCta
              selection={selection}
              basePriceCLP={basePriceCLP ?? null}
              rateUnit={rateUnit}
              onChooseDates={handleChooseDates}
            />
          )}
        </div>
        <div
          ref={calendarRef}
          className="mt-8 min-w-0 lg:col-span-4 lg:mt-0"
        >
          {availabilityElement}
        </div>
        <div className="mt-8 min-w-0 lg:col-span-4 lg:mt-0">{gallery}</div>
      </div>
      <div className="fixed bottom-0 left-0 right-0 z-40 border-t border-border bg-background/95 p-3 backdrop-blur shadow-lg lg:hidden">
        <PublicSpaceBookingCta
          mobile
          selection={selection}
          basePriceCLP={basePriceCLP ?? null}
          rateUnit={rateUnit}
          onChooseDates={handleChooseDates}
        />
      </div>
    </>
  );
}
