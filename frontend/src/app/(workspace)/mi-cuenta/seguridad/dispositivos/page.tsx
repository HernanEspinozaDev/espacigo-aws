"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  Info,
  Laptop,
  Loader2,
  LogOut,
  RefreshCw,
  Smartphone,
  X,
} from "lucide-react";
import { useLinkedDevices } from "@/features/account/ui/use-linked-devices";
import type { LinkedDevice } from "@/features/account/linked-devices-api";

function isMobileDevice(deviceLabel: string): boolean {
  const lower = deviceLabel.toLowerCase();
  return (
    lower.includes("iphone") ||
    lower.includes("ipad") ||
    lower.includes("android") ||
    lower.includes("móvil") ||
    lower.includes("movil") ||
    lower.includes("phone") ||
    lower.includes("tablet") ||
    lower.includes("ios")
  );
}

function formatRelativeActivity(isoDate: string): string {
  try {
    const timestamp = new Date(isoDate).getTime();
    if (Number.isNaN(timestamp)) return isoDate;
    const diffMs = Date.now() - timestamp;
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 120) return "Activo ahora";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `Hace ${diffMin} min`;
    const diffHours = Math.floor(diffMin / 60);
    if (diffHours < 24) return `Hace ${diffHours} h`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays === 1) return "Ayer";
    if (diffDays < 7) return `Hace ${diffDays} días`;
    return new Intl.DateTimeFormat("es-CL", {
      dateStyle: "short",
      timeStyle: "short",
    }).format(new Date(isoDate));
  } catch {
    return isoDate;
  }
}

