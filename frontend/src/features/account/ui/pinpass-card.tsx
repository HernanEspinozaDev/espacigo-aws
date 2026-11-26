"use client";

import Link from "next/link";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { KeyRound, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useAuth } from "@/features/auth/ui/use-auth";
import { PinInput } from "./pin-input";
import { useSecurityMethodsSummary } from "./use-security-methods-summary";
import {
  getPinpassStatus,
  PinpassApiError,
  sendPinpassEmailCode,
  setPinpass,
  type PinpassProof,
} from "../pinpass-api";

function PinpassCard({
  reauthenticate,
}: {
  reauthenticate: (replacing: boolean) => Promise<PinpassProof>;
}) {
  const [configured, setConfigured] = useState<boolean | null>(null);
  const [pin, setPin] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [changing, setChanging] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [needsProfile, setNeedsProfile] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void getPinpassStatus(controller.signal)
      .then((status) => {
        if (!controller.signal.aborted) setConfigured(status.configured);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [attempt]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (
      busy ||
      configured === null ||
      !/^\d{6}$/.test(pin) ||
      pin !== confirmation
    )
      return;
    setBusy(true);
    setError(null);
    setNeedsProfile(false);
    try {
      const proof = await reauthenticate(configured);
      await setPinpass(pin, confirmation, proof);
      setMessage(
        configured
          ? "Tu Pinpass quedó actualizado."
          : "Tu Pinpass quedó configurado.",
      );
      setConfigured(true);
      setChanging(false);
      setPin("");
      setConfirmation("");
    } catch (cause) {
      setError(errorMessage(cause));
      setNeedsProfile(
        cause instanceof PinpassApiError && cause.code === "profile_incomplete",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card id="pinpass" className="scroll-mt-24">
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <KeyRound className="size-5" aria-hidden="true" />
          </div>
          <div>
            <CardTitle>
              {configured ? "Pinpass configurado" : "EspaciGo Pinpass"}
            </CardTitle>
            <CardDescription className="mt-1">
              Usa tu Pinpass para desbloquear opciones y consultar tus datos
              privados.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {message && (
          <p
            role="status"
            className="flex items-center gap-2 text-sm text-emerald-800 dark:text-emerald-300"
          >
            <ShieldCheck className="size-4" aria-hidden="true" />
            {message}
          </p>
        )}
        {needsProfile && (
          <Button asChild variant="outline">
            <Link href="/mi-cuenta/perfil">Completar mi perfil</Link>
          </Button>
        )}
        {configured === null ? (
          error ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setError(null);
                setAttempt(attempt + 1);
              }}
            >
              Reintentar
            </Button>
          ) : (
            <p
              role="status"
              className="flex items-center gap-2 text-sm text-muted-foreground"
            >
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Cargando EspaciGo Pinpass…
            </p>
          )
        ) : !configured || changing ? (
          <form onSubmit={save} className="space-y-4">
            <PinInput
              id="pinpass-new"
              label="Nuevo Pinpass"
              value={pin}
              onChange={setPin}
              disabled={busy}
            />
            <PinInput
              id="pinpass-confirm"
              label="Confirma tu Pinpass"
              value={confirmation}
              onChange={setConfirmation}
              disabled={busy}
            />
            <div className="flex flex-wrap gap-2">
              <Button
                type="submit"
                disabled={busy || !/^\d{6}$/.test(pin) || pin !== confirmation}
              >
                {configured ? "Actualizar Pinpass" : "Crear Pinpass"}
              </Button>
              {changing && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => {
                    setChanging(false);
                    setPin("");
                    setConfirmation("");
                    setError(null);
                  }}
                >
                  Cancelar
                </Button>
              )}
            </div>
          </form>
        ) : (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setMessage("");
              setChanging(true);
            }}
          >
            Cambiar Pinpass
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

