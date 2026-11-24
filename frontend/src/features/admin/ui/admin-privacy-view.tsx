"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, Lock, Plus, RefreshCw, ShieldCheck } from "lucide-react";
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
import { isValidAdminReason, isValidUUID } from "../domain/admin-rules";
import { AdminFeedbackBanners } from "./admin-feedback-banners";
import { useAdmin } from "./use-admin";
import type {
  GlobalAdminKycCase,
  GlobalAdminPrivacyRequest,
  GlobalAdminPrivacyRequestType,
} from "../domain/types";

export function AdminPrivacyView() {
  const {
    snapshot,
    hasAdminCapability,
    listGlobalPrivacyRequests,
    createGlobalPrivacyRequest,
    listGlobalKycCases,
  } = useAdmin();

  const canManagePrivacy = hasAdminCapability("privacy.requests.manage");
  const canReadKyc = hasAdminCapability("kyc.case.read");

  const [privacyRequests, setPrivacyRequests] = useState<
    GlobalAdminPrivacyRequest[]
  >([]);
  const [kycCases, setKycCases] = useState<GlobalAdminKycCase[]>([]);
  const [kycStatusFilter, setKycStatusFilter] = useState("");
  const [loading, setLoading] = useState(false);

  // New ARCO+ request form state
  const [accountId, setAccountId] = useState("");
  const [rightType, setRightType] =
    useState<GlobalAdminPrivacyRequestType>("acceso");
  const [reason, setReason] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const fetchPrivacyData = useCallback(async () => {
    const tasks: Promise<void>[] = [];
    if (canManagePrivacy) {
      tasks.push(
        listGlobalPrivacyRequests().then((res) =>
          setPrivacyRequests(res.items),
        ),
      );
    }
    if (canReadKyc) {
      tasks.push(
        listGlobalKycCases({
          status: kycStatusFilter || undefined,
        }).then((res) => setKycCases(res.items)),
      );
    }
    await Promise.all(tasks);
  }, [
    canManagePrivacy,
    canReadKyc,
    kycStatusFilter,
    listGlobalPrivacyRequests,
    listGlobalKycCases,
  ]);

  const loadData = useCallback(async () => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    setLoading(true);
    try {
      await fetchPrivacyData();
    } catch {
      // Surfaced via AdminFeedbackBanners
    } finally {
      setLoading(false);
    }
  }, [snapshot.adminSessionStatus, fetchPrivacyData]);

  useEffect(() => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    void fetchPrivacyData().catch(() => {
      // Surfaced via AdminFeedbackBanners
    });
  }, [snapshot.adminSessionStatus, fetchPrivacyData]);

  const handleCreatePrivacyRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!isValidUUID(accountId.trim())) {
      setFormError("Ingresa un UUID válido de cuenta titular.");
      return;
    }
    if (!isValidAdminReason(reason)) {
      setFormError("El motivo debe tener al menos 5 caracteres válidos.");
      return;
    }
    setSubmitting(true);
    try {
      const created = await createGlobalPrivacyRequest({
        account_id: accountId.trim(),
        right_type: rightType,
        reason: reason.trim(),
      });
      setPrivacyRequests((prev) => [created, ...prev]);
      setAccountId("");
      setReason("");
    } catch {
      // Handled by facade / AdminFeedbackBanners
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Privacidad ARCO+ y Verificaciones KYC
          </h1>
          <p className="text-sm text-muted-foreground">
            Gestión de derechos Ley 21.719 (acceso, rectificación, supresión,
            oposición, portabilidad y bloqueo) y resumen de casos KYC.
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
              <Lock className="h-4 w-4" />
              Solicitudes ARCO+ (Ley 21.719)
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {canManagePrivacy && (
              <form
                onSubmit={(e) => void handleCreatePrivacyRequest(e)}
                className="space-y-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-medium text-foreground">
                  Registrar nueva solicitud ARCO+
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="privacy-account-id">
                      Cuenta titular (UUID)
                    </Label>
                    <Input
                      id="privacy-account-id"
                      value={accountId}
                      onChange={(e) => setAccountId(e.target.value)}
                      placeholder="110e8400-e29b-41d4-a716-446655440000"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="privacy-request-type">Derecho ARCO+</Label>
                    <select
                      id="privacy-request-type"
                      value={rightType}
                      onChange={(e) =>
                        setRightType(
                          e.target.value as GlobalAdminPrivacyRequestType,
                        )
                      }
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="acceso">Acceso</option>
                      <option value="rectificacion">Rectificación</option>
                      <option value="supresion">Supresión</option>
                      <option value="oposicion">Oposición</option>
                      <option value="portabilidad">Portabilidad</option>
                      <option value="bloqueo">Bloqueo</option>
                    </select>
                  </div>
                </div>
                <div className="space-y-1">
                  <Label htmlFor="privacy-reason">Motivo o respaldo</Label>
                  <Input
                    id="privacy-reason"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="Detalle de la solicitud del titular (mín. 5 caracteres)"
                  />
                </div>
                {formError && (
                  <p role="alert" className="text-xs text-destructive">
                    {formError}
                  </p>
                )}
                <Button type="submit" size="sm" disabled={submitting}>
                  <Plus className="mr-1 h-4 w-4" />
                  {submitting ? "Registrando..." : "Registrar solicitud"}
                </Button>
              </form>
            )}

            {!canManagePrivacy ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>privacy.requests.manage</code>.
              </p>
            ) : privacyRequests.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay solicitudes ARCO+ registradas.
              </p>
            ) : (
              <div className="space-y-2">
                {privacyRequests.map((req) => (
                  <div
                    key={req.id}
                    className="rounded-lg border border-border p-3 space-y-1"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Badge variant="default">{req.right_type}</Badge>
                        <Badge variant="outline">{req.status}</Badge>
                      </div>
                      <span className="font-mono text-xs text-muted-foreground">
                        {req.account_id}
                      </span>
                    </div>
                    <p className="text-xs text-foreground">{req.reason}</p>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldCheck className="h-4 w-4" />
              Resumen de casos KYC (estado primero)
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1">
              <Label htmlFor="kyc-status-filter">Filtrar por estado KYC</Label>
              <select
                id="kyc-status-filter"
                value={kycStatusFilter}
                onChange={(e) => setKycStatusFilter(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="">Todos los estados</option>
                <option value="pendiente">Pendiente</option>
                <option value="en_revision">En revisión</option>
                <option value="aprobado">Aprobado</option>
                <option value="rechazado">Rechazado</option>
              </select>
            </div>

            {!canReadKyc ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>kyc.case.read</code>.
              </p>
            ) : kycCases.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay casos KYC para los filtros seleccionados.
              </p>
            ) : (
              <div className="space-y-2">
                {kycCases.map((item) => (
                  <div
                    key={item.id}
                    className="rounded-lg border border-border p-3 space-y-1"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Badge
                          variant={
                            item.status === "aprobado"
                              ? "default"
                              : item.status === "rechazado"
                                ? "destructive"
                                : "secondary"
                          }
                        >
                          {item.status}
                        </Badge>
                        <Badge variant="outline">
                          Decisión: {item.decision_status}
                        </Badge>
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {item.environment} · {item.id_country}
                      </span>
                    </div>
                    <p className="font-mono text-xs text-muted-foreground">
                      Cuenta: {item.account_id}
                    </p>
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
