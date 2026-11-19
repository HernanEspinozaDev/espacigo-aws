"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountPasskey,
  PasskeysApiError,
  beginAccountPasskeyRegistration,
  completeAccountPasskeyRegistration,
  createWebAuthnCredential,
  deleteAccountPasskey,
  isWebAuthnSupported,
  listAccountPasskeys,
} from "../passkeys-api";

export interface UsePasskeysReturn {
  passkeys: AccountPasskey[];
  isLoading: boolean;
  isRegistering: boolean;
  deletingPasskeyId: string | null;
  error: string | null;
  notification: string | null;
  isSupported: boolean;
  refresh: (signal?: AbortSignal) => Promise<void>;
  registerPasskey: (label?: string) => Promise<AccountPasskey | null>;
  deletePasskey: (passkeyId: string) => Promise<boolean>;
  clearError: () => void;
  clearNotification: () => void;
}

const emptySubscribe = () => () => {};

export function usePasskeys(): UsePasskeysReturn {
  const { checkSession } = useAuth();

  const [passkeys, setPasskeys] = useState<AccountPasskey[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRegistering, setIsRegistering] = useState(false);
  const [deletingPasskeyId, setDeletingPasskeyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  const isSupported = useSyncExternalStore(
    emptySubscribe,
    () => isWebAuthnSupported(),
    () => true,
  );

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
        const list = await listAccountPasskeys(signal);
        setPasskeys(list);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof PasskeysApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar las llaves de acceso.",
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

    listAccountPasskeys(controller.signal)
      .then((list) => {
        if (!active) return;
        setPasskeys(list);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;
        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar las llaves de acceso.",
        );
      })
      .finally(() => {
        if (active) {
          setIsLoading(false);
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, []);

  const registerPasskey = useCallback(
    async (label?: string): Promise<AccountPasskey | null> => {
      clearError();
      clearNotification();

      if (!isWebAuthnSupported()) {
        const msg =
          "Tu navegador o dispositivo no soporta llaves de acceso WebAuthn.";
        setError(msg);
        return null;
      }

      setIsRegistering(true);
      try {
        const options = await beginAccountPasskeyRegistration();
        const verifyInput = await createWebAuthnCredential(options, label);
        const newPasskey = await completeAccountPasskeyRegistration(verifyInput);

        setPasskeys((prev) => [
          newPasskey,
          ...prev.filter((p) => p.id !== newPasskey.id),
        ]);
        setNotification(
          `Llave de acceso "${newPasskey.label}" agregada exitosamente.`,
        );
        return newPasskey;
      } catch (err) {
        if (err instanceof PasskeysApiError && err.status === 401) {
          await handleUnauthorized();
        }

        const message =
          err instanceof Error
            ? err.message
            : "Ocurrió un error al registrar la llave de acceso.";
        setError(message);
        return null;
      } finally {
        setIsRegistering(false);
      }
    },
    [clearError, clearNotification, handleUnauthorized],
  );

  const deletePasskey = useCallback(
    async (passkeyId: string): Promise<boolean> => {
      clearError();
      clearNotification();
      setDeletingPasskeyId(passkeyId);

      try {
        await deleteAccountPasskey(passkeyId);
        setPasskeys((prev) => prev.filter((p) => p.id !== passkeyId));
        setNotification("Llave de acceso eliminada exitosamente.");
        return true;
      } catch (err) {
        if (err instanceof PasskeysApiError && err.status === 401) {
          await handleUnauthorized();
        }

        const message =
          err instanceof Error
            ? err.message
            : "No se pudo eliminar la llave de acceso.";
        setError(message);
        return false;
      } finally {
        setDeletingPasskeyId(null);
      }
    },
    [clearError, clearNotification, handleUnauthorized],
  );

  return {
    passkeys,
    isLoading,
    isRegistering,
    deletingPasskeyId,
    error,
    notification,
    isSupported,
    refresh,
    registerPasskey,
    deletePasskey,
    clearError,
    clearNotification,
  };
}
