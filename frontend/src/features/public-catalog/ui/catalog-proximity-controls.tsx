"use client";

import { MapPin as MapPinIcon } from "lucide-react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import type { MapPin } from "@/components/maps/location-map";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { getCatalogCommuneCenter } from "../domain/commune-centers";
import {
  serializeCatalogQuery,
  withoutCatalogArea,
} from "../domain/query-state";
import type { CatalogQuery, PublicLocation } from "../domain/types";

const LocationMap = dynamic(
  () =>
    import("@/components/maps/location-map").then(
      (module) => module.LocationMap,
    ),
  { ssr: false, loading: () => <p role="status">Cargando mapa…</p> },
);

const RADII = [2, 5, 10, 25, 50] as const;
const DEFAULT_CENTER = { latitude: -33.45, longitude: -70.66 };

type Center = { latitude: number; longitude: number };

function approximate(value: number) {
  return Number(value.toFixed(2));
}

function nearbyCommune(
  center: Center,
  locations: readonly PublicLocation[],
): PublicLocation | undefined {
  let nearest: PublicLocation | undefined;
  let nearestDistance = Number.POSITIVE_INFINITY;
  for (const location of locations) {
    const reference = getCatalogCommuneCenter(location.comuna_code);
    if (!reference) continue;
    const latitudeDelta = (reference.latitude - center.latitude) * 111;
    const longitudeDelta =
      (reference.longitude - center.longitude) *
      111 *
      Math.cos((center.latitude * Math.PI) / 180);
    const distance = Math.hypot(latitudeDelta, longitudeDelta);
    if (distance < nearestDistance) {
      nearest = location;
      nearestDistance = distance;
    }
  }
  return nearestDistance <= 0.05 ? nearest : undefined;
}

