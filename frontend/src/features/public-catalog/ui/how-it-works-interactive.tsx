"use client";

import {
  ArrowRight,
  Building2,
  CalendarCheck,
  CheckCircle2,
  ChevronRight,
  CreditCard,
  FileSignature,
  MapPinned,
  ShieldCheck,
  UserCheck,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

export type HowItWorksRole = "arrendatario" | "arrendador";

interface RoleStep {
  number: string;
  icon: LucideIcon;
  title: string;
  mobileSummary: string;
  description: string;
  highlights: string[];
}

const RENTER_STEPS: readonly RoleStep[] = [
  {
    number: "01",
    icon: UserCheck,
    title: "Verifica tu cuenta",
    mobileSummary: "Verifícate primero para arrendar con respaldo.",
    description:
      "Crea tu cuenta y completa tu verificación de identidad. Primero verifícate y luego podrás arrendar el espacio que necesites con total confianza.",
    highlights: [
      "Registro rápido y sesiones protegidas",
      "Identidad verificada para firmar contratos",
      "Acceso inmediato a reservas y comprobantes",
    ],
  },
  {
    number: "02",
    icon: MapPinned,
    title: "Búsqueda fácil y llegada directa",
    mobileSummary: "Busca en el mapa por comuna, categoría y horario.",
    description:
      "Explora oficinas, salas, bodegas, quinchos o casas en el mapa interactivo. Filtra por cercanía, comuna y presupuesto para llegar fácil y arrendar sin rodeos.",
    highlights: [
      "Mapa interactivo y búsqueda por cercanía",
      "Tarifas transparentes por hora, día o mes",
      "Reglas y equipamiento visibles antes de elegir",
    ],
  },
  {
    number: "03",
    icon: CreditCard,
    title: "Arrienda con total seguridad de pago",
    mobileSummary: "Reserva y paga seguro con Mercado Pago.",
    description:
      "Selecciona tus fechas u horas, envía tu solicitud de arriendo y paga en el checkout oficial de Mercado Pago con confirmación verificada.",
    highlights: [
      "Checkout oficial de Mercado Pago",
      "Nunca expones tu tarjeta a terceros",
      "Comprobante digital por cada reserva",
    ],
  },
  {
    number: "04",
    icon: FileSignature,
    title: "Firma, guarda y mantén tu contrato",
    mobileSummary: "Firma, guarda tu PDF y revisa fechas de pago.",
    description:
      "Firma tu contrato digital en línea, guárdalo en PDF, mantén el contrato de tu arriendo activo, revisa tus fechas de pago y mantente al día.",
    highlights: [
      "Contrato digital con firma electrónica simple",
      "Descarga y respaldo permanente en PDF",
      "Seguimiento claro de fechas de pago y vigencia",
    ],
  },
];

const LANDLORD_STEPS: readonly RoleStep[] = [
  {
    number: "01",
    icon: ShieldCheck,
    title: "Verifica tu cuenta",
    mobileSummary: "Inicia sesión y verifícate para publicar gratis.",
    description:
      "Inicia sesión y verifica tu identidad para habilitar tu perfil de arrendador y mostrar el sello de confianza en todas tus publicaciones.",
    highlights: [
      "Verificación única en tu cuenta",
      "Distintivo visible en tus espacios",
      "Mayor confianza para recibir solicitudes",
    ],
  },
  {
    number: "02",
    icon: Building2,
    title: "Publica gratis tu espacio",
    mobileSummary: "Sube fotos, mapa, horarios y tarifa sin costo.",
    description:
      "Crea tu publicación gratis: elige la categoría, marca la ubicación en el mapa, sube fotografías reales y configura horarios, capacidad y tarifas.",
    highlights: [
      "Publicación gratuita sin costos fijos",
      "Tarifas por hora, día o mes según tu espacio",
      "Edición rápida de fotos, reglas y calendario",
    ],
  },
  {
    number: "03",
    icon: CreditCard,
    title: "Conecta Mercado Pago y contratos",
    mobileSummary: "Aprobación guardada para cobrar automático.",
    description:
      "Asigna tu cuenta de Mercado Pago una sola vez: guardamos tu aprobación para que cobres automáticamente y generes contratos digitales por cada arriendo.",
    highlights: [
      "Aprobación guardada de forma segura",
      "Generación automática de propuesta y contrato",
      "Registro de pagos y liquidaciones en línea",
    ],
  },
  {
    number: "04",
    icon: CalendarCheck,
    title: "Mantén tus espacios y arriendos al día",
    mobileSummary: "Controla reservas, fechas de pago y contratos.",
    description:
      "Mantén tus publicaciones actualizadas, aprueba o gestiona solicitudes, revisa fechas de pago y guarda todos tus contratos firmados en un solo panel.",
    highlights: [
      "Control centralizado de reservas activas",
      "Contratos firmados guardados en PDF",
      "Historial de cobros y estado al día",
    ],
  },
];

export function HowItWorksInteractive({
  initialRole = "arrendatario",
}: {
  initialRole?: HowItWorksRole;
}) {
  const [role, setRole] = useState<HowItWorksRole>(initialRole);
  const [stepIndex, setStepIndex] = useState(0);

  const steps = role === "arrendatario" ? RENTER_STEPS : LANDLORD_STEPS;
  const activeStep = steps[stepIndex] ?? steps[0];
  const ActiveIcon = activeStep.icon;

  const switchRole = (nextRole: HowItWorksRole) => {
    setRole(nextRole);
    setStepIndex(0);
  };

  return (
    <section
      aria-labelledby="how-it-works-heading"
      className="mx-auto max-w-7xl px-4 py-14 sm:px-6 sm:py-20 lg:px-8"
    >
      <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-2xl">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--brand-accent)]">
            ¿Cómo funciona EspaciGo?
          </p>
          <h2
            id="how-it-works-heading"
            className="mt-2 text-2xl font-semibold tracking-tight text-foreground sm:text-4xl"
          >
            Un flujo claro para arrendatarios y arrendadores
          </h2>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground sm:text-base">
            <span className="sm:hidden">
              Elige tu rol y toca cada paso para ver cómo funciona.
            </span>
            <span className="hidden sm:inline">
              Selecciona si buscas arrendar un espacio o publicar gratis como
              arrendador y recorre cada paso de forma interactiva.
            </span>
          </p>
        </div>

        <div
          role="tablist"
          aria-label="Seleccionar tipo de usuario"
          className="inline-flex w-full rounded-full border border-border bg-[var(--brand-surface-subtle)] p-1 sm:w-auto"
        >
          <button
            type="button"
            role="tab"
            aria-selected={role === "arrendatario"}
            onClick={() => switchRole("arrendatario")}
            className={`flex-1 rounded-full px-4 py-2.5 text-sm font-medium transition-all sm:flex-none sm:px-5 ${
              role === "arrendatario"
                ? "bg-primary text-primary-foreground shadow-xs"
                : "text-[var(--brand-muted-on-subtle)] hover:text-foreground"
            }`}
          >
            Para Arrendatarios
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={role === "arrendador"}
            onClick={() => switchRole("arrendador")}
            className={`flex-1 rounded-full px-4 py-2.5 text-sm font-medium transition-all sm:flex-none sm:px-5 ${
              role === "arrendador"
                ? "bg-primary text-primary-foreground shadow-xs"
                : "text-[var(--brand-muted-on-subtle)] hover:text-foreground"
            }`}
          >
            Para Arrendadores
          </button>
        </div>
      </div>

      <div className="mt-8 grid gap-6 lg:grid-cols-[0.95fr_1.05fr] lg:items-stretch">
        <div
          role="list"
          aria-label={`Pasos para ${role === "arrendatario" ? "arrendatarios" : "arrendadores"}`}
          className="grid gap-3"
        >
          {steps.map((step, index) => {
            const StepIcon = step.icon;
            const isSelected = index === stepIndex;
            return (
              <button
                key={`${role}-${step.number}`}
                type="button"
                aria-current={isSelected ? "step" : undefined}
                onClick={() => setStepIndex(index)}
                className={`group flex w-full items-center justify-between gap-3 rounded-2xl border p-4 text-left transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                  isSelected
                    ? "border-primary bg-[var(--brand-primary-soft)] shadow-xs"
                    : "border-border bg-background hover:border-[var(--brand-accent)]/60 hover:bg-[var(--brand-surface-subtle)]"
                }`}
              >
                <div className="flex min-w-0 items-start gap-3.5">
                  <span
                    className={`inline-flex size-10 shrink-0 items-center justify-center rounded-xl transition-colors ${
                      isSelected
                        ? "bg-primary text-primary-foreground"
                        : "bg-[var(--brand-surface-subtle)] text-[var(--brand-accent)] group-hover:bg-[var(--brand-primary-soft)]"
                    }`}
                  >
                    <StepIcon className="size-5" aria-hidden="true" />
                  </span>
                  <div className="min-w-0">
                    <span
                      className={`text-xs font-semibold tabular-nums uppercase tracking-[0.14em] ${
                        isSelected
                          ? "text-[var(--brand-primary-soft-ink)]"
                          : "text-muted-foreground"
                      }`}
                    >
                      Paso {step.number}
                    </span>
                    <p
                      className={`mt-0.5 text-base font-semibold ${
                        isSelected
                          ? "text-[var(--brand-primary-soft-ink)]"
                          : "text-foreground"
                      }`}
                    >
                      {step.title}
                    </p>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground sm:hidden">
                      {step.mobileSummary}
                    </p>
                  </div>
                </div>
                <ChevronRight
                  aria-hidden="true"
                  className={`size-4 shrink-0 transition-transform ${
                    isSelected
                      ? "translate-x-0.5 text-[var(--brand-primary-soft-ink)]"
                      : "text-muted-foreground"
                  }`}
                />
              </button>
            );
          })}
        </div>

        <div
          key={`${role}-${activeStep.number}`}
          className="flex flex-col justify-between rounded-3xl border border-border bg-[var(--brand-surface-subtle)] p-6 transition-all duration-300 sm:p-8"
        >
          <div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="inline-flex items-center gap-2 rounded-full bg-[var(--brand-primary-soft)] px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.14em] text-[var(--brand-primary-soft-ink)]">
                <ActiveIcon className="size-3.5" aria-hidden="true" />
                {role === "arrendatario" ? "Arrendatario" : "Arrendador"} · Paso{" "}
                {activeStep.number}
              </span>
              <span className="text-xs font-medium tabular-nums text-muted-foreground">
                {stepIndex + 1} de {steps.length}
              </span>
            </div>

            <h3 className="mt-5 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
              {activeStep.title}
            </h3>
            <p className="mt-3 text-sm leading-relaxed text-[var(--brand-muted-on-subtle)] sm:text-base">
              {activeStep.description}
            </p>

            <ul className="mt-6 space-y-2.5 border-t border-border pt-5">
              {activeStep.highlights.map((item) => (
                <li
                  key={item}
                  className="flex items-start gap-2.5 text-sm font-medium text-foreground"
                >
                  <CheckCircle2
                    className="mt-0.5 size-4 shrink-0 text-[var(--brand-success)]"
                    aria-hidden="true"
                  />
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="mt-8 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
            <button
              type="button"
              onClick={() => setStepIndex((prev) => (prev + 1) % steps.length)}
              className="inline-flex min-h-10 items-center gap-1.5 rounded-full border border-border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:border-[var(--brand-accent)]"
            >
              {stepIndex < steps.length - 1
                ? "Siguiente paso"
                : "Volver al paso 01"}
              <ArrowRight
                className="size-4 text-[var(--brand-accent)]"
                aria-hidden="true"
              />
            </button>

            {role === "arrendatario" ? (
              <Link
                href="/explorar"
                className="inline-flex min-h-10 items-center gap-2 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
              >
                Explorar espacios
                <ArrowRight className="size-4" aria-hidden="true" />
              </Link>
            ) : (
              <Link
                href="/acceso?return_to=%2Fanfitrion%2Fespacios%2Fnuevo"
                className="inline-flex min-h-10 items-center gap-2 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
              >
                Publicar gratis como arrendador
                <ArrowRight className="size-4" aria-hidden="true" />
              </Link>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
