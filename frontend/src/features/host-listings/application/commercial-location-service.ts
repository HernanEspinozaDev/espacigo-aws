import {
  validateLocation,
  type LocationInput,
  type LocationRead,
} from "../domain/commercial-location";
export interface LocationPort {
  read(spaceId: string, signal?: AbortSignal): Promise<LocationRead>;
  save(
    spaceId: string,
    input: LocationInput,
    etag: string,
    signal?: AbortSignal,
  ): Promise<LocationRead>;
}
export class CommercialLocationService {
  constructor(private readonly port: LocationPort) {}
  read(spaceId: string, signal?: AbortSignal) {
    return this.port.read(spaceId, signal);
  }
  save(
    spaceId: string,
    input: LocationInput,
    etag: string,
    signal?: AbortSignal,
  ) {
    validateLocation(input);
    return this.port.save(spaceId, input, etag, signal);
  }
}
