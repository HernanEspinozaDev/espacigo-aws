"use client";

import { useEffect } from "react";
import {
  getCookiePreferences,
  visitedSpacesKey,
} from "@/features/cookie-preferences/preferences";

const STORAGE_KEY = "espacigo.explore.continuity.v1";
const VISITED_LIMIT = 50;

export type ExploreCamera = { center: [number, number]; zoom: number };
export type ExploreSession = {
  filters: string;
  scrollY?: number;
  mapScrollY?: number;
  sheetOpen?: boolean;
  sheetHeight?: "collapsed" | "half" | "expanded";
  areaPending?: boolean;
  selectedSpaceId?: string;
  camera?: ExploreCamera;
};

function readAll(): Record<string, ExploreSession> {
  try {
    const value: unknown = JSON.parse(
      sessionStorage.getItem(STORAGE_KEY) ?? "{}",
    );
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    return value as Record<string, ExploreSession>;
  } catch {
    return {};
  }
}

export function readExploreSession(filters: string): ExploreSession {
  const item = readAll()[filters];
  return item?.filters === filters ? item : { filters };
}

export function updateExploreSession(
  filters: string,
  update: Partial<Omit<ExploreSession, "filters">>,
) {
  try {
    const all = readAll();
    const current = readExploreSession(filters);
    delete all[filters];
    all[filters] = { ...current, ...update, filters };
    // Bound the cache while retaining most recently touched filter contexts.
    const entries = Object.entries(all).slice(-12);
    sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify(Object.fromEntries(entries)),
    );
  } catch {
    // Continuity is an enhancement; the URL remains the source of search filters.
  }
}

export function readVisitedSpaces(): string[] {
  if (getCookiePreferences() !== "visited-spaces") return [];
  try {
    const value: unknown = JSON.parse(
      sessionStorage.getItem(visitedSpacesKey) ?? "[]",
    );
    return Array.isArray(value)
      ? value
          .filter((id): id is string => typeof id === "string")
          .slice(-VISITED_LIMIT)
      : [];
  } catch {
    return [];
  }
}

function rememberVisitedSpace(spaceId: string) {
  if (getCookiePreferences() !== "visited-spaces") return;
  const visited = readVisitedSpaces().filter((id) => id !== spaceId);
  visited.push(spaceId);
  try {
    sessionStorage.setItem(
      visitedSpacesKey,
      JSON.stringify(visited.slice(-VISITED_LIMIT)),
    );
  } catch {
    // Browsers can disable session storage.
  }
}

/** Mount on explore routes to remember public detail visits without changing URLs. */
export function ExploreDetailVisitTracker({
  filters,
  isMap,
}: {
  filters: string;
  isMap: boolean;
}) {
  useEffect(() => {
    const remember = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) return;
      const link = target.closest<HTMLAnchorElement>("a[href]");
      if (!link) return;
      const url = new URL(link.href, window.location.href);
      if (url.origin !== window.location.origin) return;
      const path = url.pathname;
      const match = /^\/espacios\/([^/]+)$/.exec(path);
      if (!match?.[1]) return;
      let spaceId: string;
      try {
        spaceId = decodeURIComponent(match[1]);
      } catch {
        return;
      }
      updateExploreSession(filters, {
        [isMap ? "mapScrollY" : "scrollY"]: window.scrollY,
      });
      rememberVisitedSpace(spaceId);
    };
    document.addEventListener("click", remember, true);
    return () => document.removeEventListener("click", remember, true);
  }, [filters, isMap]);
  return null;
}
