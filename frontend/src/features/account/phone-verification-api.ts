export interface AccountPhoneStatus {
  configured: boolean;
  verified: boolean;
  masked_phone: string;
  verified_at?: string | null;
  pending_verification: boolean;
  resend_available_at?: string | null;
}

export interface AccountPhoneCodeSent {
  status: string;
  masked_phone: string;
  resend_available_at: string;
  expires_at: string;
}

export interface AccountPhoneVerifyResult {
  status: string;
  verified: boolean;
  masked_phone: string;
  verified_at: string;
}

export class PhoneVerificationApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly retryAfterSeconds?: number;

  constructor(
    message: string,
    code: string,
    status: number,
    retryAfterSeconds?: number,
  ) {
    super(message);
    this.name = "PhoneVerificationApiError";
    this.code = code;
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

function parseErrorMessage(
  status: number,
  data: unknown,
  retryAfterHeader?: string | null,
): PhoneVerificationApiError {
  let code = "unknown_error";
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
  }

  let retryAfterSeconds: number | undefined;
  if (retryAfterHeader) {
    const parsed = parseInt(retryAfterHeader, 10);
    if (!Number.isNaN(parsed) && parsed > 0) {
      retryAfterSeconds = parsed;
    }
  }

  let userMessage =
    "Ocurrió un error al procesar la verificación telefónica. Inténtalo nuevamente.";
  switch (code) {
    case "invalid_phone":
      userMessage =
        "Número de teléfono móvil chileno inválido. Usa el formato +56 9 1234 5678.";
      break;
    case "cooldown_active":
      userMessage = retryAfterSeconds
        ? `Debes esperar ${retryAfterSeconds} segundos antes de solicitar un nuevo código.`
        : "Debes esperar unos momentos antes de solicitar un nuevo código.";
      break;
    case "no_pending_verification":
      userMessage =
        "No hay una verificación de teléfono pendiente. Solicita un código primero.";
      break;
    case "max_attempts_exceeded":
      userMessage =
        "Has superado el límite de intentos permitidos. Solicita un nuevo código.";
      break;
    case "code_expired":
      userMessage =
        "El código de verificación ha expirado. Solicita un nuevo código.";
      break;
    case "invalid_code":
      userMessage =
        "El código de verificación es incorrecto. Revisa e inténtalo nuevamente.";
      break;
    case "unauthorized":
      userMessage =
        "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      break;
    case "service_unavailable":
      userMessage =
        "El servicio de verificación telefónica no está disponible en este momento.";
      break;
    default:
      if (status === 401) {
        userMessage =
          "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      } else if (status === 429) {
        userMessage =
          "Demasiadas solicitudes. Espera un momento antes de reintentar.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new PhoneVerificationApiError(
    userMessage,
    code,
    status,
    retryAfterSeconds,
  );
}

export async function getAccountPhoneStatus(
  signal?: AbortSignal,
): Promise<AccountPhoneStatus> {
  const res = await fetch("/api/backend/account/security/phone", {
    method: "GET",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body, res.headers.get("retry-after"));
  }

  return (await res.json()) as AccountPhoneStatus;
}

export async function sendAccountPhoneCode(
  phone: string,
  signal?: AbortSignal,
): Promise<AccountPhoneCodeSent> {
  const res = await fetch("/api/backend/account/security/phone/send-code", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ phone }),
    signal,
  });

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body, res.headers.get("retry-after"));
  }

  return (await res.json()) as AccountPhoneCodeSent;
}

export async function verifyAccountPhoneCode(
  code: string,
  signal?: AbortSignal,
): Promise<AccountPhoneVerifyResult> {
  const res = await fetch("/api/backend/account/security/phone/verify", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ code }),
    signal,
  });

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body, res.headers.get("retry-after"));
  }

  return (await res.json()) as AccountPhoneVerifyResult;
}
