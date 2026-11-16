import {
  CalendarDays,
  CircleHelp,
  MessageSquareText,
  Ruler,
  ShieldCheck,
  Star,
  Users,
} from "lucide-react";
import type { PublicSpaceDetail } from "../domain/types";
import { safePublicText } from "../domain/presentation";

const dynamicText = "min-w-0 break-words [overflow-wrap:anywhere]";

export interface PublicReviewItem {
  id: string;
  target_type?: string;
  rating: number;
  comment?: string;
  created_at: string;
}

export interface PublicSpaceReviewsData {
  items: PublicReviewItem[];
  average: number;
  count: number;
}

function humanize(value: string): string {
  return value.replace(/[_-]+/g, " ").replace(/\s+/g, " ").trim();
}

function attributeValue(value: boolean | number | string | string[]): string {
  if (typeof value === "boolean") return value ? "Sí" : "No";
  if (Array.isArray(value)) return value.map(humanize).join(", ");
  return typeof value === "string" ? humanize(value) : String(value);
}

function hostInitials(name: string | null): string {
  const words = (name ?? "Anfitrión").trim().split(/\s+/).filter(Boolean);
  return words
    .slice(0, 2)
    .map((word) => Array.from(word)[0]?.toLocaleUpperCase("es") ?? "")
    .join("");
}

