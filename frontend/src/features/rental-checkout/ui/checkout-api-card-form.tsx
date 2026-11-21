"use client";

import Script from "next/script";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Clock3,
  Loader2,
  Lock,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { cn } from "@/lib/utils";
import { CheckoutSummaryCard } from "./checkout-flow-chrome";

const SDK_URL = "https://sdk.mercadopago.com/js/v2";
const CHECKOUT_LOG_PREFIX = "[EspaciGo Checkout API]";
const CARD_HOSTS = [
  ["form-checkout-api__cardNumber", "El número de tarjeta"],
  ["form-checkout-api__expirationDate", "La fecha de vencimiento"],
  ["form-checkout-api__securityCode", "El código de seguridad"],
] as const;

type PaymentStatus =
  | "not_started"
  | "approved"
  | "pending"
  | "in_process"
  | "rejected"
  | "provider_error"
  | "reconciliation_required"
  | "expired"
  | "cancelled";

interface CheckoutSnapshot {
  quoteId: string;
  status: PaymentStatus;
  amountClp: number;
  paymentId: string | null;
  statusDetail: string | null;
  spaceTitle: string | null;
  startAt: string | null;
  endAt: string | null;
  timeZone: string | null;
  expiresAt: string | null;
  units: number | null;
  rateUnit: "hora" | "dia" | "mes" | undefined;
  cardTokenization: {
    mode: "seller_public_key";
    publicKey: string | null;
    reconnectRequired: boolean;
  } | null;
}

interface SafeResult {
  status: PaymentStatus | null;
  paymentId: string | null;
  message: string;
  errorCode: string | null;
  providerHttpStatus: number | null;
  providerCode: string | null;
  providerRequestId: string | null;
}

type CardFormData = {
  token: string;
  paymentMethodId: string;
  issuerId?: string;
  cardholderEmail: string;
  identificationType: string;
  identificationNumber: string;
};

type MercadoPagoCardForm = {
  getCardFormData(): CardFormData;
};

declare global {
  interface Window {
    MercadoPago?: new (
      publicKey: string,
      options?: { locale?: string },
    ) => {
      cardForm(options: Record<string, unknown>): MercadoPagoCardForm;
    };
  }
}

const FIELD_INPUT_CLASS =
  "h-11 w-full rounded-lg border border-[#8a796b] bg-white px-3 text-sm font-normal text-[#332720] shadow-xs transition-colors placeholder:text-[#66574e]/75 focus-visible:border-[#a44731] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#a44731]/25";

const SECURE_FIELD_HOST_CLASS =
  "flex h-11 w-full items-center rounded-lg border border-[#8a796b] bg-white px-3 shadow-xs transition-colors focus-within:border-[#a44731] focus-within:ring-2 focus-within:ring-[#a44731]/25 [&>iframe]:h-full [&>iframe]:w-full";

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Browser diagnostics intentionally contain only fixed stage names and
// validated technical identifiers. Never pass SDK errors or payment form data.
function logCheckoutStage(
  stage: string,
  details: Record<string, string | number | boolean | null> = {},
) {
  console.info(CHECKOUT_LOG_PREFIX, { stage, ...details });
}

function safeIdentifier(value: unknown): string | null {
  return typeof value === "string" && /^[A-Za-z0-9._:-]{1,96}$/.test(value)
    ? value
    : null;
}

function safeProviderCode(value: unknown): string | null {
  return typeof value === "string" && /^\d{1,6}$/.test(value) ? value : null;
}

function safeCheckoutErrorCode(value: unknown): string | null {
  const knownCodes = [
    "checkout_not_configured",
    "unauthenticated",
    "forbidden",
    "quote_expired",
    "host_mp_not_connected",
    "availability_changed",
    "idempotency_conflict",
    "payment_method_unavailable",
  ];
  return typeof value === "string" && knownCodes.includes(value) ? value : null;
}

function safeHttpStatus(value: unknown): number | null {
  return typeof value === "number" &&
    Number.isInteger(value) &&
    value >= 100 &&
    value <= 599
    ? value
    : null;
}

function safeDateTime(value: unknown): string | null {
  return typeof value === "string" && Number.isFinite(Date.parse(value))
    ? value
    : null;
}

function safeTimeZone(value: unknown): string | null {
  if (typeof value !== "string" || value.length > 64) return null;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return value;
  } catch {
    return null;
  }
}

function statusMessage(status: PaymentStatus, detail: string | null): string {
  if (status === "approved") {
    return "Mercado Pago aprobó el pago. EspaciGo confirmó la reserva en tu cuenta.";
  }
  if (status === "pending" || status === "in_process") {
    return "El pago quedó pendiente de confirmación. No vuelvas a enviar el formulario; consulta su estado en unos segundos.";
  }
  if (status === "reconciliation_required") {
    return "EspaciGo está verificando el resultado con Mercado Pago. No vuelvas a enviar el pago.";
  }
  if (status === "expired") {
    return "Esta cotización venció. Solicita una cotización nueva antes de pagar.";
  }
  if (status === "cancelled") {
    return "El pago fue cancelado. Si necesitas continuar, solicita una cotización nueva.";
  }
  if (status === "rejected") {
    if (detail === "cc_rejected_bad_filled_card_number") {
      return "Revisa los datos de la tarjeta e intenta nuevamente desde una nueva cotización.";
    }
    if (detail === "cc_rejected_insufficient_amount") {
      return "El medio de pago no tiene fondos suficientes. Prueba con otro medio de pago.";
    }
    return "Mercado Pago no aprobó el pago. Revisa los datos o prueba con otro medio.";
  }
  return "No se pudo confirmar el pago. Consulta su estado antes de volver a intentarlo.";
}

