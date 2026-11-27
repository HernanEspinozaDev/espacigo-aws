import "client-only";

import { apiClient } from "@/lib/api/client";
import {
  ApiError,
  ApiProtocolError,
  isAbortError,
  normalizeApiError,
  type ApiFieldError,
} from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type {
  PublicPrivacyLookupInput,
  PublicPrivacyRequestInput,
  PublicPrivacyRequestReceipt,
  PublicPrivacyRequestStatus,
} from "../domain/types";

const FIELD_LABELS: Record<string, string> = {
  right_type: "Tipo de derecho",
  requester_type: "Tipo de solicitante",
  requester_name: "Nombre completo del solicitante",
  requester_email: "Correo electrónico de contacto",
  representative_name: "Nombre del representante",
  representative_relation: "Relación de representación",
  subject_reference: "Referencia del titular",
  detail: "Detalle del requerimiento",
  folio: "Folio",
  tracking_code: "Código de seguimiento",
};

export const UNIFORM_LOOKUP_NOT_FOUND_MESSAGE =
  "No encontramos una solicitud que coincida con el folio y código de seguimiento ingresados. Verifica los datos e intenta nuevamente.";

export class PublicPrivacyClientError extends Error {
  readonly status: number | null;
  readonly code: string;
  readonly fields: readonly ApiFieldError[];

  constructor(
    message: string,
    options?: {
      status?: number | null;
      code?: string;
      fields?: readonly ApiFieldError[];
    },
  ) {
    super(message);
    this.name = "PublicPrivacyClientError";
    this.status = options?.status ?? null;
    this.code = options?.code ?? "unknown_error";
    this.fields = options?.fields ?? [];
  }
}

function formatValidationFields(fields: readonly ApiFieldError[]): string {
  if (fields.length === 0) return "";
  const names = [
    ...new Set(fields.map((field) => FIELD_LABELS[field.field] ?? field.field)),
  ];
  return ` Campos por revisar: ${names.join(", ")}.`;
}

export function publicPrivacyErrorMessage(
  error: unknown,
  context: "submit" | "lookup",
): string {
  if (error instanceof PublicPrivacyClientError) {
    return error.message;
  }
  if (error instanceof ApiProtocolError) {
    return "La respuesta del servicio no tuvo el formato esperado. Intenta nuevamente.";
  }
  if (!(error instanceof ApiError)) {
    return "No pudimos conectar con el canal público de privacidad. Intenta nuevamente.";
  }

  if (error.status === 404 && context === "lookup") {
    return UNIFORM_LOOKUP_NOT_FOUND_MESSAGE;
  }
  if (error.status === 422 || error.status === 400) {
    return `Los datos ingresados no son válidos o están incompletos.${formatValidationFields(error.fields)}`;
  }
  if (
    error.status === 429 ||
    error.code === "rate_limited" ||
    error.code === "too_many_requests"
  ) {
    return "Se realizaron demasiadas consultas o envíos seguidos. Espera unos minutos antes de volver a intentarlo.";
  }
  if (
    error.status === 503 ||
    error.code === "service_unavailable" ||
    error.code === "api_unavailable" ||
    error.code === "api_not_configured"
  ) {
    return "El canal público de privacidad no está disponible en este momento. Intenta nuevamente más tarde.";
  }
  return context === "lookup"
    ? "No pudimos consultar el estado de la solicitud en este momento. Intenta nuevamente."
    : "No pudimos registrar tu solicitud en este momento. Intenta nuevamente.";
}

function buildPublicPrivacyRequestPayload(
  input: PublicPrivacyRequestInput,
): PublicPrivacyRequestInput {
  const representativeName = input.representative_name?.trim();
  const subjectReference = input.subject_reference?.trim();
  return {
    right_type: input.right_type,
    requester_type: input.requester_type,
    requester_name: input.requester_name.trim(),
    requester_email: input.requester_email.trim(),
    ...(input.requester_type === "representante" && representativeName
      ? { representative_name: representativeName }
      : {}),
    ...(input.requester_type === "representante" &&
    input.representative_relation
      ? { representative_relation: input.representative_relation }
      : {}),
    ...(subjectReference ? { subject_reference: subjectReference } : {}),
    detail: input.detail.trim(),
  };
}

function toClientError(
  error: unknown,
  context: "submit" | "lookup",
): PublicPrivacyClientError {
  if (error instanceof PublicPrivacyClientError) {
    return error;
  }
  if (error instanceof ApiError) {
    return new PublicPrivacyClientError(
      publicPrivacyErrorMessage(error, context),
      {
        status: error.status,
        code: error.code,
        fields: error.fields,
      },
    );
  }
  return new PublicPrivacyClientError(
    publicPrivacyErrorMessage(error, context),
  );
}

/**
 * Envía una solicitud pública ARCO+ sin requerir sesión ni cabecera Authorization.
 */
export async function submitPublicPrivacyRequest(
  input: PublicPrivacyRequestInput,
  signal?: AbortSignal,
): Promise<PublicPrivacyRequestReceipt> {
  try {
    const payload = buildPublicPrivacyRequestPayload(input);
    const result = await apiClient.POST("/privacy/public-requests", {
      body: payload,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const receipt = result.data?.data;
    if (!receipt) {
      throw new ApiProtocolError();
    }
    return receipt;
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw toClientError(error, "submit");
  }
}

/**
 * Consulta el estado de una solicitud pública ARCO+ mediante folio y código de seguimiento.
 */
export async function lookupPublicPrivacyRequest(
  input: PublicPrivacyLookupInput,
  signal?: AbortSignal,
): Promise<PublicPrivacyRequestStatus> {
  try {
    const payload: PublicPrivacyLookupInput = {
      folio: input.folio.trim(),
      tracking_code: input.tracking_code.trim(),
    };
    const result = await apiClient.POST("/privacy/public-requests/lookup", {
      body: payload,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const status = result.data?.data;
    if (!status) {
      throw new ApiProtocolError();
    }
    return status;
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw toClientError(error, "lookup");
  }
}
