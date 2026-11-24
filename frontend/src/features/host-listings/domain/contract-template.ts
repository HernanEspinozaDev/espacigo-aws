import type { components } from "@/lib/api/generated/openapi";

export type ContractSection =
  components["schemas"]["SpaceContractTemplateSection"];
export type ContractTemplateDraft =
  components["schemas"]["SpaceContractTemplateDraft"];
export type ContractTemplateVersion =
  components["schemas"]["SpaceContractTemplateVersion"];
export interface ContractTemplateRead {
  draft: ContractTemplateDraft;
  etag: string;
}
export interface ContractTemplateSource {
  id: string;
  title: string;
}

export const contractTemplateExamples: ContractSection[] = [
  {
    key: "uso_destino",
    title: "Uso permitido y prohibición de subarriendo",
    body: "El arrendatario destinará el espacio exclusivamente a la actividad acordada dentro de su capacidad máxima, quedando prohibido subarrendar, ceder o destinar el inmueble a fines ilícitos o distintos a los pactados sin autorización escrita del arrendador.",
  },
  {
    key: "cuidado_restitucion",
    title: "Estado de entrega, cuidado y restitución",
    body: "El arrendatario recibe el espacio y su equipamiento en buen estado de conservación y aseo, obligándose a restituirlo al término del horario contratado en las mismas condiciones, salvo el desgaste natural por uso legítimo, respondiendo por cualquier daño o deterioro imputable.",
  },
  {
    key: "convivencia_normas",
    title: "Horarios, convivencia y seguridad",
    body: "El arrendatario deberá respetar estrictamente el horario de inicio y término pactado, las normas de ruido, seguridad, aforo del recinto o edificio y el cuidado de los espacios comunes.",
  },
];

export function validContractSections(
  value: unknown,
): value is ContractSection[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > 12)
    return false;
  const keys = new Set<string>();
  let total = 0;
  for (const s of value) {
    if (
      !s ||
      typeof s !== "object" ||
      Object.keys(s).sort().join(",") !== "body,key,title" ||
      typeof s.key !== "string" ||
      !/^[a-z][a-z0-9_]{0,39}$/.test(s.key) ||
      keys.has(s.key) ||
      !validText(s.title, 100, false) ||
      !validText(s.body, 4000, true)
    )
      return false;
    keys.add(s.key);
    total += [...s.title].length + [...s.body].length;
  }
  return total <= 20000;
}
function validText(
  value: unknown,
  max: number,
  multiline: boolean,
): value is string {
  return (
    typeof value === "string" &&
    value.trim() === value &&
    value.length > 0 &&
    [...value].length <= max &&
    !(
      multiline
        ? /[<>\u0000-\u0008\u000b-\u001f\u007f-\u009f\u061c\u200e\u200f\u2028-\u202e\u2066-\u2069]/u
        : /[<>\p{Cc}\u061c\u200e\u200f\u2028-\u202e\u2066-\u2069]/u
    ).test(value) &&
    !containsContractContact(value)
  );
}

export function containsContractContact(value: string): boolean {
  const normalized = value.normalize("NFKC");
  return [
    /[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}/iu,
    /(?:[a-z][a-z0-9+.-]*:\/\/|www\.|mailto:|tel:|wa\.me\/|t\.me\/)|\b[a-z0-9][a-z0-9.-]*\.[a-z]{2,63}\b/iu,
    /@[\p{L}\p{N}_][\p{L}\p{N}_.-]*/u,
    /(?:\+?56[ .()-]?)?\b[2-9][ .()-]?\d{4}[ .()-]?\d{4}\b|\+\d(?:[ .()-]*\d){7,14}\b/u,
    /\p{Cf}/u,
  ].some((pattern) => pattern.test(normalized));
}
export function contractTemplateEtag(revision: number): string {
  if (!Number.isSafeInteger(revision) || revision < 0)
    throw new TypeError("Invalid revision");
  return `"contract-template-${revision}"`;
}
export function validTemplateDraft(
  value: unknown,
  spaceId: string,
): value is ContractTemplateDraft {
  if (!value || typeof value !== "object") return false;
  const v = value as ContractTemplateDraft;
  return (
    v.space_id === spaceId &&
    Number.isSafeInteger(v.revision) &&
    v.revision >= 0 &&
    Number.isSafeInteger(v.published_version) &&
    v.published_version >= 0 &&
    ((v.revision === 0 &&
      Array.isArray(v.sections) &&
      v.sections.length === 0 &&
      v.updated_at === null) ||
      (v.revision > 0 &&
        validContractSections(v.sections) &&
        typeof v.updated_at === "string" &&
        Number.isFinite(Date.parse(v.updated_at))))
  );
}
export function validTemplateVersion(
  value: unknown,
  spaceId: string,
): value is ContractTemplateVersion {
  if (!value || typeof value !== "object") return false;
  const v = value as ContractTemplateVersion;
  return (
    v.space_id === spaceId &&
    typeof v.id === "string" &&
    /^[0-9a-f-]{36}$/i.test(v.id) &&
    Number.isSafeInteger(v.version) &&
    v.version > 0 &&
    validContractSections(v.sections) &&
    typeof v.sha256 === "string" &&
    /^[0-9a-f]{64}$/.test(v.sha256) &&
    typeof v.published_at === "string" &&
    Number.isFinite(Date.parse(v.published_at))
  );
}
