"use client";

import { useEffect, useRef, useState } from "react";
import type { Map as MapLibreMap, Marker } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

export type MapPin = {
  id: string;
  latitude: number;
  longitude: number;
  label: string;
  priceLabel?: string;
  compactPriceLabel?: string;
  visited?: boolean;
  approximate?: boolean;
};
export type MapBounds = {
  west: number;
  south: number;
  east: number;
  north: number;
};
export type MapCamera = { center: [number, number]; zoom: number };
export interface LocationMapProps {
  center: [number, number];
  zoom?: number;
  camera?: MapCamera;
  viewport?: MapBounds;
  markers: MapPin[];
  selectedId?: string;
  onSelect?: (id: string) => void;
  onSelectGroup?: (ids: string[]) => void;
  editable?: boolean;
  onMove?: (latitude: number, longitude: number) => void;
  onBoundsChange?: (bounds: MapBounds) => void;
  onUserMove?: () => void;
  onCameraChange?: (camera: MapCamera) => void;
  mapHeightClassName?: string;
}

const mapRegion = process.env.NEXT_PUBLIC_AMAZON_LOCATION_REGION;
const mapKey = process.env.NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY;
const styleURL =
  process.env.NEXT_PUBLIC_MAP_STYLE_URL ||
  (mapRegion && mapKey
    ? `https://maps.geo.${mapRegion}.amazonaws.com/v2/styles/Standard/descriptor?key=${encodeURIComponent(mapKey)}&color-scheme=Light`
    : undefined);

const CLUSTER_START_COUNT = 8;
const CLUSTER_CELL_SIZE = 52;
const MAX_PROJECTED_PINS = 600;
const VIEWPORT_BUFFER = 72;

