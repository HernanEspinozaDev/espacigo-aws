"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  Building2,
  CalendarDays,
  CheckCircle2,
  CreditCard,
  FileText,
  Info,
  Lock,
  ShieldCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { ApiError } from "@/lib/api/errors";
import type {
  AdminAssistedViewAccountSummary,
  AdminAssistedViewContractItem,
  AdminAssistedViewDiagnostic,
  AdminAssistedViewMercadoPagoConnection,
  AdminAssistedViewPaymentItem,
  AdminAssistedViewProjection,
  AdminAssistedViewPublishGuide,
  AdminAssistedViewReservationItem,
  AdminAssistedViewRole,
  AdminAssistedViewSpaceItem,
} from "../domain/types";
import { useAdmin } from "./use-admin";

const dateTimeFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "short",
  timeStyle: "short",
  timeZone: "America/Santiago",
});

const clpFormatter = new Intl.NumberFormat("es-CL", {
  style: "currency",
  currency: "CLP",
  maximumFractionDigits: 0,
});

function formatRoleLabel(role: AdminAssistedViewRole): string {
  return role === "arrendador" ? "Arrendador" : "Arrendatario";
}

function formatDateTime(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  return dateTimeFormatter.format(parsed);
}

type AssistedViewTabId =
  | "resumen"
  | "guia-mp"
  | "espacios"
  | "contratos"
  | "reservas-cobros"
  | "mis-reservas"
  | "mis-contratos"
  | "pagos";

interface TabDefinition {
  readonly id: AssistedViewTabId;
  readonly label: string;
}

function buildTabsForRole(
  role: AdminAssistedViewRole,
  projection: AdminAssistedViewProjection,
): readonly TabDefinition[] {
  if (role === "arrendador") {
    return [
      { id: "resumen", label: "Resumen y diagnóstico" },
      { id: "guia-mp", label: "Guía y Mercado Pago" },
      { id: "espacios", label: `Mis Espacios (${projection.spaces.length})` },
      {
        id: "contratos",
        label: `Contratos recibidos (${projection.contracts.length})`,
      },
      {
        id: "reservas-cobros",
        label: `Reservas y cobros (${projection.reservations.length})`,
      },
    ];
  }
  return [
    { id: "resumen", label: "Resumen y diagnóstico" },
    {
      id: "mis-reservas",
      label: `Mis Reservas (${projection.reservations.length})`,
    },
    {
      id: "mis-contratos",
      label: `Mis Contratos (${projection.contracts.length})`,
    },
    {
      id: "pagos",
      label: `Intentos de pago (${projection.payments.length})`,
    },
  ];
}

