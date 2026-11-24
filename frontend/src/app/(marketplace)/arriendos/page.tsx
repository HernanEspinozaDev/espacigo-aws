import { CheckCircle2, PlusCircle } from "lucide-react";
import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { ExplorePage } from "@/app/(marketplace)/explorar/_explore-page";
import { getCategoryGuide } from "@/features/public-catalog/domain/category-guides";
import {
  hasCatalogFilters,
  parseCatalogQuery,
} from "@/features/public-catalog/domain/query-state";
import type { RawCatalogParams } from "@/features/public-catalog/domain/types";

export const dynamic = "force-dynamic";

export async function generateMetadata({
  searchParams,
}: {
  searchParams: Promise<RawCatalogParams>;
}): Promise<Metadata> {
  const raw = await searchParams;
  const query = parseCatalogQuery(raw);
  const hasRawAttributes = Boolean(
    raw.attribute &&
      (Array.isArray(raw.attribute) ? raw.attribute.length : raw.attribute),
  );
  const filtered =
    hasRawAttributes ||
    hasCatalogFilters({ ...query, category_code: undefined });
  return {
    title: "Arriendos residenciales · Casas, departamentos y cabañas",
    description:
      "Explora arriendos residenciales en EspaciGo en cuadrícula o mapa con filtros por comuna, cercanía y presupuesto.",
    alternates: { canonical: "/arriendos" },
    robots: filtered
      ? { index: false, follow: true }
      : { index: true, follow: true },
  };
}

export default async function ResidentialRentalsPage({
  searchParams,
}: {
  searchParams: Promise<RawCatalogParams>;
}) {
  const guide = getCategoryGuide("inmuebles");

  return (
    <ExplorePage
      searchParams={searchParams}
      view="grid"
      lockedCategoryCode="inmuebles"
      actionPathOverride="/arriendos"
      titleOverride="Arriendos residenciales"
      breadcrumb={
        <nav
          aria-label="Breadcrumb"
          className="mb-5 text-sm text-muted-foreground"
        >
          <Link href="/" className="hover:text-foreground">
            Inicio
          </Link>
          <span aria-hidden="true"> / </span>
          <span aria-current="page">Arriendos residenciales</span>
        </nav>
      }
      introSlot={
        <div className="mb-6 grid items-center gap-6 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-5 sm:p-6 lg:grid-cols-[1.2fr_0.8fr]">
          <div>
            <p className="inline-flex items-center rounded-full bg-[var(--brand-primary-soft)] px-3 py-0.5 text-xs font-semibold uppercase tracking-[0.14em] text-[var(--brand-primary-soft-ink)]">
              Categoría activa · Arriendos residenciales
            </p>
            <p className="mt-3 text-base leading-relaxed text-foreground">
              {guide.description}
            </p>
            <ul className="mt-3 flex flex-wrap gap-2">
              {guide.examples.map((ex) => (
                <li
                  key={ex}
                  className="inline-flex items-center gap-1 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground"
                >
                  <CheckCircle2
                    className="size-3 text-[var(--brand-success)]"
                    aria-hidden="true"
                  />
                  {ex}
                </li>
              ))}
            </ul>
            <div className="mt-4 flex flex-wrap gap-3">
              <Link
                href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
                className="inline-flex min-h-9 items-center gap-1.5 rounded-full bg-primary px-4 text-xs font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
              >
                <PlusCircle className="size-3.5" aria-hidden="true" />
                Publicar propiedad gratis
              </Link>
              <Link
                href="/explorar"
                className="inline-flex min-h-9 items-center rounded-full border border-border bg-background px-4 text-xs font-medium text-foreground hover:bg-secondary"
              >
                Explorar todas las categorías
              </Link>
            </div>
          </div>
          <div className="relative aspect-[16/10] w-full overflow-hidden rounded-xl border border-border bg-background">
            <Image
              src={guide.image}
              alt={guide.imageAlt}
              fill
              sizes="(max-width: 1024px) 100vw, 35vw"
              className="object-cover"
            />
          </div>
        </div>
      }
    />
  );
}
