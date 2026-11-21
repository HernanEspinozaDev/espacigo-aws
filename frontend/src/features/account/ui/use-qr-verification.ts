"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountQRApproveResponse,
  type AccountQRDisableResponse,
  type AccountQRStatus,
  type AccountQRTicket,
  type AccountQRTicketStatus,
  QRApiError,
  approveAccountQRTicket,
  createAccountQRTicket,
  disableAccountQR,
  getAccountQRStatus,
  getAccountQRTicketStatus,
} from "../qr-verification-api";

export interface UseQRVerificationReturn {
  status: AccountQRStatus | null;
  activeTicket: AccountQRTicket | null;
  ticketStatus: AccountQRTicketStatus | null;
  isLoading: boolean;
  isGeneratingTicket: boolean;
  isApproving: boolean;
  isDisabling: boolean;
  error: string | null;
  notification: string | null;
  refresh: (signal?: AbortSignal) => Promise<void>;
  generateTicket: () => Promise<AccountQRTicket | null>;
  approveTicket: (
    token: string,
    deviceLabel?: string,
  ) => Promise<AccountQRApproveResponse | null>;
  disableQR: () => Promise<AccountQRDisableResponse | null>;
  cancelTicket: () => void;
  clearError: () => void;
  clearNotification: () => void;
}

export function useQRVerification(): UseQRVerificationReturn {
  const { checkSession } = useAuth();

  const [status, setStatus] = useState<AccountQRStatus | null>(null);
  const [activeTicket, setActiveTicket] = useState<AccountQRTicket | null>(
    null,
  );
  const [ticketStatus, setTicketStatus] =
    useState<AccountQRTicketStatus | null>(null);

  const [isLoading, setIsLoading] = useState(true);
  const [isGeneratingTicket, setIsGeneratingTicket] = useState(false);
  const [isApproving, setIsApproving] = useState(false);
  const [isDisabling, setIsDisabling] = useState(false);

  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const handleUnauthorized = useCallback(async () => {
    try {
      await checkSession();
    } catch {
      // ignore
    }
  }, [checkSession]);

  const clearError = useCallback(() => setError(null), []);
  const clearNotification = useCallback(() => setNotification(null), []);

  const cancelTicket = useCallback(() => {
    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current);
      pollIntervalRef.current = null;
    }
    setActiveTicket(null);
    setTicketStatus(null);
  }, []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await getAccountQRStatus(signal);
        setStatus(data);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof QRApiError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de verificación QR.",
        );
      } finally {
        setIsLoading(false);
      }
    },
    [handleUnauthorized],
  );

  // Initial load
  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    getAccountQRStatus(controller.signal)
      .then((data) => {
        if (!active) return;
        setStatus(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof QRApiError && err.status === 401) {
          handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de verificación QR.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const currentTicketStatus = ticketStatus?.status;

  // Polling effect when an active ticket is pending
  useEffect(() => {
    if (!activeTicket || currentTicketStatus !== "pending") {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
      return;
    }

    const ticketId = activeTicket.ticket_id;
    const expiresAtMs = new Date(activeTicket.expires_at).getTime();

    const checkTicket = async () => {
      if (Date.now() >= expiresAtMs) {
        setTicketStatus((prev) =>
          prev
            ? { ...prev, status: "expired" }
            : {
                ticket_id: ticketId,
                status: "expired",
                expires_at: activeTicket.expires_at,
              },
        );
        setError("El código QR ha expirado. Por favor, genera uno nuevo.");
        if (pollIntervalRef.current) {
          clearInterval(pollIntervalRef.current);
          pollIntervalRef.current = null;
        }
        return;
      }

      try {
        const result = await getAccountQRTicketStatus(ticketId);
        setTicketStatus(result);

        if (result.status === "approved") {
          if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
          // Refresh QR device status to reflect newly linked device
          try {
            const updated = await getAccountQRStatus();
            setStatus(updated);
          } catch {
            // ignore
          }
          setNotification(
            "¡Dispositivo de verificación QR vinculado y aprobado con éxito!",
          );
        } else if (
          result.status === "expired" ||
          result.status === "rejected"
        ) {
          if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
          if (result.status === "expired") {
            setError("El código QR ha expirado. Por favor, genera uno nuevo.");
          } else {
            setError("La verificación del código QR fue rechazada.");
          }
        }
      } catch (err) {
        if (err instanceof QRApiError && err.status === 401) {
          handleUnauthorized();
        }
      }
    };

    // Poll every 3 seconds
    pollIntervalRef.current = setInterval(checkTicket, 3000);

    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, [activeTicket, currentTicketStatus, handleUnauthorized]);

  const generateTicket =
    useCallback(async (): Promise<AccountQRTicket | null> => {
      try {
        setIsGeneratingTicket(true);
        setError(null);
        clearNotification();

        const ticket = await createAccountQRTicket();
        setActiveTicket(ticket);
        setTicketStatus({
          ticket_id: ticket.ticket_id,
          status: "pending",
          expires_at: ticket.expires_at,
        });
        return ticket;
      } catch (err) {
        if (err instanceof QRApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo generar el código QR. Inténtalo de nuevo.",
        );
        return null;
      } finally {
        setIsGeneratingTicket(false);
      }
    }, [clearNotification, handleUnauthorized]);

  const approveTicket = useCallback(
    async (
      token: string,
      deviceLabel?: string,
    ): Promise<AccountQRApproveResponse | null> => {
      try {
        setIsApproving(true);
        setError(null);
        clearNotification();

        const response = await approveAccountQRTicket({
          token,
          device_label: deviceLabel,
        });

        // Update local ticket state
        setTicketStatus((prev) =>
          prev
            ? {
                ...prev,
                status: "approved",
                resolved_at: response.resolved_at,
              }
            : null,
        );

        // Refresh overall QR status
        const updated = await getAccountQRStatus();
        setStatus(updated);

        setNotification(
          "¡Dispositivo de verificación QR vinculado y aprobado con éxito!",
        );
        return response;
      } catch (err) {
        if (err instanceof QRApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo aprobar el ticket de verificación QR.",
        );
        return null;
      } finally {
        setIsApproving(false);
      }
    },
    [clearNotification, handleUnauthorized],
  );

  const disableQR =
    useCallback(async (): Promise<AccountQRDisableResponse | null> => {
      try {
        setIsDisabling(true);
        setError(null);
        clearNotification();

        const response = await disableAccountQR();
        setStatus({
          enabled: false,
          enabled_at: null,
          trusted_device_label: null,
        });
        cancelTicket();
        setNotification("Verificación por código QR desactivada con éxito.");
        return response;
      } catch (err) {
        if (err instanceof QRApiError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo desactivar la verificación por código QR.",
        );
        return null;
      } finally {
        setIsDisabling(false);
      }
    }, [cancelTicket, clearNotification, handleUnauthorized]);

  return {
    status,
    activeTicket,
    ticketStatus,
    isLoading,
    isGeneratingTicket,
    isApproving,
    isDisabling,
    error,
    notification,
    refresh,
    generateTicket,
    approveTicket,
    disableQR,
    cancelTicket,
    clearError,
    clearNotification,
  };
}
