import type { Metadata } from "next";
import Link from "next/link";
import { HowItWorksInteractive } from "@/features/public-catalog/ui/how-it-works-interactive";
import { MercadoPagoSecuritySection } from "@/features/public-catalog/ui/landing-promo-sections";

export const metadata: Metadata = {
  title: "Cómo funciona EspaciGo · Arrendatarios y Arrendadores",
  description:
    "Descubre cómo funciona EspaciGo paso a paso para arrendatarios y arrendadores: verificación, búsqueda en mapa, contratos digitales y pagos con Mercado Pago.",
  alternates: { canonical: "/como-funciona" },
};

export default async function HowItWorksPage({
  searchParams,
}: {
  searchParams: Promise<{ rol?: string }>;
}) {
  const { rol } = await searchParams;
  const initialRole = rol === "arrendador" ? "arrendador" : "arrendatario";

  return (
    <div>
      <div className="mx-auto max-w-7xl px-4 pt-10 sm:px-6 lg:px-8">
        <nav
          aria-label="Breadcrumb"
          className="mb-6 text-sm text-muted-foreground"
        >
          <Link href="/" className="hover:text-foreground">
            Inicio
          </Link>
          <span aria-hidden="true"> / </span>
          <span aria-current="page">Cómo funciona</span>
        </nav>
        <header className="max-w-3xl">
          <h1 className="text-3xl font-semibold tracking-tight text-foreground sm:text-5xl">
            Cómo funciona EspaciGo
          </h1>
          <p className="mt-4 text-base leading-relaxed text-muted-foreground sm:text-lg">
            Diseñado para que arrendatarios encuentren y arrienden espacios con
            respaldo, y para que arrendadores publiquen gratis y gestionen sus
            contratos y cobros en un solo lugar.
          </p>
        </header>
      </div>

      <HowItWorksInteractive initialRole={initialRole} />
      <MercadoPagoSecuritySection />
    </div>
  );
}
