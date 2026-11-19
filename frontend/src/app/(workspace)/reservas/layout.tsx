import type { Metadata } from "next";
import { AccountShell } from "../mi-cuenta/account-shell";

export const metadata: Metadata = {
  title: "Mis Reservas | EspaciGo",
  description: "Gestión y seguimiento operativo de tus arriendos y reservas.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function ReservasLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <AccountShell>{children}</AccountShell>;
}
