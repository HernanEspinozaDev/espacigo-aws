"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  CheckCircle2,
  ChevronDown,
  Info,
  KeyRound,
  Loader2,
  Mail,
  ShieldCheck,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { ApiError, ApiProtocolError } from "@/lib/api/errors";
import { contractTemplateService } from "../contract-template-runtime";
import { commercialAuthorizationService } from "../commercial-authorization-runtime";
import {
  isCommercialAuthorization,
  type CommercialAuthorization,
  type CommercialAuthorizationEvent,
  type CommercialAuthorizationOTPChallenge,
} from "../domain/commercial-authorization";

interface Props {
  readonly spaceId: string;
}

function formatDateCl(date: Date): string {
  const day = String(date.getDate()).padStart(2, "0");
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const year = date.getFullYear();
  return `${day}-${month}-${year}`;
}

function formatRemainingValidity(expiresAt: string): string {
  const expiresMs = Date.parse(expiresAt);
  if (!Number.isFinite(expiresMs)) return "";

  const diffMs = expiresMs - Date.now();
  if (diffMs <= 0) {
    return "Expirada — vuelve a activarla";
  }

  const totalHours = Math.floor(diffMs / (60 * 60 * 1000));
  const days = Math.floor(totalHours / 24);
  const hours = totalHours % 24;

  if (days === 0) {
    return "Vence hoy";
  }

  const dayLabel = days === 1 ? "día" : "días";
  const hourLabel = hours === 1 ? "hora" : "horas";
  const hoursPart = hours > 0 ? ` y ${hours} ${hourLabel}` : "";
  return `Tiempo restante: ${days} ${dayLabel}${hoursPart} para renovar (vence el ${formatDateCl(new Date(expiresMs))})`;
}

function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "otp_invalid":
        return "El código de verificación es inválido o superó el máximo de intentos. Solicita uno nuevo.";
      case "otp_expired":
        return "El código de verificación expiró (5 minutos). Solicita un código nuevo.";
      case "otp_required":
        return "Ingresa el código de 6 dígitos enviado a tu correo antes de confirmar.";
    }
    if (error.status === 409) {
      return "La plantilla publicada cambió o ya existe una autorización vigente. Recarga el panel.";
    }
    if (error.status === 404) {
      return "No se encontró el espacio, la plantilla publicada o la autorización.";
    }
  }
  if (error instanceof ApiProtocolError) {
    return "No pudimos confirmar la respuesta del servidor. Recarga el panel e inténtalo otra vez.";
  }
  return "No se pudo completar la operación. Recarga el panel e inténtalo otra vez.";
}