function providerErrorMessage(providerCode: string | null): string | null {
  switch (providerCode) {
    case "3":
      return "Mercado Pago indicó que el token debe pertenecer al entorno de prueba. Verifica la configuración de prueba antes de continuar.";
    case "2009":
      return "Mercado Pago no pudo identificar el emisor de la tarjeta. Revisa el medio de pago y consulta el estado antes de volver a cotizar.";
    case "2034":
      return "El proveedor no acepta la combinación de cuentas participantes. Contacta soporte con este código antes de volver a intentar.";
    case "2059":
      return "Mercado Pago rechazó la autorización de la cuenta anfitriona. Revisa la vinculación OAuth antes de otro intento.";
    case "2062":
      return "El token de tarjeta no es válido o ya no se puede usar. No lo reutilices; consulta el estado antes de iniciar otro intento.";
    case "2067":
      return "Mercado Pago no pudo validar el medio de pago o los datos del pagador. Revisa la tarjeta y el documento antes de iniciar otra cotización.";
    case "4033":
      return "Mercado Pago no acepta las cuotas de esta operación. Esta compra se envía en una cuota; el equipo debe revisar la configuración.";
    case "4038":
      return "Mercado Pago rechazó la configuración de esta operación. El equipo debe revisar la respuesta técnica antes de otro intento.";
    case "4039":
      return "Mercado Pago rechazó la configuración de esta operación. El equipo debe revisar la respuesta técnica antes de otro intento.";
    case "4050":
      return "El correo del pagador no tiene un formato válido. Revisa la dirección antes de iniciar otra cotización.";
    case "4051":
      return "El correo del pagador supera el límite de 254 caracteres. Usa una dirección más corta antes de iniciar otra cotización.";
    default:
      return null;
  }
}

function summaryStatusLabel(status: PaymentStatus | null): string {
  switch (status) {
    case "approved":
      return "Pago aprobado";
    case "pending":
    case "in_process":
      return "Pendiente de confirmación";
    case "reconciliation_required":
      return "En verificación";
    case "rejected":
      return "Pago rechazado";
    case "expired":
      return "Cotización vencida";
    case "cancelled":
      return "Pago cancelado";
    case "provider_error":
      return "Error de procesamiento";
    case "not_started":
      return "Listo para pagar";
    default:
      return "Consulta requerida";
  }
}

function errorMessage(code: string | null, status: number): string {
  if (code === "checkout_not_configured") {
    return "Checkout API está deshabilitado en este entorno.";
  }
  if (code === "unauthenticated") {
    return "Tu sesión venció. Inicia sesión antes de pagar.";
  }
  if (code === "forbidden") {
    return "Esta cotización no pertenece a tu cuenta.";
  }
  if (code === "quote_expired") {
    return "La cotización venció. Solicita una nueva cotización.";
  }
  if (code === "host_mp_not_connected") {
    return "El anfitrión todavía no conectó Mercado Pago. No se inició ningún pago.";
  }
  if (code === "availability_changed") {
    return "El horario ya no está disponible. Selecciona nuevas fechas.";
  }
  if (code === "idempotency_conflict") {
    return "Ya existe una operación de pago distinta para esta cotización. Consulta su estado antes de continuar.";
  }
  if (code === "payment_method_unavailable") {
    return "No hay un medio de pago disponible para esta operación de prueba.";
  }
  if (status >= 500) {
    return "No se pudo consultar EspaciGo. Vuelve a consultar el estado antes de intentarlo.";
  }
  return "No se pudo preparar el pago. Consulta el estado de la cotización.";
}

function parseSnapshot(
  value: unknown,
  expectedQuoteId: string,
): CheckoutSnapshot | null {
  if (!record(value) || !record(value.data)) return null;
  const data = value.data;
  const statuses: PaymentStatus[] = [
    "not_started",
    "approved",
    "pending",
    "in_process",
    "rejected",
    "provider_error",
    "reconciliation_required",
    "expired",
    "cancelled",
  ];
  if (
    data.quote_id !== expectedQuoteId ||
    !statuses.includes(data.status as PaymentStatus) ||
    typeof data.amount_clp !== "number" ||
    !Number.isSafeInteger(data.amount_clp) ||
    data.amount_clp <= 0 ||
    (data.payment_id !== undefined &&
      data.payment_id !== null &&
      !safeIdentifier(data.payment_id)) ||
    (data.status_detail !== undefined &&
      data.status_detail !== null &&
      !safeIdentifier(data.status_detail))
  ) {
    return null;
  }
  let cardTokenization: CheckoutSnapshot["cardTokenization"] = null;
  if (data.card_tokenization !== undefined) {
    if (
      !record(data.card_tokenization) ||
      data.card_tokenization.mode !== "seller_public_key" ||
      (data.card_tokenization.reconnect_required !== undefined &&
        typeof data.card_tokenization.reconnect_required !== "boolean")
    ) {
      return null;
    }
    const rawPublicKey = data.card_tokenization.public_key;
    const publicKey =
      typeof rawPublicKey === "string" &&
      /^[A-Za-z0-9_-]{12,256}$/.test(rawPublicKey)
        ? rawPublicKey
        : null;
    cardTokenization = {
      mode: "seller_public_key",
      publicKey,
      reconnectRequired:
        data.card_tokenization.reconnect_required === true || !publicKey,
    };
  }
  return {
    quoteId: expectedQuoteId,
    status: data.status as PaymentStatus,
    amountClp: data.amount_clp,
    paymentId: safeIdentifier(data.payment_id),
    statusDetail: safeIdentifier(data.status_detail),
    spaceTitle:
      typeof data.space_title === "string" && data.space_title.trim()
        ? data.space_title.trim().slice(0, 200)
        : null,
    startAt: safeDateTime(data.start_at),
    endAt: safeDateTime(data.end_at),
    timeZone: safeTimeZone(data.time_zone),
    expiresAt: safeDateTime(data.expires_at),
    units:
      typeof data.units === "number" &&
      Number.isFinite(data.units) &&
      data.units > 0
        ? data.units
        : null,
    rateUnit:
      data.rate_unit === "hora" ||
      data.rate_unit === "dia" ||
      data.rate_unit === "mes"
        ? data.rate_unit
        : undefined,
    cardTokenization,
  };
}

