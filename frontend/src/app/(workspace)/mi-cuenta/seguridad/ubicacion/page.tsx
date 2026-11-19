"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ChevronRight,
  Info,
  MapPin,
  Shield,
  ShieldCheck,
} from "lucide-react";
import { cn } from "@/lib/utils";

export default function UbicacionSeguridadPage() {
  const [locationEnabled, setLocationEnabled] = useState(false);

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
          Permiso de ubicación
        </span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Permiso de ubicación
        </h1>
        <p className="text-sm text-muted-foreground">
          Ayuda a prevenir accesos no autorizados validando tu ubicación habitual
          al iniciar sesión.
        </p>
      </div>

      {/* Card principal estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div className="flex items-start justify-between gap-4 pb-6 border-b border-border/60">
          <div className="flex items-start gap-4">
            <div className="size-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
              <MapPin className="size-6" />
            </div>
            <div className="space-y-1">
              <h2 className="text-base font-bold text-foreground">
                Detección geográfica de anomalías
              </h2>
              <p className="text-xs sm:text-sm text-muted-foreground">
                Permite verificar si los intentos de acceso coinciden con tus zonas
                habituales de navegación en Chile.
              </p>
            </div>
          </div>

          {/* Toggle Terracota */}
          <button
            type="button"
            role="switch"
            aria-checked={locationEnabled}
            onClick={() => setLocationEnabled((prev) => !prev)}
            className={cn(
              "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
              locationEnabled ? "bg-primary" : "bg-muted"
            )}
          >
            <span
              className={cn(
                "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                locationEnabled ? "translate-x-5" : "translate-x-0"
              )}
            />
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs sm:text-sm text-muted-foreground">
          <div className="flex items-start gap-3">
            <Shield className="size-5 text-primary shrink-0 mt-0.5" />
            <p>
              <strong>Privacidad garantizada:</strong> EspaciGo no almacena
              coordenadas GPS exactas, solo la ciudad y país aproximado mediante
              resolución segura de red.
            </p>
          </div>
          <div className="flex items-start gap-3">
            <ShieldCheck className="size-5 text-emerald-600 shrink-0 mt-0.5" />
            <p>
              <strong>Prevención en tiempo real:</strong> Si alguien intenta
              iniciar sesión desde un país distinto al habitual, solicitaremos un
              código de verificación adicional.
            </p>
          </div>
        </div>
      </div>

      {/* Nota de fase */}
      <div className="p-4 rounded-xl bg-muted/40 border border-border/60 text-xs text-muted-foreground flex items-start gap-3">
        <Info className="size-4 text-primary shrink-0 mt-0.5" />
        <p>
          <strong>Plan de seguridad EspaciGo (Fase 7):</strong> El permiso de
          ubicación es 100% opcional y revocado automáticamente al limpiar los
          permisos del navegador.
        </p>
      </div>
    </div>
  );
}
