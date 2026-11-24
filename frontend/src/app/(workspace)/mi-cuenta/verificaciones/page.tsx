import type { Metadata } from "next";
import { IdentityVerificationFlow } from "@/features/document-verification/ui/identity-verification-flow";

export const metadata: Metadata = {
  title: "Verificación de identidad | Mi Cuenta | EspaciGo",
  description: "Completa y consulta tu expediente de identidad.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function VerificacionesPage() {
  return <IdentityVerificationFlow />;
}
