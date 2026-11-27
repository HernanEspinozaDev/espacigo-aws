import Link from "next/link";
import { ArrowRight, Building2, ShieldCheck } from "lucide-react";

export function VerifiedSpaceHostPromo({
  variant = "default",
}: {
  variant?: "default" | "compact";
}) {
  const isCompact = variant === "compact";

  return (
    <section
      aria-labelledby="verified-space-host-promo-title"
      className={
        isCompact
          ? "rounded-xl border border-primary/25 bg-[var(--brand-primary-soft)]/50 p-4 sm:p-5 shadow-xs"
          : "rounded-2xl border border-primary/25 bg-[var(--brand-primary-soft)]/45 p-6 sm:p-7 shadow-sm"
      }
    >
      <div
        className={
          isCompact
            ? "flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between"
            : "flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between"
        }
      >
        <div className="flex items-start gap-3.5 sm:gap-4 max-w-3xl">
          <span
            className={
              isCompact
                ? "inline-flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/15 text-primary"
                : "inline-flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/15 text-primary"
            }
          >
            <ShieldCheck className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1.5">
            <span className="inline-flex items-center gap-1.5 rounded-full bg-background/90 px-2.5 py-0.5 text-xs font-semibold text-primary border border-primary/20">
              Mayor confianza en el marketplace
            </span>
            <h2
              id="verified-space-host-promo-title"
              className={
                isCompact
                  ? "text-base sm:text-lg font-semibold tracking-tight text-foreground"
                  : "text-lg sm:text-xl font-bold tracking-tight text-foreground"
              }
            >
              Habilita tu empresa y obtén la insignia Espacio Verificado
            </h2>
            <p className="text-xs sm:text-sm leading-relaxed text-muted-foreground">
              Sube los datos de tu empresa y la documentación de cada inmueble
              para destacar como espacio recomendado, generar más confianza y
              mantener tus publicaciones habilitadas.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2.5 shrink-0">
          <Link
            href="/mi-cuenta/verificaciones/espacios"
            className="inline-flex min-h-10 items-center justify-center gap-2 rounded-xl bg-primary px-4 py-2 text-xs sm:text-sm font-semibold text-primary-foreground shadow-xs transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ShieldCheck className="size-4" aria-hidden="true" />
            Verificar mis espacios
            <ArrowRight className="size-3.5" aria-hidden="true" />
          </Link>
          <Link
            href="/mi-cuenta/empresas"
            className="inline-flex min-h-10 items-center justify-center gap-2 rounded-xl border border-border bg-background px-4 py-2 text-xs sm:text-sm font-semibold text-foreground transition-colors hover:border-primary/40 hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Building2 className="size-4 text-primary" aria-hidden="true" />
            Configurar Perfil empresas
          </Link>
        </div>
      </div>
    </section>
  );
}
