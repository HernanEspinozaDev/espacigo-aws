"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import { CheckoutProButton } from "@/features/rental-checkout/ui/checkout-pro-button";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import {
  agreementErrorMessage,
  agreementStateLabel,
  canPayAgreement,
  isQuoteRefreshErrorCode,
  nextSigningIdempotencyKey,
  shouldClearSigningIdempotencyKey,
  type AgreementRead,
  type ContractProposal,
} from "../domain/agreements";
import { Clock3, FileText } from "lucide-react";
import { contractDate } from "./contract-proposal-terms";
import { ContractPdfModal } from "./contract-pdf-modal";
import { FesOtpSigningStep } from "./fes-otp-signing-step";

export {
  agreementErrorMessage,
  nextSigningIdempotencyKey,
  shouldClearSigningIdempotencyKey,
};

interface Props {
  quoteId?: string;
  agreementId?: string;
  checkoutApiEnabled: boolean;
  walletPurchaseEnabled: boolean;
  onAgreementStateChange?: (state: string | null) => void;
  forceCloseModal?: boolean;
  onRefreshQuote?: () => void;
}
export function RenterAgreementPanel(props: Props) {
  const { snapshot, accessToken, getAccessToken } = useAuth();
  const isCurrent = useCallback(
    () => getAccessToken() === accessToken,
    [getAccessToken, accessToken],
  );
  const scope = JSON.stringify([
    accessToken,
    snapshot.accountId,
    snapshot.restrictedMode,
    snapshot.roles,
    snapshot.capabilities,
  ]);
  if (!accessToken || snapshot.restrictedMode)
    return (
      <p role="status">Inicia sesión para revisar y firmar tu contrato.</p>
    );
  return (
    <AgreementPanel
      key={`${scope}:${props.quoteId ?? props.agreementId}`}
      {...props}
      isCurrent={isCurrent}
    />
  );
}

