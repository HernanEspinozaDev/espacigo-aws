"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Bell,
  CheckCircle2,
  ChevronRight,
  Info,
  Loader2,
  LogOut,
  Mail,
  MessageSquare,
  Plus,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  Smartphone,
  X,
} from "lucide-react";
import { useSecurityAlerts } from "@/features/account/ui/use-security-alerts";
import { cn } from "@/lib/utils";

function formatRelativeTime(isoDate: string): string {
  try {
    const timestamp = new Date(isoDate).getTime();
    if (Number.isNaN(timestamp)) return isoDate;
    const diffMs = Date.now() - timestamp;
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return "Hace un momento";
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

function formatFullDateTime(isoDate: string): string {
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

function getEventVisuals(eventCode: string) {
  switch (eventCode) {
    case "password_changed":
      return {
        icon: ShieldCheck,
        iconColor: "text-emerald-600 bg-emerald-500/10",
      };
    case "phone_verified":
      return {
        icon: Smartphone,
        iconColor: "text-emerald-600 bg-emerald-500/10",
      };
    case "session_revoked":
    case "sessions_revoked_others":
      return {
        icon: LogOut,
        iconColor: "text-amber-600 bg-amber-500/10",
      };
    case "alert_channel_updated":
      return {
        icon: Bell,
        iconColor: "text-primary bg-primary/10",
      };
    default:
      return {
        icon: Info,
        iconColor: "text-primary bg-primary/10",
      };
  }
}

export default function AlertasSeguridadPage() {
  const {
    preferences,
    alerts,
    isLoading,
    isSaving,
    error,
    notification,
    refresh,
    toggleChannel,
    saveExtraEmail,
    removeExtraEmail,
    clearNotification,
    clearError,
  } = useSecurityAlerts();

  // Additional email dialog state
  const [showAddEmailModal, setShowAddEmailModal] = useState(false);
  const [modalEmail, setModalEmail] = useState("");
  const [modalError, setModalError] = useState<string | null>(null);

  const hasPhone = Boolean(
    preferences?.masked_phone && preferences.masked_phone.trim().length > 0,
  );

  const handleOpenEmailModal = () => {
    setModalEmail("");
    setModalError(null);
    clearError();
    setShowAddEmailModal(true);
  };

  const handleCloseEmailModal = () => {
    setShowAddEmailModal(false);
    setModalEmail("");
    setModalError(null);
  };

  const handleSubmitEmail = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = modalEmail.trim();
    if (!trimmed) {
      setModalError("Por favor ingresa un correo electrónico.");
      return;
    }
    if (!trimmed.includes("@") || !trimmed.includes(".")) {
      setModalError("Por favor ingresa un correo electrónico válido.");
      return;
    }

    setModalError(null);
    const success = await saveExtraEmail(trimmed);
    if (success) {
      handleCloseEmailModal();
    } else {
      // Keep modal open and display error
      setModalError(
        error || "No se pudo guardar el correo electrónico adicional.",
      );
    }
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
          Alertas de seguridad
        </span>
      </nav>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
            Alertas de seguridad
          </h1>
          <p className="text-sm text-muted-foreground">
            Te avisaremos inmediatamente si detectamos movimientos inusuales en tu
            cuenta de EspaciGo o accesos desde nuevos dispositivos.
          </p>
        </div>
        <button
          type="button"
          onClick={() => refresh()}
          disabled={isLoading || isSaving}
          className="inline-flex items-center gap-2 px-3 py-1.5 text-xs font-semibold rounded-xl border border-input bg-background hover:bg-muted transition-colors text-foreground self-start sm:self-auto disabled:opacity-50"
          title="Actualizar alertas"
        >
          <RefreshCw className={cn("size-3.5", isLoading && "animate-spin")} />
          Actualizar
        </button>
      </div>

      {/* Notifications / Alerts banner */}
      {notification && (
        <div className="flex items-center justify-between gap-3 p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-800 dark:text-emerald-200 text-sm">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <span>{notification}</span>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="p-1 rounded-lg hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 transition-colors"
            aria-label="Cerrar notificación"
          >
            <X className="size-4" />
          </button>
        </div>
      )}

      {error && !showAddEmailModal && (
        <div className="flex items-center justify-between gap-3 p-4 rounded-xl bg-destructive/10 border border-destructive/20 text-destructive text-sm">
          <div className="flex items-center gap-2">
            <AlertTriangle className="size-4 shrink-0" />
            <span>{error}</span>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="p-1 rounded-lg hover:bg-destructive/20 transition-colors"
            aria-label="Cerrar error"
          >
            <X className="size-4" />
          </button>
        </div>
      )}

      {/* Sección Canales estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div>
          <h2 className="text-lg font-bold text-foreground">Canales de aviso</h2>
          <p className="text-xs text-muted-foreground">
            Selecciona por dónde deseas recibir las notificaciones de seguridad.
          </p>
        </div>

        {isLoading && !preferences ? (
          <div className="py-8 flex items-center justify-center gap-2 text-muted-foreground text-sm">
            <Loader2 className="size-5 animate-spin text-primary" />
            <span>Cargando preferencias de canales...</span>
          </div>
        ) : (
          <div className="divide-y divide-border/60">
            {/* Canal 1: Notificaciones de la app */}
            <div className="py-4 flex items-center justify-between gap-4">
              <div className="flex items-start gap-3.5">
                <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
                  <Bell className="size-5" />
                </div>
                <div className="space-y-0.5">
                  <p className="text-sm sm:text-base font-semibold text-foreground">
                    Notificaciones de la app
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Recibe avisos inmediatos en tu navegador web y dispositivos
                    vinculados.
                  </p>
                </div>
              </div>

              {/* Toggle Terracota */}
              <button
                type="button"
                role="switch"
                aria-checked={preferences?.app_notifications_enabled ?? true}
                aria-label="Notificaciones de la app"
                disabled={isSaving || isLoading}
                onClick={() =>
                  toggleChannel(
                    "app",
                    !(preferences?.app_notifications_enabled ?? true),
                  )
                }
                className={cn(
                  "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 disabled:opacity-50",
                  preferences?.app_notifications_enabled
                    ? "bg-primary"
                    : "bg-muted",
                )}
              >
                <span
                  className={cn(
                    "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                    preferences?.app_notifications_enabled
                      ? "translate-x-5"
                      : "translate-x-0",
                  )}
                />
              </button>
            </div>

            {/* Canal 2: E-mail */}
            <div className="py-4 flex items-start justify-between gap-4">
              <div className="flex items-start gap-3.5">
                <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
                  <Mail className="size-5" />
                </div>
                <div className="space-y-1.5">
                  <div>
                    <p className="text-sm sm:text-base font-semibold text-foreground">
                      E-mail
                    </p>
                    <p className="text-xs text-muted-foreground">
                      Avisos a tu correo principal:{" "}
                      <span className="font-mono text-foreground font-medium">
                        {preferences?.masked_email || "••••@••••.••"}
                      </span>
                    </p>
                  </div>

                  {preferences?.extra_email_configured &&
                  preferences?.masked_extra_email ? (
                    <div className="space-y-1 pt-0.5">
                      <p className="text-xs text-muted-foreground">
                        Correo adicional de respaldo:{" "}
                        <span className="font-mono text-foreground font-medium">
                          {preferences.masked_extra_email}
                        </span>
                      </p>
                      <div className="flex items-center gap-3 pt-0.5">
                        <button
                          type="button"
                          onClick={handleOpenEmailModal}
                          className="inline-flex items-center gap-1 text-xs font-semibold text-primary hover:underline"
                        >
                          Cambiar e-mail adicional
                        </button>
                        <span className="text-border">•</span>
                        <button
                          type="button"
                          onClick={() => removeExtraEmail()}
                          disabled={isSaving}
                          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-destructive transition-colors disabled:opacity-50"
                        >
                          Eliminar
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div className="pt-0.5">
                      <button
                        type="button"
                        onClick={handleOpenEmailModal}
                        className="inline-flex items-center gap-1 text-xs font-semibold text-primary hover:underline"
                      >
                        <Plus className="size-3.5" />
                        Agregar e-mail adicional
                      </button>
                    </div>
                  )}
                </div>
              </div>

              {/* Toggle Terracota */}
              <button
                type="button"
                role="switch"
                aria-checked={preferences?.email_alerts_enabled ?? true}
                aria-label="Alertas por e-mail"
                disabled={isSaving || isLoading}
                onClick={() =>
                  toggleChannel(
                    "email",
                    !(preferences?.email_alerts_enabled ?? true),
                  )
                }
                className={cn(
                  "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 disabled:opacity-50 mt-1",
                  preferences?.email_alerts_enabled
                    ? "bg-primary"
                    : "bg-muted",
                )}
              >
                <span
                  className={cn(
                    "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                    preferences?.email_alerts_enabled
                      ? "translate-x-5"
                      : "translate-x-0",
                  )}
                />
              </button>
            </div>

            {/* Canal 3: WhatsApp */}
            <div className="py-4 flex items-center justify-between gap-4">
              <div className="flex items-start gap-3.5">
                <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
                  <MessageSquare className="size-5" />
                </div>
                <div className="space-y-0.5">
                  <p className="text-sm sm:text-base font-semibold text-foreground">
                    WhatsApp
                  </p>
                  {hasPhone ? (
                    <p className="text-xs text-muted-foreground">
                      Recibe alertas críticas de seguridad vía WhatsApp al
                      teléfono registrado (
                      <span className="font-mono text-foreground font-medium">
                        {preferences?.masked_phone}
                      </span>
                      ).
                    </p>
                  ) : (
                    <div className="space-y-1">
                      <p className="text-xs text-muted-foreground">
                        No tienes un número de teléfono verificado para recibir
                        alertas por WhatsApp.
                      </p>
                      <Link
                        href="/mi-cuenta/seguridad/metodos/telefono"
                        className="inline-flex items-center gap-1 text-xs font-semibold text-primary hover:underline"
                      >
                        Configurar teléfono verificado
                        <ChevronRight className="size-3" />
                      </Link>
                    </div>
                  )}
                </div>
              </div>

              {/* Toggle Terracota */}
              <button
                type="button"
                role="switch"
                aria-checked={
                  hasPhone && (preferences?.whatsapp_alerts_enabled ?? false)
                }
                aria-label="Alertas por WhatsApp"
                disabled={!hasPhone || isSaving || isLoading}
                title={
                  !hasPhone
                    ? "Configura un teléfono primero para habilitar alertas vía WhatsApp"
                    : undefined
                }
                onClick={() => {
                  if (!hasPhone) return;
                  toggleChannel(
                    "whatsapp",
                    !(preferences?.whatsapp_alerts_enabled ?? false),
                  );
                }}
                className={cn(
                  "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 disabled:opacity-40 disabled:cursor-not-allowed",
                  hasPhone && preferences?.whatsapp_alerts_enabled
                    ? "bg-primary"
                    : "bg-muted",
                )}
              >
                <span
                  className={cn(
                    "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                    hasPhone && preferences?.whatsapp_alerts_enabled
                      ? "translate-x-5"
                      : "translate-x-0",
                  )}
                />
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Sección Alertas Recientes estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div>
          <h2 className="text-lg font-bold text-foreground">
            Alertas recientes
          </h2>
          <p className="text-xs text-muted-foreground">
            Registro de auditoría de actividad y eventos de seguridad en tu
            cuenta.
          </p>
        </div>

        {isLoading && alerts.length === 0 ? (
          <div className="py-8 flex items-center justify-center gap-2 text-muted-foreground text-sm">
            <Loader2 className="size-5 animate-spin text-primary" />
            <span>Cargando alertas de seguridad...</span>
          </div>
        ) : alerts.length === 0 ? (
          <div className="py-10 text-center space-y-2">
            <div className="size-12 rounded-2xl bg-muted/60 text-muted-foreground flex items-center justify-center mx-auto">
              <ShieldCheck className="size-6 text-emerald-600 dark:text-emerald-400" />
            </div>
            <p className="text-sm font-semibold text-foreground">
              No hay alertas de seguridad registradas
            </p>
            <p className="text-xs text-muted-foreground max-w-sm mx-auto">
              Tu cuenta está protegida. Cuando se registren inicios de sesión,
              cambios de seguridad o nuevos accesos, aparecerán aquí.
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border/60">
            {alerts.map((alert) => {
              const visual = getEventVisuals(alert.event_code);
              const Icon = visual.icon;
              return (
                <div
                  key={alert.id}
                  className="py-4 flex items-start justify-between gap-4"
                >
                  <div className="flex items-start gap-3.5">
                    <div
                      className={`size-10 rounded-xl flex items-center justify-center shrink-0 mt-0.5 ${visual.iconColor}`}
                    >
                      <Icon className="size-5" />
                    </div>
                    <div className="space-y-0.5">
                      <p className="text-sm font-semibold text-foreground">
                        {alert.title}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {alert.device_label || "Dispositivo"}
                        {alert.ip_masked ? ` • ${alert.ip_masked}` : ""}
                      </p>
                    </div>
                  </div>
                  <span
                    className="text-xs text-muted-foreground whitespace-nowrap cursor-help"
                    title={formatFullDateTime(alert.created_at)}
                  >
                    {formatRelativeTime(alert.created_at)}
                  </span>
                </div>
              );
            })}
          </div>
        )}

        <div className="pt-2 border-t border-border/50 text-xs text-muted-foreground flex items-start gap-2">
          <ShieldAlert className="size-4 text-primary shrink-0 mt-0.5" />
          <p>
            Si detectas alguna actividad que no reconoces, te recomendamos cambiar
            tu contraseña de inmediato y revisar tus dispositivos vinculados.
          </p>
        </div>
      </div>

      {/* Modal para agregar / cambiar email adicional */}
      {showAddEmailModal && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-md w-full p-6 shadow-xl space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-lg font-bold text-foreground">
                  {preferences?.extra_email_configured
                    ? "Cambiar correo electrónico adicional"
                    : "Agregar correo electrónico adicional"}
                </h3>
                <p className="text-xs text-muted-foreground mt-1">
                  Te enviaremos alertas críticas de seguridad a esta casilla de
                  respaldo.
                </p>
              </div>
              <button
                type="button"
                onClick={handleCloseEmailModal}
                className="p-1 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
                aria-label="Cerrar modal"
              >
                <X className="size-4" />
              </button>
            </div>

            <form onSubmit={handleSubmitEmail} className="space-y-4">
              <div>
                <label
                  htmlFor="additionalEmailInput"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Nuevo correo electrónico de respaldo
                </label>
                <input
                  id="additionalEmailInput"
                  type="email"
                  value={modalEmail}
                  onChange={(e) => setModalEmail(e.target.value)}
                  placeholder="seguridad@correo.cl"
                  autoFocus
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
                {modalError && (
                  <p className="text-xs text-destructive mt-1.5 flex items-center gap-1">
                    <AlertTriangle className="size-3.5 shrink-0" />
                    <span>{modalError}</span>
                  </p>
                )}
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={handleCloseEmailModal}
                  disabled={isSaving}
                  className="px-4 py-2 text-xs font-semibold rounded-xl border border-input hover:bg-muted transition-colors text-foreground disabled:opacity-50"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  disabled={isSaving}
                  className="inline-flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                >
                  {isSaving && <Loader2 className="size-3.5 animate-spin" />}
                  <span>Guardar</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
