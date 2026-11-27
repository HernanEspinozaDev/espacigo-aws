"use client";

import { ShieldCheck, SlidersHorizontal } from "lucide-react";
import { useRouter } from "next/navigation";
import type { FormEvent } from "react";
import { useState } from "react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { categoryLabel } from "../domain/presentation";
import {
  resolveSantiagoLocalDateTime,
  santiagoLocalInputValue,
} from "../domain/santiago-datetime";
import type {
  CatalogQuery,
  PublicCategoryFilter,
  PublicLocation,
  PublicSpaceCategory,
} from "../domain/types";

const scalarKeys = [
  "areaMode",
  "bbox",
  "latitude",
  "longitude",
  "radius_km",
  "q",
  "category_code",
  "region_code",
  "comuna_code",
  "min_area_m2",
  "min_capacity",
  "rate_unit",
  "min_unit_price_clp",
  "max_unit_price_clp",
  "min_total_clp",
  "max_total_clp",
  "sort",
  "page_size",
] as const;

function currentAttribute(query: CatalogQuery, code: string) {
  return query.attribute.filter((token) => token.startsWith(`${code}:`));
}

function numericValue(tokens: string[], operator: "gte" | "lte") {
  const prefix = `:${operator}:`;
  return (
    tokens.find((token) => token.includes(prefix))?.split(prefix, 2)[1] ?? ""
  );
}

function SantiagoDateTimeInput({
  name,
  label,
  original,
}: {
  name: "start_local" | "end_local";
  label: string;
  original?: string;
}) {
  const defaultValue = santiagoLocalInputValue(original);
  const [value, setValue] = useState(defaultValue);
  const resolution = value ? resolveSantiagoLocalDateTime(value) : undefined;
  const overlapName = name === "start_local" ? "start_overlap" : "end_overlap";
  return (
    <label className="block space-y-1 text-xs text-muted-foreground">
      <span>{label}</span>
      <input
        name={name}
        type="datetime-local"
        step={
          value === defaultValue && Number(value.slice(-2)) % 30
            ? "any"
            : "1800"
        }
        value={value}
        onChange={(event) => setValue(event.target.value)}
        className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      {resolution?.kind === "ambiguous" ? (
        <span className="block space-y-1 pt-2 text-foreground">
          <span>
            Esta hora ocurre dos veces por el cambio de horario. Elige cuál
            usar:
          </span>
          <select
            name={overlapName}
            defaultValue={
              original &&
              Date.parse(original) >= Date.parse(resolution.later) &&
              value === defaultValue
                ? "later"
                : "earlier"
            }
            className="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm"
          >
            <option value="earlier">
              Primera ocurrencia ({resolution.earlier.slice(-6)})
            </option>
            <option value="later">
              Segunda ocurrencia ({resolution.later.slice(-6)})
            </option>
          </select>
        </span>
      ) : null}
    </label>
  );
}

