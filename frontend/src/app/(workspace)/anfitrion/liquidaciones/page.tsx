import type { Metadata } from "next";
import { MonthlySettlements } from "@/features/host-settlements/ui/monthly-settlements";

export const metadata: Metadata = {
  title: "Liquidaciones | Panel de anfitrión | EspaciGo",
  description: "Consulta el resumen mensual privado de pagos de tus espacios.",
  robots: { index: false, follow: false },
};

export default function HostSettlementsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6">
      <header className="space-y-2">
        <p className="text-sm font-medium text-primary">Panel de anfitrión</p>
        <h1 className="text-2xl font-semibold tracking-tight">Liquidaciones</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Revisa los pagos confirmados, la comisión aplicada y el resumen del
          mes.
        </p>
      </header>
      <MonthlySettlements />
    </div>
  );
}
