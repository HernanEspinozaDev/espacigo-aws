import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight, FileCheck } from "lucide-react";
import { AccountVerificationCard } from "@/features/account/ui/account-verification-card";
import { ProfileCard } from "@/features/account/ui/profile-card";
import { ProfilePhotoCard } from "@/features/account/ui/profile-photo-card";

export const metadata: Metadata = {
  title: "Información de tu perfil | Mi Cuenta | EspaciGo",
  description:
    "Modifica tus datos personales, fotografía de perfil y estado de verificación de identidad.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function PerfilDatosPage() {
  return (
    <div className="space-y-6">
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-1.5 text-sm text-muted-foreground"
      >
        <Link
          href="/mi-cuenta"
          className="text-primary hover:underline font-medium"
        >
          Mi perfil
        </Link>
        <ChevronRight className="size-4 shrink-0" />
        <span className="text-foreground font-medium">
          Información de tu perfil
        </span>
      </nav>

      <div>
        <h2 className="text-2xl font-bold tracking-tight text-foreground">
          Información de tu perfil
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Modifica tus datos personales, cambia tu foto de perfil y revisa tu
          estado de identidad.
        </p>
      </div>

      <div className="space-y-6">
        <ProfileCard />
        <ProfilePhotoCard />
        <AccountVerificationCard />

        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-start gap-3">
            <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
              <FileCheck className="size-5" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-foreground">
                Revisión de identidad
              </h3>
              <p className="text-xs text-muted-foreground mt-0.5">
                Accede a tus casos asignados de revisión de identidad.
              </p>
            </div>
          </div>
          <Link
            href="/mi-cuenta/revisiones"
            className="inline-flex items-center justify-center gap-1.5 text-xs font-semibold px-4 py-2 rounded-xl bg-primary/10 text-primary hover:bg-primary/20 transition-colors shrink-0"
          >
            <span>Ir a revisión de identidad</span>
            <ChevronRight className="size-4" />
          </Link>
        </div>
      </div>
    </div>
  );
}
