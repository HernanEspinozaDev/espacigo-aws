import { apiClient } from "@/lib/api/client";
import { ApiError, normalizeApiError } from "@/lib/api/errors";

export function securityConfirmationError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 429)
      return "Espera un minuto antes de solicitar otro código.";
    if (error.status === 403)
      return "No pudimos confirmar tus datos. Revisa los códigos y vuelve a intentarlo.";
    if (error.code === "email_in_use")
      return "Este correo ya está asociado a una cuenta.";
  }
  return "No pudimos completar la confirmación. Inténtalo nuevamente.";
}

export async function sendAuthenticatorChangeCode() {
  const result = await apiClient.POST(
    "/account/security/confirmation/email-code",
    {
      body: { operation: "authenticator" },
    },
  );
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  return result.data;
}

export async function startAccountEmailChange(
  email: string,
  password: string,
  otp: string,
) {
  const result = await apiClient.POST("/account/security/email/change", {
    body: { email, current_password: password, otp_code: otp },
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
  return result.data;
}

export async function confirmAccountEmailChange(code: string) {
  const result = await apiClient.POST("/account/security/email/confirm", {
    body: { email_code: code },
  });
  if (!result.response.ok)
    throw normalizeApiError(result.response, result.error);
}
