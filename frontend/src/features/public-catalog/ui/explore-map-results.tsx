"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type ReactNode } from "react";
import type { MapBounds } from "@/components/maps/location-map";
import { serializeCatalogQuery } from "../domain/query-state";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import type { CatalogQuery, PublicMapMarker } from "../domain/types";

const LocationMap = dynamic(
  () => import("@/components/maps/location-map").then((m) => m.LocationMap),
  { ssr: false, loading: () => <p role="status">Cargando mapa…</p> },
);

export function ExploreMapResults({
  query,
  markers,
  mapUnavailable,
  children,
}: {
  query: CatalogQuery;
  markers: PublicMapMarker[];
  mapUnavailable: boolean;
  children: ReactNode;
}) {
  const router = useRouter();
  const [view, setView] = useState<"grid" | "map">("grid");
  const [selectedId, setSelectedId] = useState<string>();
  const [bounds, setBounds] = useState<MapBounds>();
  const [message, setMessage] = useState("");
  const [locating, setLocating] = useState(false);
  const center = useMemo<[number, number]>(
    () =>
      query.latitude !== undefined
        ? [Number(query.latitude), Number(query.longitude)]
        : markers[0]
          ? [markers[0].location.latitude, markers[0].location.longitude]
          : [-33.45, -70.66],
    [query.latitude, query.longitude, markers],
  );
  const pins = useMemo(
    () =>
      markers.map((m) => ({
        id: m.space_id,
        latitude: m.location.latitude,
        longitude: m.location.longitude,
        label: m.title,
      })),
    [markers],
  );
  const selected = markers.find((m) => m.space_id === selectedId);
  function navigate(next: CatalogQuery) {
    router.push(
      `/explorar?${serializeCatalogQuery({ ...next, cursor: undefined })}`,
      { scroll: false },
    );
  }
  function nearMe() {
    if (!window.isSecureContext || !navigator.geolocation) {
      setMessage(
        "La ubicación no está disponible. Busca por destino o comuna, o mueve el mapa.",
      );
      return;
    }
    setLocating(true);
    setMessage("");
    navigator.geolocation.getCurrentPosition(
      (position) => {
        setLocating(false);
        if (position.coords.accuracy > 5000) {
          setMessage(
            "La ubicación es imprecisa. Busca por comuna o mueve el mapa.",
          );
          return;
        }
        navigate({
          ...query,
          bbox: undefined,
          latitude: position.coords.latitude.toFixed(2),
          longitude: position.coords.longitude.toFixed(2),
          radius_km: "10",
          sort: "distance_asc",
        });
      },
      () => {
        setLocating(false);
        setMessage(
          "No pudimos usar tu ubicación. Puedes escribir un destino, elegir comuna o mover el mapa.",
        );
      },
      { timeout: 10000, maximumAge: 60000 },
    );
  }
  return (
    <section aria-label="Resultados y mapa" className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <div
          className="inline-flex rounded-full border p-1 lg:hidden"
          aria-label="Vista de resultados"
        >
          {(["grid", "map"] as const).map((mode) => (
            <button
              key={mode}
              type="button"
              aria-pressed={view === mode}
              onClick={() => setView(mode)}
              className={`min-h-11 rounded-full px-4 text-sm ${view === mode ? "bg-foreground text-background" : ""}`}
            >
              {mode === "grid" ? "Cuadrícula" : "Mapa"}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={nearMe}
          disabled={locating}
          className="min-h-11 rounded-full border px-4 text-sm"
        >
          {locating ? "Buscando ubicación…" : "Cerca de mí"}
        </button>
        {query.bbox || query.latitude ? (
          <button
            type="button"
            onClick={() =>
              navigate({
                ...query,
                bbox: undefined,
                latitude: undefined,
                longitude: undefined,
                radius_km: undefined,
                sort: query.sort === "distance_asc" ? undefined : query.sort,
              })
            }
            className="min-h-11 rounded-full border px-4 text-sm"
          >
            Quitar zona
          </button>
        ) : null}
      </div>
      {query.latitude !== undefined && (
        <p className="text-sm text-muted-foreground">
          Búsqueda en un radio de {query.radius_km ?? "10"} km desde una
          ubicación aproximada, redondeada a 2 decimales.
        </p>
      )}
      {message && (
        <p role="status" className="text-sm text-muted-foreground">
          {message}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-2">
        <div
          className={`${view === "map" ? "hidden" : ""} min-w-0 lg:block`}
          onFocusCapture={(event) => {
            const card = (event.target as HTMLElement).closest<HTMLElement>(
              "[data-space-id]",
            );
            if (card?.dataset.spaceId) setSelectedId(card.dataset.spaceId);
          }}
        >
          {children}
        </div>
        <div className={`${view === "grid" ? "hidden" : ""} min-w-0 lg:block`}>
          <div className="space-y-3 lg:sticky lg:top-24">
            <LocationMap
              center={center}
              markers={pins}
              selectedId={selectedId}
              onSelect={(id) => {
                setSelectedId(id);
                document
                  .getElementById(`space-card-${id}`)
                  ?.focus({ preventScroll: true });
              }}
              onBoundsChange={setBounds}
            />
            <button
              type="button"
              disabled={!bounds}
              onClick={() => {
                if (bounds)
                  navigate({
                    ...query,
                    bbox: [
                      bounds.west,
                      bounds.south,
                      bounds.east,
                      bounds.north,
                    ].join(","),
                    latitude: undefined,
                    longitude: undefined,
                    radius_km: undefined,
                    sort:
                      query.sort === "distance_asc" ? undefined : query.sort,
                  });
              }}
              className="min-h-11 w-full rounded-full bg-foreground px-5 text-sm font-medium text-background disabled:opacity-50"
            >
              Buscar en esta zona
            </button>
            {mapUnavailable ? (
              <p role="status" className="text-sm text-muted-foreground">
                No pudimos cargar los marcadores. La cuadrícula sigue
                disponible.
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">
                {markers.length} espacios con ubicación confirmada. Mueve el
                mapa para buscar otra zona.
              </p>
            )}
            {selected && (
              <article aria-live="polite" className="rounded-xl border p-4">
                <h3 className="font-medium">{selected.title}</h3>
                <p className="mt-2 text-sm">{selected.location.address}</p>
                <p className="mt-1 text-sm">
                  {formatPublicClp(selected.base_price_clp)} /{" "}
                  {rateUnitLabel(selected.rate_unit)}
                  {selected.distance_km !== undefined
                    ? ` · ${selected.distance_km.toFixed(1)} km`
                    : ""}
                </p>
                <Link
                  href={`/espacios/${encodeURIComponent(selected.space_id)}`}
                  className="mt-3 inline-flex min-h-11 items-center underline"
                >
                  Ver espacio
                </Link>
              </article>
            )}
            <p className="text-xs text-muted-foreground">
              Los horarios se confirman al cotizar. También puedes buscar por
              destino o comuna sin compartir tu ubicación.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
