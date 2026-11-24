"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  CheckCircle2,
  CheckSquare,
  LifeBuoy,
  Loader2,
  Plus,
  RefreshCw,
  ShieldAlert,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import {
  isRecentAuthRequired,
  isValidAdminReason,
  isValidUUID,
} from "../domain/admin-rules";
import { AdminFeedbackBanners } from "./admin-feedback-banners";
import { useAdmin } from "./use-admin";
import type {
  GlobalAdminApproval,
  GlobalAdminApprovalActionType,
  GlobalAdminApprovalDecision,
  GlobalAdminSupportTicket,
  GlobalAdminSupportTicketCategory,
  GlobalAdminSupportTicketPriority,
} from "../domain/types";

const ACTION_TYPE_LABELS: Record<GlobalAdminApprovalActionType, string> = {
  refund_request: "Reembolso de pago",
  privacy_export: "Exportación de privacidad",
  staff_role_grant: "Cambio de rol administrativo",
};

export function AdminApprovalsView() {
  const {
    snapshot,
    hasAdminCapability,
    listGlobalApprovals,
    createGlobalApproval,
    decideGlobalApproval,
    listGlobalSupportTickets,
    createGlobalSupportTicket,
  } = useAdmin();

  const canReadApprovals = hasAdminCapability("approvals.read");
  const canRequestApprovals = hasAdminCapability("approvals.request");
  const canDecideApprovals = hasAdminCapability("approvals.decide");
  const canReadTickets = hasAdminCapability("support.tickets.read");
  const canManageTickets = hasAdminCapability("support.tickets.manage");

  const currentAdminId = snapshot.adminSession?.account_id ?? null;

  const [approvals, setApprovals] = useState<GlobalAdminApproval[]>([]);
  const [tickets, setTickets] = useState<GlobalAdminSupportTicket[]>([]);
  const [loading, setLoading] = useState(false);

  // Create approval request form
  const [actionType, setActionType] =
    useState<GlobalAdminApprovalActionType>("refund_request");
  const [resourceType, setResourceType] = useState("payment");
  const [resourceId, setResourceId] = useState("");
  const [amountClp, setAmountClp] = useState("");
  const [approvalReason, setApprovalReason] = useState("");
  const [approvalFormError, setApprovalFormError] = useState<string | null>(
    null,
  );
  const [submittingApproval, setSubmittingApproval] = useState(false);

  // Decision inputs per approval ID
  const [decisionReasons, setDecisionReasons] = useState<
    Record<string, string>
  >({});
  const [decisionError, setDecisionError] = useState<string | null>(null);
  const [decidingId, setDecidingId] = useState<string | null>(null);

  // Create support ticket form
  const [ticketAccountId, setTicketAccountId] = useState("");
  const [ticketSubject, setTicketSubject] = useState("");
  const [ticketCategory, setTicketCategory] =
    useState<GlobalAdminSupportTicketCategory>("pago");
  const [ticketPriority, setTicketPriority] =
    useState<GlobalAdminSupportTicketPriority>("media");
  const [ticketCaseRef, setTicketCaseRef] = useState("");
  const [ticketFormError, setTicketFormError] = useState<string | null>(null);
  const [submittingTicket, setSubmittingTicket] = useState(false);

  const fetchApprovalsData = useCallback(async () => {
    const tasks: Promise<void>[] = [];
    if (canReadApprovals) {
      tasks.push(listGlobalApprovals().then((res) => setApprovals(res.items)));
    }
    if (canReadTickets) {
      tasks.push(
        listGlobalSupportTickets().then((res) => setTickets(res.items)),
      );
    }
    await Promise.all(tasks);
  }, [
    canReadApprovals,
    canReadTickets,
    listGlobalApprovals,
    listGlobalSupportTickets,
  ]);

  const loadData = useCallback(async () => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    setLoading(true);
    try {
      await fetchApprovalsData();
    } catch {
      // Surfaced via AdminFeedbackBanners
    } finally {
      setLoading(false);
    }
  }, [snapshot.adminSessionStatus, fetchApprovalsData]);

  useEffect(() => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    void fetchApprovalsData().catch(() => {
      // Surfaced via AdminFeedbackBanners
    });
  }, [snapshot.adminSessionStatus, fetchApprovalsData]);

  const handleCreateApproval = async (e: React.FormEvent) => {
    e.preventDefault();
    setApprovalFormError(null);
    if (!resourceType.trim() || !resourceId.trim()) {
      setApprovalFormError("Ingresa el tipo y el ID del recurso objetivo.");
      return;
    }
    if (!isValidAdminReason(approvalReason)) {
      setApprovalFormError(
        "El motivo debe tener al menos 5 caracteres válidos.",
      );
      return;
    }
    setSubmittingApproval(true);
    try {
      const created = await createGlobalApproval({
        action_type: actionType,
        resource_type: resourceType.trim(),
        resource_id: resourceId.trim(),
        reason: approvalReason.trim(),
        payload: amountClp.trim()
          ? { amount_clp: amountClp.trim() }
          : undefined,
      });
      setApprovals((prev) => [created, ...prev]);
      setResourceId("");
      setAmountClp("");
      setApprovalReason("");
    } catch {
      // Handled by facade
    } finally {
      setSubmittingApproval(false);
    }
  };

  const handleDecision = async (
    approval: GlobalAdminApproval,
    decision: GlobalAdminApprovalDecision,
  ) => {
    setDecisionError(null);
    if (currentAdminId && approval.requester_id === currentAdminId) {
      setDecisionError(
        "Auto-aprobación bloqueada: el solicitante no puede decidir su propia solicitud.",
      );
      return;
    }
    const reason = (decisionReasons[approval.id] ?? "").trim();
    if (!isValidAdminReason(reason)) {
      setDecisionError(
        "Ingresa un motivo de decisión de al menos 5 caracteres válidos.",
      );
      return;
    }
    setDecidingId(approval.id);
    try {
      const updated = await decideGlobalApproval(approval.id, {
        decision,
        decision_reason: reason,
        expected_version: approval.version,
      });
      setApprovals((prev) =>
        prev.map((item) => (item.id === updated.id ? updated : item)),
      );
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        setDecisionError(
          "Se requiere verificación MFA reciente (428) para registrar la decisión.",
        );
      } else {
        setDecisionError(
          err instanceof Error
            ? err.message
            : "No pudimos registrar la decisión de doble control.",
        );
      }
    } finally {
      setDecidingId(null);
    }
  };

  const handleCreateTicket = async (e: React.FormEvent) => {
    e.preventDefault();
    setTicketFormError(null);
    const trimmedAccountId = ticketAccountId.trim();
    if (trimmedAccountId && !isValidUUID(trimmedAccountId)) {
      setTicketFormError("Ingresa un UUID válido de cuenta asociada.");
      return;
    }
    if (ticketSubject.trim().length < 3) {
      setTicketFormError("El asunto debe tener al menos 3 caracteres.");
      return;
    }
    setSubmittingTicket(true);
    try {
      const created = await createGlobalSupportTicket({
        account_id: trimmedAccountId || undefined,
        subject: ticketSubject.trim(),
        category: ticketCategory,
        priority: ticketPriority,
        case_ref: ticketCaseRef.trim() || undefined,
      });
      setTickets((prev) => [created, ...prev]);
      setTicketAccountId("");
      setTicketSubject("");
      setTicketCaseRef("");
    } catch {
      // Handled by facade
    } finally {
      setSubmittingTicket(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Aprobaciones de Doble Control y Soporte
          </h1>
          <p className="text-sm text-muted-foreground">
            Cola de doble control para reembolsos, exportaciones y roles con
            bloqueo de auto-aprobación y bandeja de tickets.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void loadData()}
          disabled={loading}
        >
          {loading ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 h-4 w-4" />
          )}
          Actualizar
        </Button>
      </div>

      <AdminFeedbackBanners />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CheckSquare className="h-4 w-4" />
              Cola de doble control
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {canRequestApprovals && (
              <form
                onSubmit={(e) => void handleCreateApproval(e)}
                className="space-y-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-medium text-foreground">
                  Solicitar operación de doble control
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="approval-action-type">
                      Tipo de operación
                    </Label>
                    <select
                      id="approval-action-type"
                      value={actionType}
                      onChange={(e) =>
                        setActionType(
                          e.target.value as GlobalAdminApprovalActionType,
                        )
                      }
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="refund_request">Reembolso de pago</option>
                      <option value="privacy_export">
                        Exportación de privacidad
                      </option>
                      <option value="staff_role_grant">
                        Cambio de rol administrativo
                      </option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="approval-resource-type">
                      Tipo de recurso
                    </Label>
                    <Input
                      id="approval-resource-type"
                      value={resourceType}
                      onChange={(e) => setResourceType(e.target.value)}
                      placeholder="payment, privacy_request, account"
                    />
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-3">
                  <div className="space-y-1">
                    <Label htmlFor="approval-target-resource">
                      Recurso objetivo (ID)
                    </Label>
                    <Input
                      id="approval-target-resource"
                      value={resourceId}
                      onChange={(e) => setResourceId(e.target.value)}
                      placeholder="ID de pago, solicitud o cuenta"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="approval-amount-clp">
                      Monto CLP (opcional)
                    </Label>
                    <Input
                      id="approval-amount-clp"
                      value={amountClp}
                      onChange={(e) => setAmountClp(e.target.value)}
                      placeholder="Ej. 50000"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="approval-reason">Motivo</Label>
                    <Input
                      id="approval-reason"
                      value={approvalReason}
                      onChange={(e) => setApprovalReason(e.target.value)}
                      placeholder="Justificación operativa"
                    />
                  </div>
                </div>
                {approvalFormError && (
                  <p role="alert" className="text-xs text-destructive">
                    {approvalFormError}
                  </p>
                )}
                <Button type="submit" size="sm" disabled={submittingApproval}>
                  <Plus className="mr-1 h-4 w-4" />
                  {submittingApproval ? "Enviando..." : "Crear solicitud"}
                </Button>
              </form>
            )}

            {decisionError && (
              <div
                role="alert"
                className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive"
              >
                {decisionError}
              </div>
            )}

            {!canReadApprovals ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>approvals.read</code>.
              </p>
            ) : approvals.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay solicitudes en la cola de doble control.
              </p>
            ) : (
              <div className="space-y-3">
                {approvals.map((app) => {
                  const isSelfRequest =
                    Boolean(currentAdminId) &&
                    app.requester_id === currentAdminId;
                  const isPending = app.status === "pending";
                  const payloadAmount =
                    app.payload && typeof app.payload.amount_clp === "string"
                      ? app.payload.amount_clp
                      : null;

                  return (
                    <div
                      key={app.id}
                      className="rounded-lg border border-border p-3 space-y-2"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge
                            variant={
                              app.status === "rejected"
                                ? "destructive"
                                : app.status === "approved"
                                  ? "outline"
                                  : "secondary"
                            }
                          >
                            {app.status}
                          </Badge>
                          <span className="text-sm font-medium text-foreground">
                            {ACTION_TYPE_LABELS[app.action_type]}
                          </span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          v{app.version}
                        </span>
                      </div>
                      <p className="text-xs text-foreground">{app.reason}</p>
                      <p className="font-mono text-xs text-muted-foreground">
                        Recurso: {app.resource_type}/{app.resource_id}
                        {payloadAmount ? ` · ${payloadAmount} CLP` : ""}
                      </p>

                      {isPending && canDecideApprovals && (
                        <div className="space-y-2 pt-2 border-t border-border">
                          {isSelfRequest ? (
                            <p className="flex items-center gap-1.5 text-xs font-medium text-amber-700 dark:text-amber-400">
                              <ShieldAlert className="h-4 w-4 shrink-0" />
                              Auto-aprobación bloqueada: el solicitante no puede
                              aprobar su propia solicitud.
                            </p>
                          ) : (
                            <>
                              <div className="space-y-1">
                                <Label htmlFor={`decision-reason-${app.id}`}>
                                  Motivo de decisión (obligatorio)
                                </Label>
                                <Input
                                  id={`decision-reason-${app.id}`}
                                  value={decisionReasons[app.id] ?? ""}
                                  onChange={(e) =>
                                    setDecisionReasons((prev) => ({
                                      ...prev,
                                      [app.id]: e.target.value,
                                    }))
                                  }
                                  placeholder="Motivo de aprobación o rechazo"
                                />
                              </div>
                              <div className="flex gap-2">
                                <Button
                                  size="sm"
                                  disabled={decidingId === app.id}
                                  onClick={() =>
                                    void handleDecision(app, "approved")
                                  }
                                >
                                  <CheckCircle2 className="mr-1 h-4 w-4" />
                                  Aprobar
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  disabled={decidingId === app.id}
                                  onClick={() =>
                                    void handleDecision(app, "rejected")
                                  }
                                >
                                  <XCircle className="mr-1 h-4 w-4" />
                                  Rechazar
                                </Button>
                              </div>
                            </>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <LifeBuoy className="h-4 w-4" />
              Bandeja de tickets de soporte
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {canManageTickets && (
              <form
                onSubmit={(e) => void handleCreateTicket(e)}
                className="space-y-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-medium text-foreground">
                  Crear ticket operativo
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="ticket-account-id">Cuenta (UUID)</Label>
                    <Input
                      id="ticket-account-id"
                      value={ticketAccountId}
                      onChange={(e) => setTicketAccountId(e.target.value)}
                      placeholder="110e8400-e29b-41d4-a716-446655440000"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="ticket-subject">Asunto</Label>
                    <Input
                      id="ticket-subject"
                      value={ticketSubject}
                      onChange={(e) => setTicketSubject(e.target.value)}
                      placeholder="Resumen del incidente"
                    />
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="ticket-category">Categoría</Label>
                    <select
                      id="ticket-category"
                      value={ticketCategory}
                      onChange={(e) =>
                        setTicketCategory(
                          e.target.value as GlobalAdminSupportTicketCategory,
                        )
                      }
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="pago">Pago</option>
                      <option value="reserva">Reserva</option>
                      <option value="contrato">Contrato</option>
                      <option value="kyc">KYC</option>
                      <option value="privacidad">Privacidad</option>
                      <option value="cuenta">Cuenta</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="ticket-priority">Prioridad</Label>
                    <select
                      id="ticket-priority"
                      value={ticketPriority}
                      onChange={(e) =>
                        setTicketPriority(
                          e.target.value as GlobalAdminSupportTicketPriority,
                        )
                      }
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="baja">Baja</option>
                      <option value="media">Media</option>
                      <option value="alta">Alta</option>
                      <option value="critica">Crítica</option>
                    </select>
                  </div>
                </div>
                <div className="space-y-1">
                  <Label htmlFor="ticket-case-ref">
                    Referencia de caso (opcional)
                  </Label>
                  <Input
                    id="ticket-case-ref"
                    value={ticketCaseRef}
                    onChange={(e) => setTicketCaseRef(e.target.value)}
                    placeholder="Ej. CASE-2026-001"
                  />
                </div>
                {ticketFormError && (
                  <p role="alert" className="text-xs text-destructive">
                    {ticketFormError}
                  </p>
                )}
                <Button type="submit" size="sm" disabled={submittingTicket}>
                  <Plus className="mr-1 h-4 w-4" />
                  {submittingTicket ? "Creando..." : "Crear ticket"}
                </Button>
              </form>
            )}

            {!canReadTickets ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>support.tickets.read</code>.
              </p>
            ) : tickets.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay tickets de soporte registrados.
              </p>
            ) : (
              <div className="space-y-2">
                {tickets.map((t) => (
                  <div
                    key={t.id}
                    className="rounded-lg border border-border p-3 space-y-1"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline">{t.status}</Badge>
                        <Badge
                          variant={
                            t.priority === "critica" || t.priority === "alta"
                              ? "destructive"
                              : "secondary"
                          }
                        >
                          {t.priority}
                        </Badge>
                        <span className="text-sm font-medium text-foreground">
                          {t.subject}
                        </span>
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {t.category}
                      </span>
                    </div>
                    {t.case_ref && (
                      <p className="text-xs text-muted-foreground">
                        Ref: {t.case_ref}
                      </p>
                    )}
                    {t.account_id && (
                      <div className="flex flex-wrap items-center gap-3 pt-1">
                        <Link
                          href={`/administracion/cuentas/${t.account_id}`}
                          className="text-xs font-medium text-primary underline-offset-4 hover:underline"
                        >
                          Abrir Ficha 360
                        </Link>
                        <Link
                          href={`/administracion/cuentas/${t.account_id}?assisted_case=${encodeURIComponent(t.case_ref || t.id)}`}
                          className="text-xs font-medium text-primary underline-offset-4 hover:underline"
                        >
                          Abrir vista del cliente
                        </Link>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
