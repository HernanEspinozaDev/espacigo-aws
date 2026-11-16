"use client";

import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { CalendarConfigurationController } from "../application/calendar-configuration-controller";
import { OpenApiCalendarConfigurationAdapter } from "../infrastructure/calendar-configuration-openapi-adapter";

const adapter = new OpenApiCalendarConfigurationAdapter();
const emptyState = Object.freeze({
  phase: "idle" as const,
  read: null,
  message: null,
});
const emptySnapshot = () => emptyState;
const emptySubscribe = () => () => undefined;

class ControllerLease {
  private count = 0;
  private generation = 0;
  constructor(
    private readonly controller: CalendarConfigurationController | null,
  ) {}
  retain() {
    this.count++;
    return ++this.generation;
  }
  active(generation: number) {
    return this.count > 0 && this.generation === generation;
  }
  release() {
    this.count--;
    queueMicrotask(() => {
      if (this.count === 0) this.controller?.dispose();
    });
  }
}

export function useHostCalendarConfiguration(spaceId: string | undefined) {
  const { snapshot, accessToken, getAccessToken } = useAuth();
  const identity = `${snapshot.status}:${snapshot.accountId ?? ""}:${snapshot.expiresAt ?? ""}:${accessToken ?? ""}`;
  const controller = useMemo(
    () =>
      spaceId && identity.startsWith("authenticated:")
        ? new CalendarConfigurationController(adapter, spaceId, getAccessToken)
        : null,
    [spaceId, identity, getAccessToken],
  );
  const lease = useMemo(() => new ControllerLease(controller), [controller]);
  useEffect(() => {
    if (!controller) return;
    const current = lease.retain();
    queueMicrotask(() => {
      if (lease.active(current)) void controller.refresh();
    });
    return () => {
      lease.release();
    };
  }, [controller, lease]);
  const state = useSyncExternalStore(
    controller?.subscribe ?? emptySubscribe,
    controller?.getSnapshot ?? emptySnapshot,
    controller?.getServerSnapshot ?? emptySnapshot,
  );
  return { controller, state };
}
