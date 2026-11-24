"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { serializeCatalogQuery } from "../domain/query-state";
import type { CatalogQuery, CatalogSort } from "../domain/types";

const BASE_OPTIONS: { value: CatalogSort; label: string }[] = [
  { value: "default", label: "Orden del catálogo" },
  { value: "title_asc", label: "Nombre A–Z" },
  { value: "title_desc", label: "Nombre Z–A" },
];

export function catalogSortOptions(query: CatalogQuery) {
  return [
    ...BASE_OPTIONS,
    ...(query.latitude !== undefined
      ? [{ value: "distance_asc", label: "Más cercanos" } as const]
      : []),
    ...(query.rate_unit || !query.start_at || !query.end_at
      ? ([
          {
            value: "unit_price_asc",
            label: query.rate_unit ? "Menor tarifa" : "Menor tarifa por hora",
          },
          {
            value: "unit_price_desc",
            label: query.rate_unit ? "Mayor tarifa" : "Mayor tarifa por hora",
          },
        ] as const)
      : []),
    ...(query.q
      ? ([{ value: "relevance", label: "Relevancia" }] as const)
      : []),
    ...(query.start_at && query.end_at
      ? ([
          { value: "estimated_total_asc", label: "Menor total estimado" },
          { value: "estimated_total_desc", label: "Mayor total estimado" },
        ] as const)
      : []),
  ] satisfies readonly { value: CatalogSort; label: string }[];
}

function sortHref(actionPath: string, query: CatalogQuery, sort: CatalogSort) {
  const next = {
    ...query,
    sort,
    ...(sort.startsWith("unit_price_") && !query.rate_unit
      ? { rate_unit: "hora" as const }
      : {}),
    cursor: undefined,
  };
  const params = serializeCatalogQuery(next);
  return params.size ? `${actionPath}?${params.toString()}` : actionPath;
}

export function CatalogSortControls({
  actionPath,
  query,
  compact = false,
}: {
  actionPath: string;
  query: CatalogQuery;
  compact?: boolean;
}) {
  const router = useRouter();
  const options = catalogSortOptions(query);
  const current = options.some((option) => option.value === query.sort)
    ? (query.sort ?? "default")
    : "default";

  return (
    <div className="flex flex-wrap items-center gap-3">
      <span
        className={
          compact
            ? "sr-only"
            : "text-xs uppercase tracking-[0.18em] text-muted-foreground"
        }
      >
        Ordenar
      </span>
      <nav
        aria-label="Ordenar publicaciones"
        className={compact ? "hidden" : "hidden flex-wrap gap-4 sm:flex"}
      >
        {options.map((option) => (
          <Link
            key={option.value}
            href={sortHref(actionPath, query, option.value)}
            aria-current={current === option.value ? "page" : undefined}
            className={`text-sm transition-colors ${current === option.value ? "font-medium text-foreground" : "text-muted-foreground hover:text-foreground"}`}
          >
            {option.label}
          </Link>
        ))}
      </nav>
      <label className={compact ? "block" : "sm:hidden"}>
        <span className="sr-only">Ordenar publicaciones</span>
        <select
          value={current}
          onChange={(event) =>
            router.push(
              sortHref(actionPath, query, event.target.value as CatalogSort),
              {
                scroll: false,
              },
            )
          }
          className={`h-11 border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${compact ? "max-w-36 rounded-full sm:max-w-none" : "rounded-lg"}`}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}
