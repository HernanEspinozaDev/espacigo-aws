"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import {
  ChevronRight,
  KeyRound,
  LockKeyhole,
  Mail,
  QrCode,
  RectangleEllipsis,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  getPinpassStatus,
  PINPASS_SETTINGS_HREF,
} from "@/features/account/pinpass-api";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

const methods = [
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
    key: "phone",
    route: "telefono",
    title: "Teléfono",
    description: "Verifica tu número móvil con un código.",
    icon: Smartphone,
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
  const [configured, setConfigured] = useState<boolean | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    getPinpassStatus(controller.signal)
      .then((status) => {
        if (!controller.signal.aborted) setConfigured(status.configured);
      })
      .catch(() => {
        // The direct method link still works if its status is unavailable.
      });
    return () => controller.abort();
  }, []);

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
        <li>
          <Link
            href={PINPASS_SETTINGS_HREF}
            aria-labelledby="pinpass-method-action"
            aria-describedby="pinpass-method-description"
            className={`grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 rounded-t-2xl p-4 transition-colors hover:bg-muted/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary sm:gap-4 sm:px-5 ${configured === false ? "bg-primary/5" : ""}`}
          >
            <span className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <RectangleEllipsis className="size-5" aria-hidden="true" />
            </span>
            <div className="min-w-0 space-y-1">
              <h2 className="font-semibold">Pinpass</h2>
              <p
                id="pinpass-method-description"
                className="text-sm text-muted-foreground"
              >
                {configured === false
                  ? "Configura tu Pinpass para navegar y desbloquear opciones dentro de EspaciGo."
                  : "Usa tu clave de seis dígitos para desbloquear opciones y ver tus datos privados."}
              </p>
              <p
                id="pinpass-method-action"
                role="status"
                className="text-sm font-semibold text-primary"
              >
                {configured === false
                  ? "Configurar Pinpass"
                  : configured
                    ? "Administrar Pinpass"
                    : "Abrir Pinpass"}
              </p>
            </div>
            <ChevronRight
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
          </Link>
        </li>
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
                className="group grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-4 transition-colors hover:bg-muted/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary sm:gap-4 sm:px-5"
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
      </ul>
    </div>
  );
}
