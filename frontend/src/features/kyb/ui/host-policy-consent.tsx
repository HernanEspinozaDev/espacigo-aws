"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Button } from "@/components/workspace/ui/button";
import { apiClient } from "@/lib/api/client";
import type { components } from "@/lib/api/generated/openapi";

type PolicyConsent = components["schemas"]["KYBHostPolicyConsent"];

export function useHostPolicyConsent(accountId?: string | null) {
  const [result, setResult] = useState<{
    accountId: string;
    consent: PolicyConsent;
  } | null>(null);
  const consent =
    result?.accountId === accountId ? (result?.consent ?? null) : null;
  const [reload, setReload] = useState(0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!accountId) return;
    const controller = new AbortController();
    void apiClient
      .GET("/kyb/policy-consent", { signal: controller.signal })
      .then(({ data }) => {
        if (!controller.signal.aborted)
          setResult(data ? { accountId, consent: data } : null);
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult(null);
      });
    return () => controller.abort();
  }, [accountId, reload]);

  async function accept() {
    if (!accountId || !consent || saving) return;
    setSaving(true);
    setError(null);
    try {
      const { response, data } = await apiClient.POST("/kyb/policy-consent", {
        body: { version_id: consent.document.id },
      });
      if (response.status === 409) {
        setReload((value) => value + 1);
        setError("La política cambió. Lee la versión actual antes de aceptar.");
      } else if (data?.accepted) {
        setResult({ accountId, consent: data });
      } else {
        setError("No pudimos registrar la aceptación. Intenta nuevamente.");
      }
    } catch {
      setError("No pudimos registrar la aceptación. Intenta nuevamente.");
    } finally {
      setSaving(false);
    }
  }

  return { consent, saving, error, accept };
}

export function HostPolicyConsent({
  state,
}: {
  state: ReturnType<typeof useHostPolicyConsent>;
}) {
  if (!state.consent || state.consent.accepted) return null;
  return (
    <section className="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950">
      <p className="font-semibold">
        Actualización de la política para publicar
      </p>
      <p>
        Lee los términos vigentes y acepta la política de documentación
        comercial para publicar y administrar espacios.
      </p>
      <Link
        href="/terminos-y-condiciones"
        target="_blank"
        rel="noopener noreferrer"
        className="block underline"
      >
        Leer términos y condiciones
      </Link>
      {state.error ? <p role="alert">{state.error}</p> : null}
      <Button
        type="button"
        disabled={state.saving}
        onClick={() => void state.accept()}
      >
        {state.saving ? "Registrando aceptación…" : "Aceptar términos vigentes"}
      </Button>
    </section>
  );
}
