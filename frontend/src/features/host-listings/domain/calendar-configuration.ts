import type { components } from "@/lib/api/generated/openapi";

export type AvailabilityRule = components["schemas"]["OwnCalendarRule"] & {
  disponible_desde?: string | null;
  estadia_minima_meses?: number;
};
export type CalendarOverrideInput =
  components["schemas"]["CalendarInventoryOverride"];
export type CalendarInventoryUpdate =
  components["schemas"]["CalendarInventoryUpdate"] & {
    disponible_desde?: string;
    estadia_minima_meses?: number;
  };
export interface CalendarConfiguration {
  space_id: string;
  time_zone: string;
  rule: AvailabilityRule;
  overrides: CalendarOverrideInput[];
  updated_at: string;
}
export interface CalendarConfigurationRead {
  configuration: CalendarConfiguration;
  etag: string;
}
export interface CalendarConfigurationAck {
  etag: string;
}

const own = (value: object, keys: readonly string[]) => {
  const actual = Object.keys(value);
  return (
    actual.length === keys.length &&
    keys.every((key) => Object.hasOwn(value, key))
  );
};
const record = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);
const strongEtag = (value: unknown): value is string =>
  typeof value === "string" && /^"[\x21\x23-\x7e\x80-\xff]+"$/.test(value);
function validZone(value: unknown): value is string {
  if (typeof value !== "string" || !value) return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}
function positiveInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) > 0;
}
function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value))
    return false;
  const date = new Date(`${value}T00:00:00.000Z`);
  return (
    Number.isFinite(date.valueOf()) && date.toISOString().slice(0, 10) === value
  );
}
function validOverrides(value: unknown): value is CalendarOverrideInput[] {
  if (!Array.isArray(value)) return false;
  const dates = new Set<string>();
  return value.every((entry) => {
    if (
      !record(entry) ||
      !["disponible", "bloqueado", "mantenimiento"].includes(
        String(entry.estado),
      ) ||
      !validDate(entry.fecha) ||
      dates.has(entry.fecha)
    )
      return false;
    dates.add(entry.fecha);
    const allowed = ["fecha", "estado", "tarifa_personalizada_clp", "motivo"];
    if (Object.keys(entry).some((key) => !allowed.includes(key))) return false;
    if (
      entry.tarifa_personalizada_clp !== undefined &&
      !positiveInt(entry.tarifa_personalizada_clp)
    )
      return false;
    return entry.motivo === undefined || typeof entry.motivo === "string";
  });
}
function validRule(value: unknown, spaceId: string): value is AvailabilityRule {
  if (!record(value)) return false;
  const requiredKeys = [
    "space_id",
    "dias_operacion",
    "hora_apertura",
    "hora_cierre",
    "check_in_hora",
    "check_out_hora",
    "tarifa_base_dia",
    "estadia_minima",
    "estadia_maxima",
    "actualizado_en",
  ];
  const allowedKeys = new Set([
    ...requiredKeys,
    "disponible_desde",
    "estadia_minima_meses",
  ]);
  if (
    !requiredKeys.every((key) => Object.hasOwn(value, key)) ||
    Object.keys(value).some((key) => !allowedKeys.has(key))
  )
    return false;
  const days = value.dias_operacion;
  return (
    value.space_id === spaceId &&
    Array.isArray(days) &&
    days.length > 0 &&
    days.every(
      (day) => Number.isInteger(day) && Number(day) >= 1 && Number(day) <= 7,
    ) &&
    new Set(days).size === days.length &&
    ["hora_apertura", "hora_cierre", "check_in_hora", "check_out_hora"].every(
      (key) =>
        typeof value[key] === "string" &&
        (/^([01]\d|2[0-3]):[0-5]\d$/.test(value[key]) ||
          (key === "hora_cierre" && value[key] === "24:00")),
    ) &&
    positiveInt(value.tarifa_base_dia) &&
    positiveInt(value.estadia_minima) &&
    positiveInt(value.estadia_maxima) &&
    value.estadia_minima <= value.estadia_maxima &&
    typeof value.actualizado_en === "string" &&
    Number.isFinite(Date.parse(value.actualizado_en))
  );
}
export function validateCalendarConfiguration(
  value: unknown,
  expectedSpaceId: string,
): CalendarConfiguration {
  if (
    !record(value) ||
    !own(value, ["space_id", "time_zone", "rule", "overrides", "updated_at"]) ||
    value.space_id !== expectedSpaceId ||
    !validZone(value.time_zone) ||
    !validRule(value.rule, expectedSpaceId) ||
    !validOverrides(value.overrides) ||
    typeof value.updated_at !== "string" ||
    !Number.isFinite(Date.parse(value.updated_at))
  )
    throw new TypeError("Invalid calendar configuration response");
  return value as unknown as CalendarConfiguration;
}
export function validateCalendarUpdate(
  input: CalendarInventoryUpdate,
): CalendarInventoryUpdate {
  if (
    !record(input) ||
    !Object.values(input).some((value) => value !== undefined) ||
    Object.keys(input).some(
      (key) =>
        ![
          "time_zone",
          "dias_operacion",
          "hora_apertura",
          "hora_cierre",
          "check_in_hora",
          "check_out_hora",
          "tarifa_base_dia",
          "estadia_minima",
          "estadia_maxima",
          "disponible_desde",
          "estadia_minima_meses",
          "overrides",
        ].includes(key),
    )
  )
    throw new TypeError("Invalid calendar update");
  if (input.time_zone !== undefined && !validZone(input.time_zone))
    throw new TypeError("Invalid calendar timezone");
  if (
    input.disponible_desde !== undefined &&
    input.disponible_desde !== "" &&
    !validDate(input.disponible_desde)
  )
    throw new TypeError("Invalid available-from date");
  if (
    input.estadia_minima_meses !== undefined &&
    (!Number.isSafeInteger(input.estadia_minima_meses) ||
      input.estadia_minima_meses < 1 ||
      input.estadia_minima_meses > 24)
  )
    throw new TypeError("Invalid minimum months");
  if (
    input.dias_operacion !== undefined &&
    (!Array.isArray(input.dias_operacion) ||
      input.dias_operacion.length === 0 ||
      !input.dias_operacion.every(
        (d) => Number.isInteger(d) && d >= 1 && d <= 7,
      ) ||
      new Set(input.dias_operacion).size !== input.dias_operacion.length)
  )
    throw new TypeError("Invalid operation days");
  for (const key of [
    "hora_apertura",
    "hora_cierre",
    "check_in_hora",
    "check_out_hora",
  ] as const)
    if (
      input[key] !== undefined &&
      !/^([01]\d|2[0-3]):[0-5]\d$/.test(input[key]!) &&
      !(key === "hora_cierre" && input[key] === "24:00")
    )
      throw new TypeError("Invalid time");
  for (const key of [
    "tarifa_base_dia",
    "estadia_minima",
    "estadia_maxima",
  ] as const)
    if (input[key] !== undefined && !positiveInt(input[key]))
      throw new TypeError("Invalid positive integer");
  if (
    input.estadia_minima !== undefined &&
    input.estadia_maxima !== undefined &&
    input.estadia_minima > input.estadia_maxima
  )
    throw new TypeError("Inconsistent stay limits");
  if (input.overrides !== undefined && !validOverrides(input.overrides))
    throw new TypeError("Invalid overrides");
  return input;
}
export { strongEtag };
