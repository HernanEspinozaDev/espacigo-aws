"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { ArrowUpRight, Clock3, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { profileEditHref } from "@/features/host-listings/domain/publish-guide";
import type { IdentityProfileState } from "../domain/identity-profile";
import { cn } from "@/lib/utils";
import { IdentityVerifiedBadge } from "./identity-verified-badge";
import { useIdentityProfile } from "./use-identity-profile";

const STATE_LABEL: Record<IdentityProfileState, string> = {
  incomplete: "Perfil incompleto",
  ready: "Listo para verificar",
  in_progress: "Verificación en curso",
  in_review: "Identidad en revisión",
  needs_correction: "Requiere corrección",
  verified: "Verificado",
  expired: "Verificación vencida",
  revoked: "Verificación revocada",
};

export function IdentityVerificationCard({
  returnTo,
  refreshOnMount,
}: {
  returnTo: string | null;
  refreshOnMount: boolean;
}) {
  const { snapshot, startVerification, refreshVerification, reload } =
    useIdentityProfile();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const autoRefreshStarted = useRef(false);
  const status = snapshot.summary?.state ?? "incomplete";
  const verified =
    status === "verified" && !snapshot.loading && !snapshot.error;
  const verifiedUntil = verified ? snapshot.summary?.verified_until : undefined;
  const canStartVerification =
    !snapshot.loading &&
    !snapshot.error &&
    snapshot.summary?.complete &&
    ["ready", "in_progress", "needs_correction", "expired", "revoked"].includes(
      status,
    );

  useEffect(() => {
    if (
      !refreshOnMount ||
      !snapshot.accountId ||
      snapshot.loading ||
      autoRefreshStarted.current
    )
      return;
    autoRefreshStarted.current = true;
    const url = new URL(window.location.href);
    url.searchParams.delete("refresh_identity");
    window.history.replaceState(
      window.history.state,
      "",
      url.pathname + url.search + url.hash,
    );
    setBusy(true);
    void refreshVerification()
      .catch(() =>
        setError("No pudimos actualizar el resultado de verificación."),
      )
      .finally(() => setBusy(false));
  }, [
    refreshOnMount,
    refreshVerification,
    snapshot.accountId,
    snapshot.loading,
  ]);

  async function handleVerification() {
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      const result = await startVerification(returnTo);
      const url = new URL(result.url);
      if (url.protocol !== "https:")
        throw new Error("URL de verificación inválida");
      window.location.assign(url.toString());
    } catch {
      setError(
        "No pudimos iniciar la verificación. Revisa tu conexión e inténtalo nuevamente.",
      );
      setBusy(false);
    }
  }

  async function handleRefreshVerification() {
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      await refreshVerification();
      setMessage("Estado de verificación actualizado.");
    } catch {
      setError("No pudimos actualizar el resultado. Inténtalo nuevamente.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card
      id="identity-verification"
      aria-labelledby="identity-verification-title"
      className={cn(
        "scroll-mt-24 overflow-hidden rounded-2xl",
        verified && "border-emerald-200 dark:border-emerald-900",
      )}
    >
      <CardHeader className="gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div
            className={cn(
              "flex size-12 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary",
              verified &&
                "bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
            )}
          >
            <ShieldCheck className="size-6" aria-hidden="true" />
          </div>
          <div data-testid="identity-profile-state" role="status">
            {verified ? (
              <IdentityVerifiedBadge />
            ) : (
              <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted px-3 py-1 text-xs font-medium text-muted-foreground">
                {(status === "in_progress" || status === "in_review") && (
                  <Clock3 className="size-3.5" aria-hidden="true" />
                )}
                {snapshot.loading
                  ? "Consultando estado…"
                  : snapshot.error
                    ? "Estado no disponible"
                    : STATE_LABEL[status]}
              </span>
            )}
          </div>
        </div>
        <div className="space-y-2">
          <CardTitle id="identity-verification-title" className="text-xl">
            Verificación de identidad
          </CardTitle>
          <CardDescription>
            {verified
              ? "Confirmamos tu identidad con tu cédula chilena."
              : status === "in_review"
                ? "Estamos revisando el resultado de tu verificación."
                : snapshot.summary?.complete
                  ? "Confirma tu identidad con tu cédula chilena en Didit."
                  : "Completa los datos de tu perfil para iniciar este paso."}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {verifiedUntil && (
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 rounded-lg bg-emerald-50 px-3.5 py-2.5 text-sm text-emerald-900 dark:bg-emerald-950/60 dark:text-emerald-300">
            <span className="font-medium">Vigente hasta</span>
            <time dateTime={verifiedUntil}>
              {new Date(verifiedUntil).toLocaleDateString("es-CL", {
                day: "numeric",
                month: "long",
                year: "numeric",
                timeZone: "America/Santiago",
              })}
            </time>
          </div>
        )}
        <div className="flex flex-wrap gap-2">
          {!snapshot.loading &&
          !snapshot.error &&
          !snapshot.summary?.complete ? (
            <Button asChild>
              <Link href={profileEditHref(returnTo)}>
                {snapshot.summary?.draft_saved
                  ? "Continuar perfil"
                  : "Completar perfil"}
              </Link>
            </Button>
          ) : null}
          {canStartVerification ? (
            <Button
              type="button"
              disabled={busy}
              onClick={() => void handleVerification()}
            >
              {busy ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <ArrowUpRight className="size-4" aria-hidden="true" />
              )}
              {status === "in_progress"
                ? "Continuar verificación"
                : ["needs_correction", "expired", "revoked"].includes(status)
                  ? "Volver a verificar"
                  : "Iniciar verificación"}
            </Button>
          ) : null}
          {["in_progress", "in_review"].includes(status) ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => void handleRefreshVerification()}
            >
              Actualizar estado
            </Button>
          ) : null}
          {snapshot.summary?.complete && !snapshot.error ? (
            <Button asChild variant="outline">
              <Link href={profileEditHref(returnTo)}>
                {snapshot.summary.rut_locked
                  ? "Editar teléfono"
                  : "Editar datos del perfil"}
              </Link>
            </Button>
          ) : null}
          {snapshot.error ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void reload()}
            >
              Reintentar consulta
            </Button>
          ) : null}
        </div>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {message ? (
          <p role="status" className="text-sm text-muted-foreground">
            {message}
          </p>
        ) : null}
        {status === "needs_correction" ? (
          <p className="text-sm text-muted-foreground">
            Revisa los datos de tu cédula. Si ya configuraste tu identidad en
            otra cuenta, puedes{" "}
            <Link
              href="/recuperar-acceso"
              className="font-medium text-primary underline"
            >
              recuperar el acceso
            </Link>
            .
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
