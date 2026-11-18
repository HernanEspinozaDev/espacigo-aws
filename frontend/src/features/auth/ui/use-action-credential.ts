"use client";

import { useLayoutEffect, useRef, useState } from "react";
import {
  createActionCredential,
  parseActionFragment,
  type ActionCredential,
} from "../domain/action-credential";

export function useActionCredential(
  onCapture?: (credential: ActionCredential | null) => void,
) {
  const [credential, setCredential] = useState<ActionCredential | null>(null);
  const [isInitializing, setIsInitializing] = useState(true);
  const credentialRef = useRef<ActionCredential | null>(null);
  const capturedRef = useRef<ActionCredential | null>(null);
  const mountedRef = useRef(false);
  const initialCaptureRef = useRef(false);
  const onCaptureRef = useRef(onCapture);

  useLayoutEffect(() => {
    onCaptureRef.current = onCapture;
  });

  useLayoutEffect(() => {
    mountedRef.current = true;

    const captureAndScrub = () => {
      let parsed = parseActionFragment(window.location.hash);
      if (!parsed.ok && window.location.search) {
        parsed = parseActionFragment("#" + window.location.search.slice(1));
      }
      window.history.replaceState(
        window.history.state,
        "",
        window.location.pathname,
      );
      const next = parsed.ok ? parsed.credential : null;
      capturedRef.current = next;
      credentialRef.current = next;
      setCredential(next);
      setIsInitializing(false);
      onCaptureRef.current?.(next);
    };

    if (!initialCaptureRef.current) {
      initialCaptureRef.current = true;
      captureAndScrub();
    } else {
      const captured = capturedRef.current;
      queueMicrotask(() => {
        if (mountedRef.current) {
          setCredential(captured);
          setIsInitializing(false);
        }
      });
    }

    window.addEventListener("hashchange", captureAndScrub);
    return () => {
      window.removeEventListener("hashchange", captureAndScrub);
      mountedRef.current = false;
      queueMicrotask(() => {
        if (!mountedRef.current) {
          credentialRef.current = null;
          capturedRef.current = null;
        }
      });
    };
  }, []);

  function setManual(tokenId: string, token: string): boolean {
    const parsed = createActionCredential(tokenId, token);
    if (!parsed.ok) return false;
    capturedRef.current = parsed.credential;
    credentialRef.current = parsed.credential;
    setCredential(parsed.credential);
    return true;
  }

  function clear(): void {
    capturedRef.current = null;
    credentialRef.current = null;
    setCredential(null);
  }

  return { credential, isInitializing, setManual, clear };
}
