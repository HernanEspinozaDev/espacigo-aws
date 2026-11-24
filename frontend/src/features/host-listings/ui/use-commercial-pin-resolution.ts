"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/errors";
import { CHILE_COMUNAS, CHILE_REGIONS } from "../domain/chile-dpa";
import {
  shouldApplyPinAddressSuggestion,
  stripPostalCodeFromAddress,
  validPin,
  type LocationInput,
  type PinResolutionStatus,
} from "../domain/commercial-location";

type ReverseResult = {
  address?: string;
  region_code?: string;
  comuna_code?: string;
};

type PinOrigin = {
  source?: "manual" | "geocoded";
  precision?: "address" | "approximate";
  addressSuggestion?: string;
  verifyExisting?: boolean;
};

type HookOptions = {
  initial: LocationInput;
  resolvePin: (
    latitude: number,
    longitude: number,
    signal: AbortSignal,
  ) => Promise<ReverseResult>;
  onChange: (location: LocationInput) => void;
};

export function useCommercialPinResolution({
  initial,
  resolvePin,
  onChange,
}: HookOptions) {
  const [location, setLocation] = useState(initial);
  const [resolution, setResolution] = useState<PinResolutionStatus>("idle");
  const [confirmedRevision, setConfirmedRevision] = useState<number | null>(
    null,
  );
  const [resolutionRevision, setResolutionRevision] = useState(0);
  const [replacedAddress, setReplacedAddress] = useState<string | null>(null);
  const revision = useRef(0);
  const latest = useRef(location);
  const controller = useRef<AbortController | null>(null);
  const addressEditRevision = useRef(0);
  const addressAnchor = useRef(
    validPin(initial.latitude, initial.longitude)
      ? { latitude: initial.latitude!, longitude: initial.longitude! }
      : null,
  );
  const callbacks = useRef({ resolvePin, onChange });
  useEffect(() => {
    callbacks.current = { resolvePin, onChange };
  }, [resolvePin, onChange]);

  const publish = useCallback((next: LocationInput) => {
    latest.current = next;
    setLocation(next);
    callbacks.current.onChange(next);
  }, []);

  const selectPin = useCallback(
    (latitude: number, longitude: number, origin: PinOrigin = {}) => {
      controller.current?.abort();
      const request = new AbortController();
      controller.current = request;
      const selectedRevision = ++revision.current;
      setResolutionRevision(selectedRevision);
      setReplacedAddress(null);
      const addressRevisionAtStart = addressEditRevision.current;
      const previous = latest.current;
      const verifyExisting =
        origin.verifyExisting === true &&
        previous.latitude === latitude &&
        previous.longitude === longitude &&
        Boolean(previous.region_code && previous.comuna_code);
      if (!addressAnchor.current && previous.address.trim())
        addressAnchor.current = { latitude, longitude };
      const next: LocationInput = {
        ...previous,
        latitude,
        longitude,
        region_code: verifyExisting ? previous.region_code : "",
        comuna_code: verifyExisting ? previous.comuna_code : "",
        public_confirmed: verifyExisting && previous.public_confirmed,
        source: verifyExisting ? previous.source : (origin.source ?? "manual"),
        precision: verifyExisting ? previous.precision : origin.precision,
      };
      setConfirmedRevision(null);
      setResolution("resolving");
      publish(next);
      const invalidate = (status: "invalid" | "unavailable") => {
        publish({
          ...latest.current,
          region_code: "",
          comuna_code: "",
          public_confirmed: false,
        });
        setResolution(status);
      };

      if (!validPin(latitude, longitude)) {
        invalidate("invalid");
        return;
      }

      void callbacks.current
        .resolvePin(latitude, longitude, request.signal)
        .then((result) => {
          if (request.signal.aborted || selectedRevision !== revision.current)
            return;
          const region = CHILE_REGIONS.find(
            (item) => item.code === result.region_code,
          );
          const comuna = CHILE_COMUNAS.find(
            (item) => item.code === result.comuna_code,
          );
          if (!region || !comuna || comuna.regionCode !== region.code) {
            invalidate("invalid");
            return;
          }
          const current = latest.current;
          const withCodes: LocationInput = {
            ...current,
            region_code: region.code,
            comuna_code: comuna.code,
            public_confirmed:
              verifyExisting &&
              previous.region_code === region.code &&
              previous.comuna_code === comuna.code
                ? current.public_confirmed
                : false,
            source: verifyExisting
              ? previous.source
              : (origin.source ?? "manual"),
            precision: verifyExisting ? previous.precision : origin.precision,
          };
          const movedMeters = distanceMeters(
            addressAnchor.current?.latitude ?? null,
            addressAnchor.current?.longitude ?? null,
            latitude,
            longitude,
          );
          const addressWasEdited =
            addressEditRevision.current !== addressRevisionAtStart;
          const suggestedAddress =
            result.address?.trim() || origin.addressSuggestion?.trim();
          const cleanedSuggestedAddress = suggestedAddress
            ? stripPostalCodeFromAddress(suggestedAddress)
            : "";
          if (
            cleanedSuggestedAddress &&
            shouldApplyPinAddressSuggestion({
              distanceFromAddressAnchorMeters: movedMeters,
              editedDuringResolution: addressWasEdited,
              verifyingExisting: verifyExisting,
            })
          ) {
            if (current.address !== cleanedSuggestedAddress)
              setReplacedAddress(current.address);
            withCodes.address = cleanedSuggestedAddress;
            addressAnchor.current = { latitude, longitude };
          }
          publish(withCodes);
          setConfirmedRevision(
            withCodes.public_confirmed ? selectedRevision : null,
          );
          setResolution("resolved");
        })
        .catch((error: unknown) => {
          if (request.signal.aborted || selectedRevision !== revision.current)
            return;
          invalidate(
            error instanceof ApiError && error.status === 422
              ? "invalid"
              : "unavailable",
          );
        });
    },
    [publish],
  );

  const editAddress = useCallback(
    (address: string) => {
      setReplacedAddress(null);
      addressEditRevision.current += 1;
      addressAnchor.current = validPin(
        latest.current.latitude,
        latest.current.longitude,
      )
        ? {
            latitude: latest.current.latitude!,
            longitude: latest.current.longitude!,
          }
        : null;
      publish({ ...latest.current, address, public_confirmed: false });
      setConfirmedRevision(null);
    },
    [publish],
  );

  const updateLocation = useCallback(
    (next: LocationInput) => {
      if (
        next.latitude !== latest.current.latitude ||
        next.longitude !== latest.current.longitude
      ) {
        selectPin(next.latitude ?? NaN, next.longitude ?? NaN);
        return;
      }
      if (next.address !== latest.current.address) {
        editAddress(next.address);
        return;
      }
      publish(next);
      if (next.public_confirmed) setConfirmedRevision(revision.current);
      else setConfirmedRevision(null);
    },
    [editAddress, publish, selectPin],
  );

  useEffect(
    () => () => {
      revision.current += 1;
      controller.current?.abort();
    },
    [],
  );

  const isResolved =
    resolution === "resolved" &&
    validPin(location.latitude, location.longitude) &&
    Boolean(location.region_code && location.comuna_code);

  return {
    location,
    resolution,
    selectPin,
    restoredAddressAvailable: replacedAddress !== null,
    restoreAddress: () => {
      if (replacedAddress !== null) editAddress(replacedAddress);
    },
    onLocationChange: updateLocation,
    confirmedRevision,
    resolutionRevision,
    isResolved,
  };
}

function distanceMeters(
  fromLatitude: number | null,
  fromLongitude: number | null,
  toLatitude: number,
  toLongitude: number,
) {
  if (!validPin(fromLatitude, fromLongitude)) return Number.POSITIVE_INFINITY;
  const radians = (degrees: number) => (degrees * Math.PI) / 180;
  const dLat = radians(toLatitude - fromLatitude!);
  const dLon = radians(toLongitude - fromLongitude!);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(radians(fromLatitude!)) *
      Math.cos(radians(toLatitude)) *
      Math.sin(dLon / 2) ** 2;
  return 6_371_000 * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}
