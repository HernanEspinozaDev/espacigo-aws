import type {
  components,
  Readable,
  Writable,
} from "@/lib/api/generated/openapi";

export type RightsRequestKind =
  | "acceso"
  | "rectificacion"
  | "supresion"
  | "oposicion"
  | "portabilidad"
  | "bloqueo";
export type RightsRequestState = Writable<
  components["schemas"]["RightsRequest"]["state"]
>; // "recibida" | "identidad_pendiente" | "en_revision" | "resuelta" | "denegada_fundada"

export type RightsRequest = Readable<components["schemas"]["RightsRequest"]>;
export type RightsRequestList = Readable<
  components["schemas"]["RightsRequestList"]
>;
export type OwnIdentityExport = Readable<
  components["schemas"]["OwnIdentityExport"]
>;

export type PrivacyPurposePreference = Readable<
  components["schemas"]["PrivacyPurposePreference"]
>;
export type PrivacyPurpose = PrivacyPurposePreference["purpose"];
export type PrivacyPurposeState = PrivacyPurposePreference["state"];

export type UpdatePrivacyPurposePreferenceInput = Writable<
  components["schemas"]["UpdatePrivacyPurposePreferenceInput"]
>;

export type PrivacyTreatmentBlock = Readable<
  components["schemas"]["PrivacyTreatmentBlock"]
>;
export type PrivacyTreatmentBlockScope = PrivacyTreatmentBlock["scope"];
export type PrivacyTreatmentBlockState = PrivacyTreatmentBlock["state"];

export type PrivacyPortabilityPackage = Readable<
  components["schemas"]["PrivacyPortabilityPackage"]
>;

export interface PrivacyPortabilityBlobResult {
  readonly blob: Blob;
  readonly sha256: string | null;
}

export const PRIVACY_PURPOSES: readonly PrivacyPurpose[] = [
  "comunicaciones_comerciales",
  "analitica_producto",
  "personalizacion_recomendaciones",
  "comparticion_encargados_opcionales",
];

export const PRIVACY_PURPOSE_LABELS: Record<PrivacyPurpose, string> = {
  comunicaciones_comerciales: "Comunicaciones comerciales",
  analitica_producto: "Analítica de producto",
  personalizacion_recomendaciones: "Personalización y recomendaciones",
  comparticion_encargados_opcionales: "Compartición con encargados opcionales",
};

export const PRIVACY_PURPOSE_DESCRIPTIONS: Record<PrivacyPurpose, string> = {
  comunicaciones_comerciales:
    "Envío de novedades, promociones y avisos informativos no esenciales.",
  analitica_producto:
    "Medición agregada de uso para mejorar flujos y estabilidad de la plataforma.",
  personalizacion_recomendaciones:
    "Sugerencias de espacios y contenidos basadas en tu actividad reciente.",
  comparticion_encargados_opcionales:
    "Integraciones opcionales con proveedores auxiliares del servicio.",
};

export const PRIVACY_PURPOSE_STATE_LABELS: Record<PrivacyPurposeState, string> = {
  permitido: "Permitido",
  opuesto: "Opuesto",
  retirado: "Retirado",
};

export const PRIVACY_TREATMENT_BLOCK_SCOPE_LABELS: Record<
  PrivacyTreatmentBlockScope,
  string
> = {
  comunicaciones_comerciales: "Comunicaciones comerciales",
  analitica_y_perfilado: "Analítica y perfilado",
  publicaciones_y_visibilidad: "Publicaciones y visibilidad",
  exportaciones_y_terceros: "Exportaciones y terceros",
  tratamiento_general: "Tratamiento general",
};

export const PRIVACY_TREATMENT_BLOCK_STATE_LABELS: Record<
  PrivacyTreatmentBlockState,
  string
> = {
  activo: "Activo",
  levantado: "Levantado",
};

export function isSuppressionPending(
  requests: readonly RightsRequest[] | null | undefined,
): boolean {
  if (!requests || !Array.isArray(requests)) return false;
  return requests.some(
    (r) =>
      r.kind === "supresion" &&
      (r.state === "recibida" ||
        r.state === "en_revision" ||
        r.state === "identidad_pendiente"),
  );
}

export function hasResolvedSuppression(
  requests: readonly RightsRequest[] | null | undefined,
): boolean {
  if (!requests || !Array.isArray(requests)) return false;
  return requests.some((r) => r.kind === "supresion" && r.state === "resuelta");
}
