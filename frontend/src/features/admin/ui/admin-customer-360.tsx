"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import {
  ArrowLeft,
  Building2,
  CalendarDays,
  Eye,
  ShieldAlert,
  SquareActivity,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiError } from "@/lib/api/errors";
import { adminAdapter } from "../admin-runtime";
import { isValidAdminReason } from "../domain/admin-rules";
import type {
  AdminAssistedViewRole,
  AuditEvent,
  GlobalAdminAccount,
  GlobalAdminAccountResource,
  GlobalAdminAccountResourceKind,
  GlobalAdminAccountResourcePage,
  GlobalAdminPublication,
} from "../domain/types";
import { AdminSensitiveEmailControl } from "./admin-sensitive-email-control";
import { useAdmin } from "./use-admin";

const dateFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "medium",
  timeZone: "America/Santiago",
});

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

const accountStatus = {
  correo_pendiente: "Correo pendiente",
  activo: "Activa",
  bloqueado: "Bloqueada",
  baja_solicitada: "Cierre solicitado",
  desidentificado: "Desidentificada",
} as Record<string, string>;

const publicationStatus: Record<string, string> = {
  borrador: "Borrador",
  activa: "Activa",
  oculta: "Oculta",
  eliminada: "Retirada",
};

type CustomerTab =
  | "overview"
  | "spaces"
  | "commercial_reservations"
  | "commercial_contracts"
  | "commercial_payments"
  | "reservations"
  | "incidents"
  | "audit"
  | "actions";

type Result<T> =
  | { status: "ready"; value: T }
  | { status: "error"; message: string };

type OperationalReason =
  | "correccion_perfil"
  | "revision_administrativa"
  | "gestion_operativa";

function requestError(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.status === 403)
    return "No tienes permisos para acceder a esta información.";
  if (error instanceof ApiError && error.status === 404)
    return "La información no está disponible en este entorno.";
  return fallback;
}

function getOriginBadge(
  item: GlobalAdminAccountResource,
  kind: GlobalAdminAccountResourceKind,
): { label: string; className: string } {
  if (
    item.data_origin === "trial" ||
    kind === "reservations" ||
    kind === "incidents"
  ) {
    return {
      label: "Ensayo local",
      className:
        "border-slate-300 bg-slate-100 text-slate-800 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200",
    };
  }
  if (item.test_mode === true) {
    return {
      label: "Prueba MP",
      className:
        "border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-200",
    };
  }
  return {
    label: "Comercial",
    className:
      "border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200",
  };
}

function formatRoleInRecord(role: string | undefined): string | null {
  if (!role) return null;
  if (role === "arrendatario") return "Arrendatario";
  if (role === "anfitrion" || role === "arrendador") return "Anfitrión";
  return role;
}

export function AdminCustomer360({ accountId }: { accountId: string }) {
  const { snapshot, hasAdminCapability } = useAdmin();
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
    return <p role="status">Necesitas permiso para consultar esta ficha.</p>;
  }

  return (
    <CustomerRecord
      key={JSON.stringify([snapshot.adminSession?.account_id, accountId])}
      accountId={accountId}
      canReadSpaces={hasAdminCapability("publications.list")}
      canModerateSpaces={hasAdminCapability("publications.moderate")}
      canRevealEmail={hasAdminCapability("accounts.email.reveal")}
      canReadAudit={hasAdminCapability("audit.read")}
      canBlock={hasAdminCapability("accounts.block")}
      canManageRoles={hasAdminCapability("accounts.roles")}
      canUpdateAccount={hasAdminCapability("accounts.update")}
    />
  );
}

