export interface AccountPasskey {
  id: string;
  label: string;
  transports: string[];
  created_at: string;
  last_used_at?: string | null;
}

export interface AccountPasskeyListResponse {
  passkeys: AccountPasskey[];
}

export interface AccountPasskeyRegistrationOptions {
  challenge_id: string;
  challenge: string;
  rp: {
    id?: string;
    name: string;
  };
  user: {
    id: string;
    name: string;
    displayName: string;
  };
  pubKeyCredParams: {
    type: string;
    alg: number;
  }[];
  timeout: number;
  excludeCredentials: {
    type: string;
    id: string;
    transports?: string[];
  }[];
}

export interface AccountPasskeyRegistrationVerifyInput {
  challenge_id: string;
  label?: string;
  credential_id: string;
  public_key: string;
  client_data_json: string;
  attestation_object?: string | null;
  transports?: string[];
  algorithm?: number;
}

export interface AccountPasskeyDeleteResponse {
  status: "deleted";
  passkey_id: string;
}

export class PasskeysApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "PasskeysApiError";
    this.code = code;
    this.status = status;
  }
}

export function parseErrorMessage(
  status: number,
  data: unknown,
): PasskeysApiError {
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

  let userMessage = "Ocurrió un error al gestionar las llaves de acceso.";

  switch (code) {
    case "challenge_expired":
      userMessage =
        "El desafío de registro ha expirado. Por favor, inténtalo de nuevo.";
      break;
    case "challenge_invalid":
      userMessage =
        "El desafío de registro no es válido. Por favor, inténtalo de nuevo.";
      break;
    case "challenge_consumed":
      userMessage =
        "El desafío ya fue utilizado. Genera una nueva solicitud de registro.";
      break;
    case "invalid_client_data":
      userMessage =
        "Los datos de validación emitidos por el navegador son incorrectos.";
      break;
    case "invalid_public_key":
      userMessage = "La clave pública emitida por el dispositivo no es válida.";
      break;
    case "credential_conflict":
      userMessage =
        "Esta llave de acceso ya se encuentra registrada en tu cuenta.";
      break;
    case "passkey_not_found":
      userMessage =
        "La llave de acceso solicitada no fue encontrada o ya fue eliminada.";
      break;
    case "webauthn_not_supported":
      userMessage =
        "Tu navegador o dispositivo actual no soporta llaves de acceso (WebAuthn).";
      break;
    case "user_cancelled":
      userMessage =
        "El registro de la llave de acceso fue cancelado por el usuario o superó el tiempo límite.";
      break;
    case "algorithm_not_supported":
      userMessage =
        "Tu dispositivo no soporta los algoritmos criptográficos requeridos.";
      break;
    case "unauthenticated":
    case "unauthorized":
      userMessage =
        "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      break;
    case "not_found":
      userMessage = "El recurso solicitado no fue encontrado.";
      break;
    default:
      if (status === 401) {
        userMessage =
          "Tu sesión no es válida o ha expirado. Inicia sesión nuevamente.";
      } else if (status === 404) {
        userMessage =
          "La llave de acceso solicitada no fue encontrada o ya fue eliminada.";
      } else if (status === 409) {
        userMessage =
          "Esta llave de acceso ya se encuentra registrada en tu cuenta.";
      } else if (status === 400 || status === 422) {
        userMessage =
          backendMessage && backendMessage.length <= 100
            ? backendMessage
            : "Solicitud inválida al procesar la llave de acceso.";
      } else if (status >= 500) {
        userMessage =
          "Servicio temporalmente no disponible. Inténtalo más tarde.";
      }
      break;
  }

  return new PasskeysApiError(userMessage, code, status);
}

export function base64UrlToUint8Array(base64url: string): Uint8Array {
  try {
    const padding = "=".repeat((4 - (base64url.length % 4)) % 4);
    const base64 = (base64url + padding).replace(/-/g, "+").replace(/_/g, "/");
    const rawData = atob(base64);
    const outputArray = new Uint8Array(rawData.length);
    for (let i = 0; i < rawData.length; ++i) {
      outputArray[i] = rawData.charCodeAt(i);
    }
    return outputArray;
  } catch {
    return new TextEncoder().encode(base64url);
  }
}

