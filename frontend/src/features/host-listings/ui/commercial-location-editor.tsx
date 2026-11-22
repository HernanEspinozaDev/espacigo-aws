"use client";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import { CHILE_REGIONS, getComunasByRegion } from "../domain/chile-dpa";
import type {
  LocationRead,
  LocationInput,
} from "../domain/commercial-location";
import { commercialLocationService } from "../commercial-location-runtime";
import { CommercialLocationFields } from "./commercial-location-fields";

export function CommercialLocationEditor({
  spaceId,
  onSaved,
}: {
  spaceId: string;
  onSaved?: () => void;
}) {
  const { snapshot, getAccessToken } = useAuth();
  const [read, setRead] = useState<LocationRead | null>(null);
  const [input, setInput] = useState<LocationInput | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const version = ++generation.current;
    const controller = new AbortController();
    if (snapshot.status === "authenticated")
      void commercialLocationService
        .read(spaceId, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted && version === generation.current) {
            setRead(result);
            setInput(result.location);
            setMessage(null);
            setBusy(false);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted && version === generation.current)
            setMessage("No se pudo cargar la ubicación. Vuelve a intentar.");
        });
    return () => {
      generation.current = version + 1;
      controller.abort();
    };
  }, [
    spaceId,
    snapshot.accountId,
    snapshot.status,
    snapshot.expiresAt,
    reload,
  ]);
  async function save() {
    if (!input || !read || busy) return;
    const scope = getAccessToken();
    const version = generation.current;
    setBusy(true);
    setMessage(null);
    try {
      const saved = await commercialLocationService.save(
        spaceId,
        input,
        read.etag,
      );
      if (version !== generation.current || scope !== getAccessToken()) return;
      setRead(saved);
      setInput(saved.location);
      setMessage(
        saved.location.public_confirmed
          ? "Ubicación pública confirmada."
          : "Ubicación guardada sin confirmar.",
      );
      onSaved?.();
    } catch (error) {
      if (version !== generation.current || scope !== getAccessToken()) return;
      if (error instanceof ApiError && error.status === 412) {
        setRead(null);
        setMessage(
          "El espacio cambió en otra operación. Recarga la ubicación y revisa los datos antes de guardar.",
        );
      } else
        setMessage(
          error instanceof ApiError
            ? "No se pudo guardar. Revisa los datos y tus permisos."
            : error instanceof Error
              ? error.message
              : "No se pudo guardar la ubicación.",
        );
    } finally {
      if (version === generation.current) setBusy(false);
    }
  }
  function change(next: Partial<LocationInput>) {
    setInput((current) =>
      current ? { ...current, ...next, public_confirmed: false } : current,
    );
  }
  return (
    <section
      id="commercial-location"
      className="space-y-4 rounded-lg border bg-card p-5"
    >
      <h2 className="text-lg font-semibold">Dirección y mapa del espacio</h2>
      {message && <p role="status">{message}</p>}
      {!read && (
        <button
          type="button"
          className="rounded border p-2"
          onClick={() => {
            setBusy(false);
            setReload((value) => value + 1);
          }}
        >
          Recargar ubicación
        </button>
      )}
      {input && (
        <>
          <label className="block text-sm">
            Dirección
            <input
              value={input.address}
              disabled={busy}
              maxLength={500}
              onChange={(event) => change({ address: event.target.value })}
              className="mt-1 block w-full rounded border p-2"
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-2">
            <label>
              Región
              <select
                disabled={busy}
                value={input.region_code}
                onChange={(event) =>
                  change({ region_code: event.target.value, comuna_code: "" })
                }
                className="block w-full rounded border p-2"
              >
                <option value="">Selecciona región</option>
                {CHILE_REGIONS.map((region) => (
                  <option key={region.code} value={region.code}>
                    {region.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Comuna
              <select
                disabled={busy || !input.region_code}
                value={input.comuna_code}
                onChange={(event) =>
                  change({ comuna_code: event.target.value })
                }
                className="block w-full rounded border p-2"
              >
                <option value="">Selecciona comuna</option>
                {getComunasByRegion(input.region_code).map((commune) => (
                  <option key={commune.code} value={commune.code}>
                    {commune.name}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <CommercialLocationFields
            address={input.address}
            value={input}
            disabled={busy}
            onChange={(pin) => setInput({ ...input, ...pin })}
          />
          <button
            type="button"
            disabled={busy || !read}
            onClick={() => void save()}
            className="rounded bg-primary px-4 py-2 text-primary-foreground disabled:opacity-50"
          >
            {busy ? "Guardando…" : "Guardar ubicación"}
          </button>
        </>
      )}
    </section>
  );
}
