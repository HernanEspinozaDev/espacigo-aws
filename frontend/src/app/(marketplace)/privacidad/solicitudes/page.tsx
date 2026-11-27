import type { Metadata } from "next";
import { PublicPrivacyPortal } from "@/features/privacy-public/ui/public-privacy-portal";

export const metadata: Metadata = {
  title: "Solicitudes de privacidad ARCO+ sin sesión",
  description:
    "Ingresa y consulta solicitudes de derechos ARCO+ (acceso, rectificación, supresión, oposición, portabilidad y bloqueo temporal) sin iniciar sesión en EspaciGo.",
  alternates: { canonical: "/privacidad/solicitudes" },
};

export default function PublicPrivacyRequestsPage() {
  return <PublicPrivacyPortal />;
}
