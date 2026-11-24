"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/workspace/ui/card";
import { FileText } from "lucide-react";
import {
  agreementStateLabel,
  type CommercialAgreement,
} from "@/features/commercial-contracts/domain/agreements";
import { contractDate } from "@/features/commercial-contracts/ui/contract-proposal-terms";
import { ContractPdfModal } from "@/features/commercial-contracts/ui/contract-pdf-modal";
import {
  FesOtpSigningStep,
  type FesOtpPayload,
} from "@/features/commercial-contracts/ui/fes-otp-signing-step";
import {
  ContractListToolbar,
  ContractPagination,
  agreementBadgeClass,
  matchesAgreementFilter,
  type AgreementFilter,
} from "@/features/commercial-contracts/ui/contract-list-controls";
import { cn } from "@/lib/utils";
import { hostAgreements } from "../host-agreements-runtime";
import type { HostAgreementReview } from "../application/host-agreements-service";
function message(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 412)
      return "La solicitud cambió en otra sesión. Recarga y revisa la nueva versión antes de decidir.";
    if (error.code === "request_expired")
      return "La solicitud venció. Recarga para ver el estado actualizado.";
    if (error.code === "identity_verification_required")
      return "Se requiere la identificación vigente de ambas partes para firmar.";
    if (
      error.code === "proposal_changed" ||
      error.code === "agreement_conflict"
    )
      return "La ubicación, el precio o la disponibilidad cambió. No se firmó ni se reservó otro horario. Recarga y solicita una propuesta nueva si corresponde.";
    if (error.status === 401 || error.status === 403)
      return "Tu sesión no permite decidir esta solicitud.";
  }
  return error instanceof ApiError
    ? "No se pudo completar la operación. Recarga para comprobar su estado antes de reintentar."
    : error instanceof Error
      ? error.message
      : "No se pudo cargar la solicitud.";
}
export function HostAgreementRequests({
  initialAgreementId,
}: {
  initialAgreementId?: string;
} = {}) {
  const { snapshot, accessToken, getAccessToken } = useAuth();
  const isCurrent = useCallback(
    () => getAccessToken() === accessToken,
    [getAccessToken, accessToken],
  );
  if (!accessToken || snapshot.restrictedMode) return null;
  return (
    <Requests
      key={`${snapshot.accountId}:${snapshot.expiresAt}:${accessToken}`}
      isCurrent={isCurrent}
      initialAgreementId={initialAgreementId}
    />
  );
}
function Requests({
  isCurrent,
  initialAgreementId,
}: {
  isCurrent(): boolean;
  initialAgreementId?: string;
}) {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [items, setItems] = useState<CommercialAgreement[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [review, setReview] = useState<HostAgreementReview | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [consent, setConsent] = useState(false);
  const [needsReload, setNeedsReload] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [filter, setFilter] = useState<AgreementFilter>("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [modalOpen, setModalOpen] = useState(false);
  const alive = useRef(true);
  const detailVersion = useRef(0);
  const initialLoadedRef = useRef<string | undefined>(undefined);
  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      const available = await hostAgreements.available(signal);
      if (!alive.current || !isCurrent()) return;
      setEnabled(available);
      if (!available) {
        setItems([]);
        return;
      }
      const result = await hostAgreements.list(signal);
      if (alive.current && isCurrent()) setItems(result);
    },
    [isCurrent],
  );
  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    queueMicrotask(() => {
      if (controller.signal.aborted) return;
      void refresh(controller.signal)
        .catch((error) => {
          if (!controller.signal.aborted && alive.current && isCurrent())
            setError(message(error));
        })
        .finally(() => {
          if (!controller.signal.aborted && alive.current && isCurrent())
            setLoading(false);
        });
    });
    const intervalId = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void refresh(controller.signal).catch(() => {});
      }
    }, 12_000);
    const onVisibility = () => {
      if (document.visibilityState === "visible") {
        void refresh(controller.signal).catch(() => {});
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      alive.current = false;
      controller.abort();
      window.clearInterval(intervalId);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [refresh, isCurrent]);
  const load = useCallback(
    async (id: string) => {
      const version = ++detailVersion.current;
      setSelected(id);
      setModalOpen(true);
      setReview(null);
      setConsent(false);
      setNeedsReload(false);
      setNotice("");
      setError("");
      setBusy(true);
      try {
        const result = await hostAgreements.review(id);
        if (alive.current && isCurrent() && version === detailVersion.current)
          setReview(result);
      } catch (error) {
        if (alive.current && isCurrent() && version === detailVersion.current) {
          setError(message(error));
          setNeedsReload(true);
        }
      } finally {
        if (alive.current && isCurrent() && version === detailVersion.current)
          setBusy(false);
      }
    },
    [isCurrent],
  );
  useEffect(() => {
    if (
      enabled &&
      initialAgreementId &&
      initialLoadedRef.current !== initialAgreementId
    ) {
      initialLoadedRef.current = initialAgreementId;
      void load(initialAgreementId);
    }
  }, [enabled, initialAgreementId, load]);
  async function decide(action: "accept" | "reject", otp?: FesOtpPayload) {
    if (!review || busy || needsReload) return;
    const version = detailVersion.current;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await (action === "accept"
        ? hostAgreements.accept(review, consent, undefined, otp)
        : hostAgreements.reject(review));
      if (!alive.current || !isCurrent() || version !== detailVersion.current)
        return;
      setReview({ ...review, read: result });
      setConsent(false);
      setNotice(
        result.agreement.state === "ready_for_payment"
          ? "Firma registrada. El horario quedó retenido hasta el plazo de pago indicado."
          : result.agreement.state === "rejected"
            ? "Solicitud rechazada. No se creó una reserva."
            : "La solicitud ya tenía una decisión. Revisa su estado actual.",
      );
      await refresh();
    } catch (error) {
      if (alive.current && isCurrent() && version === detailVersion.current) {
        setError(message(error));
        setNeedsReload(true);
        setConsent(false);
      }
    } finally {
      if (alive.current && isCurrent() && version === detailVersion.current)
        setBusy(false);
    }
  }
  if (enabled === false) return null;
  const counts = {
    all: items.length,
    pending: items.filter((i) => i.state === "pending_landlord").length,
    ready: items.filter((i) => i.state === "ready_for_payment").length,
    closed: items.filter((i) =>
      ["rejected", "withdrawn", "expired"].includes(i.state),
    ).length,
  };
  const query = search.trim().toLowerCase();
  const filtered = items.filter((item) => {
    if (!matchesAgreementFilter(item.state, filter)) return false;
    if (!query) return true;
    return (
      item.id.toLowerCase().includes(query) ||
      agreementStateLabel(item.state).toLowerCase().includes(query)
    );
  });
  const pageSize = 10;
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageItems = filtered.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle>Solicitudes recibidas (Arrendador)</CardTitle>
        <CardDescription>
          Revisa la propuesta en PDF y registra tu firma antes de que el
          arrendatario pueda pagar. Una solicitud pendiente no retiene el
          horario.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading && <p role="status">Cargando solicitudes…</p>}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="text-sm font-medium text-emerald-700">
            {notice}
          </p>
        )}
        <ContractListToolbar
          filter={filter}
          onFilterChange={(next) => {
            setFilter(next);
            setPage(1);
          }}
          search={search}
          onSearchChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          counts={counts}
          busy={busy}
          onRefresh={() => {
            setError("");
            void refresh().catch((error) => setError(message(error)));
          }}
        />
        {!loading && items.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Todavía no tienes solicitudes de contrato.
          </p>
        )}
        {!loading && items.length > 0 && filtered.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No hay solicitudes que coincidan con el filtro seleccionado.
          </p>
        )}
        <ul className="divide-y divide-border rounded-lg border border-border">
          {pageItems.map((item) => {
            const renterSigned = item.signatures.some(
              (s) => s.role === "arrendatario",
            );
            const landlordSigned = item.signatures.some(
              (s) => s.role === "arrendador",
            );
            return (
              <li
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-3 p-3.5 hover:bg-muted/20"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-sm">
                      Solicitud {item.id.slice(0, 8)}
                    </p>
                    <span
                      className={cn(
                        "inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium",
                        agreementBadgeClass(item.state),
                      )}
                    >
                      {agreementStateLabel(item.state)}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Enviada {contractDate(item.created_at, "America/Santiago")}{" "}
                    · Arrendatario: {renterSigned ? "Firmado" : "Pendiente"} ·
                    Arrendador: {landlordSigned ? "Firmado" : "Pendiente"}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant={
                    item.state === "pending_landlord" ? "default" : "outline"
                  }
                  disabled={busy}
                  onClick={() => void load(item.id)}
                >
                  <FileText className="size-4" aria-hidden="true" />
                  <span>
                    {item.state === "pending_landlord"
                      ? "Revisar y firmar"
                      : "Ver PDF"}
                  </span>
                </Button>
              </li>
            );
          })}
        </ul>
        <ContractPagination
          page={currentPage}
          totalPages={totalPages}
          onPageChange={setPage}
        />
        {selected && modalOpen && busy && !review && (
          <p role="status" className="text-sm text-muted-foreground">
            Cargando documento de contrato…
          </p>
        )}
        <ContractPdfModal
          open={modalOpen && Boolean(review)}
          onClose={() => setModalOpen(false)}
          proposal={review?.proposal ?? null}
          agreement={review?.read.agreement}
          title="Revisión del contrato (Arrendador)"
          footer={
            review ? (
              <div className="space-y-3">
                {error && (
                  <p role="alert" className="text-sm text-destructive">
                    {error}
                  </p>
                )}
                {notice && (
                  <p
                    role="status"
                    className="text-sm font-medium text-emerald-700"
                  >
                    {notice}
                  </p>
                )}
                {review.read.agreement.payment_expires_at && (
                  <p className="text-xs text-muted-foreground">
                    Plazo de pago:{" "}
                    {contractDate(
                      review.read.agreement.payment_expires_at,
                      review.proposal.time_zone,
                    )}
                    .
                  </p>
                )}
                {review.read.agreement.state === "pending_landlord" ? (
                  <FesOtpSigningStep
                    proposalId={review.proposal.id}
                    snapshotSha256={review.read.agreement.snapshot_sha256}
                    role="arrendador"
                    agreementId={review.read.agreement.id}
                    consent={consent}
                    onConsentChange={setConsent}
                    consentText="He revisado esta versión del contrato y acepto firmarla como arrendador (FES). La aprobación revalida la disponibilidad y retiene el horario temporalmente para el pago."
                    busy={busy}
                    disabled={needsReload}
                    submitLabel="Aceptar y firmar (FES)"
                    onSubmit={(otp) => decide("accept", otp)}
                    extraActions={
                      <>
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy}
                          onClick={() => void load(review.read.agreement.id)}
                        >
                          Recargar
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || needsReload}
                          onClick={() => void decide("reject")}
                        >
                          Rechazar solicitud
                        </Button>
                      </>
                    }
                  />
                ) : (
                  <div className="flex justify-end">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setModalOpen(false)}
                    >
                      Cerrar
                    </Button>
                  </div>
                )}
                {needsReload && (
                  <p className="text-xs text-muted-foreground">
                    Recarga la solicitud y revisa el resultado antes de volver a
                    decidir.
                  </p>
                )}
              </div>
            ) : undefined
          }
        />
      </CardContent>
    </Card>
  );
}
