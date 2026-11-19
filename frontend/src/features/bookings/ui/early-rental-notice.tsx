"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { earlyRentalAdapter as api } from "../infrastructure/early-rental-adapter";

export function EarlyRentalNotice() {
  const { snapshot } = useAuth();
  if (snapshot.status !== "authenticated") return null;
  return <SessionNotice key={snapshot.accountId + ":" + snapshot.expiresAt} />;
}

function SessionNotice() {
  const [notice, setNotice] = useState<{ id: string; title: string } | null>(
    null,
  );

  useEffect(() => {
    const request = new AbortController();
    let timer: number | undefined;
    let seen = new Map<string, string>();
    async function refresh() {
      try {
        const page = await api.list(request.signal);
        if (request.signal.aborted) return;
        const now = Date.now();
        const candidates = page.items.flatMap((item) => {
          if (item.state !== "confirmado" && Date.parse(item.expires_at) <= now)
            return [];
          const status =
            item.state === "pendiente_identidad" &&
            item.actor_role === "arrendador"
              ? "Solicitud de arriendo recibida"
              : item.state === "pendiente_contrato"
                ? "Contrato de prueba disponible o actualizado"
                : item.state === "pendiente_envio_pago" &&
                    item.actor_role === "arrendatario"
                  ? "Contrato firmado. Pago disponible"
                  : item.state === "confirmado"
                    ? "Pago de prueba aprobado"
                    : undefined;
          return status
            ? [
                {
                  id: item.id,
                  revision: String(item.revision),
                  title: status + " · " + item.space_title,
                },
              ]
            : [];
        });
        const latestChange = candidates.find(
          (item) => seen.get(item.id) !== item.revision,
        );
        seen = new Map(candidates.map((item) => [item.id, item.revision]));
        if (latestChange) setNotice(latestChange);
        else
          setNotice((current) =>
            current && seen.has(current.id) ? current : null,
          );
      } catch {
        if (!request.signal.aborted) setNotice(null);
      } finally {
        if (!request.signal.aborted)
          timer = window.setTimeout(() => void refresh(), 8000);
      }
    }
    void api
      .enabled(request.signal)
      .then((enabled) => {
        if (enabled && !request.signal.aborted) void refresh();
      })
      .catch(() => {});
    return () => {
      request.abort();
      window.clearTimeout(timer);
    };
  }, []);

  return (
    <div aria-live="polite">
      {notice && (
        <Link
          className="mt-3 block rounded-lg border bg-muted/50 p-2 text-xs hover:underline"
          href={"/reservas?demo=" + encodeURIComponent(notice.id)}
        >
          {notice.title}
        </Link>
      )}
    </div>
  );
}
