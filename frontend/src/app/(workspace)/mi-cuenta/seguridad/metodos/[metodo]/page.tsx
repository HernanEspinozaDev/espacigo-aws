"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  CheckCircle2,
  ChevronRight,
  Clock,
  Info,
  Key,
  Lock,
  Mail,
  QrCode,
  ScanFace,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { useAccount } from "@/features/account/ui/use-account";
import { maskEmail, maskPhone } from "@/components/reveal-email";

interface MethodConfig {
  title: string;
  subtitle: string;
  icon: typeof Key;
  phase: string;
  active: boolean;
  activeBadge: string;
  description: string;
  technicalDetails: string;
}

const methodConfigs: Record<string, MethodConfig> = {
  "llaves-de-acceso": {
    title: "Llaves de acceso (Passkeys)",
    subtitle:
      "Inicia sesión de forma ultrarrápida y segura usando la huella digital, rostro o PIN de tu dispositivo.",
    icon: Key,
    phase: "Fase 5",
    active: false,
    activeBadge: "Próximamente",
    description:
      "Las llaves de acceso sustituyen las contraseñas tradicionales mediante el estándar criptográfico FIDO2/WebAuthn. Tu clave privada nunca sale de tu dispositivo.",
    technicalDetails:
      "Implementación prevista con WebAuthn API, credenciales públicas almacenadas en PostgreSQL y validación de firmas por hardware seguro.",
  },
  telefono: {
    title: "Teléfono y SMS",
    subtitle:
      "Códigos de seguridad por mensaje de texto para confirmar accesos y operaciones.",
    icon: Smartphone,
    phase: "Fase 1",
    active: false,
    activeBadge: "En desarrollo",
    description:
      "Configura tu número de teléfono móvil para recibir códigos temporales de 6 dígitos cada vez que inicies sesión en un nuevo navegador.",
    technicalDetails:
      "Validación de formato chileno (+56 9), almacenamiento enmascarado conforme a A.8.11, adaptador emulado en local y proveedor SNS en AWS.",
  },
  contrasena: {
    title: "Contraseña",
    subtitle:
      "Clave principal alfanumérica para autenticarte en la plataforma.",
    icon: Lock,
    phase: "Activa",
    active: true,
    activeBadge: "Activa",
    description:
      "Tu cuenta está actualmente protegida con una contraseña robusta almacenada de forma segura.",
    technicalDetails:
      "Hash con algoritmo bcrypt y sal única de 12 rondas en PostgreSQL, sin almacenamiento en texto plano.",
  },
  email: {
    title: "E-mail de seguridad",
    subtitle:
      "Enlaces de confirmación y códigos enviados a tu casilla de correo verificada.",
    icon: Mail,
    phase: "Activa",
    active: true,
    activeBadge: "Verificado",
    description:
      "Tu correo electrónico principal está verificado y activo para recibir alertas inmediatas de seguridad y recuperación de contraseña.",
    technicalDetails:
      "Tokens de verificación con firma criptográfica, expiración estricta y protección contra reutilización.",
  },
  "reconocimiento-facial": {
    title: "Reconocimiento facial biométrico",
    subtitle:
      "Validación biométrica oficial para operaciones y contratos de arriendo de alta sensibilidad.",
    icon: ScanFace,
    phase: "Fase 8 (KYC)",
    active: false,
    activeBadge: "Próximamente",
    description:
      "Permite comprobar que eres el titular legítimo comparando una selfie con tu documento de identidad mediante prueba de vida pasiva.",
    technicalDetails:
      "Integración acotada con el módulo de verificación de identidad KYC cuando esté disponible.",
  },
  "codigo-qr": {
    title: "Código QR",
    subtitle:
      "Autorización de inicio de sesión escaneando un código QR desde tu celular.",
    icon: QrCode,
    phase: "Fase 6",
    active: false,
    activeBadge: "Próximamente",
    description:
      "Inicia sesión en computadores o terminales públicas escaneando un código QR con la cámara de tu teléfono móvil previamente vinculado.",
    technicalDetails:
      "Canal de sincronización cifrado de extremo a extremo con vinculación de corta vida (60 segundos).",
  },
  "google-authenticator": {
    title: "Google Authenticator (TOTP)",
    subtitle:
      "Códigos temporales de 6 dígitos mediante aplicaciones de autenticación TOTP.",
    icon: ShieldAlert,
    phase: "Fase 4",
    active: false,
    activeBadge: "Próximamente",
    description:
      "Genera códigos de seguridad cada 30 segundos usando Google Authenticator, Microsoft Authenticator o cualquier app compatible con RFC 6238.",
    technicalDetails:
      "Secreto TOTP cifrado con keyring AES-256-GCM en el backend Go, con generación de códigos de recuperación de respaldo.",
  },
};

