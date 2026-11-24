import Link from "next/link";
import type { CatalogQuery, PublicSpacePage } from "../domain/types";
import { serializeCatalogQuery } from "../domain/query-state";

export function NextCatalogPage({
  query,
  nextCursor,
  href = "/explorar",
}: {
  query: CatalogQuery;
  nextCursor?: PublicSpacePage["next_cursor"];
  href?: string;
}) {
  if (!nextCursor) return null;
  const params = serializeCatalogQuery({ ...query, cursor: nextCursor });
  return (
    <nav
      aria-label="Paginación del catálogo"
      className="mt-10 flex justify-center px-1 sm:mt-12"
    >
      <Link
        href={`${href}?${params.toString()}`}
        className="inline-flex min-h-12 w-full items-center justify-center rounded-full border border-border px-6 text-sm font-medium transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-auto"
      >
        Ver más publicaciones
      </Link>
    </nav>
  );
}
