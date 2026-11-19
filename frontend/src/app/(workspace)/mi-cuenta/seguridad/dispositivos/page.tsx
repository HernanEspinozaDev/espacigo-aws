"use client";

import { useState } from "react";
import Link from "next/link";
import {
  CheckCircle2,
  ChevronRight,
  Info,
  Laptop,
  LogOut,
  Smartphone,
} from "lucide-react";

interface DeviceSession {
  id: string;
  name: string;
  browser: string;
  location: string;
  lastActive: string;
  isCurrent: boolean;
  icon: typeof Laptop;
}

export default function DispositivosSeguridadPage() {
  const [sessions, setSessions] = useState<DeviceSession[]>([
    {
      id: "curr",
      name: "Este dispositivo",
      browser: "Chrome en Linux",
      location: "Santiago, Chile",
      lastActive: "Activo ahora",
      isCurrent: true,
      icon: Laptop,
    },
    {
      id: "dev-2",
      name: "iPhone 15",
      browser: "Safari Mobile • iOS 18",
      location: "Valparaíso, Chile",
      lastActive: "Hace 2 días",
      isCurrent: false,
      icon: Smartphone,
    },
    {
      id: "dev-3",
      name: "MacBook Pro",
      browser: "Chrome • macOS",
      location: "Santiago, Chile",
      lastActive: "Hace 5 días",
      isCurrent: false,
      icon: Laptop,
    },
  ]);

  const [notification, setNotification] = useState<string | null>(null);

  const handleRevoke = (id: string) => {
    setSessions((prev) => prev.filter((s) => s.id !== id));
    setNotification("La sesión ha sido revocada exitosamente.");
    setTimeout(() => setNotification(null), 4000);
  };

  const handleRevokeAllOthers = () => {
    setSessions((prev) => prev.filter((s) => s.isCurrent));
    setNotification("Se cerraron todas las demás sesiones activas.");
    setTimeout(() => setNotification(null), 4000);
  };

  const otherSessions = sessions.filter((s) => !s.isCurrent);

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
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Dispositivos vinculados
        </h1>
        <p className="text-sm text-muted-foreground">
          Revisa y gestiona los navegadores y dispositivos que tienen acceso a tu
          cuenta de EspaciGo.
        </p>
      </div>

      {notification && (
        <div
          role="status"
          className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs sm:text-sm flex items-center gap-2"
        >
          <CheckCircle2 className="size-4 shrink-0" />
          <span>{notification}</span>
        </div>
      )}

      {/* Sesión actual */}
      <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4">
        <h2 className="text-base font-bold text-foreground">
          Sesión en este dispositivo
        </h2>
        <div className="flex items-center justify-between gap-4 p-4 rounded-xl bg-muted/30 border border-border/60">
          <div className="flex items-center gap-4 min-w-0">
            <div className="size-12 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
              <Laptop className="size-6" />
            </div>
            <div className="space-y-0.5 min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold text-foreground">
                  Navegador web actual
                </span>
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3" />
                  Activa ahora
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                Chrome en Linux • Santiago, Chile
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Otras sesiones */}
      <div className="bg-card border border-border rounded-2xl p-6 shadow-sm space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <div>
            <h2 className="text-base font-bold text-foreground">
              Otras sesiones iniciadas
            </h2>
            <p className="text-xs text-muted-foreground">
              Dispositivos en los que iniciaste sesión previamente.
            </p>
          </div>
          {otherSessions.length > 0 && (
            <button
              type="button"
              onClick={handleRevokeAllOthers}
              className="inline-flex items-center gap-1.5 text-xs font-semibold text-destructive hover:underline"
            >
              <LogOut className="size-3.5" />
              Cerrar todas las demás sesiones
            </button>
          )}
        </div>

        {otherSessions.length === 0 ? (
          <p className="text-xs text-muted-foreground py-4 text-center">
            No tienes otras sesiones abiertas en este momento.
          </p>
        ) : (
          <div className="divide-y divide-border/60">
            {otherSessions.map((session) => {
              const Icon = session.icon;
              return (
                <div
                  key={session.id}
                  className="py-4 flex items-center justify-between gap-4"
                >
                  <div className="flex items-center gap-3.5 min-w-0">
                    <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
                      <Icon className="size-5" />
                    </div>
                    <div className="space-y-0.5 min-w-0">
                      <p className="text-sm font-semibold text-foreground">
                        {session.name}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {session.browser} • {session.location} •{" "}
                        <span className="italic">{session.lastActive}</span>
                      </p>
                    </div>
                  </div>

                  <button
                    type="button"
                    onClick={() => handleRevoke(session.id)}
                    className="text-xs font-semibold text-primary hover:text-primary/80 transition-colors whitespace-nowrap px-3 py-1.5 rounded-lg hover:bg-primary/10"
                  >
                    Cerrar sesión
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Nota informativa de fase */}
      <div className="p-4 rounded-xl bg-muted/40 border border-border/60 text-xs text-muted-foreground flex items-start gap-3">
        <Info className="size-4 text-primary shrink-0 mt-0.5" />
        <p>
          <strong>Plan de seguridad EspaciGo (Fase 2):</strong> La revocación
          de sesiones invalida de forma atómica los tokens Bearer emitidos en AWS
          ECS y PostgreSQL bajo control de acceso de sesión.
        </p>
      </div>
    </div>
  );
}
