import type {
  CategoryProfileSchema,
  CategoryAttributeDefinition,
} from "./types";

/** Remove absent optional fields while preserving meaningful false/zero values. */
export function cleanAttributesForPayload(
  attributes: Record<string, unknown>,
): Record<string, unknown> {
  const clean: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(attributes)) {
    if (value === undefined || value === null) continue;
    if (typeof value === "string") {
      if (value.trim()) clean[key] = value;
    } else if (Array.isArray(value)) {
      // Keep invalid members intact so the category validator rejects them.
      if (value.length) clean[key] = value;
    } else clean[key] = value;
  }
  return clean;
}

function humanize(code: string): string {
  const words = code.split("_").filter(Boolean);
  return words.length
    ? words[0].charAt(0).toLocaleUpperCase("es-CL") +
        words[0].slice(1) +
        (words.length > 1 ? ` ${words.slice(1).join(" ")}` : "")
    : code;
}

const attributeValueLabels = new Map([
  ["automovil", "Automóvil"],
  ["calefaccion", "Calefacción"],
  ["camion", "Camión"],
  ["carbon", "Carbón"],
  ["electrica", "Eléctrica"],
  ["furgon", "Furgón"],
  ["lena", "Leña"],
  ["mesa_reunion", "Mesa de reunión"],
  ["porton_automatico", "Portón automático"],
  ["porton_manual", "Portón manual"],
  ["suv", "SUV"],
  ["vehiculo_liviano", "Vehículo liviano"],
  ["ventilacion_mecanica", "Ventilación mecánica"],
]);

export function formatAttributeLabel(
  code: string,
  schema?: Pick<CategoryProfileSchema, "attributes">,
): string {
  const label =
    schema?.attributes.find((attribute) => attribute.code === code)?.label ||
    code;
  return label.includes("_") || label === code
    ? (attributeValueLabels.get(label) ?? humanize(label))
    : label;
}

export function formatAttributeValue(
  value: unknown,
  definition?: CategoryAttributeDefinition,
): string {
  if (typeof value === "boolean") return value ? "Sí" : "No";
  if (Array.isArray(value))
    return value
      .map((item) => formatAttributeValue(item, definition))
      .join(", ");
  if (typeof value === "string")
    return attributeValueLabels.get(value) ?? humanize(value);
  if (typeof value === "number")
    return new Intl.NumberFormat("es-CL").format(value);
  return "";
}
