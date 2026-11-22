import { parseCatalogQuery } from "../domain/query-state";
import type {
  PublicMapMarker,
  CatalogQuery,
  CatalogResult,
  PublicCategoryFilter,
  PublicCategoryFilters,
  PublicLocations,
  PublicSpaceCategories,
  PublicSpaceDetail,
  PublicSpacePage,
  RawCatalogParams,
  TermsResponse,
} from "../domain/types";
import type { PublicCatalogPort } from "./catalog-port";

export function createPublicCatalogService(port: PublicCatalogPort) {
  return {
    async mapMarkers(
      query: CatalogQuery,
    ): Promise<CatalogResult<PublicMapMarker[]>> {
      if (!port.listMapMarkers)
        return { ok: false, error: { kind: "unavailable" } };
      const items: PublicMapMarker[] = [];
      const seen = new Set<string>();
      let cursor: string | undefined;
      do {
        const result = await port.listMapMarkers({
          ...query,
          page_size: 48,
          cursor,
        });
        if (!result.ok) return result;
        if (!Array.isArray(result.value.items))
          return { ok: false, error: { kind: "unavailable" } };
        items.push(...result.value.items);
        cursor = result.value.next_cursor;
        if (cursor) {
          if (seen.has(cursor))
            return { ok: false, error: { kind: "unavailable" } };
          seen.add(cursor);
        }
      } while (cursor);
      return { ok: true, value: items };
    },
    async categories() {
      return port.listCategories();
    },
    async locations(query?: {
      q?: string;
      region_code?: string;
      limit?: number;
    }) {
      return port.listLocations(query);
    },
    async categoryFilters(code: string) {
      return port.getCategoryFilters(code);
    },
    async spaces(query: CatalogQuery) {
      return port.listSpaces(query);
    },
    async spacesFromParams(
      params: RawCatalogParams,
      filters: ReadonlyArray<PublicCategoryFilter> = [],
    ) {
      const query = parseCatalogQuery(params, filters);
      const result = await port.listSpaces(query);
      return { query, result };
    },
    async space(spaceId: string) {
      return port.getSpace(spaceId);
    },
    async terms() {
      return port.listTerms();
    },
  } satisfies {
    mapMarkers(query: CatalogQuery): Promise<CatalogResult<PublicMapMarker[]>>;
    categories(): Promise<CatalogResult<PublicSpaceCategories>>;
    locations(query?: {
      q?: string;
      region_code?: string;
      limit?: number;
    }): Promise<CatalogResult<PublicLocations>>;
    categoryFilters(
      code: string,
    ): Promise<CatalogResult<PublicCategoryFilters>>;
    spaces(query: CatalogQuery): Promise<CatalogResult<PublicSpacePage>>;
    spacesFromParams(
      params: RawCatalogParams,
      filters?: ReadonlyArray<PublicCategoryFilter>,
    ): Promise<{
      query: CatalogQuery;
      result: CatalogResult<PublicSpacePage>;
    }>;
    space(spaceId: string): Promise<CatalogResult<PublicSpaceDetail>>;
    terms(): Promise<CatalogResult<TermsResponse>>;
  };
}
