"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowUpRight, Lock, ShieldCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { apiClient } from "@/lib/api/client";
import {
  ApiError,
  ApiProtocolError,
  isAbortError,
  normalizeApiError,
} from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";

type GlobalAdminPrivacyRequest =
  components["schemas"]["GlobalAdminPrivacyRequest"];
type GlobalAdminPrivacyLegalHold =
  components["schemas"]["GlobalAdminPrivacyLegalHold"];

type SectionState<T> =
  | { status: "loading" }
  | { status: "ready"; items: T[] }
  | { status: "forbidden" }
  | { status: "error"; message: string };

const dateFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "medium",
  timeZone: "America/Santiago",
});

const dateTimeFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "short",
  timeStyle: "short",
  timeZone: "America/Santiago",
});

const RIGHT_TYPE_LABELS: Record<
  GlobalAdminPrivacyRequest["right_type"],
  string
> = {
  acceso: "Acceso",
  rectificacion: "Rectificación",
  supresion: "Supresión",
  oposicion: "Oposición",
  portabilidad: "Portabilidad",
  bloqueo: "Bloqueo",
};

const REQUEST_STATUS_LABELS: Record<
  GlobalAdminPrivacyRequest["status"],
  string
> = {
  pendiente: "Pendiente",
  en_revision: "En revisión",
  aprobada: "Aprobada",
  rechazada: "Rechazada",
  completada: "Completada",
};

const CHANNEL_LABELS: Record<
  NonNullable<GlobalAdminPrivacyRequest["channel"]>,
  string
> = {
  web: "Web",
  api: "API",
  publico: "Público",
  correo: "Correo",
};

const HOLD_SCOPE_LABELS: Record<GlobalAdminPrivacyLegalHold["scope"], string> =
  {
    expediente_privacidad: "Expediente de privacidad",
    cuenta_completa: "Cuenta completa",
    evidencia_kyc: "Evidencia KYC",
    contratos_y_pagos: "Contratos y pagos",
    incidente_seguridad: "Incidente de seguridad",
  };

const HOLD_STATUS_LABELS: Record<
  GlobalAdminPrivacyLegalHold["status"],
  string
> = {
  activa: "Activa",
  liberada: "Liberada",
};

function formatShortDate(value?: string | null): string | null {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return dateFormatter.format(parsed);
}

function formatDateTime(value?: string | null): string | null {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return dateTimeFormatter.format(parsed);
}

function toSectionErrorState<T>(
  error: unknown,
  fallbackMessage: string,
): SectionState<T> {
  if (error instanceof ApiError && error.status === 403) {
    return { status: "forbidden" };
  }
  return { status: "error", message: fallbackMessage };
}

function requestStatusVariant(
  status: GlobalAdminPrivacyRequest["status"],
): "secondary" | "destructive" | "outline" {
  if (status === "rechazada") return "destructive";
  if (status === "completada" || status === "aprobada") return "secondary";
  return "outline";
}