function formatPaymentCountdown(ms: number): string {
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

function AgreementPanel({
  quoteId,
  agreementId,
  onAgreementStateChange,
  forceCloseModal = false,
  onRefreshQuote,
  isCurrent,
}: Props & { isCurrent(): boolean }) {
  const [read, setRead] = useState<AgreementRead | null>(null);
  const [proposal, setProposal] = useState<ContractProposal | null>(null);
  const [pdfOpen, setPdfOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [consent, setConsent] = useState(false);
  const [error, setError] = useState("");
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const alive = useRef(true);
  const key = useRef<string | null>(null);
  const lastChallengeIdRef = useRef<string | null>(null);
  const allowed = () => alive.current && isCurrent();
  const load = useCallback(async (): Promise<boolean> => {
    let id = agreementId;
    if (!id && quoteId) {
      const items = await commercialAgreements.list("renter");
      id = items.find((a) => a.quote_id === quoteId)?.id;
    }
    if (id) {
      const loaded = await commercialAgreements.resume(id);
      if (alive.current && isCurrent()) {
        setNow(Date.now());
        setRead(loaded);
        setProposal(loaded.proposal);
        setConsent(false);
      }
      return true;
    }
    return false;
  }, [agreementId, quoteId, isCurrent]);
  useEffect(() => {
    alive.current = true;
    let active = true;
    load()
      .catch((e: unknown) => {
        if (active && isCurrent()) {
          setError(agreementErrorMessage(e));
          setErrorCode(e instanceof ApiError ? e.code : null);
        }
      })
      .finally(() => {
        if (active && isCurrent()) setLoading(false);
      });
    const timer = window.setInterval(() => {
      if (active && isCurrent()) setNow(Date.now());
    }, 15000);
    return () => {
      active = false;
      alive.current = false;
      window.clearInterval(timer);
    };
  }, [load, isCurrent]);
  const action = async (operation: () => Promise<unknown>) => {
    if (loading || busy || !allowed()) return;
    setBusy(true);
    setError("");
    setErrorCode(null);
    try {
      await operation();
    } catch (e) {
      if (allowed()) {
        setError(agreementErrorMessage(e));
        setErrorCode(e instanceof ApiError ? e.code : null);
      }
    } finally {
      if (allowed()) setBusy(false);
    }
  };
  const quoteConflict = isQuoteRefreshErrorCode(errorCode);
  const refreshQuoteOrProposal = () =>
    void action(async () => {
      const foundExisting = await load();
      if (foundExisting || !quoteId) {
        if (allowed()) {
          key.current = null;
          lastChallengeIdRef.current = null;
        }
        return;
      }
      try {
        const refreshed = await commercialAgreements.proposal(quoteId);
        if (allowed()) {
          setProposal(refreshed);
          setConsent(false);
          key.current = null;
          lastChallengeIdRef.current = null;
        }
      } catch (e) {
        if (
          onRefreshQuote &&
          e instanceof ApiError &&
          (e.code === "request_expired" ||
            e.code === "agreement_conflict" ||
            e.status === 404)
        ) {
          if (allowed()) {
            setPdfOpen(false);
            onRefreshQuote();
          }
          return;
        }
        throw e;
      }
    });
  const ready = read !== null && canPayAgreement(read.agreement, now) && !error;
  const pending = read?.agreement.state === "pending_landlord";
  const deadline = read?.agreement.payment_expires_at;
  const paymentCountdownActive =
    read?.agreement.state === "ready_for_payment" &&
    typeof deadline === "string" &&
    Date.parse(deadline) > now;
  useEffect(() => {
    onAgreementStateChange?.(read?.agreement.state ?? null);
  }, [read?.agreement.state, onAgreementStateChange]);
  const modalOpen = pdfOpen && !forceCloseModal;
  useEffect(() => {
    if (!pending) return;
    const sync = () => {
      if (
        document.visibilityState === "visible" &&
        alive.current &&
        isCurrent()
      ) {
        void load().catch(() => {});
      }
    };
    const intervalId = window.setInterval(sync, 10_000);
    document.addEventListener("visibilitychange", sync);
    return () => {
      window.clearInterval(intervalId);
      document.removeEventListener("visibilitychange", sync);
    };
  }, [pending, load, isCurrent]);
  useEffect(() => {
    if (!paymentCountdownActive) return;
    const intervalId = window.setInterval(() => {
      if (alive.current && isCurrent()) {
        setNow(Date.now());
      }
    }, 1000);
    return () => {
      window.clearInterval(intervalId);
    };
  }, [paymentCountdownActive, isCurrent]);
  const terminal =
    read && ["rejected", "withdrawn", "expired"].includes(read.agreement.state);
  return (
    <section
      aria-labelledby="renter-agreement-title"
      className="space-y-4 rounded-xl border border-border bg-background p-4 sm:p-5"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="renter-agreement-title" className="text-lg font-semibold">
          Revisar y firmar el contrato
        </h2>
        <Link
          className="text-sm underline underline-offset-4"
          href="/reservas/contratos"
        >
          Mis solicitudes
        </Link>
      </div>
      {loading && <p role="status">Comprobando tus solicitudes…</p>}
      {!modalOpen && error && (
        <p role="alert" className="text-sm text-red-800">
          {error}
        </p>
      )}
      {!proposal && !loading && (
        <>
          <p className="text-sm">
            Revisa las reglas, el precio y el horario. Tu firma envía la
            solicitud al arrendador; el pago se habilita después de su firma y
            de confirmar la disponibilidad.
          </p>
          <Button
            type="button"
            className="w-full"
            disabled={busy || !quoteId}
            onClick={() =>
              void action(async () => {
                if (!quoteId) return;
                const loaded = await commercialAgreements.proposal(quoteId);
                if (allowed()) {
                  setProposal(loaded);
                  setConsent(false);
                  key.current = null;
                  lastChallengeIdRef.current = null;
                  setPdfOpen(true);
                }
              })
            }
          >
            <FileText className="size-4" aria-hidden="true" />
            <span>
              {busy ? "Cargando contrato…" : "Revisar y firmar contrato"}
            </span>
          </Button>
        </>
      )}
      {proposal && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
          <div className="text-sm">
            <p className="font-medium">
              Propuesta de contrato · versión {proposal.version}
            </p>
            <p className="text-xs text-muted-foreground">
              {read
                ? agreementStateLabel(read.agreement.state)
                : "Lista para revisar y firmar"}
            </p>
          </div>
          <Button
            type="button"
            variant={read ? "outline" : "default"}
            onClick={() => setPdfOpen(true)}
          >
            <FileText className="size-4" aria-hidden="true" />
            <span>{read ? "Ver contrato PDF" : "Abrir y firmar contrato"}</span>
          </Button>
        </div>
      )}
      <ContractPdfModal
        open={modalOpen}
        onClose={() => setPdfOpen(false)}
        proposal={proposal}
        agreement={read?.agreement}
        title="Contrato de arriendo"
        footer={
          proposal && !read ? (
            <div className="space-y-3">
              {error && (
                <div
                  role="alert"
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800"
                >
                  <p className="flex-1">{error}</p>
                  {quoteConflict && quoteId && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={busy || loading}
                      onClick={refreshQuoteOrProposal}
                    >
                      {busy ? "Actualizando…" : "Actualizar cotización"}
                    </Button>
                  )}
                </div>
              )}
              <FesOtpSigningStep
                key={`${proposal.id}:${proposal.sha256}`}
                proposalId={proposal.id}
                snapshotSha256={proposal.sha256}
                role="arrendatario"
                consent={consent}
                onConsentChange={setConsent}
                consentText={`He leído y acepto las condiciones de esta propuesta, versión ${proposal.version}, y autorizo registrar mi firma electrónica (FES) y enviar la solicitud al arrendador.`}
                busy={busy}
                disabled={loading}
                submitLabel="Confirmar y firmar (FES)"
                onRequestCode={() => {
                  setError("");
                  setErrorCode(null);
                }}
                onSubmit={async (otp) => {
                  await action(async () => {
                    const nextKey = nextSigningIdempotencyKey(
                      key.current,
                      lastChallengeIdRef.current,
                      otp.challengeId,
                    );
                    key.current = nextKey.idempotencyKey;
                    lastChallengeIdRef.current = nextKey.challengeId;
                    try {
                      const sent = await commercialAgreements.send(
                        proposal,
                        key.current,
                        otp,
                      );
                      if (allowed()) {
                        key.current = null;
                        lastChallengeIdRef.current = null;
                        setNow(Date.now());
                        setRead(sent);
                        setConsent(false);
                        setPdfOpen(false);
                      }
                    } catch (e) {
                      if (shouldClearSigningIdempotencyKey(e)) {
                        key.current = null;
                        lastChallengeIdRef.current = null;
                      }
                      throw e;
                    }
                  });
                }}
                extraActions={
                  <Button
                    type="button"
                    variant="outline"
                    disabled={busy}
                    onClick={() => setPdfOpen(false)}
                  >
                    Cerrar
                  </Button>
                }
              />
            </div>
          ) : (
            <div className="space-y-3">
              {error && (
                <p role="alert" className="text-sm text-red-800">
                  {error}
                </p>
              )}
              <div className="flex justify-end">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setPdfOpen(false)}
                >
                  Cerrar
                </Button>
              </div>
            </div>
          )
        }
      />
      {read && (
        <div className="space-y-3">
          <p role="status" className="font-semibold">
            {agreementStateLabel(read.agreement.state)}
          </p>
          <ul className="space-y-1 text-sm">
            {read.agreement.signatures.map((s) => (
              <li key={s.role}>
                {s.role === "arrendatario" ? "Tu firma" : "Firma del anfitrión"}
                :{" "}
                {contractDate(
                  s.signed_at,
                  proposal?.time_zone ?? "America/Santiago",
                )}
              </li>
            ))}
          </ul>
          {pending && (
            <p className="text-sm">
              La solicitud está sujeta a disponibilidad hasta que el anfitrión
              firme. Vence el{" "}
              {contractDate(
                read.agreement.expires_at,
                proposal?.time_zone ?? "America/Santiago",
              )}
              .
            </p>
          )}
          {deadline && (
            <p className="text-sm">
              Plazo para pagar:{" "}
              {contractDate(
                deadline,
                proposal?.time_zone ?? "America/Santiago",
              )}
              .
            </p>
          )}
          {read.agreement.state === "ready_for_payment" &&
            !canPayAgreement(read.agreement, now) && (
              <p role="status" className="text-sm">
                El plazo de pago terminó. Actualiza el estado de la solicitud.
              </p>
            )}
          <div className="flex flex-wrap gap-3">
            <Button
              type="button"
              variant="outline"
              disabled={busy || loading}
              onClick={() => void action(load)}
            >
              Actualizar estado
            </Button>
            {pending && (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() =>
                  void action(async () => {
                    const withdrawn = await commercialAgreements.withdraw(read);
                    if (allowed()) setRead(withdrawn);
                  })
                }
              >
                Retirar solicitud
              </Button>
            )}
            <Link
              className="inline-flex min-h-11 items-center text-sm underline underline-offset-4"
              href={`/reservas/contratos/${encodeURIComponent(read.agreement.id)}`}
            >
              Volver a esta solicitud
            </Link>
          </div>
          {terminal && (
            <p className="text-sm">
              Para solicitar otro arriendo, vuelve al espacio y elige un horario
              disponible.
            </p>
          )}
          {ready && deadline && (
            <div className="space-y-3">
              <div
                role="status"
                aria-live="polite"
                className="flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm font-medium text-amber-900 tabular-nums dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200"
              >
                <Clock3 className="size-4 shrink-0" aria-hidden="true" />
                <span>
                  Tiempo restante para pagar:{" "}
                  {formatPaymentCountdown(Date.parse(deadline) - now)} (vence a
                  las{" "}
                  {contractDate(
                    deadline,
                    proposal?.time_zone ?? "America/Santiago",
                  )}
                  )
                </span>
              </div>
              <CheckoutProButton
                quoteId={read.agreement.quote_id}
                expiresAt={deadline}
              />
            </div>
          )}
        </div>
      )}
      {!loading && !busy && !read && !modalOpen && error && (
        <div className="flex flex-wrap gap-3">
          {quoteConflict && quoteId && (
            <Button
              type="button"
              variant="outline"
              onClick={refreshQuoteOrProposal}
            >
              Actualizar cotización
            </Button>
          )}
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              void action(async () => {
                await load();
              })
            }
          >
            Actualizar estado
          </Button>
        </div>
      )}
    </section>
  );
}
