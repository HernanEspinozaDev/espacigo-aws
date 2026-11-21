"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronRight,
  Copy,
  Download,
  Info,
  KeyRound,
  Loader2,
  Lock,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTOTP } from "@/features/account/ui/use-totp";

function formatKeyBlocks(key: string): string {
  return (
    key
      .replace(/\s+/g, "")
      .match(/.{1,4}/g)
      ?.join(" ") || key
  );
}

function formatFullDate(isoDate?: string | null): string {
  if (!isoDate) return "Fecha no registrada";
  try {
    const date = new Date(isoDate);
    if (Number.isNaN(date.getTime())) return isoDate;
    return new Intl.DateTimeFormat("es-CL", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  } catch {
    return isoDate;
  }
}

function downloadBackupCodesFile(codes: string[]) {
  const text = [
    "===========================================================",
    "ESPACIGO - CÓDIGOS DE RESPALDO DE GOOGLE AUTHENTICATOR (TOTP)",
    `Fecha de generación: ${new Date().toLocaleString("es-CL")}`,
    "===========================================================",
    "",
    "INSTRUCCIONES IMPORTANTES:",
    "- Cada uno de estos códigos puede ser utilizado UNA SOLA VEZ.",
    "- Úsalos para iniciar sesión o desactivar TOTP si pierdes tu teléfono.",
    "- Mantén este archivo en un lugar protegido y confidencial.",
    "",
    "TUS 8 CÓDIGOS DE RESPALDO:",
    ...codes.map((code, index) => `  ${index + 1}.  ${code}`),
    "",
    "===========================================================",
  ].join("\n");

  const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = "espacigo-codigos-respaldo-totp.txt";
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

export default function GoogleAuthenticatorPage() {
  const {
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
  } = useTOTP();

  const [verifyCodeInput, setVerifyCodeInput] = useState("");
  const [disableCodeInput, setDisableCodeInput] = useState("");
  const [isCopiedKey, setIsCopiedKey] = useState(false);
  const [isCopiedCodes, setIsCopiedCodes] = useState(false);
  const [showDisableConfirm, setShowDisableConfirm] = useState(false);

  const handleCopyKey = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key.replace(/\s+/g, ""));
      setIsCopiedKey(true);
      setTimeout(() => setIsCopiedKey(false), 2500);
    } catch {
      // fallback
    }
  };

  const handleCopyBackupCodes = async (codes: string[]) => {
    try {
      await navigator.clipboard.writeText(codes.join("\n"));
      setIsCopiedCodes(true);
      setTimeout(() => setIsCopiedCodes(false), 2500);
    } catch {
      // fallback
    }
  };

  const handleEnableSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const cleanCode = verifyCodeInput.replace(/\s+/g, "");
    if (!cleanCode) return;
    const res = await enable(cleanCode);
    if (res) {
      setVerifyCodeInput("");
    }
  };

  const handleDisableSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const cleanCode = disableCodeInput.replace(/\s+/g, "");
    if (!cleanCode) return;
    const ok = await disable(cleanCode);
    if (ok) {
      setDisableCodeInput("");
      setShowDisableConfirm(false);
    }
  };

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
        <span className="text-foreground font-semibold">
          Google Authenticator
        </span>
      </nav>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-2 border-b border-border">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <Link
              href="/mi-cuenta/seguridad/metodos"
              className="size-8 rounded-full flex items-center justify-center hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
              aria-label="Volver a métodos de verificación"
            >
              <ArrowLeft className="size-4" />
            </Link>
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground flex items-center gap-2.5">
              <span>Google Authenticator</span>
              {status?.enabled && (
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3.5" />
                  Activo
                </span>
              )}
            </h1>
          </div>
          <p className="text-sm text-muted-foreground ml-11">
            Protege tu cuenta con códigos de seguridad TOTP de un solo uso
            generados en tu dispositivo móvil.
          </p>
        </div>

        <Button
          variant="outline"
          size="sm"
          onClick={() => refresh()}
          disabled={isLoading}
          className="self-start sm:self-auto shrink-0"
        >
          <RefreshCw
            className={`size-3.5 mr-1.5 ${isLoading ? "animate-spin" : ""}`}
          />
          Actualizar
        </Button>
      </div>

      {/* Banners de Notificación y Error */}
      {notification && (
        <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-xl p-4 flex items-start justify-between gap-3 text-emerald-950 dark:text-emerald-200">
          <div className="flex items-center gap-3">
            <CheckCircle2 className="size-5 text-emerald-600 shrink-0" />
            <p className="text-sm font-medium">{notification}</p>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            Cerrar
          </button>
        </div>
      )}

      {error && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-xl p-4 flex items-start justify-between gap-3 text-destructive">
          <div className="flex items-center gap-3">
            <AlertCircle className="size-5 shrink-0" />
            <p className="text-sm font-medium">{error}</p>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            Cerrar
          </button>
        </div>
      )}

      {/* Loading state */}
      {isLoading && !status && (
        <div className="p-12 flex flex-col items-center justify-center gap-3 text-muted-foreground bg-card border border-border rounded-2xl">
          <Loader2 className="size-8 animate-spin text-primary" />
          <p className="text-sm">Cargando estado de autenticación...</p>
        </div>
      )}

      {/* VISTA 1: CÓDIGOS DE RESPALDO RECIÉN GENERADOS (DESPUÉS DE ACTIVAR) */}
      {backupCodes && backupCodes.length > 0 && (
        <div className="bg-card border-2 border-primary/30 rounded-2xl p-6 sm:p-8 space-y-6 shadow-md">
          <div className="flex items-start gap-4">
            <div className="size-12 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
              <KeyRound className="size-6" />
            </div>
            <div className="space-y-1">
              <span className="text-xs font-semibold tracking-wider uppercase text-primary">
                Paso Importante
              </span>
              <h2 className="text-xl sm:text-2xl font-bold text-foreground">
                Guarda tus códigos de respaldo
              </h2>
              <p className="text-sm text-muted-foreground">
                Si alguna vez no tienes acceso a tu app de autenticación o
                pierdes tu teléfono, estos códigos de 8 caracteres te permitirán
                iniciar sesión y gestionar tu seguridad.
              </p>
            </div>
          </div>

          {/* Alerta de un solo visionado */}
          <div className="bg-amber-500/10 border border-amber-500/20 rounded-xl p-4 flex items-start gap-3">
            <AlertTriangle className="size-5 text-amber-600 shrink-0 mt-0.5" />
            <div className="text-xs sm:text-sm text-amber-950 dark:text-amber-200 space-y-1">
              <p className="font-semibold">
                Atención: estos códigos solo se mostrarán esta vez.
              </p>
              <p className="text-amber-900/80 dark:text-amber-300/80">
                Por motivos de seguridad, EspaciGo almacena únicamente un
                resumen criptográfico no reversible. Cópialos o descárgalos
                ahora mismo en un lugar seguro.
              </p>
            </div>
          </div>

          {/* Grid de códigos de respaldo */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 py-2">
            {backupCodes.map((code, index) => (
              <div
                key={code}
                className="bg-muted/70 border border-border rounded-xl p-3 text-center font-mono text-base font-semibold tracking-wider text-foreground select-all"
              >
                <span className="text-xs text-muted-foreground mr-1.5 font-sans">
                  {index + 1}.
                </span>
                {code}
              </div>
            ))}
          </div>

          {/* Acciones de respaldo */}
          <div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-2 border-t border-border">
            <div className="flex flex-wrap items-center gap-3 w-full sm:w-auto">
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleCopyBackupCodes(backupCodes)}
                className="flex items-center gap-2"
              >
                {isCopiedCodes ? (
                  <>
                    <Check className="size-4 text-emerald-600" />
                    <span>¡Copiados al portapapeles!</span>
                  </>
                ) : (
                  <>
                    <Copy className="size-4" />
                    <span>Copiar todos</span>
                  </>
                )}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => downloadBackupCodesFile(backupCodes)}
                className="flex items-center gap-2"
              >
                <Download className="size-4" />
                <span>Descargar archivo .txt</span>
              </Button>
            </div>

            <Button
              variant="default"
              size="default"
              onClick={clearBackupCodes}
              className="w-full sm:w-auto font-medium"
            >
              He guardado mis códigos
            </Button>
          </div>
        </div>
      )}

      {/* VISTA 2: ESTADO ACTIVO */}
      {status?.enabled && (!backupCodes || backupCodes.length === 0) && (
        <div className="space-y-6">
          {/* Tarjeta de estado activo */}
          <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div className="flex items-start gap-4">
                <div className="size-12 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                  <ShieldCheck className="size-7" />
                </div>
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <h2 className="text-lg sm:text-xl font-bold text-foreground">
                      Autenticador TOTP configurado y activo
                    </h2>
                    <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                      <CheckCircle2 className="size-3.5" />
                      Activo
                    </span>
                  </div>
                  <p className="text-xs sm:text-sm text-muted-foreground">
                    Activado el {formatFullDate(status.enabled_at)}
                  </p>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
              <div className="bg-muted/40 border border-border/80 rounded-xl p-4 space-y-1">
                <span className="text-xs font-medium text-muted-foreground">
                  Códigos de respaldo restantes
                </span>
                <p className="text-2xl font-bold text-foreground font-mono">
                  {status.remaining_backup_codes} de 8
                </p>
                <p className="text-xs text-muted-foreground">
                  {status.remaining_backup_codes > 0
                    ? "Cada código puede utilizarse una única vez para acceder o desactivar."
                    : "No te quedan códigos de respaldo disponibles."}
                </p>
              </div>

              <div className="bg-muted/40 border border-border/80 rounded-xl p-4 space-y-1">
                <span className="text-xs font-medium text-muted-foreground">
                  Método de protección
                </span>
                <p className="text-base font-semibold text-foreground">
                  RFC 6238 TOTP (30s)
                </p>
                <p className="text-xs text-muted-foreground">
                  Compatible con Google Authenticator, Microsoft Authenticator y
                  1Password.
                </p>
              </div>
            </div>
          </div>

          {/* Sección para Desactivar */}
          <div className="bg-card border border-destructive/20 rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
            <div className="flex items-start gap-4">
              <div className="size-10 rounded-xl bg-destructive/10 text-destructive flex items-center justify-center shrink-0 mt-0.5">
                <Trash2 className="size-5" />
              </div>
              <div className="space-y-1">
                <h3 className="text-base sm:text-lg font-bold text-foreground">
                  Desactivar Google Authenticator
                </h3>
                <p className="text-xs sm:text-sm text-muted-foreground">
                  Si ya no deseas utilizar este método o necesitas migrar a un
                  nuevo dispositivo, puedes desactivarlo ingresando tu código
                  TOTP actual o un código de respaldo.
                </p>
              </div>
            </div>

            {!showDisableConfirm ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setShowDisableConfirm(true)}
              >
                Quiero desactivar Google Authenticator
              </Button>
            ) : (
              <form
                onSubmit={handleDisableSubmit}
                className="space-y-4 pt-2 max-w-md"
              >
                <div className="space-y-2">
                  <Label htmlFor="disable-code" className="text-xs font-medium">
                    Código de 6 dígitos o código de respaldo (8 caracteres)
                  </Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="disable-code"
                      type="text"
                      inputMode="text"
                      autoComplete="one-time-code"
                      placeholder="123456 o ABCDEFGH"
                      value={disableCodeInput}
                      onChange={(e) => setDisableCodeInput(e.target.value)}
                      maxLength={10}
                      className="font-mono text-center tracking-wider text-base h-10"
                      disabled={isDisabling}
                      required
                    />
                    <Button
                      type="submit"
                      variant="destructive"
                      disabled={isDisabling || !disableCodeInput.trim()}
                      className="shrink-0"
                    >
                      {isDisabling ? (
                        <>
                          <Loader2 className="size-4 animate-spin mr-1.5" />
                          <span>Desactivando...</span>
                        </>
                      ) : (
                        <span>Confirmar desactivación</span>
                      )}
                    </Button>
                  </div>
                </div>

                <div className="flex items-center gap-3">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setShowDisableConfirm(false);
                      setDisableCodeInput("");
                    }}
                    disabled={isDisabling}
                    className="text-xs text-muted-foreground"
                  >
                    Cancelar
                  </Button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* VISTA 3: ENROLAMIENTO (NO ACTIVADO) */}
      {!status?.enabled && (!backupCodes || backupCodes.length === 0) && (
        <div className="space-y-6">
          {!setupData ? (
            /* Pantalla inicial previa a setup */
            <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
              <div className="flex flex-col sm:flex-row items-start gap-5">
                <div className="size-14 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
                  <Smartphone className="size-7" />
                </div>
                <div className="space-y-2 flex-1">
                  <h2 className="text-xl font-bold text-foreground">
                    Configura la verificación en dos pasos con Google
                    Authenticator
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Añade una capa extra de protección a tu cuenta. Al iniciar
                    sesión o realizar acciones sensibles, se te solicitará un
                    código numérico generado por la aplicación en tu celular.
                  </p>
                </div>
              </div>

              {/* Beneficios */}
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-2">
                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <Lock className="size-4 text-primary" />
                    <span>Mayor seguridad</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Protege tu cuenta incluso si tu contraseña llega a ser
                    comprometida.
                  </p>
                </div>

                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <ShieldCheck className="size-4 text-emerald-600" />
                    <span>Funciona offline</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    No depende de cobertura telefónica, SMS ni recepción de
                    correos.
                  </p>
                </div>

                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <KeyRound className="size-4 text-primary" />
                    <span>Estándar abierto</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Compatible con Google Authenticator, Microsoft
                    Authenticator, Authy o 1Password.
                  </p>
                </div>
              </div>

              <div className="pt-4 border-t border-border flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                <div className="text-xs text-muted-foreground flex items-center gap-1.5">
                  <Info className="size-4 text-muted-foreground shrink-0" />
                  <span>
                    El proceso toma menos de 2 minutos y requiere tu teléfono.
                  </span>
                </div>

                <Button
                  variant="default"
                  size="lg"
                  onClick={() => startSetup()}
                  disabled={isSettingUp}
                  className="font-medium"
                >
                  {isSettingUp ? (
                    <>
                      <Loader2 className="size-4 animate-spin mr-2" />
                      <span>Iniciando configuración...</span>
                    </>
                  ) : (
                    <span>Configurar Google Authenticator</span>
                  )}
                </Button>
              </div>
            </div>
          ) : (
            /* Pantalla de configuración en progreso (setupData) */
            <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-8 shadow-sm">
              <div className="space-y-1">
                <span className="text-xs font-semibold tracking-wider uppercase text-primary">
                  Paso 1 de 2
                </span>
                <h2 className="text-xl sm:text-2xl font-bold text-foreground">
                  Vincula tu aplicación de autenticación
                </h2>
                <p className="text-sm text-muted-foreground">
                  Abre tu aplicación (Google Authenticator, Microsoft
                  Authenticator, etc.), añade una nueva cuenta seleccionando{" "}
                  <strong>Ingresar clave de configuración</strong> y copia la
                  siguiente llave secreta:
                </p>
              </div>

              {/* Bloque visual de la Llave Manual */}
              <div className="space-y-3">
                <Label className="text-xs font-medium text-muted-foreground">
                  Clave secreta manual de configuración
                </Label>
                <div className="bg-muted/60 border border-border rounded-xl p-4 flex flex-col sm:flex-row items-center justify-between gap-4">
                  <div className="font-mono text-lg sm:text-xl font-bold tracking-widest text-foreground select-all break-all text-center sm:text-left">
                    {formatKeyBlocks(setupData.manual_key)}
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleCopyKey(setupData.manual_key)}
                    className="shrink-0 flex items-center gap-2"
                  >
                    {isCopiedKey ? (
                      <>
                        <Check className="size-4 text-emerald-600" />
                        <span>¡Copiada!</span>
                      </>
                    ) : (
                      <>
                        <Copy className="size-4" />
                        <span>Copiar clave</span>
                      </>
                    )}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Tipo de clave: Basada en tiempo (TOTP) • 6 dígitos • 30
                  segundos.
                </p>
              </div>

              {/* Paso 2: Verificar el código de 6 dígitos */}
              <div className="pt-6 border-t border-border space-y-4">
                <div className="space-y-1">
                  <span className="text-xs font-semibold tracking-wider uppercase text-primary">
                    Paso 2 de 2
                  </span>
                  <h3 className="text-lg font-bold text-foreground">
                    Verifica el código generado
                  </h3>
                  <p className="text-sm text-muted-foreground">
                    Introduce el código de 6 dígitos que muestra tu aplicación
                    para confirmar que se vinculó correctamente.
                  </p>
                </div>

                <form
                  onSubmit={handleEnableSubmit}
                  className="space-y-4 max-w-md"
                >
                  <div className="space-y-2">
                    <Label
                      htmlFor="verify-totp-code"
                      className="text-xs font-medium"
                    >
                      Código de 6 dígitos
                    </Label>
                    <div className="flex items-center gap-3">
                      <Input
                        id="verify-totp-code"
                        type="text"
                        inputMode="numeric"
                        pattern="[0-9]*"
                        autoComplete="one-time-code"
                        placeholder="123456"
                        value={verifyCodeInput}
                        onChange={(e) =>
                          setVerifyCodeInput(e.target.value.slice(0, 6))
                        }
                        maxLength={6}
                        className="font-mono text-center tracking-widest text-xl h-11"
                        disabled={isEnabling}
                        required
                        autoFocus
                      />
                      <Button
                        type="submit"
                        variant="default"
                        size="lg"
                        disabled={
                          isEnabling || verifyCodeInput.trim().length !== 6
                        }
                        className="shrink-0"
                      >
                        {isEnabling ? (
                          <>
                            <Loader2 className="size-4 animate-spin mr-2" />
                            <span>Verificando...</span>
                          </>
                        ) : (
                          <span>Verificar y activar</span>
                        )}
                      </Button>
                    </div>
                  </div>
                </form>

                <div className="pt-2">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={resetSetup}
                    disabled={isEnabling}
                    className="text-xs text-muted-foreground hover:text-foreground"
                  >
                    Cancelar configuración
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
