import { ArrowRight, Home as HomeIcon, MapPin } from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";
import { Hero } from "@/components/sections/hero";
import { ProductGrid } from "@/components/sections/product-grid";
import { publicCatalog } from "@/features/public-catalog/infrastructure/catalog-runtime";
import { CategoryRail } from "@/features/public-catalog/ui/category-rail";
import {
  CatalogUnavailable,
  EmptyCatalog,
} from "@/features/public-catalog/ui/catalog-states";
import { HowItWorksPreview } from "@/features/public-catalog/ui/how-it-works-preview";
import {
  MercadoPagoSecuritySection,
  PlatformFeaturesSection,
  VerifiedSpacesBanner,
} from "@/features/public-catalog/ui/landing-promo-sections";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Arriendos residenciales y espacios flexibles",
  description:
    "Encuentra oficinas, salas, bodegas, quinchos y arriendos residenciales. Explora en el mapa, firma contratos digitales y publica gratis en EspaciGo.",
  alternates: { canonical: "/" },
};

export default async function Home() {
  const [categoriesResult, locationsResult, spacesResult, residentialResult] =
    await Promise.all([
      publicCatalog.categories(),
      publicCatalog.locations(),
      publicCatalog.spaces({ attribute: [], page_size: 9 }),
      publicCatalog.spaces({
        category_code: "inmuebles",
        attribute: [],
        page_size: 3,
      }),
    ]);

  const categories = categoriesResult.ok ? categoriesResult.value.items : [];
  const locations = locationsResult.ok ? locationsResult.value.items : [];

  const residentialItems = residentialResult.ok
    ? residentialResult.value.items.slice(0, 3)
    : [];
  const residentialIds = new Set(residentialItems.map((s) => s.space_id));

  const allSpaces = spacesResult.ok ? spacesResult.value.items : [];
  const nonResidentialSpaces = allSpaces
    .filter((s) => !residentialIds.has(s.space_id))
    .slice(0, 6);
  const showDedicatedResidentialGrid =
    residentialItems.length > 0 && nonResidentialSpaces.length > 0;
  const mainGridSpaces = showDedicatedResidentialGrid
    ? nonResidentialSpaces
    : allSpaces.slice(0, 6);

  return (
    <>
      <Hero categories={categories} locations={locations} />

      {categoriesResult.ok ? (
        categories.length ? (
          <CategoryRail categories={categories} />
        ) : (
          <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 lg:px-8">
            <p className="text-muted-foreground">
              No hay categorías activas disponibles en este momento.
            </p>
          </section>
        )
      ) : (
        <section className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <CatalogUnavailable
            title="No pudimos cargar las categorías"
            retryHref="/categorias"
          />
        </section>
      )}

      {spacesResult.ok ? (
        mainGridSpaces.length ? (
          <ProductGrid
            title="Descubre espacios"
            description="Publicaciones vigentes listas para revisar tarifas, reglas y ubicación."
            spaces={mainGridSpaces}
            mapHref="/explorar/mapa"
          />
        ) : (
          <section className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
            <EmptyCatalog
              title="Aún no hay publicaciones públicas para mostrar"
              description="Vuelve a explorar más tarde o revisa las categorías disponibles."
            />
          </section>
        )
      ) : (
        <section className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <CatalogUnavailable title="No pudimos cargar las publicaciones" />
        </section>
      )}

      <VerifiedSpacesBanner />

      <PlatformFeaturesSection />

      <section className="border-y border-border bg-[var(--brand-surface-subtle)]">
        <div className="mx-auto max-w-7xl px-4 py-14 sm:px-6 sm:py-20 lg:px-8">
          <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div className="max-w-3xl">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--brand-muted-on-subtle)]">
                Arriendos residenciales
              </p>
              <h2 className="mt-2 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
                Arrienda una casa o departamento
              </h2>
              <p className="mt-3 leading-relaxed text-[var(--brand-muted-on-subtle)]">
                Explora casas, departamentos, habitaciones, cabañas y casas de
                veraneo disponibles en el catálogo.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <Link
                href="/explorar/mapa?category_code=inmuebles"
                className="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-full border border-border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:border-[var(--brand-accent)]"
              >
                <MapPin
                  className="size-4 text-[var(--brand-accent)]"
                  aria-hidden="true"
                />
                Ver en mapa
              </Link>
              <Link
                href="/arriendos"
                className="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Ver todos <ArrowRight className="size-4" aria-hidden="true" />
              </Link>
            </div>
          </div>
          {residentialResult.ok ? (
            showDedicatedResidentialGrid ? (
              <ProductGrid
                title="Publicaciones residenciales"
                description="Opciones habitacionales con tarifa y modalidad declarada."
                spaces={residentialItems}
                showViewAll={false}
              />
            ) : residentialItems.length > 0 ? (
              <div className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-border bg-background p-6 sm:flex-row sm:items-center">
                <div className="flex items-start gap-4">
                  <span className="inline-flex size-11 shrink-0 items-center justify-center rounded-xl bg-[var(--brand-primary-soft)] text-[var(--brand-accent)]">
                    <HomeIcon className="size-5" aria-hidden="true" />
                  </span>
                  <div>
                    <h3 className="text-base font-semibold text-foreground">
                      Explora todas las opciones residenciales
                    </h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Revisa el listado completo de inmuebles disponibles con
                      filtros por comuna, modalidad y presupuesto.
                    </p>
                  </div>
                </div>
                <Link
                  href="/arriendos"
                  className="inline-flex min-h-11 shrink-0 items-center gap-1.5 text-sm font-medium text-primary hover:underline"
                >
                  Ir a arriendos residenciales{" "}
                  <ArrowRight className="size-4" aria-hidden="true" />
                </Link>
              </div>
            ) : (
              <EmptyCatalog
                title="Aún no hay publicaciones residenciales"
                description="La categoría admite distintas formas de arriendo, pero hoy no hay anuncios residenciales públicos que mostrar."
                ownerCta
              />
            )
          ) : (
            <CatalogUnavailable
              title="No pudimos consultar los arriendos residenciales"
              retryHref="/arriendos"
            />
          )}
        </div>
      </section>

      <HowItWorksPreview />
      <MercadoPagoSecuritySection />
    </>
  );
}
