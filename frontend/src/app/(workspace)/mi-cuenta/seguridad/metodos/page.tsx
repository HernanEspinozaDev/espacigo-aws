"use client";

import Link from "next/link";
import {
  ChevronRight,
  KeyRound,
  LockKeyhole,
  Mail,
  QrCode,
  ScanFace,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

const methods = [
  {
    key: "phone",
    route: "telefono",
    title: "Teléfono",
    description: "Verifica tu número móvil con un código.",
    icon: Smartphone,
  },
  {
    key: "password",
    route: "contrasena",
    title: "Contraseña",
    description: "Cambia la clave con la que accedes a tu cuenta.",
    icon: LockKeyhole,
  },
  {
    key: "email",
    route: "email",
    title: "Correo electrónico",
    description: "Consulta la dirección que usas para recuperar tu cuenta.",
    icon: Mail,
  },
  {
    key: "passkeys",
    route: "llaves-de-acceso",
    title: "Llaves de acceso",
    description: "Administra las llaves guardadas en tus dispositivos.",
    icon: KeyRound,
  },
  {
    key: "totp",
    route: "google-authenticator",
    title: "Google Authenticator",
    description: "Configura una aplicación que genera códigos de seis dígitos.",
    icon: ShieldCheck,
  },
  {
    key: "qr",
    route: "codigo-qr",
    title: "Código QR",
    description: "Administra la vinculación de tu dispositivo.",
    icon: QrCode,
  },
] as const;

export default function MetodosVerificacionPage() {
  const { summary, isLoading, error, refresh } = useSecurityMethodsSummary();
  const current = isLoading || error ? null : summary;
  const details = current?.methods;

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Métodos de verificación"
        description="Gestiona cómo accedes a tu cuenta y confirmas tus datos."
      />

      <section
        className="rounded-2xl border border-primary/20 bg-primary/5 p-5"
        aria-label="Resumen de configuración"
        aria-busy={isLoading}
      >
        {isLoading ? (
          <p role="status" className="text-sm">
            Consultando tus métodos…
          </p>
        ) : current ? (
          <p className="text-sm">
            <strong>
              {current.active_count} de {current.total_count} métodos
              configurados
            </strong>
          </p>
        ) : (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p role="alert" className="text-sm">
              No pudimos consultar el estado de tus métodos.
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void refresh()}
            >
              Reintentar
            </Button>
          </div>
        )}
      </section>

      <ul
        className="divide-y rounded-2xl border bg-card"
        aria-label="Métodos de verificación de la cuenta"
      >
        {methods.map(({ key, route, title, description, icon: Icon }) => {
          const enabled = details?.[key].enabled;
          const label = enabled
            ? key === "phone" || key === "email"
              ? "Verificado"
              : "Configurado"
            : "Sin configurar";
          const maskedContact =
            key === "phone"
              ? details?.phone.masked_phone
              : key === "email"
                ? details?.email.masked_email
                : null;

          return (
            <li key={key}>
              <Link
                href={`/mi-cuenta/seguridad/metodos/${route}`}
                className="group grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-4 transition-colors first:rounded-t-2xl hover:bg-muted/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary sm:gap-4 sm:px-5"
              >
                <span className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <Icon className="size-5" aria-hidden="true" />
                </span>
                <div className="min-w-0 space-y-1">
                  <h2 className="font-semibold">{title}</h2>
                  <p className="text-sm text-muted-foreground">{description}</p>
                  <div className="flex flex-wrap items-center gap-2">
                    {enabled !== undefined && (
                      <span
                        className={
                          enabled
                            ? "rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-xs font-medium text-emerald-800 dark:text-emerald-300"
                            : "rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground"
                        }
                      >
                        {label}
                      </span>
                    )}
                    {maskedContact && (
                      <span className="break-all text-xs text-muted-foreground">
                        {maskedContact}
                      </span>
                    )}
                  </div>
                </div>
                <ChevronRight
                  className="size-4 text-muted-foreground"
                  aria-hidden="true"
                />
              </Link>
            </li>
          );
        })}
        <li className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-3 p-4 sm:gap-4 sm:px-5">
          <span className="flex size-10 items-center justify-center rounded-xl bg-muted text-muted-foreground">
            <ScanFace className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1">
            <h2 className="font-semibold">Reconocimiento facial</h2>
            <p className="text-sm text-muted-foreground">No disponible</p>
          </div>
        </li>
      </ul>
    </div>
  );
}
