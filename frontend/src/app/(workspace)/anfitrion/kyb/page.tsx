import type { Metadata } from "next";
import { KYBOwnerView } from "@/features/kyb/ui/kyb-owner-view";

export const metadata: Metadata = {
  title: "Verificación empresarial KYB | EspaciGo",
  description:
    "Completa y consulta la verificación KYB de tu empresa arrendadora.",
  robots: { index: false, follow: false },
};

export default function AnfitrionKYBPage() {
  return (
    <div className="mx-auto w-full max-w-5xl">
      <header className="mb-6 space-y-1">
        <p className="text-sm font-medium text-primary">Panel de anfitrión</p>
        <h1 className="text-2xl font-semibold tracking-tight">
          Verificación empresarial
        </h1>
      </header>
      <KYBOwnerView />
    </div>
  );
}
