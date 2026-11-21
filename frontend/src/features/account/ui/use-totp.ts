"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountTOTPEnableResponse,
  type AccountTOTPSetupResponse,
  type AccountTOTPStatus,
  TOTPApiError,
  disableAccountTOTP,
  enableAccountTOTP,
  getAccountTOTPStatus,
  setupAccountTOTP,
} from "../totp-api";

export interface UseTOTPReturn {
  status: AccountTOTPStatus | null;
  setupData: AccountTOTPSetupResponse | null;
  backupCodes: string[] | null;
  isLoading: boolean;
  isSettingUp: boolean;
  isEnabling: boolean;
  isDisabling: boolean;
  error: string | null;
  notification: string | null;
  refresh: (signal?: AbortSignal) => Promise<void>;
  startSetup: () => Promise<AccountTOTPSetupResponse | null>;
  enable: (code: string) => Promise<AccountTOTPEnableResponse | null>;
  disable: (code: string) => Promise<boolean>;
  clearError: () => void;
  clearNotification: () => void;
  clearBackupCodes: () => void;
  resetSetup: () => void;
}

export function useTOTP(): UseTOTPReturn {
  const { checkSession } = useAuth();

  const [status, setStatus] = useState<AccountTOTPStatus | null>(null);
  const [setupData, setSetupData] = useState<AccountTOTPSetupResponse | null>(
    null,
  );
  const [backupCodes, setBackupCodes] = useState<string[] | null>(null);

  const [isLoading, setIsLoading] = useState(true);
  const [isSettingUp, setIsSettingUp] = useState(false);
  const [isEnabling, setIsEnabling] = useState(false);
  const [isDisabling, setIsDisabling] = useState(false);

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
  const clearBackupCodes = useCallback(() => setBackupCodes(null), []);
  const resetSetup = useCallback(() => {
    setSetupData(null);
    clearError();
  }, [clearError]);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await getAccountTOTPStatus(signal);
        setStatus(data);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof TOTPApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de Google Authenticator.",
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

    getAccountTOTPStatus(controller.signal)
      .then((data) => {
        if (!active) return;
        setStatus(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof TOTPApiError && err.status === 401) {
          handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de Google Authenticator.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const startSetup =
    useCallback(async (): Promise<AccountTOTPSetupResponse | null> => {
      try {
        setIsSettingUp(true);
        setError(null);
        setNotification(null);
        const data = await setupAccountTOTP();
        setSetupData(data);
        return data;
      } catch (err) {
        if (err instanceof TOTPApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo iniciar la configuración de Google Authenticator.",
        );
        return null;
      } finally {
        setIsSettingUp(false);
      }
    }, [handleUnauthorized]);

  const enable = useCallback(
    async (code: string): Promise<AccountTOTPEnableResponse | null> => {
      try {
        setIsEnabling(true);
        setError(null);
        setNotification(null);
        const data = await enableAccountTOTP({ code });
        setStatus({
          enabled: true,
          pending_setup: false,
          enabled_at: data.enabled_at,
          remaining_backup_codes: data.backup_codes.length,
        });
        setBackupCodes(data.backup_codes);
        setSetupData(null);
        setNotification("¡Google Authenticator ha sido activado exitosamente!");
        return data;
      } catch (err) {
        if (err instanceof TOTPApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo verificar el código de Google Authenticator.",
        );
        return null;
      } finally {
        setIsEnabling(false);
      }
    },
    [handleUnauthorized],
  );

  const disable = useCallback(
    async (code: string): Promise<boolean> => {
      try {
        setIsDisabling(true);
        setError(null);
        setNotification(null);
        await disableAccountTOTP({ code });
        setStatus({
          enabled: false,
          pending_setup: false,
          enabled_at: null,
          remaining_backup_codes: 0,
        });
        setBackupCodes(null);
        setSetupData(null);
        setNotification(
          "Google Authenticator ha sido desactivado de tu cuenta.",
        );
        return true;
      } catch (err) {
        if (err instanceof TOTPApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo desactivar Google Authenticator.",
        );
        return false;
      } finally {
        setIsDisabling(false);
      }
    },
    [handleUnauthorized],
  );

  return {
    status,
    setupData,
    backupCodes,
    isLoading,
    isSettingUp,
    isEnabling,
    isDisabling,
    error,
    notification,
    refresh,
    startSetup,
    enable,
    disable,
    clearError,
    clearNotification,
    clearBackupCodes,
    resetSetup,
  };
}
