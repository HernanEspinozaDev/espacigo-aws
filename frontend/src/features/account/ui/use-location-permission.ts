"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountLocationPermission,
  SecurityLocationTrustedError,
  getAccountLocationPermission,
  updateAccountLocationPermission,
} from "../security-location-trusted-api";

export interface UseLocationPermissionReturn {
  location: AccountLocationPermission | null;
  isLoading: boolean;
  isUpdating: boolean;
  error: string | null;
  notification: string | null;
  refresh: (signal?: AbortSignal) => Promise<void>;
  updatePermission: (
    enabled: boolean,
    regionLabel?: string | null,
  ) => Promise<boolean>;
  clearError: () => void;
  clearNotification: () => void;
}

export function useLocationPermission(): UseLocationPermissionReturn {
  const { checkSession } = useAuth();

  const [location, setLocation] = useState<AccountLocationPermission | null>(
    null,
  );
  const [isLoading, setIsLoading] = useState(true);
  const [isUpdating, setIsUpdating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  const handleUnauthorized = useCallback(async () => {
    try {
      await checkSession();
    } catch {
      // ignore
    }
  }, [checkSession]);

  const clearError = useCallback(() => setError(null), []);
  const clearNotification = useCallback(() => setNotification(null), []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await getAccountLocationPermission(signal);
        setLocation(data);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (
          err instanceof SecurityLocationTrustedError &&
          err.status === 401
        ) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el permiso de ubicación.",
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

    getAccountLocationPermission(controller.signal)
      .then((data) => {
        if (!active) return;
        setLocation(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (
          err instanceof SecurityLocationTrustedError &&
          err.status === 401
        ) {
          handleUnauthorized().catch(() => {});
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el permiso de ubicación.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const updatePermission = useCallback(
    async (enabled: boolean, regionLabel?: string | null): Promise<boolean> => {
      try {
        setIsUpdating(true);
        setError(null);
        const result = await updateAccountLocationPermission({
          enabled,
          region_label: regionLabel,
        });
        setLocation(result);
        setNotification(
          enabled
            ? "Permiso de ubicación activado exitosamente."
            : "Permiso de ubicación desactivado.",
        );
        return true;
      } catch (err) {
        if (
          err instanceof SecurityLocationTrustedError &&
          err.status === 401
        ) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo actualizar el permiso de ubicación.",
        );
        return false;
      } finally {
        setIsUpdating(false);
      }
    },
    [handleUnauthorized],
  );

  return {
    location,
    isLoading,
    isUpdating,
    error,
    notification,
    refresh,
    updatePermission,
    clearError,
    clearNotification,
  };
}
