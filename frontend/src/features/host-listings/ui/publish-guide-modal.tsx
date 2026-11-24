"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  Clock,
  CreditCard,
  FileText,
  Home,
  Loader2,
  Lock,
  RefreshCw,
  ShieldCheck,
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
import { apiClient } from "@/lib/api/client";
import { isAbortError } from "@/lib/api/errors";
import { contractTemplateService } from "../contract-template-runtime";
import {
  mercadoPagoReturnHref,
  verificationReturnHref,
  type PublishGateReasonCode,
  type PublishGuideStepStatus,
} from "../domain/publish-guide";
import type { ListingDetail } from "../domain/types";
import { canPublishListing } from "../domain/validation";

export interface PublishPrerequisitesSnapshot {
  readonly loading: boolean;
  readonly loaded: boolean;
  readonly kycVerified: boolean;
  readonly mpConnected: boolean;
  readonly contractPublishedVersion: number;
  readonly error: string | null;
}

interface UsePublishPrerequisitesOptions {
  readonly accessToken: string | null;
  readonly isHost: boolean;
  readonly spaceId?: string | null;
  readonly enabled?: boolean;
  readonly onLoaded?: (snapshot: PublishPrerequisitesSnapshot) => void;
}

export function usePublishPrerequisites({
  accessToken,
  isHost,
  spaceId = null,
  enabled = true,
  onLoaded,
}: UsePublishPrerequisitesOptions) {
  const [reloadToken, setReloadToken] = useState(0);
  const requestKey =
    enabled && accessToken
      ? `${accessToken}:${isHost}:${spaceId ?? ""}:${reloadToken}`
      : "";
  const [result, setResult] = useState<
    PublishPrerequisitesSnapshot & { readonly key: string }
  >({
    key: "",
    loading: false,
    loaded: false,
    kycVerified: false,
    mpConnected: false,
    contractPublishedVersion: 0,
    error: null,
  });

  const reload = useCallback(() => {
    setReloadToken((n) => n + 1);
  }, []);

  const setContractPublishedVersion = useCallback((version: number) => {
    setResult((prev) =>
      prev.contractPublishedVersion === version
        ? prev
        : { ...prev, contractPublishedVersion: version },
    );
  }, []);

  useEffect(() => {
    if (!requestKey || !accessToken) {
      return;
    }

    const controller = new AbortController();
    let active = true;
    const headers = { Authorization: `Bearer ${accessToken}` };

    async function load() {
      let nextKyc = false;
      let nextMp = false;
      let nextContractVersion = 0;
      let hadError = false;

      const kycPromise = apiClient
        .GET("/verifications/eligibility", {
          headers,
          signal: controller.signal,
        })
        .then((res) => {
          if (!res.response.ok) {
            hadError = true;
            return;
          }
          const items = res.data?.items ?? [];
          nextKyc = items.some(
            (item) => item.type === "kyc" && item.eligible === true,
          );
        })
        .catch((err: unknown) => {
          if (!isAbortError(err)) {
            hadError = true;
          }
        });

      const mpPromise = isHost
        ? apiClient
            .GET("/host/mercadopago", {
              headers,
              signal: controller.signal,
            })
            .then((res) => {
              if (!res.response.ok) {
                if (res.response.status !== 403) {
                  hadError = true;
                }
                return;
              }
              nextMp = res.data?.connected === true;
            })
            .catch((err: unknown) => {
              if (!isAbortError(err)) {
                hadError = true;
              }
            })
        : Promise.resolve();

      const contractPromise = spaceId
        ? contractTemplateService
            .read(spaceId, controller.signal)
            .then((res) => {
              nextContractVersion = res.draft.published_version ?? 0;
            })
            .catch((err: unknown) => {
              if (!isAbortError(err)) {
                hadError = true;
              }
            })
        : Promise.resolve();

      await Promise.all([kycPromise, mpPromise, contractPromise]);

      if (!active || controller.signal.aborted) return;

      const nextSnapshot: PublishPrerequisitesSnapshot = {
        loading: false,
        loaded: true,
        kycVerified: nextKyc,
        mpConnected: nextMp,
        contractPublishedVersion: nextContractVersion,
        error: hadError
          ? "No pudimos consultar todos los requisitos. Puedes actualizar para reintentar."
          : null,
      };
      setResult({
        key: requestKey,
        ...nextSnapshot,
      });
      onLoaded?.(nextSnapshot);
    }

    void load();

    return () => {
      active = false;
      controller.abort();
    };
  }, [requestKey, accessToken, isHost, spaceId, onLoaded]);

  const isLoading = Boolean(requestKey) && result.key !== requestKey;
  const loaded =
    Boolean(requestKey) && result.key === requestKey && result.loaded;

  return {
    loading: isLoading,
    loaded,
    kycVerified: result.kycVerified,
    mpConnected: result.mpConnected,
    contractPublishedVersion: result.contractPublishedVersion,
    error: result.error,
    reload,
    setContractPublishedVersion,
  };
}

