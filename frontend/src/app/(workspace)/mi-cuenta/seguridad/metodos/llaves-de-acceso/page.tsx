"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ChevronRight,
  Fingerprint,
  Info,
  Key,
  Laptop,
  Loader2,
  Plus,
  ShieldCheck,
  Smartphone,
  Trash2,
  Usb,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePasskeys } from "@/features/account/ui/use-passkeys";

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

function getTransportBadge(transport: string) {
  switch (transport.toLowerCase()) {
    case "internal":
      return { label: "Biometría / Local", icon: Fingerprint };
    case "hybrid":
      return { label: "Móvil / QR", icon: Smartphone };
    case "usb":
      return { label: "Llave USB", icon: Usb };
    case "nfc":
      return { label: "NFC", icon: Smartphone };
    case "ble":
      return { label: "Bluetooth", icon: Laptop };
    default:
      return { label: transport, icon: Key };
  }
}

export default function LlavesDeAccesoPage() {
  const {
    passkeys,
    isLoading,
    isRegistering,
    deletingPasskeyId,
    error,
    notification,
    isSupported,
    registerPasskey,
    deletePasskey,
    clearError,
    clearNotification,
  } = usePasskeys();

  const [labelInput, setLabelInput] = useState("");
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(
    null,
  );

  const handleRegister = async (e: FormEvent) => {
    e.preventDefault();
    const cleanLabel = labelInput.trim();
    const created = await registerPasskey(cleanLabel || undefined);
    if (created) {
      setLabelInput("");
    }
  };

  const handleDelete = async (passkeyId: string) => {
    setConfirmingDeleteId(null);
    await deletePasskey(passkeyId);
  };

  const isActive = passkeys.length > 0;

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
        <span className="text-foreground font-semibold">Llaves de acceso</span>
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
              <span>Llaves de acceso</span>
              {isActive && (
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3.5" />
                  Activa
                </span>
              )}
            </h1>
          </div>
          <p className="text-sm text-muted-foreground max-w-2xl pl-11">
            Inicia sesión rápidamente con tu huella dactilar, reconocimiento
            facial (Touch ID, Face ID, Windows Hello) o llaves físicas de
            seguridad FIDO2.
          </p>
        </div>
      </div>

      {/* Notificación de éxito */}
      {notification && (
        <div className="bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 p-4 rounded-xl flex items-start justify-between gap-3 text-sm">
          <div className="flex items-center gap-2.5">
            <CheckCircle2 className="size-5 shrink-0 mt-0.5 text-emerald-600" />
            <span>{notification}</span>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="hover:opacity-75 transition-opacity p-0.5"
            aria-label="Cerrar notificación"
          >
            <X className="size-4" />
          </button>
        </div>
      )}

      {/* Notificación de error */}
      {error && (
        <div className="bg-destructive/10 border border-destructive/20 text-destructive p-4 rounded-xl flex items-start justify-between gap-3 text-sm">
          <div className="flex items-center gap-2.5">
            <AlertCircle className="size-5 shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="hover:opacity-75 transition-opacity p-0.5"
            aria-label="Cerrar error"
          >
            <X className="size-4" />
          </button>
        </div>
      )}

      {/* Advertencia de soporte de navegador */}
      {!isSupported && (
        <div className="bg-amber-500/10 border border-amber-500/30 text-amber-800 dark:text-amber-300 p-4 sm:p-5 rounded-2xl flex items-start gap-3.5 text-sm">
          <AlertTriangle className="size-5 text-amber-600 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <h4 className="font-semibold text-foreground">
              Navegador sin soporte de llaves de acceso
            </h4>
            <p className="text-xs sm:text-sm text-muted-foreground">
              Tu navegador o dispositivo actual no es compatible con el estándar
              WebAuthn / Passkeys. Para registrar y utilizar llaves de acceso,
              te recomendamos acceder desde un navegador moderno (Chrome,
              Safari, Edge o Firefox) con autenticación biométrica o llave de
              seguridad habilitada.
            </p>
          </div>
        </div>
      )}

      {/* Formulario para agregar llave de acceso */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
        <div className="flex items-start gap-4">
          <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
            <Plus className="size-5" />
          </div>
          <div className="space-y-1">
            <h2 className="text-base sm:text-lg font-bold text-foreground">
              Agregar una nueva llave de acceso
            </h2>
            <p className="text-xs sm:text-sm text-muted-foreground">
              Registra este dispositivo o una llave externa. Tu navegador te
              pedirá confirmar mediante tu huella, rostro o PIN local.
            </p>
          </div>
        </div>

        <form onSubmit={handleRegister} className="space-y-4 max-w-lg">
          <div className="space-y-2">
            <Label htmlFor="passkey-label" className="text-sm font-medium">
              Nombre de la llave (opcional)
            </Label>
            <Input
              id="passkey-label"
              type="text"
              placeholder="Ej: Laptop Linux, iPhone 15, YubiKey 5"
              value={labelInput}
              onChange={(e) => setLabelInput(e.target.value)}
              disabled={isRegistering || !isSupported}
              maxLength={100}
              className="bg-background"
            />
            <p className="text-xs text-muted-foreground">
              Te ayuda a identificar qué dispositivo o hardware registraste.
            </p>
          </div>

          <Button
            type="submit"
            disabled={isRegistering || !isSupported}
            className="w-full sm:w-auto inline-flex items-center gap-2"
          >
            {isRegistering ? (
              <>
                <Loader2 className="size-4 animate-spin" />
                <span>Confirmando en tu dispositivo...</span>
              </>
            ) : (
              <>
                <Key className="size-4" />
                <span>Agregar llave de acceso</span>
              </>
            )}
          </Button>
        </form>
      </div>

      {/* Lista de llaves registradas */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
          <div className="flex items-center gap-3">
            <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
              <Key className="size-5" />
            </div>
            <div>
              <h2 className="text-base sm:text-lg font-bold text-foreground">
                Llaves registradas
              </h2>
              <p className="text-xs sm:text-sm text-muted-foreground">
                Dispositivos autorizados para iniciar sesión con biometría o
                PIN.
              </p>
            </div>
          </div>
          <span className="text-xs font-medium text-muted-foreground self-start sm:self-auto">
            {passkeys.length}{" "}
            {passkeys.length === 1 ? "llave registrada" : "llaves registradas"}
          </span>
        </div>

        {isLoading ? (
          <div className="py-8 flex flex-col items-center justify-center gap-2 text-muted-foreground">
            <Loader2 className="size-6 animate-spin text-primary" />
            <p className="text-xs">Cargando llaves de acceso...</p>
          </div>
        ) : passkeys.length === 0 ? (
          <div className="py-8 text-center space-y-2">
            <Fingerprint className="size-10 text-muted-foreground/60 mx-auto" />
            <h3 className="text-sm font-semibold text-foreground">
              Aún no tienes llaves de acceso configuradas
            </h3>
            <p className="text-xs text-muted-foreground max-w-sm mx-auto">
              Agrega una llave arriba para acceder a EspaciGo con mayor rapidez
              y seguridad sin ingresar contraseñas.
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border/60">
            {passkeys.map((passkey) => {
              const isDeleting = deletingPasskeyId === passkey.id;
              const isConfirming = confirmingDeleteId === passkey.id;

              return (
                <div
                  key={passkey.id}
                  className="py-4.5 flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                >
                  <div className="flex items-start sm:items-center gap-3.5 min-w-0">
                    <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5 sm:mt-0">
                      <Fingerprint className="size-5" />
                    </div>
                    <div className="space-y-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-sm font-semibold text-foreground">
                          {passkey.label}
                        </span>
                        <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                          <CheckCircle2 className="size-3" />
                          Activa
                        </span>
                      </div>

                      <div className="text-xs text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span>
                          Creada: {formatFullDate(passkey.created_at)}
                        </span>
                        <span>•</span>
                        <span>
                          {passkey.last_used_at
                            ? `Último uso: ${formatFullDate(passkey.last_used_at)}`
                            : "Aún sin uso"}
                        </span>
                      </div>

                      {passkey.transports && passkey.transports.length > 0 && (
                        <div className="flex items-center gap-1.5 pt-1 flex-wrap">
                          {passkey.transports.map((t) => {
                            const badge = getTransportBadge(t);
                            const BadgeIcon = badge.icon;
                            return (
                              <span
                                key={t}
                                className="inline-flex items-center gap-1 text-[10px] text-muted-foreground bg-muted px-2 py-0.5 rounded-md font-medium"
                              >
                                <BadgeIcon className="size-3" />
                                {badge.label}
                              </span>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Botones de acción / confirmación de eliminación */}
                  <div className="shrink-0 self-end sm:self-center">
                    {isConfirming ? (
                      <div className="flex items-center gap-2">
                        <button
                          type="button"
                          onClick={() => handleDelete(passkey.id)}
                          disabled={isDeleting}
                          className="text-xs font-semibold px-2.5 py-1.5 rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors inline-flex items-center gap-1"
                        >
                          {isDeleting && (
                            <Loader2 className="size-3 animate-spin" />
                          )}
                          Sí, eliminar
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmingDeleteId(null)}
                          disabled={isDeleting}
                          className="text-xs font-medium px-2.5 py-1.5 rounded-lg bg-muted text-muted-foreground hover:bg-muted/80 transition-colors"
                        >
                          Cancelar
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        onClick={() => setConfirmingDeleteId(passkey.id)}
                        disabled={isDeleting}
                        className="text-xs text-muted-foreground hover:text-destructive hover:bg-destructive/10 px-2.5 py-1.5 rounded-lg transition-colors inline-flex items-center gap-1.5"
                        title="Eliminar llave de acceso"
                      >
                        <Trash2 className="size-3.5" />
                        <span>Eliminar</span>
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Información de seguridad sobre Passkeys */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-4 shadow-sm">
        <div className="flex items-start gap-4">
          <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
            <ShieldCheck className="size-5" />
          </div>
          <div className="space-y-1">
            <h3 className="text-base sm:text-lg font-bold text-foreground">
              ¿Por qué usar Llaves de acceso (Passkeys)?
            </h3>
            <p className="text-xs sm:text-sm text-muted-foreground">
              Las llaves de acceso son la forma más segura y moderna de acceder
              a tus cuentas en internet, reemplazando a las contraseñas
              tradicionales.
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-2">
          <div className="bg-muted/40 border border-border/60 rounded-xl p-4 space-y-1.5">
            <div className="flex items-center gap-2 text-foreground font-semibold text-xs sm:text-sm">
              <Fingerprint className="size-4 text-primary" />
              <span>Cero contraseñas</span>
            </div>
            <p className="text-xs text-muted-foreground">
              Inicia sesión con tu biometría o el PIN de tu pantalla de bloqueo
              en menos de dos segundos.
            </p>
          </div>

          <div className="bg-muted/40 border border-border/60 rounded-xl p-4 space-y-1.5">
            <div className="flex items-center gap-2 text-foreground font-semibold text-xs sm:text-sm">
              <ShieldCheck className="size-4 text-emerald-600" />
              <span>Inmune al phishing</span>
            </div>
            <p className="text-xs text-muted-foreground">
              La llave solo responde al dominio legítimo verificado por el
              navegador, protegiéndote de sitios falsificados.
            </p>
          </div>

          <div className="bg-muted/40 border border-border/60 rounded-xl p-4 space-y-1.5">
            <div className="flex items-center gap-2 text-foreground font-semibold text-xs sm:text-sm">
              <Info className="size-4 text-primary" />
              <span>Privacidad total</span>
            </div>
            <p className="text-xs text-muted-foreground">
              Tu huella o rostro nunca sale de tu dispositivo; solo se verifica
              la firma criptográfica pública en nuestros servidores.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