function AssistedViewTriggerModal({ accountId }: { accountId: string }) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialAssistedCase = searchParams.get("assisted_case")?.trim() ?? "";
  const { startAssistedView } = useAdmin();
  const [open, setOpen] = useState(Boolean(initialAssistedCase));
  const [role, setRole] = useState<AdminAssistedViewRole>("arrendador");
  const [caseId, setCaseId] = useState(initialAssistedCase);
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!caseId.trim()) {
      setLocalError("El identificador de caso es obligatorio.");
      return;
    }
    if (!isValidAdminReason(reason)) {
      setLocalError("El motivo es obligatorio y debe tener al menos 5 caracteres.");
      return;
    }
    setSubmitting(true);
    setLocalError(null);
    try {
      const grant = await startAssistedView(accountId, {
        role,
        case_id: caseId.trim(),
        reason: reason.trim(),
      });
      if (grant) {
        setOpen(false);
        router.push(`/administracion/cuentas/${accountId}/vista-asistida`);
      }
    } catch (err) {
      if (
        err instanceof ApiError &&
        (err.status === 428 || err.code === "recent_authentication_required")
      ) {
        setLocalError(
          "Se requiere verificación MFA reciente. Completa el código en la ventana emergente y reintenta.",
        );
      } else {
        setLocalError("No pudimos iniciar la vista asistida en este momento.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => {
          setLocalError(null);
          setOpen(true);
        }}
      >
        <Eye className="size-4 mr-1.5" aria-hidden="true" />
        Abrir vista del cliente
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ShieldAlert className="size-5 text-amber-600" aria-hidden="true" />
              <span>Abrir vista asistida (solo lectura)</span>
            </DialogTitle>
            <DialogDescription>
              Proyección temporal de solo lectura acotada al rol seleccionado.
              Las acciones comerciales quedan bloqueadas y el acceso se audita.
            </DialogDescription>
          </DialogHeader>

          <form className="space-y-4" onSubmit={handleSubmit}>
            {localError && (
              <p
                role="alert"
                className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
              >
                {localError}
              </p>
            )}

            <div className="space-y-1.5">
              <Label htmlFor={`assisted-role-${accountId}`}>Rol a proyectar</Label>
              <Select
                value={role}
                onValueChange={(value) => setRole(value as AdminAssistedViewRole)}
              >
                <SelectTrigger id={`assisted-role-${accountId}`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="arrendador">Arrendador</SelectItem>
                  <SelectItem value="arrendatario">Arrendatario</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor={`assisted-case-${accountId}`}>
                Caso / Ticket (obligatorio)
              </Label>
              <Input
                id={`assisted-case-${accountId}`}
                value={caseId}
                onChange={(e) => setCaseId(e.target.value)}
                placeholder="Ej: SUP-2026-2040"
                required
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor={`assisted-reason-${accountId}`}>
                Motivo (obligatorio, mínimo 5 caracteres)
              </Label>
              <textarea
                id={`assisted-reason-${accountId}`}
                className="flex min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Ej: Revisión de reservas y contratos reportados en soporte"
                rows={3}
                required
              />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting ? "Abriendo…" : "Iniciar vista asistida"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

function CustomerRecord({
  accountId,
  canReadSpaces,
  canModerateSpaces,
  canRevealEmail,
  canReadAudit,
  canBlock,
  canManageRoles,
  canUpdateAccount,
}: {
  accountId: string;
  canReadSpaces: boolean;
  canModerateSpaces: boolean;
  canRevealEmail: boolean;
  canReadAudit: boolean;
  canBlock: boolean;
  canManageRoles: boolean;
  canUpdateAccount: boolean;
}) {
  const [record, setRecord] = useState<Result<GlobalAdminAccount | null>>();
  const [reloadNonce, setReloadNonce] = useState(0);
  const [tab, setTab] = useState<CustomerTab>("overview");
  const canOperate = canBlock || canManageRoles || canUpdateAccount;

  useEffect(() => {
    const controller = new AbortController();
    void adminAdapter.getGlobalAccount(accountId, controller.signal).then(
      (value) => {
        if (!controller.signal.aborted) {
          setRecord({
            status: "ready",
            value: value?.account_id === accountId ? value : null,
          });
        }
      },
      (error) => {
        if (!controller.signal.aborted) {
          setRecord({
            status: "error",
            message: requestError(error, "No pudimos cargar la ficha."),
          });
        }
      },
    );
    return () => controller.abort();
  }, [accountId, reloadNonce]);

  const tabs: { id: CustomerTab; label: string; visible: boolean }[] = [
    { id: "overview", label: "Resumen", visible: true },
    { id: "spaces", label: "Espacios", visible: canReadSpaces },
    {
      id: "commercial_reservations",
      label: "Reservas comerciales",
      visible: true,
    },
    { id: "commercial_contracts", label: "Contratos", visible: true },
    { id: "commercial_payments", label: "Pagos", visible: true },
    { id: "reservations", label: "Reservas (ensayo)", visible: true },
    { id: "incidents", label: "Incidentes (ensayo)", visible: true },
    { id: "audit", label: "Auditoría", visible: canReadAudit },
    { id: "actions", label: "Acciones operativas", visible: canOperate },
  ];

  return (
    <div className="space-y-5 min-w-0">
      <Link
        href="/administracion/cuentas"
        className="inline-flex min-h-11 items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" aria-hidden="true" /> Volver a cuentas
      </Link>

      {!record ? (
        <p role="status">Cargando ficha…</p>
      ) : record.status === "error" ? (
        <p role="alert">{record.message}</p>
      ) : record.value ? (
        <>
          <header className="rounded-2xl border border-t-2 border-t-primary bg-card text-card-foreground shadow-sm">
            <div className="flex flex-wrap items-start justify-between gap-4 rounded-t-2xl border-b bg-muted/30 p-5 sm:px-7">
              <div className="min-w-0">
                <p className="text-sm font-medium text-muted-foreground">
                  Ficha 360
                </p>
                <h2 className="mt-1 break-words text-2xl font-semibold tracking-tight sm:text-3xl">
                  {record.value.display_name || "Cuenta sin nombre"}
                </h2>
                <p className="mt-1 text-sm text-muted-foreground break-all">
                  {canRevealEmail && record.value.state !== "desidentificado" ? (
                    <AdminSensitiveEmailControl
                      accountId={accountId}
                      maskedEmail={record.value.email}
                      defaultScope="account_summary"
                    />
                  ) : (
                    record.value.email
                  )}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <AssistedViewTriggerModal accountId={accountId} />
                <span className="rounded-full border bg-background px-3 py-1 text-sm font-medium">
                  {record.value.administrative_blocked
                    ? "Bloqueo administrativo"
                    : (accountStatus[record.value.state] ?? record.value.state)}
                </span>
              </div>
            </div>
            <dl className="grid grid-cols-3 divide-x p-2 sm:p-4">
              {[
                {
                  label: "Espacios",
                  count: record.value.publication_count,
                  Icon: Building2,
                },
                {
                  label: "Reservas de ensayo",
                  count: record.value.reservation_count,
                  Icon: CalendarDays,
                },
                {
                  label: "Incidentes de ensayo",
                  count: record.value.incident_count,
                  Icon: SquareActivity,
                },
              ].map(({ label, count, Icon }) => (
                <div key={label} className="min-w-0 px-2 py-3 sm:px-4">
                  <dt className="flex items-center gap-2 text-xs font-medium text-muted-foreground sm:text-sm">
                    <Icon
                      className="hidden size-4 sm:block"
                      aria-hidden="true"
                    />
                    {label}
                  </dt>
                  <dd className="mt-1 text-xl font-semibold tabular-nums">
                    {count}
                  </dd>
                </div>
              ))}
            </dl>
          </header>

          <nav
            aria-label="Secciones de la ficha"
            className="flex flex-wrap gap-1 rounded-xl border bg-muted/30 p-1 min-w-0"
          >
            {tabs
              .filter((item) => item.visible)
              .map((item) => (
                <button
                  key={item.id}
                  type="button"
                  aria-pressed={tab === item.id}
                  onClick={() => setTab(item.id)}
                  className="min-h-10 rounded-lg px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground aria-pressed:bg-[var(--brand-primary-soft)] aria-pressed:text-[var(--brand-primary-soft-ink)] aria-pressed:font-semibold aria-pressed:shadow-xs"
                >
                  {item.label}
                </button>
              ))}
          </nav>

          {tab === "spaces" && canReadSpaces ? (
            <CustomerSpaces
              accountId={accountId}
              canModerate={canModerateSpaces}
            />
          ) : tab === "commercial_reservations" ||
            tab === "commercial_contracts" ||
            tab === "commercial_payments" ||
            tab === "reservations" ||
            tab === "incidents" ? (
            <CustomerResources
              key={tab}
              accountId={accountId}
              kind={tab}
              title={tabs.find((t) => t.id === tab)?.label ?? tab}
            />
          ) : tab === "audit" && canReadAudit ? (
            <CustomerAudit accountId={accountId} />
          ) : tab === "actions" && canOperate ? (
            <CustomerOperationalActions
              account={record.value}
              canBlock={canBlock}
              canManageRoles={canManageRoles}
              canUpdateAccount={canUpdateAccount}
              onAccountUpdated={() => setReloadNonce((n) => n + 1)}
            />
          ) : (
            <section className="rounded-2xl border bg-card p-5 sm:p-7">
              <h3 className="font-semibold">Datos de la cuenta</h3>
              <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2">
                <div>
                  <dt className="text-muted-foreground">Registro</dt>
                  <dd className="mt-1">
                    {dateFormatter.format(new Date(record.value.created_at))}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Roles</dt>
                  <dd className="mt-1 break-words">
                    {record.value.roles.join(", ").replaceAll("_", " ") ||
                      "Sin roles asignados"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Teléfono</dt>
                  <dd className="mt-1">
                    {record.value.phone ?? "No registrado"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">ID de cuenta</dt>
                  <dd className="mt-1 break-all font-mono">{accountId}</dd>
                </div>
              </dl>
            </section>
          )}
        </>
      ) : (
        <p role="status">No encontramos esta cuenta.</p>
      )}
    </div>
  );
}

function CustomerSpaces({
  accountId,
  canModerate,
}: {
  accountId: string;
  canModerate: boolean;
}) {
  const { snapshot, setGlobalPublicationState } = useAdmin();
  const [cursor, setCursor] = useState("");
  const [reloadNonce, setReloadNonce] = useState(0);
  const [page, setPage] =
    useState<Result<{ items: GlobalAdminPublication[]; nextCursor: string }>>();

  useEffect(() => {
    const controller = new AbortController();
    void adminAdapter
      .listGlobalPublicationPage(
        { owner_id: accountId, page_size: 12, cursor: cursor || undefined },
        controller.signal,
      )
      .then(
        (value) => {
          if (!controller.signal.aborted) {
            setPage({
              status: "ready",
              value: {
                ...value,
                items: value.items.filter(
                  (item) => item.owner_id === accountId,
                ),
              },
            });
          }
        },
        (error) => {
          if (!controller.signal.aborted) {
            setPage({
              status: "error",
              message: requestError(error, "No pudimos cargar los espacios."),
            });
          }
        },
      );
    return () => controller.abort();
  }, [accountId, cursor, reloadNonce]);

  const handleToggleState = async (space: GlobalAdminPublication) => {
    const nextState = space.state === "activa" ? "oculta" : "activa";
    const ok = await setGlobalPublicationState(space.publication_id, {
      state: nextState,
      expected_updated_at: space.updated_at,
      reason: "revision_administrativa",
    });
    if (ok) {
      setReloadNonce((n) => n + 1);
    }
  };

  return (
    <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-4">
      <h3 className="font-semibold">Espacios de la cuenta</h3>
      {snapshot.error && (
        <p
          role="alert"
          className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
        >
          {snapshot.error}
        </p>
      )}
      {!page ? (
        <p role="status" className="text-sm">
          Cargando espacios…
        </p>
      ) : page.status === "error" ? (
        <p role="alert" className="text-sm">
          {page.message}
        </p>
      ) : page.value.items.length ? (
        <>
          <ul className="divide-y">
            {page.value.items.map((space) => (
              <li
                className="flex flex-wrap items-center justify-between gap-3 py-3"
                key={space.publication_id}
              >
                <div className="min-w-0">
                  <p className="break-words font-medium">{space.title}</p>
                  <p className="text-sm text-muted-foreground">
                    {space.category_name}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded-full border px-2.5 py-1 text-xs">
                    {publicationStatus[space.state] ?? space.state}
                  </span>
                  {canModerate &&
                    (space.state === "activa" || space.state === "oculta") && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={snapshot.actionLoading}
                        onClick={() => void handleToggleState(space)}
                      >
                        {space.state === "activa" ? "Ocultar" : "Activar"}
                      </Button>
                    )}
                </div>
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap gap-2">
            {page.value.nextCursor && (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setCursor(page.value.nextCursor);
                  setPage(undefined);
                }}
              >
                Siguiente página
              </Button>
            )}
            {cursor && (
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setCursor("");
                  setPage(undefined);
                }}
              >
                Volver al inicio
              </Button>
            )}
          </div>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          Esta cuenta no tiene espacios en el catálogo administrativo.
        </p>
      )}
    </section>
  );
}

function CustomerResources({
  accountId,
  kind,
  title,
}: {
  accountId: string;
  kind: GlobalAdminAccountResourceKind;
  title: string;
}) {
  const { listGlobalAccountResources } = useAdmin();
  const [cursor, setCursor] = useState("");
  const [page, setPage] = useState<Result<GlobalAdminAccountResourcePage>>();

  useEffect(() => {
    const controller = new AbortController();
    void listGlobalAccountResources(
      accountId,
      kind,
      { page_size: 12, cursor: cursor || undefined },
      controller.signal,
    ).then(
      (value) => {
        if (!controller.signal.aborted) {
          setPage({ status: "ready", value });
        }
      },
      (error) => {
        if (!controller.signal.aborted) {
          setPage({
            status: "error",
            message: requestError(error, `No pudimos cargar ${title.toLowerCase()}.`),
          });
        }
      },
    );
    return () => controller.abort();
  }, [accountId, kind, cursor, title, listGlobalAccountResources]);

  return (
    <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-semibold">{title}</h3>
        {page?.status === "ready" && (
          <span className="text-xs text-muted-foreground tabular-nums">
            Total: {page.value.matching_total}
          </span>
        )}
      </div>

      {!page ? (
        <p role="status" className="text-sm">
          Cargando {title.toLowerCase()}…
        </p>
      ) : page.status === "error" ? (
        <p role="alert" className="text-sm">
          {page.message}
        </p>
      ) : page.value.items.length ? (
        <>
          <ul className="divide-y">
            {page.value.items.map((item) => {
              const origin = getOriginBadge(item, kind);
              const roleLabel = formatRoleInRecord(
                item.role_in_record ?? item.participant_role,
              );
              const displayAmount =
                typeof item.total_charged_clp === "number"
                  ? item.total_charged_clp
                  : typeof item.amount_clp === "number"
                    ? item.amount_clp
                    : null;

              return (
                <li key={item.id} className="py-3.5 space-y-2">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0 space-y-0.5">
                      <p className="font-medium break-words">
                        {item.space_title || item.space_id || item.id}
                      </p>
                      <p className="text-xs text-muted-foreground font-mono break-all">
                        ID: {item.id}
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Badge variant="outline" className={origin.className}>
                        {origin.label}
                      </Badge>
                      <Badge variant="secondary">{item.state}</Badge>
                    </div>
                  </div>

                  <dl className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs text-muted-foreground">
                    <div>
                      <dt className="font-medium text-foreground/80">Fecha</dt>
                      <dd>{dateTimeFormatter.format(new Date(item.occurred_at))}</dd>
                    </div>
                    {roleLabel && (
                      <div>
                        <dt className="font-medium text-foreground/80">Rol</dt>
                        <dd>{roleLabel}</dd>
                      </div>
                    )}
                    {displayAmount !== null && (
                      <div>
                        <dt className="font-medium text-foreground/80">Monto</dt>
                        <dd className="tabular-nums font-medium text-foreground">
                          {clpFormatter.format(displayAmount)}
                        </dd>
                      </div>
                    )}
                    {item.document_status && (
                      <div>
                        <dt className="font-medium text-foreground/80">
                          Documento
                        </dt>
                        <dd>{item.document_status}</dd>
                      </div>
                    )}
                    {(item.provider_payment_id || item.provider_status) && (
                      <div className="col-span-2">
                        <dt className="font-medium text-foreground/80">
                          Pago proveedor
                        </dt>
                        <dd className="break-all font-mono">
                          {[item.provider_payment_id, item.provider_status]
                            .filter(Boolean)
                            .join(" • ")}
                        </dd>
                      </div>
                    )}
                  </dl>
                </li>
              );
            })}
          </ul>

          <div className="flex flex-wrap gap-2">
            {page.value.next_cursor && (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setCursor(page.value.next_cursor);
                  setPage(undefined);
                }}
              >
                Siguiente página
              </Button>
            )}
            {cursor && (
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setCursor("");
                  setPage(undefined);
                }}
              >
                Volver al inicio
              </Button>
            )}
          </div>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          No hay registros en {title.toLowerCase()} para esta cuenta.
        </p>
      )}
    </section>
  );
}

