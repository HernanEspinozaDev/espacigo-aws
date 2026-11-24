import Link from "next/link";
import {
  parseCatalogQuery,
  hasCatalogFilters,
  serializeCatalogQuery,
} from "@/features/public-catalog/domain/query-state";
import type {
  RawCatalogParams,
  PublicCategoryFilter,
} from "@/features/public-catalog/domain/types";
import { publicCatalog } from "@/features/public-catalog/infrastructure/catalog-runtime";
import { ActiveCatalogFilters } from "@/features/public-catalog/ui/active-catalog-filters";
import { ExploreDetailVisitTracker } from "@/features/public-catalog/ui/explore-session";
import { CatalogFilters } from "@/features/public-catalog/ui/catalog-filters";
import { CatalogSortControls } from "@/features/public-catalog/ui/catalog-sort-controls";
import {
  CatalogUnavailable,
  EmptyCatalog,
} from "@/features/public-catalog/ui/catalog-states";
import { PublicCatalogGrid } from "@/features/public-catalog/ui/catalog-sections";
import { PublicSearchForm } from "@/features/public-catalog/ui/public-search-form";

import { CatalogProximityControls } from "@/features/public-catalog/ui/catalog-proximity-controls";
import { ExploreViewControls } from "@/features/public-catalog/ui/explore-view-controls";
import { ExploreMapResults } from "@/features/public-catalog/ui/explore-map-results";

function first(params: RawCatalogParams, key: string) {
  const value = params[key];
  return typeof value === "string" ? value : undefined;
}

