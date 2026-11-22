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
    <ReservationsList
      mode="host"
      initialSpaceId={initialSpaceId}
      initialSection={initialTab}
      title="Reservas, Finanzas y Horarios"
      description="Supervisa KPIs de reservas, finanzas netas con Mercado Pago, agenda horaria y comprobantes de cada espacio."
    />
  );
}
