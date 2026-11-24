import type { Metadata } from "next";
import { AdminApprovalsView } from "@/features/admin/ui/admin-approvals-view";

export const metadata: Metadata = {
  title: "Aprobaciones y Soporte | Administración | EspaciGo",
  description:
    "Cola de doble control para operaciones críticas y bandeja de tickets de soporte.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function AdministracionAprobacionesPage() {
  return <AdminApprovalsView />;
}
