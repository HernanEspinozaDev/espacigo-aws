export type LocationInput = {
  address: string;
  region_code: string;
  comuna_code: string;
  latitude: number | null;
  longitude: number | null;
  public_confirmed: boolean;
};
export type CommercialLocation = LocationInput & {
  space_id: string;
  confirmed_at: string | null;
  revision: number;
  updated_at: string;
};
export type LocationRead = { location: CommercialLocation; etag: string };
export function validPin(
  latitude: number | null,
  longitude: number | null,
): boolean {
  return (
    latitude !== null &&
    longitude !== null &&
    Number.isFinite(latitude) &&
    Number.isFinite(longitude) &&
    Math.abs(latitude) <= 90 &&
    Math.abs(longitude) <= 180
  );
}
export function validateLocation(input: LocationInput): void {
  if (
    input.public_confirmed &&
    /@|https?:\/\/|www\.|mailto:|tel:|wa\.me\/|t\.me\/|(?:\+?56[ .-]?)?\b[2-9][ .-]?\d{4}[ .-]?\d{4}\b/i.test(
      input.address,
    )
  )
    throw new Error(
      "La dirección pública no puede incluir contactos ni enlaces.",
    );
  if (!input.address.trim() || input.address.length > 500)
    throw new Error("Indica una dirección de hasta 500 caracteres.");
  if ((input.latitude === null) !== (input.longitude === null))
    throw new Error("Completa ambas coordenadas.");
  if (input.latitude !== null && !validPin(input.latitude, input.longitude))
    throw new Error("Las coordenadas no son válidas.");
  if (
    input.public_confirmed &&
    (!validPin(input.latitude, input.longitude) ||
      !input.region_code ||
      !input.comuna_code)
  )
    throw new Error(
      "Confirma dirección, región, comuna y pin antes de guardar.",
    );
}
