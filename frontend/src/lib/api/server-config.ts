import "server-only";

const expectedPath = "/api/v1";

export type Environment = "development" | "production";

/**
 * Returns the validated runtime environment: 'development' | 'production'.
 * If process.env.ENVIRONMENT is set, it MUST be 'development' or 'production'.
 * If omitted, it infers from NODE_ENV or defaults to 'development' for local backward compatibility.
 */
export function getEnvironment(): Environment {
  const raw = process.env.ENVIRONMENT;
  if (!raw || raw.trim() === "") {
    return process.env.NODE_ENV === "production" ? "production" : "development";
  }

  const trimmed = raw.trim();
  if (trimmed === "development" || trimmed === "production") {
    return trimmed;
  }

  throw new Error(
    `ENVIRONMENT must be 'development' or 'production', received: '${raw}'`,
  );
}

export function isProductionEnvironment(): boolean {
  return getEnvironment() === "production";
}

export function getApiBackendBaseUrl(): URL {
  const raw =
    process.env.API_BACKEND_BASE_URL || process.env.API_INTERNAL_BASE_URL;
  if (!raw) {
    if (getEnvironment() === "development") {
      return new URL("http://127.0.0.1:8080/api/v1");
    }
    throw new Error("API_BACKEND_BASE_URL is not set");
  }

  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error("API_BACKEND_BASE_URL is invalid");
  }

  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("API_BACKEND_BASE_URL must use http or https");
  }

  if (
    url.username !== "" ||
    url.password !== "" ||
    url.search !== "" ||
    url.hash !== ""
  ) {
    throw new Error(
      "API_BACKEND_BASE_URL must not contain credentials, query, or hash",
    );
  }

  if (url.pathname !== expectedPath) {
    if (url.pathname === "/" || url.pathname === "") {
      url.pathname = expectedPath;
    } else if (!url.pathname.endsWith(expectedPath)) {
      url.pathname = `${url.pathname.replace(/\/$/, "")}${expectedPath}`;
    }
  }

  return url;
}

export interface ServerConfigValidation {
  valid: boolean;
  environment: Environment;
  apiBackendBaseUrl: string | null;
  error?: string;
}

/**
 * Validates the overall server configuration coherence.
 * Ensures ENVIRONMENT is valid, API_BACKEND_BASE_URL is configured and syntactically sound,
 * and environment modes do not conflict.
 */
export function validateServerConfig(): ServerConfigValidation {
  try {
    const environment = getEnvironment();
    const apiUrl = getApiBackendBaseUrl();

    if (environment === "production") {
      if (
        apiUrl.hostname !== "127.0.0.1" &&
        apiUrl.hostname !== "localhost" &&
        apiUrl.protocol !== "https:"
      ) {
        throw new Error(
          "In production, remote API_BACKEND_BASE_URL must use https:",
        );
      }
    }

    return {
      valid: true,
      environment,
      apiBackendBaseUrl: apiUrl.origin + apiUrl.pathname,
    };
  } catch (err) {
    let fallbackEnv: Environment = "development";
    try {
      fallbackEnv = getEnvironment();
    } catch {
      if (process.env.ENVIRONMENT === "production") {
        fallbackEnv = "production";
      }
    }
    return {
      valid: false,
      environment: fallbackEnv,
      apiBackendBaseUrl: null,
      error:
        err instanceof Error ? err.message : "Invalid server configuration",
    };
  }
}
