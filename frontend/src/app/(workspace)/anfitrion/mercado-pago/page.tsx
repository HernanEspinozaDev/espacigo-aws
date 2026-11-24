import type { Metadata } from "next";
import { MercadoPagoConnectionCard } from "@/features/host-payments/ui/mercado-pago-connection-card";
import { safeHostReturnPath } from "@/features/host-listings/domain/publish-guide";

export const metadata: Metadata = {
  title: "Mercado Pago | Panel de anfitrión | EspaciGo",
  robots: { index: false, follow: false },
};

const validCallbacks = new Set([
  "connected",
  "denied",
  "mode_mismatch",
  "invalid_state",
  "exchange_failed",
  "not_configured",
]);

export default async function HostMercadoPagoPage({
  searchParams,
}: {
  searchParams: Promise<{
    connection?: string | string[];
    return_to?: string | string[];
  }>;
}) {
  const { connection: value, return_to } = await searchParams;
  const callback =
    typeof value === "string" && validCallbacks.has(value) ? value : null;
  const returnTo = safeHostReturnPath(return_to);
  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      <header className="space-y-2">
        <p className="text-sm font-medium text-primary">Panel de anfitrión</p>
        <h1 className="text-2xl font-semibold tracking-tight">Pagos</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Administra la conexión de Mercado Pago que se utilizará para los pagos
          de tus reservas.
        </p>
      </header>
      <MercadoPagoConnectionCard callback={callback} returnTo={returnTo} />
    </div>
  );
}
