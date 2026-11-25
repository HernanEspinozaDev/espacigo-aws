import { normalizeApiError } from "@/lib/api/errors";
import { apiClient } from "@/lib/api/client";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { components } from "@/lib/api/generated/openapi";
import type {
  IdentityProfileInput,
  IdentityProfileSummary,
} from "../domain/identity-profile";

export async function getIdentityProfile(
  signal?: AbortSignal,
): Promise<IdentityProfileSummary> {
  const result = await apiClient.GET("/account/identity-profile", { signal });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data) throw new Error("Respuesta vacía del perfil de identidad.");
  return result.data;
}

export async function saveIdentityProfile(
  input: IdentityProfileInput,
): Promise<IdentityProfileSummary> {
  const body: IdentityProfileInput = {
    username: input.username.trim(),
    first_name: input.first_name.trim(),
    first_last_name: input.first_last_name.trim(),
    ...(input.no_second_last_name
      ? {}
      : { second_last_name: input.second_last_name?.trim() }),
    no_second_last_name: input.no_second_last_name,
    rut: input.rut ? input.rut.trim().toLocaleUpperCase("en-US") : "",
    date_of_birth: input.date_of_birth,
    document_type: "cl_id",
    phone: input.phone.trim(),
    consent: input.consent,
  };
  const result = await apiClient.PUT("/account/identity-profile", {
    body: body as never,
    bodySerializer: serializeExactJson,
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data) throw new Error("Respuesta vacía del perfil de identidad.");
  return result.data;
}

export async function saveIdentityUsername(
  username: string,
): Promise<IdentityProfileSummary> {
  const result = await apiClient.PATCH("/account/identity-profile/username", {
    body: { username: username.trim() },
    bodySerializer: serializeExactJson,
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data) throw new Error("Respuesta vacía del perfil de identidad.");
  return result.data;
}

export async function saveIdentityProfileDraft(
  draft: components["schemas"]["IdentityProfileDraft"],
): Promise<void> {
  const result = await apiClient.PATCH("/account/identity-profile/draft", {
    body: draft as never,
    bodySerializer: serializeExactJson,
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
}

export async function restoreIdentityProfileDraft(
  currentPassword: string,
): Promise<components["schemas"]["IdentityProfileDraft"]> {
  const result = await apiClient.POST(
    "/account/identity-profile/draft/restore",
    {
      body: { current_password: currentPassword },
      bodySerializer: serializeExactJson,
    },
  );
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data)
    throw new Error("Respuesta vacía al restaurar el borrador.");
  return result.data;
}

export async function startIdentityVerification(
  returnTo?: string | null,
): Promise<{ url: string }> {
  const result = await apiClient.POST(
    "/account/identity-profile/verification",
    {
      body: (returnTo ? { return_to: returnTo } : {}) as never,
      bodySerializer: serializeExactJson,
    },
  );
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data)
    throw new Error("Respuesta vacía al iniciar la verificación.");
  return result.data;
}

export async function refreshIdentityVerification(): Promise<IdentityProfileSummary> {
  const result = await apiClient.POST(
    "/account/identity-profile/verification/refresh",
    {},
  );
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  if (!result.data)
    throw new Error("Respuesta vacía al actualizar la verificación.");
  return result.data;
}
