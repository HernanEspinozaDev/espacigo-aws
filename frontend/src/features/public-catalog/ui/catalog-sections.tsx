import Link from "next/link";
import type { CatalogQuery, PublicSpace } from "../domain/types";
import { PublicCatalogCard } from "./public-catalog-card";
import { NextCatalogPage } from "./next-catalog-page";
import { EmptyCatalog } from "./catalog-states";

export function PublicCatalogGrid({
  spaces,
  query,
  nextCursor,
  actionPath = "/explorar",
  emptyTitle = "Todavía no hay publicaciones que mostrar",
  emptyDescription = "Prueba con otra búsqueda o vuelve más tarde.",
  ownerCta = false,
  explore = false,
}: {
  spaces: readonly PublicSpace[];
  query: CatalogQuery;
  nextCursor?: string;
  actionPath?: string;
  emptyTitle?: string;
  emptyDescription?: string;
  ownerCta?: boolean;
  explore?: boolean;
}) {
  if (spaces.length === 0) {
    return (
      <EmptyCatalog
        title={emptyTitle}
        description={emptyDescription}
        ownerCta={ownerCta}
      />
    );
  }
  return (
    <>
      <div
        className={`grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 ${explore ? "gap-x-5 gap-y-8 sm:gap-x-7 sm:gap-y-10" : "gap-x-6 gap-y-10 sm:gap-x-8 lg:gap-y-14"}`}
      >
        {spaces.map((space, index) => (
          <div
            key={space.space_id}
            id={`space-card-${space.space_id}`}
            data-space-id={space.space_id}
            tabIndex={-1}
            className="rounded-xl focus-visible:outline-2 focus-visible:outline-ring"
          >
            <PublicCatalogCard
              space={space}
              priority={index === 0}
              explore={explore}
            />
          </div>
        ))}
      </div>
      <NextCatalogPage
        query={query}
        nextCursor={nextCursor}
        href={actionPath}
      />
    </>
  );
}

export function OwnerCallout({
  title = "¿Tienes un espacio o propiedad disponible? Publícalo en EspaciGo.",
  description = "Ofrece oficinas, salas, bodegas, quinchos, estacionamientos o arriendos residenciales con reglas claras, tarifas visibles y gestión centralizada.",
}: {
  title?: string;
  description?: string;
}) {
  return (
    <section className="bg-foreground text-background">
      <div className="mx-auto flex max-w-7xl flex-col gap-7 px-4 py-16 sm:px-6 sm:py-20 lg:flex-row lg:items-end lg:justify-between lg:px-8">
        <div className="max-w-3xl">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--brand-accent)]">
            Para propietarios y anfitriones
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
            {title}
          </h2>
          <p className="mt-4 max-w-2xl leading-relaxed text-background/80">
            {description}
          </p>
        </div>
        <Link
          href="/anfitriones"
          className="inline-flex min-h-12 shrink-0 items-center justify-center rounded-full bg-primary px-6 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background"
        >
          Conoce cómo ofrecer tu espacio
        </Link>
      </div>
    </section>
  );
}
