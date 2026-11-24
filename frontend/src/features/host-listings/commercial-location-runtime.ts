import "client-only";
import { CommercialLocationService } from "./application/commercial-location-service";
import { CommercialLocationOpenApiAdapter } from "./infrastructure/commercial-location-openapi-adapter";
export const commercialLocationService = new CommercialLocationService(
  new CommercialLocationOpenApiAdapter(),
);

export {
  getCommercialCommuneViewport,
  reverseGeocodeCommercialPin,
  searchCommercialAddress,
} from "./infrastructure/commercial-geocoder";
