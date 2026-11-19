export interface AccountLocationPermission {
  enabled: boolean;
  consent_updated_at?: string | null;
  last_region_label?: string | null;
}

export interface UpdateAccountLocationPermissionInput {
  enabled: boolean;
  region_label?: string | null;
}

export interface AccountTrustedContact {
  id: string;
  full_name: string;
  masked_email: string;
  masked_phone?: string | null;
  relationship: string;
  created_at: string;
}

export interface AddAccountTrustedContactInput {
  full_name: string;
  email: string;
  phone?: string | null;
  relationship: string;
}

export interface RemoveAccountTrustedContactResponse {
  status: string;
  contact_id: string;
}

export interface AccountSecurityMethodsSummary {
  password: {
    enabled: boolean;
  };
  email: {
    enabled: boolean;
    masked_email?: string | null;
  };
  phone: {
    enabled: boolean;
    masked_phone?: string | null;
  };
  totp: {
    enabled: boolean;
  };
  passkeys: {
    enabled: boolean;
    count: number;
  };
  qr: {
    enabled: boolean;
  };
  facial_kyc: {
    enabled: boolean;
    status: string;
  };
}

export interface AccountSecurityMethodsResponse {
  protected: boolean;
  active_count: number;
  total_count: number;
  methods: AccountSecurityMethodsSummary;
}

export class SecurityLocationTrustedError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "SecurityLocationTrustedError";
    this.code = code;
    this.status = status;
  }
}

function parseErrorMessage(
  status: number,
  data: unknown,
  fallbackMessage: string,
): SecurityLocationTrustedError {
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

  let userMessage = fallbackMessage;

  switch (code) {
    case "coordinates_not_allowed":
      userMessage =
        "No se permiten coordenadas GPS exactas; usa una etiqueta de región aproximada (ej. Región Metropolitana).";
      break;
    case "invalid_region_label":
      userMessage =
        "La etiqueta de región no debe exceder 80 caracteres.";
      break;
    case "trusted_contact_limit":
      userMessage =
        "Se ha alcanzado el límite máximo de 3 personas de confianza.";
      break;
    case "invalid_full_name":
      userMessage = "El nombre debe tener entre 2 y 80 caracteres.";
      break;
    case "invalid_relationship":
      userMessage = "El parentesco o relación debe tener entre 2 y 40 caracteres.";
      break;
    case "invalid_email":
      userMessage = "Formato de correo electrónico inválido.";
      break;
    case "invalid_phone":
      userMessage = "Formato de teléfono chileno inválido (ej: +56912345678).";
      break;
    case "not_found":
      userMessage = "El recurso o contacto de confianza no fue encontrado.";
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
        userMessage =
          "Conflicto con el recurso o se alcanzó el límite permitido.";
      } else if (status === 400) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : fallbackMessage;
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new SecurityLocationTrustedError(userMessage, code, status);
}

export async function getAccountLocationPermission(
  signal?: AbortSignal,
): Promise<AccountLocationPermission> {
  const res = await fetch("/api/backend/account/security/location", {
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
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo cargar el permiso de ubicación.",
    );
  }

  return {
    enabled: Boolean(data?.enabled),
    consent_updated_at: data?.consent_updated_at ?? null,
    last_region_label: data?.last_region_label ?? null,
  };
}

export async function updateAccountLocationPermission(
  input: UpdateAccountLocationPermissionInput,
  signal?: AbortSignal,
): Promise<AccountLocationPermission> {
  const payload: { enabled: boolean; region_label?: string | null } = {
    enabled: input.enabled,
  };
  if (input.region_label !== undefined) {
    payload.region_label = input.region_label;
  }

  const res = await fetch("/api/backend/account/security/location", {
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

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo actualizar el permiso de ubicación.",
    );
  }

  return {
    enabled: Boolean(data?.enabled),
    consent_updated_at: data?.consent_updated_at ?? null,
    last_region_label: data?.last_region_label ?? null,
  };
}

