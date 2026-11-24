import type { Metadata } from "next";
import { AdminFinanceView } from "@/features/admin/ui/admin-finance-view";

export const metadata: Metadata = {
  title: "Finanzas y Reservas | Administración | EspaciGo",
  description:
    "Conciliación de pagos en CLP, reservas y contratos comerciales transversales.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function AdministracionFinanzasPage() {
  return <AdminFinanceView />;
}
