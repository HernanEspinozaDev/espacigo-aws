"use client";

import { useEffect, useRef, useState } from "react";

type NearbyPoiItem = {
  name: string;
  category: "transit" | "parking";
  distance_meters: number;
  walking_minutes?: number;
  distance_mode: "route" | "straight_line";
};

export function SpaceNearbyPois({ spaceId }: { spaceId: string }) {
  const [status, setStatus] = useState<
    "idle" | "loading" | "loaded" | "error"
  >("idle");
  const [available, setAvailable] = useState(true);
  const [items, setItems] = useState<NearbyPoiItem[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = abortRef.current;
    return () => controller?.abort();
  }, [spaceId]);

  async function handleLoad() {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setStatus("loading");
    try {
      const res = await fetch(
        `/api/backend/public/spaces/${encodeURIComponent(spaceId)}/nearby-pois`,
        {
          headers: { Accept: "application/json" },
          signal: controller.signal,
        },
      );
      if (!res.ok) {
        setStatus("error");
        return;
      }
      const data = (await res.json()) as {
        available?: boolean;
        items?: NearbyPoiItem[];
      };
      setAvailable(Boolean(data.available));
      setItems(Array.isArray(data.items) ? data.items : []);
      setStatus("loaded");
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setStatus("error");
    }
  }

  return (
    <div className="mt-3 space-y-2">
      {status === "idle" && (
        <button
          type="button"
          onClick={handleLoad}
          className="inline-flex min-h-10 items-center rounded-full border px-3 text-xs font-medium hover:bg-muted"
        >
          Ver cercanía (Transporte y Estacionamiento)
        </button>
      )}
      {status === "loading" && (
        <p role="status" className="text-xs text-muted-foreground">
          Consultando transporte y estacionamiento…
        </p>
      )}
      {status === "error" && (
        <p role="status" className="text-xs text-destructive">
          No pudimos consultar la cercanía en este momento.
        </p>
      )}
      {status === "loaded" && (
        <div className="rounded-lg border bg-background p-3 text-xs">
          <p className="font-medium text-foreground">
            Transporte y estacionamiento cercanos
          </p>
          {!available || items.length === 0 ? (
            <p className="mt-1 text-muted-foreground">
              No hay información de cercanía disponible para este punto.
            </p>
          ) : (
            <ul className="mt-2 space-y-1.5">
              {items.map((item, index) => (
                <li
                  key={`${item.category}-${item.name}-${index}`}
                  className="flex items-center justify-between gap-2 text-muted-foreground"
                >
                  <span>
                    <strong className="font-medium text-foreground">
                      {item.category === "transit"
                        ? "Transporte"
                        : "Estacionamiento"}
                      :
                    </strong>{" "}
                    {item.name}
                  </span>
                  <span className="shrink-0 tabular-nums">
                    {item.distance_mode === "route" &&
                    item.walking_minutes !== undefined
                      ? `${item.walking_minutes} min a pie (${item.distance_meters} m)`
                      : `${item.distance_meters} m en línea recta`}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
