"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  type AccountPhoneStatus,
  getAccountPhoneStatus,
  sendAccountPhoneCode,
  verifyAccountPhoneCode,
} from "../phone-verification-api";

function calculateRemainingSeconds(resendAvailableAt?: string | null): number {
  if (!resendAvailableAt) return 0;
  const targetTime = new Date(resendAvailableAt).getTime();
  const diff = targetTime - Date.now();
  return diff > 0 ? Math.ceil(diff / 1000) : 0;
}

export function usePhoneVerification() {
  const [status, setStatus] = useState<AccountPhoneStatus | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [cooldownSeconds, setCooldownSeconds] = useState(0);

  const activeResendTargetRef = useRef<string | null>(null);

  // Sync cooldown timer
  useEffect(() => {
    const target = activeResendTargetRef.current;
    if (!target) return;

    const timer = setInterval(() => {
      const remaining = calculateRemainingSeconds(target);
      setCooldownSeconds(remaining);
      if (remaining <= 0) {
        clearInterval(timer);
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [status?.resend_available_at]);

  const updateResendTarget = useCallback((targetIso?: string | null) => {
    activeResendTargetRef.current = targetIso ?? null;
    setCooldownSeconds(calculateRemainingSeconds(targetIso));
  }, []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        const data = await getAccountPhoneStatus(signal);
        setStatus(data);
        if (data.resend_available_at) {
          updateResendTarget(data.resend_available_at);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de verificación telefónica.",
        );
      } finally {
        setIsLoading(false);
      }
    },
    [updateResendTarget],
  );

  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    getAccountPhoneStatus(controller.signal)
      .then((data) => {
        if (!active) return;
        setStatus(data);
        if (data.resend_available_at) {
          activeResendTargetRef.current = data.resend_available_at;
          setCooldownSeconds(
            calculateRemainingSeconds(data.resend_available_at),
          );
        }
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar el estado de verificación telefónica.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, []);

  const sendCode = useCallback(
    async (phone: string): Promise<boolean> => {
      try {
        setIsSending(true);
        setError(null);
        setSuccess(null);
        const result = await sendAccountPhoneCode(phone);
        updateResendTarget(result.resend_available_at);
        setStatus((prev) => ({
          configured: true,
          verified: false,
          masked_phone: result.masked_phone,
          verified_at: prev?.verified_at ?? null,
          pending_verification: true,
          resend_available_at: result.resend_available_at,
        }));
        setSuccess("Código de verificación enviado.");
        return true;
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError")
          return false;
        if (err instanceof Error && err.name === "AbortError") return false;
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo enviar el código de verificación.",
        );
        return false;
      } finally {
        setIsSending(false);
      }
    },
    [updateResendTarget],
  );

  const verifyCode = useCallback(
    async (code: string): Promise<boolean> => {
      try {
        setIsVerifying(true);
        setError(null);
        setSuccess(null);
        const result = await verifyAccountPhoneCode(code);
        setStatus({
          configured: true,
          verified: result.verified,
          masked_phone: result.masked_phone,
          verified_at: result.verified_at,
          pending_verification: false,
          resend_available_at: null,
        });
        updateResendTarget(null);
        setSuccess("Teléfono verificado con éxito.");
        return true;
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError")
          return false;
        if (err instanceof Error && err.name === "AbortError") return false;
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo verificar el código.",
        );
        return false;
      } finally {
        setIsVerifying(false);
      }
    },
    [updateResendTarget],
  );

  const clearError = useCallback(() => setError(null), []);

  return {
    status,
    setStatus,
    isLoading,
    isSending,
    isVerifying,
    error,
    clearError,
    success,
    cooldownSeconds,
    canResend: cooldownSeconds <= 0,
    sendCode,
    verifyCode,
    refresh,
  };
}
