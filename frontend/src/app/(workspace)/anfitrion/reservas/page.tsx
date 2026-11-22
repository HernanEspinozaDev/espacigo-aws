"use client";

import Link from "next/link";
import { HostAgreementRequests } from "@/features/host-agreements/ui/host-agreement-requests";
import { ReservationsList } from "@/features/bookings/ui/reservations-list";

export default function HostReservasPage() {
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
        title="Gestión de Reservas de Anfitrión"
        description="Supervisa solicitudes, aprueba arriendos y administra las operaciones de entrega y recepción."
      />
    </div>
  );
}
