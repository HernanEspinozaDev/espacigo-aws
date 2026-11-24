import { ArrowRight, CheckCircle2, MapPin } from "lucide-react";
import Link from "next/link";
import type {
  PublicLocation,
  PublicSpaceCategory,
} from "@/features/public-catalog/domain/types";
import { PublicSearchForm } from "@/features/public-catalog/ui/public-search-form";
import { HeroShowcase } from "./hero-showcase";

export function Hero({
  categories,
  locations,
}: {
  categories?: readonly PublicSpaceCategory[];
  locations?: readonly PublicLocation[];
}) {
  return (
    <section className="relative overflow-hidden border-b border-border bg-[var(--brand-surface-subtle)]">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div className="grid items-center gap-10 py-10 sm:py-14 lg:grid-cols-[1.15fr_0.85fr] lg:gap-12 lg:py-16">
          <div className="min-w-0">
            <p className="mb-4 inline-flex items-center gap-2 rounded-full bg-[var(--brand-primary-soft)] px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.14em] text-[var(--brand-primary-soft-ink)]">
              Espacios para vivir, trabajar y celebrar
            </p>
            <h1 className="max-w-[18ch] text-3xl font-semibold tracking-tight text-foreground sm:text-5xl lg:text-[3.25rem] lg:leading-[1.08]">
              Arrienda el espacio que necesitas. Publica gratis el que tienes.
            </h1>
            <p className="mt-4 max-w-2xl text-base leading-relaxed text-[var(--brand-muted-on-subtle)] sm:text-lg">
              Encuentra oficinas, salas, bodegas, quinchos, estacionamientos y
              casas en todo Chile. Compara en el mapa, firma contratos digitales
              y gestiona tus arriendos en un solo lugar.
            </p>
            <PublicSearchForm categories={categories} locations={locations} />
            <div className="mt-5 flex flex-wrap items-center gap-3 sm:gap-5">
              <Link
                href="/explorar/mapa"
                className="inline-flex min-h-10 items-center gap-2 rounded-full border border-border bg-background px-4 text-sm font-medium text-foreground shadow-xs transition-colors hover:border-[var(--brand-accent)] hover:bg-[var(--brand-primary-soft)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <MapPin
                  className="size-4 text-[var(--brand-accent)]"
                  aria-hidden="true"
                />
                Explorar en mapa
              </Link>
              <Link
                href="/arriendos"
                className="inline-flex min-h-10 items-center gap-1.5 text-sm font-medium text-foreground underline-offset-4 hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Arrienda una casa
                <ArrowRight
                  className="size-4 text-[var(--brand-accent)]"
                  aria-hidden="true"
                />
              </Link>
              <Link
                href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
                className="inline-flex min-h-10 items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Publica tu espacio gratis
              </Link>
            </div>
            <ul className="mt-6 flex flex-wrap gap-x-5 gap-y-2 border-t border-border/80 pt-4 text-xs font-medium text-[var(--brand-muted-on-subtle)]">
              <li className="inline-flex items-center gap-1.5">
                <CheckCircle2
                  className="size-3.5 text-[var(--brand-success)]"
                  aria-hidden="true"
                />
                Publicación gratuita
              </li>
              <li className="inline-flex items-center gap-1.5">
                <CheckCircle2
                  className="size-3.5 text-[var(--brand-success)]"
                  aria-hidden="true"
                />
                Contratos digitales guardados
              </li>
              <li className="inline-flex items-center gap-1.5">
                <CheckCircle2
                  className="size-3.5 text-[var(--brand-success)]"
                  aria-hidden="true"
                />
                Pagos con Mercado Pago
              </li>
            </ul>
          </div>
          <HeroShowcase />
        </div>
      </div>
    </section>
  );
}
