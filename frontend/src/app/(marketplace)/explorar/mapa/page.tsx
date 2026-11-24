import type { Metadata } from "next";
import type { RawCatalogParams } from "@/features/public-catalog/domain/types";
import { ExplorePage } from "../_explore-page";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Mapa de arriendos",
  alternates: { canonical: "/explorar" },
  robots: { index: false, follow: true },
};

export default function Page({
  searchParams,
}: {
  searchParams: Promise<RawCatalogParams>;
}) {
  return <ExplorePage searchParams={searchParams} view="map" />;
}