function DiagnosticAlert({
  diagnostic,
}: {
  diagnostic: AdminAssistedViewDiagnostic;
}) {
  const styles =
    diagnostic.severity === "error"
      ? "border-red-300 bg-red-50 text-red-950 dark:border-red-800 dark:bg-red-950/40 dark:text-red-100"
      : diagnostic.severity === "warning"
        ? "border-amber-300 bg-amber-50 text-amber-950 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
        : "border-sky-300 bg-sky-50 text-sky-950 dark:border-sky-800 dark:bg-sky-950/40 dark:text-sky-100";

  const Icon =
    diagnostic.severity === "error"
      ? AlertCircle
      : diagnostic.severity === "warning"
        ? AlertTriangle
        : Info;

  return (
    <li className={`rounded-xl border p-4 space-y-1 ${styles}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 font-medium text-sm">
          <Icon className="size-4 shrink-0" aria-hidden="true" />
          <span>{diagnostic.title}</span>
        </div>
        <div className="flex items-center gap-1.5">
          <Badge variant="outline" className="text-[11px]">
            {diagnostic.area}
          </Badge>
          <Badge variant="secondary" className="text-[11px] font-mono">
            {diagnostic.code}
          </Badge>
        </div>
      </div>
      <p className="text-xs opacity-90">{diagnostic.detail}</p>
    </li>
  );
}

function SummaryAndDiagnosticsSection({
  summary,
  diagnostics,
}: {
  summary: AdminAssistedViewAccountSummary;
  diagnostics: readonly AdminAssistedViewDiagnostic[];
}) {
  return (
    <div className="space-y-6">
      <section aria-labelledby="assisted-diagnostics-heading" className="space-y-3">
        <h3
          id="assisted-diagnostics-heading"
          className="flex items-center gap-2 text-lg font-semibold"
        >
          <AlertTriangle className="size-4 text-primary" aria-hidden="true" />
          Diagnóstico operativo ({diagnostics.length})
        </h3>
        {diagnostics.length === 0 ? (
          <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
            Sin hallazgos o bloqueos detectados para esta cuenta.
          </p>
        ) : (
          <ul className="space-y-2.5">
            {diagnostics.map((diagnostic) => (
              <DiagnosticAlert
                key={`${diagnostic.code}-${diagnostic.area}`}
                diagnostic={diagnostic}
              />
            ))}
          </ul>
        )}
      </section>

      <section aria-labelledby="assisted-summary-heading" className="space-y-3">
        <h3
          id="assisted-summary-heading"
          className="flex items-center gap-2 text-lg font-semibold"
        >
          <ShieldCheck className="size-4 text-primary" aria-hidden="true" />
          Resumen de cuenta
        </h3>
        <dl className="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2 lg:grid-cols-3 text-sm">
          <div>
            <dt className="text-xs text-muted-foreground">Estado de cuenta</dt>
            <dd className="mt-1 flex items-center gap-2 font-medium">
              <Badge variant="secondary">{summary.state}</Badge>
              {summary.admin_blocked && (
                <Badge variant="destructive">Bloqueo administrativo</Badge>
              )}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Verificación contacto</dt>
            <dd className="mt-1 flex flex-wrap gap-1.5">
              <Badge variant={summary.email_verified ? "secondary" : "outline"}>
                Email: {summary.email_verified ? "Verificado" : "Pendiente"}
              </Badge>
              <Badge variant={summary.phone_verified ? "secondary" : "outline"}>
                Teléfono: {summary.phone_verified ? "Verificado" : "Pendiente"}
              </Badge>
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Estado KYC</dt>
            <dd className="mt-1 space-y-1">
              <Badge variant="outline">{summary.kyc_status}</Badge>
              {summary.kyc_rejection_reason && (
                <p className="text-xs text-red-700 dark:text-red-300">
                  Motivo: {summary.kyc_rejection_reason}
                </p>
              )}
            </dd>
          </div>
          <div className="sm:col-span-2 lg:col-span-3">
            <dt className="text-xs text-muted-foreground">Roles asignados</dt>
            <dd className="mt-1 flex flex-wrap gap-1.5">
              {summary.roles.length === 0 ? (
                <span className="text-xs text-muted-foreground">Sin roles</span>
              ) : (
                summary.roles.map((r) => (
                  <Badge key={r} variant="secondary">
                    {r}
                  </Badge>
                ))
              )}
            </dd>
          </div>
        </dl>
      </section>
    </div>
  );
}

function PublishGuideAndMercadoPagoSection({
  guide,
  mp,
}: {
  guide?: AdminAssistedViewPublishGuide | null;
  mp?: AdminAssistedViewMercadoPagoConnection | null;
}) {
  return (
    <div className="space-y-6">
      <section aria-labelledby="assisted-mp-heading" className="space-y-3">
        <h3
          id="assisted-mp-heading"
          className="flex items-center gap-2 text-lg font-semibold"
        >
          <CreditCard className="size-4 text-primary" aria-hidden="true" />
          Conexión Mercado Pago
        </h3>
        {!mp ? (
          <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
            Sin información de conexión Mercado Pago disponible.
          </p>
        ) : (
          <dl className="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2 lg:grid-cols-4 text-sm">
            <div>
              <dt className="text-xs text-muted-foreground">Estado</dt>
              <dd className="mt-1">
                <Badge variant={mp.connected ? "secondary" : "outline"}>
                  {mp.connected ? "Conectado" : "No conectado"}
                </Badge>
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Estado de enlace</dt>
              <dd className="mt-1 font-medium">
                {mp.connection_state || "—"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Modo</dt>
              <dd className="mt-1 font-medium">
                {typeof mp.live_mode === "boolean"
                  ? mp.live_mode
                    ? "Producción (live)"
                    : "Pruebas (sandbox)"
                  : "—"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Última actualización</dt>
              <dd className="mt-1 text-xs tabular-nums text-muted-foreground">
                {formatDateTime(mp.updated_at || mp.connected_at) || "—"}
              </dd>
            </div>
          </dl>
        )}
      </section>

      <section aria-labelledby="assisted-guide-heading" className="space-y-3">
        <h3
          id="assisted-guide-heading"
          className="flex items-center gap-2 text-lg font-semibold"
        >
          <CheckCircle2 className="size-4 text-primary" aria-hidden="true" />
          Guía de publicación (4 pasos)
        </h3>
        {!guide ? (
          <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
            Sin guía de publicación registrada para este arrendador.
          </p>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap gap-2 text-xs">
              <Badge variant="outline">KYC: {guide.kyc_status}</Badge>
              <Badge variant="outline">Borrador: {guide.draft_status}</Badge>
              <Badge variant="outline">MP: {guide.mercadopago_status}</Badge>
              <Badge variant="outline">Contrato: {guide.contract_status}</Badge>
            </div>
            <ol className="grid gap-3 sm:grid-cols-2">
              {guide.steps.map((step) => (
                <li
                  key={step.key}
                  className="rounded-xl border bg-card p-4 space-y-1.5"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-sm font-semibold">
                      Paso {step.step}: {step.title}
                    </span>
                    <Badge variant="secondary">{step.status}</Badge>
                  </div>
                  <p className="text-xs text-muted-foreground">{step.detail}</p>
                </li>
              ))}
            </ol>
          </div>
        )}
      </section>
    </div>
  );
}

function SpacesSection({
  spaces,
}: {
  spaces: readonly AdminAssistedViewSpaceItem[];
}) {
  return (
    <section aria-labelledby="assisted-spaces-heading" className="space-y-3">
      <h3
        id="assisted-spaces-heading"
        className="flex items-center gap-2 text-lg font-semibold"
      >
        <Building2 className="size-4 text-primary" aria-hidden="true" />
        Mis Espacios ({spaces.length})
      </h3>
      {spaces.length === 0 ? (
        <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
          Sin espacios asociados para este rol.
        </p>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2">
          {spaces.map((space) => (
            <li
              key={space.id}
              className="rounded-xl border bg-card p-4 space-y-2"
            >
              <div className="flex items-start justify-between gap-2">
                <p className="font-medium text-foreground">{space.title}</p>
                <Badge variant="secondary">{space.status}</Badge>
              </div>
              <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                {space.modality && <span>Modalidad: {space.modality}</span>}
                {typeof space.price_clp === "number" && (
                  <span className="font-medium text-foreground tabular-nums">
                    {clpFormatter.format(space.price_clp)}
                  </span>
                )}
                <span>
                  Ubicación: {space.location_confirmed ? "Confirmada" : "Pendiente"}
                </span>
                <span>Contrato v{space.contract_published_version}</span>
              </div>
              {space.publish_blockers.length > 0 && (
                <div className="rounded-lg border border-amber-300 bg-amber-50 p-2.5 text-xs text-amber-950 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100 space-y-1">
                  <p className="font-medium">Bloqueos de publicación:</p>
                  <ul className="list-disc pl-4 space-y-0.5">
                    {space.publish_blockers.map((blocker) => (
                      <li key={blocker}>{blocker}</li>
                    ))}
                  </ul>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function ContractsSection({
  title,
  contracts,
}: {
  title: string;
  contracts: readonly AdminAssistedViewContractItem[];
}) {
  return (
    <section aria-labelledby="assisted-contracts-heading" className="space-y-3">
      <h3
        id="assisted-contracts-heading"
        className="flex items-center gap-2 text-lg font-semibold"
      >
        <FileText className="size-4 text-primary" aria-hidden="true" />
        {title} ({contracts.length})
      </h3>
      {contracts.length === 0 ? (
        <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
          Sin contratos asociados para este rol.
        </p>
      ) : (
        <ul className="space-y-2.5">
          {contracts.map((contract) => (
            <li
              key={contract.id}
              className="rounded-xl border bg-card p-4 flex flex-wrap items-center justify-between gap-3"
            >
              <div className="space-y-1 min-w-0">
                <p className="font-medium text-foreground">
                  {contract.space_title || contract.id}
                </p>
                <p className="text-xs text-muted-foreground font-mono">
                  {contract.id}
                </p>
                <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                  {contract.signing_mode && (
                    <span>Modalidad firma: {contract.signing_mode}</span>
                  )}
                  {contract.created_at && (
                    <span className="tabular-nums">
                      {formatDateTime(contract.created_at)}
                    </span>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {typeof contract.subtotal_clp === "number" && (
                  <span className="text-sm font-semibold tabular-nums">
                    {clpFormatter.format(contract.subtotal_clp)}
                  </span>
                )}
                {contract.document_status && (
                  <Badge variant="outline">{contract.document_status}</Badge>
                )}
                <Badge variant="secondary">{contract.status}</Badge>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function ReservationsSection({
  title,
  reservations,
}: {
  title: string;
  reservations: readonly AdminAssistedViewReservationItem[];
}) {
  return (
    <section aria-labelledby="assisted-reservations-heading" className="space-y-3">
      <h3
        id="assisted-reservations-heading"
        className="flex items-center gap-2 text-lg font-semibold"
      >
        <CalendarDays className="size-4 text-primary" aria-hidden="true" />
        {title} ({reservations.length})
      </h3>
      {reservations.length === 0 ? (
        <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
          Sin reservas asociadas para este rol.
        </p>
      ) : (
        <ul className="space-y-2.5">
          {reservations.map((reservation) => (
            <li
              key={reservation.id}
              className="rounded-xl border bg-card p-4 flex flex-wrap items-center justify-between gap-3"
            >
              <div className="space-y-1 min-w-0">
                <p className="font-medium text-foreground">
                  {reservation.space_title || reservation.id}
                </p>
                <p className="text-xs text-muted-foreground font-mono">
                  {reservation.id}
                </p>
                <p className="text-xs text-muted-foreground tabular-nums">
                  {formatDateTime(reservation.start_at)} –{" "}
                  {formatDateTime(reservation.end_at)}
                </p>
              </div>
              <div className="flex items-center gap-2">
                {typeof reservation.subtotal_clp === "number" && (
                  <span className="text-sm font-semibold tabular-nums">
                    {clpFormatter.format(reservation.subtotal_clp)}
                  </span>
                )}
                <Badge variant="secondary">{reservation.status}</Badge>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function PaymentsSection({
  title,
  payments,
}: {
  title: string;
  payments: readonly AdminAssistedViewPaymentItem[];
}) {
  return (
    <section aria-labelledby="assisted-payments-heading" className="space-y-3">
      <h3
        id="assisted-payments-heading"
        className="flex items-center gap-2 text-lg font-semibold"
      >
        <CreditCard className="size-4 text-primary" aria-hidden="true" />
        {title} ({payments.length})
      </h3>
      {payments.length === 0 ? (
        <p className="rounded-xl border bg-card p-4 text-sm text-muted-foreground">
          Sin intentos de pago registrados para este rol.
        </p>
      ) : (
        <ul className="space-y-2.5">
          {payments.map((payment) => (
            <li
              key={payment.id}
              className="rounded-xl border bg-card p-4 space-y-2"
            >
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="space-y-1 min-w-0">
                  <p className="font-medium text-foreground">
                    {payment.space_title || payment.id}
                  </p>
                  <p className="text-xs text-muted-foreground font-mono">
                    {payment.id}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold tabular-nums">
                    {clpFormatter.format(payment.total_charged_clp)}
                  </span>
                  <Badge variant="outline">{payment.checkout_method}</Badge>
                  {payment.test_mode && <Badge variant="outline">Test</Badge>}
                  <Badge variant="secondary">{payment.status}</Badge>
                </div>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                {payment.provider_payment_status && (
                  <span>
                    Estado proveedor:{" "}
                    <strong className="text-foreground">
                      {payment.provider_payment_status}
                    </strong>
                  </span>
                )}
                {payment.reconciliation_reason && (
                  <span>
                    Reconciliación:{" "}
                    <strong className="text-foreground">
                      {payment.reconciliation_reason}
                    </strong>
                  </span>
                )}
                <span className="tabular-nums">
                  {formatDateTime(payment.created_at)}
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

type ProjectionState =
  | { status: "loading" }
  | { status: "ready"; value: AdminAssistedViewProjection }
  | { status: "error"; message: string };

export function AdminAssistedViewPanel({ accountId }: { accountId: string }) {
  const { snapshot, hasAdminCapability, getAssistedViewProjection } = useAdmin();
  const activeGrant =
    snapshot.assistedViewGrant?.account_id === accountId
      ? snapshot.assistedViewGrant
      : null;

  const [state, setState] = useState<ProjectionState>({ status: "loading" });
  const [activeTab, setActiveTab] = useState<AssistedViewTabId>("resumen");

  useEffect(() => {
    if (!activeGrant) return;
    const controller = new AbortController();
    void getAssistedViewProjection(activeGrant.grant_id, controller.signal)
      .then((projection) => {
        if (controller.signal.aborted) return;
        if (!projection) {
          setState({
            status: "error",
            message: "No fue posible cargar la proyección asistida.",
          });
          return;
        }
        setState({ status: "ready", value: projection });
        setActiveTab("resumen");
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 404) {
          setState({
            status: "error",
            message: "La vista asistida expiró o ya fue cerrada.",
          });
          return;
        }
        setState({
          status: "error",
          message: "No fue posible cargar la proyección de solo lectura.",
        });
      });
    return () => controller.abort();
  }, [activeGrant, getAssistedViewProjection]);

  const tabs = useMemo(() => {
    if (state.status !== "ready") return [];
    return buildTabsForRole(state.value.role, state.value);
  }, [state]);

  if (
    snapshot.adminSessionStatus === "unknown" ||
    snapshot.adminSessionStatus === "checking"
  ) {
    return <p role="status">Comprobando sesión…</p>;
  }

  if (
    snapshot.adminSessionStatus !== "authenticated" ||
    !hasAdminCapability("accounts.read")
  ) {
    return (
      <p role="status">
        Necesitas permiso para consultar la vista asistida de esta cuenta.
      </p>
    );
  }

  if (!activeGrant) {
    return (
      <div className="space-y-4">
        <Link
          href={`/administracion/cuentas/${accountId}`}
          className="inline-flex min-h-11 items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" /> Volver a Ficha 360
        </Link>
        <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">
          No hay una vista asistida activa para esta cuenta. Inicia una desde la
          Ficha 360 indicando rol, caso y motivo.
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link
          href={`/administracion/cuentas/${accountId}`}
          className="inline-flex min-h-11 items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" /> Volver a Ficha 360
        </Link>
        <Badge
          variant="outline"
          className="gap-1.5 border-amber-500 bg-amber-50 text-amber-950 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-100"
        >
          <Lock className="size-3.5" aria-hidden="true" />
          Vista asistida · Solo lectura (acciones comerciales bloqueadas)
        </Badge>
      </div>

      <header className="rounded-2xl border bg-card p-5 shadow-xs space-y-2">
        <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Proyección de cuenta ({formatRoleLabel(activeGrant.role)})
        </p>
        <h2 className="text-xl font-semibold tracking-tight sm:text-2xl">
          Vista asistida · Cuenta{" "}
          <span className="font-mono text-base sm:text-lg">{accountId}</span>
        </h2>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>
            Caso: <strong className="text-foreground">{activeGrant.case_id}</strong>
          </span>
          <span>
            Motivo:{" "}
            <strong className="text-foreground">{activeGrant.reason}</strong>
          </span>
          <span>
            Expira:{" "}
            <strong className="text-foreground tabular-nums">
              {formatDateTime(activeGrant.expires_at)}
            </strong>
          </span>
        </div>
      </header>

      {state.status === "loading" ? (
        <p role="status" className="text-sm text-muted-foreground">
          Cargando proyección de solo lectura…
        </p>
      ) : state.status === "error" ? (
        <p
          role="alert"
          className="rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
        >
          {state.message}
        </p>
      ) : (
        <div className="space-y-5">
          <div
            role="tablist"
            aria-label="Secciones de vista asistida"
            className="flex flex-wrap gap-2 border-b border-border pb-2"
          >
            {tabs.map((tab) => {
              const selected = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  type="button"
                  role="tab"
                  id={`assisted-tab-${tab.id}`}
                  aria-selected={selected}
                  aria-controls={`assisted-panel-${tab.id}`}
                  onClick={() => setActiveTab(tab.id)}
                  className={`min-h-10 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    selected
                      ? "bg-primary text-primary-foreground shadow-xs"
                      : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground"
                  }`}
                >
                  {tab.label}
                </button>
              );
            })}
          </div>

          <div
            role="tabpanel"
            id={`assisted-panel-${activeTab}`}
            aria-labelledby={`assisted-tab-${activeTab}`}
          >
            {activeTab === "resumen" && (
              <SummaryAndDiagnosticsSection
                summary={state.value.account_summary}
                diagnostics={state.value.diagnostics}
              />
            )}
            {activeTab === "guia-mp" && (
              <PublishGuideAndMercadoPagoSection
                guide={state.value.publish_guide}
                mp={state.value.mercadopago_connection}
              />
            )}
            {activeTab === "espacios" && (
              <SpacesSection spaces={state.value.spaces} />
            )}
            {activeTab === "contratos" && (
              <ContractsSection
                title="Contratos recibidos"
                contracts={state.value.contracts}
              />
            )}
            {activeTab === "reservas-cobros" && (
              <div className="space-y-6">
                <ReservationsSection
                  title="Reservas"
                  reservations={state.value.reservations}
                />
                <PaymentsSection
                  title="Cobros asociados"
                  payments={state.value.payments}
                />
              </div>
            )}
            {activeTab === "mis-reservas" && (
              <ReservationsSection
                title="Mis Reservas"
                reservations={state.value.reservations}
              />
            )}
            {activeTab === "mis-contratos" && (
              <ContractsSection
                title="Mis Contratos"
                contracts={state.value.contracts}
              />
            )}
            {activeTab === "pagos" && (
              <PaymentsSection
                title="Intentos de pago"
                payments={state.value.payments}
              />
            )}
          </div>
        </div>
      )}
    </div>
  );
}