function buildSubmission(
  form: HTMLFormElement,
  filters: readonly PublicCategoryFilter[],
) {
  const data = new FormData(form);
  const params = new URLSearchParams();
  for (const key of scalarKeys) {
    const value = data.get(key);
    if (typeof value === "string" && value !== "") params.set(key, value);
  }
  if (data.get("verified_only") === "true") {
    params.set("verified_only", "true");
  }

  const startLocal = data.get("start_local");
  const endLocal = data.get("end_local");
  if (typeof startLocal === "string" && typeof endLocal === "string") {
    if (Boolean(startLocal) !== Boolean(endLocal))
      throw new Error("Indica fecha y hora de inicio y término.");
    if (startLocal && endLocal) {
      const resolved: string[] = [];
      for (const [local, original, overlap, key] of [
        [
          startLocal,
          data.get("start_original"),
          data.get("start_overlap"),
          "start_at",
        ],
        [endLocal, data.get("end_original"), data.get("end_overlap"), "end_at"],
      ] as const) {
        const resolution = resolveSantiagoLocalDateTime(local);
        const chosenTimestamp =
          resolution.kind === "ambiguous"
            ? overlap === "later"
              ? resolution.later
              : resolution.earlier
            : undefined;
        if (
          typeof original === "string" &&
          original &&
          santiagoLocalInputValue(original) === local &&
          (!chosenTimestamp ||
            Math.abs(Date.parse(original) - Date.parse(chosenTimestamp)) <
              60000)
        ) {
          params.set(key, original);
          resolved.push(original);
          continue;
        }
        if (Number(local.slice(-2)) % 30 !== 0)
          throw new Error("Selecciona intervalos de 30 minutos.");
        if (resolution.kind === "nonexistent")
          throw new Error(
            `La hora ${local.replace("T", " ")} no existe en Santiago por el cambio de horario.`,
          );
        if (resolution.kind === "invalid")
          throw new Error("Revisa la fecha y hora ingresadas.");
        const timestamp =
          resolution.kind === "unique"
            ? resolution.value
            : overlap === "later"
              ? resolution.later
              : resolution.earlier;
        params.set(key, timestamp);
        resolved.push(timestamp);
      }
      if (Date.parse(resolved[1]!) <= Date.parse(resolved[0]!))
        throw new Error("El término debe ser posterior al inicio.");
    }
  }

  for (const filter of filters) {
    const field = `attribute-${filter.code}`;
    if (filter.type === "integer" || filter.type === "number") {
      const minimum = data.get(`${field}-min`);
      const maximum = data.get(`${field}-max`);
      if (typeof minimum === "string" && minimum !== "")
        params.append("attribute", `${filter.code}:gte:${minimum}`);
      if (typeof maximum === "string" && maximum !== "")
        params.append("attribute", `${filter.code}:lte:${maximum}`);
    } else if (filter.type === "enum_list") {
      const values = data
        .getAll(field)
        .filter(
          (value): value is string => typeof value === "string" && value !== "",
        );
      if (values.length)
        params.append("attribute", `${filter.code}:in:${values.join(",")}`);
    } else {
      const value = data.get(field);
      if (typeof value === "string" && value !== "")
        params.append("attribute", value);
    }
  }
  return params;
}