export function CommercialAuthorizationPanel({ spaceId }: Props) {
  const [authorization, setAuthorization] =
    useState<CommercialAuthorization | null>(null);
  const [history, setHistory] = useState<CommercialAuthorizationEvent[]>([]);
  const [publishedVersion, setPublishedVersion] = useState(0);
  const [consent, setConsent] = useState(false);
  const [otpChallenge, setOtpChallenge] =
    useState<CommercialAuthorizationOTPChallenge | null>(null);
  const [otpCode, setOtpCode] = useState("");
  const [busyAction, setBusyAction] = useState<
    "request_otp" | "resend_otp" | "activate" | "revoke" | null
  >(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const [template, events, current] = await Promise.all([
          contractTemplateService.read(spaceId, signal),
          commercialAuthorizationService.history(spaceId, signal),
          commercialAuthorizationService
            .current(spaceId, signal)
            .catch((cause: unknown) => {
              if (cause instanceof ApiError && cause.status === 404)
                return null;
              throw cause;
            }),
        ]);
        setPublishedVersion(template.draft.published_version);
        setHistory(events);
        setAuthorization(
          current && isCommercialAuthorization(current) && !current.revoked_at
            ? current
            : null,
        );
      } catch (cause) {
        if (!signal?.aborted) setError(errorText(cause));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [spaceId],
  );

  useEffect(() => {
    const controller = new AbortController();
    const task = window.setTimeout(() => {
      void load(controller.signal);
    }, 0);
    return () => {
      window.clearTimeout(task);
      controller.abort();
    };
  }, [load]);

  const activeAuthorization =
    authorization && !authorization.revoked_at ? authorization : null;
  const busy = busyAction !== null;
  const otpCodeValid = /^[0-9]{6}$/.test(otpCode);

  async function requestOtpChallenge(isResend = false) {
    if (publishedVersion < 1 || !consent) {
      setError(
        "Publica una plantilla y acepta los términos para continuar con la activación.",
      );
      return;
    }
    setBusyAction(isResend ? "resend_otp" : "request_otp");
    setError("");
    setNotice("");
    try {
      const challenge =
        await commercialAuthorizationService.requestOtpChallenge(
          spaceId,
          publishedVersion,
        );
      setOtpChallenge(challenge);
      setOtpCode("");
      if (isResend) {
        setNotice(
          `Se envió un nuevo código de 6 dígitos a ${challenge.masked_email}.`,
        );
      }
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusyAction(null);
    }
  }

  async function confirmActivation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!otpChallenge) return;
    if (publishedVersion < 1 || !consent) {
      setError(
        "Publica una plantilla y acepta los términos antes de confirmar.",
      );
      return;
    }
    if (!otpCodeValid) {
      setError(
        "Ingresa el código de 6 dígitos enviado a tu correo antes de confirmar.",
      );
      return;
    }

    setBusyAction("activate");
    setError("");
    setNotice("");
    try {
      const result = await commercialAuthorizationService.activate(spaceId, {
        template_version: publishedVersion,
        consent: true,
        otp_challenge_id: otpChallenge.challenge_id,
        otp_code: otpCode,
      });
      setAuthorization(result);
      setOtpChallenge(null);
      setOtpCode("");
      setConsent(false);
      setNotice(
        "Firma automática activada por 3 meses y sin límites de monto para las reservas que usen la plantilla publicada.",
      );
      await load();
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusyAction(null);
    }
  }

  function cancelOtpStep() {
    if (busy) return;
    setOtpChallenge(null);
    setOtpCode("");
    setError("");
    setNotice("");
  }

  async function revoke() {
    if (!activeAuthorization) return;
    setBusyAction("revoke");
    setError("");
    setNotice("");
    try {
      await commercialAuthorizationService.revoke(
        spaceId,
        activeAuthorization.id,
        "owner_request",
      );
      setAuthorization(null);
      setNotice("Firma automática desactivada y registrada en el historial.");
      await load();
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusyAction(null);
    }
  }

  const remainingValidityText = activeAuthorization
    ? formatRemainingValidity(activeAuthorization.expires_at)
    : "";

  return (
    <Card className="mt-6" aria-labelledby="commercial-authorization-title">
      <CardHeader className="border-b border-border/60">
        <CardTitle
          id="commercial-authorization-title"
          className="flex items-center gap-2 text-lg font-semibold"
        >
          <ShieldCheck className="size-5 shrink-0" aria-hidden="true" />
          <span>Firma automática</span>
        </CardTitle>
        <CardAction>
          <span
            className={
              loading && !activeAuthorization
                ? "inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/60 px-2.5 py-1 text-xs font-medium whitespace-nowrap text-muted-foreground"
                : activeAuthorization
                  ? "inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium whitespace-nowrap text-emerald-800"
                  : "inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/40 px-2.5 py-1 text-xs font-medium whitespace-nowrap text-muted-foreground"
            }
          >
            <span
              className={
                loading && !activeAuthorization
                  ? "size-1.5 rounded-full bg-muted-foreground"
                  : activeAuthorization
                    ? "size-1.5 rounded-full bg-emerald-600"
                    : "size-1.5 rounded-full bg-muted-foreground"
              }
              aria-hidden="true"
            />
            {loading && !activeAuthorization
              ? "Cargando estado…"
              : activeAuthorization
                ? "Activa · Sin límites"
                : "Solo aprobación manual"}
          </span>
        </CardAction>
        <CardDescription>
          Cuando está activa, las solicitudes que usen la plantilla de contrato
          publicada registran automáticamente tu firma electrónica (FES) y pasan
          directo al pago.
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-5">
        {loading ? (
          <p
            role="status"
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            <span>Cargando estado privado…</span>
          </p>
        ) : null}

        {error ? (
          <div
            role="alert"
            className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"
          >
            {error}
          </div>
        ) : null}

        {notice ? (
          <div
            role="status"
            className="rounded-lg border border-emerald-200 bg-emerald-50/80 px-4 py-3 text-sm text-emerald-900"
          >
            {notice}
          </div>
        ) : null}

        {activeAuthorization ? (
          <div className="space-y-4">
            <div
              className="rounded-lg border border-emerald-200 bg-emerald-50/70 px-4 py-3 text-sm text-emerald-950"
              role="note"
            >
              <p className="flex items-start gap-2.5 font-semibold">
                <CheckCircle2
                  className="mt-0.5 size-4 shrink-0 text-emerald-700"
                  aria-hidden="true"
                />
                <span>La firma automática está activa y es sin límites.</span>
              </p>
              {remainingValidityText ? (
                <p className="mt-1.5 pl-6.5 font-medium text-emerald-900">
                  {remainingValidityText}
                </p>
              ) : null}
              <p className="mt-1 pl-6.5 text-emerald-900">
                Esta validación automática dura 3 meses.
              </p>
            </div>

            <div className="rounded-lg border border-border bg-muted/20 p-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="space-y-1">
                  <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Autorización vigente
                  </p>
                  <p className="text-sm font-semibold text-foreground">
                    Plantilla publicada: versión{" "}
                    {activeAuthorization.template_version} · Sin límites de
                    monto
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Publicar una nueva versión de la plantilla invalida esta
                    autorización y requiere volver a activarla.
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void revoke()}
                >
                  {busyAction === "revoke"
                    ? "Desactivando…"
                    : "Desactivar firma automática"}
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <div
              className="rounded-lg border border-border bg-muted/30 px-4 py-3 text-sm text-foreground"
              role="note"
            >
              <p className="flex items-start gap-2.5 font-medium">
                <Info
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
                <span>
                  Esta firma automática tiene una validez de 3 meses y es sin
                  límites de monto para las reservas que usen la plantilla
                  publicada.
                </span>
              </p>
            </div>

            <div
              className={
                !loading && publishedVersion < 1
                  ? "rounded-lg border border-amber-200 bg-amber-50/70 px-4 py-3 text-sm text-amber-950"
                  : "rounded-lg border border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground"
              }
            >
              <span className="font-medium text-foreground">
                Plantilla publicada:{" "}
                {publishedVersion > 0
                  ? `versión ${publishedVersion}`
                  : "ninguna"}
                .
              </span>{" "}
              Publicar otra versión invalida la autorización y exige volver a
              activarla.
            </div>

            <label className="flex items-start gap-2.5 rounded-lg border border-border bg-background px-4 py-3 text-sm">
              <input
                type="checkbox"
                className="mt-0.5 size-4 shrink-0"
                checked={consent}
                disabled={busy || loading || publishedVersion < 1}
                onChange={(event) => setConsent(event.target.checked)}
              />
              <span className="text-foreground">
                Acepto activar la firma automática y autorizo registrar
                automáticamente mi firma electrónica (FES) en los contratos de
                este espacio durante 3 meses.
              </span>
            </label>

            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">
                {publishedVersion < 1 && !loading
                  ? "Publica una versión de la plantilla para habilitar la activación."
                  : "Se enviará un código de verificación por correo para confirmar tu autorización."}
              </p>
              <Button
                type="button"
                disabled={
                  busy ||
                  loading ||
                  publishedVersion < 1 ||
                  !consent ||
                  otpChallenge !== null
                }
                onClick={() => void requestOtpChallenge(false)}
              >
                <Mail className="size-4" aria-hidden="true" />
                <span>
                  {busyAction === "request_otp"
                    ? "Enviando código…"
                    : "Activar firma automática"}
                </span>
              </Button>
            </div>

            {otpChallenge ? (
              <form
                onSubmit={confirmActivation}
                className="space-y-4 rounded-lg border border-border bg-muted/30 p-4"
              >
                <div className="space-y-1">
                  <p className="text-sm font-medium text-foreground">
                    Se envió un código de 6 dígitos a{" "}
                    <strong>{otpChallenge.masked_email}</strong>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Ingresa el código recibido en tu correo para activar la
                    firma automática por 3 meses.
                  </p>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="commercial-authorization-otp-code">
                    Código de verificación
                  </Label>
                  <div className="relative max-w-xs">
                    <KeyRound
                      className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                      aria-hidden="true"
                    />
                    <Input
                      id="commercial-authorization-otp-code"
                      type="text"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      maxLength={6}
                      value={otpCode}
                      disabled={busy}
                      onChange={(event) =>
                        setOtpCode(
                          event.target.value.replace(/\D/g, "").slice(0, 6),
                        )
                      }
                      placeholder="000000"
                      className="pl-9 tracking-widest"
                    />
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    type="submit"
                    disabled={
                      busy ||
                      loading ||
                      publishedVersion < 1 ||
                      !consent ||
                      !otpCodeValid
                    }
                  >
                    {busyAction === "activate"
                      ? "Confirmando…"
                      : "Confirmar y activar firma"}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={busy || loading || publishedVersion < 1}
                    onClick={() => void requestOtpChallenge(true)}
                  >
                    {busyAction === "resend_otp"
                      ? "Reenviando…"
                      : "Reenviar código"}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    disabled={busy}
                    onClick={cancelOtpStep}
                  >
                    Cancelar
                  </Button>
                </div>
              </form>
            ) : null}
          </div>
        )}

        <div className="border-t border-border pt-4">
          <details className="group rounded-lg border border-border bg-muted/15 px-4 py-3">
            <summary className="flex cursor-pointer list-none items-center justify-between gap-2 text-sm font-semibold select-none [&::-webkit-details-marker]:hidden">
              <span>Historial de autorización ({history.length})</span>
              <ChevronDown
                className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180"
                aria-hidden="true"
              />
            </summary>
            {history.length ? (
              <ul className="mt-3 divide-y divide-border border-t border-border pt-2 text-sm">
                {history.slice(0, 8).map((item) => (
                  <li
                    key={`${item.grant_id}-${item.action}-${item.occurred_at}`}
                    className="flex flex-wrap items-center justify-between gap-3 py-2 first:pt-1 last:pb-0"
                  >
                    <span className="font-medium text-foreground">
                      {item.action === "activated"
                        ? "Activada"
                        : item.action === "auto_confirmed"
                          ? "Confirmada automáticamente"
                          : "Revocada"}{" "}
                      <span className="font-normal text-muted-foreground">
                        · {item.reason}
                      </span>
                    </span>
                    <time
                      dateTime={item.occurred_at}
                      className="text-xs text-muted-foreground"
                    >
                      {new Date(item.occurred_at).toLocaleString("es-CL")}
                    </time>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-3 border-t border-border pt-3 text-sm text-muted-foreground">
                Sin eventos todavía.
              </p>
            )}
          </details>
        </div>
      </CardContent>
    </Card>
  );
}
