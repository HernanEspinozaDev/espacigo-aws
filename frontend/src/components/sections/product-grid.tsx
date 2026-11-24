import { MapPin } from "lucide-react";
import Link from "next/link";
import type { PublicSpace } from "@/features/public-catalog/domain/types";
import { PublicCatalogCard } from "@/features/public-catalog/ui/public-catalog-card";

export type ProductGridProps = {
  title?: string;
  description?: string;
  spaces: readonly PublicSpace[];
  showViewAll?: boolean;
  viewAllHref?: string;
  mapHref?: string;
};

export function ProductGrid({
  title = "Descubre espacios",
  description = "Publicaciones que la API pública de EspaciGo entrega como elegibles.",
  spaces,
  showViewAll = true,
  viewAllHref = "/explorar",
  mapHref,
}: ProductGridProps) {
  return (
    <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
      <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
            {title}
          </h2>
          <p className="mt-2 max-w-2xl text-muted-foreground">{description}</p>
        </div>
        {(showViewAll || mapHref) ? (
          <div className="hidden items-center gap-4 sm:inline-flex">
            {mapHref ? (
              <Link
                href={mapHref}
                className="inline-flex min-h-10 items-center gap-1.5 rounded-full border border-border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:border-[var(--brand-accent)] hover:bg-[var(--brand-primary-soft)]/50"
              >
                <MapPin className="size-4 text-[var(--brand-accent)]" aria-hidden="true" />
                Ver en mapa
              </Link>
            ) : null}
            {showViewAll ? (
              <Link
                href={viewAllHref}
                className="inline-flex min-h-11 items-center gap-1 whitespace-nowrap text-sm font-medium text-muted-foreground transition-colors hover:text-primary"
              >
                Ver todos <span aria-hidden="true">→</span>
              </Link>
            ) : null}
          </div>
        ) : null}
      </div>
      {spaces.length ? (
        <div className="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
          {spaces.map((space, index) => (
            <PublicCatalogCard
              key={space.space_id}
              space={space}
              priority={index === 0}
            />
          ))}
        </div>
      ) : (
        <p className="border-t border-border py-10 text-muted-foreground">
          No hay publicaciones públicas en esta selección.
        </p>
      )}
      {showViewAll ? (
        <div className="mt-8 text-center sm:hidden">
          <Link
            href={viewAllHref}
            className="inline-flex min-h-11 items-center gap-1 text-sm font-medium hover:underline"
          >
            Ver todos <span aria-hidden="true">→</span>
          </Link>
        </div>
      ) : null}
    </section>
  );
}
