import Link from "next/link";
import { X } from "lucide-react";
import {
  serializeCatalogQuery,
  withoutCatalogArea,
} from "../domain/query-state";
import type {
  CatalogQuery,
  PublicCategoryFilter,
  PublicLocation,
  PublicSpaceCategory,
} from "../domain/types";

const displayDateTime = new Intl.DateTimeFormat("es-CL", {
  timeZone: "America/Santiago",
  dateStyle: "short",
  timeStyle: "short",
});
const currency = new Intl.NumberFormat("es-CL", {
  style: "currency",
  currency: "CLP",
});
const unitLabels = { hora: "Por hora", dia: "Por día", mes: "Por mes" };

export function ActiveCatalogFilters({
  actionPath,
  query,
  locations,
  categories,
  categoryFilters,
}: {
  actionPath: string;
  query: CatalogQuery;
  locations: readonly PublicLocation[];
  categories: readonly PublicSpaceCategory[];
  categoryFilters: readonly PublicCategoryFilter[];
}) {
  const chips: { key: string; label: string; next: CatalogQuery }[] = [];
  function add(key: string, label: string, update: Partial<CatalogQuery>) {
    chips.push({ key, label, next: { ...query, ...update } });
  }

  if (query.verified_only) {
    add("verified_only", "Solo espacios verificados", {
      verified_only: undefined,
    });
  }
  if (query.category_code) {
    add(
      "category",
      categories.find(({ code }) => code === query.category_code)?.name ??
        query.category_code,
      { category_code: undefined, attribute: [] },
    );
  }
  if (query.areaMode) {
    const location = locations.find(({ comuna_code, region_code }) =>
      query.comuna_code
        ? comuna_code === query.comuna_code
        : region_code === query.region_code,
    );
    const areaLabel = {
      admin:
        (query.comuna_code ? location?.comuna_name : location?.region_name) ??
        "Ubicación",
      radius: `Radio de ${query.radius_km} km`,
      bbox: "Zona del mapa",
    }[query.areaMode];
    chips.push({
      key: "location",
      label: areaLabel,
      next: withoutCatalogArea(query),
    });
  }
  if (query.start_at && query.end_at) {
    add(
      "interval",
      displayDateTime.formatRange(
        new Date(query.start_at),
        new Date(query.end_at),
      ),
      {
        start_at: undefined,
        end_at: undefined,
        min_total_clp: undefined,
        max_total_clp: undefined,
        sort: query.sort?.startsWith("estimated_total_")
          ? undefined
          : query.sort,
      },
    );
  }
  if (query.min_capacity) {
    add("capacity", `Desde ${query.min_capacity} personas`, {
      min_capacity: undefined,
    });
  }
  if (query.min_area_m2) {
    add("surface", `Desde ${query.min_area_m2} m²`, { min_area_m2: undefined });
  }
  if (query.rate_unit) {
    add("unit", unitLabels[query.rate_unit], {
      rate_unit: undefined,
      min_unit_price_clp: undefined,
      max_unit_price_clp: undefined,
      sort: query.sort?.startsWith("unit_price_") ? undefined : query.sort,
    });
  }
  const priceFields = [
    ["min_unit_price_clp", "Tarifa desde"],
    ["max_unit_price_clp", "Tarifa hasta"],
    ["min_total_clp", "Total desde"],
    ["max_total_clp", "Total hasta"],
  ] as const;
  for (const [key, label] of priceFields) {
    const amount = query[key];
    if (amount)
      add(key, `${label} ${currency.format(BigInt(amount))}`, {
        [key]: undefined,
      });
  }
  for (const token of query.attribute) {
    const [code, condition, value] = token.split(":");
    const field = categoryFilters.find((item) => item.code === code);
    if (!field) continue;
    const detail =
      {
        true: "Sí",
        false: "No",
        gte: `desde ${value}`,
        lte: `hasta ${value}`,
      }[condition] ?? value?.replaceAll("_", " ").replaceAll(",", ", ");
    add(token, `${field.label}: ${detail}`, {
      attribute: query.attribute.filter((item) => item !== token),
    });
  }
  if (!chips.length) return null;

  return (
    <ul
      aria-label="Filtros activos"
      className="flex gap-2 overflow-x-auto pb-1 sm:flex-wrap"
    >
      {chips.map(({ key, label, next }) => {
        const params = serializeCatalogQuery({ ...next, cursor: undefined });
        return (
          <li key={key} className="shrink-0">
            <Link
              href={params.size ? `${actionPath}?${params}` : actionPath}
              scroll={false}
              aria-label={`Quitar filtro: ${label}`}
              className="inline-flex min-h-11 items-center gap-1.5 rounded-full bg-muted px-3 text-sm font-medium hover:bg-muted/70 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring"
            >
              {label}
              <X aria-hidden="true" className="size-4" />
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
