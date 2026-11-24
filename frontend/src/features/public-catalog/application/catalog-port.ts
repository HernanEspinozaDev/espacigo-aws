import type {
  PublicMapPage,
  CatalogQuery,
  CatalogResult,
  PublicCategoryFilters,
  PublicLocations,
  PublicSpaceCategories,
  PublicSpaceDetail,
  PublicSpacePage,
  TermsResponse,
} from "../domain/types";

export type NearbyMarkerQuery = CatalogQuery & {
  latitude: string;
  longitude: string;
  exclude_bbox?: string;
  exclude_radius_km?: string;
};

export interface PublicCatalogPort {
  listMapMarkers?(
    query: CatalogQuery,
    signal?: AbortSignal,
  ): Promise<CatalogResult<PublicMapPage>>;
  listNearbyMarkers?(
    query: NearbyMarkerQuery,
    signal?: AbortSignal,
  ): Promise<CatalogResult<PublicMapPage>>;
  listSpaces(query: CatalogQuery): Promise<CatalogResult<PublicSpacePage>>;
  listCategories(): Promise<CatalogResult<PublicSpaceCategories>>;
  getCategoryFilters(
    code: string,
  ): Promise<CatalogResult<PublicCategoryFilters>>;
  listLocations(query?: {
    q?: string;
    region_code?: string;
    limit?: number;
  }): Promise<CatalogResult<PublicLocations>>;
  getSpace(spaceId: string): Promise<CatalogResult<PublicSpaceDetail>>;
  listTerms(): Promise<CatalogResult<TermsResponse>>;
}
