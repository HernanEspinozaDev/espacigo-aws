import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { EmpresasView } from "@/features/organizations/ui/empresas-view";

export const metadata: Metadata = {
  title: "Perfil empresas | Mi Cuenta | EspaciGo",
  description:
    "Registra tus empresas, gestiona representantes y envía la verificación empresarial KYB.",
  robots: { index: false, follow: false },
};

export default function MiCuentaEmpresasPage() {
  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-1.5 text-sm text-muted-foreground"
      >
        <Link
          href="/mi-cuenta"
          className="font-medium text-primary hover:underline"
        >
          General
        </Link>
        <ChevronRight className="size-4 shrink-0" aria-hidden="true" />
        <span aria-current="page" className="font-medium text-foreground">
          Perfil empresas
        </span>
      </nav>
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Perfil empresas</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Registra tus empresas (razón social, nombre comercial y RUT), gestiona
          representantes y completa la verificación empresarial (KYB).
        </p>
      </div>
      <EmpresasView />
    </div>
  );
}
