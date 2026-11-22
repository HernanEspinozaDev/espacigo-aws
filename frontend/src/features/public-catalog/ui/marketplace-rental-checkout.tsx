"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, Loader2, ShieldCheck } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import type { RentalQuote } from "../domain/rental-quote";
import { formatPublicClp, rateUnitLabel } from "../domain/presentation";
import { publicRentalQuote } from "../infrastructure/rental-quote-runtime";
import { CheckoutProButton } from "@/features/rental-checkout/ui/checkout-pro-button";
import { CheckoutSummaryCard } from "@/features/rental-checkout/ui/checkout-flow-chrome";
import { ContractCheckoutGate } from "@/features/commercial-contracts/ui/contract-checkout-gate";

export interface MarketplaceCheckoutSelection {
  spaceId: string;
  startAt: string;
  endAt: string;
  timeZone: string;
  duration?: number;
  rateUnit?: "hora" | "dia" | "mes";
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
    quote: RentalQuote | null;
  } | null>(null);
  const [operationState, setOperationState] = useState<{
    authKey: string;
    loading: boolean;
    error: string | null;
  } | null>(null);
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
      const result = await publicRentalQuote.createQuote(
        {
          space_id: selection.spaceId,
          start_at: selection.startAt,
          end_at: selection.endAt,
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

  const formattedTotal = quote ? formatPublicClp(quote.subtotal_clp) : null;
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
              <p className="text-xs font-medium text-[#66574e]">
                Vence{" "}
                {new Intl.DateTimeFormat("es-CL", {
                  dateStyle: "short",
                  timeStyle: "short",
                  timeZone: quote.time_zone || "America/Santiago",
                }).format(new Date(quote.expires_at))}
              </p>
            </div>

            <div className="space-y-1.5 text-sm">
              <p className="font-semibold text-[#332720]">
                {quote.space_title}
              </p>
              <p className="text-[#66574e]">
                {quote.units} {rateUnitLabel(quote.rate_unit)}
                {Number(quote.units) === 1 ? "" : "s"} · {rentalInterval(quote)}
              </p>
              <div className="flex items-baseline justify-between gap-3 border-t border-[#e8c5b5] pt-3">
                <span className="font-medium text-[#332720]">
                  Total del arriendo
                </span>
                <span className="text-xl font-bold tabular-nums text-[#702a1b]">
                  {formattedTotal
                    ? `${formattedTotal} CLP`
                    : "Monto no disponible"}
                </span>
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
              {walletPurchaseEnabled || checkoutApiEnabled ? (
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
        baseClp={quote?.subtotal_clp}
        totalClp={quote?.subtotal_clp}
        expiresAt={quote?.expires_at}
        className="lg:sticky lg:top-6"
      />
    </div>
  );
}
