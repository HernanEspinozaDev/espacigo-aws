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
import {
  agreementStateLabel,
  type CommercialAgreement,
} from "@/features/commercial-contracts/domain/agreements";
import {
  ContractProposalTerms,
  contractDate,
} from "@/features/commercial-contracts/ui/contract-proposal-terms";
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
export function HostAgreementRequests() {
  const { snapshot, accessToken, getAccessToken } = useAuth();
  const isCurrent = useCallback(
    () => getAccessToken() === accessToken,
    [getAccessToken, accessToken],
  );
  if (
    !accessToken ||
    snapshot.restrictedMode ||
    !snapshot.roles.includes("arrendador")
  )
    return null;
  return (
    <Requests
      key={`${snapshot.accountId}:${snapshot.expiresAt}:${accessToken}`}
      isCurrent={isCurrent}
    />
  );
}
function Requests({ isCurrent }: { isCurrent(): boolean }) {
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
  const alive = useRef(true);
  const detailVersion = useRef(0);
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
    return () => {
      alive.current = false;
      controller.abort();
    };
  }, [refresh, isCurrent]);
  async function load(id: string) {
    const version = ++detailVersion.current;
    setSelected(id);
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
  }
  async function decide(action: "accept" | "reject") {
    if (!review || busy || needsReload) return;
    const version = detailVersion.current;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await (action === "accept"
        ? hostAgreements.accept(review, consent)
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
  return (
    <Card>
      <CardHeader>
        <CardTitle>Solicitudes de contrato</CardTitle>
        <CardDescription>
          Revisa la propuesta y registra tu decisión antes de que el
          arrendatario pueda pagar. Una solicitud pendiente no retiene el
          horario.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {loading && <p role="status">Cargando solicitudes…</p>}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => {
            setError("");
            void refresh().catch((error) => setError(message(error)));
          }}
        >
          Actualizar solicitudes
        </Button>
        {!loading && items.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Todavía no tienes solicitudes de contrato.
          </p>
        )}
        <ul className="space-y-2">
          {items.map((item) => (
            <li
              key={item.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"
            >
              <div>
                <p className="font-medium">Solicitud {item.id.slice(0, 8)}</p>
                <p className="text-sm text-muted-foreground">
                  {agreementStateLabel(item.state)} · Enviada{" "}
                  {contractDate(item.created_at, "America/Santiago")}
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => void load(item.id)}
              >
                Revisar solicitud{" "}
                <span className="sr-only">{item.id.slice(0, 8)}</span>
              </Button>
            </li>
          ))}
        </ul>
        {selected && (
          <section
            aria-label="Detalle de solicitud"
            className="space-y-4 border-t pt-5"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="font-semibold">Revisión del contrato</h3>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => void load(selected)}
              >
                Recargar solicitud
              </Button>
            </div>
            {busy && !review && (
              <p role="status">Cargando la versión de la propuesta…</p>
            )}
            {review && (
              <>
                <p className="font-medium">
                  {agreementStateLabel(review.read.agreement.state)}
                </p>
                <p className="text-sm">
                  La solicitud vence el{" "}
                  {contractDate(
                    review.read.agreement.expires_at,
                    review.proposal.time_zone,
                  )}
                  .
                </p>
                <ContractProposalTerms proposal={review.proposal} />
                <dl className="space-y-1 text-sm">
                  {review.read.agreement.signatures.map((signature) => (
                    <div key={signature.role}>
                      <dt className="inline font-medium">
                        Firma {signature.role}:{" "}
                      </dt>
                      <dd className="inline">
                        {contractDate(
                          signature.signed_at,
                          review.proposal.time_zone,
                        )}{" "}
                        · Versión {signature.proposal_version}
                      </dd>
                    </div>
                  ))}
                </dl>
                {review.read.agreement.payment_expires_at && (
                  <p className="rounded-lg border p-3 text-sm">
                    Plazo de pago:{" "}
                    {contractDate(
                      review.read.agreement.payment_expires_at,
                      review.proposal.time_zone,
                    )}
                    . El pago se confirma por separado en el servidor.
                  </p>
                )}
                {review.read.agreement.state === "pending_landlord" && (
                  <>
                    <label className="flex items-start gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={consent}
                        disabled={busy || needsReload}
                        onChange={(event) => setConsent(event.target.checked)}
                      />
                      He revisado esta versión del contrato y acepto firmarla
                      como arrendador. La aprobación revalida la disponibilidad
                      y retiene el horario temporalmente para el pago.
                    </label>
                    <div className="flex flex-wrap gap-3">
                      <Button
                        type="button"
                        disabled={busy || needsReload || !consent}
                        onClick={() => void decide("accept")}
                      >
                        {busy ? "Registrando decisión…" : "Aceptar y firmar"}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        disabled={busy || needsReload}
                        onClick={() => void decide("reject")}
                      >
                        Rechazar solicitud
                      </Button>
                    </div>
                  </>
                )}
                {needsReload && (
                  <p className="text-sm">
                    Recarga la solicitud y revisa el resultado antes de volver a
                    decidir.
                  </p>
                )}
              </>
            )}
          </section>
        )}
      </CardContent>
    </Card>
  );
}