function safeResult(value: unknown, httpStatus: number): SafeResult | null {
  if (!record(value)) return null;
  if (record(value.error)) {
    return {
      status: null,
      paymentId: null,
      message: errorMessage(safeIdentifier(value.error.code), httpStatus),
      errorCode: safeIdentifier(value.error.code),
      providerHttpStatus: null,
      providerCode: null,
      providerRequestId: null,
    };
  }
  const data = record(value.data) ? value.data : value;
  const statuses: PaymentStatus[] = [
    "approved",
    "pending",
    "in_process",
    "rejected",
    "provider_error",
    "reconciliation_required",
    "expired",
    "cancelled",
  ];
  if (!statuses.includes(data.status as PaymentStatus)) return null;
  return {
    status: data.status as PaymentStatus,
    paymentId: safeIdentifier(data.payment_id),
    message:
      (data.status === "provider_error"
        ? providerErrorMessage(safeIdentifier(data.provider_code))
        : null) ??
      statusMessage(
        data.status as PaymentStatus,
        safeIdentifier(data.status_detail),
      ),
    errorCode: safeIdentifier(data.error_code),
    providerHttpStatus: safeHttpStatus(data.provider_http_status),
    providerCode: safeIdentifier(data.provider_code),
    providerRequestId: safeIdentifier(data.provider_request_id),
  };
}

function fieldsMounted(): boolean {
  return CARD_HOSTS.every(([id]) => {
    const host = document.getElementById(id);
    const iframe = host?.querySelectorAll("iframe");
    const rect = host?.getBoundingClientRect();
    return Boolean(
      host && iframe?.length === 1 && rect && rect.width > 0 && rect.height > 0,
    );
  });
}

function CardFormSkeleton() {
  return (
    <div
      role="status"
      aria-busy="true"
      className="space-y-4 rounded-xl border border-[#ded5cc] bg-[#fbf7f2] p-4 sm:p-5"
    >
      <span className="sr-only">Consultando cotización y estado…</span>
      <div className="space-y-2">
        <div className="h-4 w-36 animate-pulse rounded bg-[#ded5cc]" />
        <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
      </div>
      <div className="space-y-2">
        <div className="h-4 w-32 animate-pulse rounded bg-[#ded5cc]" />
        <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <div className="h-4 w-24 animate-pulse rounded bg-[#ded5cc]" />
          <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
        </div>
        <div className="space-y-2">
          <div className="h-4 w-36 animate-pulse rounded bg-[#ded5cc]" />
          <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-[140px_1fr]">
        <div className="space-y-2">
          <div className="h-4 w-28 animate-pulse rounded bg-[#ded5cc]" />
          <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
        </div>
        <div className="space-y-2">
          <div className="h-4 w-36 animate-pulse rounded bg-[#ded5cc]" />
          <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
        </div>
      </div>
      <div className="space-y-2">
        <div className="h-4 w-32 animate-pulse rounded bg-[#ded5cc]" />
        <div className="h-11 w-full animate-pulse rounded-lg bg-[#e8c5b5]/60" />
      </div>
      <div className="h-12 w-full animate-pulse rounded-xl bg-[#e8c5b5]" />
    </div>
  );
}

function SummaryCardSkeleton() {
  return (
    <aside
      aria-hidden="true"
      className="min-w-0 space-y-4 rounded-2xl border border-[#ded5cc] bg-white p-5 shadow-sm lg:sticky lg:top-6"
    >
      <div className="h-5 w-40 animate-pulse rounded bg-[#ded5cc]" />
      <div className="space-y-3 rounded-xl border border-[#ded5cc] bg-[#fbf7f2] p-3.5">
        <div className="h-3.5 w-24 animate-pulse rounded bg-[#ded5cc]" />
        <div className="h-4 w-full animate-pulse rounded bg-[#e8c5b5]/60" />
        <div className="h-5 w-28 animate-pulse rounded-full bg-[#e8c5b5]/70" />
      </div>
      <div className="flex items-baseline justify-between border-t border-[#ded5cc] pt-3.5">
        <div className="h-4 w-20 animate-pulse rounded bg-[#ded5cc]" />
        <div className="h-6 w-28 animate-pulse rounded bg-[#ded5cc]" />
      </div>
    </aside>
  );
}

