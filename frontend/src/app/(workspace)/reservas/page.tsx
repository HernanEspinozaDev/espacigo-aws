import Link from "next/link";
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
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card px-4 py-3">
        <p className="text-sm text-muted-foreground">
          Revisa tus solicitudes de contrato pendientes de firma o aprobación.
        </p>
        <Link
          href="/reservas/contratos"
          className="inline-flex items-center rounded-lg border border-border bg-background px-3 py-1.5 text-sm font-medium hover:bg-muted"
        >
          Ver mis contratos
        </Link>
      </div>
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
    </div>
  );
}
