"use client";
import dynamic from "next/dynamic";
import { useMemo } from "react";
import type { PublicSpaceDetail } from "../domain/types";
import { SpaceNearbyPois } from "./space-nearby-pois";
const LocationMap = dynamic(
  () => import("@/components/maps/location-map").then((m) => m.LocationMap),
  { ssr: false, loading: () => <p role="status">Cargando mapa…</p> },
);
export function PublicSpaceLocation({ space }: { space: PublicSpaceDetail }) {
  const location = space.location;
  const center = useMemo<[number, number]>(
    () => [location?.latitude ?? -33.45, location?.longitude ?? -70.66],
    [location],
  );
  const markers = useMemo(
    () =>
      location
        ? [
            {
              id: space.space_id,
              latitude: location.latitude,
              longitude: location.longitude,
              label: space.title,
              approximate: location.visibility === "approximate",
            },
          ]
        : [],
    [location, space.space_id, space.title],
  );
  if (!location) return null;
  return (
    <section
      id="ubicacion"
      aria-labelledby="public-space-location-heading"
      className="scroll-mt-24 space-y-4"
    >
      <h2 id="public-space-location-heading" className="text-xl font-semibold">
        Ubicación
      </h2>
      <p>{location.address}</p>
      {location.visibility === "approximate" && (
        <p className="text-sm text-muted-foreground">
          Ubicación aproximada (~250 m). La dirección exacta se comparte al
          confirmar la reserva.
        </p>
      )}
      <LocationMap center={center} markers={markers} />
      <SpaceNearbyPois spaceId={space.space_id} />
    </section>
  );
}
