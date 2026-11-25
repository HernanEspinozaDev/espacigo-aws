import type { Metadata } from "next";
import { KYBAdminView } from "@/features/kyb/ui/kyb-admin-view";

export const metadata: Metadata = {
  title: "Solicitudes KYB | Administración | EspaciGo",
  description: "Revisión documental de empresas arrendadoras.",
  robots: { index: false, follow: false },
};

export default function AdministracionKYBPage() {
  return <KYBAdminView />;
}
