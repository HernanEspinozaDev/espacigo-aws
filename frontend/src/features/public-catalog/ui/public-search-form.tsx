import { MapPin, Search, Tag } from "lucide-react";
import type { PublicLocation, PublicSpaceCategory } from "../domain/types";
import { categoryLabel } from "../domain/presentation";

export function PublicSearchForm({
  initialQuery = "",
  actionPath = "/explorar",
  retainedFilters,
  compact = false,
  categories,
  locations,
}: {
  initialQuery?: string;
  actionPath?: string;
  retainedFilters?: URLSearchParams;
  compact?: boolean;
  categories?: readonly PublicSpaceCategory[];
  locations?: readonly PublicLocation[];
}) {
  if (compact) {
    return (
      <form
        action={actionPath}
        method="get"
        role="search"
        aria-label="Buscar publicaciones"
        className="mt-3 flex max-w-2xl items-center gap-2 rounded-full border border-border bg-background p-1 pl-2 shadow-xs sm:mt-5 sm:pl-4"
      >
        {Array.from(retainedFilters ?? [], ([name, value], index) => (
          <input
            key={`${name}-${index}`}
            type="hidden"
            name={name}
            value={value}
          />
        ))}
        <label htmlFor="home-search" className="sr-only">
          ¿Qué quieres arrendar?
        </label>
        <Search
          className="hidden size-5 shrink-0 text-muted-foreground sm:block"
          aria-hidden="true"
        />
        <input
          id="home-search"
          type="search"
          name="q"
          maxLength={120}
          defaultValue={initialQuery}
          placeholder="Casa, oficina, bodega, quincho…"
          className="h-11 min-w-0 flex-1 bg-transparent px-3 text-base text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring sm:px-0"
        />
        <button
          type="submit"
          className="inline-flex min-h-11 min-w-11 items-center justify-center gap-2 rounded-full bg-primary text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:px-5"
        >
          <Search className="size-4 sm:hidden" aria-hidden="true" />
          <span className="sr-only sm:not-sr-only">Buscar</span>
        </button>
      </form>
    );
  }

  const sortedLocations = locations
    ? [...locations].sort((a, b) =>
        a.comuna_name.localeCompare(b.comuna_name, "es"),
      )
    : [];

  return (
    <form
      action={actionPath}
      method="get"
      role="search"
      aria-label="Buscar publicaciones"
      className="mt-7 flex flex-col gap-2.5 rounded-2xl border border-border bg-background p-3 shadow-sm"
    >
      {Array.from(retainedFilters ?? [], ([name, value], index) => (
        <input
          key={`${name}-${index}`}
          type="hidden"
          name={name}
          value={value}
        />
      ))}
      <div className="flex min-w-0 items-center gap-3 rounded-xl bg-[var(--brand-surface-subtle)]/70 px-3.5">
        <label htmlFor="home-search" className="sr-only">
          ¿Qué quieres arrendar?
        </label>
        <Search
          className="size-5 shrink-0 text-[var(--brand-accent)]"
          aria-hidden="true"
        />
        <input
          id="home-search"
          type="search"
          name="q"
          maxLength={120}
          defaultValue={initialQuery}
          placeholder="¿Qué espacio buscas? Oficina, sala, bodega, quincho, casa…"
          className="h-12 w-full min-w-0 flex-1 bg-transparent text-base text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        />
      </div>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_auto]">
        {categories && categories.length > 0 ? (
          <div className="flex min-w-0 items-center gap-2 rounded-xl border border-border/80 bg-background px-3">
            <label htmlFor="home-category" className="sr-only">
              Categoría
            </label>
            <Tag
              className="size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
            <select
              id="home-category"
              name="category_code"
              defaultValue=""
              className="h-11 w-full min-w-0 bg-transparent text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Todas las categorías</option>
              {categories.map((cat) => (
                <option key={cat.code} value={cat.code}>
                  {categoryLabel(cat)}
                </option>
              ))}
            </select>
          </div>
        ) : null}

        {sortedLocations.length > 0 ? (
          <div className="flex min-w-0 items-center gap-2 rounded-xl border border-border/80 bg-background px-3">
            <label htmlFor="home-comuna" className="sr-only">
              Comuna
            </label>
            <MapPin
              className="size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
            <select
              id="home-comuna"
              name="comuna_code"
              defaultValue=""
              className="h-11 w-full min-w-0 bg-transparent text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Toda ubicación</option>
              {sortedLocations.map((loc) => (
                <option key={loc.comuna_code} value={loc.comuna_code}>
                  {loc.comuna_name}
                </option>
              ))}
            </select>
          </div>
        ) : null}

        <button
          type="submit"
          className="inline-flex min-h-11 shrink-0 items-center justify-center gap-2 rounded-xl bg-primary px-7 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Explorar
        </button>
      </div>
    </form>
  );
}