function FilterFields({
  categories,
  locations,
  filters,
  query,
  fixedCategoryCode,
  hideAdministrativeLocation,
}: {
  categories: readonly PublicSpaceCategory[];
  locations: readonly PublicLocation[];
  filters: readonly PublicCategoryFilter[];
  query: CatalogQuery;
  fixedCategoryCode?: string;
  hideAdministrativeLocation?: boolean;
}) {
  const regions = Array.from(
    new Map(
      locations.map((item) => [item.region_code, item.region_name]),
    ).entries(),
  );
  const selectedRegion = query.region_code ?? "";

  return (
    <>
      {query.q ? <input type="hidden" name="q" value={query.q} /> : null}
      <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border bg-[var(--brand-surface-subtle)]/60 p-3.5 text-sm transition-colors hover:border-emerald-700/35">
        <input
          key={query.verified_only ? "verified-on" : "verified-off"}
          type="checkbox"
          name="verified_only"
          value="true"
          defaultChecked={Boolean(query.verified_only)}
          className="mt-0.5 size-4 shrink-0 accent-emerald-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
        <span className="space-y-1">
          <span className="inline-flex items-center gap-1.5 font-medium text-foreground">
            <ShieldCheck
              className="size-4 text-[var(--brand-success)]"
              aria-hidden="true"
            />
            Solo espacios verificados
          </span>
          <span className="block text-xs leading-relaxed text-muted-foreground">
            Mostrar únicamente espacios con titularidad y documentación
            aprobada.
          </span>
        </span>
      </label>
      {fixedCategoryCode ? (
        <input type="hidden" name="category_code" value={fixedCategoryCode} />
      ) : (
        <label className="block space-y-2 text-sm">
          <span className="font-medium">Categoría</span>
          <select
            name="category_code"
            defaultValue={query.category_code ?? ""}
            className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">Todas las categorías</option>
            {categories.map((category) => (
              <option key={category.code} value={category.code}>
                {categoryLabel(category)}
              </option>
            ))}
          </select>
        </label>
      )}

      {hideAdministrativeLocation ? (
        <>
          {query.region_code && (
            <input type="hidden" name="region_code" value={query.region_code} />
          )}
          {query.comuna_code && (
            <input type="hidden" name="comuna_code" value={query.comuna_code} />
          )}
        </>
      ) : (
        <>
          <label className="block space-y-2 text-sm">
            <span className="font-medium">Región</span>
            <select
              name="region_code"
              defaultValue={selectedRegion}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Todas las regiones</option>
              {regions.map(([code, name]) => (
                <option key={code} value={code}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label className="block space-y-2 text-sm">
            <span className="font-medium">Comuna</span>
            <select
              name="comuna_code"
              defaultValue={query.comuna_code ?? ""}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Todas las comunas</option>
              {locations
                .filter(
                  (location) =>
                    !selectedRegion || location.region_code === selectedRegion,
                )
                .map((location) => (
                  <option
                    key={location.comuna_code}
                    value={location.comuna_code}
                  >
                    {location.comuna_name} · {location.region_name}
                  </option>
                ))}
            </select>
          </label>
        </>
      )}

      <label className="block space-y-2 text-sm">
        <span className="font-medium">Superficie mínima (m²)</span>
        <input
          name="min_area_m2"
          type="number"
          min="0.01"
          step="0.01"
          defaultValue={query.min_area_m2 ?? ""}
          className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      <label className="block space-y-2 text-sm">
        <span className="font-medium">Capacidad mínima</span>
        <input
          name="min_capacity"
          type="number"
          min="1"
          max="2147483647"
          step="1"
          defaultValue={query.min_capacity ?? ""}
          className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>

      <label className="block space-y-2 text-sm">
        <span className="font-medium">Modalidad de tarifa</span>
        <select
          name="rate_unit"
          defaultValue={query.rate_unit ?? ""}
          className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">Cualquiera</option>
          <option value="hora">Por hora</option>
          <option value="dia">Por día</option>
          <option value="mes">Por mes</option>
        </select>
      </label>

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">Tarifa base (CLP)</legend>
        <div className="grid grid-cols-2 gap-2">
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Mínimo</span>
            <input
              name="min_unit_price_clp"
              inputMode="numeric"
              pattern="[0-9]{1,19}"
              defaultValue={query.min_unit_price_clp ?? ""}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Máximo</span>
            <input
              name="max_unit_price_clp"
              inputMode="numeric"
              pattern="[0-9]{1,19}"
              defaultValue={query.max_unit_price_clp ?? ""}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
        </div>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">
          Intervalo para estimación
        </legend>
        <p className="text-xs leading-relaxed text-muted-foreground">
          Selecciona el horario de Santiago en bloques de 30 minutos.
        </p>
        <SantiagoDateTimeInput
          name="start_local"
          label="Inicio"
          original={query.start_at}
        />
        <SantiagoDateTimeInput
          name="end_local"
          label="Término"
          original={query.end_at}
        />
        <div className="grid grid-cols-2 gap-2">
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Total mínimo (CLP)</span>
            <input
              name="min_total_clp"
              inputMode="numeric"
              pattern="[0-9]{1,19}"
              defaultValue={query.min_total_clp ?? ""}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Total máximo (CLP)</span>
            <input
              name="max_total_clp"
              inputMode="numeric"
              pattern="[0-9]{1,19}"
              defaultValue={query.max_total_clp ?? ""}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
        </div>
      </fieldset>

      {(fixedCategoryCode ?? query.category_code) && filters.length > 0 ? (
        <fieldset className="space-y-4 border-t border-border pt-4">
          <legend className="text-sm font-medium">
            Características de la categoría
          </legend>
          {filters.map((filter) => {
            const tokens = currentAttribute(query, filter.code);
            const field = `attribute-${filter.code}`;
            if (filter.type === "boolean") {
              const selected = tokens[0]?.slice(filter.code.length + 1) ?? "";
              return (
                <label key={filter.code} className="block space-y-2 text-sm">
                  <span>{filter.label}</span>
                  <select
                    name={field}
                    defaultValue={selected ? `${filter.code}:${selected}` : ""}
                    className="h-11 w-full rounded-lg border border-border bg-background px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">Cualquiera</option>
                    <option value={`${filter.code}:true`}>Sí</option>
                    <option value={`${filter.code}:false`}>No</option>
                  </select>
                </label>
              );
            }
            if (filter.type === "enum") {
              const selected = tokens[0]?.split(":eq:", 2)[1] ?? "";
              return (
                <label key={filter.code} className="block space-y-2 text-sm">
                  <span>{filter.label}</span>
                  <select
                    name={field}
                    defaultValue={
                      selected ? `${filter.code}:eq:${selected}` : ""
                    }
                    className="h-11 w-full rounded-lg border border-border bg-background px-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">Cualquiera</option>
                    {(filter.options ?? []).map((option) => (
                      <option
                        key={option}
                        value={`${filter.code}:eq:${option}`}
                      >
                        {option}
                      </option>
                    ))}
                  </select>
                </label>
              );
            }
            if (filter.type === "enum_list") {
              const selected = new Set(
                tokens[0]?.split(":in:", 2)[1]?.split(",") ?? [],
              );
              return (
                <fieldset key={filter.code} className="space-y-2">
                  <legend className="text-sm font-medium">
                    {filter.label}
                  </legend>
                  {(filter.options ?? []).map((option) => (
                    <label
                      key={option}
                      className="flex min-h-10 items-center gap-3 text-sm text-muted-foreground"
                    >
                      <input
                        type="checkbox"
                        name={field}
                        value={option}
                        defaultChecked={selected.has(option)}
                        className="size-4 accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      />
                      {option}
                    </label>
                  ))}
                </fieldset>
              );
            }
            const min = numericValue(tokens, "gte");
            const max = numericValue(tokens, "lte");
            return (
              <fieldset key={filter.code} className="space-y-2">
                <legend className="text-sm font-medium">
                  {filter.label}
                  {filter.unit ? ` (${filter.unit})` : ""}
                </legend>
                <div className="grid grid-cols-2 gap-2">
                  <label className="space-y-1 text-xs text-muted-foreground">
                    <span>Mínimo</span>
                    <input
                      name={`${field}-min`}
                      type="number"
                      min={filter.minimum}
                      max={filter.maximum}
                      step={
                        filter.type === "integer" ? 1 : (filter.step ?? "any")
                      }
                      defaultValue={min}
                      className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    />
                  </label>
                  <label className="space-y-1 text-xs text-muted-foreground">
                    <span>Máximo</span>
                    <input
                      name={`${field}-max`}
                      type="number"
                      min={filter.minimum}
                      max={filter.maximum}
                      step={
                        filter.type === "integer" ? 1 : (filter.step ?? "any")
                      }
                      defaultValue={max}
                      className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    />
                  </label>
                </div>
              </fieldset>
            );
          })}
        </fieldset>
      ) : null}
    </>
  );
}

function FilterForm({
  actionPath,
  categories,
  locations,
  filters,
  query,
  fixedCategoryCode,
  hideAdministrativeLocation,
  onComplete,
}: {
  actionPath: string;
  categories: readonly PublicSpaceCategory[];
  locations: readonly PublicLocation[];
  filters: readonly PublicCategoryFilter[];
  query: CatalogQuery;
  fixedCategoryCode?: string;
  hideAdministrativeLocation?: boolean;
  onComplete?: () => void;
}) {
  const router = useRouter();
  const [submitError, setSubmitError] = useState("");
  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    let params: URLSearchParams;
    try {
      params = buildSubmission(event.currentTarget, filters);
      if (params.has("region_code") || params.has("comuna_code")) {
        params.set("areaMode", "admin");
        for (const key of ["bbox", "latitude", "longitude", "radius_km"])
          params.delete(key);
        if (params.get("sort") === "distance_asc") params.delete("sort");
      } else if (params.get("areaMode") === "admin") {
        params.delete("areaMode");
      }
      if (
        !params.has("rate_unit") &&
        params.get("sort")?.startsWith("unit_price_")
      )
        params.delete("sort");
      setSubmitError("");
    } catch (error) {
      setSubmitError(
        error instanceof Error
          ? error.message
          : "Revisa el intervalo ingresado.",
      );
      return;
    }
    const suffix = params.toString();
    router.push(suffix ? `${actionPath}?${suffix}` : actionPath, {
      scroll: false,
    });
    onComplete?.();
  };

  return (
    <form
      action={actionPath}
      method="get"
      onSubmit={onSubmit}
      className="space-y-5"
    >
      {(["bbox", "latitude", "longitude", "radius_km"] as const).map((key) =>
        query[key] !== undefined ? (
          <input key={key} type="hidden" name={key} value={query[key]} />
        ) : null,
      )}
      {query.sort ? (
        <input type="hidden" name="sort" value={query.sort} />
      ) : null}
      {query.page_size ? (
        <input type="hidden" name="page_size" value={query.page_size} />
      ) : null}
      {query.start_at ? (
        <input type="hidden" name="start_original" value={query.start_at} />
      ) : null}
      {query.end_at ? (
        <input type="hidden" name="end_original" value={query.end_at} />
      ) : null}
      {query.areaMode ? (
        <input type="hidden" name="areaMode" value={query.areaMode} />
      ) : null}
      <FilterFields
        categories={categories}
        locations={locations}
        filters={filters}
        query={query}
        fixedCategoryCode={fixedCategoryCode}
        hideAdministrativeLocation={hideAdministrativeLocation}
      />
      {submitError ? (
        <p role="alert" className="text-sm text-destructive">
          {submitError}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2 border-t border-border pt-4">
        <button
          type="submit"
          className="inline-flex min-h-11 items-center rounded-full bg-foreground px-5 text-sm font-medium text-background hover:bg-foreground/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Aplicar filtros
        </button>
        <a
          href={
            fixedCategoryCode
              ? `${actionPath}?category_code=${encodeURIComponent(fixedCategoryCode)}`
              : actionPath
          }
          className="inline-flex min-h-11 items-center rounded-full border border-border px-4 text-sm font-medium hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Limpiar
        </a>
      </div>
    </form>
  );
}

export function CatalogFilters(props: {
  actionPath: string;
  categories: readonly PublicSpaceCategory[];
  locations: readonly PublicLocation[];
  filters: readonly PublicCategoryFilter[];
  query: CatalogQuery;
  fixedCategoryCode?: string;
  compact?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {!props.compact && (
        <aside className="hidden lg:block">
          <div className="sticky top-24 max-h-[calc(100vh-7rem)] overflow-y-auto overscroll-contain rounded-2xl border border-border p-5 [scrollbar-gutter:stable]">
            <h2 className="mb-5 text-sm font-medium">Filtros</h2>
            <FilterForm {...props} />
          </div>
        </aside>
      )}
      <div className={props.compact ? undefined : "lg:hidden"}>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetTrigger className="inline-flex min-h-11 items-center gap-2 rounded-full border border-border px-4 text-sm font-medium hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <SlidersHorizontal className="size-4" aria-hidden="true" />
            Filtrar
          </SheetTrigger>
          <SheetContent
            side="bottom"
            className="max-h-[85vh] overflow-y-auto rounded-t-2xl p-5"
          >
            <SheetHeader className="mb-5 text-left">
              <SheetTitle className="text-2xl font-medium">
                Refinar publicaciones
              </SheetTitle>
              <SheetDescription>
                Ajusta el precio, la disponibilidad y las características del
                espacio.
              </SheetDescription>
            </SheetHeader>
            <FilterForm
              {...props}
              hideAdministrativeLocation={props.compact}
              onComplete={() => setOpen(false)}
            />
          </SheetContent>
        </Sheet>
      </div>
    </>
  );
}
