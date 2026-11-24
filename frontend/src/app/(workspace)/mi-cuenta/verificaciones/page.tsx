import type { Metadata } from "next";
import { AccountVerificationCard } from "@/features/account/ui/account-verification-card";
import { safeHostReturnPath } from "@/features/host-listings/domain/publish-guide";

export const metadata: Metadata = {
  title: "Verificación de Cuenta | Mi Cuenta | EspaciGo",
  description:
    "Verifica tu cuenta de usuario de manera rápida y sencilla en EspaciGo.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function VerificacionesPage({
  searchParams,
}: {
  searchParams: Promise<{ return_to?: string | string[] }>;
}) {
  const { return_to } = await searchParams;
  const returnTo = safeHostReturnPath(return_to);
  return (
    <div className="space-y-8">
      <div className="border-b border-border/60 pb-5">
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          Verificación de cuenta
        </h1>
        <p className="text-sm text-muted-foreground mt-1.5 leading-relaxed">
          Activa la verificación rápida de tu cuenta marcando la casilla para
          operar en EspaciGo sin pasos adicionales.
        </p>
      </div>

      <AccountVerificationCard
        showDeclarationLink={false}
        returnTo={returnTo}
      />
    </div>
  );
}
