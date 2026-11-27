"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
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

export function LegalAcceptancesCard({ accountId }: { accountId: string }) {
  return <AccountLegalAcceptances key={accountId} />;
}

function AccountLegalAcceptances() {
  const [items, setItems] = useState<AcceptanceItems | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    apiClient
      .GET("/privacy/legal-acceptances", { signal: controller.signal })
      .then(({ response, data }) => {
        if (controller.signal.aborted) return;
        if (response.ok && data) setItems(data.items);
        else setError(true);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      });
    return () => controller.abort();
  }, []);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tus documentos legales</CardTitle>
      </CardHeader>
      <CardContent>
        {error ? (
          <p className="text-sm text-muted-foreground">
            No pudimos consultar tus aceptaciones.
          </p>
        ) : items ? (
          <ul className="space-y-3 text-sm">
            {items.map((item) => (
              <li
                key={item.type}
                className="flex flex-wrap items-baseline justify-between gap-2"
              >
                <div>
                  <Link
                    className="font-medium text-primary underline underline-offset-2"
                    href={
                      item.type === "terminos"
                        ? "/terminos-y-condiciones"
                        : "/politica-de-privacidad"
                    }
                  >
                    {item.type === "terminos"
                      ? "Términos y condiciones"
                      : "Política de privacidad"}
                  </Link>
                  <p className="text-muted-foreground">
                    Versión vigente: {item.current_code}
                  </p>
                  {item.latest_accepted_code && (
                    <p className="text-muted-foreground">
                      Última versión aceptada: {item.latest_accepted_code}
                    </p>
                  )}
                </div>
                <span>
                  {item.current_accepted
                    ? "Versión vigente aceptada"
                    : "Sin aceptación de la versión vigente"}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p role="status" className="text-sm text-muted-foreground">
            Consultando tus aceptaciones…
          </p>
        )}
      </CardContent>
    </Card>
  );
}
