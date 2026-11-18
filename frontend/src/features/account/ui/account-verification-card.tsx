"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  RotateCcw,
  ShieldCheck,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Button } from "@/components/workspace/ui/button";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { LoadingSpinner } from "@/components/ui/loading-spinner";
import { useAuth } from "@/features/auth/ui/use-auth";
import { apiClient } from "@/lib/api/client";
import { isAbortError } from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";

type VerificationStatusView =
  | { readonly kind: "loading" }
  | { readonly kind: "error" }
  | { readonly kind: "ready"; readonly verified: boolean };

interface AccountVerificationCardProps {
  readonly showDeclarationLink?: boolean;
}

export function AccountVerificationCard({
  showDeclarationLink = true,
}: AccountVerificationCardProps) {
  const { snapshot: authSnapshot } = useAuth();

  if (authSnapshot.status !== "authenticated") {
    return null;
  }

  const sessionKey = JSON.stringify([
    authSnapshot.status,
    authSnapshot.accountId,
    [...authSnapshot.roles].sort(),
    [...authSnapshot.capabilities].sort(),
    authSnapshot.restrictedMode,
    authSnapshot.expiresAt,
  ]);

  return (
    <AuthenticatedAccountVerificationCard
      key={sessionKey}
      showDeclarationLink={showDeclarationLink}
    />
  );
}

