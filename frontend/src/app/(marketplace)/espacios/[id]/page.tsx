import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import {
  isPublicUuid,
  publicGalleryImages,
} from "@/features/public-catalog/domain/media";
import {
  formatPublicClp,
  safePublicText,
} from "@/features/public-catalog/domain/presentation";
import {
  addCivilDays,
  localDateISO,
} from "@/features/public-catalog/domain/local-date";
import { parseRentalReturnSelection } from "@/features/public-catalog/domain/availability-return";
import type { RentalReturnSelection } from "@/features/public-catalog/domain/availability-return";
import { publicCatalog } from "@/features/public-catalog/infrastructure/catalog-runtime";
import { getApiBackendBaseUrl } from "@/lib/api/server-config";
import { CatalogUnavailable } from "@/features/public-catalog/ui/catalog-states";
import { PublicSpaceRentalAvailability } from "@/features/public-catalog/ui/public-space-rental-availability";
import { PublicSpaceSummary } from "@/features/public-catalog/ui/public-space-summary";
import { PublicSpaceDetails } from "@/features/public-catalog/ui/public-space-details";
import { PublicSpaceMarketplaceGallery } from "@/features/public-catalog/ui/public-space-gallery";
import { PublicSpaceMarketplaceLayout } from "@/features/public-catalog/ui/public-space-marketplace-layout";

async function fetchSpaceReviews(spaceId: string) {
  try {
    const base = getApiBackendBaseUrl();
    const url = new URL(
      `/api/v1/spaces/${encodeURIComponent(spaceId)}/reviews`,
      base.origin,
    );
    const res = await fetch(url.toString(), { cache: "no-store" });
    if (!res.ok) return null;
    const json = await res.json();
    return json.data ?? null;
  } catch {
    return null;
  }
}

export const dynamic = "force-dynamic";
type PageProps = {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

function categoryHref(code: string) {
  return `/categorias/${encodeURIComponent(code)}`;
}

function jsonLd(value: unknown) {
  return JSON.stringify(value)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026");
}

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const { id } = await params;
  if (!isPublicUuid(id))
    return {
      title: "Espacio no encontrado",
      robots: { index: false, follow: true },
    };
  const result = await publicCatalog.space(id);
  if (!result.ok)
    return {
      title: "Espacio no disponible",
      robots: { index: false, follow: true },
    };
  const space = result.value;
  return {
    title: safePublicText(space.title),
    description: safePublicText(space.description).slice(0, 180),
    alternates: { canonical: `/espacios/${encodeURIComponent(id)}` },
  };
}

export default async function PublicSpacePage({
  params,
  searchParams,
}: PageProps) {
  const { id } = await params;
  if (!isPublicUuid(id)) notFound();
  const [result, reviews] = await Promise.all([
    publicCatalog.space(id),
    fetchSpaceReviews(id),
  ]);
  if (!result.ok) {
    if (result.error.kind === "not-found") notFound();
    return (
      <section className="mx-auto max-w-7xl px-4 py-12 sm:px-6 lg:px-8">
        <CatalogUnavailable
          title="No pudimos cargar esta publicación"
          retryHref={`/espacios/${encodeURIComponent(id)}`}
        />
      </section>
    );
  }
  const space = result.value;
  const rentalTimeZone = space.weekly_schedule?.time_zone ?? "America/Santiago";
  const rentalToday = localDateISO(new Date(), rentalTimeZone);
  const query = new URLSearchParams();
  const rawSearchParams = await searchParams;
  for (const [key, value] of Object.entries(rawSearchParams ?? {})) {
    for (const item of Array.isArray(value)
      ? value
      : value === undefined
        ? []
        : [value])
      query.append(key, item);
  }
  const candidateRentalSelection = parseRentalReturnSelection(query.toString());
  const validDurations =
    space.rate_unit === "hora"
      ? [1, 2, 4]
      : space.rate_unit === "dia"
        ? [1, 2, 3]
        : [1];
  const rentalSelection: RentalReturnSelection | undefined =
    candidateRentalSelection &&
    candidateRentalSelection.date >= rentalToday &&
    candidateRentalSelection.date <= addCivilDays(rentalToday, 90) &&
    validDurations.includes(candidateRentalSelection.duration)
      ? candidateRentalSelection
      : undefined;
  const title = safePublicText(space.title);
  const description = safePublicText(space.description);
  const images = publicGalleryImages(space.space_id, space.images);
  const price = formatPublicClp(space.base_price_clp);
  const estimate = formatPublicClp(space.estimated_total_clp);

  const placeJsonLd = {
    "@context": "https://schema.org",
    "@type": "Place",
    name: title,
    description,
  };
  const breadcrumb = (
    <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
      <Link href="/" className="hover:text-foreground">
        Inicio
      </Link>
      <span aria-hidden="true" className="px-2">
        /
      </span>
      <Link href="/explorar" className="hover:text-foreground">
        Explorar
      </Link>
      <span aria-hidden="true" className="px-2">
        /
      </span>
      <Link
        href={categoryHref(space.category_code)}
        className="hover:text-foreground"
      >
        {space.category_name}
      </Link>
      <span aria-hidden="true" className="px-2">
        /
      </span>
      <span aria-current="page" className="text-foreground">
        {title}
      </span>
    </nav>
  );
  return (
    <article className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
      <PublicSpaceMarketplaceLayout
        basePriceCLP={space.base_price_clp}
        rateUnit={space.rate_unit}
        summary={
          <PublicSpaceSummary
            title={title}
            description={description}
            space={space}
            price={price}
            estimate={estimate}
            breadcrumb={breadcrumb}
          />
        }
        availability={
          <PublicSpaceRentalAvailability
            key={space.space_id}
            spaceId={space.space_id}
            rateUnit={space.rate_unit}
            basePriceCLP={space.base_price_clp}
            today={rentalToday}
            initialSelection={rentalSelection}
          />
        }
        gallery={
          <PublicSpaceMarketplaceGallery
            key={space.space_id}
            images={images}
            productName={title}
          />
        }
      />
      <div className="mt-8">
        <PublicSpaceDetails space={space} reviews={reviews} />
      </div>
      <p className="mt-8 border-t border-border pt-5 text-xs leading-relaxed text-muted-foreground">
        Esta ficha muestra un perfil público y permite preparar una solicitud.
        No publica datos personales privados ni la dirección exacta; preparar
        una solicitud no confirma una reserva ni un pago.
      </p>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: jsonLd(placeJsonLd) }}
      />
    </article>
  );
}
