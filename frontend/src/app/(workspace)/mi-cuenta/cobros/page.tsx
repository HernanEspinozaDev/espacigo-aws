import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { SyntheticPayoutCard } from "@/features/account/ui/synthetic-payout-card";

export const metadata: Metadata = {
  title: "Cuenta de cobro | Mi Cuenta | EspaciGo",
  description:
    "Administra tu cuenta de cobro y liquidaciones para anfitriones.",
  robots: {
    index: false,
    follow: false,
  },
};

export default function CobrosPage() {
  return (
    <div className="space-y-6">
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-1.5 text-sm text-muted-foreground"
      >
        <Link
          href="/mi-cuenta"
          className="text-primary hover:underline font-medium"
        >
          Mi perfil
        </Link>
        <ChevronRight className="size-4 shrink-0" />
        <span className="text-foreground font-medium">Cuenta de cobro</span>
      </nav>

      <div>
        <h2 className="text-2xl font-bold tracking-tight text-foreground">
          Cuenta de cobro
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configura la cuenta de recepción de fondos para tus arriendos como
          anfitrión.
        </p>
      </div>

      <SyntheticPayoutCard />
    </div>
  );
}
