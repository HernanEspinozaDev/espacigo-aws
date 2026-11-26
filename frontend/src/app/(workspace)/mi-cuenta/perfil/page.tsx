import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { redirect } from "next/navigation";
import { IdentityProfileCard } from "@/features/identity-profile/ui/identity-profile-card";
import { ProfilePhotoCard } from "@/features/account/ui/profile-photo-card";
import {
  safeHostReturnPath,
  verificationReturnHref,
} from "@/features/host-listings/domain/publish-guide";

export const metadata: Metadata = {
  title: "Información de tu perfil | Mi Cuenta | EspaciGo",
  description:
    "Modifica tus datos personales, fotografía de perfil y estado de verificación de identidad.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function PerfilDatosPage({
  searchParams,
}: {
  searchParams: Promise<{
    return_to?: string | string[];
    refresh_identity?: string | string[];
    editar?: string | string[];
  }>;
}) {
  const { return_to, refresh_identity, editar } = await searchParams;
  const returnTo = safeHostReturnPath(return_to);
  if (refresh_identity === "1") {
    redirect(verificationReturnHref(returnTo, { refresh: true }));
  }
  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
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
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          Información de tu perfil
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Modifica tus datos personales, cambia tu foto de perfil y revisa tu
          estado de identidad.
        </p>
      </div>

      <div className="space-y-6">
        <IdentityProfileCard editOnMount={editar === "1"} returnTo={returnTo} />
        <ProfilePhotoCard />
        {returnTo ? (
          <Link
            href={returnTo}
            className="inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm font-medium text-primary hover:bg-secondary"
          >
            Volver al borrador de publicación
            <ChevronRight className="size-4" aria-hidden="true" />
          </Link>
        ) : null}
      </div>
    </div>
  );
}
