import { EarlyRentalPanel } from "@/features/bookings/ui/early-rental-panel";
import { ReservationsList } from "@/features/bookings/ui/reservations-list";

export default async function ReservasPage({
  searchParams,
}: {
  searchParams: Promise<{
    demo?: string | string[];
    space_id?: string | string[];
    tab?: string | string[];
  }>;
}) {
  const { demo, space_id, tab } = await searchParams;
  const selectedId = typeof demo === "string" ? demo : undefined;
  const initialSpaceId = typeof space_id === "string" ? space_id : undefined;
  const initialTab =
    tab === "general" || tab === "finanzas" || tab === "lista"
      ? tab
      : undefined;
  return (
    <EarlyRentalPanel
      key={selectedId}
      selectedId={selectedId}
      fallback={
        <ReservationsList
          mode="renter"
          initialSpaceId={initialSpaceId}
          initialSection={initialTab}
        />
      }
    />
  );
}
