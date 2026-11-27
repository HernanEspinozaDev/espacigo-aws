"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { apiClient } from "@/lib/api/client";
import type { operations } from "@/lib/api/generated/openapi";

type AcceptanceItems =
  operations["listOwnLegalAcceptances"]["responses"][200]["content"]["application/json"]["items"];

function versionLabel(code: string) {
  const version = code.match(/-v(\d+(?:-\d+)*)$/)?.[1];
  return version ? `v${version.replaceAll("-", ".")}` : code;
}

export function LegalAcceptancesCard({ accountId }: { accountId: string }) {
  return <AccountLegalAcceptances key={accountId} />;
}

async function readAcceptances(signal: AbortSignal) {
  const { response, data } = await apiClient.GET("/privacy/legal-acceptances", {
    signal,
  });
  if (!response.ok || !data)
    throw new Error("No pudimos consultar tus confirmaciones.");
  return data.items;
}

function AccountLegalAcceptances() {
  const [items, setItems] = useState<AcceptanceItems | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<string | null>(null);
  const lifetime = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    lifetime.current = controller;
    readAcceptances(controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) setItems(items);
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setError("No pudimos consultar tus confirmaciones.");
      });
    return () => controller.abort();
  }, []);

  async function confirmDocument(code: string) {
    const signal = lifetime.current?.signal;
    if (!signal || signal.aborted || pending) return;
    setPending(code);
    setError("");
    try {
      const { response } = await apiClient.PUT("/privacy/legal-acceptances", {
        body: { code },
        signal,
      });
      if (response.status === 409) {
        const versions = await readAcceptances(signal);
        if (!signal.aborted) {
          setItems(versions);
          setError(
            "El documento cambió. Revisa la versión vigente antes de confirmar.",
          );
        }
        return;
      }
      if (!response.ok) throw new Error("No pudimos guardar la confirmación.");
      const versions = await readAcceptances(signal);
      if (!signal.aborted) setItems(versions);
    } catch {
      if (!signal.aborted)
        setError("No pudimos confirmar el estado. Vuelve a intentarlo.");
    } finally {
      if (!signal.aborted) setPending(null);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tus documentos legales</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {items ? (
          <ul className="divide-y text-sm">
            {items.map((item) => {
              const terms = item.type === "terminos";
              return (
                <li key={item.type} className="space-y-2 py-3 first:pt-0">
                  <Link
                    className="font-medium text-primary underline underline-offset-2"
                    href={
                      terms
                        ? "/terminos-y-condiciones"
                        : "/politica-de-privacidad"
                    }
                    target="_blank"
                    rel="noopener"
                  >
                    {terms
                      ? "Leer términos y condiciones"
                      : "Leer política de privacidad"}
                  </Link>
                  <p className="text-muted-foreground">
                    Versión vigente {versionLabel(item.current_code)}
                  </p>
                  {item.current_accepted ? (
                    <p role="status">
                      {terms
                        ? "Versión vigente aceptada"
                        : "Lectura de la versión vigente confirmada"}
                    </p>
                  ) : (
                    <>
                      {item.latest_accepted_code && (
                        <p className="text-muted-foreground">
                          {terms
                            ? "Última versión aceptada"
                            : "Última versión registrada"}{" "}
                          {versionLabel(item.latest_accepted_code)}
                        </p>
                      )}
                      <Button
                        type="button"
                        size="sm"
                        disabled={pending !== null}
                        onClick={() => void confirmDocument(item.current_code)}
                      >
                        {pending === item.current_code
                          ? "Guardando…"
                          : terms
                            ? "Aceptar términos vigentes"
                            : "Confirmar lectura de la política"}
                      </Button>
                    </>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          !error && (
            <p role="status" className="text-sm text-muted-foreground">
              Consultando tus confirmaciones…
            </p>
          )
        )}
      </CardContent>
    </Card>
  );
}
