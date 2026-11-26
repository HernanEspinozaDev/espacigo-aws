export const PINPASS_SETTINGS_HREF = "/mi-cuenta/seguridad/metodos/pinpass";

export interface PinpassStatus {
  configured: boolean;
  blocked_until: string | null;
  access_expires_at: string | null;
}

export type PinpassProof =
  | { current_password: string; otp_code?: string }
  | { current_password?: string; otp_code: string };

export interface PinpassSensitiveValue {
  field: PinpassField;
  value: string;
  access_expires_at: string;
}

export type PinpassField =
  "email" | "run" | "phone" | "legal_name" | "date_of_birth";

export class PinpassApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
    readonly retryAfter: string | null = null,
  ) {
    super(message);
    this.name = "PinpassApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/backend/account/security/pinpass${path}`, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });
  const data: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const code =
      data &&
      typeof data === "object" &&
      "error" in data &&
      data.error &&
      typeof data.error === "object" &&
      "code" in data.error &&
      typeof data.error.code === "string"
        ? data.error.code
        : "unknown_error";
    const messages: Record<string, string> = {
      invalid_pin: "El Pinpass no coincide. Revisa los seis dígitos.",
      temporarily_locked:
        "Se pausaron los intentos. Espera antes de volver a probar.",
      pinpass_not_configured: "Primero crea tu EspaciGo Pinpass.",
      profile_incomplete:
        "Completa y guarda los datos obligatorios de tu perfil antes de crear Pinpass.",
      pinpass_access_required: "Ingresa tu Pinpass para consultar este dato.",
      reauthentication_required: "Confirma tu identidad para continuar.",
      reauthentication_failed:
        "Revisa la contraseña o usa un código nuevo de tu autenticador.",
      unauthorized: "Tu sesión venció. Vuelve a iniciar sesión.",
    };
    throw new PinpassApiError(
      messages[code] ??
        "No se pudo completar la operación. Inténtalo nuevamente.",
      code,
      response.status,
      response.headers.get("Retry-After"),
    );
  }
  if (response.status === 204) return undefined as T;
  return (data ?? {}) as T;
}

export function getPinpassStatus(signal?: AbortSignal) {
  return request<PinpassStatus>("", { method: "GET", signal });
}

export function setPinpass(
  pin: string,
  confirmation: string,
  proof: PinpassProof,
) {
  return request<{ configured: boolean }>("", {
    method: "PUT",
    body: JSON.stringify({ pin, confirmation, ...proof }),
  });
}

export function unlockPinpass(pin: string) {
  return request<{ access_expires_at: string }>("/unlock", {
    method: "POST",
    body: JSON.stringify({ pin }),
  });
}

export function revealPinpassValue(field: PinpassField, signal?: AbortSignal) {
  return request<PinpassSensitiveValue>(`/reveal/${field}`, {
    method: "GET",
    signal,
  });
}

export function resetPinpass(proof: PinpassProof) {
  return request<void>("", { method: "DELETE", body: JSON.stringify(proof) });
}
