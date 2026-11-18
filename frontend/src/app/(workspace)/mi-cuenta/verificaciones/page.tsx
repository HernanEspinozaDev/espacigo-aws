import type { Metadata } from "next";
import { AccountVerificationCard } from "@/features/account/ui/account-verification-card";

export const metadata: Metadata = {
  title: "Verificación de Cuenta | Mi Cuenta | EspaciGo",
  description:
    "Verifica tu cuenta de usuario de manera rápida y sencilla en EspaciGo.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function VerificacionesPage() {
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

      <AccountVerificationCard showDeclarationLink={false} />
    </div>
  );
}