export function AdminCustomer360PrivacyTab({
  accountId,
}: {
  accountId: string;
}) {
  const [requestsState, setRequestsState] = useState<
    SectionState<GlobalAdminPrivacyRequest>
  >({ status: "loading" });
  const [holdsState, setHoldsState] = useState<
    SectionState<GlobalAdminPrivacyLegalHold>
  >({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();

    async function loadPrivacyRequests() {
      try {
        const result = await apiClient.GET("/admin/global/privacy-requests", {
          params: {
            query: {
              account_id: accountId,
              page_size: 25,
            },
          },
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        if (!result.response.ok) {
          throw normalizeApiError(result.response, result.error);
        }
        if (!result.data?.data || !Array.isArray(result.data.data.items)) {
          throw new ApiProtocolError();
        }
        setRequestsState({
          status: "ready",
          items: result.data.data.items,
        });
      } catch (error) {
        if (controller.signal.aborted || isAbortError(error)) return;
        setRequestsState(
          toSectionErrorState(
            error,
            "No pudimos cargar las solicitudes ARCO+ de esta cuenta.",
          ),
        );
      }
    }

    async function loadLegalHolds() {
      try {
        const result = await apiClient.GET(
          "/admin/global/privacy-legal-holds",
          {
            params: {
              query: {
                account_id: accountId,
                page_size: 25,
              },
            },
            signal: controller.signal,
          },
        );
        if (controller.signal.aborted) return;
        if (!result.response.ok) {
          throw normalizeApiError(result.response, result.error);
        }
        if (!result.data?.data || !Array.isArray(result.data.data.items)) {
          throw new ApiProtocolError();
        }
        setHoldsState({
          status: "ready",
          items: result.data.data.items,
        });
      } catch (error) {
        if (controller.signal.aborted || isAbortError(error)) return;
        setHoldsState(
          toSectionErrorState(
            error,
            "No pudimos cargar las retenciones legales de esta cuenta.",
          ),
        );
      }
    }

    void Promise.all([loadPrivacyRequests(), loadLegalHolds()]);

    return () => controller.abort();
  }, [accountId]);

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border bg-card p-4 sm:px-7">
        <div className="min-w-0">
          <h3 className="font-semibold">Privacidad y ARCO+ (Ley 21.719)</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            Resumen de solicitudes ARCO+ y retenciones legales asociadas a esta
            cuenta.
          </p>
        </div>
        <Link
          href="/administracion/privacidad"
          className="inline-flex min-h-10 items-center gap-1.5 rounded-lg border bg-background px-3 py-1.5 text-sm font-medium text-foreground shadow-xs transition-colors hover:bg-muted"
        >
          Ir a gestión completa de privacidad y retenciones
          <ArrowUpRight className="size-4" aria-hidden="true" />
        </Link>
      </div>

      <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h4 className="flex items-center gap-2 font-semibold">
            <ShieldCheck className="size-4 text-primary" aria-hidden="true" />
            Solicitudes ARCO+ vinculadas a la cuenta
          </h4>
          {requestsState.status === "ready" && (
            <span className="text-xs text-muted-foreground tabular-nums">
              Total: {requestsState.items.length}
            </span>
          )}
        </div>

        {requestsState.status === "loading" ? (
          <p role="status" className="text-sm">
            Cargando solicitudes ARCO+…
          </p>
        ) : requestsState.status === "forbidden" ? (
          <p role="status" className="text-sm text-muted-foreground">
            No tienes permisos de lectura para consultar las solicitudes ARCO+
            de esta cuenta.
          </p>
        ) : requestsState.status === "error" ? (
          <p
            role="alert"
            className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
          >
            {requestsState.message}
          </p>
        ) : requestsState.items.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Esta cuenta no registra solicitudes ARCO+ vinculadas.
          </p>
        ) : (
          <ul className="divide-y">
            {requestsState.items.map((req) => {
              const displayFolio = req.folio?.trim() || req.id.slice(0, 8);
              const dueAtValue = req.extended_due_at ?? req.due_at;
              const formattedDueAt = formatShortDate(dueAtValue);
              const rightLabel =
                RIGHT_TYPE_LABELS[req.right_type] ?? req.right_type;
              const statusLabel =
                REQUEST_STATUS_LABELS[req.status] ?? req.status;
              const channelLabel = req.channel
                ? (CHANNEL_LABELS[req.channel] ?? req.channel)
                : "No informado";

              return (
                <li key={req.id} className="py-3.5 space-y-2">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0 space-y-0.5">
                      <p className="font-mono text-sm font-medium break-all">
                        {displayFolio}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Derecho: {rightLabel}
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {req.is_overdue === true && (
                        <Badge variant="destructive">Vencida</Badge>
                      )}
                      <Badge variant={requestStatusVariant(req.status)}>
                        {statusLabel}
                      </Badge>
                    </div>
                  </div>

                  <dl className="grid grid-cols-2 sm:grid-cols-3 gap-2 text-xs text-muted-foreground">
                    <div>
                      <dt className="font-medium text-foreground/80">Canal</dt>
                      <dd>{channelLabel}</dd>
                    </div>
                    <div>
                      <dt className="font-medium text-foreground/80">
                        Fecha límite
                      </dt>
                      <dd>
                        {formattedDueAt
                          ? `${formattedDueAt}${req.extended_due_at ? " (prorrogada)" : ""}`
                          : "Sin fecha límite"}
                      </dd>
                    </div>
                    <div>
                      <dt className="font-medium text-foreground/80">
                        Vencimiento
                      </dt>
                      <dd>
                        {req.is_overdue === true
                          ? "Plazo vencido"
                          : "Dentro de plazo"}
                      </dd>
                    </div>
                  </dl>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h4 className="flex items-center gap-2 font-semibold">
            <Lock className="size-4 text-amber-600" aria-hidden="true" />
            Retenciones legales (Legal Hold) de la cuenta
          </h4>
          {holdsState.status === "ready" && (
            <span className="text-xs text-muted-foreground tabular-nums">
              Total: {holdsState.items.length}
            </span>
          )}
        </div>

        {holdsState.status === "loading" ? (
          <p role="status" className="text-sm">
            Cargando retenciones legales…
          </p>
        ) : holdsState.status === "forbidden" ? (
          <p role="status" className="text-sm text-muted-foreground">
            No tienes permisos de lectura para consultar las retenciones legales
            de esta cuenta.
          </p>
        ) : holdsState.status === "error" ? (
          <p
            role="alert"
            className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
          >
            {holdsState.message}
          </p>
        ) : holdsState.items.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Esta cuenta no registra retenciones legales.
          </p>
        ) : (
          <ul className="divide-y">
            {holdsState.items.map((hold) => {
              const scopeLabel = HOLD_SCOPE_LABELS[hold.scope] ?? hold.scope;
              const statusLabel =
                HOLD_STATUS_LABELS[hold.status] ?? hold.status;
              const appliedAtLabel =
                formatDateTime(hold.applied_at) ?? hold.applied_at;

              return (
                <li key={hold.id} className="py-3.5 space-y-2">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0 space-y-0.5">
                      <p className="font-mono text-sm font-medium break-all">
                        {hold.folio}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Ámbito: {scopeLabel}
                      </p>
                    </div>
                    <Badge
                      variant={
                        hold.status === "activa" ? "outline" : "secondary"
                      }
                      className={
                        hold.status === "activa"
                          ? "border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-200"
                          : undefined
                      }
                    >
                      {statusLabel}
                    </Badge>
                  </div>

                  <dl className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs text-muted-foreground">
                    <div>
                      <dt className="font-medium text-foreground/80">
                        Autoridad
                      </dt>
                      <dd className="break-words">{hold.authority}</dd>
                    </div>
                    <div>
                      <dt className="font-medium text-foreground/80">
                        Fecha de aplicación
                      </dt>
                      <dd>{appliedAtLabel}</dd>
                    </div>
                    <div className="sm:col-span-2">
                      <dt className="font-medium text-foreground/80">
                        Fundamento
                      </dt>
                      <dd className="break-words">{hold.legal_basis}</dd>
                    </div>
                  </dl>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
