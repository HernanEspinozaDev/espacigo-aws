import { CheckCircle2, PlusCircle } from "lucide-react";
import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { ExplorePage } from "@/app/(marketplace)/explorar/_explore-page";
import { getCategoryGuide } from "@/features/public-catalog/domain/category-guides";
import { categoryLabel } from "@/features/public-catalog/domain/presentation";
import {
  hasCatalogFilters,
  parseCatalogQuery,
} from "@/features/public-catalog/domain/query-state";
import type { RawCatalogParams } from "@/features/public-catalog/domain/types";
import { publicCatalog } from "@/features/public-catalog/infrastructure/catalog-runtime";
import { CatalogUnavailable } from "@/features/public-catalog/ui/catalog-states";

export const dynamic = "force-dynamic";

function pathFor(code: string) {
  return `/categorias/${encodeURIComponent(code)}`;
}

export async function generateMetadata({
  params,
  searchParams,
}: {
  params: Promise<{ code: string }>;
  searchParams: Promise<RawCatalogParams>;
}): Promise<Metadata> {
  const { code } = await params;
  const raw = await searchParams;
  const [categories, query] = await Promise.all([
    publicCatalog.categories(),
    Promise.resolve(parseCatalogQuery({ ...raw, category_code: undefined })),
  ]);
  const category = categories.ok
    ? categories.value.items.find((item) => item.code === code)
    : undefined;
  const title = category ? categoryLabel(category) : "Categoría";
  const hasRawAttributes = Boolean(
    raw.attribute &&
    (Array.isArray(raw.attribute) ? raw.attribute.length : raw.attribute),
  );
  return {
    title: `${title} en arriendo`,
    description: `Explora publicaciones públicas de ${title} en EspaciGo en cuadrícula o mapa.`,
    alternates: { canonical: pathFor(code) },
    robots:
      hasRawAttributes || hasCatalogFilters(query)
        ? { index: false, follow: true }
        : { index: true, follow: true },
  };
}

export default async function CategoryPage({
  params,
  searchParams,
}: {
  params: Promise<{ code: string }>;
  searchParams: Promise<RawCatalogParams>;
}) {
  const { code } = await params;
  const categoriesResult = await publicCatalog.categories();
  if (!categoriesResult.ok) {
    return (
      <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 lg:px-8">
        <CatalogUnavailable
          title="No pudimos verificar esta categoría"
          retryHref={pathFor(code)}
        />
      </section>
    );
  }
  const category = categoriesResult.value.items.find(
    (item) => item.code === code,
  );
  if (!category) notFound();

  const label = categoryLabel(category);
  const guide = getCategoryGuide(category.code);

  return (
    <ExplorePage
      searchParams={searchParams}
      view="grid"
      lockedCategoryCode={code}
      actionPathOverride={pathFor(code)}
      titleOverride={label}
      breadcrumb={
        <nav
          aria-label="Breadcrumb"
          className="mb-5 text-sm text-muted-foreground"
        >
          <Link href="/" className="hover:text-foreground">
            Inicio
          </Link>
          <span aria-hidden="true"> / </span>
          <Link href="/categorias" className="hover:text-foreground">
            Categorías
          </Link>
          <span aria-hidden="true"> / </span>
          <span aria-current="page">{label}</span>
        </nav>
      }
      introSlot={
        <div className="mb-6 grid items-center gap-6 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-5 sm:p-6 lg:grid-cols-[1.2fr_0.8fr]">
          <div>
            <p className="inline-flex items-center rounded-full bg-[var(--brand-primary-soft)] px-3 py-0.5 text-xs font-semibold uppercase tracking-[0.14em] text-[var(--brand-primary-soft-ink)]">
              Categoría activa · {label}
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
            <p className="mt-3 text-xs leading-relaxed text-[var(--brand-muted-on-subtle)]">
              <strong className="font-semibold text-foreground">
                ¿Quieres publicar en {label}?{" "}
              </strong>
              {guide.howToPublish}
            </p>
            <div className="mt-4 flex flex-wrap gap-3">
              <Link
                href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
                className="inline-flex min-h-9 items-center gap-1.5 rounded-full bg-primary px-4 text-xs font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
              >
                <PlusCircle className="size-3.5" aria-hidden="true" />
                Publicar gratis en esta categoría
              </Link>
              <Link
                href="/explorar"
                className="inline-flex min-h-9 items-center rounded-full border border-border bg-background px-4 text-xs font-medium text-foreground hover:bg-secondary"
              >
                Ver todas las categorías
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