function StepStatusBadge({
  status,
}: {
  readonly status: PublishGuideStepStatus;
}) {
  switch (status) {
    case "completed":
      return (
        <Badge
          variant="default"
          className="bg-emerald-600 text-white hover:bg-emerald-600 gap-1 shrink-0"
        >
          <CheckCircle2 className="size-3" aria-hidden="true" />
          Completado
        </Badge>
      );
    case "blocked":
      return (
        <Badge
          variant="secondary"
          className="bg-slate-100 text-slate-700 border border-slate-200 gap-1 shrink-0"
        >
          <Lock className="size-3" aria-hidden="true" />
          Bloqueado
        </Badge>
      );
    case "pending":
    default:
      return (
        <Badge
          variant="outline"
          className="border-amber-500/50 bg-amber-50 text-amber-800 gap-1 shrink-0"
        >
          <Clock className="size-3" aria-hidden="true" />
          Pendiente
        </Badge>
      );
  }
}

export interface PublishGuideChecklistProps {
  readonly mode: "list" | "wizard" | "editor";
  readonly isHost: boolean;
  readonly prerequisites: PublishPrerequisitesSnapshot;
  readonly returnPath: string;
  readonly spaceId?: string | null;
  readonly listing?: ListingDetail | null;
  readonly hasAnyListing?: boolean;
  readonly onOpenContractTab?: () => void;
  readonly onFixDraft?: () => void;
  readonly onCloseModal?: () => void;
}