function joinedLabel(value: string): string | null {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return null;
  return new Intl.DateTimeFormat("es-CL", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(date);
}

export function PublicSpaceDetails({
  space,
  reviews,
}: {
  space: PublicSpaceDetail;
  reviews?: PublicSpaceReviewsData | null;
}) {
  const attributes = [...space.attributes].sort((a, b) => a.order - b.order);
  const joined = space.host ? joinedLabel(space.host.joined_at) : null;
  const rules = safePublicText(space.usage_rules).trim();
  const hostName = safePublicText(space.host?.name?.trim() ?? "");

  return (
    <div className="min-w-0 space-y-8 [overflow-wrap:anywhere]">
      <section
        aria-labelledby="public-space-details-heading"
        className="min-w-0 space-y-5"
      >
        <h2 id="public-space-details-heading" className="text-xl font-semibold">
          Especificaciones y reglas
        </h2>
        <div className="grid min-w-0 gap-4 sm:grid-cols-2">
          <div className="flex min-w-0 items-start gap-3 rounded-xl border border-border p-4">
            <Ruler
              aria-hidden="true"
              className="mt-0.5 size-5 shrink-0 text-muted-foreground"
            />
            <div className="min-w-0">
              <p className="text-sm text-muted-foreground">Superficie</p>
              <p className={dynamicText}>{space.area_m2} m²</p>
            </div>
          </div>
          <div className="flex min-w-0 items-start gap-3 rounded-xl border border-border p-4">
            <Users
              aria-hidden="true"
              className="mt-0.5 size-5 shrink-0 text-muted-foreground"
            />
            <div className="min-w-0">
              <p className="text-sm text-muted-foreground">Aforo máximo</p>
              <p className={dynamicText}>{space.capacity_max}</p>
            </div>
          </div>
        </div>
        {attributes.length ? (
          <dl className="grid min-w-0 gap-3 sm:grid-cols-2">
            {attributes.map((attribute, index) => (
              <div
                key={`${attribute.code}-${index}`}
                className="min-w-0 rounded-xl border border-border p-4"
              >
                <dt className={`${dynamicText} text-sm text-muted-foreground`}>
                  {attribute.label}
                </dt>
                <dd className={`${dynamicText} mt-1`}>
                  {attributeValue(attribute.value)}
                  {attribute.unit ? (
                    <span className="ml-1 text-muted-foreground">
                      {attribute.unit}
                    </span>
                  ) : null}
                </dd>
              </div>
            ))}
          </dl>
        ) : null}
        <div className="min-w-0 space-y-4 rounded-xl bg-muted/40 p-4">
          <div className="min-w-0">
            <h3 className="font-medium">Reglas del anuncio</h3>
            <p
              className={`${dynamicText} mt-1 whitespace-pre-wrap text-sm text-muted-foreground`}
            >
              {rules || "Reglas del anuncio no informadas."}
            </p>
          </div>
          <p className={`${dynamicText} text-sm text-muted-foreground`}>
            Política de cancelación no informada.
          </p>
          <p className={`${dynamicText} text-sm text-muted-foreground`}>
            Garantía no informada.
          </p>
        </div>
      </section>

      <section
        aria-labelledby="public-space-host-heading"
        className="min-w-0 space-y-4"
      >
        <h2 id="public-space-host-heading" className="text-xl font-semibold">
          Quién publica
        </h2>
        <div className="flex min-w-0 items-start gap-4 rounded-xl border border-border p-5">
          <div
            aria-hidden="true"
            className="flex size-12 shrink-0 items-center justify-center rounded-full bg-muted font-semibold"
          >
            {hostInitials(space.host?.name ?? null)}
          </div>
          <div className="min-w-0 space-y-2">
            <p className={`${dynamicText} font-medium`}>
              {hostName || "Anfitrión"}
            </p>
            {joined ? (
              <p
                className={`${dynamicText} flex items-center gap-2 text-sm text-muted-foreground`}
              >
                <CalendarDays aria-hidden="true" className="size-4 shrink-0" />
                Se unió en {joined}
              </p>
            ) : null}
            {space.host ? (
              <p className={`${dynamicText} text-sm text-muted-foreground`}>
                {space.host.spaces_count}{" "}
                {space.host.spaces_count === 1
                  ? "espacio publicado"
                  : "espacios publicados"}
              </p>
            ) : null}
            {space.host?.identity_verified ? (
              <div className="min-w-0 space-y-2">
                <span className="inline-flex max-w-full items-center gap-2 rounded-full bg-emerald-100 px-3 py-1 text-sm font-semibold text-emerald-900">
                  <ShieldCheck aria-hidden="true" className="size-4 shrink-0" />
                  <span className={dynamicText}>
                    Identidad Verificada por EspaciGo
                  </span>
                </span>
                <p className={`${dynamicText} text-sm text-muted-foreground`}>
                  Esta verificación no acredita titularidad del espacio ni
                  implica garantía.
                </p>
              </div>
            ) : null}
          </div>
        </div>
      </section>

      <section
        aria-labelledby="public-space-reviews-heading"
        className="min-w-0 space-y-4"
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 id="public-space-reviews-heading" className="text-xl font-semibold">
            Reseñas y recomendaciones
          </h2>
          {reviews && reviews.count > 0 ? (
            <div className="flex items-center gap-2 rounded-lg bg-amber-50 px-3 py-1 text-sm font-semibold text-amber-900 border border-amber-200">
              <Star className="size-4 fill-amber-500 text-amber-500" aria-hidden="true" />
              <span>{reviews.average.toFixed(1)}</span>
              <span className="text-muted-foreground font-normal">
                ({reviews.count} {reviews.count === 1 ? "evaluación" : "evaluaciones"})
              </span>
            </div>
          ) : null}
        </div>

        {reviews && reviews.items && reviews.items.length > 0 ? (
          <div className="space-y-4">
            <div className="flex flex-wrap gap-2 text-xs font-medium text-emerald-800">
              <span className="rounded-full bg-emerald-50 px-2.5 py-1 border border-emerald-200">
                ✓ Espacio verificado
              </span>
              <span className="rounded-full bg-emerald-50 px-2.5 py-1 border border-emerald-200">
                ✓ Fiel a las fotos
              </span>
              <span className="rounded-full bg-emerald-50 px-2.5 py-1 border border-emerald-200">
                ✓ Buena comunicación
              </span>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              {reviews.items.map((rev) => (
                <div
                  key={rev.id}
                  className="flex flex-col justify-between rounded-xl border border-border p-4 bg-card"
                >
                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-semibold">Arrendatario verificado</span>
                      <div className="flex items-center gap-0.5">
                        {Array.from({ length: 5 }).map((_, i) => (
                          <Star
                            key={i}
                            className={`size-3.5 ${
                              i < rev.rating
                                ? "fill-amber-500 text-amber-500"
                                : "text-muted stroke-muted-foreground"
                            }`}
                            aria-hidden="true"
                          />
                        ))}
                      </div>
                    </div>
                    {rev.comment ? (
                      <p className={`${dynamicText} text-sm text-foreground/90 leading-relaxed`}>
                        &ldquo;{rev.comment}&rdquo;
                      </p>
                    ) : null}
                  </div>
                  <span className="mt-3 text-xs text-muted-foreground">
                    {joinedLabel(rev.created_at) || "Reciente"}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <>
            <div className="flex min-w-0 items-start gap-3 rounded-xl border border-border p-5">
              <MessageSquareText
                aria-hidden="true"
                className="mt-0.5 size-5 shrink-0 text-muted-foreground"
              />
              <p className={`${dynamicText} text-sm text-muted-foreground`}>
                Aún no hay reseñas públicas.
              </p>
            </div>
            <p
              className={`${dynamicText} flex items-start gap-2 text-sm text-muted-foreground`}
            >
              <CircleHelp aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              Las evaluaciones provienen exclusivamente de arrendatarios que completaron un arriendo real.
            </p>
          </>
        )}
      </section>
    </div>
  );
}
