"use client";

import { useEffect, useRef, useState } from "react";

export function useAftercareRequest() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const running = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const last = useRef<{ intent: string; key: string } | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

  async function run(
    intent: string,
    perform: (key: string, signal: AbortSignal) => Promise<void>,
  ) {
    if (running.current) return;
    running.current = true;
    const request = new AbortController();
    controller.current = request;
    setBusy(true);
    setError("");
    if (last.current?.intent !== intent) {
      last.current = { intent, key: crypto.randomUUID() };
    }
    try {
      await perform(last.current.key, request.signal);
      last.current = null;
    } catch {
      if (!request.signal.aborted)
        setError("No se pudo completar la operación. Vuelve a intentarlo.");
    } finally {
      running.current = false;
      controller.current = null;
      if (!request.signal.aborted) setBusy(false);
    }
  }

  return { busy, error, run };
}
