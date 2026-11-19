import { EarlyRentalPanel } from "@/features/bookings/ui/early-rental-panel";
import { ReservationsList } from "@/features/bookings/ui/reservations-list";

export default async function ReservasPage({
  searchParams,
}: {
  searchParams: Promise<{ demo?: string | string[] }>;
}) {
  const { demo } = await searchParams;
  const selectedId = typeof demo === "string" ? demo : undefined;
  return (
    <EarlyRentalPanel
      key={selectedId}
      selectedId={selectedId}
      fallback={<ReservationsList mode="renter" />}
    />
  );
}
