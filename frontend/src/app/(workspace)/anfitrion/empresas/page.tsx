import type { Metadata } from "next";
import { EmpresasView } from "@/features/organizations/ui/empresas-view";

export const metadata: Metadata = {
  title: "Empresas | EspaciGo",
  description:
    "Administra tu perfil comercial y el equipo de tu empresa anfitriona.",
  robots: { index: false, follow: false },
};

export default function EmpresasPage() {
  return (
    <div className="mx-auto w-full max-w-5xl">
      <header className="mb-6 space-y-1">
        <p className="text-sm font-medium text-primary">Panel de anfitrión</p>
        <h1 className="text-2xl font-semibold tracking-tight">Empresas</h1>
        <p className="text-sm text-muted-foreground">
          Administra tus perfiles comerciales, representantes y equipo.
        </p>
      </header>
      <EmpresasView />
    </div>
  );
}
