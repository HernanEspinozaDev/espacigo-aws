import { notFound } from "next/navigation";
import { ReservationLifecyclePanel } from "@/features/bookings/ui/reservation-lifecycle-panel";
import { BookingConversationPanel } from "@/features/bookings/ui/booking-conversation-panel";
import { BookingReviewsPanel } from "@/features/bookings/ui/booking-reviews-panel";
import { BookingDisputesPanel } from "@/features/bookings/ui/booking-disputes-panel";
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
      <ReservationLifecyclePanel reservationId={id} role="arrendatario" />
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        <BookingConversationPanel reservationId={id} />
        <BookingReviewsPanel reservationId={id} />
      </div>
      <BookingDisputesPanel reservationId={id} />
    </div>
  );
}
