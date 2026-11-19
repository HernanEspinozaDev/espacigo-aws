"use client";

import Link from "next/link";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Clock,
  Key,
  Lock,
  Mail,
  QrCode,
  ScanFace,
  ShieldAlert,
  Smartphone,
} from "lucide-react";
import { useAccount } from "@/features/account/ui/use-account";
import { maskEmail } from "@/components/reveal-email";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

export default function MetodosVerificacionPage() {
  const { snapshot } = useAccount();
  const { summary } = useSecurityMethodsSummary();

  const profile = snapshot.profile;
  const rawEmail =
    (profile as { email?: string } | null)?.email || "usuario@espacigo.cl";
  const maskedEmailFallback = maskEmail(rawEmail);

  const methodsData = summary?.methods;

  const isPasskeysActive = Boolean(methodsData?.passkeys?.enabled);
  const passkeysCount = methodsData?.passkeys?.count ?? 0;
  const passkeysCountText = isPasskeysActive
    ? `${passkeysCount} ${passkeysCount === 1 ? "llave activa" : "llaves activas"}`
    : "Sin configurar";

  const isPhoneActive = Boolean(methodsData?.phone?.enabled);
  const displayPhoneMasked =
    methodsData?.phone?.masked_phone || "Sin configurar";

  const isPasswordActive = Boolean(methodsData?.password?.enabled ?? true);

  const isEmailActive = Boolean(methodsData?.email?.enabled ?? true);
  const displayEmailMasked =
    methodsData?.email?.masked_email || maskedEmailFallback;

  const isQrActive = Boolean(methodsData?.qr?.enabled);

  const isTotpActive = Boolean(methodsData?.totp?.enabled);

  const isFacialActive = Boolean(methodsData?.facial_kyc?.enabled);

  const methods = [
    {
      id: "llaves-de-acceso",
      title: "Llaves de acceso",
      subtitle:
        "Inicia sesión con tu huella, rostro o PIN con Passkeys / WebAuthn.",
      href: "/mi-cuenta/seguridad/metodos/llaves-de-acceso",
      icon: Key,
      iconColor: isPasskeysActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isPasskeysActive,
      badgeText: isPasskeysActive ? "Activa" : "Sin configurar",
      dataMasked: passkeysCountText,
    },
    {
      id: "telefono",
      title: "Teléfono",
      subtitle:
        "Código de verificación SMS o mensaje a tu número celular.",
      href: "/mi-cuenta/seguridad/metodos/telefono",
      icon: Smartphone,
      iconColor: isPhoneActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isPhoneActive,
      badgeText: isPhoneActive ? "Verificado" : "Sin configurar",
      dataMasked: displayPhoneMasked,
    },
    {
      id: "contrasena",
      title: "Contraseña",
      subtitle:
        "Clave alfanumérica registrada para acceso seguro.",
      href: "/mi-cuenta/seguridad/metodos/contrasena",
      icon: Lock,
      iconColor: isPasswordActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isPasswordActive,
      badgeText: isPasswordActive ? "Activa" : "Sin configurar",
      dataMasked: "••••••••••••••••••",
    },
    {
      id: "email",
      title: "E-mail",
      subtitle:
        "Enlaces mágicos y códigos de seguridad enviados a tu casilla.",
      href: "/mi-cuenta/seguridad/metodos/email",
      icon: Mail,
      iconColor: isEmailActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isEmailActive,
      badgeText: isEmailActive ? "Verificado" : "Sin configurar",
      dataMasked: displayEmailMasked,
    },
    {
      id: "reconocimiento-facial",
      title: "Reconocimiento facial",
      subtitle:
        "Validación biométrica oficial para operaciones y arriendos críticos.",
      href: "/mi-cuenta/seguridad/metodos/reconocimiento-facial",
      icon: ScanFace,
      iconColor: isFacialActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isFacialActive,
      badgeText: "Pendiente módulo KYC",
      dataMasked: "Pendiente módulo KYC",
    },
    {
      id: "codigo-qr",
      title: "Código QR",
      subtitle:
        "Escaneo rápido de autorización desde tu dispositivo móvil de confianza.",
      href: "/mi-cuenta/seguridad/metodos/codigo-qr",
      icon: QrCode,
      iconColor: isQrActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isQrActive,
      badgeText: isQrActive ? "Activo" : "Sin configurar",
      dataMasked: isQrActive ? "Dispositivo vinculado" : "Sin configurar",
    },
    {
      id: "google-authenticator",
      title: "Google Authenticator",
      subtitle:
        "Códigos dinámicos de 6 dígitos mediante aplicación de autenticación TOTP.",
      href: "/mi-cuenta/seguridad/metodos/google-authenticator",
      icon: ShieldAlert,
      iconColor: isTotpActive
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isTotpActive,
      badgeText: isTotpActive ? "Activo" : "Sin configurar",
      dataMasked: isTotpActive ? "Configurado con TOTP" : "Sin configurar",
    },
  ];

  const isProtected = summary?.protected ?? false;
  const activeCount = summary?.active_count ?? methods.filter((m) => m.active).length;
  const totalCount = summary?.total_count ?? methods.length;
  const progressPercent = Math.round((activeCount / totalCount) * 100);

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
          Métodos de verificación
        </span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Configura tus métodos de verificación
        </h1>
        <p className="text-sm text-muted-foreground">
          Elige qué métodos quieres usar para confirmar que eres tú cuando sea
          necesario.
        </p>
      </div>

      {/* Banner superior de estado */}
      <div
        className={
          isProtected
            ? "bg-emerald-500/10 border border-emerald-500/20 rounded-2xl p-5 sm:p-6 shadow-sm space-y-4"
            : "bg-amber-500/10 border border-amber-500/20 rounded-2xl p-5 sm:p-6 shadow-sm space-y-4"
        }
      >
        <div className="flex items-start gap-4">
          <div
            className={`size-10 rounded-xl flex items-center justify-center shrink-0 mt-0.5 ${
              isProtected
                ? "bg-emerald-500/20 text-emerald-600"
                : "bg-amber-500/20 text-amber-600"
            }`}
          >
            {isProtected ? (
              <CheckCircle2 className="size-6" />
            ) : (
              <AlertCircle className="size-6" />
            )}
          </div>
          <div className="space-y-1">
            <h2 className="text-base sm:text-lg font-bold text-foreground">
              {isProtected
                ? "Tu cuenta está protegida porque tienes los métodos necesarios."
                : "Configura métodos de verificación para mayor protección."}
            </h2>
            <p className="text-xs sm:text-sm text-muted-foreground">
              {isProtected
                ? `Tienes ${activeCount} de ${totalCount} métodos de verificación activos. Puedes configurar métodos adicionales para aumentar tu protección y simplificar tu inicio de sesión.`
                : `Tienes ${activeCount} de ${totalCount} métodos de verificación activos. Se recomienda activar al menos un método adicional para proteger completamente tu cuenta.`}
            </p>
          </div>
        </div>

        {/* Barra de progreso */}
        <div className="space-y-1.5 pt-1">
          <div className="flex justify-between text-xs font-medium text-muted-foreground">
            <span>
              Progreso de métodos ({activeCount}/{totalCount})
            </span>
            <span className="text-primary font-semibold">
              {progressPercent}%
            </span>
          </div>
          <div className="w-full h-2 rounded-full bg-muted overflow-hidden">
            <div
              className={`h-full transition-all rounded-full ${
                isProtected ? "bg-emerald-600" : "bg-amber-500"
              }`}
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>
      </div>

      {/* Lista de los 7 métodos exactos */}
      <div className="bg-card border border-border rounded-2xl divide-y divide-border/60 shadow-sm overflow-hidden">
        {methods.map((method) => {
          const Icon = method.icon;
          return (
            <Link
              key={method.id}
              href={method.href}
              className="flex items-center justify-between p-5 hover:bg-muted/40 transition-colors group"
            >
              <div className="flex items-center gap-4 min-w-0 pr-4">
                <div
                  className={`size-11 rounded-xl flex items-center justify-center shrink-0 ${method.iconColor}`}
                >
                  <Icon className="size-5" />
                </div>
                <div className="min-w-0 space-y-0.5">
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm sm:text-base font-semibold text-foreground group-hover:text-primary transition-colors">
                      {method.title}
                    </h3>
                  </div>
                  <p className="text-xs sm:text-sm text-muted-foreground line-clamp-1">
                    {method.subtitle}
                  </p>
                  <p className="text-xs font-mono text-muted-foreground">
                    {method.dataMasked}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 shrink-0">
                {method.active ? (
                  <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                    <CheckCircle2 className="size-3.5" />
                    {method.badgeText}
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground bg-muted px-2 py-0.5 rounded-full">
                    <Clock className="size-3" />
                    {method.badgeText}
                  </span>
                )}
                <ChevronRight className="size-5 text-muted-foreground group-hover:text-primary group-hover:translate-x-0.5 transition-all" />
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
