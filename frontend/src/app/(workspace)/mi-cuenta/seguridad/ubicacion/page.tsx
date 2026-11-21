"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  Calendar,
  CheckCircle2,
  ChevronRight,
  Compass,
  Info,
  MapPin,
  RefreshCw,
  Shield,
  ShieldCheck,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useLocationPermission } from "@/features/account/ui/use-location-permission";

const PRESET_CHILEAN_REGIONS = [
  "Región Metropolitana, Chile",
  "Región de Valparaíso, Chile",
  "Región del Biobío, Chile",
  "Región de Antofagasta, Chile",
  "Región de Coquimbo, Chile",
  "Región del Maule, Chile",
  "Región de La Araucanía, Chile",
  "Región de Los Lagos, Chile",
  "Región de Tarapacá, Chile",
  "Región de Atacama, Chile",
  "Región de O'Higgins, Chile",
  "Región de Los Ríos, Chile",
  "Región de Arica y Parinacota, Chile",
  "Región de Aysén, Chile",
  "Región de Magallanes, Chile",
  "Región de Ñuble, Chile",
];

export default function UbicacionSeguridadPage() {
  const {
    location,
    isLoading,
    isUpdating,
    error,
    notification,
    updatePermission,
    clearError,
    clearNotification,
  } = useLocationPermission();

  const [userRegionInput, setUserRegionInput] = useState<string | null>(null);
  const regionLabel =
    userRegionInput !== null
      ? userRegionInput
      : location?.last_region_label || "Región Metropolitana, Chile";
  const [regionInputError, setRegionInputError] = useState<string | null>(null);

  const handleToggle = async () => {
    if (isLoading || isUpdating) return;
    clearError();
    clearNotification();
    setRegionInputError(null);

    const nextEnabled = !location?.enabled;
    const finalRegion = nextEnabled
      ? regionLabel.trim() || "Región Metropolitana, Chile"
      : null;

    if (nextEnabled && finalRegion && finalRegion.length > 80) {
      setRegionInputError("La región no puede exceder 80 caracteres.");
      return;
    }

    await updatePermission(nextEnabled, finalRegion);
  };

  const handleDetectApproximateRegion = () => {
    try {
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
      if (
        tz.includes("Santiago") ||
        tz.includes("Chile") ||
        tz.includes("Easter")
      ) {
        setUserRegionInput("Región Metropolitana, Chile");
      } else {
        setUserRegionInput("Región Metropolitana, Chile");
      }
      setRegionInputError(null);
    } catch {
      setUserRegionInput("Región Metropolitana, Chile");
    }
  };

  const handleSaveRegion = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!location?.enabled || isUpdating) return;
    clearError();
    clearNotification();

    const trimmed = regionLabel.trim();
    if (!trimmed) {
      setRegionInputError("Ingresa una región aproximada válida.");
      return;
    }
    if (trimmed.length > 80) {
      setRegionInputError("La región no puede exceder 80 caracteres.");
      return;
    }
    setRegionInputError(null);
    await updatePermission(true, trimmed);
  };

  const formattedConsentDate = location?.consent_updated_at
    ? new Date(location.consent_updated_at).toLocaleString("es-CL", {
        day: "2-digit",
        month: "long",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : null;

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
          Ayuda a prevenir accesos no autorizados validando tu ubicación
          habitual al iniciar sesión.
        </p>
      </div>

      {notification && (
        <div
          role="status"
          className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2">
            <CheckCircle2 className="size-4 shrink-0" />
            <span>{notification}</span>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="text-xs hover:underline"
          >
            Cerrar
          </button>
        </div>
      )}

      {(error || regionInputError) && (
        <div
          role="alert"
          className="p-4 rounded-xl bg-destructive/10 border border-destructive/20 text-destructive text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" />
            <span>{error || regionInputError}</span>
          </div>
          <button
            type="button"
            onClick={() => {
              clearError();
              setRegionInputError(null);
            }}
            className="text-xs hover:underline"
          >
            Cerrar
          </button>
        </div>
      )}

      {/* Card principal estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div className="flex items-start justify-between gap-4 pb-6 border-b border-border/60">
          <div className="flex items-start gap-4">
            <div className="size-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0 mt-0.5">
              <MapPin className="size-6" />
            </div>
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <h2 className="text-base font-bold text-foreground">
                  Detección geográfica de anomalías
                </h2>
                {location?.enabled ? (
                  <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                    <CheckCircle2 className="size-3" />
                    Activado
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground bg-muted px-2 py-0.5 rounded-full">
                    Desactivado
                  </span>
                )}
              </div>
              <p className="text-xs sm:text-sm text-muted-foreground">
                Permite verificar si los intentos de acceso coinciden con tus
                zonas habituales de navegación en Chile.
              </p>
            </div>
          </div>

          {/* Toggle Terracota */}
          <button
            type="button"
            role="switch"
            aria-checked={Boolean(location?.enabled)}
            disabled={isLoading || isUpdating}
            onClick={handleToggle}
            aria-label={
              location?.enabled
                ? "Desactivar permiso de ubicación"
                : "Activar permiso de ubicación"
            }
            className={cn(
              "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed",
              location?.enabled ? "bg-primary" : "bg-muted",
            )}
          >
            <span
              className={cn(
                "pointer-events-none inline-block size-5 transform rounded-full bg-background shadow-lg ring-0 transition duration-200 ease-in-out",
                location?.enabled ? "translate-x-5" : "translate-x-0",
              )}
            />
          </button>
        </div>

        {/* Sección de configuración de Región Habitual */}
        <div className="space-y-4 pt-2">
          <div className="space-y-1">
            <h3 className="text-sm font-semibold text-foreground">
              Región habitual aproximada
            </h3>
            <p className="text-xs text-muted-foreground">
              Indica tu región en Chile para comparar inicios de sesión sin
              exponer coordenadas de geolocalización precisas.
            </p>
          </div>

          <form onSubmit={handleSaveRegion} className="space-y-3">
            <div className="flex flex-col sm:flex-row gap-2">
              <div className="relative flex-1">
                <input
                  type="text"
                  list="chilean-regions-list"
                  value={regionLabel}
                  onChange={(e) => setUserRegionInput(e.target.value)}
                  maxLength={80}
                  placeholder="Ej. Región Metropolitana, Chile"
                  disabled={!location?.enabled || isUpdating}
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50 disabled:bg-muted/40"
                />
                <datalist id="chilean-regions-list">
                  {PRESET_CHILEAN_REGIONS.map((region) => (
                    <option key={region} value={region} />
                  ))}
                </datalist>
              </div>

              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={handleDetectApproximateRegion}
                  disabled={!location?.enabled || isUpdating}
                  className="inline-flex items-center justify-center gap-1.5 px-3.5 py-2 rounded-xl border border-input text-xs font-semibold hover:bg-muted transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-foreground"
                >
                  <Compass className="size-3.5" />
                  <span>Detectar</span>
                </button>

                {location?.enabled && (
                  <button
                    type="submit"
                    disabled={isUpdating}
                    className="inline-flex items-center justify-center gap-1.5 px-4 py-2 rounded-xl bg-primary text-primary-foreground text-xs font-semibold hover:bg-primary/90 transition-colors disabled:opacity-50 shadow-sm"
                  >
                    {isUpdating ? (
                      <>
                        <RefreshCw className="size-3.5 animate-spin" />
                        <span>Guardando...</span>
                      </>
                    ) : (
                      <span>Actualizar región</span>
                    )}
                  </button>
                )}
              </div>
            </div>

            {location?.last_region_label && (
              <p className="text-xs text-muted-foreground flex items-center gap-1.5">
                <MapPin className="size-3.5 text-primary shrink-0" />
                <span>
                  Última región registrada:{" "}
                  <strong className="text-foreground font-medium">
                    {location.last_region_label}
                  </strong>
                </span>
              </p>
            )}

            {formattedConsentDate && (
              <p className="text-xs text-muted-foreground flex items-center gap-1.5">
                <Calendar className="size-3.5 text-emerald-600 shrink-0" />
                <span>
                  Fecha de consentimiento:{" "}
                  <strong className="text-foreground font-medium">
                    {formattedConsentDate}
                  </strong>
                </span>
              </p>
            )}
          </form>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs sm:text-sm text-muted-foreground pt-4 border-t border-border/50">
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
              iniciar sesión desde un país distinto al habitual, solicitaremos
              un código de verificación adicional.
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
