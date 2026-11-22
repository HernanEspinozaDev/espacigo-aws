"use client";

import { useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  CircleDollarSign,
  ExternalLink,
  Loader2,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { useMercadoPago } from "./use-mercado-pago";

const callbackMessages: Record<
  string,
  { text: string; tone: "success" | "error" }
> = {
  connected: {
    text: "La cuenta de Mercado Pago quedó vinculada.",
    tone: "success",
  },
  mode_mismatch: {
    text: "La cuenta de Mercado Pago no corresponde al entorno de prueba configurado.",
    tone: "error",
  },
  denied: {
    text: "No se autorizó el acceso. Puedes volver a intentarlo.",
    tone: "error",
  },
  invalid_state: {
    text: "El enlace expiró o ya se utilizó. Inicia una nueva vinculación.",
    tone: "error",
  },
  exchange_failed: {
    text: "Mercado Pago no pudo completar la vinculación. Inténtalo nuevamente.",
    tone: "error",
  },
  not_configured: {
    text: "La conexión de Mercado Pago aún no está habilitada.",
    tone: "error",
  },
};

export function MercadoPagoConnectionCard({
  callback,
}: {
  callback: string | null;
}) {
  const { auth, snapshot, startAuthorization, reload } = useMercadoPago();
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState(false);
  const notice = callback ? callbackMessages[callback] : undefined;
  const connection = snapshot.connection;
  const isHost =
    auth.status === "authenticated" && auth.roles.includes("arrendador");

  async function connect() {
    setStartError(false);
    setStarting(true);
    try {
      await startAuthorization();
    } catch {
      setStartError(true);
    } finally {
      setStarting(false);
    }
  }

  return (
    <Card aria-labelledby="mercado-pago-title">
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <CircleDollarSign
              className="size-5 text-primary"
              aria-hidden="true"
            />
            <CardTitle id="mercado-pago-title">Mercado Pago</CardTitle>
          </div>
          {connection?.connected ? (
            <Badge variant="secondary" className="gap-1">
              <ShieldCheck
                className="size-3.5 text-emerald-600"
                aria-hidden="true"
              />
              Conectado
            </Badge>
          ) : null}
        </div>
        <CardDescription>
          Vincula tu cuenta para recibir pagos de tus reservas mediante Mercado
          Pago.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {notice ? (
          <div
            role={notice.tone === "success" ? "status" : "alert"}
            className={`flex items-start gap-2 rounded-md border p-3 text-sm ${
              notice.tone === "success"
                ? "border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200"
                : "border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200"
            }`}
          >
            {notice.tone === "success" ? (
              <CheckCircle2
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
            ) : (
              <AlertCircle
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
            )}
            {notice.text}
          </div>
        ) : null}

        {!isHost ? (
          <p role="status" className="text-sm text-muted-foreground">
            Esta configuración está disponible para cuentas anfitrionas.
          </p>
        ) : snapshot.loading && !connection ? (
          <p
            role="status"
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            Consultando estado de conexión…
          </p>
        ) : snapshot.error === "request_failed" && !connection ? (
          <div className="space-y-3">
            <p role="alert" className="text-sm text-destructive">
              No fue posible consultar la conexión. Vuelve a intentarlo.
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void reload()}
            >
              <RefreshCw className="mr-2 size-4" aria-hidden="true" />
              Reintentar
            </Button>
          </div>
        ) : connection?.connected ? (
          <div className="flex flex-col gap-4 rounded-lg border bg-muted/30 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="font-medium">Tu cuenta está vinculada</p>
              <p className="mt-1 text-sm text-muted-foreground">
                Mercado Pago autorizó a EspaciGo para gestionar los pagos de tus
                reservas.
              </p>
            </div>
            <Button
              type="button"
              variant="outline"
              onClick={connect}
              disabled={starting || !connection.authorizationAvailable}
            >
              {starting ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <RefreshCw className="mr-2 size-4" aria-hidden="true" />
              )}
              Volver a autorizar
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-4 rounded-lg border border-dashed p-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="font-medium">Aún no has vinculado Mercado Pago</p>
              <p className="mt-1 text-sm text-muted-foreground">
                Te enviaremos a Mercado Pago para autorizar la conexión de forma
                segura.
              </p>
            </div>
            <Button
              type="button"
              onClick={connect}
              disabled={
                starting ||
                snapshot.loading ||
                !connection?.authorizationAvailable
              }
            >
              {starting ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <ExternalLink className="mr-2 size-4" aria-hidden="true" />
              )}
              Vincular Mercado Pago
            </Button>
          </div>
        )}

        {(!connection?.authorizationAvailable && connection) ||
        snapshot.error === "not_configured" ? (
          <p role="status" className="text-sm text-muted-foreground">
            La vinculación no está habilitada en este entorno por ahora.
          </p>
        ) : null}
        {startError || (snapshot.error === "request_failed" && connection) ? (
          <p role="alert" className="text-sm text-destructive">
            No se pudo iniciar la conexión. Reintenta; la autorización anterior
            sigue vigente.
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          EspaciGo no verá ni guardará tu contraseña de Mercado Pago.
        </p>
      </CardContent>
    </Card>
  );
}
