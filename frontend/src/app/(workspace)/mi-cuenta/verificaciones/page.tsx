import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { safeHostReturnPath } from "@/features/host-listings/domain/publish-guide";
import { IdentityVerificationCard } from "@/features/identity-profile/ui/identity-verification-card";
import { ReviewerQueueView } from "@/features/reviewer-queue/ui/reviewer-queue-view";

export const metadata: Metadata = {
  title: "Verificaciones | Mi Cuenta | EspaciGo",
};

export default async function VerificacionesPage({
  searchParams,
}: {
  searchParams: Promise<{
    return_to?: string | string[];
    refresh_identity?: string | string[];
  }>;
}) {
  const { return_to, refresh_identity } = await searchParams;
  const returnTo = safeHostReturnPath(return_to);
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
          Verificaciones
        </span>
      </nav>
      <h1 className="text-2xl font-bold tracking-tight">Verificaciones</h1>
      <IdentityVerificationCard
        returnTo={returnTo}
        refreshOnMount={refresh_identity === "1"}
      />
      <ReviewerQueueView assignedOnly />
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
  );
}
