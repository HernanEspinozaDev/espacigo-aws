import Link from "next/link";
import type { Metadata } from "next";
import { RenterAgreementList } from "@/features/commercial-contracts/ui/renter-agreement-list";
export const metadata: Metadata = {
  title: "Mis solicitudes de contrato | EspaciGo",
  robots: { index: false, follow: false },
};
export default function ContractRequestsPage() {
  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header className="space-y-2">
        <h1 className="text-2xl font-semibold">Mis solicitudes de contrato</h1>
        <p className="text-sm text-muted-foreground">
          Consulta las firmas y retoma el pago cuando el anfitrión acepte tu
          solicitud.
        </p>
      </header>
      <RenterAgreementList />
      <Link
        href="/explorar"
        className="inline-flex min-h-11 items-center text-sm underline underline-offset-4"
      >
        Explorar espacios
      </Link>
    </div>
  );
}
