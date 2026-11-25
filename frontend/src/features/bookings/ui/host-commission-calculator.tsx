"use client";

import { useId, useState } from "react";
import { calculateBuyerFeeBreakdown } from "@/features/rental-checkout/domain/fee-breakdown";
import type { BookingReservation } from "../domain/types";
import { formatBookingClp } from "./format";

function parseWholeCLP(value: string): number | undefined {
  if (!/^\d+$/.test(value)) return undefined;
  const amount = Number(value);
  return Number.isSafeInteger(amount) && amount <= Number.MAX_SAFE_INTEGER / 100
    ? amount
    : undefined;
}

function parseRateBps(value: string): bigint | undefined {
  const rate = value.trim().replace(",", ".");
  if (!/^\d{1,3}(\.\d{1,2})?$/.test(rate)) return undefined;
  const [whole, decimal = ""] = rate.split(".");
  const bps = BigInt(whole) * BigInt(100) + BigInt(decimal.padEnd(2, "0"));
  return bps <= BigInt(10000) ? bps : undefined;
}

const inputStyle = "w-full rounded-md border border-input bg-background p-2";

const paymentMethodLabels: Record<string, string> = {
  debit_card: "Débito",
  credit_card: "Crédito",
  prepaid_card: "Prepago",
  account_money: "Saldo de Mercado Pago",
};

export function HostCommissionCalculator({
  reservations,
}: {
  reservations: readonly BookingReservation[];
}) {
  const reference = reservations
    .flatMap(({ participant_role, provider_settlement, subtotal_clp }) => {
      if (
        participant_role !== "anfitrion" ||
        provider_settlement?.reconciliation !== "reconciled" ||
        provider_settlement.processing_fee_clp === undefined
      )
        return [];
      return [
        {
          amount: subtotal_clp,
          fee: provider_settlement.processing_fee_clp,
          method: provider_settlement.payment_type,
          date:
            provider_settlement.approved_at ??
            provider_settlement.provider_updated_at,
        },
      ];
    })
    .sort((a, b) => Date.parse(b.date) - Date.parse(a.date))[0];
  const id = useId();
  const [rent, setRent] = useState("");
  const [value, setValue] = useState("");
  const [other, setOther] = useState("");
  const [kind, setKind] = useState("amount");
  const [basis, setBasis] = useState("charged");
  const [includesVAT, setIncludesVAT] = useState(true);

  const base = parseWholeCLP(rent);
  const amounts =
    base === undefined ? undefined : calculateBuyerFeeBreakdown(base);
  let providerFee = kind === "amount" ? parseWholeCLP(value) : undefined;
  if (amounts && kind === "rate") {
    const rate = parseRateBps(value);
    if (rate !== undefined) {
      const feeBase = BigInt(
        basis === "charged" ? amounts.totalClp : amounts.baseClp,
      );
      const factor = includesVAT ? BigInt(100) : BigInt(119);
      providerFee = Number(
        (feeBase * rate * factor + BigInt(500000)) / BigInt(1000000),
      );
    }
  }
  const otherCharges = other === "" ? 0 : parseWholeCLP(other);
  const money = (amount: number) => `${formatBookingClp(amount)} CLP`;

  return (
    <details className="space-y-4 rounded-2xl border border-border bg-card p-5">
      <summary className="cursor-pointer font-semibold">
        Calculadora de comisiones
      </summary>
      {reference && (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-sm">
          <p>
            Cargo confirmado {money(reference.fee)} ·{" "}
            {paymentMethodLabels[reference.method ?? ""] ?? "Mercado Pago"} ·{" "}
            {new Date(reference.date).toLocaleDateString("es-CL", {
              timeZone: "America/Santiago",
            })}
          </p>
          <button
            type="button"
            className="rounded-md border border-input px-3 py-2"
            onClick={() => {
              setRent(String(reference.amount));
              setKind("amount");
              setValue(String(reference.fee));
            }}
          >
            Usar como referencia
          </button>
        </div>
      )}
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <label htmlFor={`${id}-rent`} className="grid gap-1 text-sm">
          Valor del arriendo en CLP
          <input
            id={`${id}-rent`}
            className={inputStyle}
            inputMode="numeric"
            value={rent}
            onChange={(event) => setRent(event.target.value)}
          />
        </label>
        <label htmlFor={`${id}-kind`} className="grid gap-1 text-sm">
          Forma de ingresar el costo de Mercado Pago
          <select
            id={`${id}-kind`}
            className={inputStyle}
            value={kind}
            onChange={(event) => {
              setKind(event.target.value);
              setValue("");
            }}
          >
            <option value="amount">Monto en CLP</option>
            <option value="rate">Porcentaje estimado</option>
          </select>
        </label>
        <label htmlFor={`${id}-fee`} className="grid gap-1 text-sm">
          {kind === "amount"
            ? "Costo de Mercado Pago en CLP"
            : "Tasa de Mercado Pago (%)"}
          <input
            id={`${id}-fee`}
            className={inputStyle}
            inputMode={kind === "amount" ? "numeric" : "decimal"}
            value={value}
            onChange={(event) => setValue(event.target.value)}
          />
        </label>
        <label htmlFor={`${id}-other`} className="grid gap-1 text-sm">
          Otros cargos estimados en CLP (opcional)
          <input
            id={`${id}-other`}
            className={inputStyle}
            inputMode="numeric"
            value={other}
            onChange={(event) => setOther(event.target.value)}
          />
        </label>
        {kind === "rate" && (
          <>
            <label htmlFor={`${id}-basis`} className="grid gap-1 text-sm">
              Base del porcentaje
              <select
                id={`${id}-basis`}
                className={inputStyle}
                value={basis}
                onChange={(event) => setBasis(event.target.value)}
              >
                <option value="charged">
                  Total pagado por el arrendatario
                </option>
                <option value="rent">Valor del arriendo</option>
              </select>
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={includesVAT}
                onChange={(event) => setIncludesVAT(event.target.checked)}
              />
              La tasa ingresada ya incluye IVA
            </label>
          </>
        )}
      </div>
      {amounts && providerFee !== undefined && otherCharges !== undefined ? (
        <dl className="mt-4 grid gap-2 text-sm" aria-live="polite">
          <div className="flex justify-between gap-2">
            <dt>EspaciGo del arrendatario (3 % + IVA)</dt>
            <dd>+{money(amounts.serviceFeeClp)}</dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt>Total estimado del arrendatario</dt>
            <dd>{money(amounts.totalClp)}</dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt>EspaciGo del anfitrión (3 % + IVA)</dt>
            <dd>−{money(amounts.serviceFeeClp)}</dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt>Comisión total de EspaciGo en el split</dt>
            <dd>{money(amounts.serviceFeeClp * 2)}</dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt>Costo estimado de Mercado Pago</dt>
            <dd>−{money(providerFee)}</dd>
          </div>
          <div className="flex justify-between gap-2 border-t pt-2 font-semibold">
            <dt>Neto estimado del anfitrión</dt>
            <dd>
              {money(
                amounts.baseClp -
                  amounts.serviceFeeClp -
                  providerFee -
                  otherCharges,
              )}
            </dd>
          </div>
        </dl>
      ) : (
        <p className="mt-4 text-sm text-muted-foreground">
          Ingresa el valor del arriendo y el costo de Mercado Pago para calcular
          el neto.
        </p>
      )}
    </details>
  );
}
