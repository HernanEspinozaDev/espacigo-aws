"use client";

import Link from "next/link";
import { Eye, Lock, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import { useAdmin } from "./use-admin";

const timeFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "short",
  timeStyle: "short",
  timeZone: "America/Santiago",
});

function formatRoleLabel(role: "arrendador" | "arrendatario"): string {
  return role === "arrendador" ? "Arrendador" : "Arrendatario";
}

function formatExpiration(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  return timeFormatter.format(parsed);
}

export function AdminAssistedViewBanner() {
  const { snapshot, closeAssistedView } = useAdmin();
  const grant = snapshot.assistedViewGrant;
  if (!grant) return null;

  return (
    <aside
      aria-label="Vista asistida activa"
      className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-400 bg-amber-50/90 px-4 py-3 text-amber-950 shadow-xs dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-100"
    >
      <div className="flex flex-wrap items-center gap-2.5 min-w-0">
        <Badge
          variant="outline"
          className="gap-1.5 border-amber-500 bg-amber-100 text-amber-950 dark:border-amber-600 dark:bg-amber-900/60 dark:text-amber-100"
        >
          <Lock className="size-3.5" aria-hidden="true" />
          <span>
            Vista asistida · Solo lectura (acciones comerciales bloqueadas)
          </span>
        </Badge>

        <span className="text-xs font-medium">
          Cuenta:{" "}
          <span className="font-mono font-semibold">{grant.account_id}</span>
        </span>
        <span className="text-xs font-medium">
          Rol:{" "}
          <span className="font-semibold">{formatRoleLabel(grant.role)}</span>
        </span>
        <span className="text-xs font-medium">
          Caso: <span className="font-mono font-semibold">{grant.case_id}</span>
        </span>
        <span className="text-xs text-amber-900 dark:text-amber-200 tabular-nums">
          Expira: {formatExpiration(grant.expires_at)}
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Link
          href={`/administracion/cuentas/${grant.account_id}/vista-asistida`}
          className="inline-flex min-h-8 items-center gap-1.5 rounded-md border border-amber-500/60 bg-background px-2.5 py-1 text-xs font-medium text-foreground hover:bg-muted"
        >
          <Eye className="size-3.5" aria-hidden="true" />
          Ver proyección
        </Link>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={snapshot.actionLoading}
          onClick={() => void closeAssistedView(grant.grant_id)}
          className="h-8 border-amber-500/60 bg-background px-2.5 text-xs text-foreground hover:bg-muted"
        >
          <X className="size-3.5 mr-1" aria-hidden="true" />
          Cerrar vista del cliente
        </Button>
      </div>
    </aside>
  );
}
