export interface AccountQRStatus {
  enabled: boolean;
  enabled_at?: string | null;
  trusted_device_label?: string | null;
}

export interface AccountQRTicket {
  ticket_id: string;
  qr_payload: string;
  expires_at: string;
}

export type QRTicketStatus = "pending" | "approved" | "rejected" | "expired";

export interface AccountQRTicketStatus {
  ticket_id: string;
  status: QRTicketStatus;
  expires_at: string;
  resolved_at?: string | null;
}

export interface AccountQRApproveInput {
  token: string;
  device_label?: string;
}

export interface AccountQRApproveResponse {
  status: string;
  resolved_at: string;
}

export interface AccountQRDisableResponse {
  enabled: boolean;
}

export class QRApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "QRApiError";
    this.code = code;
    this.status = status;
  }
}

function parseErrorMessage(status: number, data: unknown): QRApiError {
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
    "Ocurrió un error al gestionar la verificación por código QR.";

  switch (code) {
    case "ticket_not_found":
    case "not_found":
      userMessage =
        "El ticket de verificación QR no fue encontrado o no existe.";
      break;
    case "ticket_expired":
      userMessage =
        "El código QR ha expirado. Por favor, genera uno nuevo para continuar.";
      break;
    case "ticket_already_resolved":
      userMessage = "El código QR ya fue utilizado o resuelto previamente.";
      break;
    case "invalid_qr_token":
    case "invalid_token":
      userMessage =
        "El token de verificación QR no es válido. Verifica el código e inténtalo de nuevo.";
      break;
    case "qr_not_enabled":
      userMessage =
        "El dispositivo de verificación QR no se encuentra activado.";
      break;
    case "unauthenticated":
    case "unauthorized":
      userMessage =
        "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      break;
    default:
      if (status === 401) {
        userMessage =
          "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      } else if (status === 404) {
        userMessage = "El recurso solicitado no fue encontrado.";
      } else if (status === 409) {
        userMessage = "El ticket ya fue resuelto previamente.";
      } else if (status === 422) {
        userMessage = "El código QR ha expirado o no puede ser procesado.";
      } else if (status === 400) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : "Solicitud inválida al procesar el código QR.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new QRApiError(userMessage, code, status);
}

export async function getAccountQRStatus(
  signal?: AbortSignal,
): Promise<AccountQRStatus> {
  const res = await fetch("/api/backend/account/security/qr", {
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
    enabled_at: data?.enabled_at ?? null,
    trusted_device_label: data?.trusted_device_label ?? null,
  };
}

export async function createAccountQRTicket(
  signal?: AbortSignal,
): Promise<AccountQRTicket> {
  const res = await fetch("/api/backend/account/security/qr/ticket", {
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
    ticket_id: typeof data?.ticket_id === "string" ? data.ticket_id : "",
    qr_payload: typeof data?.qr_payload === "string" ? data.qr_payload : "",
    expires_at: typeof data?.expires_at === "string" ? data.expires_at : "",
  };
}

export async function getAccountQRTicketStatus(
  ticketId: string,
  signal?: AbortSignal,
): Promise<AccountQRTicketStatus> {
  const res = await fetch(
    `/api/backend/account/security/qr/ticket/${encodeURIComponent(ticketId)}`,
    {
      method: "GET",
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
      signal,
    },
  );

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  const rawStatus = data?.status;
  const status: QRTicketStatus =
    rawStatus === "approved" ||
    rawStatus === "rejected" ||
    rawStatus === "expired"
      ? rawStatus
      : "pending";

  return {
    ticket_id: typeof data?.ticket_id === "string" ? data.ticket_id : ticketId,
    status,
    expires_at: typeof data?.expires_at === "string" ? data.expires_at : "",
    resolved_at: data?.resolved_at ?? null,
  };
}

export async function approveAccountQRTicket(
  input: AccountQRApproveInput,
  signal?: AbortSignal,
): Promise<AccountQRApproveResponse> {
  const payload: Record<string, string> = { token: input.token };
  if (input.device_label) {
    payload.device_label = input.device_label;
  }

  const res = await fetch("/api/backend/account/security/qr/approve", {
    method: "POST",
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(payload),
    signal,
  });

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    status: typeof data?.status === "string" ? data.status : "approved",
    resolved_at: typeof data?.resolved_at === "string" ? data.resolved_at : "",
  };
}

export async function disableAccountQR(
  signal?: AbortSignal,
): Promise<AccountQRDisableResponse> {
  const res = await fetch("/api/backend/account/security/qr", {
    method: "DELETE",
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
  };
}
