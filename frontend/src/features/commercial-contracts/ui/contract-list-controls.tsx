"use client";

import { Search } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { cn } from "@/lib/utils";

export type AgreementFilter = "all" | "pending" | "ready" | "closed";

export function matchesAgreementFilter(state: string, filter: AgreementFilter) {
  if (filter === "all") return true;
  if (filter === "pending") return state === "pending_landlord";
  if (filter === "ready") return state === "ready_for_payment";
  return ["rejected", "withdrawn", "expired"].includes(state);
}

export function agreementBadgeClass(state: string) {
  switch (state) {
    case "pending_landlord":
      return "bg-amber-100 text-amber-900 border-amber-200";
    case "ready_for_payment":
      return "bg-emerald-100 text-emerald-900 border-emerald-200";
    case "rejected":
    case "expired":
      return "bg-red-100 text-red-900 border-red-200";
    default:
      return "bg-neutral-100 text-neutral-700 border-neutral-200";
  }
}

export function ContractListToolbar({
  filter,
  onFilterChange,
  search,
  onSearchChange,
  counts,
  onRefresh,
  busy,
}: {
  filter: AgreementFilter;
  onFilterChange: (next: AgreementFilter) => void;
  search: string;
  onSearchChange: (next: string) => void;
  counts: { all: number; pending: number; ready: number; closed: number };
  onRefresh: () => void;
  busy?: boolean;
}) {
  const pills: Array<{ id: AgreementFilter; label: string; count: number }> = [
    { id: "all", label: "Todas", count: counts.all },
    { id: "pending", label: "Pendientes de firma", count: counts.pending },
    { id: "ready", label: "Listas para pagar", count: counts.ready },
    { id: "closed", label: "Cerradas", count: counts.closed },
  ];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1.5">
          {pills.map((pill) => (
            <button
              key={pill.id}
              type="button"
              onClick={() => onFilterChange(pill.id)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors",
                filter === pill.id
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-white text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              <span>{pill.label}</span>
              <span className="opacity-75">({pill.count})</span>
            </button>
          ))}
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={onRefresh}
        >
          Actualizar
        </Button>
      </div>
      <div className="relative">
        <Search
          className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <input
          type="search"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Buscar por folio o estado…"
          className="w-full rounded-lg border border-border bg-white py-2 pl-9 pr-3 text-sm outline-none focus:border-primary"
        />
      </div>
    </div>
  );
}

export function ContractPagination({
  page,
  totalPages,
  onPageChange,
}: {
  page: number;
  totalPages: number;
  onPageChange: (next: number) => void;
}) {
  if (totalPages <= 1) return null;
  return (
    <div className="flex items-center justify-between border-t border-border pt-3 text-sm">
      <span className="text-xs text-muted-foreground">
        Página {page} de {totalPages}
      </span>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          Anterior
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          Siguiente
        </Button>
      </div>
    </div>
  );
}
