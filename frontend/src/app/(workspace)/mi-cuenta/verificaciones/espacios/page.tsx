import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { SpaceVerificationsHub } from "@/features/kyb/ui/space-verifications-hub";

export const metadata: Metadata = {
  title: "Verificaciones de espacio | Mi Cuenta | EspaciGo",
  description:
    "Consulta y envía la verificación documental, Certificado SII y verificación empresarial KYB de cada espacio.",
  robots: { index: false, follow: false },
};

export default function VerificacionesEspaciosPage() {
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
        <Link
          href="/mi-cuenta/verificaciones"
          className="font-medium text-primary hover:underline"
        >
          Verificaciones
        </Link>
        <ChevronRight className="size-4 shrink-0" aria-hidden="true" />
        <span aria-current="page" className="font-medium text-foreground">
          Verificaciones KYB
        </span>
      </nav>
      <div>
        <h1 className="text-2xl font-bold tracking-tight">
          Verificaciones de espacio
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Documentación asignada a cada espacio publicado, respaldo de inmueble,
          Inicio de Actividades SII y carpeta empresarial KYB.
        </p>
      </div>
      <SpaceVerificationsHub />
    </div>
  );
}
