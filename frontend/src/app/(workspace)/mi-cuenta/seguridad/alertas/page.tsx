"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Bell,
  CheckCircle2,
  ChevronRight,
  Info,
  Mail,
  MessageSquare,
  Plus,
  ShieldAlert,
  ShieldCheck,
} from "lucide-react";
import { useAccount } from "@/features/account/ui/use-account";
import { maskEmail, maskPhone } from "@/components/reveal-email";
import { cn } from "@/lib/utils";

export default function AlertasSeguridadPage() {
  const { snapshot } = useAccount();
  const profile = snapshot.profile;
  const rawEmail =
    (profile as { email?: string } | null)?.email || "usuario@espacigo.cl";
  const maskedEmailValue = maskEmail(rawEmail);
  const maskedPhoneValue = profile?.phone
    ? maskPhone(profile.phone)
    : "+56 •••• 5678";

  // Channel toggles state
  const [appNotifications, setAppNotifications] = useState(true);
  const [emailAlerts, setEmailAlerts] = useState(true);
  const [whatsappAlerts, setWhatsappAlerts] = useState(false);

  // Additional email dialog state
  const [showAddEmailModal, setShowAddEmailModal] = useState(false);
  const [additionalEmail, setAdditionalEmail] = useState("");
  const [addedEmails, setAddedEmails] = useState<string[]>([]);
  const [emailMessage, setEmailMessage] = useState<string | null>(null);

  const handleAddEmail = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = additionalEmail.trim();
    if (!trimmed || !trimmed.includes("@")) {
      setEmailMessage("Por favor ingresa un correo electrónico válido.");
      return;
    }
    setAddedEmails((prev) => [...prev, trimmed]);
    setAdditionalEmail("");
    setEmailMessage(null);
    setShowAddEmailModal(false);
  };

  const recentAlerts = [
    {
      id: "alert-1",
      title: "Inicio de sesión exitoso",
      device: "Navegador Web (Chrome / Linux)",
      location: "Santiago, Chile",
      time: "Hace 2 horas",
      type: "success",
      icon: CheckCircle2,
      iconColor: "text-emerald-600 bg-emerald-500/10",
    },
    {
      id: "alert-2",
      title: "Contraseña verificada",
      device: "Portal de autenticación EspaciGo",
      location: "Sesión activa",
      time: "Hace 3 días",
      type: "info",
      icon: ShieldCheck,
      iconColor: "text-emerald-600 bg-emerald-500/10",
    },
    {
      id: "alert-3",
      title: "Nueva sesión autorizada",
      device: "Navegador Web (Firefox / MacOS)",
      location: "Valparaíso, Chile",
      time: "Hace 1 semana",
      type: "neutral",
      icon: Info,
      iconColor: "text-primary bg-primary/10",
    },
  ];

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
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Alertas de seguridad
        </h1>
        <p className="text-sm text-muted-foreground">
          Te avisaremos inmediatamente si detectamos movimientos inusuales en tu
          cuenta de EspaciGo o accesos desde nuevos dispositivos.
        </p>
      </div>

      {/* Sección Canales estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div>
          <h2 className="text-lg font-bold text-foreground">Canales de aviso</h2>
          <p className="text-xs text-muted-foreground">
            Selecciona por dónde deseas recibir las notificaciones de seguridad.
          </p>
        </div>

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
              aria-checked={appNotifications}
              onClick={() => setAppNotifications((prev) => !prev)}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
                appNotifications ? "bg-primary" : "bg-muted"
              )}
            >
              <span
                className={cn(
                  "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                  appNotifications ? "translate-x-5" : "translate-x-0"
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
                      {maskedEmailValue}
                    </span>
                  </p>
                </div>

                {addedEmails.length > 0 && (
                  <div className="space-y-1">
                    <p className="text-xs font-semibold text-foreground">
                      Correos adicionales configurados:
                    </p>
                    {addedEmails.map((em, idx) => (
                      <p
                        key={idx}
                        className="text-xs font-mono text-muted-foreground"
                      >
                        • {maskEmail(em)}
                      </p>
                    ))}
                  </div>
                )}

                <button
                  type="button"
                  onClick={() => setShowAddEmailModal(true)}
                  className="inline-flex items-center gap-1 text-xs font-semibold text-primary hover:underline"
                >
                  <Plus className="size-3.5" />
                  Agregar e-mail adicional
                </button>
              </div>
            </div>

            {/* Toggle Terracota */}
            <button
              type="button"
              role="switch"
              aria-checked={emailAlerts}
              onClick={() => setEmailAlerts((prev) => !prev)}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 mt-1",
                emailAlerts ? "bg-primary" : "bg-muted"
              )}
            >
              <span
                className={cn(
                  "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                  emailAlerts ? "translate-x-5" : "translate-x-0"
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
                <p className="text-xs text-muted-foreground">
                  Recibe alertas críticas de seguridad vía WhatsApp al teléfono
                  registrado ({maskedPhoneValue}).
                </p>
              </div>
            </div>

            {/* Toggle Terracota */}
            <button
              type="button"
              role="switch"
              aria-checked={whatsappAlerts}
              onClick={() => setWhatsappAlerts((prev) => !prev)}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
                whatsappAlerts ? "bg-primary" : "bg-muted"
              )}
            >
              <span
                className={cn(
                  "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                  whatsappAlerts ? "translate-x-5" : "translate-x-0"
                )}
              />
            </button>
          </div>
        </div>
      </div>

      {/* Sección Alertas Recientes estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div>
          <h2 className="text-lg font-bold text-foreground">Alertas recientes</h2>
          <p className="text-xs text-muted-foreground">
            Registro de auditoría de actividad reciente en tu cuenta.
          </p>
        </div>

        <div className="divide-y divide-border/60">
          {recentAlerts.map((alert) => {
            const Icon = alert.icon;
            return (
              <div
                key={alert.id}
                className="py-4 flex items-start justify-between gap-4"
              >
                <div className="flex items-start gap-3.5">
                  <div
                    className={`size-10 rounded-xl flex items-center justify-center shrink-0 mt-0.5 ${alert.iconColor}`}
                  >
                    <Icon className="size-5" />
                  </div>
                  <div className="space-y-0.5">
                    <p className="text-sm font-semibold text-foreground">
                      {alert.title}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {alert.device} • {alert.location}
                    </p>
                  </div>
                </div>
                <span className="text-xs text-muted-foreground whitespace-nowrap">
                  {alert.time}
                </span>
              </div>
            );
          })}
        </div>

        <div className="pt-2 border-t border-border/50 text-xs text-muted-foreground flex items-start gap-2">
          <ShieldAlert className="size-4 text-primary shrink-0 mt-0.5" />
          <p>
            Si detectas alguna actividad que no reconoces, te recomendamos cambiar
            tu contraseña de inmediato y revisar tus dispositivos vinculados.
          </p>
        </div>
      </div>

      {/* Modal para agregar email adicional */}
      {showAddEmailModal && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-md w-full p-6 shadow-xl space-y-4">
            <h3 className="text-lg font-bold text-foreground">
              Agregar correo electrónico adicional
            </h3>
            <p className="text-xs text-muted-foreground">
              Te enviaremos un código de confirmación a esta casilla antes de
              habilitarla para alertas críticas de seguridad.
            </p>

            <form onSubmit={handleAddEmail} className="space-y-4">
              <div>
                <label
                  htmlFor="additionalEmailInput"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Nuevo correo electrónico
                </label>
                <input
                  id="additionalEmailInput"
                  type="email"
                  value={additionalEmail}
                  onChange={(e) => setAdditionalEmail(e.target.value)}
                  placeholder="ejemplo@correo.cl"
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
                {emailMessage && (
                  <p className="text-xs text-destructive mt-1">
                    {emailMessage}
                  </p>
                )}
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowAddEmailModal(false)}
                  className="px-4 py-2 text-xs font-semibold rounded-xl border border-input hover:bg-muted transition-colors text-foreground"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
                >
                  Guardar y verificar
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
