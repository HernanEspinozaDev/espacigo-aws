"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type LinkedDevice,
  LinkedDevicesApiError,
  listLinkedDevices,
  revokeLinkedDevice,
  revokeOtherLinkedDevices,
} from "../linked-devices-api";

export interface UseLinkedDevicesReturn {
  devices: LinkedDevice[];
  currentDevice: LinkedDevice | null;
  otherDevices: LinkedDevice[];
  isLoading: boolean;
  revokingSessionId: string | null;
  isRevokingOthers: boolean;
  error: string | null;
  notification: string | null;
  refresh: () => Promise<void>;
  revokeDevice: (sessionId: string) => Promise<boolean>;
  revokeOtherDevices: () => Promise<number | null>;
  clearNotification: () => void;
  clearError: () => void;
}

export function useLinkedDevices(): UseLinkedDevicesReturn {
  const router = useRouter();
  const { checkSession, logout } = useAuth();

  const [devices, setDevices] = useState<LinkedDevice[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [revokingSessionId, setRevokingSessionId] = useState<string | null>(
    null,
  );
  const [isRevokingOthers, setIsRevokingOthers] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  const handleUnauthorized = useCallback(async () => {
    try {
      await checkSession();
    } catch {
      // ignore
    }
  }, [checkSession]);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const list = await listLinkedDevices(signal);
        setDevices(list);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof LinkedDevicesApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar los dispositivos vinculados.",
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

    listLinkedDevices(controller.signal)
      .then((list) => {
        if (!active) return;
        setDevices(list);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof LinkedDevicesApiError && err.status === 401) {
          handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar los dispositivos vinculados.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const revokeDevice = useCallback(
    async (sessionId: string): Promise<boolean> => {
      const target = devices.find((d) => d.id === sessionId);
      const isCurrentSession = target?.is_current === true;

      try {
        setRevokingSessionId(sessionId);
        setError(null);
        setNotification(null);

        await revokeLinkedDevice(sessionId);

        if (isCurrentSession) {
          // Current session revoked: cleanly log out and redirect to /login
          try {
            await logout();
          } catch {
            // ignore
          }
          router.push("/login");
          return true;
        }

        // Removed successfully: update state and notify
        setDevices((prev) => prev.filter((d) => d.id !== sessionId));
        setNotification("La sesión ha sido revocada exitosamente.");
        return true;
      } catch (err) {
        if (err instanceof LinkedDevicesApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo revocar la sesión solicitada.",
        );
        return false;
      } finally {
        setRevokingSessionId(null);
      }
    },
    [devices, logout, router, handleUnauthorized],
  );

  const revokeOtherDevices = useCallback(async (): Promise<number | null> => {
    try {
      setIsRevokingOthers(true);
      setError(null);
      setNotification(null);

      const result = await revokeOtherLinkedDevices();
      const count = result.revoked_count ?? 0;

      // Keep only current session
      setDevices((prev) => prev.filter((d) => d.is_current));

      const message =
        count === 1
          ? "Se cerró 1 sesión activa adicional."
          : `Se cerraron ${count} sesiones activas adicionales.`;
      setNotification(message);

      return count;
    } catch (err) {
      if (err instanceof LinkedDevicesApiError && err.status === 401) {
        await handleUnauthorized();
      }

      setError(
        err instanceof Error
          ? err.message
          : "No se pudieron revocar las otras sesiones.",
      );
      return null;
    } finally {
      setIsRevokingOthers(false);
    }
  }, [handleUnauthorized]);

  const clearNotification = useCallback(() => setNotification(null), []);
  const clearError = useCallback(() => setError(null), []);

  const currentDevice = devices.find((d) => d.is_current) ?? null;
  const otherDevices = devices.filter((d) => !d.is_current);

  return {
    devices,
    currentDevice,
    otherDevices,
    isLoading,
    revokingSessionId,
    isRevokingOthers,
    error,
    notification,
    refresh,
    revokeDevice,
    revokeOtherDevices,
    clearNotification,
    clearError,
  };
}
