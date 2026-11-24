import {
  ArrowRight,
  Briefcase,
  Building2,
  Car,
  Flame,
  Home,
  LayoutGrid,
  Package,
  Presentation,
  Store,
  Trees,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import type { PublicSpaceCategory } from "../domain/types";
import { categoryLabel } from "../domain/presentation";

const CATEGORY_ICONS: Record<string, LucideIcon> = {
  oficina: Briefcase,
  sala_multiproposito: Presentation,
  bodega: Package,
  estacionamiento: Car,
  local_flexible: Store,
  stand: LayoutGrid,
  quincho: Flame,
  parcela_eventos: Trees,
  inmuebles: Home,
};

export function CategoryRail({
  categories,
}: {
  categories: readonly PublicSpaceCategory[];
}) {
  return (
    <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
      <div className="mb-7 flex items-end justify-between gap-6">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
            Explora por tipo
          </p>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
            Explora por categoría
          </h2>
        </div>
        <Link
          href="/categorias"
          className="hidden min-h-11 items-center gap-1 text-sm font-medium text-muted-foreground underline-offset-4 hover:text-primary hover:underline sm:inline-flex"
        >
          Ver categorías <ArrowRight className="size-4" aria-hidden="true" />
        </Link>
      </div>
      <nav aria-label="Categorías de arriendo">
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {categories.map((category) => {
            const Icon = CATEGORY_ICONS[category.code] ?? Building2;
            return (
              <li key={category.code}>
                <Link
                  href={`/categorias/${encodeURIComponent(category.code)}`}
                  className="group flex min-h-28 flex-col justify-between gap-3 rounded-2xl border border-border bg-background p-4 transition-colors hover:border-[var(--brand-accent)] hover:bg-[var(--brand-primary-soft)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:p-5"
                >
                  <span className="inline-flex size-10 items-center justify-center rounded-xl bg-[var(--brand-primary-soft)] text-[var(--brand-accent)] transition-colors group-hover:bg-primary group-hover:text-primary-foreground">
                    <Icon className="size-5" aria-hidden="true" />
                  </span>
                  <span className="flex items-end justify-between gap-2">
                    <span className="text-sm font-medium leading-snug text-foreground group-hover:text-[var(--brand-primary-soft-ink)] sm:text-base">
                      {categoryLabel(category)}
                    </span>
                    <ArrowRight
                      aria-hidden="true"
                      className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-[var(--brand-primary-soft-ink)]"
                    />
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>
    </section>
  );
}