export async function listAccountTrustedContacts(
  signal?: AbortSignal,
): Promise<AccountTrustedContact[]> {
  const res = await fetch("/api/backend/account/security/trusted-contacts", {
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
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo obtener la lista de personas de confianza.",
    );
  }

  const rawContacts = Array.isArray(data?.contacts) ? data.contacts : [];
  return rawContacts.map((c: Record<string, unknown>) => ({
    id: typeof c.id === "string" ? c.id : "",
    full_name: typeof c.full_name === "string" ? c.full_name : "",
    masked_email: typeof c.masked_email === "string" ? c.masked_email : "",
    masked_phone: typeof c.masked_phone === "string" ? c.masked_phone : null,
    relationship: typeof c.relationship === "string" ? c.relationship : "",
    created_at: typeof c.created_at === "string" ? c.created_at : "",
  }));
}

export async function addAccountTrustedContact(
  input: AddAccountTrustedContactInput,
  signal?: AbortSignal,
): Promise<AccountTrustedContact> {
  const payload: Record<string, string | null | undefined> = {
    full_name: input.full_name,
    email: input.email,
    relationship: input.relationship,
  };
  if (input.phone !== undefined && input.phone !== null && input.phone.trim() !== "") {
    payload.phone = input.phone.trim();
  }

  const res = await fetch("/api/backend/account/security/trusted-contacts", {
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
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo agregar la persona de confianza.",
    );
  }

  return {
    id: typeof data?.id === "string" ? data.id : "",
    full_name: typeof data?.full_name === "string" ? data.full_name : "",
    masked_email: typeof data?.masked_email === "string" ? data.masked_email : "",
    masked_phone: typeof data?.masked_phone === "string" ? data.masked_phone : null,
    relationship: typeof data?.relationship === "string" ? data.relationship : "",
    created_at: typeof data?.created_at === "string" ? data.created_at : "",
  };
}

export async function removeAccountTrustedContact(
  contactId: string,
  signal?: AbortSignal,
): Promise<RemoveAccountTrustedContactResponse> {
  const res = await fetch(
    `/api/backend/account/security/trusted-contacts/${encodeURIComponent(contactId)}`,
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

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo revocar la persona de confianza.",
    );
  }

  return {
    status: typeof data?.status === "string" ? data.status : "revoked",
    contact_id: typeof data?.contact_id === "string" ? data.contact_id : contactId,
  };
}

export async function getAccountSecurityMethodsSummary(
  signal?: AbortSignal,
): Promise<AccountSecurityMethodsResponse> {
  const res = await fetch("/api/backend/account/security/methods", {
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
    throw parseErrorMessage(
      res.status,
      data,
      "No se pudo cargar el resumen de métodos de verificación.",
    );
  }

  const rawMethods = data?.methods || {};

  return {
    protected: Boolean(data?.protected),
    active_count: typeof data?.active_count === "number" ? data.active_count : 0,
    total_count: typeof data?.total_count === "number" ? data.total_count : 7,
    methods: {
      password: {
        enabled: Boolean(rawMethods?.password?.enabled),
      },
      email: {
        enabled: Boolean(rawMethods?.email?.enabled),
        masked_email: rawMethods?.email?.masked_email ?? null,
      },
      phone: {
        enabled: Boolean(rawMethods?.phone?.enabled),
        masked_phone: rawMethods?.phone?.masked_phone ?? null,
      },
      totp: {
        enabled: Boolean(rawMethods?.totp?.enabled),
      },
      passkeys: {
        enabled: Boolean(rawMethods?.passkeys?.enabled),
        count: typeof rawMethods?.passkeys?.count === "number" ? rawMethods.passkeys.count : 0,
      },
      qr: {
        enabled: Boolean(rawMethods?.qr?.enabled),
      },
      facial_kyc: {
        enabled: Boolean(rawMethods?.facial_kyc?.enabled),
        status: typeof rawMethods?.facial_kyc?.status === "string" ? rawMethods.facial_kyc.status : "pending_kyc_module",
      },
    },
  };
}
