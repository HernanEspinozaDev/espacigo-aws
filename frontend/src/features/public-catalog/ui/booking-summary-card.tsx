"use client";

import {
  PublicSpaceBookingCta,
  type PublicSpaceBookingCtaProps,
} from "./public-space-booking-cta";

export type BookingSummaryCardProps = PublicSpaceBookingCtaProps;

export function BookingSummaryCard(props: BookingSummaryCardProps) {
  return <PublicSpaceBookingCta {...props} />;
}
