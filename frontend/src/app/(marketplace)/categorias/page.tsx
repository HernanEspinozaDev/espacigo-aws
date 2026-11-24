import { ArrowRight, CheckCircle2, MapPin, PlusCircle } from "lucide-react";
import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { getCategoryGuide } from "@/features/public-catalog/domain/category-guides";
import { categoryLabel } from "@/features/public-catalog/domain/presentation";
import { publicCatalog } from "@/features/public-catalog/infrastructure/catalog-runtime";
import { CatalogUnavailable } from "@/features/public-catalog/ui/catalog-states";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Categorías de arriendo",
  description:
    "Explora las categorías de espacios y arriendos residenciales en EspaciGo, qué puedes publicar en cada una y cómo explorarlas en el mapa.",
  alternates: { canonical: "/categorias" },
};

export default async function CategoriesPage() {
  const result = await publicCatalog.categories();
  if (!result.ok) {
    return (
      <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 lg:px-8">
        <CatalogUnavailable
          title="No pudimos cargar las categorías"
          retryHref="/categorias"
        />
      </section>
    );
  }
  const categories = [...result.value.items].sort((a, b) => a.order - b.order);

  return (
    <section className="mx-auto max-w-7xl px-4 py-10 sm:px-6 sm:py-14 lg:px-8">
      <nav
        aria-label="Breadcrumb"
        className="mb-7 text-sm text-muted-foreground"
      >
        <Link href="/" className="hover:text-foreground">
          Inicio
        </Link>
        <span aria-hidden="true"> / </span>
        <span aria-current="page">Categorías</span>
      </nav>

      <header className="rounded-3xl border border-border bg-[var(--brand-surface-subtle)] p-6 sm:p-10">
        <p className="inline-flex items-center rounded-full bg-[var(--brand-primary-soft)] px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-[var(--brand-primary-soft-ink)]">
          Qué puedes arrendar o publicar
        </p>
        <h1 className="mt-4 max-w-3xl text-3xl font-semibold tracking-tight text-foreground sm:text-5xl">
          Categorías de espacios en EspaciGo
        </h1>
        <p className="mt-4 max-w-3xl text-base leading-relaxed text-[var(--brand-muted-on-subtle)] sm:text-lg">
          Conoce qué tipo de espacios puedes buscar o publicar gratis en la
          plataforma. Entra a cualquier categoría para ver su guía de
          publicación y explorar anuncios disponibles en cuadrícula o mapa.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link
            href="/explorar/mapa"
            className="inline-flex min-h-11 items-center gap-2 rounded-full border border-border bg-background px-5 text-sm font-medium text-foreground transition-colors hover:border-[var(--brand-accent)]"
          >
            <MapPin
              className="size-4 text-[var(--brand-accent)]"
              aria-hidden="true"
            />
            Explorar mapa completo
          </Link>
          <Link
            href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
            className="inline-flex min-h-11 items-center gap-2 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
          >
            <PlusCircle className="size-4" aria-hidden="true" />
            Publicar espacio gratis
          </Link>
        </div>
      </header>

      {categories.length ? (
        <ul className="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {categories.map((category) => {
            const guide = getCategoryGuide(category.code);
            const label = categoryLabel(category);
            return (
              <li
                key={category.code}
                className="flex flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xs transition-shadow hover:shadow-md"
              >
                <div className="relative aspect-[16/10] w-full overflow-hidden bg-[var(--brand-surface-subtle)]">
                  <Image
                    src={guide.image}
                    alt={guide.imageAlt}
                    fill
                    sizes="(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw"
                    className="object-cover"
                  />
                  <span className="absolute left-3 top-3 rounded-full bg-background/95 px-3 py-1 text-xs font-semibold tabular-nums text-foreground shadow-xs">
                    {String(category.order).padStart(2, "0")} · {label}
                  </span>
                </div>

                <div className="flex flex-1 flex-col justify-between p-5 sm:p-6">
                  <div>
                    <h2 className="text-xl font-semibold text-foreground">
                      {label}
                    </h2>
                    <p className="mt-1 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--brand-accent)]">
                      {guide.tagline}
                    </p>
                    <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
                      {guide.description}
                    </p>

                    <ul
                      aria-label={`Ejemplos de ${label}`}
                      className="mt-4 flex flex-wrap gap-1.5"
                    >
                      {guide.examples.map((example) => (
                        <li
                          key={example}
                          className="inline-flex items-center gap-1 rounded-full border border-border bg-[var(--brand-surface-subtle)] px-2.5 py-1 text-xs text-foreground"
                        >
                          <CheckCircle2
                            className="size-3 text-[var(--brand-success)]"
                            aria-hidden="true"
                          />
                          {example}
                        </li>
                      ))}
                    </ul>

                    <p className="mt-4 rounded-xl bg-[var(--brand-surface-subtle)] p-3 text-xs leading-relaxed text-[var(--brand-muted-on-subtle)]">
                      <strong className="font-semibold text-foreground">
                        Cómo se sube:{" "}
                      </strong>
                      {guide.howToPublish}
                    </p>
                  </div>

                  <div className="mt-5 flex items-center justify-between gap-3 border-t border-border pt-4">
                    <Link
                      href={`/categorias/${encodeURIComponent(category.code)}`}
                      className="inline-flex min-h-10 items-center gap-1.5 rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
                    >
                      Ver categoría
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                    <Link
                      href={`/explorar/mapa?category_code=${encodeURIComponent(category.code)}`}
                      className="inline-flex min-h-10 items-center gap-1 text-xs font-medium text-muted-foreground hover:text-primary"
                    >
                      <MapPin className="size-3.5" aria-hidden="true" />
                      Ver mapa
                    </Link>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="mt-10 border-t border-border py-10 text-muted-foreground">
          No hay categorías activas disponibles en el catálogo en este momento.
        </p>
      )}
    </section>
  );
}
