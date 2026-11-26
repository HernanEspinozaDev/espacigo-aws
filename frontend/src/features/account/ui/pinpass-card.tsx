"use client";

import Link from "next/link";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { Eye, EyeOff, KeyRound, Loader2, ShieldCheck } from "lucide-react";
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
import { useSecurityMethodsSummary } from "./use-security-methods-summary";
import {
  getPinpassStatus,
  PinpassApiError,
  PINPASS_SETTINGS_HREF,
  revealPinpassValue,
  setPinpass,
  unlockPinpass,
  type PinpassProof,
  type PinpassStatus,
} from "../pinpass-api";

interface PinpassCardProps {
  readonly manage: boolean;
  /** Requests an account factor for reauthentication. */
  readonly reauthenticate: () => Promise<PinpassProof>;
  /** Change this when the authenticated account/session changes to clear local values. */
  readonly sessionKey: string;
}

const sensitiveFields = {
  legal_name: "Nombre legal",
  date_of_birth: "Fecha de nacimiento",
} as const;

type SensitiveField = keyof typeof sensitiveFields;
const sensitiveFieldKeys = Object.keys(sensitiveFields) as SensitiveField[];

function PinpassCard({ manage, reauthenticate, sessionKey }: PinpassCardProps) {
  const [status, setStatus] = useState<PinpassStatus | null>(null);
  const [pin, setPin] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [values, setValues] = useState<Partial<Record<SensitiveField, string>>>(
    {},
  );
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [isAccessActive, setIsAccessActive] = useState(false);
  const [isBlocked, setIsBlocked] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [showSetup, setShowSetup] = useState(false);
  const [needsProfile, setNeedsProfile] = useState(false);
  const [requestedField, setRequestedField] = useState<SensitiveField | null>(
    null,
  );
  const clearTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const blockTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const accessExpiry = useRef<number | null>(null);

  const clearSensitiveValues = useCallback(() => {
    setValues({});
    setIsAccessActive(false);
    accessExpiry.current = null;
    if (clearTimer.current) clearTimeout(clearTimer.current);
    clearTimer.current = null;
  }, []);

  const acceptExpiry = useCallback(
    (expiresAt: string) => {
      const deadline = Date.parse(expiresAt);
      if (!Number.isFinite(deadline) || deadline <= Date.now()) {
        clearSensitiveValues();
        return false;
      }
      accessExpiry.current = deadline;
      setIsAccessActive(true);
      if (clearTimer.current) clearTimeout(clearTimer.current);
      clearTimer.current = setTimeout(
        clearSensitiveValues,
        deadline - Date.now(),
      );
      return true;
    },
    [clearSensitiveValues],
  );

  useEffect(() => {
    const controller = new AbortController();
    getPinpassStatus(controller.signal)
      .then((next) => {
        setStatus(next);
        if (next.access_expires_at) acceptExpiry(next.access_expires_at);
        const remaining = next.blocked_until
          ? Date.parse(next.blocked_until) - Date.now()
          : 0;
        setIsBlocked(remaining > 0);
        if (blockTimer.current) clearTimeout(blockTimer.current);
        if (remaining > 0)
          blockTimer.current = setTimeout(() => setIsBlocked(false), remaining);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      controller.abort();
      clearSensitiveValues();
      if (blockTimer.current) clearTimeout(blockTimer.current);
    };
  }, [sessionKey, loadAttempt, acceptExpiry, clearSensitiveValues]);

  useEffect(() => {
    const expireIfNeeded = () => {
      if (accessExpiry.current !== null && Date.now() >= accessExpiry.current) {
        clearSensitiveValues();
        setStatus((current) =>
          current ? { ...current, access_expires_at: null } : current,
        );
      }
    };
    const hideOnBackground = () => {
      if (document.visibilityState !== "visible") clearSensitiveValues();
      else expireIfNeeded();
    };
    document.addEventListener("visibilitychange", hideOnBackground);
    window.addEventListener("focus", expireIfNeeded);
    return () => {
      document.removeEventListener("visibilitychange", hideOnBackground);
      window.removeEventListener("focus", expireIfNeeded);
    };
  }, [clearSensitiveValues]);

  useEffect(() => {
    if (typeof BroadcastChannel === "undefined") return;
    const channel = new BroadcastChannel("espacigo_auth");
    channel.onmessage = (event: MessageEvent<{ type?: string }>) => {
      if (
        event.data?.type !== "logout" &&
        event.data?.type !== "session_changed"
      )
        return;
      clearSensitiveValues();
      setStatus(null);
      setIsBlocked(false);
      if (event.data.type === "session_changed") {
        setError(null);
        setLoading(true);
        setLoadAttempt((attempt) => attempt + 1);
      }
    };
    return () => channel.close();
  }, [clearSensitiveValues]);

  const submitSetup = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setMessage(null);
    setNeedsProfile(false);
    if (!/^\d{6}$/.test(pin) || pin !== confirmation) {
      setError("Usa seis dígitos y confirma el mismo Pinpass.");
      return;
    }
    setBusy(true);
    try {
      const proof = await reauthenticate();
      await setPinpass(pin, confirmation, proof);
      setPin("");
      setConfirmation("");
      clearSensitiveValues();
      setStatus({
        configured: true,
        blocked_until: null,
        access_expires_at: null,
      });
      setShowSetup(false);
      setMessage(
        status?.configured
          ? "Tu Pinpass quedó actualizado."
          : "Tu Pinpass quedó configurado.",
      );
    } catch (cause) {
      setNeedsProfile(
        cause instanceof PinpassApiError && cause.code === "profile_incomplete",
      );
      setError(errorMessage(cause));
    } finally {
      setPin("");
      setConfirmation("");
      setBusy(false);
    }
  };

  useEffect(() => {
    if (requestedField && !isBlocked)
      document.getElementById("pinpass-unlock")?.focus();
  }, [requestedField, isBlocked]);

  const unlock = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setMessage(null);
    setBusy(true);
    try {
      const result = await unlockPinpass(pin);
      setPin("");
      if (!acceptExpiry(result.access_expires_at))
        throw new Error("La autorización venció. Inténtalo nuevamente.");
      setStatus((current) =>
        current
          ? { ...current, access_expires_at: result.access_expires_at }
          : current,
      );
      setMessage("Acceso temporal activado por 15 minutos.");
      if (requestedField) {
        setRequestedField(null);
        await toggleField(requestedField);
      }
    } catch (cause) {
      setError(errorMessage(cause));
      if (cause instanceof PinpassApiError && cause.status === 429) {
        const seconds = Number(cause.retryAfter) || 60;
        setIsBlocked(true);
        if (blockTimer.current) clearTimeout(blockTimer.current);
        blockTimer.current = setTimeout(
          () => setIsBlocked(false),
          seconds * 1000,
        );
      }
    } finally {
      setPin("");
      setBusy(false);
    }
  };

  const retryLoad = () => {
    setStatus(null);
    setError(null);
    setLoading(true);
    setLoadAttempt((attempt) => attempt + 1);
  };

  const toggleField = async (field: SensitiveField) => {
    if (values[field] !== undefined) {
      setValues((current) => ({ ...current, [field]: undefined }));
      return;
    }
    const deadline = accessExpiry.current;
    if (deadline === null) {
      clearSensitiveValues();
      setStatus((current) =>
        current ? { ...current, access_expires_at: null } : current,
      );
      setRequestedField(field);
      document.getElementById("pinpass-unlock")?.focus();
      return;
    }
    const controller = new AbortController();
    setBusy(true);
    setError(null);
    try {
      const result = await revealPinpassValue(field, controller.signal);
      if (
        accessExpiry.current !== deadline ||
        Date.parse(result.access_expires_at) !== deadline ||
        !acceptExpiry(result.access_expires_at)
      ) {
        clearSensitiveValues();
        return;
      }
      setValues((current) => ({ ...current, [field]: result.value }));
    } catch (cause) {
      setError(errorMessage(cause));
      if (
        cause instanceof PinpassApiError &&
        cause.code === "pinpass_access_required"
      ) {
        clearSensitiveValues();
        setStatus((current) =>
          current ? { ...current, access_expires_at: null } : current,
        );
      }
    } finally {
      setBusy(false);
    }
  };

  if (!status)
    return (
      <Card id="pinpass" className="scroll-mt-24">
        <CardContent className="space-y-3 p-6">
          {loading ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Cargando EspaciGo Pinpass…
            </p>
          ) : (
            <>
              <p role="alert" className="text-sm text-destructive">
                {error ?? "No se pudo cargar EspaciGo Pinpass."}
              </p>
              <Button type="button" variant="outline" onClick={retryLoad}>
                Reintentar
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    );

  const isUnlocked = Boolean(status.access_expires_at && isAccessActive);
  return (
    <Card id="pinpass" className="scroll-mt-24">
      <CardHeader>
        <div className="flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <KeyRound className="size-5" aria-hidden="true" />
          </div>
          <div>
            <CardTitle>
              {manage
                ? status.configured
                  ? "Pinpass configurado"
                  : "Configurar Pinpass"
                : "Datos privados"}
            </CardTitle>
            <CardDescription className="mt-1">
              {manage
                ? "Usa tu Pinpass para navegar y desbloquear opciones dentro de EspaciGo."
                : "Ingresa tu Pinpass para ver tus datos privados."}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {error && (
          <p
            role="alert"
            className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
          >
            {error}
          </p>
        )}
        {message && (
          <p
            role="status"
            className="rounded-md border border-emerald-600/20 bg-emerald-600/10 p-3 text-sm"
          >
            {message}
          </p>
        )}
        {needsProfile && (
          <Button asChild variant="outline">
            <Link href="/mi-cuenta/perfil">Completar mi perfil</Link>
          </Button>
        )}
        {manage && (!status.configured || showSetup) ? (
          <form onSubmit={submitSetup} className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {status.configured
                ? "Elige un nuevo código. Se cerrará cualquier acceso temporal activo."
                : "Crea tu código para consultar tus datos personales."}{" "}
              Confirma tu identidad antes de guardar.
            </p>
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
                {busy && (
                  <Loader2
                    className="mr-2 size-4 animate-spin"
                    aria-hidden="true"
                  />
                )}
                {status.configured ? "Actualizar Pinpass" : "Crear Pinpass"}
              </Button>
              {status.configured && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setShowSetup(false);
                    setPin("");
                    setConfirmation("");
                  }}
                >
                  Cancelar
                </Button>
              )}
            </div>
          </form>
        ) : manage ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setMessage(null);
              setShowSetup(true);
            }}
          >
            Cambiar Pinpass
          </Button>
        ) : !status.configured ? (
          <Button asChild>
            <Link href={PINPASS_SETTINGS_HREF}>Configurar Pinpass</Link>
          </Button>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
              <div className="flex items-center gap-2 text-sm">
                <ShieldCheck
                  className="size-4 text-emerald-700"
                  aria-hidden="true"
                />
                <span>
                  {isUnlocked
                    ? "Acceso temporal activo"
                    : "Tus datos están protegidos"}
                </span>
              </div>
              {isUnlocked && (
                <span className="text-xs text-muted-foreground">
                  Hasta{" "}
                  {new Date(status.access_expires_at!).toLocaleTimeString(
                    "es-CL",
                    { hour: "2-digit", minute: "2-digit" },
                  )}
                </span>
              )}
            </div>
            <div className="space-y-3">
              {sensitiveFieldKeys.map((field) => (
                <SensitiveRow
                  key={field}
                  label={sensitiveFields[field]}
                  value={values[field]}
                  onToggle={() => void toggleField(field)}
                  disabled={busy}
                />
              ))}
            </div>
            {!isUnlocked && requestedField ? (
              <form onSubmit={unlock} className="space-y-3">
                <PinInput
                  id="pinpass-unlock"
                  label="Ingresa tu Pinpass"
                  value={pin}
                  onChange={setPin}
                  disabled={busy || isBlocked}
                />
                <Button
                  type="submit"
                  disabled={busy || !/^\d{6}$/.test(pin) || isBlocked}
                >
                  {busy && (
                    <Loader2
                      className="mr-2 size-4 animate-spin"
                      aria-hidden="true"
                    />
                  )}
                  Verificar Pinpass
                </Button>
              </form>
            ) : null}
            {isUnlocked && (
              <p className="text-xs text-muted-foreground">
                La autorización dura 15 minutos desde que ingresaste el Pinpass.
              </p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/** Account-bound wrapper with an accessible password step-up dialog. */
export function PinpassAccountSection({
  manage = false,
}: {
  manage?: boolean;
}) {
  const { snapshot } = useAuth();
  const { summary } = useSecurityMethodsSummary();
  const otpAvailable = summary?.methods.totp.enabled !== false;
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
      setReauthOpen(false);
      setPassword("");
      setOTPCode("");
    };
    return () => {
      channel.close();
      pending.current?.reject(
        new Error("Se cerró la confirmación de identidad."),
      );
      pending.current = null;
    };
  }, []);

  const reauthenticate = useCallback(
    () =>
      new Promise<PinpassProof>((resolve, reject) => {
        pending.current = { resolve, reject };
        setPassword("");
        setOTPCode("");
        setReauthOpen(true);
      }),
    [],
  );

  const hasValidOTP = /^\d{6}$/.test(otpCode);
  const ready =
    (password.length > 0 || (otpAvailable && hasValidOTP)) &&
    (!otpAvailable || otpCode === "" || hasValidOTP);
  const submitProof = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || !pending.current) return;
    const code = otpAvailable ? otpCode : "";
    pending.current.resolve(
      password
        ? { current_password: password, ...(code ? { otp_code: code } : {}) }
        : { otp_code: code },
    );
    pending.current = null;
    setPassword("");
    setOTPCode("");
    setReauthOpen(false);
  };

  const handleDialogOpenChange = (open: boolean) => {
    setReauthOpen(open);
    if (!open && pending.current) {
      pending.current.reject(
        new Error("Se canceló la confirmación de identidad."),
      );
      pending.current = null;
      setPassword("");
      setOTPCode("");
    }
  };

  if (snapshot.status !== "authenticated" || !snapshot.accountId) return null;
  return (
    <>
      <PinpassCard
        key={sessionKey}
        manage={manage}
        sessionKey={sessionKey}
        reauthenticate={reauthenticate}
      />
      <Dialog open={reauthOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirma tu identidad</DialogTitle>
            <DialogDescription>
              {otpAvailable
                ? "Usa tu contraseña actual, un código de tu autenticador o ambos para confirmar el cambio."
                : "Usa tu contraseña actual para confirmar el cambio. Puedes activar un autenticador en Seguridad."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submitProof} className="space-y-4">
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
            {otpAvailable ? (
              <div className="space-y-2">
                <Label htmlFor="pinpass-otp">Código del autenticador</Label>
                <Input
                  id="pinpass-otp"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={otpCode}
                  onChange={(event) =>
                    setOTPCode(event.target.value.replace(/\D/g, ""))
                  }
                  aria-describedby="pinpass-otp-hint"
                />
                <p
                  id="pinpass-otp-hint"
                  className="text-xs text-muted-foreground"
                >
                  Ingresa los seis dígitos de tu autenticador TOTP activo.
                </p>
              </div>
            ) : null}
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

function PinInput({
  id,
  label,
  value,
  onChange,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>
        {label} <span className="text-muted-foreground">(6 dígitos)</span>
      </Label>
      <div className="relative flex max-w-xs gap-2 rounded-md focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2">
        {Array.from({ length: 6 }, (_, index) => (
          <span
            key={index}
            aria-hidden="true"
            className="flex size-11 items-center justify-center rounded-md border bg-background text-lg font-semibold"
          >
            {value[index] ? "•" : ""}
          </span>
        ))}
        <Input
          id={id}
          type="password"
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="[0-9]{6}"
          maxLength={6}
          value={value}
          onChange={(event) =>
            onChange(event.target.value.replace(/\D/g, "").slice(0, 6))
          }
          disabled={disabled}
          className="absolute inset-0 z-10 h-full w-full cursor-text opacity-0"
          aria-describedby={`${id}-hint`}
          aria-label={label}
          required
        />
      </div>
      <p id={`${id}-hint`} className="text-xs text-muted-foreground">
        Escribe o pega los seis dígitos. El código no se guarda en este
        dispositivo.
      </p>
    </div>
  );
}

function SensitiveRow({
  label,
  value,
  onToggle,
  disabled,
}: {
  label: string;
  value?: string;
  onToggle: () => void;
  disabled: boolean;
}) {
  const visible = value !== undefined;
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-medium">
          {visible ? value : "••••••••••••"}
        </p>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onToggle}
        disabled={disabled}
        aria-label={`${visible ? "Ocultar" : "Mostrar"} ${label}`}
        aria-pressed={visible}
      >
        {visible ? (
          <EyeOff className="size-4" aria-hidden="true" />
        ) : (
          <Eye className="size-4" aria-hidden="true" />
        )}
      </Button>
    </div>
  );
}

function errorMessage(cause: unknown): string {
  if (cause instanceof Error && cause.message) return cause.message;
  return "No se pudo completar la operación. Inténtalo nuevamente.";
}
