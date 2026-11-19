"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type SecurityAlertEvent,
  type SecurityAlertPreferences,
  SecurityAlertsApiError,
  type UpdateSecurityAlertChannelsInput,
  getSecurityAlerts,
  updateSecurityAlertChannels,
} from "../security-alerts-api";

export interface UseSecurityAlertsReturn {
  preferences: SecurityAlertPreferences | null;
  alerts: SecurityAlertEvent[];
  isLoading: boolean;
  isSaving: boolean;
  error: string | null;
  notification: string | null;
  refresh: () => Promise<void>;
  toggleChannel: (
    channel: "app" | "email" | "whatsapp",
    enabled: boolean,
  ) => Promise<boolean>;
  saveExtraEmail: (email: string) => Promise<boolean>;
  removeExtraEmail: () => Promise<boolean>;
  updateChannels: (
    input: Partial<UpdateSecurityAlertChannelsInput>,
  ) => Promise<boolean>;
  clearNotification: () => void;
  clearError: () => void;
}

export function useSecurityAlerts(): UseSecurityAlertsReturn {
  const { checkSession } = useAuth();

  const [preferences, setPreferences] =
    useState<SecurityAlertPreferences | null>(null);
  const [alerts, setAlerts] = useState<SecurityAlertEvent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
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
        const data = await getSecurityAlerts(signal);
        setPreferences(data.preferences);
        setAlerts(data.alerts);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityAlertsApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar las alertas de seguridad.",
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

    getSecurityAlerts(controller.signal)
      .then((data) => {
        if (!active) return;
        setPreferences(data.preferences);
        setAlerts(data.alerts);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityAlertsApiError && err.status === 401) {
          handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron cargar las alertas de seguridad.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const toggleChannel = useCallback(
    async (
      channel: "app" | "email" | "whatsapp",
      enabled: boolean,
    ): Promise<boolean> => {
      if (!preferences) return false;

      const currentApp = preferences.app_notifications_enabled;
      const currentEmail = preferences.email_alerts_enabled;
      const currentWhatsApp = preferences.whatsapp_alerts_enabled;

      const nextApp = channel === "app" ? enabled : currentApp;
      const nextEmail = channel === "email" ? enabled : currentEmail;
      const nextWhatsApp = channel === "whatsapp" ? enabled : currentWhatsApp;

      // Optimistic state
      setPreferences((prev) =>
        prev
          ? {
              ...prev,
              app_notifications_enabled: nextApp,
              email_alerts_enabled: nextEmail,
              whatsapp_alerts_enabled: nextWhatsApp,
            }
          : prev,
      );

      try {
        setIsSaving(true);
        setError(null);
        setNotification(null);

        const updated = await updateSecurityAlertChannels({
          app_notifications_enabled: nextApp,
          email_alerts_enabled: nextEmail,
          whatsapp_alerts_enabled: nextWhatsApp,
        });

        setPreferences(updated);
        setNotification("Preferencias de canales actualizadas.");

        // Refresh alerts feed in background to reflect audit event
        getSecurityAlerts()
          .then((data) => {
            if (data?.alerts) setAlerts(data.alerts);
          })
          .catch(() => {});

        return true;
      } catch (err) {
        // Revert on failure
        setPreferences((prev) =>
          prev
            ? {
                ...prev,
                app_notifications_enabled: currentApp,
                email_alerts_enabled: currentEmail,
                whatsapp_alerts_enabled: currentWhatsApp,
              }
            : prev,
        );

        if (err instanceof SecurityAlertsApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron actualizar los canales de alerta.",
        );
        return false;
      } finally {
        setIsSaving(false);
      }
    },
    [preferences, handleUnauthorized],
  );

  const saveExtraEmail = useCallback(
    async (email: string): Promise<boolean> => {
      if (!preferences) return false;
      const trimmed = email.trim();
      if (!trimmed) {
        setError("El correo electrónico adicional no puede estar vacío.");
        return false;
      }

      try {
        setIsSaving(true);
        setError(null);
        setNotification(null);

        const updated = await updateSecurityAlertChannels({
          app_notifications_enabled: preferences.app_notifications_enabled,
          email_alerts_enabled: preferences.email_alerts_enabled,
          whatsapp_alerts_enabled: preferences.whatsapp_alerts_enabled,
          extra_email: trimmed,
        });

        setPreferences(updated);
        setNotification(
          "Correo electrónico adicional configurado exitosamente.",
        );

        // Refresh alerts feed in background
        getSecurityAlerts()
          .then((data) => {
            if (data?.alerts) setAlerts(data.alerts);
          })
          .catch(() => {});

        return true;
      } catch (err) {
        if (err instanceof SecurityAlertsApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo guardar el correo electrónico adicional.",
        );
        return false;
      } finally {
        setIsSaving(false);
      }
    },
    [preferences, handleUnauthorized],
  );

  const removeExtraEmail = useCallback(async (): Promise<boolean> => {
    if (!preferences) return false;

    try {
      setIsSaving(true);
      setError(null);
      setNotification(null);

      const updated = await updateSecurityAlertChannels({
        app_notifications_enabled: preferences.app_notifications_enabled,
        email_alerts_enabled: preferences.email_alerts_enabled,
        whatsapp_alerts_enabled: preferences.whatsapp_alerts_enabled,
        extra_email: "",
      });

      setPreferences(updated);
      setNotification("Correo electrónico adicional eliminado.");

      getSecurityAlerts()
        .then((data) => {
          if (data?.alerts) setAlerts(data.alerts);
        })
        .catch(() => {});

      return true;
    } catch (err) {
      if (err instanceof SecurityAlertsApiError && err.status === 401) {
        await handleUnauthorized();
      }

      setError(
        err instanceof Error
          ? err.message
          : "No se pudo eliminar el correo adicional.",
      );
      return false;
    } finally {
      setIsSaving(false);
    }
  }, [preferences, handleUnauthorized]);

  const updateChannels = useCallback(
    async (
      input: Partial<UpdateSecurityAlertChannelsInput>,
    ): Promise<boolean> => {
      if (!preferences) return false;

      const app =
        input.app_notifications_enabled !== undefined
          ? input.app_notifications_enabled
          : preferences.app_notifications_enabled;
      const email =
        input.email_alerts_enabled !== undefined
          ? input.email_alerts_enabled
          : preferences.email_alerts_enabled;
      const whatsapp =
        input.whatsapp_alerts_enabled !== undefined
          ? input.whatsapp_alerts_enabled
          : preferences.whatsapp_alerts_enabled;

      try {
        setIsSaving(true);
        setError(null);
        setNotification(null);

        const updated = await updateSecurityAlertChannels({
          app_notifications_enabled: app,
          email_alerts_enabled: email,
          whatsapp_alerts_enabled: whatsapp,
          extra_email: input.extra_email,
        });

        setPreferences(updated);
        setNotification("Preferencias actualizadas exitosamente.");

        getSecurityAlerts()
          .then((data) => {
            if (data?.alerts) setAlerts(data.alerts);
          })
          .catch(() => {});

        return true;
      } catch (err) {
        if (err instanceof SecurityAlertsApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudieron actualizar las preferencias.",
        );
        return false;
      } finally {
        setIsSaving(false);
      }
    },
    [preferences, handleUnauthorized],
  );

  const clearNotification = useCallback(() => setNotification(null), []);
  const clearError = useCallback(() => setError(null), []);

  return {
    preferences,
    alerts,
    isLoading,
    isSaving,
    error,
    notification,
    refresh,
    toggleChannel,
    saveExtraEmail,
    removeExtraEmail,
    updateChannels,
    clearNotification,
    clearError,
  };
}
