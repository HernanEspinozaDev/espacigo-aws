import {
  ArrowRight,
  CalendarClock,
  CheckCircle2,
  CreditCard,
  FileSignature,
  LayoutDashboard,
  MapPinned,
  ShieldCheck,
  Sparkles,
  Wallet,
} from "lucide-react";
import Link from "next/link";

const PLATFORM_FEATURES = [
  {
    icon: MapPinned,
    title: "Explora en mapa y por cercanía",
    description:
      "Ubica oficinas, salas, bodegas, quinchos y casas en el mapa interactivo, filtra por comuna y compara opciones en segundos.",
    href: "/explorar/mapa",
    cta: "Abrir mapa",
  },
  {
    icon: CalendarClock,
    title: "Reserva por hora, día o mes",
    description:
      "Elige la duración que necesitas según cada espacio, consulta horarios disponibles y revisa el desglose de tarifa antes de confirmar.",
    href: "/explorar",
    cta: "Ver espacios",
  },
  {
    icon: FileSignature,
    title: "Genera y guarda contratos",
    description:
      "Cada arriendo comercial cuenta con propuesta de condiciones, firma electrónica simple y respaldo en PDF disponible en tu panel.",
    href: "/como-funciona",
    cta: "Cómo funciona",
  },
  {
    icon: CreditCard,
    title: "Pagos con Mercado Pago",
    description:
      "Paga con trazabilidad y comprobantes en línea. Los anfitriones reciben sus cobros conectando su cuenta de Mercado Pago.",
    href: "/anfitriones",
    cta: "Para anfitriones",
  },
];

const HOST_STEPS = [
  {
    step: "01",
    title: "Inicia sesión y verifícate",
    description:
      "Crea tu cuenta y completa tu verificación de identidad para dar respaldo a tus publicaciones.",
  },
  {
    step: "02",
    title: "Crea tu espacio",
    description:
      "Elige la categoría, marca la ubicación en el mapa, sube fotos reales y define tarifas y horarios.",
  },
  {
    step: "03",
    title: "Conecta Mercado Pago",
    description:
      "Asocia tu cuenta para recibir los pagos de tus arriendos y seguir tus liquidaciones.",
  },
  {
    step: "04",
    title: "Genera tu contrato y publica gratis",
    description:
      "Publica sin costo, aprueba solicitudes, firma contratos digitales y mantén todo guardado.",
  },
];

