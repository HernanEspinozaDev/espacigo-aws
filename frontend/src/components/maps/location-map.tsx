"use client";

import { useEffect, useRef, useState } from "react";
import type { Map as LeafletMap } from "leaflet";
import "leaflet/dist/leaflet.css";

export type MapPin = {
  id: string;
  latitude: number;
  longitude: number;
  label: string;
};
export type MapBounds = {
  west: number;
  south: number;
  east: number;
  north: number;
};
export interface LocationMapProps {
  center: [number, number];
  markers: MapPin[];
  selectedId?: string;
  onSelect?: (id: string) => void;
  editable?: boolean;
  onMove?: (latitude: number, longitude: number) => void;
  onBoundsChange?: (bounds: MapBounds) => void;
  tileURL?: string;
  tileAttribution?: string;
}
export function LocationMap(props: LocationMapProps) {
  const element = useRef<HTMLDivElement>(null);
  const map = useRef<LeafletMap | null>(null);
  const latest = useRef(props);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState(false);
  useEffect(() => {
    latest.current = props;
  });
  useEffect(() => {
    let cancelled = false;
    let resizeObserver: ResizeObserver | null = null;
    void import("leaflet")
      .then((L) => {
        if (cancelled || !element.current) return;
        const initialize = () => {
          if (
            cancelled ||
            !element.current ||
            map.current ||
            element.current.clientWidth === 0 ||
            element.current.clientHeight === 0
          )
            return;
          const instance = L.map(element.current).setView(
            latest.current.center,
            13,
          );
          map.current = instance;
          const url =
            latest.current.tileURL ?? process.env.NEXT_PUBLIC_MAP_TILE_URL;
          const attribution =
            latest.current.tileAttribution ??
            process.env.NEXT_PUBLIC_MAP_ATTRIBUTION;
          if (url && attribution)
            L.tileLayer(url, { attribution, maxZoom: 19 })
              .on("tileerror", () => setError(true))
              .addTo(instance);
          instance.on("click", (event) => {
            if (latest.current.editable)
              latest.current.onMove?.(event.latlng.lat, event.latlng.lng);
          });
          instance.on("moveend", () => {
            const b = instance.getBounds();
            latest.current.onBoundsChange?.({
              west: b.getWest(),
              south: b.getSouth(),
              east: b.getEast(),
              north: b.getNorth(),
            });
          });
          setReady(true);
        };
        resizeObserver = new ResizeObserver((entries) => {
          if (
            entries.some(
              ({ contentRect }) =>
                contentRect.width > 0 && contentRect.height > 0,
            )
          ) {
            if (!map.current) initialize();
            else map.current.invalidateSize({ pan: true, animate: false });
          }
        });
        resizeObserver.observe(element.current);
        initialize();
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });
    return () => {
      cancelled = true;
      resizeObserver?.disconnect();
      map.current?.remove();
      map.current = null;
    };
  }, []);
  useEffect(() => {
    if (!ready || !map.current) return;
    map.current.panTo(props.center, { animate: false });
  }, [ready, props.center]);
  useEffect(() => {
    if (!ready || !map.current) return;
    let cancelled = false;
    let cleanup = () => {};
    void import("leaflet").then((L) => {
      if (cancelled || !map.current) return;
      const layer = L.layerGroup().addTo(map.current);
      cleanup = () => {
        layer.remove();
      };
      for (const pin of props.markers) {
        const marker = L.circleMarker([pin.latitude, pin.longitude], {
          radius: pin.id === props.selectedId ? 11 : 8,
          color: "#065f46",
          fillColor: "#10b981",
          fillOpacity: 0.9,
        }).addTo(layer);
        marker.bindTooltip(() => {
          const label = document.createElement("span");
          label.textContent = pin.label;
          return label;
        });
        marker.on("click", () => latest.current.onSelect?.(pin.id));
      }
    });
    return () => {
      cancelled = true;
      cleanup();
    };
  }, [ready, props.markers, props.selectedId]);
  const configured = Boolean(
    (props.tileURL ?? process.env.NEXT_PUBLIC_MAP_TILE_URL) &&
    (props.tileAttribution ?? process.env.NEXT_PUBLIC_MAP_ATTRIBUTION),
  );
  return (
    <div className="space-y-2">
      <div
        ref={element}
        aria-label={
          props.editable
            ? "Mapa: selecciona el punto del espacio"
            : "Mapa de espacios"
        }
        className="relative z-0 h-80 w-full rounded-lg border bg-muted"
      />
      {(!configured || error) && (
        <p role="status" className="text-sm text-muted-foreground">
          {error
            ? "No se pudo cargar la cartografía."
            : "Cartografía no configurada."}{" "}
          {props.editable
            ? "Puedes ingresar coordenadas manualmente."
            : "La cuadrícula y la dirección siguen disponibles."}
        </p>
      )}
    </div>
  );
}
