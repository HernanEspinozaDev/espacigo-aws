import { notFound } from "next/navigation";
import { ReservationLifecyclePanel } from "@/features/bookings/ui/reservation-lifecycle-panel";
import { CommercialReservationTabs } from "@/features/bookings/ui/commercial-reservation-tabs";
import { CommercialReservationDocuments } from "@/features/commercial-contracts/ui/commercial-reservation-documents";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function ReservationDetailPage({ params }: PageProps) {
  const { id } = await params;

  if (!id) {
    notFound();
  }

  return (
    <div className="space-y-10">
      <CommercialReservationDocuments key={id} reservationId={id} />
      <ReservationLifecyclePanel
        key={id}
        reservationId={id}
        role="arrendatario"
      />
      <CommercialReservationTabs key={id} reservationId={id} />
    </div>
  );
}
