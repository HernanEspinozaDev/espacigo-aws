import type { Metadata } from "next";
import { AccountShell } from "./account-shell";

export const metadata: Metadata = {
  title: "Mi Cuenta | EspaciGo",
  description:
    "Configuración de cuenta, perfil, privacidad, seguridad y verificaciones.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function MiCuentaLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <AccountShell>{children}</AccountShell>;
}
