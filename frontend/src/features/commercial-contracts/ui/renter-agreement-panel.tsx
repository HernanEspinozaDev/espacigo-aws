"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import { CheckoutProButton } from "@/features/rental-checkout/ui/checkout-pro-button";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import {
  agreementStateLabel,
  canPayAgreement,
  type AgreementRead,
  type ContractProposal,
} from "../domain/agreements";
import { ContractProposalTerms, contractDate } from "./contract-proposal-terms";

interface Props {
  quoteId?: string;
  agreementId?: string;
  checkoutApiEnabled: boolean;
  walletPurchaseEnabled: boolean;
}
export function agreementErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401 || error.status === 403)
      return "Inicia sesión con la cuenta que participa en esta solicitud.";
    switch (error.code) {
      case "identity_required":
      case "identity_verification_required":
        return "Ambas partes necesitan una verificación de identidad vigente para firmar.";
      case "space_location_confirmation_required":
        return "El anfitrión debe confirmar la ubicación del espacio antes de preparar el contrato.";
      case "contract_template_required":
        return "El anfitrión debe publicar una plantilla de contrato para este espacio.";
      case "contract_terms_changed":
      case "proposal_changed":
        return "Las condiciones cambiaron. Solicita una nueva cotización y revisa el acuerdo actualizado.";
      case "request_expired":
        return "La solicitud venció. Vuelve a cotizar para enviar un nuevo acuerdo.";
      case "contract_terms_unavailable":
      case "contract_evidence_unavailable":
        return "Los contratos no están disponibles en este momento. No se habilitó ningún pago.";
      case "precondition_failed":
        return "La solicitud cambió en otra sesión. Actualiza su estado antes de continuar.";
    }
    if (error.status === 404)
      return "La cotización o la solicitud ya no están disponibles para esta cuenta.";
  }
  return "No pudimos confirmar la operación. Actualiza el estado antes de continuar o reintenta el envío.";
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

function AgreementPanel({
  quoteId,
  agreementId,
  checkoutApiEnabled,
  walletPurchaseEnabled,
  isCurrent,
}: Props & { isCurrent(): boolean }) {
  const [read, setRead] = useState<AgreementRead | null>(null);
  const [proposal, setProposal] = useState<ContractProposal | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [consent, setConsent] = useState(false);
  const [error, setError] = useState("");
  const [now, setNow] = useState(() => Date.now());
  const alive = useRef(true);
  const key = useRef<string | null>(null);
  const allowed = () => alive.current && isCurrent();
  const load = useCallback(async () => {
    let id = agreementId;
    if (!id && quoteId) {
      const items = await commercialAgreements.list("renter");
      id = items.find((a) => a.quote_id === quoteId)?.id;
    }
    if (id) {
      const loaded = await commercialAgreements.resume(id);
      if (alive.current && isCurrent()) {
        setRead(loaded);
        setProposal(loaded.proposal);
        setConsent(false);
      }
    }
  }, [agreementId, quoteId, isCurrent]);
  useEffect(() => {
    alive.current = true;
    let active = true;
    load()
      .catch((e: unknown) => {
        if (active && isCurrent()) setError(agreementErrorMessage(e));
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
  const action = async (operation: () => Promise<void>) => {
    if (loading || busy || !allowed()) return;
    setBusy(true);
    setError("");
    try {
      await operation();
    } catch (e) {
      if (allowed()) setError(agreementErrorMessage(e));
    } finally {
      if (allowed()) setBusy(false);
    }
  };
  const ready = read !== null && canPayAgreement(read.agreement, now) && !error;
  const pending = read?.agreement.state === "pending_landlord";
  const deadline = read?.agreement.payment_expires_at;
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
      {error && (
        <p role="alert" className="text-sm text-red-800">
          {error}
        </p>
      )}
      {!proposal && !loading && (
        <>
          <p className="text-sm">
            Revisa las reglas, el precio y el horario. Tu firma envía la
            solicitud al anfitrión; el pago se habilita después de su firma y de
            confirmar la disponibilidad.
          </p>
          <Button
            type="button"
            disabled={busy || !quoteId}
            onClick={() =>
              void action(async () => {
                if (!quoteId) return;
                const loaded = await commercialAgreements.proposal(quoteId);
                if (allowed()) {
                  setProposal(loaded);
                  setConsent(false);
                  key.current = null;
                }
              })
            }
          >
            Revisar contrato
          </Button>
        </>
      )}
      {proposal && (
        <>
          <details open={!read} className="rounded-lg border border-border p-4">
            <summary className="cursor-pointer font-medium">
              Condiciones de la propuesta · versión {proposal.version}
            </summary>
            <div className="mt-4">
              <ContractProposalTerms proposal={proposal} />
            </div>
          </details>
          {!read && (
            <div className="space-y-3">
              <label className="flex items-start gap-3 text-sm">
                <input
                  className="mt-1 size-4"
                  type="checkbox"
                  disabled={busy}
                  checked={consent}
                  onChange={(e) => setConsent(e.target.checked)}
                />
                <span>
                  He leído y acepto las condiciones de esta propuesta, versión{" "}
                  {proposal.version}, y autorizo registrar mi firma y enviar la
                  solicitud al anfitrión.
                </span>
              </label>
              <Button
                type="button"
                disabled={!consent || busy || loading}
                onClick={() =>
                  void action(async () => {
                    if (!consent) return;
                    key.current ??= crypto.randomUUID();
                    const sent = await commercialAgreements.send(
                      proposal,
                      key.current,
                    );
                    if (allowed()) {
                      setRead(sent);
                      setConsent(false);
                    }
                  })
                }
              >
                {busy ? "Enviando solicitud…" : "Firmar y enviar solicitud"}
              </Button>
            </div>
          )}
        </>
      )}
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
          {ready &&
            deadline &&
            (checkoutApiEnabled || walletPurchaseEnabled ? (
              <Link
                data-mp-checkout-cta="checkout-options"
                className="inline-flex min-h-12 w-full items-center justify-center rounded-xl bg-[#a44731] px-5 py-3 font-semibold text-white"
                href={`/reservas/checkout/opciones/${encodeURIComponent(read.agreement.quote_id)}`}
              >
                Ir a pagar
              </Link>
            ) : (
              <CheckoutProButton
                quoteId={read.agreement.quote_id}
                expiresAt={deadline}
              />
            ))}
        </div>
      )}
      {!loading && !busy && !read && error && (
        <Button
          type="button"
          variant="outline"
          onClick={() => void action(load)}
        >
          Actualizar estado
        </Button>
      )}
    </section>
  );
}
