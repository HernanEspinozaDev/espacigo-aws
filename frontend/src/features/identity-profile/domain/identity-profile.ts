import type { components } from "@/lib/api/generated/openapi";

export type IdentityProfileState =
  | "incomplete"
  | "ready"
  | "in_progress"
  | "in_review"
  | "needs_correction"
  | "verified"
  | "expired"
  | "revoked";

export type IdentityProfileSummary =
  components["schemas"]["IdentityProfileStatus"];

export interface IdentityProfileInput {
  username: string;
  first_name: string;
  first_last_name: string;
  second_last_name?: string;
  no_second_last_name: boolean;
  rut: string;
  date_of_birth: string;
  document_type: "cl_id";
  phone: string;
  consent: boolean;
}

export type IdentityProfileField = keyof IdentityProfileInput;
export type IdentityProfileErrors = Partial<
  Record<IdentityProfileField, string>
>;

const USERNAME = /^[a-z0-9_][a-z0-9_.-]{2,29}$/i;
const LEGAL_NAME = /^[\p{L}]+(?:[ '\u2019-][\p{L}]+)*$/u;

export function normalizeUsername(value: string): string {
  return value.trim().toLocaleLowerCase("en-US");
}

export function validateUsername(value: string): string | null {
  const normalized = normalizeUsername(value);
  if (!normalized) return "Elige un nombre de usuario.";
  if (!USERNAME.test(normalized)) {
    return "Usa 3 a 30 caracteres: letras, números, guion bajo, punto o guion. El primero debe ser letra, número o guion bajo.";
  }
  return null;
}

export function validateLegalName(value: string, label: string): string | null {
  const normalized = value.trim();
  if (!normalized) return `Ingresa ${label.toLocaleLowerCase("es-CL")}.`;
  if (Array.from(normalized).length > 80 || !LEGAL_NAME.test(normalized)) {
    return "Usa letras, espacios, guiones o apóstrofos; máximo 80 caracteres.";
  }
  return null;
}

export function normalizeChileanRut(value: string): string {
  const compact = value.replace(/[.\s-]/g, "").toLocaleUpperCase("en-US");
  if (!/^[0-9]+[0-9K]$/.test(compact)) return compact;
  return `${compact.slice(0, -1).replace(/\B(?=(\d{3})+(?!\d))/g, ".")}-${compact.slice(-1)}`;
}

export function isValidChileanRut(value: string): boolean {
  const input = value.replace(/\s/g, "").toLocaleUpperCase("en-US");
  if (!/^(?:[0-9]{7,8}-?|[0-9]{1,2}(?:\.[0-9]{3}){2}-?)[0-9K]$/.test(input))
    return false;
  const compact = input.replace(/[.-]/g, "");
  const body = compact.slice(0, -1);
  if (/^0+$/.test(body)) return false;
  const providedCheckDigit = compact.slice(-1);
  let factor = 2;
  let sum = 0;
  for (let index = body.length - 1; index >= 0; index -= 1) {
    sum += Number(body[index]) * factor;
    factor = factor === 7 ? 2 : factor + 1;
  }
  const remainder = 11 - (sum % 11);
  const expected =
    remainder === 11 ? "0" : remainder === 10 ? "K" : String(remainder);
  return expected === providedCheckDigit;
}

function isRealDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day
  );
}

export function normalizeChileanE164(value: string): string | null {
  const compact = value.trim().replace(/[\s().-]/g, "");
  const digits = compact.startsWith("+") ? compact.slice(1) : compact;
  if (/^9\d{8}$/.test(digits)) return `+56${digits}`;
  if (/^569\d{8}$/.test(digits)) return `+${digits}`;
  return null;
}

export function validateIdentityProfile(
  input: IdentityProfileInput,
  today = new Date(),
  rutLocked = false,
): IdentityProfileErrors {
  const errors: IdentityProfileErrors = {};
  const usernameError = validateUsername(input.username);
  if (usernameError) errors.username = usernameError;

  for (const [field, label] of [
    ["first_name", "tu nombre"],
    ["first_last_name", "tu primer apellido"],
  ] as const) {
    const error = validateLegalName(input[field], label);
    if (error) errors[field] = error;
  }

  if (!input.no_second_last_name) {
    const error = validateLegalName(
      input.second_last_name ?? "",
      "tu segundo apellido",
    );
    if (error) errors.second_last_name = error;
  }

  const rut = input.rut.trim();
  if (!rutLocked) {
    if (!rut) errors.rut = "Ingresa tu RUN.";
    else if (!/^[0-9.kK\s-]+$/.test(rut))
      errors.rut = "El RUN solo puede contener números y el dígito K.";
    else if (!isValidChileanRut(rut))
      errors.rut = "Revisa el RUN y su dígito verificador.";
  }

  if (!input.date_of_birth)
    errors.date_of_birth = "Ingresa tu fecha de nacimiento.";
  else if (!isRealDate(input.date_of_birth))
    errors.date_of_birth = "Ingresa una fecha real en formato día, mes y año.";
  else {
    const selected = new Date(`${input.date_of_birth}T00:00:00.000Z`);
    const current = new Date(
      Date.UTC(today.getFullYear(), today.getMonth(), today.getDate()),
    );
    if (selected > current)
      errors.date_of_birth = "La fecha de nacimiento no puede ser futura.";
  }

  const phone = normalizeChileanE164(input.phone);
  if (!phone)
    errors.phone =
      "Ingresa un celular chileno válido con código de país, por ejemplo +56 9 1234 5678.";
  if (!input.consent)
    errors.consent =
      "Debes aceptar el uso de estos datos para verificar tu identidad.";
  return errors;
}

export function formatChileanRutProgressively(value: string): string {
  const upper = value.toLocaleUpperCase("en-US");
  if (!/^[0-9.kK\s-]*$/.test(upper)) return upper;
  const hasExplicitCheckDigit = /-\s*[0-9kK]$/i.test(upper);
  const hasTrailingDash = upper.endsWith("-");
  const compact = upper.replace(/[.\s-]/g, "");
  if (!compact) return "";
  if (compact.length > 9 || (upper.match(/-/g)?.length ?? 0) > 1) return upper;
  const hasCheckDigit = hasExplicitCheckDigit || compact.length > 8;
  const body = (hasCheckDigit ? compact.slice(0, -1) : compact).replace(
    /\D/g,
    "",
  );
  const checkDigit = hasCheckDigit ? compact.slice(-1) : "";
  const grouped = body.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return hasCheckDigit
    ? `${grouped}-${checkDigit}`
    : grouped + (hasTrailingDash ? "-" : "");
}

export function formatChileanRutAtCursor(
  value: string,
  cursor: number,
): { readonly value: string; readonly cursor: number } {
  const formatted = formatChileanRutProgressively(value);
  const meaningfulBeforeCursor = value
    .slice(0, cursor)
    .replace(/[^0-9kK]/g, "").length;
  let seen = 0;
  let nextCursor = formatted.length;
  for (let index = 0; index < formatted.length; index += 1) {
    if (/[0-9kK]/.test(formatted[index])) seen += 1;
    if (seen >= meaningfulBeforeCursor) {
      nextCursor = index + (/[0-9kK]/.test(formatted[index]) ? 1 : 0);
      break;
    }
  }
  return { value: formatted, cursor: nextCursor };
}
