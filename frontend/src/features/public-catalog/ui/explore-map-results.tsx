"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import type { MapBounds } from "@/components/maps/location-map";
import { getCatalogCommuneCenter } from "../domain/commune-centers";
import { publicImageUrl } from "../domain/media";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import {
  serializeCatalogApiQuery,
  serializeCatalogQuery,
} from "../domain/query-state";
import type { CatalogQuery, PublicMapMarker } from "../domain/types";
import {
  type ExploreSession,
  readExploreSession,
  readVisitedSpaces,
  updateExploreSession,
} from "./explore-session";
import { SpaceNearbyPois } from "./space-nearby-pois";
import { subscribeToCookiePreferences } from "@/features/cookie-preferences/preferences";

const LocationMap = dynamic(
  () => import("@/components/maps/location-map").then((m) => m.LocationMap),
  { ssr: false, loading: () => <p role="status">Cargando mapa…</p> },
);

const shortUnitSuffix: Record<PublicMapMarker["rate_unit"], string> = {
  hora: "h",
  dia: "día",
  mes: "mes",
};

function formatPillPrices(marker: PublicMapMarker): {
  priceLabel: string;
  compactPriceLabel: string;
} {
  const hasEstimate =
    marker.estimated_total_clp !== undefined &&
    marker.estimated_total_clp !== null;
  const rawAmount = Number(
    hasEstimate ? marker.estimated_total_clp : marker.base_price_clp,
  );
  const fullAmount =
    formatPublicClp(
      hasEstimate ? marker.estimated_total_clp : marker.base_price_clp,
    ) ?? `$${rawAmount}`;
  const compactAmount =
    Number.isFinite(rawAmount) && rawAmount >= 1000
      ? `$${Math.round(rawAmount / 1000)}k`
      : fullAmount;
  if (hasEstimate) {
    return {
      priceLabel: `${fullAmount} total`,
      compactPriceLabel: `${compactAmount} tot`,
    };
  }
  const suffix = shortUnitSuffix[marker.rate_unit] ?? marker.rate_unit;
  return {
    priceLabel: `${fullAmount}/${suffix}`,
    compactPriceLabel: `${compactAmount}/${suffix}`,
  };
}

function markerCoverSrc(marker: PublicMapMarker): string | null {
  if (marker.cover_image_id) {
    return publicImageUrl(
      marker.space_id,
      { image_id: marker.cover_image_id, is_cover: true, source: "media" },
      "thumbnail",
    );
  }
  return marker.cover_url ?? null;
}