export function CatalogProximityControls({
  query,
  locations,
  actionPath,
}: {
  query: CatalogQuery;
  locations: readonly PublicLocation[];
  actionPath: string;
}) {
  const router = useRouter();
  const [center, setCenter] = useState<Center>(() => {
    if (query.latitude !== undefined && query.longitude !== undefined) {
      return {
        latitude: Number(query.latitude),
        longitude: Number(query.longitude),
      };
    }
    return DEFAULT_CENTER;
  });
  const [communeCode, setCommuneCode] = useState(() => {
    if (query.comuna_code) return query.comuna_code;
    if (query.latitude === undefined || query.longitude === undefined)
      return "";
    return (
      nearbyCommune(
        {
          latitude: Number(query.latitude),
          longitude: Number(query.longitude),
        },
        locations,
      )?.comuna_code ?? ""
    );
  });
  const [regionCode, setRegionCode] = useState(query.region_code ?? "");
  const [mode, setMode] = useState<"admin" | "radius">(
    query.areaMode === "admin" ? "admin" : "radius",
  );
  const [communeSearch, setCommuneSearch] = useState("");
  const [suggestions, setSuggestions] = useState<
    Array<{
      kind: "comuna" | "place";
      label: string;
      subtitle?: string;
      region_code?: string;
      comuna_code?: string;
      latitude?: number;
      longitude?: number;
    }>
  >([]);
  useEffect(() => {
    const trimmed = communeSearch.trim();
    if (trimmed.length < 2) return;
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      try {
        const res = await fetch(
          `/api/backend/public/destinations/suggest?q=${encodeURIComponent(trimmed)}&limit=8`,
          {
            headers: { Accept: "application/json" },
            signal: controller.signal,
          },
        );
        if (!res.ok) return;
        const data = (await res.json()) as { items?: typeof suggestions };
        if (Array.isArray(data.items)) setSuggestions(data.items);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
      }
    }, 250);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [communeSearch]);
  const [radius, setRadius] = useState(query.radius_km ?? "10");
  const [locating, setLocating] = useState(false);
  const [message, setMessage] = useState("");
  const [open, setOpen] = useState(false);
  const [mapPickerOpen, setMapPickerOpen] = useState(false);
  const [hasChosenCenter, setHasChosenCenter] = useState(
    query.latitude !== undefined,
  );

  const communes = useMemo(
    () =>
      [...locations].sort((a, b) =>
        `${a.comuna_name} ${a.region_name}`.localeCompare(
          `${b.comuna_name} ${b.region_name}`,
          "es",
        ),
      ),
    [locations],
  );

  const regions = [
    ...new Map(
      communes.map((location) => [location.region_code, location.region_name]),
    ).entries(),
  ].sort((a, b) => a[1].localeCompare(b[1], "es"));
  const administrativeCommune = communes.find(
    (location) => location.comuna_code === communeCode,
  );

  const normalized = (value: string) =>
    value
      .normalize("NFD")
      .replace(/\p{Diacritic}/gu, "")
      .toLowerCase();
  const communeText = normalized(communeSearch.trim());
  const matchingCommunes = communes.filter(
    (location) =>
      (mode === "radius" ||
        !regionCode ||
        location.region_code === regionCode) &&
      (location.comuna_code === communeCode ||
        normalized(`${location.comuna_name} ${location.region_name}`).includes(
          communeText,
        )),
  );

  const marker: MapPin[] = hasChosenCenter
    ? [
        {
          id: "search-center",
          latitude: center.latitude,
          longitude: center.longitude,
          label: "Centro aproximado de búsqueda",
        },
      ]
    : [];

  function navigate(next: CatalogQuery) {
    const serialized = serializeCatalogQuery({ ...next, cursor: undefined });
    router.push(`${actionPath}${serialized.size ? `?${serialized}` : ""}`, {
      scroll: false,
    });
  }

  function apply(nextCenter: Center) {
    setCenter(nextCenter);
    setHasChosenCenter(true);
    setMessage(
      communeCode
        ? `Búsqueda cerca de ${communes.find((item) => item.comuna_code === communeCode)?.comuna_name ?? "la comuna"}.`
        : "Búsqueda desde el punto seleccionado.",
    );
    navigate({
      ...withoutCatalogArea(query),
      areaMode: "radius",
      latitude: nextCenter.latitude.toFixed(5),
      longitude: nextCenter.longitude.toFixed(5),
      radius_km: radius,
      sort:
        query.sort && query.sort !== "default" ? query.sort : "distance_asc",
    });
    setOpen(false);
  }

  function useMyLocation() {
    setMessage("");
    if (!window.isSecureContext || !navigator.geolocation) {
      setMessage(
        "No pudimos acceder a la ubicación. Elige una comuna o marca un punto en el mapa.",
      );
      return;
    }
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (position) => {
        setLocating(false);
        if (position.coords.accuracy > 5000) {
          setMessage(
            "La ubicación es demasiado imprecisa. Elige una comuna o marca un punto en el mapa.",
          );
          return;
        }
        const nextCenter = {
          latitude: approximate(position.coords.latitude),
          longitude: approximate(position.coords.longitude),
        };
        setCenter(nextCenter);
        setHasChosenCenter(true);
        setCommuneCode("");
        navigate({
          ...withoutCatalogArea(query),
          areaMode: "radius",
          latitude: nextCenter.latitude.toFixed(2),
          longitude: nextCenter.longitude.toFixed(2),
          radius_km: radius,
          sort:
            query.sort && query.sort !== "default"
              ? query.sort
              : "distance_asc",
        });
        setMessage(
          "Ubicación aproximada · coordenadas redondeadas a 2 decimales.",
        );
        setOpen(false);
      },
      () => {
        setLocating(false);
        setMessage(
          "No pudimos usar tu ubicación. Elige una comuna o marca un punto en el mapa.",
        );
      },
      { timeout: 10000, maximumAge: 60000 },
    );
  }

  function applyAdministrativeArea() {
    navigate({
      ...withoutCatalogArea(query),
      areaMode: "admin",
      region_code:
        administrativeCommune?.region_code ?? (regionCode || undefined),
      comuna_code: administrativeCommune?.comuna_code,
    });
    setOpen(false);
  }

  function clear() {
    navigate(withoutCatalogArea(query));
    setOpen(false);
  }

  const active = Boolean(query.areaMode);
  const mapCenter: [number, number] = [center.latitude, center.longitude];
  const searchLabel =
    query.latitude !== undefined && query.longitude !== undefined
      ? (nearbyCommune(
          {
            latitude: Number(query.latitude),
            longitude: Number(query.longitude),
          },
          locations,
        )?.comuna_name ?? "Ubicación aproximada")
      : "";
  const areaName =
    query.areaMode === "admin"
      ? (administrativeCommune?.comuna_name ??
        regions.find(([code]) => code === query.region_code)?.[1] ??
        "Ubicación seleccionada")
      : null;
  const triggerLabel = query.bbox
    ? "Zona del mapa"
    : (areaName ??
      (query.areaMode === "radius"
        ? `${searchLabel} · ${query.radius_km} km`
        : "Dónde"));
  const triggerAccessibleLabel =
    query.areaMode === "radius"
      ? `Buscar por cercanía: ${searchLabel}, radio ${query.radius_km} kilómetros`
      : `Ubicación y distancia${active ? `: ${triggerLabel}` : ""}`;

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger
        aria-label={triggerAccessibleLabel}
        className="inline-flex min-h-11 items-center rounded-full border border-border px-4 text-sm font-medium hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <MapPinIcon className="mr-1.5 size-4 shrink-0" aria-hidden="true" />
        <span className="max-w-32 truncate sm:max-w-60">{triggerLabel}</span>
      </SheetTrigger>
      <SheetContent
        side="bottom"
        className="max-h-[85vh] overflow-y-auto rounded-t-2xl p-5"
      >
        <SheetHeader className="mb-4 p-0 text-left">
          <SheetTitle className="text-xl font-medium">Dónde buscas</SheetTitle>
          <SheetDescription>
            Comparte tu ubicación solo si quieres. También puedes elegir una
            comuna o marcar un punto.
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              aria-pressed={mode === "admin"}
              onClick={() => setMode("admin")}
              className="min-h-11 rounded-full border px-4 text-sm aria-pressed:border-primary aria-pressed:bg-primary/10"
            >
              Por comuna o región
            </button>
            <button
              type="button"
              aria-pressed={mode === "radius"}
              onClick={() => {
                setMode("radius");
                const reference = getCatalogCommuneCenter(communeCode);
                if (reference) {
                  setCenter(reference);
                  setHasChosenCenter(true);
                }
              }}
              className="min-h-11 rounded-full border px-4 text-sm aria-pressed:border-primary aria-pressed:bg-primary/10"
            >
              Por cercanía
            </button>
            {active && (
              <button
                type="button"
                onClick={clear}
                className="min-h-11 rounded-full px-4 text-sm underline"
              >
                Quitar ubicación
              </button>
            )}
          </div>

          {mode === "admin" && (
            <label className="block space-y-1 text-sm">
              <span className="font-medium">Región</span>
              <select
                value={regionCode}
                onChange={(event) => {
                  setRegionCode(event.target.value);
                  setCommuneCode("");
                  setCommuneSearch("");
                }}
                className="h-11 w-full rounded-lg border border-border bg-background px-3"
              >
                <option value="">Todas las regiones</option>
                {regions.map(([code, name]) => (
                  <option key={code} value={code}>
                    {name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="block space-y-1 text-sm">
            <span className="font-medium">Buscar comuna, barrio o calle</span>
            <input
              type="search"
              value={communeSearch}
              onChange={(event) => {
                const value = event.target.value;
                setCommuneSearch(value);
                if (value.trim().length < 2) setSuggestions([]);
              }}
              placeholder="Escribe una comuna, barrio o hito"
              className="h-11 w-full rounded-lg border border-border bg-background px-3"
            />
          </label>
          {suggestions.length > 0 && (
            <ul
              aria-label="Sugerencias de destino"
              className="max-h-44 space-y-1 overflow-y-auto rounded-lg border bg-muted/30 p-2 text-sm"
            >
              {suggestions.map((item, idx) => (
                <li key={`${item.kind}-${item.label}-${idx}`}>
                  <button
                    type="button"
                    onClick={() => {
                      if (item.kind === "comuna" && item.comuna_code) {
                        setRegionCode(item.region_code ?? "");
                        setCommuneCode(item.comuna_code);
                        const ref = getCatalogCommuneCenter(item.comuna_code);
                        if (ref) {
                          setCenter(ref);
                          setHasChosenCenter(true);
                        }
                        setCommuneSearch(item.label);
                        setSuggestions([]);
                      } else if (
                        item.kind === "place" &&
                        item.latitude !== undefined &&
                        item.longitude !== undefined
                      ) {
                        setMode("radius");
                        setCenter({
                          latitude: item.latitude,
                          longitude: item.longitude,
                        });
                        setHasChosenCenter(true);
                        setCommuneCode("");
                        setCommuneSearch(item.label);
                        setSuggestions([]);
                      }
                    }}
                    className="flex w-full items-center justify-between rounded-md px-2.5 py-2 text-left hover:bg-muted"
                  >
                    <span>
                      <strong className="font-medium">{item.label}</strong>
                      {item.subtitle ? (
                        <span className="ml-1.5 text-xs text-muted-foreground">
                          {item.subtitle}
                        </span>
                      ) : null}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {item.kind === "comuna" ? "Comuna" : "Lugar"}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block space-y-1 text-sm">
              <span className="font-medium">
                {mode === "admin" ? "Comuna" : "Comuna de referencia"}
              </span>
              <select
                value={communeCode}
                onChange={(event) => {
                  const nextCode = event.target.value;
                  setCommuneCode(nextCode);
                  if (mode === "admin") {
                    setHasChosenCenter(false);
                    const selected = communes.find(
                      ({ comuna_code }) => comuna_code === nextCode,
                    );
                    if (selected) setRegionCode(selected.region_code);
                    return;
                  }
                  if (!nextCode) {
                    setHasChosenCenter(false);
                    return;
                  }
                  const resolved = getCatalogCommuneCenter(nextCode);
                  if (!resolved) {
                    setMessage(
                      "No encontramos un centro aproximado para esa comuna. Puedes marcar un punto en el mapa.",
                    );
                    setHasChosenCenter(false);
                    return;
                  }
                  setCenter(resolved);
                  setHasChosenCenter(true);
                  setMessage(
                    "Centro aproximado de la comuna. Puedes ajustar el punto en el mapa.",
                  );
                }}
                className="h-11 w-full rounded-lg border border-border bg-background px-3"
              >
                <option value="">Elige una comuna</option>
                {matchingCommunes.map((location) => (
                  <option
                    key={location.comuna_code}
                    value={location.comuna_code}
                  >
                    {location.comuna_name} · {location.region_name}
                  </option>
                ))}
              </select>
            </label>
            {mode === "radius" && (
              <label className="block space-y-1 text-sm">
                <span className="font-medium">Radio de búsqueda</span>
                <select
                  value={radius}
                  onChange={(event) => setRadius(event.target.value)}
                  className="h-11 w-full rounded-lg border border-border bg-background px-3"
                >
                  {!RADII.some((value) => String(value) === radius) && (
                    <option value={radius}>{radius} km</option>
                  )}
                  {RADII.map((value) => (
                    <option key={value} value={value}>
                      {value} km{value === 10 ? " · recomendado" : ""}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </div>

          {mode === "admin" ? (
            <button
              type="button"
              onClick={applyAdministrativeArea}
              disabled={!administrativeCommune && !regionCode}
              className="min-h-11 rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {administrativeCommune
                ? "Buscar en esta comuna"
                : "Buscar en esta región"}
            </button>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={useMyLocation}
                  disabled={locating}
                  className="min-h-11 rounded-full border px-4 text-sm disabled:opacity-60"
                >
                  {locating ? "Buscando ubicación…" : "Usar mi ubicación"}
                </button>
                <button
                  type="button"
                  onClick={() => apply(center)}
                  disabled={!hasChosenCenter}
                  className="min-h-11 rounded-full bg-foreground px-4 text-sm font-medium text-background disabled:opacity-50"
                >
                  Aplicar cercanía
                </button>
              </div>

              <div className="space-y-2">
                <button
                  type="button"
                  onClick={() => setMapPickerOpen((value) => !value)}
                  aria-expanded={mapPickerOpen}
                  className="min-h-11 rounded-full border px-4 text-sm"
                >
                  {mapPickerOpen ? "Ocultar mapa" : "Elegir en el mapa"}
                </button>
                {mapPickerOpen ? (
                  <LocationMap
                    center={mapCenter}
                    zoom={communeCode ? 10 : 11}
                    markers={marker}
                    editable
                    onMove={(latitude, longitude) => {
                      setCenter({ latitude, longitude });
                      setCommuneCode("");
                      setHasChosenCenter(true);
                      setMessage(
                        "Punto seleccionado en el mapa. Aplica para buscar en esta zona.",
                      );
                    }}
                  />
                ) : null}
              </div>

              {message ? (
                <p role="status" className="text-sm text-muted-foreground">
                  {message}
                </p>
              ) : null}
              <p className="text-xs text-muted-foreground">
                La ubicación del dispositivo no se comparte hasta que pulses
                “Usar mi ubicación”. Las coordenadas que se aplican se redondean
                a una zona aproximada.
              </p>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