export async function ExplorePage({
  searchParams,
  view,
  lockedCategoryCode,
  actionPathOverride,
  breadcrumb,
  titleOverride,
  introSlot,
}: {
  searchParams: Promise<RawCatalogParams>;
  view: "grid" | "map";
  lockedCategoryCode?: string;
  actionPathOverride?: string;
  breadcrumb?: React.ReactNode;
  titleOverride?: string;
  introSlot?: React.ReactNode;
}) {
  const raw = await searchParams;
  const [categoriesResult, locationsResult] = await Promise.all([
    publicCatalog.categories(),
    publicCatalog.locations({ limit: 400 }),
  ]);

  let effectiveParams: RawCatalogParams = {
    ...raw,
    category_code: lockedCategoryCode ?? raw.category_code ?? raw.category,
    category: undefined,
  };
  let invalidCategory = false;
  const requestedCategory = first(effectiveParams, "category_code");
  if (
    categoriesResult.ok &&
    requestedCategory &&
    !categoriesResult.value.items.some(
      (category) => category.code === requestedCategory,
    )
  ) {
    effectiveParams = {
      ...effectiveParams,
      category_code: undefined,
      attribute: undefined,
    };
    invalidCategory = true;
  }

  const categoryCode = first(effectiveParams, "category_code");
  let categoryFilters: readonly PublicCategoryFilter[] = [];
  let filterProfileUnavailable = false;
  if (categoryCode) {
    const profile = await publicCatalog.categoryFilters(categoryCode);
    if (profile.ok) categoryFilters = profile.value.items;
    else if (profile.error.status !== 404) filterProfileUnavailable = true;
  }

  const isMap = view === "map";
  const actionPath =
    actionPathOverride ?? (isMap ? "/explorar/mapa" : "/explorar");
  const query = parseCatalogQuery(effectiveParams, categoryFilters);
  const filterKey = serializeCatalogQuery({
    ...query,
    cursor: undefined,
  }).toString();
  const results = isMap
    ? { view: "map" as const, result: await publicCatalog.mapMarkers(query) }
    : { view: "grid" as const, result: await publicCatalog.spaces(query) };
  const noResults =
    results.result.ok && results.result.value.items.length === 0;
  const radius = Number(query.radius_km);
  const expandedRadius = radius < 25 ? 25 : 50;
  const categories = categoriesResult.ok ? categoriesResult.value.items : [];
  const locations = locationsResult.ok ? locationsResult.value.items : [];
  const rawAttributes = raw.attribute;
  const requestedAttributeCount = Array.isArray(rawAttributes)
    ? rawAttributes.length
    : rawAttributes
      ? 1
      : 0;
  const attributesWereDiscarded =
    requestedAttributeCount > query.attribute.length;

  return (
    <section className="mx-auto max-w-7xl px-3 pb-20 pt-5 sm:px-6 sm:pt-9 lg:px-8">
      {breadcrumb}
      {introSlot}
      <header className="mb-4 sm:mb-6">
        <h1 className="text-2xl font-semibold tracking-tight sm:text-4xl">
          {titleOverride ?? (isMap ? "Mapa de arriendos" : "Explorar arriendos")}
        </h1>
        <PublicSearchForm
          compact
          initialQuery={query.q ?? ""}
          actionPath={actionPath}
          retainedFilters={serializeCatalogQuery({
            ...query,
            q: undefined,
            cursor: undefined,
          })}
        />
      </header>

      <div className="sticky top-16 z-30 -mx-3 mb-3 space-y-2 border-b bg-background/95 px-3 py-2 backdrop-blur sm:static sm:mx-0 sm:mb-5 sm:px-0">
        <ExploreDetailVisitTracker filters={filterKey} isMap={isMap} />
        <ExploreViewControls
          isMap={isMap}
          query={query}
          categories={lockedCategoryCode ? [] : categories}
          gridHref={lockedCategoryCode ? actionPath : "/explorar"}
        />
        <div className="flex flex-wrap items-center gap-2">
          <CatalogProximityControls
            key={[
              query.areaMode,
              query.region_code,
              query.comuna_code,
              query.latitude,
              query.longitude,
              query.radius_km,
              query.bbox,
            ].join(":")}
            query={query}
            locations={locations}
            actionPath={actionPath}
          />
          <CatalogFilters
            actionPath={actionPath}
            categories={lockedCategoryCode ? [] : categories}
            locations={locations}
            filters={categoryFilters}
            query={query}
            compact
          />
          <CatalogSortControls actionPath={actionPath} query={query} compact />
        </div>
        <ActiveCatalogFilters
          actionPath={actionPath}
          query={query}
          locations={locations}
          categories={categories}
          categoryFilters={categoryFilters}
        />
      </div>

      <div className="min-w-0">
        {!isMap && (
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm text-muted-foreground">
              {query.q
                ? `Resultados para “${query.q}”`
                : "Espacios disponibles"}
            </p>
          </div>
        )}

        {invalidCategory ? (
          <p
            role="status"
            className="mb-5 rounded-xl border border-border p-4 text-sm text-muted-foreground"
          >
            La categoría seleccionada ya no está activa; se omitió ese filtro.
          </p>
        ) : null}
        {!categoriesResult.ok ? (
          <p
            role="status"
            className="mb-5 rounded-xl border border-border p-4 text-sm text-muted-foreground"
          >
            Las categorías no están disponibles temporalmente; puedes seguir
            explorando sin ese selector.
          </p>
        ) : null}
        {!locationsResult.ok ? (
          <p
            role="status"
            className="mb-5 rounded-xl border border-border p-4 text-sm text-muted-foreground"
          >
            Las ubicaciones no están disponibles temporalmente; puedes seguir
            explorando sin ese filtro.
          </p>
        ) : null}
        {filterProfileUnavailable || attributesWereDiscarded ? (
          <p
            role="status"
            className="mb-5 rounded-xl border border-border p-4 text-sm text-muted-foreground"
          >
            Algunos filtros de categoría no pudieron validarse con el perfil
            público y se omitieron.
          </p>
        ) : null}

        {results.view === "map" ? (
          <ExploreMapResults
            key={filterKey}
            query={query}
            markers={results.result.ok ? results.result.value.items : []}
            mapTruncated={results.result.ok && results.result.value.truncated}
            totalCount={
              results.result.ok ? results.result.value.total_count : 0
            }
            mapUnavailable={!results.result.ok}
          />
        ) : !results.result.ok ? (
          results.result.error.kind === "invalid" ? (
            <section role="alert" className="border-y border-border py-12">
              <h2 className="text-2xl font-medium">
                No pudimos aplicar esos filtros
              </h2>
              <p className="mt-3 max-w-xl text-muted-foreground">
                Revisa los valores o restablece la búsqueda para continuar.
              </p>
              <Link
                href="/explorar"
                className="mt-6 inline-flex min-h-11 items-center rounded-full bg-foreground px-5 text-sm font-medium text-background"
              >
                Restablecer filtros
              </Link>
            </section>
          ) : (
            <CatalogUnavailable title="No pudimos cargar las publicaciones" />
          )
        ) : results.result.value.items.length ? (
          <PublicCatalogGrid
            spaces={results.result.value.items}
            query={query}
            nextCursor={results.result.value.next_cursor}
            actionPath={actionPath}
            explore
          />
        ) : (
          <EmptyCatalog
            title={
              query.q
                ? `No encontramos publicaciones para “${query.q}”`
                : hasCatalogFilters(query)
                  ? "No hay publicaciones que coincidan con estos filtros"
                  : "Aún no hay publicaciones públicas en el catálogo"
            }
            description={
              hasCatalogFilters(query)
                ? "Prueba con otra palabra, ubicación o combinación de filtros."
                : "Vuelve a explorar más tarde para revisar las nuevas publicaciones."
            }
          />
        )}
        {noResults && (query.latitude || query.category_code) && (
          <div className="mt-3 flex flex-wrap justify-center gap-2">
            {query.latitude && radius < 50 && (
              <Link
                href={`${actionPath}?${serializeCatalogQuery({
                  ...query,
                  radius_km: String(expandedRadius),
                  cursor: undefined,
                })}`}
                className="inline-flex min-h-11 items-center rounded-full border px-4 text-sm font-medium"
              >
                Ampliar a {expandedRadius} km
              </Link>
            )}
            {query.category_code && (
              <Link
                href={`${actionPath}?${serializeCatalogQuery({
                  ...query,
                  category_code: undefined,
                  attribute: [],
                  cursor: undefined,
                })}`}
                className="inline-flex min-h-11 items-center rounded-full border px-4 text-sm font-medium"
              >
                Ver todas las categorías
              </Link>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
