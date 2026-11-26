"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Loader2, ShieldCheck } from "lucide-react";
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
import { useIdentityProfile } from "./use-identity-profile";

const STATE_LABEL: Record<IdentityProfileState, string> = {
  incomplete: "Perfil incompleto",
  ready: "Listo para verificar",
  in_progress: "Verificación en curso",
  in_review: "Identidad en revisión",
  needs_correction: "Requiere corrección",
  verified: "Identidad verificada",
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
      className="scroll-mt-24"
    >
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <ShieldCheck className="size-5" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <CardTitle id="identity-verification-title">
              Verificación de identidad
            </CardTitle>
            <CardDescription>
              {status === "verified" && snapshot.summary?.verified_until
                ? "Vigente hasta el " +
                  new Date(snapshot.summary.verified_until).toLocaleDateString(
                    "es-CL",
                    {
                      timeZone: "America/Santiago",
                    },
                  )
                : status === "in_review"
                  ? "Estamos revisando el resultado de tu verificación."
                  : snapshot.summary?.complete
                    ? "Confirma tu identidad con tu cédula chilena en Didit."
                    : "Completa los datos de tu perfil para iniciar este paso."}
            </CardDescription>
          </div>
        </div>
        <span
          className="w-fit shrink-0 rounded-full border border-border bg-muted px-3 py-1 text-xs font-medium text-foreground"
          data-testid="identity-profile-state"
          role="status"
        >
          {snapshot.loading
            ? "Consultando estado…"
            : snapshot.error
              ? "Estado no disponible"
              : STATE_LABEL[status]}
        </span>
      </CardHeader>
      <CardContent className="space-y-3">
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
              {busy && (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
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
                Editar datos del perfil
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