function CheckoutResultCard({
  quoteId,
  status,
  message,
  paymentId,
  providerHttpStatus,
  providerCode,
  providerRequestId,
  isRefreshing,
  onRefresh,
}: {
  quoteId: string;
  status: PaymentStatus | null;
  message: string;
  paymentId: string | null;
  providerHttpStatus: number | null;
  providerCode: string | null;
  providerRequestId: string | null;
  isRefreshing: boolean;
  onRefresh: () => void;
}) {
  if (status === "approved") {
    return (
      <div
        role="status"
        className="space-y-4 rounded-xl border border-emerald-300 bg-emerald-50/90 p-5 text-emerald-950"
      >
        <div className="flex items-start gap-3">
          <CheckCircle2
            className="mt-0.5 size-5 shrink-0 text-emerald-700"
            aria-hidden="true"
          />
          <div className="space-y-1 text-sm">
            <p className="font-semibold">Pago aprobado</p>
            <p>{message}</p>
            {paymentId ? (
              <p className="pt-1 font-mono text-xs text-emerald-900/90">
                Referencia de pago: {paymentId}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3 pt-1">
          <Link
            href="/reservas"
            className="inline-flex min-h-11 items-center justify-center rounded-xl bg-[#a44731] px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#1d4ed8]"
          >
            Ver mis reservas
          </Link>
          <Link
            href={`/reservas/checkout/${quoteId}`}
            className="inline-flex min-h-11 items-center text-sm font-medium text-emerald-950 underline underline-offset-4 hover:text-emerald-800"
          >
            Volver al estado de la cotización
          </Link>
        </div>
      </div>
    );
  }

  if (status === "pending" || status === "in_process") {
    return (
      <div
        role="status"
        className="space-y-4 rounded-xl border border-amber-300 bg-amber-50 p-5 text-[#332720]"
      >
        <div className="flex items-start gap-3">
          <Clock3
            className="mt-0.5 size-5 shrink-0 text-amber-700"
            aria-hidden="true"
          />
          <div className="space-y-1 text-sm">
            <p className="font-semibold text-[#332720]">
              Pago pendiente de confirmación
            </p>
            <p className="text-[#332720]">{message}</p>
            {paymentId ? (
              <p className="pt-1 font-mono text-xs text-[#66574e]">
                Operación registrada: {paymentId}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3 pt-1">
          <Button
            type="button"
            variant="outline"
            onClick={onRefresh}
            disabled={isRefreshing}
            className="min-h-11 border-[#ded5cc] bg-white text-[#332720] hover:bg-[#fbf7f2] hover:text-[#702a1b]"
          >
            {isRefreshing ? (
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            ) : (
              <RefreshCw className="size-4" aria-hidden="true" />
            )}
            Consultar estado
          </Button>
          <Link
            href={`/reservas/checkout/${quoteId}`}
            className="inline-flex min-h-11 items-center text-sm font-medium text-[#332720] underline underline-offset-4 hover:text-[#a44731]"
          >
            Volver al estado de la cotización
          </Link>
        </div>
      </div>
    );
  }

  const errorHeading =
    status === "reconciliation_required"
      ? "Pago en verificación"
      : status === "expired"
        ? "Cotización vencida"
        : status === "cancelled"
          ? "Pago cancelado"
          : status === "rejected"
            ? "Pago no aprobado"
            : "No se pudo completar el pago";

  return (
    <div
      role="alert"
      className="space-y-4 rounded-xl border border-red-300 bg-red-50 p-5 text-[#332720]"
    >
      <div className="flex items-start gap-3">
        {status === "reconciliation_required" ? (
          <Clock3
            className="mt-0.5 size-5 shrink-0 text-[#a44731]"
            aria-hidden="true"
          />
        ) : (
          <AlertCircle
            className="mt-0.5 size-5 shrink-0 text-red-700"
            aria-hidden="true"
          />
        )}
        <div className="space-y-1 text-sm">
          <p className="font-semibold text-red-800">{errorHeading}</p>
          <p className="text-[#332720]">{message}</p>
          {paymentId ? (
            <p className="pt-1 font-mono text-xs text-[#66574e]">
              Operación registrada: {paymentId}
            </p>
          ) : null}
          {providerHttpStatus ? (
            <p className="pt-0.5 text-xs text-[#66574e]">
              Respuesta técnica: HTTP {providerHttpStatus}
              {providerCode ? ` · ${providerCode}` : ""}
              {providerRequestId ? ` · solicitud ${providerRequestId}` : ""}
            </p>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3 pt-1">
        <Button
          type="button"
          variant="outline"
          onClick={onRefresh}
          disabled={isRefreshing}
          className="min-h-11 border-[#ded5cc] bg-white text-[#332720] hover:bg-[#fbf7f2] hover:text-[#702a1b]"
        >
          {isRefreshing ? (
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          ) : (
            <RefreshCw className="size-4" aria-hidden="true" />
          )}
          Consultar estado
        </Button>
        <Link
          href={`/reservas/checkout/${quoteId}`}
          className="inline-flex min-h-11 items-center text-sm font-medium text-[#332720] underline underline-offset-4 hover:text-[#a44731]"
        >
          Volver al estado de la cotización
        </Link>
        <Link
          href="/explorar"
          className="inline-flex min-h-11 items-center text-sm font-medium text-[#66574e] underline underline-offset-4 hover:text-[#332720]"
        >
          Volver al marketplace
        </Link>
      </div>
    </div>
  );
}

export function CheckoutApiCardForm({
  quoteId,
  enabled,
  publicKey,
}: {
  quoteId: string;
  enabled: boolean;
  publicKey: string;
}) {
  const { snapshot: auth } = useAuth();
  const [checkout, setCheckout] = useState<CheckoutSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SafeResult | null>(null);
  const [sdkLoaded, setSdkLoaded] = useState(false);
  const [sdkLoadError, setSdkLoadError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [paymentAttempted, setPaymentAttempted] = useState(false);
  const [secureFieldsReady, setSecureFieldsReady] = useState(false);
  const [email, setEmail] = useState("");
  const [identificationNumber, setIdentificationNumber] = useState("");
  const [cardholderName, setCardholderName] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const cardFormRef = useRef<MercadoPagoCardForm | null>(null);
  const mountedRef = useRef(false);
  const submitPaymentRef = useRef<(event: Event) => void>(() => undefined);
  const idempotencyKey = useRef<string | null>(null);

  const cardTokenization = checkout?.cardTokenization ?? null;
  const sellerKeyMode = cardTokenization?.mode === "seller_public_key";
  const sellerKeyReconnectRequired =
    sellerKeyMode &&
    (cardTokenization?.reconnectRequired !== false ||
      !cardTokenization?.publicKey);
  const activePublicKey = sellerKeyMode
    ? (cardTokenization?.publicKey ?? "")
    : publicKey;
  // The API snapshot opts into seller OAuth credentials only in the local sandbox.
  // Retain the configured TEST key for older local snapshots, but never fall back
  // to it when the server explicitly selects the seller key flow.
  const isTestMode =
    !sellerKeyReconnectRequired &&
    (sellerKeyMode || publicKey.startsWith("TEST-"));

  const loadCheckout = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      setError(null);
      logCheckoutStage("checkout_status_request", {
        quoteId: safeIdentifier(quoteId),
        signalAborted: signal?.aborted ?? false,
      });
      try {
        const response = await fetch(
          `/api/backend/rental-quotes/${encodeURIComponent(quoteId)}/checkout-api`,
          {
            method: "GET",
            credentials: "same-origin",
            cache: "no-store",
            headers: { Accept: "application/json" },
            signal,
          },
        );
        const body: unknown = await response.json().catch(() => null);
        logCheckoutStage("checkout_status_response", {
          quoteId: safeIdentifier(quoteId),
          httpStatus: response.status,
          ok: response.ok,
        });
        if (!response.ok) {
          const code =
            record(body) && record(body.error)
              ? safeIdentifier(body.error.code)
              : null;
          setError(errorMessage(code, response.status));
          setCheckout(null);
          logCheckoutStage("checkout_status_rejected", {
            quoteId: safeIdentifier(quoteId),
            httpStatus: response.status,
            errorCode: code,
          });
          return;
        }
        const parsed = parseSnapshot(body, quoteId);
        if (!parsed) {
          logCheckoutStage("checkout_status_invalid_response", {
            quoteId: safeIdentifier(quoteId),
            httpStatus: response.status,
          });
          setError(
            "EspaciGo devolvió un estado de pago que no pudimos validar.",
          );
          setCheckout(null);
          return;
        }
        setCheckout(parsed);
        logCheckoutStage("checkout_status_parsed", {
          quoteId: safeIdentifier(quoteId),
          status: parsed.status,
          amountClp: parsed.amountClp,
          sellerKeyMode: parsed.cardTokenization?.mode === "seller_public_key",
          reconnectRequired: parsed.cardTokenization?.reconnectRequired ?? null,
        });
      } catch {
        if (!signal?.aborted) {
          logCheckoutStage("checkout_status_network_error", {
            quoteId: safeIdentifier(quoteId),
          });
          setError("No se pudo consultar el estado de la cotización.");
        }
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [quoteId],
  );

  useEffect(() => {
    if (!enabled || auth.status !== "authenticated") {
      return;
    }
    const controller = new AbortController();
    void Promise.resolve().then(() => {
      if (!controller.signal.aborted) {
        void loadCheckout(controller.signal);
      }
    });
    return () => controller.abort();
  }, [auth.status, enabled, loadCheckout]);

  const submitPayment = useCallback(
    async (event: Event) => {
      event.preventDefault();
      if (
        submitting ||
        sellerKeyReconnectRequired ||
        !checkout ||
        checkout.status !== "not_started" ||
        !cardFormRef.current
      ) {
        logCheckoutStage("card_submit_blocked", {
          quoteId: safeIdentifier(quoteId),
          submitting,
          reconnectRequired: sellerKeyReconnectRequired,
          checkoutStatus: checkout?.status ?? null,
          cardFormReady: Boolean(cardFormRef.current),
        });
        return;
      }
      logCheckoutStage("card_form_submit_received", {
        quoteId: safeIdentifier(quoteId),
      });
      setFormError(null);
      let formData: CardFormData;
      try {
        formData = cardFormRef.current.getCardFormData();
      } catch {
        logCheckoutStage("card_tokenization_error", {
          quoteId: safeIdentifier(quoteId),
        });
        setFormError(
          "Mercado Pago no pudo preparar los datos de pago. Recarga el checkout e inténtalo otra vez.",
        );
        return;
      }
      if (!cardholderName.trim()) {
        setFormError("Escribe el nombre que aparece en la tarjeta.");
        return;
      }
      if (
        !formData.token ||
        !formData.paymentMethodId ||
        !formData.cardholderEmail.trim() ||
        !formData.identificationType.trim() ||
        !formData.identificationNumber.trim()
      ) {
        logCheckoutStage("card_tokenization_incomplete", {
          quoteId: safeIdentifier(quoteId),
          tokenPresent: Boolean(formData.token),
          paymentMethodPresent: Boolean(formData.paymentMethodId),
          payerEmailPresent: Boolean(formData.cardholderEmail.trim()),
          identificationPresent: Boolean(
            formData.identificationType.trim() &&
            formData.identificationNumber.trim(),
          ),
        });
        setFormError(
          "Completa los datos de pago y del titular antes de continuar.",
        );
        return;
      }
      logCheckoutStage("card_tokenization_succeeded", {
        quoteId: safeIdentifier(quoteId),
        paymentMethodPresent: Boolean(formData.paymentMethodId),
        issuerPresent: Boolean(formData.issuerId),
        payerIdentificationPresent: Boolean(formData.identificationNumber),
      });
      idempotencyKey.current ??= crypto.randomUUID();
      setSubmitting(true);
      setPaymentAttempted(true);
      setResult(null);
      logCheckoutStage("payment_create_request", {
        quoteId: safeIdentifier(quoteId),
        method: "POST",
        testMode: isTestMode,
        sellerKeyMode,
      });
      try {
        const response = await fetch(
          `/api/backend/rental-quotes/${encodeURIComponent(quoteId)}/checkout-api`,
          {
            method: "POST",
            credentials: "same-origin",
            cache: "no-store",
            headers: {
              Accept: "application/json",
              "Content-Type": "application/json",
              "Idempotency-Key": idempotencyKey.current,
            },
            body: JSON.stringify({
              token: formData.token,
              payment_method_id: formData.paymentMethodId,
              issuer_id: formData.issuerId || undefined,
              ...(sellerKeyMode
                ? { tokenization_public_key: activePublicKey }
                : {}),
              payer: {
                email: formData.cardholderEmail.trim(),
                identification: {
                  type: formData.identificationType,
                  number: formData.identificationNumber.replace(
                    /[^0-9kK]/g,
                    "",
                  ),
                },
              },
            }),
          },
        );
        logCheckoutStage("payment_create_response", {
          quoteId: safeIdentifier(quoteId),
          httpStatus: response.status,
          ok: response.ok,
        });
        const body: unknown = await response.json().catch(() => null);
        const parsed = safeResult(body, response.status);
        if (!parsed) {
          logCheckoutStage("payment_create_invalid_response", {
            quoteId: safeIdentifier(quoteId),
            httpStatus: response.status,
          });
          setResult({
            status: null,
            paymentId: null,
            message:
              "La respuesta de EspaciGo no se pudo validar. Consulta el estado antes de intentar de nuevo.",
            errorCode: null,
            providerHttpStatus: null,
            providerCode: null,
            providerRequestId: null,
          });
          return;
        }
        logCheckoutStage("payment_create_result", {
          quoteId: safeIdentifier(quoteId),
          status: parsed.status,
          errorCode: safeCheckoutErrorCode(parsed.errorCode),
          providerHttpStatus: parsed.providerHttpStatus,
          providerCode: safeProviderCode(parsed.providerCode),
          providerRequestId: parsed.providerRequestId,
          paymentId: parsed.paymentId,
        });
        if (!response.ok && parsed.status === null) {
          setResult(parsed);
          return;
        }
        setResult(parsed);
        if (
          parsed.status === "pending" ||
          parsed.status === "in_process" ||
          parsed.status === "reconciliation_required" ||
          parsed.status === "approved"
        ) {
          setCheckout((current) =>
            current
              ? {
                  ...current,
                  status: parsed.status!,
                  paymentId: parsed.paymentId,
                }
              : current,
          );
        }
      } catch {
        logCheckoutStage("payment_create_network_error", {
          quoteId: safeIdentifier(quoteId),
        });
        setResult({
          status: null,
          paymentId: null,
          message:
            "No se pudo confirmar el inicio de pago. Consulta el estado antes de intentar de nuevo.",
          errorCode: null,
          providerHttpStatus: null,
          providerCode: null,
          providerRequestId: null,
        });
      } finally {
        setSubmitting(false);
      }
    },
    [
      cardholderName,
      checkout,
      activePublicKey,
      isTestMode,
      quoteId,
      sellerKeyMode,
      sellerKeyReconnectRequired,
      submitting,
    ],
  );

  useEffect(() => {
    submitPaymentRef.current = submitPayment;
  }, [submitPayment]);

  const checkoutStatus = checkout?.status ?? null;
  const checkoutAmountClp = checkout?.amountClp ?? null;

  useEffect(() => {
    if (
      !enabled ||
      !isTestMode ||
      sellerKeyReconnectRequired ||
      !sdkLoaded ||
      checkoutStatus !== "not_started" ||
      checkoutAmountClp === null ||
      !window.MercadoPago ||
      mountedRef.current
    ) {
      return;
    }
    mountedRef.current = true;
    setSdkLoadError(null);
    let cardForm: MercadoPagoCardForm;
    try {
      const mp = new window.MercadoPago(activePublicKey, { locale: "es-CL" });
      logCheckoutStage("card_form_mount_started", {
        quoteId: safeIdentifier(quoteId),
        testMode: isTestMode,
        sellerKeyMode,
      });
      cardForm = mp.cardForm({
        amount: checkoutAmountClp.toFixed(2),
        iframe: true,
        form: {
          id: "form-checkout-api",
          cardNumber: {
            id: "form-checkout-api__cardNumber",
            placeholder: "Número de tarjeta",
          },
          expirationDate: {
            id: "form-checkout-api__expirationDate",
            placeholder: "MM/AA",
          },
          securityCode: {
            id: "form-checkout-api__securityCode",
            placeholder: "CVV",
          },
          cardholderName: { id: "form-checkout-api__cardholderName" },
          issuer: { id: "form-checkout-api__issuer" },
          installments: { id: "form-checkout-api__installments" },
          identificationType: { id: "form-checkout-api__identificationType" },
          identificationNumber: {
            id: "form-checkout-api__identificationNumber",
          },
          cardholderEmail: { id: "form-checkout-api__cardholderEmail" },
        },
        callbacks: {
          onFormMounted: (mountError: unknown) => {
            if (mountError) {
              logCheckoutStage("card_form_mount_error", {
                quoteId: safeIdentifier(quoteId),
              });
              setSdkLoadError(
                "No se pudieron cargar los campos seguros de Mercado Pago. Verifica el SDK e inténtalo más tarde.",
              );
              return;
            }
            window.requestAnimationFrame(() => {
              if (!fieldsMounted()) {
                logCheckoutStage("card_form_fields_missing", {
                  quoteId: safeIdentifier(quoteId),
                });
                setSdkLoadError(
                  "Los campos seguros no se inicializaron completamente.",
                );
              } else {
                logCheckoutStage("card_form_ready", {
                  quoteId: safeIdentifier(quoteId),
                });
                setSdkLoadError(null);
                setSecureFieldsReady(true);
              }
            });
          },
          onSubmit: (event: Event) => {
            void submitPaymentRef.current(event);
          },
        },
      });
    } catch {
      mountedRef.current = false;
      logCheckoutStage("card_form_initialization_error", {
        quoteId: safeIdentifier(quoteId),
      });
      window.setTimeout(() => {
        setSdkLoadError(
          "Mercado Pago no pudo inicializar los campos seguros. Recarga el checkout; si continúa, permite sdk.mercadopago.com y sus campos seguros en tu navegador.",
        );
      }, 0);
      return;
    }
    cardFormRef.current = cardForm;
    const timer = window.setTimeout(() => {
      if (!fieldsMounted()) {
        logCheckoutStage("card_form_mount_timeout", {
          quoteId: safeIdentifier(quoteId),
        });
        setSdkLoadError(
          "Mercado Pago no cargó los campos seguros. Revisa bloqueadores de contenido y vuelve a abrir el checkout.",
        );
      }
    }, 8000);
    return () => {
      window.clearTimeout(timer);
      cardFormRef.current = null;
      mountedRef.current = false;
      setSecureFieldsReady(false);
    };
  }, [
    checkoutAmountClp,
    checkoutStatus,
    enabled,
    isTestMode,
    activePublicKey,
    sellerKeyReconnectRequired,
    sdkLoaded,
    quoteId,
    sellerKeyMode,
  ]);

  const isLoading = loading && enabled && auth.status === "authenticated";
  const canMountCardForm =
    enabled &&
    isTestMode &&
    !sellerKeyReconnectRequired &&
    Boolean(activePublicKey) &&
    checkout?.status === "not_started" &&
    auth.status === "authenticated" &&
    !paymentAttempted;

  const effectiveStatus: PaymentStatus | null =
    result?.status ??
    (checkout && checkout.status !== "not_started" ? checkout.status : null);
  const effectivePaymentId = result?.paymentId ?? checkout?.paymentId ?? null;
  const effectiveMessage =
    result?.message ??
    (checkout && checkout.status !== "not_started"
      ? statusMessage(checkout.status, checkout.statusDetail)
      : null);
  const showResultCard = Boolean(effectiveMessage);

  const summaryStatus = !enabled
    ? "Checkout deshabilitado"
    : auth.status !== "authenticated"
      ? "Sesión requerida"
      : summaryStatusLabel(effectiveStatus ?? checkout?.status ?? null);

  return (
    <div
      data-mp-marketplace-checkout="checkout-api"
      className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[1.35fr_0.9fr]"
    >
      <section
        aria-labelledby="checkout-api-title"
        className="min-w-0 space-y-6 rounded-2xl border border-[#ded5cc] bg-white p-5 text-[#332720] shadow-sm sm:p-7"
      >
        <header className="space-y-2 border-b border-[#ded5cc] pb-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[#a44731]">
              <ShieldCheck className="size-4 shrink-0" aria-hidden="true" />
              Pago seguro con tarjeta
            </p>
            {isTestMode ? (
              <span className="inline-flex items-center gap-1 rounded-full border border-[#e8c5b5] bg-[#f5e3db] px-2.5 py-0.5 text-xs font-semibold text-[#702a1b]">
                Modo de prueba
              </span>
            ) : null}
          </div>
          <h1
            id="checkout-api-title"
            className="text-2xl font-semibold tracking-tight text-[#332720]"
          >
            Pagar con tarjeta
          </h1>
          <p className="flex items-center gap-1.5 text-sm text-[#66574e]">
            <Lock
              className="size-3.5 shrink-0 text-[#a44731]"
              aria-hidden="true"
            />
            Los datos de tu tarjeta se tokenizan directamente en Mercado Pago.
          </p>
        </header>

        {!enabled ? (
          <p
            role="status"
            className="rounded-xl border border-[#ded5cc] bg-[#fbf7f2] p-4 text-sm text-[#66574e]"
          >
            Checkout API está desactivado en este entorno.
          </p>
        ) : null}

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
        ) : null}

        {auth.status !== "checking" && auth.status !== "authenticated" ? (
          <div
            role="alert"
            className="space-y-3 rounded-xl border border-[#e8c5b5] bg-[#fbf7f2] p-4 text-sm"
          >
            <p className="text-[#332720]">
              Inicia sesión para continuar con el pago de esta cotización.
            </p>
            <Link
              className="inline-flex min-h-11 items-center justify-center rounded-xl bg-[#a44731] px-4 py-2 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#1d4ed8]"
              href={`/acceso?return_to=${encodeURIComponent(`/reservas/checkout/tarjeta/${quoteId}`)}`}
            >
              Iniciar sesión
            </Link>
          </div>
        ) : null}

        {enabled && !isTestMode && !sellerKeyReconnectRequired ? (
          <p
            role="alert"
            className="rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-800"
          >
            Falta una clave pública de prueba válida. No se cargaron campos de
            tarjeta.
          </p>
        ) : null}

        {enabled && sellerKeyReconnectRequired ? (
          <div
            role="alert"
            className="space-y-2 rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950"
          >
            <p className="font-semibold">Vuelve a vincular Mercado Pago</p>
            <p>
              El anfitrión debe volver a autorizar su cuenta para habilitar el
              pago con tarjeta. No se cargaron campos de pago y no se enviará
              ningún cobro hasta completar esa vinculación.
            </p>
            <Link
              href="/mi-cuenta/cobros"
              className="inline-flex min-h-10 items-center font-medium underline underline-offset-4"
            >
              Ir a la cuenta de cobro
            </Link>
          </div>
        ) : null}

        {isLoading && !checkout ? <CardFormSkeleton /> : null}

        {error ? (
          <div
            role="alert"
            className="space-y-3 rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-800"
          >
            <p className="flex items-start gap-2">
              <AlertCircle
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              {error}
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void loadCheckout()}
              disabled={isLoading}
              className="min-h-10 border-[#ded5cc] bg-white text-[#332720] hover:bg-[#fbf7f2] hover:text-[#702a1b]"
            >
              {isLoading ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <RefreshCw className="size-4" aria-hidden="true" />
              )}
              Consultar estado
            </Button>
          </div>
        ) : null}

        {canMountCardForm && checkout ? (
          <>
            <Script
              src={SDK_URL}
              strategy="afterInteractive"
              onReady={() => {
                setSdkLoadError(null);
                setSdkLoaded(true);
                logCheckoutStage("sdk_ready", {
                  quoteId: safeIdentifier(quoteId),
                });
              }}
              onError={() => {
                logCheckoutStage("sdk_load_error", {
                  quoteId: safeIdentifier(quoteId),
                });
                setSdkLoadError(
                  "No se pudo cargar MercadoPago.js. Revisa tu conexión o bloqueadores de contenido.",
                );
              }}
            />
            <form id="form-checkout-api" className="space-y-4">
              {!secureFieldsReady && !sdkLoadError ? (
                <p
                  role="status"
                  className="flex items-center gap-2 rounded-xl border border-[#e8c5b5] bg-[#fbf7f2] px-3.5 py-2.5 text-xs font-medium text-[#66574e]"
                >
                  <Loader2
                    className="size-3.5 shrink-0 animate-spin text-[#a44731]"
                    aria-hidden="true"
                  />
                  Preparando campos seguros de Mercado Pago…
                </p>
              ) : null}

              <div className="space-y-1.5">
                <label
                  htmlFor="form-checkout-api__cardholderName"
                  className="block text-sm font-medium text-[#332720]"
                >
                  Titular de la tarjeta
                </label>
                <input
                  id="form-checkout-api__cardholderName"
                  className={FIELD_INPUT_CLASS}
                  placeholder="Nombre como aparece en la tarjeta"
                  value={cardholderName}
                  onChange={(event) => {
                    setCardholderName(event.target.value);
                    setFormError(null);
                  }}
                  autoComplete="cc-name"
                  required
                />
              </div>

              <div className="space-y-1.5">
                <label
                  htmlFor="form-checkout-api__cardNumber"
                  className="block text-sm font-medium text-[#332720]"
                >
                  Número de tarjeta
                </label>
                <div
                  id="form-checkout-api__cardNumber"
                  className={SECURE_FIELD_HOST_CLASS}
                />
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label
                    htmlFor="form-checkout-api__expirationDate"
                    className="block text-sm font-medium text-[#332720]"
                  >
                    Vencimiento
                  </label>
                  <div
                    id="form-checkout-api__expirationDate"
                    className={SECURE_FIELD_HOST_CLASS}
                  />
                </div>
                <div className="space-y-1.5">
                  <label
                    htmlFor="form-checkout-api__securityCode"
                    className="block text-sm font-medium text-[#332720]"
                  >
                    Código de seguridad (CVV)
                  </label>
                  <div
                    id="form-checkout-api__securityCode"
                    className={SECURE_FIELD_HOST_CLASS}
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-[140px_1fr]">
                <div className="space-y-1.5">
                  <label
                    htmlFor="form-checkout-api__identificationType"
                    className="block text-sm font-medium text-[#332720]"
                  >
                    Tipo de documento
                  </label>
                  <select
                    id="form-checkout-api__identificationType"
                    onChange={() => setFormError(null)}
                    className={FIELD_INPUT_CLASS}
                  />
                </div>
                <div className="space-y-1.5">
                  <label
                    htmlFor="form-checkout-api__identificationNumber"
                    className="block text-sm font-medium text-[#332720]"
                  >
                    Número de documento
                  </label>
                  <input
                    id="form-checkout-api__identificationNumber"
                    className={FIELD_INPUT_CLASS}
                    placeholder="12.345.678-9"
                    inputMode="text"
                    autoComplete="off"
                    value={identificationNumber}
                    onChange={(event) => {
                      setIdentificationNumber(event.target.value);
                      setFormError(null);
                    }}
                    required
                  />
                </div>
              </div>

              <div className="space-y-1.5">
                <label
                  htmlFor="form-checkout-api__cardholderEmail"
                  className="block text-sm font-medium text-[#332720]"
                >
                  Correo del pagador
                </label>
                <input
                  id="form-checkout-api__cardholderEmail"
                  className={FIELD_INPUT_CLASS}
                  placeholder="tu@correo.cl"
                  type="email"
                  value={email}
                  onChange={(event) => {
                    setEmail(event.target.value);
                    setFormError(null);
                  }}
                  // Avoid silently substituting the EspaciGo login email. The
                  // payer in sandbox must use the Mercado Pago test buyer email.
                  autoComplete="off"
                  required
                />
                {isTestMode ? (
                  <p className="text-xs text-[#66574e]">
                    Para esta prueba, usa el correo del comprador de prueba de
                    Mercado Pago.
                  </p>
                ) : null}
              </div>

              <div className="sr-only" aria-hidden="true">
                <select
                  id="form-checkout-api__issuer"
                  tabIndex={-1}
                  aria-hidden="true"
                />
                <select
                  id="form-checkout-api__installments"
                  tabIndex={-1}
                  aria-hidden="true"
                />
              </div>

              {sdkLoadError ? (
                <p
                  role="alert"
                  className="flex items-start gap-2 rounded-xl border border-red-300 bg-red-50 p-3.5 text-sm text-red-800"
                >
                  <AlertCircle
                    className="mt-0.5 size-4 shrink-0"
                    aria-hidden="true"
                  />
                  {sdkLoadError}
                </p>
              ) : null}

              {sdkLoadError ? (
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 w-full"
                  onClick={() => window.location.reload()}
                >
                  Volver a abrir el checkout
                </Button>
              ) : null}

              {formError ? (
                <p
                  role="alert"
                  className="flex items-start gap-2 rounded-xl border border-red-300 bg-red-50 p-3.5 text-sm text-red-800"
                >
                  <AlertCircle
                    className="mt-0.5 size-4 shrink-0"
                    aria-hidden="true"
                  />
                  {formError}
                </p>
              ) : null}

              <Button
                id="form-checkout-api__submit"
                type="submit"
                className={cn(
                  "min-h-12 w-full rounded-xl bg-[#a44731] px-5 py-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-[#843522] active:bg-[#702a1b] disabled:bg-[#a44731]/75 disabled:text-white disabled:opacity-100",
                )}
                disabled={
                  !secureFieldsReady ||
                  submitting ||
                  paymentAttempted ||
                  Boolean(sdkLoadError)
                }
              >
                {submitting ? (
                  <>
                    <Loader2
                      className="size-4 animate-spin"
                      aria-hidden="true"
                    />
                    Enviando pago…
                  </>
                ) : (
                  "Pagar con tarjeta"
                )}
              </Button>

              <p className="flex items-center justify-center gap-1.5 text-center text-xs text-[#66574e]">
                <Lock className="size-3.5 shrink-0" aria-hidden="true" />
                EspaciGo no agrega comisión en esta prueba. El anfitrión recibe
                el importe del arriendo, sujeto a cargos que aplique Mercado
                Pago.
              </p>
            </form>
          </>
        ) : null}

        {showResultCard && effectiveMessage ? (
          <CheckoutResultCard
            quoteId={quoteId}
            status={effectiveStatus}
            message={effectiveMessage}
            paymentId={effectivePaymentId}
            providerHttpStatus={result?.providerHttpStatus ?? null}
            providerCode={result?.providerCode ?? null}
            providerRequestId={result?.providerRequestId ?? null}
            isRefreshing={isLoading}
            onRefresh={() => void loadCheckout()}
          />
        ) : (
          <div className="border-t border-[#ded5cc] pt-3">
            <Link
              href={`/reservas/checkout/${quoteId}`}
              className="inline-flex min-h-10 items-center text-sm font-medium text-[#66574e] underline underline-offset-4 hover:text-[#332720]"
            >
              Volver al estado de la cotización
            </Link>
          </div>
        )}
      </section>

      {isLoading && !checkout ? (
        <SummaryCardSkeleton />
      ) : (
        <CheckoutSummaryCard
          spaceTitle={checkout?.spaceTitle}
          startAt={checkout?.startAt}
          endAt={checkout?.endAt}
          timeZone={checkout?.timeZone}
          duration={checkout?.units}
          rateUnit={checkout?.rateUnit}
          quoteId={quoteId}
          statusLabel={summaryStatus}
          totalClp={checkout?.amountClp ?? null}
          expiresAt={checkout?.expiresAt}
          contextNote="EspaciGo no agrega comisión en esta prueba. El anfitrión recibe el importe del arriendo, sujeto a cargos que aplique Mercado Pago."
          className="lg:sticky lg:top-6"
        />
      )}
    </div>
  );
}
