"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
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
import { useAuth } from "@/features/auth/ui/use-auth";
import { maskEmail, maskPhone } from "@/components/reveal-email";
import {
  type AccountPhoneStatus,
  getAccountPhoneStatus,
} from "@/features/account/phone-verification-api";
import {
  type AccountTOTPStatus,
  getAccountTOTPStatus,
} from "@/features/account/totp-api";
import {
  type AccountPasskey,
  listAccountPasskeys,
} from "@/features/account/passkeys-api";
import {
  type AccountQRStatus,
  getAccountQRStatus,
} from "@/features/account/qr-verification-api";

export default function MetodosVerificacionPage() {
  const { snapshot: authSnapshot } = useAuth();
  const { snapshot } = useAccount();
  const [phoneStatus, setPhoneStatus] = useState<AccountPhoneStatus | null>(null);
  const [totpStatus, setTotpStatus] = useState<AccountTOTPStatus | null>(null);
  const [passkeys, setPasskeys] = useState<AccountPasskey[]>([]);
  const [qrStatus, setQrStatus] = useState<AccountQRStatus | null>(null);

  useEffect(() => {
    if (authSnapshot.status !== "authenticated") return;
    const controller = new AbortController();
    getAccountPhoneStatus(controller.signal)
      .then((data) => setPhoneStatus(data))
      .catch(() => {
        // Silently preserve defaults on network or session interruption
      });
    getAccountTOTPStatus(controller.signal)
      .then((data) => setTotpStatus(data))
      .catch(() => {
        // Silently preserve defaults on network or session interruption
      });
    listAccountPasskeys(controller.signal)
      .then((data) => setPasskeys(data))
      .catch(() => {
        // Silently preserve defaults on network or session interruption
      });
    getAccountQRStatus(controller.signal)
      .then((data) => setQrStatus(data))
      .catch(() => {
        // Silently preserve defaults on network or session interruption
      });
    return () => controller.abort();
  }, [authSnapshot.status]);

  const profile = snapshot.profile;
  const rawEmail =
    (profile as { email?: string } | null)?.email || "usuario@espacigo.cl";
  const maskedEmailValue = maskEmail(rawEmail);
  const maskedPhoneValue = profile?.phone
    ? maskPhone(profile.phone)
    : "+56 •••• 5678";

  const isPhoneVerified = Boolean(phoneStatus?.verified);
  const isPhonePending = Boolean(phoneStatus?.pending_verification);
  const displayPhoneMasked = phoneStatus?.masked_phone || maskedPhoneValue;

  const isTotpActive = Boolean(totpStatus?.enabled);
  const totpRemainingCodes = totpStatus?.remaining_backup_codes ?? 0;
  const totpMasked = isTotpActive
    ? `${totpRemainingCodes} códigos de respaldo`
    : "Sin configurar";

  const isPasskeysActive = passkeys.length > 0;
  const passkeysCountText = isPasskeysActive
    ? `${passkeys.length} ${passkeys.length === 1 ? "llave activa" : "llaves activas"}`
    : "Sin configurar";

  const isQrActive = Boolean(qrStatus?.enabled);
  const qrDeviceLabel = qrStatus?.trusted_device_label || "Dispositivo vinculado";

  const phoneBadgeText = isPhoneVerified
    ? "Verificado"
    : isPhonePending
      ? "Pendiente"
      : "Sin configurar";

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
      iconColor: isPhoneVerified
        ? "text-emerald-600 bg-emerald-500/10"
        : "text-primary bg-primary/10",
      active: isPhoneVerified,
      badgeText: phoneBadgeText,
      dataMasked: isPhoneVerified || isPhonePending
        ? displayPhoneMasked
        : (profile?.phone ? maskedPhoneValue : "Sin configurar"),
    },
    {
      id: "contrasena",
      title: "Contraseña",
      subtitle:
        "Clave alfanumérica registrada para acceso seguro.",
      href: "/mi-cuenta/seguridad/metodos/contrasena",
      icon: Lock,
      iconColor: "text-emerald-600 bg-emerald-500/10",
      active: true,
      badgeText: "Activa",
      dataMasked: "••••••••••••••••••",
    },
    {
      id: "email",
      title: "E-mail",
      subtitle:
        "Enlaces mágicos y códigos de seguridad enviados a tu casilla.",
      href: "/mi-cuenta/seguridad/metodos/email",
      icon: Mail,
      iconColor: "text-emerald-600 bg-emerald-500/10",
      active: true,
      badgeText: "Verificado",
      dataMasked: maskedEmailValue,
    },
    {
      id: "reconocimiento-facial",
      title: "Reconocimiento facial",
      subtitle:
        "Validación biométrica oficial para operaciones y arriendos críticos.",
      href: "/mi-cuenta/seguridad/metodos/reconocimiento-facial",
      icon: ScanFace,
      iconColor: "text-primary bg-primary/10",
      active: false,
      badgeText: "Próximamente",
      dataMasked: "Fase 8 (KYC)",
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
      dataMasked: isQrActive ? qrDeviceLabel : "Sin configurar",
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
      dataMasked: totpMasked,
    },
  ];

  const activeCount = methods.filter((m) => m.active).length;
  const totalCount = methods.length;
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
      <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-2xl p-5 sm:p-6 shadow-sm space-y-4">
        <div className="flex items-start gap-4">
          <div className="size-10 rounded-xl bg-emerald-500/20 text-emerald-600 flex items-center justify-center shrink-0 mt-0.5">
            <CheckCircle2 className="size-6" />
          </div>
          <div className="space-y-1">
            <h2 className="text-base sm:text-lg font-bold text-foreground">
              Tu cuenta está protegida porque tienes los métodos necesarios.
            </h2>
            <p className="text-xs sm:text-sm text-muted-foreground">
              Tienes {activeCount} de {totalCount} métodos de verificación
              activos ({activeCount === 2 ? "Contraseña y E-mail" : activeCount === 3 ? "Teléfono, Contraseña y E-mail" : "Múltiples métodos"}). Puedes configurar
              métodos adicionales para aumentar tu protección y simplificar tu
              inicio de sesión.
            </p>
          </div>
        </div>

        {/* Barra de progreso */}
        <div className="space-y-1.5 pt-1">
          <div className="flex justify-between text-xs font-medium text-muted-foreground">
            <span>Progreso de métodos ({activeCount}/{totalCount})</span>
            <span className="text-primary font-semibold">{progressPercent}%</span>
          </div>
          <div className="w-full h-2 rounded-full bg-muted overflow-hidden">
            <div
              className="h-full bg-emerald-600 transition-all rounded-full"
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
