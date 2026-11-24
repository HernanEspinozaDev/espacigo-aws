"use client";

import { useEffect, useRef, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import type {
  LocationInput,
  LocationRead,
} from "../domain/commercial-location";
import {
  commercialLocationService,
  reverseGeocodeCommercialPin,
} from "../commercial-location-runtime";
import { CommercialLocationFields } from "./commercial-location-fields";
import { useCommercialPinResolution } from "./use-commercial-pin-resolution";

export function CommercialLocationEditor({
  spaceId,
  onSaved,
}: {
  spaceId: string;
  onSaved?: () => void;
}) {
  const { snapshot } = useAuth();
  const [loaded, setLoaded] = useState<{
    key: string;
    version: number;
    value: LocationRead;
  } | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  const loadKey = JSON.stringify([
    spaceId,
    snapshot.accountId,
    snapshot.status,
    snapshot.expiresAt,
    reload,
  ]);
  const current = loaded?.key === loadKey ? loaded : null;

  useEffect(() => {
    const version = ++generation.current;
    const controller = new AbortController();
    if (snapshot.status === "authenticated")
      void commercialLocationService
        .read(spaceId, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted && version === generation.current) {
            setLoaded({ key: loadKey, version, value: result });
            setMessage(null);
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
  }, [loadKey, spaceId, snapshot.status]);

  return (
    <section
      id="commercial-location"
      className="space-y-4 rounded-lg border bg-card p-5"
    >
      <h2 className="text-lg font-semibold">Dirección y mapa del espacio</h2>
      {message && <p role="status">{message}</p>}
      {!current ? (
        <button
          type="button"
          className="rounded border p-2"
          onClick={() => {
            setMessage(null);
            setReload((value) => value + 1);
          }}
        >
          Recargar ubicación
        </button>
      ) : (
        <LoadedLocationEditor
          key={`${loadKey}:${current.value.etag}`}
          spaceId={spaceId}
          initial={current.value}
          loadGeneration={current.version}
          onSaved={(next, version) => {
            if (version !== generation.current) return;
            setLoaded({ key: loadKey, version, value: next });
            onSaved?.();
          }}
          onError={(next, version) => {
            if (version === generation.current) setMessage(next);
          }}
        />
      )}
    </section>
  );
}

function LoadedLocationEditor({
  spaceId,
  initial,
  loadGeneration,
  onSaved,
  onError,
}: {
  spaceId: string;
  initial: LocationRead;
  loadGeneration: number;
  onSaved: (read: LocationRead, version: number) => void;
  onError: (message: string | null, version: number) => void;
}) {
  const { getAccessToken } = useAuth();
  const [busy, setBusy] = useState(false);
  const pin = useCommercialPinResolution({
    initial: initial.location,
    resolvePin: reverseGeocodeCommercialPin,
    onChange: () => {},
  });

  const { selectPin } = pin;
  useEffect(() => {
    const location = initial.location;
    if (location.latitude !== null && location.longitude !== null)
      selectPin(location.latitude, location.longitude, {
        source: location.source,
        precision: location.precision,
        verifyExisting: true,
      });
  }, [initial.location, selectPin]);

  async function save() {
    if (busy || !pin.isResolved) return;
    if (
      pin.location.public_confirmed &&
      pin.confirmedRevision !== pin.resolutionRevision
    ) {
      onError(
        "Vuelve a confirmar la dirección pública después de resolver el pin.",
        loadGeneration,
      );
      return;
    }
    const accessScope = getAccessToken();
    if (!accessScope) {
      onError(
        "Sesión no disponible. Inicia sesión nuevamente.",
        loadGeneration,
      );
      return;
    }
    setBusy(true);
    onError(null, loadGeneration);
    try {
      const saved = await commercialLocationService.save(
        spaceId,
        pin.location,
        initial.etag,
      );
      if (accessScope !== getAccessToken()) return;
      onSaved(saved, loadGeneration);
      onError(
        saved.location.public_confirmed
          ? "Ubicación pública confirmada."
          : "Ubicación guardada sin confirmar.",
        loadGeneration,
      );
    } catch (error) {
      if (accessScope !== getAccessToken()) return;
      if (error instanceof ApiError && error.status === 412) {
        onError(
          "El espacio cambió en otra operación. Recarga la ubicación y revisa los datos antes de guardar.",
          loadGeneration,
        );
      } else if (
        error instanceof ApiError &&
        error.fields.some(
          (field) =>
            (field.field === "latitude" ||
              field.field === "location.latitude") &&
            field.code === "outside_selected_commune",
        )
      ) {
        onError(
          "El pin confirmado queda fuera de la comuna seleccionada. Ajusta el pin y vuelve a guardar.",
          loadGeneration,
        );
      } else if (
        error instanceof ApiError &&
        (error.status === 503 ||
          error.fields.some((field) => field.code === "boundary_unavailable"))
      ) {
        onError(
          "No pudimos verificar el límite de esta comuna. Vuelve a resolver el pin antes de guardar.",
          loadGeneration,
        );
      } else {
        onError(
          error instanceof Error
            ? error.message
            : "No se pudo guardar la ubicación.",
          loadGeneration,
        );
      }
    } finally {
      if (accessScope === getAccessToken()) setBusy(false);
    }
  }

  const onLocationChange = (location: LocationInput) =>
    pin.onLocationChange(location);
  return (
    <>
      <CommercialLocationFields
        idPrefix="location"
        location={pin.location}
        onLocationChange={onLocationChange}
        onSelectPin={pin.selectPin}
        resolution={pin.resolution}
        onRestoreAddress={
          pin.restoredAddressAvailable ? pin.restoreAddress : undefined
        }
        disabled={busy}
      />
      <button
        type="button"
        disabled={busy || !pin.isResolved}
        onClick={() => void save()}
        className="rounded bg-primary px-4 py-2 text-primary-foreground disabled:opacity-50"
      >
        {busy ? "Guardando…" : "Guardar ubicación"}
      </button>
    </>
  );
}
