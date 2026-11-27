import type { components } from "@/lib/api/generated/openapi";

export type PublicPrivacyRequestInput =
  components["schemas"]["PublicPrivacyRequestInput"];
export type PublicPrivacyRequestReceipt =
  components["schemas"]["PublicPrivacyRequestReceipt"];
export type PublicPrivacyLookupInput =
  components["schemas"]["PublicPrivacyLookupInput"];
export type PublicPrivacyRequestStatus =
  components["schemas"]["PublicPrivacyRequestStatus"];
export type RightsRequestEvent = components["schemas"]["RightsRequestEvent"];

export type PublicPrivacyRightType = PublicPrivacyRequestInput["right_type"];
export type PublicPrivacyRequesterType =
  PublicPrivacyRequestInput["requester_type"];
export type PublicPrivacyRepresentativeRelation = NonNullable<
  PublicPrivacyRequestInput["representative_relation"]
>;
export type PublicPrivacyRequestState = PublicPrivacyRequestStatus["state"];
export type RightsRequestEventType = RightsRequestEvent["event_type"];

export interface PublicPrivacyRightOption {
  readonly value: PublicPrivacyRightType;
  readonly label: string;
  readonly slaLabel: string;
  readonly isUrgentSla: boolean;
  readonly description: string;
}

export const PUBLIC_PRIVACY_RIGHT_OPTIONS: readonly PublicPrivacyRightOption[] =
  [
    {
      value: "acceso",
      label: "Acceso",
      slaLabel: "30 días corridos",
      isUrgentSla: false,
      description:
        "Solicita conocer si tratamos tus datos personales y obtener copia de ellos.",
    },
    {
      value: "rectificacion",
      label: "Rectificación",
      slaLabel: "30 días corridos",
      isUrgentSla: false,
      description:
        "Pide corregir o completar datos personales inexactos, desactualizados o incompletos.",
    },
    {
      value: "supresion",
      label: "Supresión",
      slaLabel: "30 días corridos",
      isUrgentSla: false,
      description:
        "Solicita eliminar datos personales cuando corresponda conforme a la ley vigente.",
    },
    {
      value: "oposicion",
      label: "Oposición",
      slaLabel: "30 días corridos",
      isUrgentSla: false,
      description:
        "Oponte a determinados tratamientos de tus datos por motivos legítimos.",
    },
    {
      value: "portabilidad",
      label: "Portabilidad",
      slaLabel: "30 días corridos",
      isUrgentSla: false,
      description:
        "Solicita recibir tus datos en formato estructurado, genérico y de uso común.",
    },
    {
      value: "bloqueo",
      label: "Bloqueo temporal",
      slaLabel: "2 días hábiles",
      isUrgentSla: true,
      description:
        "Suspende temporalmente el tratamiento mientras se resuelve una rectificación, supresión u oposición.",
    },
  ] as const;

export const RIGHT_TYPE_LABELS: Record<PublicPrivacyRightType, string> = {
  acceso: "Acceso",
  rectificacion: "Rectificación",
  supresion: "Supresión",
  oposicion: "Oposición",
  portabilidad: "Portabilidad",
  bloqueo: "Bloqueo temporal",
};

export const REQUESTER_TYPE_LABELS: Record<PublicPrivacyRequesterType, string> =
  {
    titular: "Titular de los datos",
    representante: "Representante",
  };

export const REPRESENTATIVE_RELATION_LABELS: Record<
  PublicPrivacyRepresentativeRelation,
  string
> = {
  mandatario: "Mandatario",
  representante_legal: "Representante legal",
  tutor_nna: "Tutor o representante de NNA",
};

export const REPRESENTATIVE_RELATION_OPTIONS: readonly {
  readonly value: PublicPrivacyRepresentativeRelation;
  readonly label: string;
}[] = [
  { value: "mandatario", label: REPRESENTATIVE_RELATION_LABELS.mandatario },
  {
    value: "representante_legal",
    label: REPRESENTATIVE_RELATION_LABELS.representante_legal,
  },
  { value: "tutor_nna", label: REPRESENTATIVE_RELATION_LABELS.tutor_nna },
] as const;

export const REQUEST_STATE_LABELS: Record<PublicPrivacyRequestState, string> = {
  recibida: "Recibida",
  identidad_pendiente: "Identidad pendiente",
  en_revision: "En revisión",
  resuelta: "Resuelta",
  denegada_fundada: "Denegada con fundamento",
};

export const EVENT_TYPE_LABELS: Record<RightsRequestEventType, string> = {
  recepcion: "Solicitud registrada",
  identidad_verificada: "Identidad verificada",
  en_revision: "En revisión",
  aclaracion_solicitada: "Aclaración solicitada",
  prorroga: "Prórroga operativa registrada",
  ejecucion_derecho: "Derecho ejecutado",
  resolucion: "Respuesta publicada",
  despacho_registrado: "Despacho registrado",
};
