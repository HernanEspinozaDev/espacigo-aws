"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import type { RentalQuote, RentalQuoteInterval } from "../domain/rental-quote";
import type { MarketplaceBookingInterval } from "../domain/marketplace-booking-selection";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import { publicRentalQuote } from "../infrastructure/rental-quote-runtime";
import { calculateBuyerFeeBreakdown } from "@/features/rental-checkout/domain/fee-breakdown";
import { CheckoutProButton } from "@/features/rental-checkout/ui/checkout-pro-button";
import { CheckoutSummaryCard } from "@/features/rental-checkout/ui/checkout-flow-chrome";
import { ContractCheckoutGate } from "@/features/commercial-contracts/ui/contract-checkout-gate";
import { ReservationCountdown } from "@/features/rental-checkout/ui/reservation-countdown";

export interface MarketplaceCheckoutSelection {
  spaceId: string;
  startAt: string;
  endAt: string;
  timeZone: string;
  duration?: number;
  rateUnit?: "hora" | "dia" | "mes";
  intervals?: readonly MarketplaceBookingInterval[];
  billingMode?: "full" | "monthly_installments";
}

function quoteError(status: number): string {
  if (status === 401)
    return "Inicia sesión para solicitar la cotización oficial.";
  if (status === 403)
    return "Esta cuenta no puede solicitar una cotización para este espacio.";
  if (status === 404)
    return "El espacio ya no está disponible. Vuelve a la publicación.";
  if (status === 409)
    return "El horario cambió o ya no está disponible. Selecciona otra fecha.";
  if (status === 422)
    return "El intervalo seleccionado no es válido. Vuelve al calendario.";
  if (status === 503)
    return "El servicio de cotizaciones no está disponible por ahora.";
  return "No fue posible calcular una cotización. Intenta nuevamente.";
}

function rentalInterval(quote: RentalQuote): string {
  try {
    return new Intl.DateTimeFormat("es-CL", {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: quote.time_zone,
    }).formatRange(new Date(quote.start_at), new Date(quote.end_at));
  } catch {
    return `${quote.start_at} – ${quote.end_at}`;
  }
}