export function PublishGuideChecklist({
  mode,
  isHost,
  prerequisites,
  returnPath,
  spaceId = null,
  listing = null,
  hasAnyListing = false,
  onOpenContractTab,
  onFixDraft,
  onCloseModal,
}: PublishGuideChecklistProps) {
  const effectiveSpaceId = spaceId ?? listing?.id ?? null;
  const draftCheck = listing ? canPublishListing(listing) : null;

  const kycStatus: PublishGuideStepStatus = prerequisites.kycVerified
    ? "completed"
    : "pending";

  const mpStatus: PublishGuideStepStatus = prerequisites.mpConnected
    ? "completed"
    : isHost
      ? "pending"
      : "blocked";

  const draftStatus: PublishGuideStepStatus =
    mode === "editor"
      ? draftCheck?.eligible
        ? "completed"
        : "pending"
      : mode === "list" && hasAnyListing
        ? "completed"
        : "pending";

  const contractStatus: PublishGuideStepStatus = !effectiveSpaceId
    ? "blocked"
    : prerequisites.contractPublishedVersion >= 1
      ? "completed"
      : "pending";

  const kycHref = verificationReturnHref(returnPath);
  const mpHref = mercadoPagoReturnHref(returnPath);

  return (
    <ol className="space-y-3" aria-label="Pasos obligatorios para publicar">
      {/* Paso 1: KYC */}
      <li
        className={`rounded-xl border p-4 transition-colors ${
          kycStatus === "completed"
            ? "border-emerald-200 bg-emerald-50/40"
            : "border-amber-200 bg-amber-50/30"
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                kycStatus === "completed"
                  ? "bg-emerald-600 text-white"
                  : "bg-amber-600 text-white"
              }`}
            >
              1
            </div>
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <ShieldCheck
                  className="size-4 text-primary shrink-0"
                  aria-hidden="true"
                />
                <h3 className="text-sm font-semibold text-foreground">
                  Paso 1 — Confirmar tu identidad (KYC)
                </h3>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                Debes tener tu verificación de identidad aprobada para publicar
                espacios y celebrar contratos de arriendo.
              </p>
            </div>
          </div>
          <StepStatusBadge status={kycStatus} />
        </div>
        {kycStatus !== "completed" && (
          <div className="mt-3 pl-10">
            <Button asChild size="sm" variant="default" className="gap-1.5">
              <Link href={kycHref} onClick={() => onCloseModal?.()}>
                Confirmar identidad (KYC)
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Link>
            </Button>
          </div>
        )}
      </li>

      {/* Paso 2: Borrador del espacio */}
      <li
        className={`rounded-xl border p-4 transition-colors ${
          draftStatus === "completed"
            ? "border-emerald-200 bg-emerald-50/40"
            : "border-amber-200 bg-amber-50/30"
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                draftStatus === "completed"
                  ? "bg-emerald-600 text-white"
                  : "bg-amber-600 text-white"
              }`}
            >
              2
            </div>
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <Home
                  className="size-4 text-primary shrink-0"
                  aria-hidden="true"
                />
                <h3 className="text-sm font-semibold text-foreground">
                  Paso 2 — Crear y completar borrador del espacio
                </h3>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {mode === "editor"
                  ? draftCheck?.eligible
                    ? "Tu borrador cuenta con título, ubicación confirmada, tarifa, capacidad, superficie y zona horaria."
                    : "Completa los datos básicos obligatorios del espacio (título, dirección, tarifa, capacidad, superficie y zona horaria)."
                  : mode === "wizard"
                    ? "Completa los pasos de este asistente para guardar el borrador privado de tu espacio."
                    : hasAnyListing
                      ? "Ya tienes al menos un borrador registrado. Puedes editarlo o crear otro espacio cuando quieras."
                      : "Crea el borrador de tu espacio con su ubicación, características, tarifa y horarios."}
              </p>
              {mode === "editor" &&
                draftCheck &&
                !draftCheck.eligible &&
                draftCheck.reasons.length > 0 && (
                  <ul className="mt-1 list-disc list-inside text-xs text-amber-900 space-y-0.5">
                    {draftCheck.reasons.map((reason, idx) => (
                      <li key={idx}>{reason}</li>
                    ))}
                  </ul>
                )}
            </div>
          </div>
          <StepStatusBadge status={draftStatus} />
        </div>
        {mode === "list" && !hasAnyListing && (
          <div className="mt-3 pl-10">
            <Button asChild size="sm" variant="outline" className="gap-1.5">
              <Link
                href="/anfitrion/espacios/nuevo"
                onClick={() => onCloseModal?.()}
              >
                Crear borrador del espacio
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Link>
            </Button>
          </div>
        )}
        {mode === "editor" && draftStatus !== "completed" && onFixDraft && (
          <div className="mt-3 pl-10">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                onFixDraft();
                onCloseModal?.();
              }}
            >
              Completar datos del espacio
            </Button>
          </div>
        )}
      </li>

      {/* Paso 3: Mercado Pago */}
      <li
        className={`rounded-xl border p-4 transition-colors ${
          mpStatus === "completed"
            ? "border-emerald-200 bg-emerald-50/40"
            : mpStatus === "blocked"
              ? "border-border bg-muted/30"
              : "border-amber-200 bg-amber-50/30"
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                mpStatus === "completed"
                  ? "bg-emerald-600 text-white"
                  : mpStatus === "blocked"
                    ? "bg-muted text-muted-foreground"
                    : "bg-amber-600 text-white"
              }`}
            >
              3
            </div>
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <CreditCard
                  className="size-4 text-primary shrink-0"
                  aria-hidden="true"
                />
                <h3 className="text-sm font-semibold text-foreground">
                  Paso 3 — Sincronizar cuenta de Mercado Pago
                </h3>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {isHost
                  ? "Vincula tu cuenta de vendedor en Mercado Pago para recibir los pagos de tus reservas. Una vez vinculada, podrás publicar tus espacios."
                  : "Se habilita al crear tu primer borrador de espacio (que activa tu perfil de anfitrión). Una vez vinculada, podrás publicar tus espacios."}
              </p>
            </div>
          </div>
          <StepStatusBadge status={mpStatus} />
        </div>
        {isHost && mpStatus !== "completed" && (
          <div className="mt-3 pl-10">
            <Button asChild size="sm" variant="default" className="gap-1.5">
              <Link href={mpHref} onClick={() => onCloseModal?.()}>
                Sincronizar Mercado Pago
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Link>
            </Button>
          </div>
        )}
      </li>

      {/* Paso 4: Plantilla de contrato publicada */}
      <li
        className={`rounded-xl border p-4 transition-colors ${
          contractStatus === "completed"
            ? "border-emerald-200 bg-emerald-50/40"
            : contractStatus === "blocked"
              ? "border-border bg-muted/30"
              : "border-amber-200 bg-amber-50/30"
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                contractStatus === "completed"
                  ? "bg-emerald-600 text-white"
                  : contractStatus === "blocked"
                    ? "bg-muted text-muted-foreground"
                    : "bg-amber-600 text-white"
              }`}
            >
              4
            </div>
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <FileText
                  className="size-4 text-primary shrink-0"
                  aria-hidden="true"
                />
                <h3 className="text-sm font-semibold text-foreground">
                  Paso 4 — Contrato listo (plantilla publicada)
                </h3>
              </div>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {contractStatus === "completed"
                  ? `Plantilla publicada (versión v${prerequisites.contractPublishedVersion}). Lista para generar contratos con tus arrendatarios.`
                  : effectiveSpaceId
                    ? "Configura las cláusulas del contrato y publica al menos una versión (v1) antes de activar la publicación del espacio."
                    : "Disponible en la pestaña Contrato del editor una vez creado el borrador del espacio."}
              </p>
            </div>
          </div>
          <StepStatusBadge status={contractStatus} />
        </div>
        {effectiveSpaceId && contractStatus !== "completed" && (
          <div className="mt-3 pl-10">
            {onOpenContractTab ? (
              <Button
                type="button"
                size="sm"
                variant="default"
                className="gap-1.5"
                onClick={() => {
                  onOpenContractTab();
                  onCloseModal?.();
                }}
              >
                Configurar y publicar contrato
                <ArrowRight className="size-3.5" aria-hidden="true" />
              </Button>
            ) : (
              <Button asChild size="sm" variant="default" className="gap-1.5">
                <Link
                  href={`/anfitrion/espacios/${effectiveSpaceId}/editar?tab=contrato`}
                  onClick={() => onCloseModal?.()}
                >
                  Configurar y publicar contrato
                  <ArrowRight className="size-3.5" aria-hidden="true" />
                </Link>
              </Button>
            )}
          </div>
        )}
      </li>
    </ol>
  );
}

