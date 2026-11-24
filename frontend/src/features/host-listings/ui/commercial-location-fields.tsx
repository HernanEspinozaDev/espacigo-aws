"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { LocationMap, type MapBounds } from "@/components/maps/location-map";
import {
  getCommercialCommuneViewport,
  searchCommercialAddress,
} from "../commercial-location-runtime";
import {
  validPin,
  type LocationInput,
  type PinResolutionStatus,
} from "../domain/commercial-location";
import { CHILE_COMUNAS, CHILE_REGIONS } from "../domain/chile-dpa";

type SelectPin = (
  latitude: number,
  longitude: number,
  origin?: {
    source?: "manual" | "geocoded";
    precision?: "address" | "approximate";
    addressSuggestion?: string;
  },
) => void;

export function CommercialLocationFields({
  location,
  onLocationChange,
  onSelectPin,
  resolution = "idle",
  idPrefix,
  addressErrorId,
  addressInvalid = false,
  onRestoreAddress,
  disabled = false,
}: {
  location: LocationInput;
  onLocationChange: (location: LocationInput) => void;
  onSelectPin: SelectPin;
  resolution?: PinResolutionStatus;
  idPrefix?: string;
  addressErrorId?: string;
  addressInvalid?: boolean;
  onRestoreAddress?: () => void;
  disabled?: boolean;
}) {
  const id = useId();
  const fieldId = idPrefix ?? id;
  const [message, setMessage] = useState<string | null>(null);
  const [searchText, setSearchText] = useState("");
  const [searching, setSearching] = useState(false);
  const [locating, setLocating] = useState(false);
  const [communeViewport, setCommuneViewport] = useState<{
    code: string;
    bounds: MapBounds;
  } | null>(null);
  const [candidates, setCandidates] = useState<
    Awaited<ReturnType<typeof searchCommercialAddress>>
  >([]);
  const searchController = useRef<AbortController | null>(null);
  const selectionGeneration = useRef(0);
  const valid = validPin(location.latitude, location.longitude);
  const regionName = CHILE_REGIONS.find(
    (region) => region.code === location.region_code,
  )?.name;
  const comunaName = CHILE_COMUNAS.find(
    (comuna) =>
      comuna.code === location.comuna_code &&
      comuna.regionCode === location.region_code,
  )?.name;
  const center = useMemo<[number, number]>(
    () =>
      valid ? [location.latitude!, location.longitude!] : [-33.45, -70.66],
    [valid, location.latitude, location.longitude],
  );
  const pins = useMemo(
    () =>
      valid
        ? [
            {
              id: "space",
              latitude: location.latitude!,
              longitude: location.longitude!,
              label: location.address || "Ubicación del espacio",
            },
          ]
        : [],
    [valid, location.latitude, location.longitude, location.address],
  );

  useEffect(() => {
    const controller = new AbortController();
    if (!valid && location.comuna_code) {
      void getCommercialCommuneViewport(location.comuna_code, controller.signal)
        .then((bounds) => {
          if (!controller.signal.aborted)
            setCommuneViewport(
              bounds ? { code: location.comuna_code, bounds } : null,
            );
        })
        .catch(() => {
          if (!controller.signal.aborted) setCommuneViewport(null);
        });
    }
    return () => controller.abort();
  }, [valid, location.comuna_code]);

  useEffect(
    () => () => {
      selectionGeneration.current += 1;
      searchController.current?.abort();
    },
    [],
  );

  async function searchAddress() {
    const query = searchText.trim();
    if (!query) {
      setMessage("Escribe una dirección o lugar para buscar.");
      return;
    }
    searchController.current?.abort();
    const controller = new AbortController();
    searchController.current = controller;
    setSearching(true);
    setCandidates([]);
    setMessage(null);
    try {
      const items = await searchCommercialAddress(
        { address: query },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      setCandidates(items);
      if (!items.length)
        setMessage(
          "No encontramos ese lugar. Puedes elegir el punto directamente en el mapa.",
        );
    } catch {
      if (!controller.signal.aborted)
        setMessage(
          "No se pudo buscar. Puedes elegir el punto directamente en el mapa.",
        );
    } finally {
      if (!controller.signal.aborted) setSearching(false);
    }
  }

  function selectCandidate(candidate: (typeof candidates)[number]) {
    selectPin(candidate.latitude, candidate.longitude, {
      source: "geocoded",
      precision: candidate.precision,
      addressSuggestion: candidate.label,
    });
    setMessage(
      candidate.precision === "approximate"
        ? "Punto aproximado seleccionado; revisa el marcador."
        : "Punto seleccionado; revisa el marcador.",
    );
  }

  function selectPin(
    latitude: number,
    longitude: number,
    origin?: Parameters<SelectPin>[2],
  ) {
    selectionGeneration.current += 1;
    searchController.current?.abort();
    setCandidates([]);
    setLocating(false);
    onSelectPin(latitude, longitude, origin);
  }

  function selectMapPin(latitude: number, longitude: number) {
    selectPin(latitude, longitude);
  }

  function requestCurrentPosition() {
    if (!navigator.geolocation) {
      setMessage(
        "Este navegador no permite obtener la ubicación. Selecciona el punto en el mapa.",
      );
      return;
    }
    setLocating(true);
    setMessage(null);
    const requestGeneration = ++selectionGeneration.current;
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        if (requestGeneration !== selectionGeneration.current) return;
        setLocating(false);
        selectMapPin(coords.latitude, coords.longitude);
      },
      () => {
        if (requestGeneration !== selectionGeneration.current) return;
        setLocating(false);
        setMessage(
          "No pudimos obtener tu ubicación. Tu punto actual se conserva; puedes elegirlo en el mapa.",
        );
      },
      { enableHighAccuracy: true, timeout: 12_000, maximumAge: 0 },
    );
  }

  const resolutionText: Record<PinResolutionStatus, string> = {
    idle: "Selecciona un punto para detectar la región y comuna.",
    resolving: "Detectando comuna…",
    resolved: "Mueve el pin para cambiar la región o comuna.",
    invalid:
      "No pudimos resolver este punto en una zona válida en Chile. Mueve el pin.",
    unavailable:
      "No se pudo consultar la comuna. Inténtalo de nuevo o mueve el pin.",
  };

  return (
    <fieldset disabled={disabled} className="space-y-3 rounded-lg border p-4">
      <legend className="px-2 font-semibold">Ubicación en el mapa</legend>
      <p className="text-sm text-muted-foreground">
        Busca un lugar o selecciona el punto. La región y comuna se determinan
        desde el pin.
      </p>
      <div className="flex flex-col gap-2 sm:flex-row">
        <label htmlFor={`${fieldId}-search`} className="sr-only">
          Buscar dirección o lugar
        </label>
        <input
          id={`${fieldId}-search`}
          className="min-h-11 min-w-0 flex-1 rounded border px-3"
          value={searchText}
          onChange={(event) => setSearchText(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              void searchAddress();
            }
          }}
          placeholder="Dirección, comuna o lugar"
        />
        <button
          type="button"
          className="min-h-11 rounded-lg border px-4 text-sm font-medium disabled:opacity-50"
          disabled={searching || disabled}
          onClick={() => void searchAddress()}
        >
          {searching ? "Buscando…" : "Buscar"}
        </button>
        <button
          type="button"
          className="min-h-11 rounded-lg border px-4 text-sm font-medium disabled:opacity-50"
          disabled={locating || disabled}
          onClick={requestCurrentPosition}
        >
          {locating ? "Ubicando…" : "Usar mi ubicación"}
        </button>
      </div>
      <LocationMap
        center={center}
        viewport={
          valid || communeViewport?.code !== location.comuna_code
            ? undefined
            : communeViewport.bounds
        }
        markers={pins}
        editable={!disabled}
        onMove={selectMapPin}
      />
      <p role="status" className="text-sm">
        {resolutionText[resolution]}
      </p>
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      {onRestoreAddress && (
        <div className="flex flex-wrap items-center gap-x-3 text-sm">
          <span role="status">Dirección sugerida por el mapa.</span>
          <button
            type="button"
            className="min-h-11 underline"
            onClick={onRestoreAddress}
          >
            Deshacer cambio de dirección
          </button>
        </div>
      )}
      {candidates.length > 0 && (
        <ul className="space-y-1" aria-label="Lugares encontrados">
          {candidates.map((candidate, index) => (
            <li key={`${candidate.latitude}:${candidate.longitude}:${index}`}>
              <button
                type="button"
                className="block w-full rounded border p-2 text-left hover:bg-muted"
                onClick={() => selectCandidate(candidate)}
              >
                {candidate.label}
                {candidate.precision === "approximate" && (
                  <span className="ml-2 text-xs text-muted-foreground">
                    Aproximada
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="space-y-1">
        <label
          htmlFor={`${fieldId}-address`}
          className="block text-sm font-medium"
        >
          Dirección
        </label>
        <input
          id={`${fieldId}-address`}
          aria-invalid={addressInvalid || undefined}
          aria-describedby={[`${fieldId}-address-count`, addressErrorId]
            .filter(Boolean)
            .join(" ")}
          className="block min-h-11 w-full rounded border px-3 font-normal"
          value={location.address}
          onChange={(event) =>
            onLocationChange({
              ...location,
              address: event.target.value,
              public_confirmed: false,
            })
          }
        />
        <span
          id={`${fieldId}-address-count`}
          className="block text-right text-xs font-normal text-muted-foreground"
        >
          {Array.from(location.address).length}/500 caracteres
        </span>
      </div>
      <div className="grid gap-3 sm:grid-cols-2" aria-live="polite">
        <div className="space-y-1 text-sm">
          <label className="font-medium" htmlFor={`${fieldId}-region`}>
            Región
          </label>
          <output
            id={`${fieldId}-region`}
            className="block min-h-10 rounded border bg-muted/30 px-3 py-2"
          >
            {regionName ||
              (resolution === "resolving"
                ? "Detectando…"
                : "Pendiente de resolución")}
          </output>
        </div>
        <div className="space-y-1 text-sm">
          <label className="font-medium" htmlFor={`${fieldId}-comuna`}>
            Comuna
          </label>
          <output
            id={`${fieldId}-comuna`}
            className="block min-h-10 rounded border bg-muted/30 px-3 py-2"
          >
            {comunaName ||
              (resolution === "resolving"
                ? "Detectando…"
                : "Pendiente de resolución")}
          </output>
        </div>
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={location.public_confirmed}
          disabled={
            disabled ||
            resolution !== "resolved" ||
            !location.address.trim() ||
            !location.region_code ||
            !location.comuna_code ||
            !valid
          }
          onChange={(event) =>
            onLocationChange({
              ...location,
              public_confirmed: event.target.checked,
            })
          }
        />
        Confirmo que esta dirección y este pin identifican mi espacio y autorizo
        mostrarlos públicamente.
      </label>
      <p className="text-sm text-muted-foreground">
        La dirección y el pin confirmados serán públicos. No incluyas teléfonos,
        correos ni códigos de acceso.
      </p>
    </fieldset>
  );
}
