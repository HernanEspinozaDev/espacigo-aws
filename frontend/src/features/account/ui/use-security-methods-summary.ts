"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountSecurityMethodsResponse,
  SecurityLocationTrustedError,
  getAccountSecurityMethodsSummary,
} from "../security-location-trusted-api";

export interface UseSecurityMethodsSummaryReturn {
  summary: AccountSecurityMethodsResponse | null;
  isLoading: boolean;
  error: string | null;
  refresh: (signal?: AbortSignal) => Promise<void>;
  clearError: () => void;
}

export function useSecurityMethodsSummary(): UseSecurityMethodsSummaryReturn {
  const { checkSession } = useAuth();

  const [summary, setSummary] = useState<AccountSecurityMethodsResponse | null>(
    null,
  );
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const handleUnauthorized = useCallback(async () => {
    try {
      await checkSession();
    } catch {
      // ignore
    }
  }, [checkSession]);

  const clearError = useCallback(() => setError(null), []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await getAccountSecurityMethodsSummary(signal);
        setSummary(data);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el resumen de métodos de verificación.",
        );
      } finally {
        setIsLoading(false);
      }
    },
    [handleUnauthorized],
  );

  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    getAccountSecurityMethodsSummary(controller.signal)
      .then((data) => {
        if (!active) return;
        setSummary(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          handleUnauthorized().catch(() => {});
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el resumen de métodos de verificación.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  return {
    summary,
    isLoading,
    error,
    refresh,
    clearError,
  };
}
