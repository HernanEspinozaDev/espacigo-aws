export interface SecurityAlertPreferences {
  app_notifications_enabled: boolean;
  email_alerts_enabled: boolean;
  whatsapp_alerts_enabled: boolean;
  masked_email: string;
  masked_extra_email: string;
  extra_email_configured: boolean;
  masked_phone: string;
}

export interface SecurityAlertEvent {
  id: string;
  event_code: string;
  title: string;
  device_label: string;
  ip_masked: string;
  created_at: string;
}

export interface SecurityAlertsResponse {
  preferences: SecurityAlertPreferences;
  alerts: SecurityAlertEvent[];
}

export interface UpdateSecurityAlertChannelsInput {
  app_notifications_enabled: boolean;
  email_alerts_enabled: boolean;
  whatsapp_alerts_enabled: boolean;
  extra_email?: string | null;
}

export class SecurityAlertsApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "SecurityAlertsApiError";
    this.code = code;
    this.status = status;
  }
}

function parseErrorMessage(
  status: number,
  data: unknown,
): SecurityAlertsApiError {
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

  let userMessage =
    "Ocurrió un error al gestionar las alertas y canales de seguridad.";

  switch (code) {
    case "invalid_email":
      userMessage =
        "Formato de correo electrónico secundario inválido. Revisa la dirección ingresada.";
      break;
    case "email_same_as_primary":
      userMessage =
        "El correo secundario no puede ser igual al correo principal de la cuenta.";
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
      } else if (status === 400) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : "Solicitud inválida al actualizar preferencias de alerta.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new SecurityAlertsApiError(userMessage, code, status);
}

export async function getSecurityAlerts(
  signal?: AbortSignal,
): Promise<SecurityAlertsResponse> {
  const res = await fetch("/api/backend/account/security/alerts", {
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
    throw parseErrorMessage(res.status, body);
  }

  const data = (await res.json()) as SecurityAlertsResponse;
  return {
    preferences: data?.preferences ?? {
      app_notifications_enabled: true,
      email_alerts_enabled: true,
      whatsapp_alerts_enabled: false,
      masked_email: "",
      masked_extra_email: "",
      extra_email_configured: false,
      masked_phone: "",
    },
    alerts: Array.isArray(data?.alerts) ? data.alerts : [],
  };
}

export async function updateSecurityAlertChannels(
  input: UpdateSecurityAlertChannelsInput,
  signal?: AbortSignal,
): Promise<SecurityAlertPreferences> {
  const payload: Record<string, unknown> = {
    app_notifications_enabled: Boolean(input.app_notifications_enabled),
    email_alerts_enabled: Boolean(input.email_alerts_enabled),
    whatsapp_alerts_enabled: Boolean(input.whatsapp_alerts_enabled),
  };
  if (input.extra_email !== undefined) {
    payload.extra_email = input.extra_email;
  }

  const res = await fetch("/api/backend/account/security/alerts/channels", {
    method: "PUT",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(payload),
    signal,
  });

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body);
  }

  return (await res.json()) as SecurityAlertPreferences;
}