export default function MetodoDetallePage() {
  const params = useParams();
  const rawSlug = (params?.metodo as string) || "";
  const slug = rawSlug.toLowerCase();
  const config = methodConfigs[slug] || {
    title: `Método: ${slug}`,
    subtitle: "Configuración del método de verificación de seguridad.",
    icon: Shield,
    phase: "Próximamente",
    active: false,
    activeBadge: "En preparación",
    description: "Este método forma parte del plan de seguridad de EspaciGo.",
    technicalDetails: "Arquitectura segura conforme a la norma ISO/IEC 27001.",
  };

  const { snapshot } = useAccount();
  const profile = snapshot.profile;
  const rawEmail =
    (profile as { email?: string } | null)?.email || "usuario@espacigo.cl";
  const maskedEmailValue = maskEmail(rawEmail);
  const maskedPhoneValue = profile?.phone
    ? maskPhone(profile.phone)
    : "+56 •••• 5678";

  const Icon = config.icon;

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground flex-wrap"
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
        <Link
          href="/mi-cuenta/seguridad/metodos"
          className="hover:text-primary transition-colors font-medium"
        >
          Métodos de verificación
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="text-foreground font-semibold">{config.title}</span>
      </nav>

      {/* Header con botón Volver */}
      <div className="space-y-4">
        <Link
          href="/mi-cuenta/seguridad/metodos"
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-primary hover:underline"
        >
          <ArrowLeft className="size-4" />
          Volver a todos los métodos
        </Link>

        <div className="space-y-1">
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
            {config.title}
          </h1>
          <p className="text-sm text-muted-foreground">{config.subtitle}</p>
        </div>
      </div>

      {/* Card principal estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 border-b border-border/60">
          <div className="flex items-center gap-4">
            <div
              className={`size-12 rounded-2xl flex items-center justify-center shrink-0 ${
                config.active
                  ? "bg-emerald-500/10 text-emerald-600"
                  : "bg-primary/10 text-primary"
              }`}
            >
              <Icon className="size-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-lg font-bold text-foreground">
                  Estado del método
                </h2>
                {config.active ? (
                  <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                    <CheckCircle2 className="size-3.5" />
                    {config.activeBadge}
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground bg-muted px-2.5 py-0.5 rounded-full">
                    <Clock className="size-3.5" />
                    {config.activeBadge}
                  </span>
                )}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5">
                Plan de despliegue: {config.phase}
              </p>
            </div>
          </div>

          {config.active && slug === "contrasena" && (
            <Link
              href="/mi-cuenta/seguridad/metodos/contrasena"
              className="inline-flex items-center justify-center px-4 py-2 rounded-xl bg-primary text-primary-foreground font-semibold text-xs transition-colors hover:bg-primary/90"
            >
              Modificar contraseña
            </Link>
          )}
          {slug === "telefono" && (
            <Link
              href="/mi-cuenta/seguridad/metodos/telefono"
              className="inline-flex items-center justify-center px-4 py-2 rounded-xl bg-primary text-primary-foreground font-semibold text-xs transition-colors hover:bg-primary/90"
            >
              Configurar teléfono
            </Link>
          )}
        </div>

        {/* Datos y descripción */}
        <div className="space-y-4 text-xs sm:text-sm text-muted-foreground">
          <p className="text-foreground leading-relaxed">
            {config.description}
          </p>

          {slug === "email" && (
            <div className="p-4 rounded-xl bg-muted/30 border border-border/60 space-y-1">
              <span className="font-semibold text-foreground text-xs">
                Correo verificado:
              </span>
              <p className="font-mono text-sm font-semibold text-foreground">
                {maskedEmailValue}
              </p>
            </div>
          )}

          {slug === "telefono" && (
            <div className="p-4 rounded-xl bg-muted/30 border border-border/60 space-y-1">
              <span className="font-semibold text-foreground text-xs">
                Número registrado:
              </span>
              <p className="font-mono text-sm font-semibold text-foreground">
                {maskedPhoneValue}
              </p>
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2">
            <div className="flex items-start gap-3">
              <ShieldCheck className="size-5 text-emerald-600 shrink-0 mt-0.5" />
              <div>
                <strong className="text-foreground font-medium block">
                  Seguridad y cifrado
                </strong>
                <span>{config.technicalDetails}</span>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <Info className="size-5 text-primary shrink-0 mt-0.5" />
              <div>
                <strong className="text-foreground font-medium block">
                  Enmascaramiento A.8.11
                </strong>
                <span>
                  Los identificadores y secretos nunca se exponen completos en
                  pantalla ni en logs públicos.
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
