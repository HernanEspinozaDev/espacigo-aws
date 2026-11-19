import type { Metadata } from "next";
import { AccountShell } from "../mi-cuenta/account-shell";

export const metadata: Metadata = {
  title: "Mis Espacios | Mi Cuenta | EspaciGo",
  description:
    "Gestión de propiedades, tarifas, calendarios y publicaciones de anfitrión en EspaciGo.",
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
