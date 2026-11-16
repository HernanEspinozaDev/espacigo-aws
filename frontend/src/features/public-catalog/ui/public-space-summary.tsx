"use client";

import Link from "next/link";
import { useState } from "react";
import { PublicVerificationBadges } from "./public-verification-badges";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import type { PublicSpace, PublicSpaceDetail } from "../domain/types";

export function PublicSpaceSummary({
  title,
  description,
  space,
  price,
  estimate,
  breadcrumb,
}: {
  title: string;
  description: string;
  space: PublicSpaceDetail;
  price: string | null;
  estimate: string | null;
  breadcrumb: React.ReactNode;
}) {
  const [expanded, setExpanded] = useState(false);
  const area = space.area_m2 > 0 ? `${space.area_m2} m²` : null;
  const scheduleReported = Boolean(
    space.weekly_schedule?.enabled &&
    space.weekly_schedule.days.some((day) => day.periods.length > 0),
  );
  const chips = [
    space.comuna_name,
    space.region_name,
    area,
    space.capacity_max > 0 ? `Hasta ${space.capacity_max} personas` : null,
    scheduleReported ? "Horario informado" : "Horario no informado",
    space.cancellation_policy === null
      ? "Política de cancelación no informada"
      : null,
  ].filter((chip): chip is string => Boolean(chip));
  const features = (space.features ?? []).filter((feature) => feature.value);

  return (
    <div className="min-w-0 space-y-6 [overflow-wrap:anywhere]">
      {breadcrumb}
      <header className="space-y-3">
        <Link
          href={`/categorias/${encodeURIComponent(space.category_code)}`}
          className="inline-flex min-h-11 min-w-0 items-center text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground [overflow-wrap:anywhere] hover:text-foreground"
        >
          {space.category_name}
        </Link>
        <h1 className="text-3xl font-medium tracking-tight text-foreground">
          {title}
        </h1>
        <PublicVerificationBadges verification={space.verification} />
        {chips.length ? (
          <ul aria-label="Datos del espacio" className="flex flex-wrap gap-2">
            {chips.map((chip) => (
              <li
                key={chip}
                className="rounded-full border border-border px-3 py-1 text-sm text-muted-foreground"
              >
                {chip}
              </li>
            ))}
          </ul>
        ) : null}
      </header>
      <section
        aria-label="Tarifa publicada"
        className="rounded-2xl border border-border p-5"
      >
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="text-2xl font-semibold">
            {price ?? "Tarifa no disponible"}
          </span>
          {price ? (
            <span className="text-muted-foreground">
              / {rateUnitLabel(space.rate_unit)}
            </span>
          ) : null}
        </div>
        {estimate ? (
          <p className="mt-3 min-w-0 text-sm text-muted-foreground [overflow-wrap:anywhere]">
            Estimación para el intervalo consultado: {estimate}.
          </p>
        ) : null}
        <p className="mt-3 min-w-0 text-sm leading-relaxed text-muted-foreground [overflow-wrap:anywhere]">
          La tarifa y cualquier estimación no confirman disponibilidad y no
          constituyen cotización ni reserva.
        </p>
        <p className="mt-2 min-w-0 text-sm text-muted-foreground [overflow-wrap:anywhere]">
          Garantía no informada.
        </p>
      </section>
      <section className="space-y-2">
        <h2 className="text-lg font-medium">Sobre esta publicación</h2>
        <p
          className={
            expanded
              ? "whitespace-pre-wrap break-words leading-relaxed text-muted-foreground"
              : "line-clamp-5 whitespace-pre-wrap break-words leading-relaxed text-muted-foreground"
          }
        >
          {description || "La publicación no incluye una descripción."}
        </p>
        {description.length > 260 ? (
          <button
            type="button"
            aria-expanded={expanded}
            onClick={() => setExpanded((value) => !value)}
            className="min-h-11 rounded-md text-sm font-medium text-primary underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            Ver {expanded ? "menos" : "más"}
          </button>
        ) : null}
      </section>
      {features.length ? (
        <ul aria-label="Características declaradas" className="space-y-2">
          {features.map((feature) => (
            <li
              key={feature.label}
              className="break-words text-sm text-muted-foreground"
            >
              {feature.label}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function publicPrice(space: PublicSpace) {
  return formatPublicClp(space.base_price_clp);
}
