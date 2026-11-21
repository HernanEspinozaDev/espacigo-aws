export interface LinkedDevice {
  id: string;
  device_label: string;
  ip_masked: string;
  created_at: string;
  last_activity_at: string;
  expires_at: string;
  is_current: boolean;
}

export interface LinkedDevicesList {
  devices: LinkedDevice[];
}

export interface RevokeLinkedDeviceResult {
  status: string;
  session_id: string;
}

export interface RevokeOtherLinkedDevicesResult {
  status: string;
  revoked_count: number;
}

export class LinkedDevicesApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "LinkedDevicesApiError";
    this.code = code;
    this.status = status;
  }
}

function parseErrorMessage(
  status: number,
  data: unknown,
): LinkedDevicesApiError {
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
    "Ocurrió un error al gestionar los dispositivos vinculados.";

  switch (code) {
    case "unauthenticated":
    case "unauthorized":
      userMessage =
        "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      break;
    case "device_not_found":
    case "not_found":
      userMessage =
        "El dispositivo o sesión que intentas revocar no fue encontrado o ya expiró.";
      break;
    case "invalid_device":
    case "invalid_session_id":
    case "bad_request":
      userMessage = "El identificador de sesión es inválido.";
      break;
    default:
      if (status === 401) {
        userMessage =
          "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      } else if (status === 404) {
        userMessage = "El dispositivo o sesión no fue encontrado.";
      } else if (status === 400) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : "Solicitud inválida al gestionar dispositivos.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new LinkedDevicesApiError(userMessage, code, status);
}

export async function listLinkedDevices(
  signal?: AbortSignal,
): Promise<LinkedDevice[]> {
  const res = await fetch("/api/backend/account/security/devices", {
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

  const data = (await res.json()) as LinkedDevicesList;
  return Array.isArray(data?.devices) ? data.devices : [];
}

export async function revokeLinkedDevice(
  sessionId: string,
  signal?: AbortSignal,
): Promise<RevokeLinkedDeviceResult> {
  const trimmed = sessionId?.trim();
  if (!trimmed) {
    throw new LinkedDevicesApiError(
      "El identificador de sesión es requerido.",
      "invalid_device",
      400,
    );
  }

  const res = await fetch(
    `/api/backend/account/security/devices/${encodeURIComponent(trimmed)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
      signal,
    },
  );

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body);
  }

  return (await res.json()) as RevokeLinkedDeviceResult;
}

export async function revokeOtherLinkedDevices(
  signal?: AbortSignal,
): Promise<RevokeOtherLinkedDevicesResult> {
  const res = await fetch(
    "/api/backend/account/security/devices/revoke-others",
    {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
      signal,
    },
  );

  if (!res.ok) {
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw parseErrorMessage(res.status, body);
  }

  return (await res.json()) as RevokeOtherLinkedDevicesResult;
}
