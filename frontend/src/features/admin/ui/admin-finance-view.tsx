"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Building2,
  FileText,
  Landmark,
  Loader2,
  RefreshCw,
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
import { AdminFeedbackBanners } from "./admin-feedback-banners";
import { useAdmin } from "./use-admin";
import type {
  GlobalAdminContract,
  GlobalAdminPayment,
  GlobalAdminReservation,
} from "../domain/types";

function formatClp(raw: number | string): string {
  const num = typeof raw === "number" ? raw : Number(raw);
  if (!Number.isFinite(num)) return `${raw} CLP`;
  return `${new Intl.NumberFormat("es-CL").format(num)} CLP`;
}

export function AdminFinanceView() {
  const {
    snapshot,
    hasAdminCapability,
    listGlobalPayments,
    listGlobalReservations,
    listGlobalContracts,
  } = useAdmin();

  const canReadPayments = hasAdminCapability("payments.read");
  const canReadReservations = hasAdminCapability("reservations.read");
  const canReadContracts = hasAdminCapability("contracts.read");

  const [activeTab, setActiveTab] = useState<
    "payments" | "reservations" | "contracts"
  >("payments");
  const [testModeFilter, setTestModeFilter] = useState<"" | "true" | "false">(
    "",
  );
  const [statusFilter, setStatusFilter] = useState("");
  const [payments, setPayments] = useState<GlobalAdminPayment[]>([]);
  const [reservations, setReservations] = useState<GlobalAdminReservation[]>(
    [],
  );
  const [contracts, setContracts] = useState<GlobalAdminContract[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchFinanceData = useCallback(async () => {
    const tasks: Promise<void>[] = [];
    if (canReadPayments) {
      tasks.push(
        listGlobalPayments({
          test_mode:
            testModeFilter === "" ? undefined : testModeFilter === "true",
          status: statusFilter.trim() || undefined,
        }).then((res) => setPayments(res.items)),
      );
    }
    if (canReadReservations) {
      tasks.push(
        listGlobalReservations().then((res) => setReservations(res.items)),
      );
    }
    if (canReadContracts) {
      tasks.push(listGlobalContracts().then((res) => setContracts(res.items)));
    }
    await Promise.all(tasks);
  }, [
    canReadPayments,
    canReadReservations,
    canReadContracts,
    testModeFilter,
    statusFilter,
    listGlobalPayments,
    listGlobalReservations,
    listGlobalContracts,
  ]);

  const loadData = useCallback(async () => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    setLoading(true);
    try {
      await fetchFinanceData();
    } catch {
      // Errors are surfaced through AdminFeedbackBanners
    } finally {
      setLoading(false);
    }
  }, [snapshot.adminSessionStatus, fetchFinanceData]);

  useEffect(() => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    void fetchFinanceData().catch(() => {
      // Errors are surfaced through AdminFeedbackBanners
    });
  }, [snapshot.adminSessionStatus, fetchFinanceData]);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Finanzas, Reservas y Contratos
          </h1>
          <p className="text-sm text-muted-foreground">
            Conciliación de pagos en CLP, separación Prueba MP vs Comercial,
            reservas y contratos transversales.
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

      <div
        role="tablist"
        aria-label="Secciones financieras y operativas"
        className="flex flex-wrap gap-2 border-b border-border pb-2"
      >
        <Button
          role="tab"
          aria-selected={activeTab === "payments"}
          variant={activeTab === "payments" ? "default" : "outline"}
          size="sm"
          onClick={() => setActiveTab("payments")}
        >
          <Landmark className="mr-2 h-4 w-4" />
          Conciliación de pagos ({payments.length})
        </Button>
        <Button
          role="tab"
          aria-selected={activeTab === "reservations"}
          variant={activeTab === "reservations" ? "default" : "outline"}
          size="sm"
          onClick={() => setActiveTab("reservations")}
        >
          <Building2 className="mr-2 h-4 w-4" />
          Reservas ({reservations.length})
        </Button>
        <Button
          role="tab"
          aria-selected={activeTab === "contracts"}
          variant={activeTab === "contracts" ? "default" : "outline"}
          size="sm"
          onClick={() => setActiveTab("contracts")}
        >
          <FileText className="mr-2 h-4 w-4" />
          Contratos ({contracts.length})
        </Button>
      </div>

      {activeTab === "payments" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Conciliación de pagos (CLP)
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="finance-env-filter">Ambiente</Label>
                <select
                  id="finance-env-filter"
                  value={testModeFilter}
                  onChange={(e) =>
                    setTestModeFilter(e.target.value as "" | "true" | "false")
                  }
                  className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                >
                  <option value="">Todos los ambientes</option>
                  <option value="false">Comercial</option>
                  <option value="true">Prueba MP</option>
                </select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="finance-status-filter">Estado de pago</Label>
                <Input
                  id="finance-status-filter"
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  placeholder="Ej. conciliado, aprobado"
                />
              </div>
            </div>

            {!canReadPayments ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>payments.read</code>.
              </p>
            ) : payments.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No se encontraron pagos para los filtros seleccionados.
              </p>
            ) : (
              <div className="space-y-3">
                {payments.map((p) => (
                  <div
                    key={p.id}
                    className="rounded-lg border border-border p-4 space-y-2"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={p.test_mode ? "secondary" : "default"}>
                          {p.test_mode ? "Prueba MP" : "Comercial"}
                        </Badge>
                        <Badge variant="outline">{p.status}</Badge>
                        <span className="font-mono text-xs text-muted-foreground">
                          {p.provider_payment_id}
                        </span>
                      </div>
                      <span className="font-semibold text-foreground">
                        {formatClp(p.total_charged_clp)}
                      </span>
                    </div>
                    <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-4">
                      <div>Base: {formatClp(p.base_clp)}</div>
                      <div>Comisión: {formatClp(p.commission_clp)}</div>
                      <div>IVA: {formatClp(p.iva_clp)}</div>
                      <div>Fee comprador: {formatClp(p.buyer_fee_clp)}</div>
                    </div>
                    {p.reconciliation_reason && (
                      <p className="text-xs text-foreground">
                        <span className="font-medium">
                          Motivo de conciliación:
                        </span>{" "}
                        {p.reconciliation_reason}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {activeTab === "reservations" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Reservas transversales</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {!canReadReservations ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>reservations.read</code>.
              </p>
            ) : reservations.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay reservas registradas para mostrar.
              </p>
            ) : (
              reservations.map((r) => (
                <div
                  key={r.id}
                  className="flex flex-col gap-2 rounded-lg border border-border p-4 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline">{r.status}</Badge>
                      <span className="font-medium text-foreground">
                        {r.space_title}
                      </span>
                    </div>
                    <p className="font-mono text-xs text-muted-foreground">
                      Reserva: {r.id}
                    </p>
                  </div>
                  <div className="text-sm font-semibold text-foreground">
                    {formatClp(r.subtotal_clp)}
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      )}

      {activeTab === "contracts" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Contratos comerciales transversales
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {!canReadContracts ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>contracts.read</code>.
              </p>
            ) : contracts.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay contratos comerciales para mostrar.
              </p>
            ) : (
              contracts.map((c) => (
                <div
                  key={c.id}
                  className="flex flex-col gap-2 rounded-lg border border-border p-4 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline">{c.status}</Badge>
                      <span className="font-mono text-xs text-foreground">
                        {c.id}
                      </span>
                    </div>
                    <p className="font-mono text-xs text-muted-foreground">
                      Cotización: {c.quote_id} · {c.document_status}
                    </p>
                  </div>
                  <div className="text-sm font-semibold text-foreground">
                    {formatClp(c.subtotal_clp)}
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
