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
import type { NearbyMarkerQuery, PublicCatalogPort } from "./catalog-port";

const mapMarkerLimit = 250;

type MapResults = {
  items: PublicMapMarker[];
  truncated: boolean;
  has_more: boolean;
  total_count: number;
};

export function createPublicCatalogService(port: PublicCatalogPort) {
  return {
    async mapMarkers(
      query: CatalogQuery,
      signal?: AbortSignal,
    ): Promise<CatalogResult<MapResults>> {
      if (!port.listMapMarkers)
        return { ok: false, error: { kind: "unavailable" } };
      const result = await port.listMapMarkers(
        {
          ...query,
          page_size: mapMarkerLimit,
          cursor: undefined,
        },
        signal,
      );
      if (!result.ok) return result;
      if (!Array.isArray(result.value.items))
        return { ok: false, error: { kind: "unavailable" } };
      const items = result.value.items.slice(0, mapMarkerLimit);
      const hasMore = Boolean(
        result.value.has_more ?? result.value.next_cursor,
      );
      const totalCount =
        typeof result.value.total_count === "number"
          ? result.value.total_count
          : items.length;
      return {
        ok: true,
        value: {
          items,
          truncated: hasMore,
          has_more: hasMore,
          total_count: totalCount,
        },
      };
    },
    async nearbyMarkers(
      query: NearbyMarkerQuery,
      signal?: AbortSignal,
    ): Promise<CatalogResult<MapResults>> {
      if (!port.listNearbyMarkers)
        return { ok: false, error: { kind: "unavailable" } };
      const result = await port.listNearbyMarkers(query, signal);
      if (!result.ok) return result;
      if (!Array.isArray(result.value.items))
        return { ok: false, error: { kind: "unavailable" } };
      return {
        ok: true,
        value: {
          items: result.value.items.slice(0, 6),
          truncated: false,
          has_more: false,
          total_count: result.value.items.length,
        },
      };
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
    mapMarkers(
      query: CatalogQuery,
      signal?: AbortSignal,
    ): Promise<CatalogResult<MapResults>>;
    nearbyMarkers(
      query: NearbyMarkerQuery,
      signal?: AbortSignal,
    ): Promise<CatalogResult<MapResults>>;
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
