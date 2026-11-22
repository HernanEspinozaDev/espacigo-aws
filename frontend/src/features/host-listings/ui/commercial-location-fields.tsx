"use client";
import { useId, useMemo, useRef, useEffect, useState } from "react";
import { LocationMap } from "@/components/maps/location-map";
import { searchCommercialAddress } from "../commercial-location-runtime";
import { validPin } from "../domain/commercial-location";

type PinValue = {
  latitude: number | null;
  longitude: number | null;
  public_confirmed: boolean;
};
export function CommercialLocationFields({
  address,
  value,
  onChange,
  disabled = false,
}: {
  address: string;
  value: PinValue;
  onChange: (value: PinValue) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const [message, setMessage] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);
  const [candidates, setCandidates] = useState<
    Awaited<ReturnType<typeof searchCommercialAddress>>
  >([]);
  const generation = useRef(0);
  const search = useRef<AbortController | null>(null);
  useEffect(() => {
    const version = ++generation.current;
    search.current?.abort();
    queueMicrotask(() => {
      if (generation.current === version) {
        setSearching(false);
        setCandidates([]);
      }
    });
    return () => {
      generation.current = version + 1;
      search.current?.abort();
    };
  }, [address]);
  const center = useMemo<[number, number]>(
    () =>
      validPin(value.latitude, value.longitude)
        ? [value.latitude!, value.longitude!]
        : [-33.45, -70.66],
    [value.latitude, value.longitude],
  );
  const pins = useMemo(
    () =>
      validPin(value.latitude, value.longitude)
        ? [
            {
              id: "space",
              latitude: value.latitude!,
              longitude: value.longitude!,
              label: address,
            },
          ]
        : [],
    [value.latitude, value.longitude, address],
  );
  async function searchAddress() {
    if (!address.trim()) {
      setMessage("Escribe una dirección antes de buscar.");
      return;
    }
    const version = ++generation.current;
    search.current?.abort();
    const controller = new AbortController();
    search.current = controller;
    setSearching(true);
    setMessage(null);
    try {
      const items = await searchCommercialAddress(address, controller.signal);
      if (controller.signal.aborted || version !== generation.current) return;
      setCandidates(items);
      if (!items.length)
        setMessage("Sin resultados. Ajusta el pin o ingresa coordenadas.");
    } catch (error) {
      if (!controller.signal.aborted && version === generation.current)
        setMessage(
          error instanceof Error
            ? error.message
            : "No se pudo buscar la dirección.",
        );
    } finally {
      if (!controller.signal.aborted && version === generation.current)
        setSearching(false);
    }
  }
  return (
    <fieldset disabled={disabled} className="space-y-3 rounded-lg border p-4">
      <legend className="px-2 font-semibold">
        Ubicación pública confirmada
      </legend>
      <p className="text-sm text-muted-foreground">
        La dirección y el pin confirmados serán públicos. No incluyas teléfonos,
        correos ni códigos de acceso.
      </p>
      <button
        type="button"
        className="rounded border px-3 py-2 text-sm"
        disabled={searching || disabled}
        onClick={() => void searchAddress()}
      >
        {searching ? "Buscando…" : "Buscar dirección"}
      </button>
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      {candidates.length > 0 && (
        <ul>
          {candidates.map((candidate, index) => (
            <li key={index}>
              <button
                type="button"
                className="p-2 text-left underline"
                onClick={() => {
                  onChange({
                    latitude: candidate.latitude,
                    longitude: candidate.longitude,
                    public_confirmed: false,
                  });
                  setCandidates([]);
                }}
              >
                {candidate.label}
              </button>
            </li>
          ))}
        </ul>
      )}
      <LocationMap
        center={center}
        markers={pins}
        editable={!disabled}
        onMove={(latitude, longitude) =>
          onChange({ latitude, longitude, public_confirmed: false })
        }
      />
      <div className="grid gap-3 sm:grid-cols-2">
        {(["latitude", "longitude"] as const).map((key) => (
          <label
            key={key}
            htmlFor={`${id}-${key}`}
            className="space-y-1 text-sm"
          >
            {key === "latitude" ? "Latitud" : "Longitud"}
            <input
              id={`${id}-${key}`}
              type="number"
              min={key === "latitude" ? -90 : -180}
              max={key === "latitude" ? 90 : 180}
              step="any"
              className="block w-full rounded border p-2"
              value={value[key] ?? ""}
              onChange={(event) =>
                onChange({
                  ...value,
                  [key]:
                    event.target.value === ""
                      ? null
                      : Number(event.target.value),
                  public_confirmed: false,
                })
              }
            />
          </label>
        ))}
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={value.public_confirmed}
          disabled={disabled || !validPin(value.latitude, value.longitude)}
          onChange={(event) =>
            onChange({ ...value, public_confirmed: event.target.checked })
          }
        />
        Confirmo que esta dirección y este pin identifican mi espacio y autorizo
        mostrarlos públicamente.
      </label>
    </fieldset>
  );
}
