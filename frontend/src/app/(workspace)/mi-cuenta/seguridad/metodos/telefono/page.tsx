"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  ExternalLink,
  Info,
  Loader2,
  Mail,
  PenLine,
  RefreshCw,
  Send,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePhoneVerification } from "@/features/account/ui/use-phone-verification";

export default function TelefonoMetodoPage() {
  const {
    status,
    isLoading,
    isSending,
    isVerifying,
    error,
    clearError,
    success,
    cooldownSeconds,
    canResend,
    sendCode,
    verifyCode,
  } = usePhoneVerification();

  const [phoneInput, setPhoneInput] = useState("");
  const [codeInput, setCodeInput] = useState("");
  const [isChanging, setIsChanging] = useState(false);
  const [stepOverride, setStepOverride] = useState<"phone" | "code" | null>(
    null,
  );

  const handleSendCode = async (e: FormEvent) => {
    e.preventDefault();
    if (!phoneInput.trim()) return;
    const ok = await sendCode(phoneInput.trim());
    if (ok) {
      setCodeInput("");
      setStepOverride("code");
    }
  };

  const handleResendCode = async () => {
    if (!canResend || isSending) return;
    const targetPhone = phoneInput.trim() || status?.masked_phone || "";
    await sendCode(targetPhone);
  };

  const handleVerifyCode = async (e: FormEvent) => {
    e.preventDefault();
    if (codeInput.trim().length !== 6) return;
    const ok = await verifyCode(codeInput.trim());
    if (ok) {
      setCodeInput("");
      setPhoneInput("");
      setIsChanging(false);
      setStepOverride(null);
    }
  };

  const isPendingStep =
    stepOverride === "code" ||
    (status?.pending_verification && stepOverride !== "phone");

  const isVerifiedView = status?.verified && !isChanging && !isPendingStep;

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground flex-wrap"
      >
        <Link
          href="/mi-cuenta"
          className="hover:text-primary transition-colors font-medium"
        >
          Mi perfil
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad"
          className="hover:text-primary transition-colors font-medium"
        >
          Seguridad
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad/metodos"
          className="hover:text-primary transition-colors font-medium"
        >
          Métodos de verificación
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="text-foreground font-semibold">Teléfono</span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Teléfono y SMS
        </h1>
        <p className="text-sm text-muted-foreground">
          Gestiona tu número celular para recibir códigos de seguridad por
          mensaje de texto.
        </p>
      </div>

      {/* Alertas de error o éxito */}
      {error && (
        <div
          role="alert"
          className="p-4 rounded-2xl bg-destructive/10 border border-destructive/20 text-destructive text-sm flex items-start gap-3"
        >
          <AlertCircle className="size-5 shrink-0 mt-0.5" />
          <div className="flex-1">
            <p className="font-semibold">{error}</p>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="text-xs underline hover:no-underline font-medium"
          >
            Cerrar
          </button>
        </div>
      )}

      {success && !error && (
        <div
          role="status"
          className="p-4 rounded-2xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-sm flex items-start gap-3"
        >
          <CheckCircle2 className="size-5 shrink-0 mt-0.5 text-emerald-600" />
          <p className="font-medium">{success}</p>
        </div>
      )}

      {isLoading ? (
        <div className="bg-card border border-border rounded-2xl p-12 flex flex-col items-center justify-center gap-3">
          <Loader2 className="size-8 text-primary animate-spin" />
          <p className="text-sm text-muted-foreground">
            Cargando estado de verificación...
          </p>
        </div>
      ) : isVerifiedView ? (
        /* Tarjeta de estado Verificado (Estilo Mercado Pago) */
        <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 border-b border-border/60">
            <div className="flex items-center gap-4">
              <div className="size-12 rounded-2xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                <Smartphone className="size-6" />
              </div>
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-bold text-foreground">
                    Número verificado
                  </h2>
                  <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                    <CheckCircle2 className="size-3.5" />
                    Verificado
                  </span>
                </div>
                <p className="text-base font-mono tracking-wider text-foreground font-semibold">
                  {status?.masked_phone || "+56 •••• 5678"}
                </p>
                {status?.verified_at && (
                  <p className="text-xs text-muted-foreground">
                    Verificado el{" "}
                    {new Date(status.verified_at).toLocaleDateString("es-CL", {
                      day: "numeric",
                      month: "long",
                      year: "numeric",
                    })}
                  </p>
                )}
              </div>
            </div>

            <div>
              <button
                type="button"
                onClick={() => {
                  clearError();
                  setIsChanging(true);
                }}
                className="inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 font-semibold text-xs sm:text-sm transition-colors w-full sm:w-auto shadow-sm"
              >
                <PenLine className="size-4" />
                Cambiar número
              </button>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs sm:text-sm text-muted-foreground">
            <div className="flex items-start gap-3">
              <ShieldCheck className="size-5 text-emerald-600 shrink-0 mt-0.5" />
              <div>
                <strong className="text-foreground font-medium block">
                  Seguridad y cifrado A.8.11
                </strong>
                <span>
                  Tu número de teléfono está enmascarado y cifrado en
                  PostgreSQL.
                </span>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <Info className="size-5 text-primary shrink-0 mt-0.5" />
              <div>
                <strong className="text-foreground font-medium block">
                  Uso en verificación
                </strong>
                <span>
                  Recibirás códigos OTP temporales de 6 dígitos para validar
                  operaciones y accesos sensibles.
                </span>
              </div>
            </div>
          </div>
        </div>
      ) : (
        /* Formulario Paso 1 o Paso 2 */
        <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
          {!isPendingStep ? (
            /* Paso 1: Ingreso de número de celular */
            <form onSubmit={handleSendCode} className="space-y-6 max-w-xl">
              <div className="space-y-1">
                <h2 className="text-lg font-bold text-foreground">
                  {isChanging
                    ? "Cambiar número de teléfono"
                    : "Registrar número de teléfono"}
                </h2>
                <p className="text-xs sm:text-sm text-muted-foreground">
                  Ingresa tu número de teléfono móvil chileno. Te enviaremos un
                  código de seguridad por mensaje de texto.
                </p>
              </div>

              <div className="space-y-2">
                <Label
                  htmlFor="phone-number"
                  className="text-xs sm:text-sm font-semibold text-foreground"
                >
                  Número de teléfono celular
                </Label>
                <Input
                  id="phone-number"
                  type="tel"
                  autoComplete="tel"
                  placeholder="+56 9 1234 5678"
                  value={phoneInput}
                  onChange={(e) => setPhoneInput(e.target.value)}
                  className="font-mono text-sm"
                  disabled={isSending}
                  required
                />
                <p className="text-xs text-muted-foreground">
                  Formato: +56 9 XXXXXXXX o 9XXXXXXXX (9 dígitos).
                </p>
              </div>

              <div className="flex flex-col sm:flex-row items-center gap-3 pt-2">
                <button
                  type="submit"
                  disabled={isSending || !phoneInput.trim()}
                  className="inline-flex items-center justify-center gap-2 px-6 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50 font-semibold text-xs sm:text-sm transition-colors w-full sm:w-auto shadow-sm"
                >
                  {isSending ? (
                    <>
                      <Loader2 className="size-4 animate-spin" />
                      Enviando código...
                    </>
                  ) : (
                    <>
                      <Send className="size-4" />
                      Enviar código por mensaje
                    </>
                  )}
                </button>

                {isChanging && (
                  <button
                    type="button"
                    onClick={() => {
                      clearError();
                      setIsChanging(false);
                    }}
                    className="inline-flex items-center justify-center px-4 py-2.5 rounded-xl border border-border text-foreground hover:bg-muted font-medium text-xs sm:text-sm transition-colors w-full sm:w-auto"
                  >
                    Cancelar
                  </button>
                )}
              </div>
            </form>
          ) : (
            /* Paso 2: Verificación de código OTP de 6 dígitos */
            <form onSubmit={handleVerifyCode} className="space-y-6 max-w-xl">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-bold text-foreground">
                    Ingresa el código de verificación
                  </h2>
                  <span className="inline-flex items-center text-xs font-semibold text-amber-600 bg-amber-500/10 px-2.5 py-0.5 rounded-full border border-amber-500/20">
                    Pendiente
                  </span>
                </div>
                <p className="text-xs sm:text-sm text-muted-foreground">
                  Enviamos un código de 6 dígitos al número{" "}
                  <strong className="text-foreground font-mono">
                    {status?.masked_phone || phoneInput || "+56 9 •••• ••••"}
                  </strong>
                  .
                </p>
              </div>

              {/* Aviso para entorno local de desarrollo */}
              <div className="p-4 rounded-xl bg-muted/50 border border-border/60 text-xs sm:text-sm space-y-2">
                <div className="flex items-center gap-2 font-semibold text-foreground">
                  <Mail className="size-4 text-primary" />
                  <span>Mensaje en entorno local (Mailpit)</span>
                </div>
                <p className="text-muted-foreground leading-relaxed">
                  En este entorno de desarrollo local, el SMS simulado se
                  entrega en tu bandeja Mailpit. Abre tu bandeja para consultar
                  el código OTP:
                </p>
                <div className="pt-1">
                  <a
                    href="http://127.0.0.1:8025"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-primary hover:underline"
                  >
                    Abrir Mailpit (http://127.0.0.1:8025)
                    <ExternalLink className="size-3.5" />
                  </a>
                </div>
              </div>

              <div className="space-y-2">
                <Label
                  htmlFor="verification-code"
                  className="text-xs sm:text-sm font-semibold text-foreground"
                >
                  Código de 6 dígitos
                </Label>
                <Input
                  id="verification-code"
                  type="text"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  placeholder="123456"
                  value={codeInput}
                  onChange={(e) =>
                    setCodeInput(e.target.value.replace(/\D/g, ""))
                  }
                  className="font-mono text-center text-lg tracking-widest max-w-xs"
                  disabled={isVerifying}
                  autoFocus
                  required
                />
              </div>

              <div className="flex flex-col sm:flex-row items-center gap-3 pt-2">
                <button
                  type="submit"
                  disabled={isVerifying || codeInput.trim().length !== 6}
                  className="inline-flex items-center justify-center gap-2 px-6 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50 font-semibold text-xs sm:text-sm transition-colors w-full sm:w-auto shadow-sm"
                >
                  {isVerifying ? (
                    <>
                      <Loader2 className="size-4 animate-spin" />
                      Verificando...
                    </>
                  ) : (
                    <>
                      <CheckCircle2 className="size-4" />
                      Verificar código
                    </>
                  )}
                </button>

                <button
                  type="button"
                  onClick={handleResendCode}
                  disabled={!canResend || isSending}
                  className="inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl border border-border text-foreground hover:bg-muted disabled:opacity-50 font-medium text-xs sm:text-sm transition-colors w-full sm:w-auto"
                >
                  <RefreshCw
                    className={`size-3.5 ${isSending ? "animate-spin" : ""}`}
                  />
                  {canResend
                    ? "Reenviar código"
                    : `Reenviar código (${cooldownSeconds}s)`}
                </button>
              </div>

              <div className="pt-2 border-t border-border/40">
                <button
                  type="button"
                  onClick={() => {
                    clearError();
                    setStepOverride("phone");
                    setPhoneInput("");
                    setCodeInput("");
                  }}
                  className="text-xs text-muted-foreground hover:text-primary transition-colors underline hover:no-underline"
                >
                  Ingresar un número diferente
                </button>
              </div>
            </form>
          )}
        </div>
      )}
    </div>
  );
}
