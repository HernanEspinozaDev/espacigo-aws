export interface AccountTOTPStatus {
  enabled: boolean;
  pending_setup: boolean;
  enabled_at?: string | null;
  remaining_backup_codes: number;
}

export interface AccountTOTPSetupResponse {
  otpauth_uri: string;
  manual_key: string;
}

export interface AccountTOTPEnableInput {
  code: string;
}

export interface AccountTOTPEnableResponse {
  enabled: boolean;
  enabled_at: string;
  backup_codes: string[];
}

export interface AccountTOTPDisableInput {
  code: string;
}

export interface AccountTOTPDisableResponse {
  enabled: boolean;
}

export class TOTPApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "TOTPApiError";
    this.code = code;
    this.status = status;
  }
}

function parseErrorMessage(status: number, data: unknown): TOTPApiError {
  let code = "unknown_error";
  let backendMessage = "";

  if (data && typeof data === "object") {
    const errorObj = data as {
      error?: { code?: string; message?: string };
      code?: string;
      message?: string;
    };
    if (typeof errorObj.error?.code === "string") {
      code = errorObj.error.code;
    } else if (typeof errorObj.code === "string") {
      code = errorObj.code;
    }

    if (typeof errorObj.error?.message === "string") {
      backendMessage = errorObj.error.message;
    } else if (typeof errorObj.message === "string") {
      backendMessage = errorObj.message;
    }
  }

  let userMessage = "Ocurrió un error al gestionar el autenticador TOTP.";

  switch (code) {
    case "invalid_code":
      userMessage =
        "Código de autenticación incorrecto o expirado. Verifica el código e inténtalo de nuevo.";
      break;
    case "code_replayed":
      userMessage =
        "Este código ya fue utilizado. Espera al siguiente intervalo de 30 segundos en tu app.";
      break;
    case "totp_not_pending":
      userMessage =
        "No hay una configuración de autenticador TOTP pendiente. Inicia la configuración nuevamente.";
      break;
    case "totp_already_enabled":
      userMessage =
        "Google Authenticator ya se encuentra activado en tu cuenta.";
      break;
    case "totp_not_enabled":
      userMessage =
        "Google Authenticator no se encuentra activado en tu cuenta.";
      break;
    case "invalid_request":
      userMessage =
        "El código debe ser de 6 dígitos numéricos o un código de respaldo de 8 caracteres.";
      break;
    case "unauthenticated":
    case "unauthorized":
      userMessage =
        "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      break;
    case "not_found":
      userMessage = "La cuenta o recurso solicitado no fue encontrado.";
      break;
    default:
      if (status === 401) {
        userMessage =
          "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      } else if (status === 404) {
        userMessage = "La cuenta o recurso solicitado no fue encontrado.";
      } else if (status === 400 || status === 422) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : "Solicitud inválida al procesar el autenticador TOTP.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new TOTPApiError(userMessage, code, status);
}

export async function getAccountTOTPStatus(
  signal?: AbortSignal,
): Promise<AccountTOTPStatus> {
  const res = await fetch("/api/backend/account/security/totp", {
    method: "GET",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    enabled: Boolean(data?.enabled),
    pending_setup: Boolean(data?.pending_setup),
    enabled_at: data?.enabled_at ?? null,
    remaining_backup_codes:
      typeof data?.remaining_backup_codes === "number"
        ? data.remaining_backup_codes
        : 0,
  };
}

export async function setupAccountTOTP(
  signal?: AbortSignal,
): Promise<AccountTOTPSetupResponse> {
  const res = await fetch("/api/backend/account/security/totp/setup", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    otpauth_uri: typeof data?.otpauth_uri === "string" ? data.otpauth_uri : "",
    manual_key: typeof data?.manual_key === "string" ? data.manual_key : "",
  };
}

export async function enableAccountTOTP(
  input: AccountTOTPEnableInput,
  signal?: AbortSignal,
): Promise<AccountTOTPEnableResponse> {
  const res = await fetch("/api/backend/account/security/totp/enable", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ code: input.code }),
    signal,
  });

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    enabled: Boolean(data?.enabled),
    enabled_at: typeof data?.enabled_at === "string" ? data.enabled_at : "",
    backup_codes: Array.isArray(data?.backup_codes) ? data.backup_codes : [],
  };
}

export async function disableAccountTOTP(
  input: AccountTOTPDisableInput,
  signal?: AbortSignal,
): Promise<AccountTOTPDisableResponse> {
  const res = await fetch("/api/backend/account/security/totp/disable", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ code: input.code }),
    signal,
  });

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    enabled: Boolean(data?.enabled),
  };
}