function CustomerAudit({ accountId }: { accountId: string }) {
  const [events, setEvents] = useState<Result<AuditEvent[]>>();

  useEffect(() => {
    const controller = new AbortController();
    const until = new Date().toISOString();
    const from = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString();

    void adminAdapter
      .listGlobalAuditEvents(
        {
          target_account_id: accountId,
          from,
          until,
          page_size: 25,
        },
        controller.signal,
      )
      .then(
        (value) => {
          if (!controller.signal.aborted) {
            setEvents({ status: "ready", value });
          }
        },
        (error) => {
          if (!controller.signal.aborted) {
            setEvents({
              status: "error",
              message: requestError(
                error,
                "No pudimos cargar los eventos de auditoría.",
              ),
            });
          }
        },
      );

    return () => controller.abort();
  }, [accountId]);

  return (
    <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-4">
      <h3 className="font-semibold">Bitácora de auditoría (últimos 30 días)</h3>
      {!events ? (
        <p role="status" className="text-sm">
          Cargando eventos de auditoría…
        </p>
      ) : events.status === "error" ? (
        <p role="alert" className="text-sm">
          {events.message}
        </p>
      ) : events.value.length ? (
        <ul className="divide-y">
          {events.value.map((evt) => (
            <li key={evt.event_id} className="py-3 space-y-1 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-mono text-xs font-medium">
                  {evt.action}
                </span>
                <Badge
                  variant={evt.result === "exito" ? "secondary" : "destructive"}
                >
                  {evt.result}
                </Badge>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span>{dateTimeFormatter.format(new Date(evt.timestamp))}</span>
                {evt.reason_code && <span>Motivo: {evt.reason_code}</span>}
                {evt.resource_type && (
                  <span>
                    Recurso: {evt.resource_type}
                    {evt.resource_id ? ` (${evt.resource_id})` : ""}
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">
          No se registran eventos de auditoría para esta cuenta en los últimos
          30 días.
        </p>
      )}
    </section>
  );
}

function CustomerOperationalActions({
  account,
  canBlock,
  canManageRoles,
  canUpdateAccount,
  onAccountUpdated,
}: {
  account: GlobalAdminAccount;
  canBlock: boolean;
  canManageRoles: boolean;
  canUpdateAccount: boolean;
  onAccountUpdated: () => void;
}) {
  const {
    snapshot,
    clearError,
    setGlobalAccountBlocked,
    revokeGlobalAccountSessions,
    setGlobalAccountRole,
    updateGlobalAccountProfile,
  } = useAdmin();

  const [blockReason, setBlockReason] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [revokeReason, setRevokeReason] = useState<OperationalReason>(
    "revision_administrativa",
  );
  const [roleReason, setRoleReason] =
    useState<OperationalReason>("gestion_operativa");
  const [displayName, setDisplayName] = useState(account.display_name ?? "");
  const [phone, setPhone] = useState(account.phone ?? "");
  const [profileReason, setProfileReason] =
    useState<OperationalReason>("correccion_perfil");

  const handleToggleBlock = async () => {
    clearError();
    setFeedback(null);
    if (!isValidAdminReason(blockReason)) {
      setLocalError(
        "El motivo de bloqueo/desbloqueo debe tener entre 5 y 500 caracteres válidos.",
      );
      return;
    }
    setLocalError(null);
    const nextBlocked = !account.administrative_blocked;
    const ok = await setGlobalAccountBlocked(
      account.account_id,
      nextBlocked,
      blockReason.trim(),
    );
    if (ok) {
      setBlockReason("");
      setFeedback(
        nextBlocked
          ? "Bloqueo administrativo aplicado."
          : "Bloqueo administrativo retirado.",
      );
      onAccountUpdated();
    }
  };

  const handleRevokeSessions = async () => {
    clearError();
    setLocalError(null);
    setFeedback(null);
    const ok = await revokeGlobalAccountSessions(
      account.account_id,
      revokeReason,
    );
    if (ok) {
      setFeedback("Sesiones activas revocadas.");
    }
  };

  const handleToggleRole = async (role: "arrendatario" | "arrendador") => {
    clearError();
    setLocalError(null);
    setFeedback(null);
    const hasRole = account.roles.includes(role);
    const ok = await setGlobalAccountRole(
      account.account_id,
      role,
      !hasRole,
      roleReason,
    );
    if (ok) {
      setFeedback(
        !hasRole ? `Rol ${role} asignado.` : `Rol ${role} retirado.`,
      );
      onAccountUpdated();
    }
  };

  const handleUpdateProfile = async () => {
    clearError();
    setLocalError(null);
    setFeedback(null);
    const expectedUpdatedAt = account.profile_updated_at ?? account.created_at;
    const ok = await updateGlobalAccountProfile(account.account_id, {
      display_name: displayName.trim(),
      phone: phone.trim() || null,
      expected_updated_at: expectedUpdatedAt,
      reason: profileReason,
    });
    if (ok) {
      setFeedback("Perfil de la cuenta actualizado.");
      onAccountUpdated();
    }
  };

  return (
    <section className="rounded-2xl border bg-card p-5 sm:p-7 space-y-6">
      <div>
        <h3 className="font-semibold">Acciones operativas</h3>
        <p className="text-xs text-muted-foreground mt-1">
          Todas las acciones quedan auditadas y pueden requerir confirmación MFA
          reciente (HTTP 428).
        </p>
      </div>

      {(localError || snapshot.error) && (
        <p
          role="alert"
          className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
        >
          {localError ?? snapshot.error}
        </p>
      )}

      {feedback && (
        <p
          role="status"
          className="rounded-lg border border-emerald-300 bg-emerald-50 px-3 py-2 text-xs text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200"
        >
          {feedback}
        </p>
      )}

      {canBlock && (
        <div className="space-y-3 rounded-xl border p-4">
          <h4 className="text-sm font-semibold">
            {account.administrative_blocked
              ? "Retirar bloqueo administrativo"
              : "Aplicar bloqueo administrativo"}
          </h4>
          <div className="space-y-1.5">
            <Label htmlFor="customer-block-reason">
              Motivo (mínimo 5 caracteres)
            </Label>
            <Input
              id="customer-block-reason"
              value={blockReason}
              onChange={(e) => setBlockReason(e.target.value)}
              placeholder="Ej. Revisión operativa de seguridad"
            />
          </div>
          <Button
            type="button"
            variant={account.administrative_blocked ? "outline" : "destructive"}
            disabled={snapshot.actionLoading}
            onClick={() => void handleToggleBlock()}
          >
            {account.administrative_blocked
              ? "Desbloquear cuenta"
              : "Bloquear cuenta"}
          </Button>
        </div>
      )}

      {canUpdateAccount && (
        <div className="space-y-3 rounded-xl border p-4">
          <h4 className="text-sm font-semibold">Revocar sesiones activas</h4>
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-1.5 min-w-48">
              <Label htmlFor="customer-revoke-reason">Motivo</Label>
              <select
                id="customer-revoke-reason"
                value={revokeReason}
                onChange={(e) =>
                  setRevokeReason(e.target.value as OperationalReason)
                }
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="revision_administrativa">
                  Revisión administrativa
                </option>
                <option value="gestion_operativa">Gestión operativa</option>
                <option value="correccion_perfil">Corrección de perfil</option>
              </select>
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={snapshot.actionLoading}
              onClick={() => void handleRevokeSessions()}
            >
              Revocar sesiones
            </Button>
          </div>
        </div>
      )}

      {canManageRoles && (
        <div className="space-y-3 rounded-xl border p-4">
          <h4 className="text-sm font-semibold">Roles de marketplace</h4>
          <div className="space-y-1.5 max-w-xs">
            <Label htmlFor="customer-role-reason">Motivo</Label>
            <select
              id="customer-role-reason"
              value={roleReason}
              onChange={(e) =>
                setRoleReason(e.target.value as OperationalReason)
              }
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              <option value="gestion_operativa">Gestión operativa</option>
              <option value="revision_administrativa">
                Revisión administrativa
              </option>
              <option value="correccion_perfil">Corrección de perfil</option>
            </select>
          </div>
          <div className="flex flex-wrap gap-2">
            {(["arrendatario", "arrendador"] as const).map((role) => {
              const assigned = account.roles.includes(role);
              return (
                <Button
                  key={role}
                  type="button"
                  variant={assigned ? "secondary" : "outline"}
                  disabled={snapshot.actionLoading}
                  onClick={() => void handleToggleRole(role)}
                >
                  {assigned ? `Quitar rol ${role}` : `Asignar rol ${role}`}
                </Button>
              );
            })}
          </div>
        </div>
      )}

      {canUpdateAccount && (
        <div className="space-y-3 rounded-xl border p-4">
          <h4 className="text-sm font-semibold">Actualizar perfil</h4>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="customer-display-name">Nombre visible</Label>
              <Input
                id="customer-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="customer-phone">Teléfono (9 dígitos)</Label>
              <Input
                id="customer-phone"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                placeholder="912345678"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="customer-profile-reason">Motivo</Label>
              <select
                id="customer-profile-reason"
                value={profileReason}
                onChange={(e) =>
                  setProfileReason(e.target.value as OperationalReason)
                }
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="correccion_perfil">Corrección de perfil</option>
                <option value="revision_administrativa">
                  Revisión administrativa
                </option>
                <option value="gestion_operativa">Gestión operativa</option>
              </select>
            </div>
          </div>
          <Button
            type="button"
            disabled={snapshot.actionLoading || !displayName.trim()}
            onClick={() => void handleUpdateProfile()}
          >
            Guardar perfil
          </Button>
        </div>
      )}
    </section>
  );
}