/** Account-bound wrapper with an accessible password step-up dialog. */
export function PinpassAccountSection() {
  const { snapshot } = useAuth();
  const { summary } = useSecurityMethodsSummary();
  const otpAvailable = summary?.methods.totp.enabled === true;
  const [replacing, setReplacing] = useState(false);
  const [emailCode, setEmailCode] = useState("");
  const [sentTo, setSentTo] = useState("");
  const [mailError, setMailError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const emailRequest = useRef<AbortController | null>(null);
  const [otpCode, setOTPCode] = useState("");
  const [reauthOpen, setReauthOpen] = useState(false);
  const [password, setPassword] = useState("");
  const pending = useRef<{
    resolve: (proof: PinpassProof) => void;
    reject: (reason: Error) => void;
  } | null>(null);
  const sessionKey =
    snapshot.status === "authenticated" && snapshot.accountId
      ? `${snapshot.accountId}:${snapshot.expiresAt ?? "active"}`
      : "anonymous";

  useEffect(() => {
    if (typeof BroadcastChannel === "undefined") return;
    const channel = new BroadcastChannel("espacigo_auth");
    channel.onmessage = (event: MessageEvent<{ type?: string }>) => {
      if (
        event.data?.type !== "logout" &&
        event.data?.type !== "session_changed"
      )
        return;
      pending.current?.reject(
        new Error("La sesión cambió durante la confirmación."),
      );
      pending.current = null;
      emailRequest.current?.abort();
      setEmailCode("");
      setReauthOpen(false);
      setPassword("");
      setOTPCode("");
    };
    return () => {
      channel.close();
      emailRequest.current?.abort();
      pending.current?.reject(
        new Error("Se cerró la confirmación de identidad."),
      );
      pending.current = null;
    };
  }, []);

  const reauthenticate = useCallback(
    (isReplacing: boolean) =>
      new Promise<PinpassProof>((resolve, reject) => {
        pending.current = { resolve, reject };
        setReplacing(isReplacing);
        setSentTo("");
        setMailError(null);
        setSending(false);
        setEmailCode("");
        setPassword("");
        setOTPCode("");
        setReauthOpen(true);
      }),
    [],
  );

  const ready =
    /^\d{6}$/.test(emailCode) &&
    (replacing ? otpAvailable && /^\d{6}$/.test(otpCode) : password.length > 0);

  async function sendEmail() {
    const controller = new AbortController();
    emailRequest.current?.abort();
    emailRequest.current = controller;
    setSending(true);
    setMailError(null);
    try {
      const result = await sendPinpassEmailCode(controller.signal);
      if (!controller.signal.aborted) setSentTo(result.masked_email);
    } catch (error) {
      if (!controller.signal.aborted) setMailError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setSending(false);
    }
  }

  const submitProof = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || !pending.current) return;
    pending.current.resolve(
      replacing
        ? { email_code: emailCode, otp_code: otpCode }
        : { email_code: emailCode, current_password: password },
    );
    pending.current = null;
    handleDialogOpenChange(false);
  };

  const handleDialogOpenChange = (open: boolean) => {
    setReauthOpen(open);
    if (!open) {
      emailRequest.current?.abort();
      pending.current?.reject(
        new Error("Se canceló la confirmación de identidad."),
      );
      pending.current = null;
      setEmailCode("");
      setPassword("");
      setOTPCode("");
    }
  };

  if (snapshot.status !== "authenticated" || !snapshot.accountId) return null;
  return (
    <>
      <PinpassCard key={sessionKey} reauthenticate={reauthenticate} />
      <Dialog open={reauthOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirma tu identidad</DialogTitle>
            <DialogDescription>
              {replacing
                ? "Confirma el cambio con un código de tu correo y otro de tu aplicación de autenticación."
                : "Confirma tu primera configuración con tu contraseña y un código enviado al correo de tu cuenta."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submitProof} className="space-y-4">
            {replacing ? (
              <div className="space-y-2">
                <Label htmlFor="pinpass-otp">Código del autenticador</Label>
                <Input
                  id="pinpass-otp"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={otpCode}
                  disabled={!otpAvailable}
                  onChange={(event) =>
                    setOTPCode(event.target.value.replace(/\D/g, ""))
                  }
                />
                {!otpAvailable && (
                  <Link
                    className="text-sm font-medium text-primary underline"
                    href="/mi-cuenta/seguridad/metodos/google-authenticator"
                  >
                    Configurar una aplicación de autenticación
                  </Link>
                )}
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="pinpass-current-password">
                  Contraseña actual
                </Label>
                <Input
                  id="pinpass-current-password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  maxLength={72}
                />
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="pinpass-email-code">
                Código de correo electrónico
              </Label>
              <div className="flex flex-wrap gap-2">
                <Input
                  id="pinpass-email-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  className="min-w-24 flex-1"
                  value={emailCode}
                  onChange={(event) =>
                    setEmailCode(event.target.value.replace(/\D/g, ""))
                  }
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void sendEmail()}
                  disabled={sending || (replacing && !otpAvailable)}
                >
                  {sending
                    ? "Enviando…"
                    : sentTo
                      ? "Reenviar código"
                      : "Enviar código"}
                </Button>
              </div>
              {sentTo && (
                <p role="status" className="text-sm text-muted-foreground">
                  Enviamos un código a {sentTo}.
                </p>
              )}
              {mailError && (
                <p role="alert" className="text-sm text-destructive">
                  {mailError}
                </p>
              )}
            </div>
            <DialogFooter>
              <Button type="submit" disabled={!ready}>
                Confirmar
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

function errorMessage(cause: unknown): string {
  if (cause instanceof Error && cause.message) return cause.message;
  return "No se pudo completar la operación. Inténtalo nuevamente.";
}
