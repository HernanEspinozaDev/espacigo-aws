"use client";

import Link from "next/link";
import { LayoutGrid, Map } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { serializeCatalogQuery } from "../domain/query-state";
import type { CatalogQuery, PublicSpaceCategory } from "../domain/types";
import { readExploreSession, updateExploreSession } from "./explore-session";

export function ExploreViewControls({
  isMap,
  query,
  categories,
  gridHref = "/explorar",
}: {
  isMap: boolean;
  query: CatalogQuery;
  categories: readonly PublicSpaceCategory[];
  gridHref?: string;
}) {
  const router = useRouter();
  const filters = serializeCatalogQuery({
    ...query,
    cursor: undefined,
  }).toString();
  const href = (path: string, suffix = filters) =>
    suffix ? `${path}?${suffix}` : path;

  useEffect(() => {
    if (isMap) return;
    const frame = requestAnimationFrame(() => {
      try {
        const saved = readExploreSession(filters);
        if (saved.scrollY !== undefined)
          window.scrollTo({ top: saved.scrollY, behavior: "instant" });
      } catch {
        // Scroll restoration is optional in browsers that block storage.
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [isMap, filters]);

  function rememberGridPosition() {
    try {
      updateExploreSession(filters, { scrollY: window.scrollY });
    } catch {
      // Native history still restores scroll when storage is unavailable.
    }
  }

  const buttonClass =
    "inline-flex min-h-11 items-center justify-center gap-1.5 rounded-full px-3 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring";
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <nav
        aria-label="Vista de resultados"
        className="inline-flex shrink-0 rounded-full border bg-muted/50 p-0.5"
      >
        {isMap ? (
          <Link
            href={href(gridHref)}
            aria-label="Ver cuadrícula"
            prefetch={false}
            onNavigate={(event) => {
              try {
                const saved = readExploreSession(filters);
                if (saved.scrollY === undefined) return;
                event.preventDefault();
                router.push(href(gridHref));
              } catch {
                // Use the regular view link when the saved page is unavailable.
              }
            }}
            className={buttonClass}
          >
            <LayoutGrid className="size-4" aria-hidden="true" />
            Espacios
          </Link>
        ) : (
          <span
            aria-current="page"
            className={`${buttonClass} bg-primary text-primary-foreground shadow-xs`}
          >
            <LayoutGrid className="size-4" aria-hidden="true" />
            Espacios
          </span>
        )}
        {isMap ? (
          <span
            aria-current="page"
            className={`${buttonClass} bg-primary text-primary-foreground shadow-xs`}
          >
            <Map className="size-4" aria-hidden="true" />
            Mapa
          </span>
        ) : (
          <Link
            href={href("/explorar/mapa")}
            aria-label="Ver mapa"
            prefetch={false}
            onClick={rememberGridPosition}
            className={buttonClass}
          >
            <Map className="size-4" aria-hidden="true" />
            Mapa
          </Link>
        )}
      </nav>
      {!!categories.length && (
        <select
          aria-label="Categoría de espacios"
          className="min-h-11 max-w-full min-w-0 flex-1 truncate rounded-full border bg-background px-3 text-sm sm:flex-none"
          value={query.category_code ?? ""}
          onChange={(event) => {
            const next = serializeCatalogQuery({
              ...query,
              category_code: event.target.value || undefined,
              attribute: [],
              cursor: undefined,
            });
            router.push(
              href(isMap ? "/explorar/mapa" : "/explorar", next.toString()),
              {
                scroll: false,
              },
            );
          }}
        >
          <option value="">Todas las categorías</option>
          {categories.map((category) => (
            <option key={category.code} value={category.code}>
              {category.name}
            </option>
          ))}
        </select>
      )}
    </div>
  );
}