function formatCreatedAt(isoDate: string): string {
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

export default function DispositivosSeguridadPage() {
  const {
    devices,
    currentDevice,
    otherDevices,
    isLoading,
    revokingSessionId,
    isRevokingOthers,
    error,
    notification,
    refresh,
    revokeDevice,
    revokeOtherDevices,
    clearNotification,
    clearError,
  } = useLinkedDevices();

  const [confirmingSessionId, setConfirmingSessionId] = useState<string | null>(
    null,
  );
  const [confirmingRevokeAllOthers, setConfirmingRevokeAllOthers] =
    useState(false);

  const handleRevokeSingle = async (device: LinkedDevice) => {
    setConfirmingSessionId(null);
    await revokeDevice(device.id);
  };

  const handleRevokeOthers = async () => {
    setConfirmingRevokeAllOthers(false);
    await revokeOtherDevices();
  };

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground"
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
        <span className="text-foreground font-semibold">
          Dispositivos vinculados
        </span>
      </nav>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
            Dispositivos vinculados
          </h1>
          <p className="text-sm text-muted-foreground">
            Revisa y gestiona los navegadores y dispositivos que tienen acceso a
            tu cuenta de EspaciGo.
          </p>
        </div>

        <button
          type="button"
          onClick={() => refresh()}
          disabled={isLoading}
          aria-label="Refrescar lista de dispositivos"
          className="self-start sm:self-auto inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-border bg-card hover:bg-muted/50 text-xs font-semibold text-foreground transition-colors disabled:opacity-50"
        >
          <RefreshCw
            className={`size-3.5 ${isLoading ? "animate-spin" : ""}`}
          />
          <span>Actualizar</span>
        </button>
      </div>

      {/* Notificación de éxito */}
      {notification && (
        <div
          role="status"
          className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2 min-w-0">
            <CheckCircle2 className="size-4 shrink-0" />
            <span>{notification}</span>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="p-1 rounded-md hover:bg-emerald-500/20 shrink-0"
            aria-label="Cerrar notificación"
          >
            <X className="size-3.5" />
          </button>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div
          role="alert"
          className="p-4 rounded-xl bg-destructive/10 border border-destructive/20 text-destructive text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2 min-w-0">
            <AlertTriangle className="size-4 shrink-0" />
            <span>{error}</span>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="p-1 rounded-md hover:bg-destructive/20 shrink-0"
            aria-label="Cerrar alerta de error"
          >
            <X className="size-3.5" />
          </button>
        </div>
      )}

      {/* Loading Skeleton */}
      {isLoading && devices.length === 0 && (
        <div className="space-y-6">
          <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4 animate-pulse">
            <div className="h-4 w-48 bg-muted rounded" />
            <div className="h-20 bg-muted/50 rounded-xl" />
          </div>
          <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4 animate-pulse">
            <div className="h-4 w-48 bg-muted rounded" />
            <div className="h-16 bg-muted/50 rounded-xl" />
          </div>
        </div>
      )}

      {/* Sesión actual */}
      {(!isLoading || devices.length > 0) && (
        <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4">
          <h2 className="text-base font-bold text-foreground">
            Sesión en este dispositivo
          </h2>

          {currentDevice ? (
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 p-4 rounded-xl bg-muted/30 border border-border/60">
              <div className="flex items-center gap-4 min-w-0">
                <div className="size-12 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                  {isMobileDevice(currentDevice.device_label) ? (
                    <Smartphone className="size-6" />
                  ) : (
                    <Laptop className="size-6" />
                  )}
                </div>
                <div className="space-y-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-foreground">
                      {currentDevice.device_label}
                    </span>
                    <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                      <CheckCircle2 className="size-3" />
                      Este dispositivo
                    </span>
                  </div>
                  <div className="text-xs text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5">
                    <span>IP: {currentDevice.ip_masked}</span>
                    <span>•</span>
                    <span>
                      Última actividad:{" "}
                      {formatRelativeActivity(currentDevice.last_activity_at)}
                    </span>
                    <span>•</span>
                    <span>
                      Iniciado: {formatCreatedAt(currentDevice.created_at)}
                    </span>
                  </div>
                </div>
              </div>

              {confirmingSessionId === currentDevice.id ? (
                <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                  <button
                    type="button"
                    onClick={() => handleRevokeSingle(currentDevice)}
                    disabled={revokingSessionId === currentDevice.id}
                    className="text-xs font-semibold px-2.5 py-1 rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors inline-flex items-center gap-1"
                  >
                    {revokingSessionId === currentDevice.id && (
                      <Loader2 className="size-3 animate-spin" />
                    )}
                    Sí, cerrar
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmingSessionId(null)}
                    className="text-xs font-medium px-2 py-1 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                  >
                    Cancelar
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmingSessionId(currentDevice.id)}
                  disabled={revokingSessionId === currentDevice.id}
                  className="text-xs font-semibold text-muted-foreground hover:text-destructive transition-colors whitespace-nowrap px-3 py-1.5 rounded-lg hover:bg-destructive/10 self-end sm:self-center"
                >
                  Cerrar sesión actual
                </button>
              )}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground py-2">
              No se detectó la sesión actual como activa en este navegador.
            </p>
          )}
        </div>
      )}

      {/* Otras sesiones */}
      {(!isLoading || devices.length > 0) && (
        <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h2 className="text-base font-bold text-foreground">
                Otras sesiones iniciadas
              </h2>
              <p className="text-xs text-muted-foreground">
                Dispositivos en los que iniciaste sesión previamente.
              </p>
            </div>

            {otherDevices.length > 0 && (
              <div>
                {confirmingRevokeAllOthers ? (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground font-medium">
                      ¿Cerrar todas las demás sesiones?
                    </span>
                    <button
                      type="button"
                      onClick={handleRevokeOthers}
                      disabled={isRevokingOthers}
                      className="text-xs font-semibold px-2.5 py-1 rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors inline-flex items-center gap-1"
                    >
                      {isRevokingOthers && (
                        <Loader2 className="size-3 animate-spin" />
                      )}
                      Confirmar
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirmingRevokeAllOthers(false)}
                      className="text-xs font-medium px-2 py-1 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                    >
                      Cancelar
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => setConfirmingRevokeAllOthers(true)}
                    disabled={isRevokingOthers}
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-destructive hover:underline disabled:opacity-50"
                  >
                    <LogOut className="size-3.5" />
                    Cerrar todas las demás sesiones
                  </button>
                )}
              </div>
            )}
          </div>

          {otherDevices.length === 0 ? (
            <p className="text-xs text-muted-foreground py-4 text-center">
              No tienes otras sesiones abiertas en este momento.
            </p>
          ) : (
            <div className="divide-y divide-border/60">
              {otherDevices.map((session) => {
                const isMobile = isMobileDevice(session.device_label);
                const isConfirming = confirmingSessionId === session.id;
                const isRevoking = revokingSessionId === session.id;

                return (
                  <div
                    key={session.id}
                    className="py-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3"
                  >
                    <div className="flex items-center gap-3.5 min-w-0">
                      <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
                        {isMobile ? (
                          <Smartphone className="size-5" />
                        ) : (
                          <Laptop className="size-5" />
                        )}
                      </div>
                      <div className="space-y-0.5 min-w-0">
                        <p className="text-sm font-semibold text-foreground">
                          {session.device_label}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          IP: {session.ip_masked} • Última actividad:{" "}
                          <span className="italic">
                            {formatRelativeActivity(session.last_activity_at)}
                          </span>
                          {" • "}
                          Iniciado: {formatCreatedAt(session.created_at)}
                        </p>
                      </div>
                    </div>

                    <div className="shrink-0 self-end sm:self-center">
                      {isConfirming ? (
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            onClick={() => handleRevokeSingle(session)}
                            disabled={isRevoking}
                            className="text-xs font-semibold px-2.5 py-1 rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors inline-flex items-center gap-1"
                          >
                            {isRevoking && (
                              <Loader2 className="size-3 animate-spin" />
                            )}
                            Sí, cerrar
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirmingSessionId(null)}
                            className="text-xs font-medium px-2 py-1 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                          >
                            Cancelar
                          </button>
                        </div>
                      ) : (
                        <button
                          type="button"
                          onClick={() => setConfirmingSessionId(session.id)}
                          disabled={isRevoking}
                          className="text-xs font-semibold text-primary hover:text-primary/80 transition-colors whitespace-nowrap px-3 py-1.5 rounded-lg hover:bg-primary/10"
                        >
                          Cerrar sesión
                        </button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Nota informativa de fase */}
      <div className="p-4 rounded-xl bg-muted/40 border border-border/60 text-xs text-muted-foreground flex items-start gap-3">
        <Info className="size-4 text-primary shrink-0 mt-0.5" />
        <p>
          <strong>Plan de seguridad EspaciGo (Fase 2):</strong> La revocación de
          sesiones invalida de forma atómica los tokens Bearer emitidos en AWS
          ECS y PostgreSQL bajo control de acceso de sesión.
        </p>
      </div>
    </div>
  );
}
