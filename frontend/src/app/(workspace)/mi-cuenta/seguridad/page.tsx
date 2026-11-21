"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Bell,
  CheckCircle2,
  ChevronRight,
  Clock,
  HelpCircle,
  MapPin,
  ShieldAlert,
  ShieldCheck,
  Smartphone,
  Users,
} from "lucide-react";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

export default function SeguridadPage() {
  const [showReportDialog, setShowReportDialog] = useState(false);
  const [showSafetyTips, setShowSafetyTips] = useState(false);
  const { summary } = useSecurityMethodsSummary();

  const securityItems = [
    {
      title: "Métodos de verificación",
      subtitle:
        "Elige cómo confirmar que eres tú al iniciar sesión o realizar acciones sensibles.",
      href: "/mi-cuenta/seguridad/metodos",
      icon: ShieldCheck,
      iconColor: summary?.protected
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      status: summary?.protected ? (
        <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
          <CheckCircle2 className="size-3.5" />
          Protegida ({summary.active_count}/{summary.total_count})
        </span>
      ) : summary ? (
        <span className="inline-flex items-center gap-1 text-xs font-semibold text-amber-600 bg-amber-500/10 px-2 py-0.5 rounded-full border border-amber-500/20">
          <Clock className="size-3.5" />
          Pendiente ({summary.active_count}/{summary.total_count})
        </span>
      ) : (
        <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
          <CheckCircle2 className="size-3.5" />
          Activo
        </span>
      ),
    },
    {
      title: "Dispositivos vinculados",
      subtitle:
        "Administra las sesiones activas en tus navegadores y teléfonos móviles.",
      href: "/mi-cuenta/seguridad/dispositivos",
      icon: Smartphone,
      iconColor: "text-primary bg-primary/10",
      status: null,
    },
    {
      title: "Alertas de seguridad",
      subtitle:
        "Notificaciones sobre inicios de sesión inusuales y actividad sospechosa.",
      href: "/mi-cuenta/seguridad/alertas",
      icon: Bell,
      iconColor: "text-primary bg-primary/10",
      status: null,
    },
    {
      title: "Permiso de ubicación",
      subtitle:
        "Ayuda a prevenir accesos no autorizados validando tu ubicación habitual.",
      href: "/mi-cuenta/seguridad/ubicacion",
      icon: MapPin,
      iconColor: "text-primary bg-primary/10",
      status: null,
    },
    {
      title: "Personas de confianza",
      subtitle:
        "Contactos de respaldo para recuperar el acceso a tu cuenta en caso de emergencia.",
      href: "/mi-cuenta/seguridad/personas-confianza",
      icon: Users,
      iconColor: "text-primary bg-primary/10",
      status: null,
    },
  ];

  return (
    <div className="space-y-8">
      {/* Breadcrumb estilo Mercado Pago */}
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
        <span className="text-foreground font-semibold">Seguridad</span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Seguridad
        </h1>
        <p className="text-sm text-muted-foreground">
          Configuraciones y herramientas para proteger tu cuenta y tus
          transacciones en EspaciGo.
        </p>
      </div>

      {/* Lista de 5 ítems estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl divide-y divide-border/60 shadow-sm overflow-hidden">
        {securityItems.map((item) => {
          const Icon = item.icon;
          return (
            <Link
              key={item.href}
              href={item.href}
              className="flex items-center justify-between p-5 hover:bg-muted/40 transition-colors group"
            >
              <div className="flex items-center gap-4 min-w-0 pr-4">
                <div
                  className={`size-11 rounded-xl flex items-center justify-center shrink-0 ${item.iconColor}`}
                >
                  <Icon className="size-5" />
                </div>
                <div className="min-w-0 space-y-0.5">
                  <h2 className="text-sm sm:text-base font-semibold text-foreground group-hover:text-primary transition-colors">
                    {item.title}
                  </h2>
                  <p className="text-xs sm:text-sm text-muted-foreground line-clamp-2">
                    {item.subtitle}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 shrink-0">
                {item.status}
                <ChevronRight className="size-5 text-muted-foreground group-hover:text-primary group-hover:translate-x-0.5 transition-all" />
              </div>
            </Link>
          );
        })}
      </div>

      {/* Enlaces inferiores de soporte y buenas prácticas */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-4 pt-2 text-xs sm:text-sm text-muted-foreground">
        <button
          type="button"
          onClick={() => setShowReportDialog(true)}
          className="inline-flex items-center gap-1.5 text-primary hover:underline font-medium"
        >
          <ShieldAlert className="size-4" />
          Reportar un problema de seguridad
        </button>

        <button
          type="button"
          onClick={() => setShowSafetyTips(true)}
          className="inline-flex items-center gap-1.5 text-primary hover:underline font-medium"
        >
          <HelpCircle className="size-4" />
          Conoce cómo usar tu cuenta de forma segura
        </button>
      </div>

      {/* Modal / Panel informativo: Reportar un problema de seguridad */}
      {showReportDialog && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-md w-full p-6 shadow-xl space-y-4">
            <div className="flex items-center gap-3 text-primary">
              <ShieldAlert className="size-6" />
              <h3 className="text-lg font-bold text-foreground">
                Reportar un problema de seguridad
              </h3>
            </div>
            <p className="text-sm text-muted-foreground">
              Si detectaste actividad sospechosa, inicios de sesión no
              autorizados o vulnerabilidades, nuestro equipo de ciberseguridad
              está disponible 24/7.
            </p>
            <div className="bg-muted/50 rounded-xl p-3 text-xs text-foreground space-y-1">
              <p>
                <strong>Canal directo:</strong> seguridad@espacigo.cl
              </p>
              <p>
                <strong>Tiempo estimado de respuesta:</strong> menos de 15
                minutos.
              </p>
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setShowReportDialog(false)}
                className="px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                Entendido
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Modal / Panel informativo: Buenas prácticas de seguridad */}
      {showSafetyTips && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-lg w-full p-6 shadow-xl space-y-4">
            <div className="flex items-center gap-3 text-emerald-600">
              <ShieldCheck className="size-6" />
              <h3 className="text-lg font-bold text-foreground">
                Usa tu cuenta de forma segura
              </h3>
            </div>
            <ul className="text-xs sm:text-sm text-muted-foreground space-y-2 list-disc list-inside">
              <li>
                Nunca compartas tus contraseñas ni códigos de verificación por
                WhatsApp o teléfono.
              </li>
              <li>
                EspaciGo nunca te pedirá contraseñas por correo ni enlaces
                externos.
              </li>
              <li>
                Utiliza contraseñas robustas con letras, números y símbolos.
              </li>
              <li>
                Revisa periódicamente tus dispositivos vinculados y cierra
                sesiones inactivas.
              </li>
            </ul>
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setShowSafetyTips(false)}
                className="px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                Cerrar
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