function AuthenticatedAccountVerificationCard({
  showDeclarationLink,
}: {
  readonly showDeclarationLink: boolean;
}) {
  const [status, setStatus] = useState<VerificationStatusView>({
    kind: "loading",
  });
  const [checked, setChecked] = useState(false);
  const [optionalName, setOptionalName] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  const reload = useCallback(() => {
    setActionError(null);
    setStatus({ kind: "loading" });
    setReloadToken((value) => value + 1);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    void apiClient
      .GET("/verifications/eligibility", { signal: controller.signal })
      .then((result) => {
        if (cancelled) return;
        if (!result.response.ok) {
          setStatus({ kind: "error" });
          return;
        }
        const items = result.data?.items ?? [];
        const kycVerified = items.some(
          (item) => item.type === "kyc" && item.eligible === true,
        );
        setChecked(kycVerified);
        setStatus({ kind: "ready", verified: kycVerified });
      })
      .catch((error: unknown) => {
        if (!cancelled && !isAbortError(error)) {
          setStatus({ kind: "error" });
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [reloadToken]);

  async function handleQuickVerify(nextChecked: boolean) {
    setChecked(nextChecked);
    if (!nextChecked || verifying) return;
    setVerifying(true);
    setActionError(null);
    try {
      const result = await apiClient.POST("/verifications", {
        params: {
          header: {
            "Idempotency-Key": crypto.randomUUID(),
          },
        },
        body: { type: "kyc" },
        bodySerializer: serializeExactJson,
      });
      if (!result.response.ok) {
        setChecked(false);
        setActionError(
          "No pudimos completar la verificación rápida. Inténtalo de nuevo.",
        );
        return;
      }
      const eligibilityResult = await apiClient.GET(
        "/verifications/eligibility",
      );
      const items = eligibilityResult.data?.items ?? [];
      const kycVerified = items.some(
        (item) => item.type === "kyc" && item.eligible === true,
      );
      const isApproved =
        (result.data as { state?: string } | undefined)?.state === "aprobada";

      if (kycVerified || isApproved) {
        setChecked(true);
        setStatus({ kind: "ready", verified: true });
      } else {
        setChecked(false);
        setActionError(
          "La verificación no pudo confirmarse de inmediato. Inténtalo de nuevo.",
        );
      }
    } catch (error: unknown) {
      if (isAbortError(error)) return;
      setChecked(false);
      setActionError(
        "No pudimos completar la verificación rápida. Inténtalo de nuevo.",
      );
    } finally {
      setVerifying(false);
    }
  }

  const isVerified = status.kind === "ready" && status.verified;

  return (
    <Card
      aria-labelledby="account-verification-title"
      className={
        isVerified
          ? "border-emerald-200/80 bg-emerald-50/30 dark:border-emerald-900/50 dark:bg-emerald-950/15"
          : "border-amber-200/80 bg-amber-50/30 dark:border-amber-900/50 dark:bg-amber-950/15"
      }
    >
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2.5">
            {isVerified ? (
              <CheckCircle2
                className="size-5 text-emerald-600 shrink-0"
                aria-hidden="true"
              />
            ) : (
              <ShieldCheck
                className="size-5 text-amber-700 shrink-0"
                aria-hidden="true"
              />
            )}
            <CardTitle
              id="account-verification-title"
              className="text-base sm:text-lg"
            >
              Verificación de la cuenta
            </CardTitle>
          </div>
          {isVerified ? (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-100 px-3 py-1 text-xs font-semibold text-emerald-800">
              <CheckCircle2 className="size-3.5" aria-hidden="true" />
              Cuenta verificada
            </span>
          ) : null}
        </div>
        <CardDescription className="text-sm text-foreground/80 mt-1">
          {isVerified
            ? "Tu cuenta se encuentra verificada y lista para operar en EspaciGo."
            : "Marca la casilla de verificación rápida para activar tu cuenta de inmediato."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {status.kind === "loading" ? (
            <LoadingSpinner
              label="Consultando estado de verificación…"
              className="min-h-[80px] p-3"
            />
          ) : null}

          {status.kind === "error" ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p
                role="alert"
                className="flex items-center gap-2 text-sm text-destructive"
              >
                <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
                No pudimos consultar el estado de verificación de tu cuenta.
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={reload}
                className="gap-1.5"
              >
                <RotateCcw className="size-3.5" aria-hidden="true" />
                Reintentar
              </Button>
            </div>
          ) : null}

          {status.kind === "ready" && status.verified ? (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 rounded-lg border border-emerald-200 bg-white/80 p-3.5 text-sm text-emerald-900">
              <div className="flex items-center gap-2.5 font-medium">
                <CheckCircle2
                  className="size-5 text-emerald-600 shrink-0"
                  aria-hidden="true"
                />
                <span>
                  Cuenta verificada correctamente. Ya puedes reservar y publicar
                  espacios.
                </span>
              </div>
              {showDeclarationLink ? (
                <Button asChild size="sm" variant="outline" className="gap-1.5">
                  <Link href="/mi-cuenta/verificaciones">
                    Ver estado
                    <ArrowRight className="size-4" aria-hidden="true" />
                  </Link>
                </Button>
              ) : null}
            </div>
          ) : null}

          {status.kind === "ready" && !status.verified ? (
            <div className="space-y-4 rounded-xl border border-amber-200/80 bg-white/90 p-4">
              <div className="space-y-1.5">
                <Label
                  htmlFor="quick-verify-name"
                  className="text-xs font-medium text-muted-foreground"
                >
                  Nombre o referencia titular (opcional)
                </Label>
                <Input
                  id="quick-verify-name"
                  type="text"
                  value={optionalName}
                  onChange={(event) => setOptionalName(event.target.value)}
                  placeholder="Puedes dejarlo en blanco o ingresar tu nombre"
                  disabled={verifying}
                  className="max-w-md bg-white"
                />
              </div>

              <label
                htmlFor="quick-verify-checkbox"
                className="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-gray-200 bg-gray-50/80 px-3.5 py-2.5 text-sm font-medium text-gray-900 hover:bg-gray-100/80"
              >
                <input
                  id="quick-verify-checkbox"
                  type="checkbox"
                  checked={checked}
                  disabled={verifying}
                  onChange={(event) =>
                    void handleQuickVerify(event.target.checked)
                  }
                  className="size-5 shrink-0 rounded accent-orange-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-orange-700"
                />
                <span>Verificar cuenta</span>
                {verifying ? (
                  <LoadingSpinner
                    inline
                    label="Verificando…"
                    className="ml-auto text-orange-700"
                  />
                ) : null}
              </label>

              {actionError ? (
                <p role="alert" className="text-xs font-medium text-red-700">
                  {actionError}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