const REASON_MESSAGES: Record<PublishGateReasonCode, string> = {
  identity_verification_required:
    "Para publicar este espacio primero debes confirmar tu identidad (KYC) en el Paso 1.",
  mercadopago_connection_required:
    "Para publicar este espacio primero debes vincular tu cuenta de Mercado Pago en el Paso 2.",
  contract_template_required:
    "Para publicar este espacio primero debes configurar y publicar una versión de su plantilla de contrato en el Paso 4.",
  location_confirmation_required:
    "Confirma la dirección y el pin del espacio en el editor antes de publicar por primera vez.",
  time_zone_required:
    "Configura una zona horaria IANA válida (ej. America/Santiago) antes de publicar.",
};

export interface PublishGuideModalProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly mode: "list" | "wizard" | "editor";
  readonly isHost: boolean;
  readonly prerequisites: PublishPrerequisitesSnapshot;
  readonly onRefresh: () => void;
  readonly returnPath: string;
  readonly spaceId?: string | null;
  readonly listing?: ListingDetail | null;
  readonly hasAnyListing?: boolean;
  readonly triggerReason?: PublishGateReasonCode | null;
  readonly onOpenContractTab?: () => void;
  readonly onFixDraft?: () => void;
}

export function PublishGuideModal({
  open,
  onOpenChange,
  mode,
  isHost,
  prerequisites,
  onRefresh,
  returnPath,
  spaceId = null,
  listing = null,
  hasAnyListing = false,
  triggerReason = null,
  onOpenContractTab,
  onFixDraft,
}: PublishGuideModalProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto rounded-2xl">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold">
            Guía para publicar espacio
          </DialogTitle>
          <DialogDescription>
            Completa estos 4 pasos obligatorios en orden para activar tu espacio
            en el catálogo público de EspaciGo.
          </DialogDescription>
        </DialogHeader>

        {triggerReason && (
          <div
            role="alert"
            className="rounded-lg border border-amber-300 bg-amber-50 p-3.5 text-xs text-amber-950 flex items-start gap-2.5"
          >
            <AlertCircle
              className="size-4 shrink-0 text-amber-700 mt-0.5"
              aria-hidden="true"
            />
            <span>{REASON_MESSAGES[triggerReason]}</span>
          </div>
        )}

        {prerequisites.error && (
          <div
            role="status"
            className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive flex items-center justify-between gap-2"
          >
            <span>{prerequisites.error}</span>
          </div>
        )}

        {prerequisites.loading && !prerequisites.loaded ? (
          <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            <span>Verificando requisitos de publicación…</span>
          </div>
        ) : (
          <PublishGuideChecklist
            mode={mode}
            isHost={isHost}
            prerequisites={prerequisites}
            returnPath={returnPath}
            spaceId={spaceId}
            listing={listing}
            hasAnyListing={hasAnyListing}
            onOpenContractTab={onOpenContractTab}
            onFixDraft={onFixDraft}
            onCloseModal={() => onOpenChange(false)}
          />
        )}

        <DialogFooter className="sm:justify-between gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={prerequisites.loading}
            onClick={onRefresh}
            className="gap-1.5"
          >
            <RefreshCw
              className={`size-3.5 ${prerequisites.loading ? "animate-spin" : ""}`}
              aria-hidden="true"
            />
            Actualizar estado
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => onOpenChange(false)}
          >
            Entendido
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