export function ExploreMapResults({
  query,
  markers,
  mapUnavailable,
  mapTruncated,
  totalCount,
}: {
  query: CatalogQuery;
  markers: PublicMapMarker[];
  mapUnavailable: boolean;
  mapTruncated?: boolean;
  totalCount?: number;
}) {
  const router = useRouter();
  const [session, setSession] = useState<ExploreSession>();
  const [visitedIds, setVisitedIds] = useState<string[]>([]);
  const [bounds, setBounds] = useState<MapBounds>();
  const [coincidentIds, setCoincidentIds] = useState<string[] | null>(null);
  const [nearbyMarkers, setNearbyMarkers] = useState<PublicMapMarker[] | null>(
    null,
  );
  const [nearbyState, setNearbyState] = useState<"idle" | "loading" | "error">(
    "idle",
  );
  const nearbyAbortRef = useRef<AbortController | null>(null);
  const panelTouchStart = useRef<number | null>(null);
  const itemRefs = useRef(new Map<string, HTMLLIElement>());
  const filters = serializeCatalogQuery({
    ...query,
    cursor: undefined,
  }).toString();

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const saved = readExploreSession(filters);
      setSession(saved);
      setVisitedIds(readVisitedSpaces());
      if (saved.mapScrollY !== undefined) window.scrollTo(0, saved.mapScrollY);
    });
    return () => {
      cancelAnimationFrame(frame);
      nearbyAbortRef.current?.abort();
    };
  }, [filters]);

  useEffect(
    () =>
      subscribeToCookiePreferences(() => setVisitedIds(readVisitedSpaces())),
    [],
  );

  const selectedId = session?.selectedSpaceId;
  const sheetHeight =
    session?.sheetHeight ??
    (session?.sheetOpen || Boolean(selectedId) ? "half" : "collapsed");
  const sheetOpen = sheetHeight !== "collapsed";
  const hasUserMovedMap = session?.areaPending ?? false;
  const activeMarkers = nearbyMarkers ?? markers;
  const visibleMarkers = useMemo(() => {
    if (!coincidentIds?.length) return activeMarkers;
    const set = new Set(coincidentIds);
    const filtered = activeMarkers.filter((m) => set.has(m.space_id));
    return filtered.length ? filtered : activeMarkers;
  }, [activeMarkers, coincidentIds]);

  function updateSession(update: Partial<ExploreSession>) {
    setSession((current) => current && { ...current, ...update });
    updateExploreSession(filters, update);
  }

  function setSheetState(next: "collapsed" | "half" | "expanded") {
    updateSession({ sheetHeight: next, sheetOpen: next !== "collapsed" });
  }

  function selectSpace(id: string) {
    setCoincidentIds(null);
    updateSession({
      selectedSpaceId: id,
      sheetOpen: true,
      sheetHeight: sheetHeight === "collapsed" ? "half" : sheetHeight,
    });
    const el = itemRefs.current.get(id);
    el?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }

  const center = useMemo<[number, number]>(() => {
    if (query.latitude !== undefined && query.longitude !== undefined)
      return [Number(query.latitude), Number(query.longitude)];
    if (activeMarkers[0])
      return [
        activeMarkers[0].location.latitude,
        activeMarkers[0].location.longitude,
      ];
    if (query.comuna_code) {
      const commune = getCatalogCommuneCenter(query.comuna_code);
      if (commune) return [commune.latitude, commune.longitude];
    }
    return [-33.45, -70.66];
  }, [query.latitude, query.longitude, query.comuna_code, activeMarkers]);

  const viewport = useMemo<MapBounds | undefined>(() => {
    if (nearbyMarkers?.length) return undefined;
    if (query.bbox) {
      const [west, south, east, north] = query.bbox.split(",").map(Number);
      return { west, south, east, north };
    }
    if (query.latitude === undefined || query.longitude === undefined)
      return undefined;
    const radius = Number(query.radius_km ?? "10");
    const lat = Number(query.latitude);
    const lng = Number(query.longitude);
    const latDelta = radius / 110.574;
    const lngDelta =
      radius / (111.32 * Math.max(0.15, Math.cos((lat * Math.PI) / 180)));
    return {
      west: Math.max(-180, lng - lngDelta),
      east: Math.min(180, lng + lngDelta),
      south: Math.max(-85, lat - latDelta),
      north: Math.min(85, lat + latDelta),
    };
  }, [
    nearbyMarkers,
    query.bbox,
    query.latitude,
    query.longitude,
    query.radius_km,
  ]);

  const visitedSet = useMemo(() => new Set(visitedIds), [visitedIds]);
  const pins = useMemo(
    () =>
      activeMarkers.map((m) => ({
        id: m.space_id,
        latitude: m.location.latitude,
        longitude: m.location.longitude,
        label: m.title,
        visited: visitedSet.has(m.space_id),
        approximate: m.location.visibility === "approximate",
        ...formatPillPrices(m),
      })),
    [activeMarkers, visitedSet],
  );
  const selected = activeMarkers.find((m) => m.space_id === selectedId);
  const selectedCover = selected ? markerCoverSrc(selected) : null;

  function navigate(next: CatalogQuery) {
    router.push(
      `/explorar/mapa?${serializeCatalogQuery({ ...next, cursor: undefined })}`,
      { scroll: false },
    );
  }

  function applyArea() {
    if (!bounds || !hasUserMovedMap) return;
    setNearbyMarkers(null);
    setCoincidentIds(null);
    navigate({
      ...query,
      areaMode: "bbox",
      bbox: [bounds.west, bounds.south, bounds.east, bounds.north].join(","),
      latitude: undefined,
      longitude: undefined,
      radius_km: undefined,
      region_code: undefined,
      comuna_code: undefined,
      sort: query.sort === "distance_asc" ? undefined : query.sort,
    });
    updateSession({ areaPending: false });
  }

  function applyRadius() {
    if (!bounds || !hasUserMovedMap) return;
    setNearbyMarkers(null);
    setCoincidentIds(null);
    navigate({
      ...query,
      areaMode: "radius",
      bbox: undefined,
      latitude: ((bounds.south + bounds.north) / 2).toFixed(5),
      longitude: ((bounds.west + bounds.east) / 2).toFixed(5),
      radius_km: query.radius_km ?? "10",
      region_code: undefined,
      comuna_code: undefined,
      sort:
        query.sort && query.sort !== "default" ? query.sort : "distance_asc",
    });
    updateSession({ areaPending: false });
  }

  async function loadNearbyOutsideArea() {
    nearbyAbortRef.current?.abort();
    const controller = new AbortController();
    nearbyAbortRef.current = controller;
    setNearbyState("loading");
    try {
      const apiParams = serializeCatalogApiQuery({
        ...query,
        areaMode: undefined,
        bbox: undefined,
        region_code: undefined,
        comuna_code: undefined,
        radius_km: undefined,
        sort: undefined,
        cursor: undefined,
      });
      const fallbackLat =
        bounds !== undefined
          ? ((bounds.south + bounds.north) / 2).toFixed(5)
          : String(center[0]);
      const fallbackLng =
        bounds !== undefined
          ? ((bounds.west + bounds.east) / 2).toFixed(5)
          : String(center[1]);
      apiParams.set("latitude", query.latitude ?? fallbackLat);
      apiParams.set("longitude", query.longitude ?? fallbackLng);
      if (query.bbox) apiParams.set("exclude_bbox", query.bbox);
      else if (query.radius_km)
        apiParams.set("exclude_radius_km", query.radius_km);

      const res = await fetch(
        `/api/backend/public/spaces/nearby?${apiParams.toString()}`,
        {
          headers: { Accept: "application/json" },
          signal: controller.signal,
        },
      );
      if (!res.ok) {
        setNearbyState("error");
        return;
      }
      const data = (await res.json()) as { items?: PublicMapMarker[] };
      setNearbyMarkers(Array.isArray(data.items) ? data.items : []);
      setNearbyState("idle");
      setSheetState("half");
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setNearbyState("error");
    }
  }

  function handleListKeyDown(event: React.KeyboardEvent) {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    if (!visibleMarkers.length) return;
    event.preventDefault();
    const index = visibleMarkers.findIndex((m) => m.space_id === selectedId);
    const nextIndex =
      event.key === "ArrowDown"
        ? Math.min(visibleMarkers.length - 1, index + 1)
        : Math.max(0, index <= 0 ? 0 : index - 1);
    const next = visibleMarkers[nextIndex];
    if (next) selectSpace(next.space_id);
  }

  const sheetHeightClass =
    sheetHeight === "expanded"
      ? "block max-h-[78vh]"
      : sheetHeight === "half"
        ? "block max-h-[48vh]"
        : "hidden";

  return (
    <section aria-label="Resultados en el mapa" className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {nearbyMarkers !== null
          ? "Mostrando hasta 6 espacios cercanos fuera de la zona original."
          : query.bbox
            ? "Mostrando el área rectangular del mapa."
            : query.latitude !== undefined
              ? `Mostrando espacios dentro de ${query.radius_km ?? "10"} km de una ubicación aproximada.`
              : "Explora espacios por ubicación en el mapa."}
      </p>

      {nearbyMarkers !== null && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-2 rounded-xl border bg-muted/60 px-4 py-3 text-sm"
        >
          <span>
            Mostrando espacios cercanos fuera de tu zona original manteniendo
            tus filtros.
          </span>
          <button
            type="button"
            onClick={() => {
              setNearbyMarkers(null);
              setCoincidentIds(null);
            }}
            className="min-h-10 rounded-full border bg-background px-4 font-medium hover:bg-muted"
          >
            Volver al área original
          </button>
        </div>
      )}

      <div className="relative lg:grid lg:min-h-[min(78vh,760px)] lg:grid-cols-[minmax(18rem,24rem)_minmax(0,1fr)] lg:gap-4">
        <div className="relative min-h-[58vh] overflow-hidden rounded-2xl border bg-muted lg:order-2 lg:min-h-0 lg:h-[min(78vh,760px)]">
          {session && (
            <LocationMap
              center={center}
              camera={session.camera}
              viewport={viewport}
              markers={pins}
              selectedId={selectedId}
              onSelect={selectSpace}
              onSelectGroup={(ids) => {
                setCoincidentIds(ids);
                if (ids[0]) updateSession({ selectedSpaceId: ids[0] });
                setSheetState("half");
              }}
              onBoundsChange={setBounds}
              onCameraChange={(camera) => updateSession({ camera })}
              onUserMove={() => updateSession({ areaPending: true })}
              mapHeightClassName="h-[58vh] min-h-[26rem] sm:h-[68vh] lg:h-full"
            />
          )}
          {mapTruncated && !nearbyMarkers && !mapUnavailable && (
            <div
              role="status"
              className="absolute bottom-16 left-3 right-3 z-10 mx-auto max-w-md rounded-full bg-background/95 px-4 py-2 text-center text-xs font-medium shadow-lg lg:bottom-3"
            >
              Mostrando {markers.length}
              {totalCount && totalCount > markers.length
                ? ` de ${totalCount}`
                : ""}{" "}
              espacios. Acércate para ver más espacios en esta zona.
            </div>
          )}
          {hasUserMovedMap && !mapUnavailable && (
            <div className="absolute left-3 right-12 top-3 z-10 flex flex-wrap items-center justify-center gap-2 sm:left-auto sm:right-14">
              <span
                role="status"
                className="rounded-full bg-background px-3 py-2 text-xs font-medium shadow-lg"
              >
                Área pendiente
              </span>
              <button
                type="button"
                onClick={applyArea}
                className="min-h-11 rounded-full bg-background px-4 text-sm font-semibold shadow-lg"
              >
                Buscar en esta zona
              </button>
              <button
                type="button"
                onClick={applyRadius}
                className="min-h-11 rounded-full bg-background px-4 text-sm font-semibold shadow-lg"
              >
                Buscar en un radio
              </button>
            </div>
          )}
          <button
            type="button"
            aria-expanded={sheetOpen}
            aria-controls="map-results-list"
            onClick={() => setSheetState(sheetOpen ? "collapsed" : "half")}
            className="absolute bottom-3 left-1/2 z-10 min-h-11 -translate-x-1/2 rounded-full bg-background px-5 text-sm font-semibold shadow-lg lg:hidden"
          >
            {sheetOpen
              ? "Ocultar resultados"
              : `Ver ${activeMarkers.length} ${activeMarkers.length === 1 ? "resultado" : "resultados"}`}
          </button>
        </div>

        <div
          id="map-results-list"
          onKeyDown={handleListKeyDown}
          className={`${sheetHeightClass} absolute inset-x-0 bottom-0 z-20 overflow-y-auto rounded-t-2xl border bg-background p-4 shadow-2xl lg:static lg:order-1 lg:block lg:max-h-[min(78vh,760px)] lg:rounded-2xl lg:shadow-none`}
        >
          <div
            className="mb-3 flex flex-wrap items-center justify-between gap-2"
            onTouchStart={(event) => {
              panelTouchStart.current = event.touches[0]?.clientY ?? null;
            }}
            onTouchEnd={(event) => {
              const start = panelTouchStart.current;
              const end = event.changedTouches[0]?.clientY;
              panelTouchStart.current = null;
              if (start !== null && end !== undefined) {
                if (end - start > 44) setSheetState("collapsed");
                else if (start - end > 44) setSheetState("expanded");
              }
            }}
            onTouchCancel={() => {
              panelTouchStart.current = null;
            }}
          >
            <h2 className="font-semibold">
              {coincidentIds?.length
                ? `${visibleMarkers.length} espacios en este punto`
                : "Espacios en el mapa"}
            </h2>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-muted-foreground">
                {activeMarkers.length} espacios con ubicación confirmada
                mostrados
              </span>
              {coincidentIds?.length ? (
                <button
                  type="button"
                  onClick={() => setCoincidentIds(null)}
                  className="min-h-9 rounded-full border px-3 text-xs font-medium"
                >
                  Ver todos
                </button>
              ) : null}
              <button
                type="button"
                onClick={() =>
                  setSheetState(
                    sheetHeight === "expanded" ? "half" : "expanded",
                  )
                }
                className="min-h-9 rounded-full border px-3 text-xs font-medium lg:hidden"
              >
                {sheetHeight === "expanded" ? "Contraer" : "Expandir"}
              </button>
              <button
                type="button"
                onClick={() => setSheetState("collapsed")}
                className="min-h-11 rounded-full border px-3 text-sm lg:hidden"
              >
                Ocultar resultados
              </button>
            </div>
          </div>

          {selected && (
            <article
              aria-live="polite"
              className="mb-3 overflow-hidden rounded-xl border bg-muted/20"
            >
              {selectedCover && (
                /* eslint-disable-next-line @next/next/no-img-element */
                <img
                  src={selectedCover}
                  alt=""
                  className="h-36 w-full object-cover"
                />
              )}
              <div className="p-4">
                <h3 className="font-medium">{selected.title}</h3>
                {selected.comuna_name && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {selected.comuna_name}
                  </p>
                )}
                <p className="mt-1 text-sm">{selected.location.address}</p>
                <p className="mt-1 text-sm font-medium tabular-nums">
                  {selected.estimated_total_clp !== undefined &&
                  selected.estimated_total_clp !== null
                    ? `${formatPublicClp(selected.estimated_total_clp)} total estimado`
                    : `${formatPublicClp(selected.base_price_clp)} / ${rateUnitLabel(selected.rate_unit)}`}
                  {selected.distance_km !== undefined
                    ? ` · ${selected.distance_km.toFixed(1)} km`
                    : ""}
                </p>
                <Link
                  href={`/espacios/${encodeURIComponent(selected.space_id)}`}
                  className="mt-3 inline-flex min-h-11 items-center rounded-full bg-foreground px-4 text-sm font-medium text-background"
                >
                  Ver espacio
                </Link>
                <SpaceNearbyPois
                  key={selected.space_id}
                  spaceId={selected.space_id}
                />
              </div>
            </article>
          )}

          {mapUnavailable ? (
            <p
              role="status"
              className="rounded-lg bg-muted p-3 text-sm text-muted-foreground"
            >
              No pudimos cargar los marcadores. La cuadrícula sigue disponible.
            </p>
          ) : visibleMarkers.length === 0 ? (
            <div className="space-y-3 rounded-lg bg-muted p-4 text-sm text-muted-foreground">
              <p>
                No hay espacios con ubicación confirmada para mostrar en esta
                búsqueda.
              </p>
              {nearbyMarkers === null && (
                <button
                  type="button"
                  disabled={nearbyState === "loading"}
                  onClick={loadNearbyOutsideArea}
                  className="min-h-11 rounded-full bg-foreground px-4 text-sm font-medium text-background disabled:opacity-60"
                >
                  {nearbyState === "loading"
                    ? "Buscando cercanos…"
                    : "Ver cercanos fuera de esta zona"}
                </button>
              )}
              {nearbyState === "error" && (
                <p role="status" className="text-xs text-destructive">
                  No pudimos cargar espacios cercanos en este momento.
                </p>
              )}
            </div>
          ) : (
            <ul className="space-y-2">
              {visibleMarkers.map((marker) => {
                const isSelected = selectedId === marker.space_id;
                const isVisited = visitedSet.has(marker.space_id);
                return (
                  <li
                    key={marker.space_id}
                    ref={(el) => {
                      if (el) itemRefs.current.set(marker.space_id, el);
                      else itemRefs.current.delete(marker.space_id);
                    }}
                  >
                    <button
                      type="button"
                      aria-pressed={isSelected}
                      onClick={() => selectSpace(marker.space_id)}
                      className={`w-full rounded-xl border p-3 text-left transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary ${
                        isSelected
                          ? "border-primary bg-muted"
                          : isVisited
                            ? "bg-muted/30"
                            : ""
                      }`}
                    >
                      <span className="block font-medium">{marker.title}</span>
                      {marker.comuna_name && (
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {marker.comuna_name}
                        </span>
                      )}
                      <span className="mt-1 block text-sm tabular-nums text-muted-foreground">
                        {marker.estimated_total_clp !== undefined &&
                        marker.estimated_total_clp !== null
                          ? `${formatPublicClp(marker.estimated_total_clp)} total estimado`
                          : `${formatPublicClp(marker.base_price_clp)} / ${rateUnitLabel(marker.rate_unit)}`}
                        {marker.distance_km !== undefined
                          ? ` · ${marker.distance_km.toFixed(1)} km`
                          : ""}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
          <p className="mt-3 text-xs text-muted-foreground">
            {mapTruncated
              ? "Se alcanzó el límite de 250 marcadores para el mapa. Acércate o ajusta los filtros para explorar otros espacios."
              : "Puede haber más espacios que los mostrados en el mapa. Los horarios se confirman al cotizar."}
          </p>
        </div>
      </div>
    </section>
  );
}
