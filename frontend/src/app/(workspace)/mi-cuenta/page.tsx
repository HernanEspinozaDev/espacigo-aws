"use client";

import { useEffect } from "react";
import Link from "next/link";
import {
  CheckCircle2,
  ChevronRight,
  Clock,
  CreditCard,
  FileText,
  PenLine,
  Shield,
  ShieldCheck,
  User,
} from "lucide-react";
import { useAccount } from "@/features/account/ui/use-account";
import { useAuth } from "@/features/auth/ui/use-auth";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";
import { useIdentityProfile } from "@/features/identity-profile/ui/use-identity-profile";

export default function MiCuentaPage() {
  const { snapshot, loadOverview, loadProfilePhoto } = useAccount();
  const { snapshot: authSnapshot } = useAuth();
  const { snapshot: identityProfile } = useIdentityProfile();
  const { summary: securitySummary } = useSecurityMethodsSummary();

  useEffect(() => {
    if (authSnapshot.status === "authenticated") {
      loadOverview().catch(() => {});
      loadProfilePhoto().catch(() => {});
    }
  }, [authSnapshot.status, loadOverview, loadProfilePhoto]);

  const displayName =
    identityProfile.summary?.username?.trim() ||
    (identityProfile.loading ? "Cargando perfil…" : "Completa tu perfil");
  const maskedUserEmail =
    identityProfile.summary?.masked_email || "Oculto hasta completar el perfil";
  const maskedUserPhone =
    identityProfile.summary?.masked_phone || "No configurado";

  const initials =
    displayName
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part.charAt(0).toUpperCase())
      .join("") || "EG";

  return (
    <div className="space-y-8">
      {/* Encabezado estilo Mercado Pago con identidad terracota de EspaciGo */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6 pb-6 border-b border-border/60">
          <div className="flex items-center gap-4">
            <div className="relative size-16 sm:size-20 rounded-full overflow-hidden bg-primary/10 border-2 border-primary/20 flex items-center justify-center shrink-0">
              {snapshot.photoUrl ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={snapshot.photoUrl}
                  alt={`Foto de perfil de ${displayName}`}
                  className="size-full object-cover"
                />
              ) : (
                <span className="text-xl sm:text-2xl font-bold text-primary">
                  {initials}
                </span>
              )}
            </div>
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-foreground">
                  {displayName}
                </h1>
                <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-600 border border-emerald-500/20">
                  <CheckCircle2 className="size-3" />
                  Activa
                </span>
              </div>
              <p className="text-sm text-muted-foreground flex items-center gap-2">
                <span>E-mail:</span>
                <span className="font-mono text-foreground font-medium">
                  {maskedUserEmail}
                </span>
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 self-stretch sm:self-auto">
            <Link
              href="/mi-cuenta/perfil"
              className="inline-flex items-center justify-center gap-2 text-sm font-medium px-4 py-2 rounded-xl bg-primary/10 text-primary hover:bg-primary/20 transition-colors w-full sm:w-auto"
            >
              <PenLine className="size-4" />
              Editar datos y foto
            </Link>
          </div>
        </div>

        {/* Pestaña única Cuenta con indicador inferior terracota */}
        <div className="flex items-center gap-8 pt-4 -mb-2">
          <span className="pb-3 text-sm font-semibold text-primary border-b-2 border-primary">
            Cuenta
          </span>
        </div>
      </div>

      {/* Grilla limpia de 3 columnas estilo Mercado Pago */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {/* 1. Información de tu perfil */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <User className="size-5" />
              </div>
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Información de tu perfil
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Datos personales, fotografía de perfil y revisión de identidad.
              </p>
            </div>
            <div className="pt-2 text-xs space-y-1 text-muted-foreground border-t border-border/50">
              <p>
                <strong className="text-foreground font-medium">Nombre:</strong>{" "}
                {displayName}
              </p>
              <p>
                <strong className="text-foreground font-medium">
                  Teléfono:
                </strong>{" "}
                <span className="font-mono">{maskedUserPhone}</span>
              </p>
              <p>
                <strong className="text-foreground font-medium">E-mail:</strong>{" "}
                <span className="font-mono">{maskedUserEmail}</span>
              </p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/perfil"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Modificar datos personales</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>

        {/* 2. Seguridad */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div
                className={`size-10 rounded-xl flex items-center justify-center ${
                  securitySummary?.protected
                    ? "bg-emerald-500/10 text-emerald-600"
                    : "bg-amber-500/10 text-amber-600"
                }`}
              >
                <ShieldCheck className="size-5" />
              </div>
              {securitySummary?.protected ? (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3" />
                  Protegida
                </span>
              ) : securitySummary ? (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-amber-600 bg-amber-500/10 px-2 py-0.5 rounded-full border border-amber-500/20">
                  <Clock className="size-3" />
                  Pendiente
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-600 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3" />
                  Protegida
                </span>
              )}
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Seguridad
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Contraseña, métodos de verificación, alertas y sesiones activas.
              </p>
            </div>
            <div className="pt-2 text-xs text-muted-foreground border-t border-border/50">
              <p
                className={
                  securitySummary?.protected
                    ? "text-emerald-700 dark:text-emerald-400 font-medium"
                    : "text-amber-700 dark:text-amber-400 font-medium"
                }
              >
                •{" "}
                {securitySummary
                  ? `${securitySummary.active_count} de ${securitySummary.total_count} métodos de verificación activos`
                  : "2 métodos activos (Contraseña y E-mail)"}
              </p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/seguridad"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Administrar seguridad</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>

        {/* 3. Privacidad */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <Shield className="size-5" />
              </div>
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Privacidad
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Control de datos personales, solicitudes de derechos ARCO y
                exportación.
              </p>
            </div>
            <div className="pt-2 text-xs text-muted-foreground border-t border-border/50">
              <p>
                Ejerce tus derechos conforme a la normativa de protección de
                datos.
              </p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/privacidad"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Gestionar privacidad</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>

        {/* 4. Cuenta de cobro */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <CreditCard className="size-5" />
              </div>
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Cuenta de cobro
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Cuenta bancaria o método de liquidación para anfitriones.
              </p>
            </div>
            <div className="pt-2 text-xs text-muted-foreground border-t border-border/50">
              <p>Configura el destino de recepción de fondos por arriendos.</p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/cobros"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Administrar cobros</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>

        {/* 5. Verificaciones */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <CheckCircle2 className="size-5" />
              </div>
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Verificaciones
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Estado de verificación de identidad y cumplimiento de cuenta.
              </p>
            </div>
            <div className="pt-2 text-xs text-muted-foreground border-t border-border/50">
              <p>Consulta tus validaciones activas o en curso.</p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/perfil#identity-verification"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Ver verificaciones</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>

        {/* 6. Documentos */}
        <div className="bg-card border border-border rounded-2xl p-5 shadow-sm hover:border-primary/40 hover:shadow-md transition-all flex flex-col justify-between">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="size-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <FileText className="size-5" />
              </div>
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-base">
                Documentos
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Expedientes privados por finalidad y comprobantes asociados.
              </p>
            </div>
            <div className="pt-2 text-xs text-muted-foreground border-t border-border/50">
              <p>
                Revisa tu documentación respaldada en almacenamiento seguro.
              </p>
            </div>
          </div>
          <div className="pt-4 mt-2">
            <Link
              href="/mi-cuenta/documentos"
              className="w-full flex items-center justify-between text-xs font-semibold text-primary hover:text-primary/80 transition-colors py-1.5"
            >
              <span>Ver documentos</span>
              <ChevronRight className="size-4" />
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