export function PlatformFeaturesSection() {
  return (
    <section className="border-t border-border bg-background py-14 sm:py-20">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-2xl">
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--brand-accent)]">
              Todo en una sola plataforma
            </p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
              Arrienda y gestiona espacios con respaldo de principio a fin
            </h2>
            <p className="mt-3 text-base leading-relaxed text-muted-foreground">
              EspaciGo reúne búsqueda en mapa, disponibilidad real, contratos
              digitales y pagos en línea para que arrendadores y arrendatarios
              operen sin fricción.
            </p>
          </div>
          <Link
            href="/explorar/mapa"
            className="inline-flex min-h-11 shrink-0 items-center gap-2 rounded-full border border-border bg-background px-5 text-sm font-medium text-foreground transition-colors hover:border-[var(--brand-accent)] hover:bg-[var(--brand-primary-soft)]/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <MapPinned
              className="size-4 text-[var(--brand-accent)]"
              aria-hidden="true"
            />
            Explorar en mapa
          </Link>
        </div>

        <div className="mt-10 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
          {PLATFORM_FEATURES.map((item) => {
            const Icon = item.icon;
            return (
              <article
                key={item.title}
                className="flex flex-col justify-between rounded-2xl border border-border bg-[var(--brand-surface-subtle)]/60 p-6 transition-colors hover:border-[var(--brand-accent)]/60 hover:bg-background"
              >
                <div>
                  <span className="inline-flex size-11 items-center justify-center rounded-xl bg-[var(--brand-primary-soft)] text-[var(--brand-accent)]">
                    <Icon className="size-5" aria-hidden="true" />
                  </span>
                  <h3 className="mt-4 text-lg font-semibold text-foreground">
                    {item.title}
                  </h3>
                  <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                    {item.description}
                  </p>
                </div>
                <Link
                  href={item.href}
                  className="mt-5 inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
                >
                  {item.cta}
                  <ArrowRight className="size-3.5" aria-hidden="true" />
                </Link>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}

export function HostPromoSection() {
  return (
    <section className="bg-foreground text-background">
      <div className="mx-auto max-w-7xl px-4 py-16 sm:px-6 sm:py-24 lg:px-8">
        <div className="grid gap-10 lg:grid-cols-[1fr_1.15fr] lg:items-center lg:gap-14">
          <div>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-white/10 px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-[var(--brand-accent)]">
              <Sparkles className="size-3.5" aria-hidden="true" />
              Publica gratis en EspaciGo
            </span>
            <h2 className="mt-4 text-3xl font-semibold tracking-tight sm:text-4xl">
              Publica gratis, gestiona tus espacios y cobra en línea
            </h2>
            <p className="mt-4 text-base leading-relaxed text-background/80">
              Ofrece oficinas, salas, bodegas, estacionamientos, quinchos o
              propiedades residenciales. Mantén tus publicaciones actualizadas,
              revisa reservas en curso, genera contratos digitales y guarda cada
              respaldo en un solo panel.
            </p>

            <ul className="mt-6 space-y-2.5 text-sm text-background/90">
              {[
                {
                  icon: CheckCircle2,
                  text: "Publicación gratuita sin costos de entrada.",
                },
                {
                  icon: LayoutDashboard,
                  text: "Control total de calendario, horarios, fotos y tarifas.",
                },
                {
                  icon: Wallet,
                  text: "Pagos directos conectando tu cuenta de Mercado Pago.",
                },
                {
                  icon: ShieldCheck,
                  text: "Contratos generados, firmados y almacenados en PDF.",
                },
              ].map(({ icon: ItemIcon, text }) => (
                <li key={text} className="flex items-center gap-2.5">
                  <ItemIcon
                    className="size-4 shrink-0 text-[var(--brand-accent)]"
                    aria-hidden="true"
                  />
                  <span>{text}</span>
                </li>
              ))}
            </ul>

            <div className="mt-8 flex flex-wrap gap-3">
              <Link
                href="/anfitrion/espacios/nuevo"
                className="inline-flex min-h-12 items-center justify-center gap-2 rounded-full bg-primary px-6 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background"
              >
                Publicar espacio gratis
                <ArrowRight className="size-4" aria-hidden="true" />
              </Link>
              <Link
                href="/anfitriones"
                className="inline-flex min-h-12 items-center justify-center rounded-full border border-white/25 bg-white/5 px-6 text-sm font-medium text-background transition-colors hover:bg-white/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background"
              >
                Ver guía para anfitriones
              </Link>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {HOST_STEPS.map((item) => (
              <div
                key={item.step}
                className="rounded-2xl border border-white/15 bg-white/5 p-5 backdrop-blur-xs"
              >
                <span className="inline-flex rounded-full bg-[var(--brand-primary)]/90 px-2.5 py-0.5 text-xs font-semibold tabular-nums text-white">
                  Paso {item.step}
                </span>
                <h3 className="mt-3 text-base font-semibold text-background">
                  {item.title}
                </h3>
                <p className="mt-1.5 text-sm leading-relaxed text-background/75">
                  {item.description}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

export function MercadoPagoSecuritySection() {
  return (
    <section className="border-t border-border bg-background py-14 sm:py-20">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div className="grid gap-8 rounded-3xl border border-border bg-[var(--brand-surface-subtle)] p-6 sm:p-10 lg:grid-cols-[1.15fr_0.85fr] lg:items-center">
          <div>
            <span className="inline-flex items-center gap-2 rounded-full bg-[#009EE3]/10 px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.15em] text-[#0073A8]">
              <ShieldCheck className="size-3.5" aria-hidden="true" />
              Total seguridad con Mercado Pago
            </span>
            <h2 className="mt-4 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
              Publica gratis, conecta Mercado Pago y cobra automáticamente
            </h2>
            <p className="mt-3 text-base leading-relaxed text-[var(--brand-muted-on-subtle)]">
              Guardamos tu aprobación de Mercado Pago de forma segura para que
              puedas recibir los pagos de tus arriendos automáticamente. Quien
              arrienda paga en el checkout oficial de Mercado Pago con
              comprobantes verificados y contrato digital.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link
                href="/mercado-pago"
                className="inline-flex min-h-11 items-center gap-2 rounded-full bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)]"
              >
                Cómo usamos Mercado Pago
                <ArrowRight className="size-4" aria-hidden="true" />
              </Link>
              <Link
                href="/acceso?return_to=%2Fanfitrion%2Fmercado-pago"
                className="inline-flex min-h-11 items-center rounded-full border border-border bg-background px-5 text-sm font-medium text-foreground hover:bg-secondary"
              >
                Conectar mi cuenta
              </Link>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
            <div className="rounded-2xl border border-border bg-background p-4">
              <p className="text-sm font-semibold text-foreground">
                Aprobación segura y cobro automático
              </p>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                Autorizas tu cuenta una sola vez por OAuth y tus reservas
                aprobadas se procesan con trazabilidad completa.
              </p>
            </div>
            <div className="rounded-2xl border border-border bg-background p-4">
              <p className="text-sm font-semibold text-foreground">
                Plataforma protegida de extremo a extremo
              </p>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                Verificación de identidad, MFA en tu cuenta, contratos digitales
                y almacenamiento privado sobre infraestructura AWS.
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