export function LocationMap(props: LocationMapProps) {
  const element = useRef<HTMLDivElement>(null);
  const renderer = useRef<{
    map: MapLibreMap;
    createMarker: typeof Marker;
  } | null>(null);
  const latest = useRef(props);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [latitude, longitude] = props.center;
  const { west, south, east, north } = props.viewport ?? {};

  useEffect(() => {
    latest.current = props;
  });

  useEffect(() => {
    const container = element.current;
    if (!container || !styleURL) return;
    let cancelled = false;
    let resizeObserver: ResizeObserver | undefined;

    void import("maplibre-gl")
      .then(({ Map, Marker, NavigationControl }) => {
        if (cancelled) return;
        const initialize = () => {
          if (
            renderer.current ||
            !container.clientWidth ||
            !container.clientHeight
          )
            return;
          const [lng, lat] = latest.current.camera?.center ?? [
            latest.current.center[1],
            latest.current.center[0],
          ];
          try {
            const map = new Map({
              container,
              style: styleURL,
              center: [lng, lat],
              zoom: latest.current.camera?.zoom ?? latest.current.zoom ?? 13,
              maxPitch: 0,
              dragRotate: false,
              pitchWithRotate: false,
              scrollZoom: false,
              renderWorldCopies: false,
              attributionControl: { compact: true },
              locale: {
                "NavigationControl.ZoomIn": "Acercar",
                "NavigationControl.ZoomOut": "Alejar",
                "AttributionControl.ToggleAttribution": "Mostrar atribución",
                "Map.Title": "Mapa interactivo",
              },
            });
            renderer.current = { map, createMarker: Marker };
            map.addControl(new NavigationControl({ showCompass: false }));
            const updateBounds = () => {
              const bounds = map.getBounds();
              latest.current.onBoundsChange?.({
                west: Math.max(-180, bounds.getWest()),
                east: Math.min(180, bounds.getEast()),
                south: bounds.getSouth(),
                north: bounds.getNorth(),
              });
            };
            map.on("load", () => {
              setReady(true);
              setError(false);
              updateBounds();
            });
            map.on("error", () => {
              setError(true);
            });
            map.on("moveend", (event) => {
              updateBounds();
              const currentCenter = map.getCenter();
              latest.current.onCameraChange?.({
                center: [currentCenter.lng, currentCenter.lat],
                zoom: map.getZoom(),
              });
              if (event.originalEvent) latest.current.onUserMove?.();
            });
            map.on("click", ({ lngLat }) => {
              if (latest.current.editable) {
                latest.current.onMove?.(lngLat.lat, lngLat.wrap().lng);
              }
            });
          } catch {
            setError(true);
          }
        };

        resizeObserver = new ResizeObserver(() => {
          if (renderer.current) renderer.current.map.resize();
          else initialize();
        });
        resizeObserver.observe(container);
        initialize();
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });
    return () => {
      cancelled = true;
      resizeObserver?.disconnect();
      renderer.current?.map.remove();
      renderer.current = null;
    };
  }, [attempt]);

  useEffect(() => {
    const instance = renderer.current?.map;
    if (!ready || !instance) return;
    if (props.camera) {
      const current = instance.getCenter();
      if (
        Math.abs(current.lng - props.camera.center[0]) > 0.000001 ||
        Math.abs(current.lat - props.camera.center[1]) > 0.000001 ||
        Math.abs(instance.getZoom() - props.camera.zoom) > 0.001
      )
        instance.jumpTo({ center: props.camera.center, zoom: props.camera.zoom });
    } else if (
      west !== undefined &&
      south !== undefined &&
      east !== undefined &&
      north !== undefined
    ) {
      instance.fitBounds(
        [
          [west, south],
          [east, north],
        ],
        {
          duration: 0,
          maxZoom: 17,
        },
      );
    } else {
      instance.jumpTo({ center: [longitude, latitude], zoom: props.zoom });
    }
  }, [ready, latitude, longitude, props.zoom, props.camera, west, south, east, north]);

  useEffect(() => {
    const instance = renderer.current;
    if (!ready || !instance) return;
    const map = instance.map;
    let rendered: Marker[] = [];
    const renderMarkers = () => {
      rendered.forEach((marker) => marker.remove());
      rendered = [];
      const { width, height } = map.getContainer().getBoundingClientRect();
      const candidates = props.markers.slice(0, MAX_PROJECTED_PINS);
      const visible = candidates.flatMap((pin) => {
        const point = map.project([pin.longitude, pin.latitude]);
        return point.x >= -VIEWPORT_BUFFER &&
          point.x <= width + VIEWPORT_BUFFER &&
          point.y >= -VIEWPORT_BUFFER &&
          point.y <= height + VIEWPORT_BUFFER
          ? [{ pin, x: point.x, y: point.y }]
          : [];
      });
      const clusters = new Map<string, typeof visible>();
      if (!props.editable && props.markers.length >= CLUSTER_START_COUNT) {
        for (const item of visible) {
          const key = `${Math.floor(item.x / CLUSTER_CELL_SIZE)}:${Math.floor(item.y / CLUSTER_CELL_SIZE)}`;
          const group = clusters.get(key) ?? [];
          group.push(item);
          clusters.set(key, group);
        }
      }

      const addMarker = (
        element: HTMLElement,
        position: [number, number],
        draggable = false,
      ) => {
        const marker = new instance.createMarker({ element, draggable })
          .setLngLat(position)
          .addTo(map);
        rendered.push(marker);
        return marker;
      };
      const addPin = (pin: MapPin) => {
        const interactive = Boolean(latest.current.onSelect);
        const isSelected = pin.id === latest.current.selectedId;
        const target = interactive
          ? document.createElement("button")
          : document.createElement("span");
        if (target instanceof HTMLButtonElement) {
          target.type = "button";
          target.setAttribute("aria-pressed", String(isSelected));
          target.addEventListener("click", (event) => {
            event.stopPropagation();
            latest.current.onSelect?.(pin.id);
          });
        }
        target.title = pin.label;
        target.setAttribute("aria-label", pin.label);
        if (pin.priceLabel && !latest.current.editable) {
          const isMobile = width < 640;
          target.textContent =
            isMobile && pin.compactPriceLabel
              ? pin.compactPriceLabel
              : pin.priceLabel;
          const stateClass = isSelected
            ? "z-20 scale-110 border-foreground bg-foreground text-background ring-2 ring-primary/40"
            : pin.visited
              ? "border-border bg-muted text-muted-foreground hover:scale-105 hover:border-foreground"
              : "border-border bg-background text-foreground hover:scale-105 hover:border-foreground";
          target.className = `inline-flex min-h-8 items-center rounded-full border px-2.5 py-1 text-xs font-semibold tabular-nums shadow-md transition-transform ${stateClass} ${interactive ? "cursor-pointer focus-visible:scale-105 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary" : ""}`;
        } else {
          target.className = `grid size-10 place-items-center rounded-full ${interactive ? "cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary" : ""}`;
          const dot = document.createElement("span");
          dot.className =
            isSelected || latest.current.editable
              ? "size-5 rounded-full border-[3px] border-white bg-orange-700 shadow-md"
              : pin.visited
                ? "size-4 rounded-full border-2 border-white bg-slate-500 shadow-md"
                : "size-4 rounded-full border-2 border-white bg-emerald-700 shadow-md";
          target.append(dot);
        }
        if (pin.approximate && !latest.current.editable) {
          const ring = document.createElement("span");
          ring.setAttribute("aria-hidden", "true");
          ring.className =
            "pointer-events-none grid size-16 place-items-center rounded-full border border-emerald-700/50 bg-emerald-500/15";
          const markerRing = addMarker(
            ring,
            [pin.longitude, pin.latitude],
            false,
          );
          void markerRing;
        }
        const marker = addMarker(
          target,
          [pin.longitude, pin.latitude],
          latest.current.editable,
        );
        marker.on("dragend", () => {
          if (!latest.current.editable) return;
          const position = marker.getLngLat().wrap();
          latest.current.onMove?.(position.lat, position.lng);
        });
      };

      const grouped = new Set<string>();
      for (const group of clusters.values()) {
        if (group.length < 2) continue;
        group.forEach(({ pin }) => grouped.add(pin.id));
        const centerLng =
          group.reduce((sum, { pin }) => sum + pin.longitude, 0) / group.length;
        const centerLat =
          group.reduce((sum, { pin }) => sum + pin.latitude, 0) / group.length;
        const coincidentAtMaxZoom =
          map.getZoom() >= 16 ||
          group.every(
            ({ pin }) =>
              Math.abs(pin.latitude - centerLat) < 0.00005 &&
              Math.abs(pin.longitude - centerLng) < 0.00005,
          );
        const button = document.createElement("button");
        button.type = "button";
        button.className =
          "grid size-11 place-items-center rounded-full border-2 border-white bg-emerald-800 text-xs font-bold text-white shadow-lg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary";
        button.textContent = String(group.length);
        button.title = `${group.length} espacios agrupados`;
        button.setAttribute(
          "aria-label",
          coincidentAtMaxZoom && latest.current.onSelectGroup
            ? `${group.length} espacios en este punto. Abrir lista.`
            : `${group.length} espacios agrupados. Acercar el mapa para verlos.`,
        );
        button.addEventListener("click", (event) => {
          event.stopPropagation();
          if (coincidentAtMaxZoom && latest.current.onSelectGroup) {
            latest.current.onSelectGroup(group.map(({ pin }) => pin.id));
            return;
          }
          map.easeTo({
            center: [centerLng, centerLat],
            zoom: Math.min(map.getZoom() + 2, 17),
            duration: 350,
          });
        });
        addMarker(button, [centerLng, centerLat]);
      }
      for (const { pin } of visible) {
        if (!grouped.has(pin.id)) addPin(pin);
      }
    };

    renderMarkers();
    map.on("moveend", renderMarkers);
    return () => {
      map.off("moveend", renderMarkers);
      rendered.forEach((marker) => marker.remove());
    };
  }, [ready, props.markers, props.selectedId, props.editable]);

  return (
    <div className="space-y-2">
      <div
        ref={element}
        aria-label={
          props.editable
            ? "Mapa: selecciona el punto del espacio"
            : "Mapa de espacios"
        }
        className={
          styleURL
            ? `relative z-0 w-full rounded-xl border bg-muted ${props.mapHeightClassName ?? "h-80 sm:h-96"}`
            : "hidden"
        }
      />
      {styleURL && !ready && !error && (
        <p role="status" className="text-sm text-muted-foreground">
          Cargando mapa…
        </p>
      )}
      {(!styleURL || error) && (
        <div className="flex flex-wrap items-center gap-2">
          <p role="status" className="text-sm text-muted-foreground">
            {error
              ? "No se pudo cargar la cartografía."
              : "Cartografía no configurada."}{" "}
            {props.editable
              ? "Puedes ingresar coordenadas manualmente."
              : "La cuadrícula y la dirección siguen disponibles."}
          </p>
          {styleURL && error && (
            <button
              type="button"
              className="min-h-11 rounded-lg border px-3 text-sm"
              onClick={() => {
                setReady(false);
                setError(false);
                setAttempt((value) => value + 1);
              }}
            >
              Reintentar mapa
            </button>
          )}
        </div>
      )}
    </div>
  );
}