export function MarketplaceRentalCheckout({
  selection,
  signInReturn,
  checkoutApiEnabled,
  walletPurchaseEnabled,
}: {
  selection: MarketplaceCheckoutSelection;
  signInReturn: string;
  checkoutApiEnabled: boolean;
  walletPurchaseEnabled: boolean;
}) {
  const { snapshot: auth } = useAuth();
  const [quoteState, setQuoteState] = useState<{
    authKey: string;
    quote: (RentalQuote & { intervals?: RentalQuoteInterval[] }) | null;
  } | null>(null);
  const [expiredQuoteId, setExpiredQuoteId] = useState<string | null>(null);
  const [operationState, setOperationState] = useState<{
    authKey: string;
    loading: boolean;
    error: string | null;
  } | null>(null);
  const [billingMode, setBillingMode] = useState<
    "full" | "monthly_installments"
  >(selection.billingMode ?? "full");
  const requestGeneration = useRef(0);
  const requestController = useRef<AbortController | null>(null);
  const loadingRef = useRef(false);
  const autoRequestedKeyRef = useRef<string | null>(null);

  const currentAuthKey =
    auth.status === "authenticated"
      ? (auth.accountId ?? "authenticated")
      : auth.status;
  const autoRequestKey = `${currentAuthKey}:${selection.spaceId}:${selection.startAt}:${selection.endAt}`;

  const quote =
    quoteState?.authKey === currentAuthKey ? quoteState.quote : null;
  const operation =
    operationState?.authKey === currentAuthKey
      ? operationState
      : { authKey: currentAuthKey, loading: false, error: null };
  const { loading, error } = operation;

  const requestQuote = useCallback(async () => {
    if (loadingRef.current || auth.status !== "authenticated") return;
    const authKey = currentAuthKey;
    const generation = ++requestGeneration.current;
    const controller = new AbortController();
    requestController.current?.abort();
    requestController.current = controller;
    loadingRef.current = true;
    setOperationState({ authKey, loading: true, error: null });
    setQuoteState({ authKey, quote: null });
    try {
      const hasMulti = selection.intervals && selection.intervals.length > 1;
      const monthlyMode =
        selection.rateUnit === "mes" ? billingMode : undefined;
      const result = hasMulti
        ? await publicRentalQuote.createMultiQuote(
            {
              space_id: selection.spaceId,
              intervals: selection.intervals!.map((iv) => ({
                start_at: iv.startAt,
                end_at: iv.endAt,
              })),
              ...(monthlyMode ? { billing_mode: monthlyMode } : {}),
            },
            controller.signal,
          )
        : await publicRentalQuote.createQuote(
            {
              space_id: selection.spaceId,
              start_at: selection.startAt,
              end_at: selection.endAt,
              ...(monthlyMode ? { billing_mode: monthlyMode } : {}),
            },
            controller.signal,
          );
      if (generation !== requestGeneration.current) return;
      if (result.ok) {
        setQuoteState({ authKey, quote: result.value });
      } else {
        setOperationState({
          authKey,
          loading: false,
          error: quoteError(result.error.status),
        });
      }
    } catch {
      if (
        generation === requestGeneration.current &&
        !controller.signal.aborted
      ) {
        setOperationState({
          authKey,
          loading: false,
          error: "No fue posible calcular una cotización. Intenta nuevamente.",
        });
      }
    } finally {
      if (generation === requestGeneration.current) {
        requestController.current = null;
        loadingRef.current = false;
        setOperationState((previous) =>
          previous?.authKey === authKey
            ? { ...previous, loading: false }
            : previous,
        );
      }
    }
  }, [
    auth.status,
    currentAuthKey,
    selection.endAt,
    selection.spaceId,
    selection.startAt,
  ]);

  const abortInFlightQuoteRequest = useCallback(() => {
    if (!requestController.current && !loadingRef.current) return false;
    requestGeneration.current += 1;
    requestController.current?.abort();
    requestController.current = null;
    loadingRef.current = false;
    return true;
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    if (
      auth.status === "authenticated" &&
      autoRequestedKeyRef.current !== autoRequestKey
    ) {
      void Promise.resolve().then(() => {
        if (controller.signal.aborted) return;
        if (autoRequestedKeyRef.current === autoRequestKey) return;
        autoRequestedKeyRef.current = autoRequestKey;
        void requestQuote();
      });
    }

    return () => {
      controller.abort();
      if (abortInFlightQuoteRequest()) {
        autoRequestedKeyRef.current = null;
      }
    };
  }, [abortInFlightQuoteRequest, auth.status, autoRequestKey, requestQuote]);

  const feeBreakdown =
    quote && quote.subtotal_clp !== undefined && quote.subtotal_clp !== null
      ? calculateBuyerFeeBreakdown(Number(quote.subtotal_clp))
      : null;
  const formattedBase = feeBreakdown
    ? formatPublicClp(feeBreakdown.baseClp)
    : null;
  const formattedCommission = feeBreakdown
    ? formatPublicClp(feeBreakdown.commissionClp)
    : null;
  const formattedVat = feeBreakdown
    ? formatPublicClp(feeBreakdown.vatClp)
    : null;
  const formattedServiceFee = feeBreakdown
    ? formatPublicClp(feeBreakdown.serviceFeeClp)
    : null;
  const formattedTotal = feeBreakdown
    ? formatPublicClp(feeBreakdown.totalClp)
    : null;
  return (
    <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[1.35fr_0.9fr]">
      <section
        aria-labelledby="marketplace-checkout-title"
        className="min-w-0 space-y-5 rounded-2xl border border-[#ded5cc] bg-white p-5 text-[#332720] shadow-sm sm:p-6"
      >
        <header className="space-y-1.5">
          <p className="text-xs font-semibold uppercase tracking-wider text-[#a44731]">
            Solicitud desde el marketplace
          </p>
          <h3
            id="marketplace-checkout-title"
            className="text-xl font-semibold tracking-tight text-[#332720]"
          >
            Cotización oficial y pago
          </h3>
          <p className="text-sm text-[#66574e]">
            EspaciGo confirma precio y disponibilidad desde el servidor antes de
            iniciar el pago.
          </p>
        </header>

        {error ? (
          <p
            role="alert"
            className="flex items-start gap-2 rounded-xl border border-red-300 bg-red-50 p-3.5 text-sm text-red-800"
          >
            <AlertCircle
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {error}
          </p>
        ) : null}

        {!quote ? (
          <div className="space-y-4">
            {auth.status === "checking" ? (
              <p
                role="status"
                className="flex items-center gap-2 text-sm text-[#66574e]"
              >
                <Loader2
                  className="size-4 animate-spin text-[#a44731]"
                  aria-hidden="true"
                />
                Comprobando tu sesión…
              </p>
            ) : auth.status !== "authenticated" ? (
              <p
                role="status"
                className="rounded-xl border border-[#e8c5b5] bg-[#fbf7f2] p-3.5 text-sm text-[#332720]"
              >
                Inicia sesión para calcular la cotización oficial de tu
                arriendo.
              </p>
            ) : null}

            {selection.rateUnit === "mes" && (
              <fieldset className="space-y-2 rounded-lg border border-[#e8c5b5] bg-[#fbf7f2] p-3 text-xs">
                <legend className="px-1 font-semibold text-[#332720]">
                  Modalidad de pago mensual
                </legend>
                <div className="grid gap-2 sm:grid-cols-2">
                  <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-[#e8c5b5] bg-white p-2.5">
                    <input
                      type="radio"
                      name="monthly-billing-mode"
                      value="full"
                      checked={billingMode === "full"}
                      onChange={() => setBillingMode("full")}
                      className="mt-0.5"
                    />
                    <span>
                      <strong className="block text-[#332720]">
                        Pagar todo junto
                      </strong>
                      <span className="text-[#66574e]">
                        Abonas el total de los meses en un solo pago.
                      </span>
                    </span>
                  </label>
                  <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-[#e8c5b5] bg-white p-2.5">
                    <input
                      type="radio"
                      name="monthly-billing-mode"
                      value="monthly_installments"
                      checked={billingMode === "monthly_installments"}
                      onChange={() => setBillingMode("monthly_installments")}
                      className="mt-0.5"
                    />
                    <span>
                      <strong className="block text-[#332720]">
                        Pagar mes a mes
                      </strong>
                      <span className="text-[#66574e]">
                        Pagas el primer mes ahora y se guardan las cuotas
                        mensuales restantes.
                      </span>
                    </span>
                  </label>
                </div>
              </fieldset>
            )}
            <Button
              type="button"
              className="min-h-11 w-full rounded-xl bg-[#a44731] px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] disabled:border disabled:border-[#e8c5b5] disabled:bg-[#f5e3db] disabled:text-[#702a1b] disabled:opacity-100"
              onClick={() => void requestQuote()}
              disabled={loading || auth.status !== "authenticated"}
            >
              {loading ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : null}
              {loading
                ? "Validando precio y disponibilidad…"
                : "Calcular cotización oficial"}
            </Button>

            {auth.status !== "authenticated" && auth.status !== "checking" ? (
              <Link
                href={`/acceso?return_to=${encodeURIComponent(signInReturn)}`}
                className="inline-flex min-h-11 w-full items-center justify-center rounded-xl bg-[#a44731] px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#1d4ed8]"
              >
                Iniciar sesión para cotizar
              </Link>
            ) : null}
          </div>
        ) : (
          <div className="space-y-5 rounded-xl border border-[#e8c5b5] bg-[#fbf7f2] p-4 sm:p-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h4 className="inline-flex items-center gap-2 rounded-full border border-[#e8c5b5] bg-[#f5e3db] px-3 py-1 text-sm font-semibold text-[#702a1b]">
                <ShieldCheck
                  className="size-4 text-[#a44731]"
                  aria-hidden="true"
                />
                Cotización oficial
              </h4>
              <ReservationCountdown
                expiresAt={quote.expires_at}
                size="lg"
                onExpire={() => setExpiredQuoteId(quote.id)}
              />
            </div>

            <div className="space-y-2 text-sm">
              <p className="font-semibold text-[#332720]">
                {quote.space_title}
              </p>
              <p className="text-[#66574e]">
                {quote.units} {rateUnitLabel(quote.rate_unit)}
                {Number(quote.units) === 1 ? "" : "s"}
                {(!quote.intervals || quote.intervals.length <= 1) &&
                  ` · ${rentalInterval(quote)}`}
              </p>
              {quote.intervals && quote.intervals.length > 1 ? (
                <div className="space-y-1.5 rounded-lg border border-[#e8c5b5] bg-white/70 p-2.5 text-xs">
                  <p className="font-semibold uppercase tracking-wider text-[#66574e]">
                    Desglose de días y horarios ({quote.intervals.length}{" "}
                    tramos)
                  </p>
                  <ul className="space-y-1">
                    {quote.intervals.map((iv) => {
                      const rowSubtotal = formatPublicClp(
                        (
                          BigInt(quote.unit_price_clp) * BigInt(iv.units)
                        ).toString(),
                      );
                      return (
                        <li
                          key={`${iv.start_at}-${iv.end_at}`}
                          className="flex flex-wrap items-center justify-between gap-2 text-[#332720]"
                        >
                          <span className="tabular-nums">
                            {new Intl.DateTimeFormat("es-CL", {
                              dateStyle: "medium",
                              timeStyle: "short",
                              timeZone: quote.time_zone,
                            }).formatRange(
                              new Date(iv.start_at),
                              new Date(iv.end_at),
                            )}
                          </span>
                          <span className="font-semibold tabular-nums">
                            {iv.units} {rateUnitLabel(quote.rate_unit)}
                            {Number(iv.units) === 1 ? "" : "s"}
                            {rowSubtotal ? ` · ${rowSubtotal} CLP` : ""}
                          </span>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ) : null}
              <div className="space-y-1.5 border-t border-[#e8c5b5] pt-3">
                <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
                  <span>Precio original del espacio</span>
                  <span className="font-medium tabular-nums text-[#332720]">
                    {formattedBase
                      ? `${formattedBase} CLP`
                      : "Monto no disponible"}
                  </span>
                </div>
                <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
                  <span>Comisión de servicio (3%)</span>
                  <span className="font-medium tabular-nums text-[#332720]">
                    {formattedCommission ? `${formattedCommission} CLP` : "—"}
                  </span>
                </div>
                <div className="flex items-baseline justify-between gap-3 text-[#66574e]">
                  <span>IVA sobre comisión (19%)</span>
                  <span className="font-medium tabular-nums text-[#332720]">
                    {formattedVat ? `${formattedVat} CLP` : "—"}
                  </span>
                </div>
                <div className="flex items-baseline justify-between gap-3 border-t border-dashed border-[#e8c5b5] pt-1.5 text-[#66574e]">
                  <span>Comisión total con IVA (3,57%)</span>
                  <span className="font-medium tabular-nums text-[#332720]">
                    {formattedServiceFee ? `${formattedServiceFee} CLP` : "—"}
                  </span>
                </div>
                <div className="flex items-baseline justify-between gap-3 border-t border-[#e8c5b5] pt-2.5">
                  <span className="font-semibold text-[#332720]">
                    Total a pagar
                  </span>
                  <span className="text-xl font-bold tabular-nums text-[#702a1b]">
                    {formattedTotal
                      ? `${formattedTotal} CLP`
                      : "Monto no disponible"}
                  </span>
                </div>
              </div>
            </div>

            <ContractCheckoutGate
              quoteId={quote.id}
              checkoutApiEnabled={checkoutApiEnabled}
              walletPurchaseEnabled={walletPurchaseEnabled}
            >
              <p className="text-xs leading-relaxed text-[#66574e]">
                {checkoutApiEnabled
                  ? walletPurchaseEnabled
                    ? "Elige pagar con tarjeta aquí o ingresar a tu cuenta de Mercado Pago."
                    : "Completarás el pago en EspaciGo. Mercado Pago procesa la transacción."
                  : "El pago continúa en el sitio seguro de Mercado Pago y EspaciGo confirma el estado al regresar."}
              </p>
              {expiredQuoteId === quote.id ? (
                <div className="space-y-3 rounded-xl border border-red-300 bg-red-50 p-3.5 text-sm text-red-800">
                  <p>
                    El tiempo de 6 minutos para esta reserva venció y el horario
                    volvió a quedar libre en el calendario.
                  </p>
                  <Button
                    type="button"
                    onClick={() => {
                      setExpiredQuoteId(null);
                      void requestQuote();
                    }}
                    className="min-h-11 w-full rounded-xl bg-[#a44731] px-4 py-2.5 text-sm font-semibold text-white hover:bg-[#843522]"
                  >
                    Solicitar nueva cotización
                  </Button>
                </div>
              ) : walletPurchaseEnabled || checkoutApiEnabled ? (
                <Link
                  href={`/reservas/checkout/opciones/${encodeURIComponent(quote.id)}`}
                  data-mp-checkout-cta="checkout-options"
                  className="inline-flex min-h-12 w-full items-center justify-center gap-2 rounded-xl bg-[#a44731] px-5 py-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#1d4ed8]"
                >
                  Ir a pagar
                </Link>
              ) : checkoutApiEnabled ? (
                <Link
                  href={`/reservas/checkout/tarjeta/${encodeURIComponent(quote.id)}`}
                  data-mp-checkout-cta="checkout-api"
                  className="inline-flex min-h-12 w-full items-center justify-center gap-2 rounded-xl bg-[#a44731] px-5 py-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#1d4ed8]"
                >
                  Pagar con Mercado Pago
                </Link>
              ) : (
                <CheckoutProButton
                  quoteId={quote.id}
                  expiresAt={quote.expires_at}
                />
              )}
            </ContractCheckoutGate>

            {selection.rateUnit === "mes" && (
              <fieldset className="space-y-2 rounded-lg border border-[#e8c5b5] bg-[#fbf7f2] p-3 text-xs">
                <legend className="px-1 font-semibold text-[#332720]">
                  Modalidad de pago mensual
                </legend>
                <div className="grid gap-2 sm:grid-cols-2">
                  <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-[#e8c5b5] bg-white p-2.5">
                    <input
                      type="radio"
                      name="monthly-billing-mode"
                      value="full"
                      checked={billingMode === "full"}
                      onChange={() => setBillingMode("full")}
                      className="mt-0.5"
                    />
                    <span>
                      <strong className="block text-[#332720]">
                        Pagar todo junto
                      </strong>
                      <span className="text-[#66574e]">
                        Abonas el total de los meses en un solo pago.
                      </span>
                    </span>
                  </label>
                  <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-[#e8c5b5] bg-white p-2.5">
                    <input
                      type="radio"
                      name="monthly-billing-mode"
                      value="monthly_installments"
                      checked={billingMode === "monthly_installments"}
                      onChange={() => setBillingMode("monthly_installments")}
                      className="mt-0.5"
                    />
                    <span>
                      <strong className="block text-[#332720]">
                        Pagar mes a mes
                      </strong>
                      <span className="text-[#66574e]">
                        Pagas el primer mes ahora y se guardan las cuotas
                        mensuales restantes.
                      </span>
                    </span>
                  </label>
                </div>
              </fieldset>
            )}
            <Button
              type="button"
              variant="outline"
              className="min-h-10 w-full rounded-xl border-[#ded5cc] bg-white text-[#332720] hover:bg-[#f5e3db] hover:text-[#702a1b]"
              onClick={() => {
                setQuoteState({ authKey: currentAuthKey, quote: null });
                setOperationState({
                  authKey: currentAuthKey,
                  loading: false,
                  error: null,
                });
              }}
            >
              Solicitar otra cotización
            </Button>
          </div>
        )}
      </section>

      <CheckoutSummaryCard
        spaceTitle={quote?.space_title}
        startAt={quote?.start_at ?? selection.startAt}
        endAt={quote?.end_at ?? selection.endAt}
        timeZone={quote?.time_zone ?? selection.timeZone}
        duration={quote ? quote.units : selection.duration}
        rateUnit={quote?.rate_unit ?? selection.rateUnit}
        unitPriceClp={quote?.unit_price_clp}
        baseClp={feeBreakdown?.baseClp ?? quote?.subtotal_clp}
        commissionClp={feeBreakdown?.commissionClp}
        vatClp={feeBreakdown?.vatClp}
        serviceFeeClp={feeBreakdown?.serviceFeeClp}
        totalClp={feeBreakdown?.totalClp ?? quote?.subtotal_clp}
        expiresAt={quote?.expires_at}
        className="lg:sticky lg:top-6"
      />
    </div>
  );
}
