export type LocationInput = {
  address: string;
  region_code: string;
  comuna_code: string;
  latitude: number | null;
  longitude: number | null;
  public_confirmed: boolean;
  source?: "manual" | "geocoded";
  precision?: "address" | "approximate";
};

const CHILEAN_POSTAL_CODE_TOKEN = /(^|[\s,])\d{7}($|[\s,])/g;

/**
 * Removes standalone Chilean 7-digit postal codes (e.g. "8320000") from an
 * address without touching 1-5 digit street numbers.
 */
export function stripPostalCodeFromAddress(address: string): string {
  let s = address.trim();
  if (!s) return "";
  while (true) {
    const next = s.replace(CHILEAN_POSTAL_CODE_TOKEN, "$1$2");
    if (next === s) break;
    s = next;
  }
  return s
    .replace(/\s*,\s*(?:,\s*)+/g, ", ")
    .replace(/\s+,/g, ",")
    .replace(/\s+/g, " ")
    .replace(/^[\s,]+|[\s,]+$/g, "");
}
export type CommercialLocation = LocationInput & {
  space_id: string;
  confirmed_at: string | null;
  revision: number;
  updated_at: string;
};
export type PinResolutionStatus =
  "idle" | "resolving" | "resolved" | "invalid" | "unavailable";

export type PinResolution = {
  revision: number;
  status: PinResolutionStatus;
  message?: string;
};

export function shouldApplyPinAddressSuggestion(input: {
  distanceFromAddressAnchorMeters: number;
  editedDuringResolution: boolean;
  verifyingExisting: boolean;
}): boolean {
  return (
    !input.verifyingExisting &&
    !input.editedDuringResolution &&
    input.distanceFromAddressAnchorMeters > 30
  );
}
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
  if (!input.address.trim() || Array.from(input.address.trim()).length > 500)
    throw new Error("Indica una dirección de hasta 500 caracteres.");
  if (!validPin(input.latitude, input.longitude))
    throw new Error("Selecciona un pin válido en el mapa.");
  if (
    !/^\d{2}$/.test(input.region_code) ||
    !/^\d{5}$/.test(input.comuna_code) ||
    !input.comuna_code.startsWith(input.region_code)
  )
    throw new Error(
      "Espera a que el mapa detecte la región y la comuna antes de guardar.",
    );
}
