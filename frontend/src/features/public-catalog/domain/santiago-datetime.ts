export const SANTIAGO_TIME_ZONE = "America/Santiago";

export type LocalDateTimeResolution =
  | { kind: "invalid" }
  | { kind: "nonexistent" }
  | { kind: "unique"; value: string }
  | { kind: "ambiguous"; earlier: string; later: string };

const formatter = new Intl.DateTimeFormat("en-CA", {
  timeZone: SANTIAGO_TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

function partsAt(timestamp: number) {
  const parts = Object.fromEntries(
    formatter
      .formatToParts(new Date(timestamp))
      .map(({ type, value }) => [type, value]),
  );
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
}

function offsetAt(timestamp: number) {
  const parts = Object.fromEntries(
    formatter
      .formatToParts(new Date(timestamp))
      .map(({ type, value }) => [type, value]),
  );
  const localAsUtc = Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour),
    Number(parts.minute),
    Number(parts.second),
  );
  return (localAsUtc - Math.floor(timestamp / 1000) * 1000) / 60_000;
}

function isoWithOffset(timestamp: number) {
  const local = partsAt(timestamp);
  const offset = offsetAt(timestamp);
  const sign = offset < 0 ? "-" : "+";
  const absolute = Math.abs(offset);
  const hours = String(Math.floor(absolute / 60)).padStart(2, "0");
  const minutes = String(absolute % 60).padStart(2, "0");
  return `${local}:00${sign}${hours}:${minutes}`;
}

export function resolveSantiagoLocalDateTime(
  value: string,
): LocalDateTimeResolution {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value))
    return { kind: "invalid" };
  const [date, time] = value.split("T");
  const [year, month, day] = date!.split("-").map(Number);
  const [hour, minute] = time!.split(":").map(Number);
  const nominal = Date.UTC(year!, month! - 1, day!, hour!, minute!);
  const check = new Date(nominal);
  if (
    check.getUTCFullYear() !== year ||
    check.getUTCMonth() + 1 !== month ||
    check.getUTCDate() !== day ||
    hour! > 23 ||
    minute! > 59
  )
    return { kind: "invalid" };

  const offsets = new Set<number>();
  for (let delta = -36; delta <= 36; delta += 3)
    offsets.add(offsetAt(nominal + delta * 3_600_000));
  const instants = [...offsets]
    .map((offset) => nominal - offset * 60_000)
    .filter((timestamp) => partsAt(timestamp) === value)
    .sort((a, b) => a - b);
  if (instants.length === 0) return { kind: "nonexistent" };
  if (instants.length === 1)
    return { kind: "unique", value: isoWithOffset(instants[0]!) };
  return {
    kind: "ambiguous",
    earlier: isoWithOffset(instants[0]!),
    later: isoWithOffset(instants.at(-1)!),
  };
}

export function santiagoLocalInputValue(rfc3339: string | undefined) {
  if (!rfc3339) return "";
  const timestamp = Date.parse(rfc3339);
  return Number.isFinite(timestamp) ? partsAt(timestamp) : "";
}
