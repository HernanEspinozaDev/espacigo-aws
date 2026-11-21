import type { Metadata } from "next";
import { AccountShell } from "../mi-cuenta/account-shell";

export const metadata: Metadata = {
  title: "Panel de Anfitrión | Mi Cuenta | EspaciGo",
  description:
    "Gestión de propiedades, reservas, pagos con Mercado Pago, tarifas y calendarios de anfitrión en EspaciGo.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function AnfitrionLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <AccountShell>{children}</AccountShell>;
}
