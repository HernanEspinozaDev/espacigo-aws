"use client";

import { KeyRound, Mail, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { apiClient } from "@/lib/api/client";
import {
  ApiError,
  ApiProtocolError,
  normalizeApiError,
} from "@/lib/api/errors";

export interface FesOtpPayload {
  challengeId: string;
  code: string;
}

function otpErrorMessage(
  error: unknown,
  action: "request" | "submit" = "request",
): string {
  if (error instanceof ApiError) {
    if (error.status === 401 || error.status === 403)
      return "Inicia sesión con la cuenta que participa en esta solicitud.";
    switch (error.code) {
      case "otp_invalid":
        return "El código de firma es inválido o superó el máximo de intentos. Solicita uno nuevo.";
      case "otp_expired":
        return "El código de firma expiró (5 minutos). Solicita un código nuevo.";
      case "otp_required":
        return "Ingresa el código de 6 dígitos enviado a tu correo antes de confirmar.";
      case "identity_required":
      case "identity_verification_required":
        return "Se requiere verificación de identidad vigente para firmar.";
      case "agreement_conflict":
        return "La cotización venció (6 minutos) o cambió el estado de la solicitud. Actualiza la cotización para continuar.";
      case "idempotency_conflict":
        return "Ya se procesó un intento previo con este envío. Actualiza el estado o solicita un nuevo código.";
      case "validation_error":
        return "Los datos de firma no son válidos. Revisa el código e inténtalo nuevamente.";
      case "proposal_changed":
      case "contract_terms_changed":
        return "Las condiciones cambiaron. Solicita una nueva cotización y revisa el acuerdo actualizado.";
      case "request_expired":
        return "La solicitud venció. Vuelve a cotizar para enviar un nuevo acuerdo.";
    }
    if (error.status === 404)
      return "La cotización o la solicitud ya no están disponibles para esta cuenta.";
  }
  if (error instanceof ApiProtocolError) {
    return "No pudimos confirmar la respuesta del servidor. Actualiza la página o intenta nuevamente.";
  }
  return action === "submit"
    ? "No pudimos confirmar la firma. Actualiza el estado o intenta nuevamente."
    : "No pudimos enviar el código de verificación. Intenta nuevamente.";
}

export function FesOtpSigningStep({
  proposalId,
  snapshotSha256,
  role,
  agreementId,
  consent,
  onConsentChange,
  consentText,
  busy,
  disabled,
  submitLabel,
  onSubmit,
  onRequestCode,
  extraActions,
}: {
  proposalId: string;
  snapshotSha256: string;
  role: "arrendatario" | "arrendador";
  agreementId?: string;
  consent: boolean;
  onConsentChange: (checked: boolean) => void;
  consentText: string;
  busy: boolean;
  disabled?: boolean;
  submitLabel: string;
  onSubmit: (otp: FesOtpPayload) => Promise<void>;
  onRequestCode?: () => void;
  extraActions?: React.ReactNode;
}) {
  const [challengeId, setChallengeId] = useState<string | null>(null);
  const [maskedEmail, setMaskedEmail] = useState("");
  const [code, setCode] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");

  async function requestCode() {
    if (sending || busy || disabled) return;
    setSending(true);
    setError("");
    onRequestCode?.();
    try {
      const result = await apiClient.POST(
        "/commercial-agreements/otp-challenge",
        {
          body: {
            proposal_id: proposalId,
            snapshot_sha256: snapshotSha256,
            role,
            ...(agreementId ? { agreement_id: agreementId } : {}),
          },
        },
      );
      if (!result.response.ok || !result.data) {
        throw normalizeApiError(result.response, result.error);
      }
      setChallengeId(result.data.challenge_id);
      setMaskedEmail(result.data.masked_email);
      setCode("");
    } catch (err) {
      setError(otpErrorMessage(err));
    } finally {
      setSending(false);
    }
  }

  const codeValid = /^[0-9]{6}$/.test(code.trim());

  return (
    <div className="space-y-3">
      <p className="rounded-lg border border-amber-200 bg-amber-50/70 px-3 py-2 text-xs text-amber-950">
        <strong>
          Nota informativa (Ley N.º 19.799 y Ley N.º 18.101 / SII):
        </strong>{" "}
        Este documento es un Contrato de Arrendamiento Simple firmado
        electrónicamente (FES). Si se requiere acreditar domicilio comercial
        ante el SII o ejercer el procedimiento monitorio del Art. 20 de la Ley
        N.º 18.101, deberá otorgarse o autorizarse ante Notario Público.
      </p>
      <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50/70 px-3 py-2 text-xs text-emerald-900">
        <ShieldCheck className="size-4 shrink-0" aria-hidden="true" />
        <span>
          Firma Electrónica Simple (FES) — Ley N.º 19.799 · Verificada con
          código único por correo y sellada criptográficamente en PDF.
        </span>
      </div>

      <label className="flex items-start gap-2.5 text-sm">
        <input
          className="mt-1 size-4"
          type="checkbox"
          checked={consent}
          disabled={busy || disabled}
          onChange={(e) => onConsentChange(e.target.checked)}
        />
        <span>{consentText}</span>
      </label>

      {error && (
        <p role="alert" className="text-xs font-medium text-destructive">
          {error}
        </p>
      )}

      {!challengeId ? (
        <div className="flex flex-wrap items-center justify-end gap-2">
          {extraActions}
          <Button
            type="button"
            disabled={!consent || sending || busy || disabled}
            onClick={() => void requestCode()}
          >
            <Mail className="size-4" aria-hidden="true" />
            <span>
              {sending
                ? "Enviando código…"
                : "Enviar código de firma al correo"}
            </span>
          </Button>
        </div>
      ) : (
        <div className="space-y-3 rounded-lg border border-border bg-muted/30 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
            <span className="font-medium text-foreground">
              Código enviado a <strong>{maskedEmail}</strong> (válido por 5 min)
            </span>
            <button
              type="button"
              disabled={sending || busy || disabled}
              onClick={() => void requestCode()}
              className="font-medium text-primary underline underline-offset-4 disabled:opacity-50"
            >
              {sending ? "Reenviando…" : "Reenviar código"}
            </button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative flex-1 min-w-[180px]">
              <KeyRound
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <input
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={code}
                disabled={busy || disabled}
                onChange={(e) =>
                  setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                }
                placeholder="Código de 6 dígitos"
                className="w-full rounded-lg border border-border bg-white py-2 pl-9 pr-3 text-sm tracking-widest outline-none focus:border-primary"
              />
            </div>
            {extraActions}
            <Button
              type="button"
              disabled={!consent || !codeValid || busy || disabled}
              onClick={() => {
                setError("");
                void onSubmit({ challengeId, code: code.trim() }).catch(
                  (err) => {
                    setError(otpErrorMessage(err, "submit"));
                  },
                );
              }}
            >
              {busy ? "Registrando firma…" : submitLabel}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
