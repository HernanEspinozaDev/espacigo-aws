import type { Metadata } from "next";
import { AdminPrivacyView } from "@/features/admin/ui/admin-privacy-view";

export const metadata: Metadata = {
  title: "Privacidad y KYC | Administración | EspaciGo",
  description:
    "Solicitudes ARCO+ (Ley 21.719) y resumen operativo de casos KYC.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function AdministracionPrivacidadPage() {
  return <AdminPrivacyView />;
}