export function uint8ArrayToBase64Url(bytes: Uint8Array | ArrayBuffer): string {
  const array = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let binary = "";
  for (let i = 0; i < array.byteLength; i++) {
    binary += String.fromCharCode(array[i]);
  }
  const base64 = btoa(binary);
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function isWebAuthnSupported(): boolean {
  const nav =
    typeof window !== "undefined"
      ? window.navigator
      : typeof navigator !== "undefined"
        ? navigator
        : undefined;

  return Boolean(
    nav && nav.credentials && typeof nav.credentials.create === "function",
  );
}

export async function isPlatformAuthenticatorAvailable(): Promise<boolean> {
  if (!isWebAuthnSupported()) return false;
  const pkc =
    typeof window !== "undefined"
      ? window.PublicKeyCredential
      : typeof PublicKeyCredential !== "undefined"
        ? PublicKeyCredential
        : undefined;

  if (
    typeof pkc !== "undefined" &&
    typeof pkc.isUserVerifyingPlatformAuthenticatorAvailable === "function"
  ) {
    try {
      return await pkc.isUserVerifyingPlatformAuthenticatorAvailable();
    } catch {
      return false;
    }
  }
  return false;
}

export async function createWebAuthnCredential(
  options: AccountPasskeyRegistrationOptions,
  label?: string,
): Promise<AccountPasskeyRegistrationVerifyInput> {
  const nav =
    typeof window !== "undefined"
      ? window.navigator
      : typeof navigator !== "undefined"
        ? navigator
        : undefined;

  if (
    !nav ||
    !nav.credentials ||
    typeof nav.credentials.create !== "function"
  ) {
    throw new PasskeysApiError(
      "Tu navegador o dispositivo actual no soporta llaves de acceso (WebAuthn).",
      "webauthn_not_supported",
      400,
    );
  }

  const challengeBytes = base64UrlToUint8Array(options.challenge);
  const userBytes = new TextEncoder().encode(options.user.id);

  const excludeCredentials: PublicKeyCredentialDescriptor[] = (
    options.excludeCredentials || []
  ).map((item) => ({
    type: "public-key" as const,
    id: base64UrlToUint8Array(item.id) as unknown as BufferSource,
    transports: item.transports as AuthenticatorTransport[] | undefined,
  }));

  const pubKeyCredParams: PublicKeyCredentialParameters[] =
    options.pubKeyCredParams && options.pubKeyCredParams.length > 0
      ? options.pubKeyCredParams.map((p) => ({
          type: "public-key" as const,
          alg: p.alg,
        }))
      : [
          { type: "public-key", alg: -7 }, // ES256
          { type: "public-key", alg: -257 }, // RS256
        ];

  const creationOptions: CredentialCreationOptions = {
    publicKey: {
      challenge: challengeBytes as unknown as BufferSource,
      rp: {
        id: options.rp.id,
        name: options.rp.name,
      },
      user: {
        id: userBytes as unknown as BufferSource,
        name: options.user.name,
        displayName: options.user.displayName,
      },
      pubKeyCredParams,
      timeout: options.timeout || 60000,
      excludeCredentials,
      authenticatorSelection: {
        residentKey: "preferred",
        requireResidentKey: false,
        userVerification: "preferred",
      },
      attestation: "none",
    },
  };

  let credential: PublicKeyCredential | null = null;
  try {
    credential = (await nav.credentials.create(
      creationOptions,
    )) as PublicKeyCredential | null;
  } catch (err: unknown) {
    const errorName =
      err instanceof Error || (err && typeof err === "object" && "name" in err)
        ? String((err as { name?: unknown }).name)
        : "";

    if (errorName === "NotAllowedError") {
      throw new PasskeysApiError(
        "El registro de la llave de acceso fue cancelado por el usuario o superó el tiempo límite.",
        "user_cancelled",
        400,
      );
    }
    if (errorName === "InvalidStateError") {
      throw new PasskeysApiError(
        "Esta llave de acceso ya se encuentra registrada en tu dispositivo.",
        "credential_conflict",
        409,
      );
    }
    if (errorName === "NotSupportedError") {
      throw new PasskeysApiError(
        "Tu dispositivo no soporta los algoritmos criptográficos requeridos.",
        "algorithm_not_supported",
        400,
      );
    }
    throw err;
  }

  if (!credential) {
    throw new PasskeysApiError(
      "No se pudo crear la credencial en el dispositivo.",
      "credential_creation_failed",
      400,
    );
  }

  const response = credential.response as AuthenticatorAttestationResponse;
  const clientDataJSON = uint8ArrayToBase64Url(response.clientDataJSON);

  let publicKey = "";
  if (typeof response.getPublicKey === "function") {
    const pkBuffer = response.getPublicKey();
    if (pkBuffer) {
      publicKey = uint8ArrayToBase64Url(pkBuffer);
    }
  }

  let attestationObject: string | null = null;
  if (response.attestationObject) {
    attestationObject = uint8ArrayToBase64Url(response.attestationObject);
    if (!publicKey) {
      publicKey = attestationObject;
    }
  }

  const transports =
    typeof response.getTransports === "function"
      ? (response.getTransports() as string[])
      : undefined;

  const algorithm =
    typeof response.getPublicKeyAlgorithm === "function"
      ? response.getPublicKeyAlgorithm()
      : undefined;

  return {
    challenge_id: options.challenge_id,
    label: label?.trim() || undefined,
    credential_id: credential.id,
    public_key: publicKey,
    client_data_json: clientDataJSON,
    attestation_object: attestationObject,
    transports,
    algorithm,
  };
}

export async function listAccountPasskeys(
  signal?: AbortSignal,
): Promise<AccountPasskey[]> {
  const res = await fetch("/api/backend/account/security/passkeys", {
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

  if (!data || !Array.isArray(data.passkeys)) {
    return [];
  }

  return data.passkeys.map((item: Partial<AccountPasskey>) => ({
    id: String(item.id || ""),
    label: String(item.label || "Llave de acceso"),
    transports: Array.isArray(item.transports)
      ? item.transports.map((t) => String(t))
      : [],
    created_at: String(item.created_at || ""),
    last_used_at: item.last_used_at ? String(item.last_used_at) : null,
  }));
}

export async function beginAccountPasskeyRegistration(
  signal?: AbortSignal,
): Promise<AccountPasskeyRegistrationOptions> {
  const res = await fetch(
    "/api/backend/account/security/passkeys/register/options",
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

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return data as AccountPasskeyRegistrationOptions;
}

export async function completeAccountPasskeyRegistration(
  input: AccountPasskeyRegistrationVerifyInput,
  signal?: AbortSignal,
): Promise<AccountPasskey> {
  const res = await fetch(
    "/api/backend/account/security/passkeys/register/verify",
    {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
      signal,
    },
  );

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw parseErrorMessage(res.status, data);
  }

  return {
    id: String(data?.id || ""),
    label: String(data?.label || input.label || "Llave de acceso"),
    transports: Array.isArray(data?.transports)
      ? data.transports.map((t: unknown) => String(t))
      : input.transports || [],
    created_at: String(data?.created_at || new Date().toISOString()),
    last_used_at: data?.last_used_at ? String(data.last_used_at) : null,
  };
}

export async function deleteAccountPasskey(
  passkeyId: string,
  signal?: AbortSignal,
): Promise<AccountPasskeyDeleteResponse> {
  const res = await fetch(
    `/api/backend/account/security/passkeys/${encodeURIComponent(passkeyId)}`,
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
    throw parseErrorMessage(res.status, data);
  }

  return {
    status: "deleted",
    passkey_id: String(data?.passkey_id || passkeyId),
  };
}
