import type { Metadata } from "next";
import {
  hasCatalogFilters,
  parseCatalogQuery,
} from "@/features/public-catalog/domain/query-state";
import type { RawCatalogParams } from "@/features/public-catalog/domain/types";
import { ExplorePage } from "./_explore-page";

export const dynamic = "force-dynamic";

export async function generateMetadata({
  searchParams,
}: {
  searchParams: Promise<RawCatalogParams>;
}): Promise<Metadata> {
  const query = parseCatalogQuery(await searchParams);
  const filtered = hasCatalogFilters(query);
  return {
    title: query.q ? `Búsqueda: ${query.q}` : "Explorar arriendos",
    description:
      "Explora publicaciones públicas de arriendos residenciales y espacios flexibles en EspaciGo.",
    alternates: { canonical: "/explorar" },
    robots: filtered
      ? { index: false, follow: true }
      : { index: true, follow: true },
  };
}

export default function Page({
  searchParams,
}: {
  searchParams: Promise<RawCatalogParams>;
}) {
  return <ExplorePage searchParams={searchParams} view="grid" />;
}
