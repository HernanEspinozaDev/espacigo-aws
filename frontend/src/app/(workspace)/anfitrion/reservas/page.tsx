import Link from "next/link";
import { HostAgreementRequests } from "@/features/host-agreements/ui/host-agreement-requests";
import { ReservationsList } from "@/features/bookings/ui/reservations-list";

export default async function HostReservasPage({
  searchParams,
}: {
  searchParams: Promise<{
    space_id?: string | string[];
    tab?: string | string[];
  }>;
}) {
  const { space_id, tab } = await searchParams;
  const initialSpaceId = typeof space_id === "string" ? space_id : undefined;
  const initialTab =
    tab === "general" || tab === "finanzas" || tab === "lista"
      ? tab
      : initialSpaceId
        ? "lista"
        : undefined;

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Link
          href="/anfitrion/liquidaciones"
          className="text-sm font-medium underline"
        >
          Ver liquidaciones
        </Link>
      </div>
      <HostAgreementRequests />
      <ReservationsList
        mode="host"
        initialSpaceId={initialSpaceId}
        initialSection={initialTab}
        title="Reservas, Finanzas y Horarios"
        description="Supervisa KPIs de reservas, finanzas netas con Mercado Pago, agenda horaria y comprobantes de cada espacio."
      />
    </div>
  );
}
