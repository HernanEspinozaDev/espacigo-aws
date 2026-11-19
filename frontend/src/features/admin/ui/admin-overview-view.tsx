"use client";

import Link from "next/link";
import { useEffect } from "react";
import { BarChart3, RefreshCw, Users } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Button } from "@/components/workspace/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAdmin } from "./use-admin";
import { AdminFeedbackBanners } from "./admin-feedback-banners";

export function AdminOverviewView() {
  const { snapshot, loadOverview, loadGlobalMe, canAccess } = useAdmin();
  const { overview, loading } = snapshot;

  useEffect(() => {
    void loadGlobalMe();
    void loadOverview();
  }, [loadGlobalMe, loadOverview]);

  return (
    <div className="space-y-6">
      <AdminFeedbackBanners />

      {/* Cabecera y acciones rápidas */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">
            Resumen administrativo
          </h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            Consulta las cuentas registradas y cuántas tienen una sesión
            vigente.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void loadOverview()}
            disabled={loading}
            className="gap-1.5"
          >
            <RefreshCw
              className={`size-4 ${loading ? "animate-spin" : ""}`}
              aria-hidden="true"
            />
            <span>Actualizar</span>
          </Button>

          {canAccess("accounts") && (
            <Button asChild size="sm">
              <Link href="/administracion/cuentas">Ver usuarios</Link>
            </Button>
          )}
        </div>
      </div>

      {/* Tarjetas de KPIs Principales */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Cuentas
            </CardTitle>
            <Users
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {overview?.total_accounts ?? "-"}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              {overview?.administrative_blocked ?? 0} bloqueadas
              administrativamente
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Cuentas conectadas
            </CardTitle>
            <Users
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {overview?.connected_accounts ?? "–"}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              Una cuenta por cada usuario con sesión vigente, aunque tenga
              varias.
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Desglose de estados y roles */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <BarChart3 className="size-4 text-primary" aria-hidden="true" />
              <span>Distribución de Cuentas por Estado</span>
            </CardTitle>
            <CardDescription>
              Estado de vida de las cuentas en el sistema
            </CardDescription>
          </CardHeader>
          <CardContent>
            {overview?.by_state && Object.keys(overview.by_state).length > 0 ? (
              <ul className="space-y-2.5">
                {Object.entries(overview.by_state).map(([state, count]) => (
                  <li
                    key={state}
                    className="flex items-center justify-between py-1 border-b border-border/50 text-sm last:border-0"
                  >
                    <span className="capitalize text-muted-foreground">
                      {state.replace(/_/g, " ")}
                    </span>
                    <Badge variant="outline" className="font-mono">
                      {String(count)}
                    </Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground italic py-4 text-center">
                No hay desglose por estado disponible.
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <Users className="size-4 text-primary" aria-hidden="true" />
              <span>Distribución de Cuentas por Rol</span>
            </CardTitle>
            <CardDescription>
              Roles asignados a usuarios en el directorio global
            </CardDescription>
          </CardHeader>
          <CardContent>
            {overview?.by_role && Object.keys(overview.by_role).length > 0 ? (
              <ul className="space-y-2.5">
                {Object.entries(overview.by_role).map(([role, count]) => (
                  <li
                    key={role}
                    className="flex items-center justify-between py-1 border-b border-border/50 text-sm last:border-0"
                  >
                    <span className="capitalize text-muted-foreground">
                      {role.replace(/_/g, " ")}
                    </span>
                    <Badge variant="secondary" className="font-mono">
                      {String(count)}
                    </Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground italic py-4 text-center">
                No hay desglose por rol disponible.
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Metadatos de generación */}
      {overview?.generated_at && (
        <p className="text-xs text-muted-foreground text-right">
          Última actualización global:{" "}
          {new Date(overview.generated_at).toLocaleString("es-CL")}
        </p>
      )}
    </div>
  );
}
