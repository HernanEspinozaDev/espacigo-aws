"use client";

import type { BookingReservation } from "../domain/types";
import { getReservationFinancialBreakdown } from "../domain/financial-breakdown";
import { groupReservationIntervalsByDay } from "../domain/reservation-intervals";
import { ReservationScheduleBreakdown } from "./reservation-schedule-breakdown";
import { formatBookingClp } from "./format";
import { Button } from "@/components/workspace/ui/button";
import {
  CalendarClock,
  CheckCircle2,
  Download,
  MapPin,
  Printer,
  Receipt,
  ShieldCheck,
  X,
} from "lucide-react";

interface ReservationInvoiceModalProps {
  reservation: BookingReservation | null;
  onClose: () => void;
}

export function buildInvoiceNumber(reservationId: string): string {
  return `ESP-INV-${reservationId.replace(/-/g, "").slice(0, 8).toUpperCase()}`;
}

const ESPACIGO_MARK_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="154 130 512 512" width="44" height="44" aria-hidden="true">
  <defs>
    <linearGradient id="esp-terracotta" x1="295" y1="175" x2="535" y2="553" gradientUnits="userSpaceOnUse">
      <stop stop-color="#EE8953"/>
      <stop offset="0.48" stop-color="#D46236"/>
      <stop offset="1" stop-color="#B94628"/>
    </linearGradient>
    <linearGradient id="esp-fold" x1="328" y1="516" x2="516" y2="433" gradientUnits="userSpaceOnUse">
      <stop stop-color="#C04C2B"/>
      <stop offset="1" stop-color="#E67B46"/>
    </linearGradient>
  </defs>
  <path fill="url(#esp-terracotta)" d="M592 204C548 166 489 150 433 156C315 164 218 255 217 363C214 418 233 460 272 491L402 612Q409 622 416 612C456 566 519 535 561 488C595 450 605 404 603 356Q603 335 582 335H511Q500 335 491 328L439 286Q426 275 414 284L329 352Q317 362 317 377V410Q325 431 340 444C310 420 298 392 298 369C298 293 358 231 433 231C466 230 496 243 532 265Q540 270 548 262L592 218Q599 211 592 204Z"/>
  <path fill="url(#esp-fold)" d="M272 491C340 537 425 532 489 475C524 444 550 442 578 451C556 501 499 538 455 572L416 612Q409 622 402 612Z"/>
  <path fill="#FFF" d="M340 444V401Q340 389 350 381L407 340Q417 332 427 340L491 389Q501 398 514 400V428C514 441 508 450 498 458C456 492 393 499 350 474Q344 470 340 466Z"/>
  <g fill="#CA5732">
    <rect x="394" y="387" width="21" height="20" rx="4"/>
    <rect x="421" y="387" width="21" height="20" rx="4"/>
    <rect x="394" y="414" width="21" height="21" rx="4"/>
    <rect x="421" y="414" width="21" height="21" rx="4"/>
  </g>
</svg>`;

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function ReservationInvoiceModal({
  reservation,
  onClose,
}: ReservationInvoiceModalProps) {
  if (!reservation) return null;

  const isHost = reservation.participant_role === "anfitrion";
  const invoiceNumber = buildInvoiceNumber(reservation.id);
  const {
    baseClp,
    hostCommissionClp,
    hostVatClp,
    hostFeeClp,
    netHostClp,
    confirmedNetHostClp,
    processingFeeClp,
    buyerFeeClp,
    marketplaceFeeClp,
    buyerCommissionClp,
    buyerVatClp,
    totalChargedClp,
  } = getReservationFinancialBreakdown(reservation);

  const paymentMethodLabel =
    reservation.checkout_method === "payments_api"
      ? "Mercado Pago Checkout API (Tarjeta directa)"
      : "Mercado Pago Checkout Pro";

  const issuedAtFormatted = new Date(reservation.updated_at).toLocaleString(
    "es-CL",
  );
  const startAtFormatted = new Date(reservation.start_at).toLocaleString(
    "es-CL",
  );
  const endAtFormatted = new Date(reservation.end_at).toLocaleString("es-CL");
  const unitsLabel = `${reservation.units} ${reservation.rate_unit}${
    reservation.units > 1 ? "s" : ""
  } × ${formatBookingClp(reservation.unit_price_clp)} CLP`;
  const mpOperationLabel = reservation.provider_payment_id
    ? `#${reservation.provider_payment_id}`
    : "Confirmada";

  const settlement = reservation.provider_settlement;
  const paymentLabel =
    settlement?.payment_status === "charged_back"
      ? "Contracargo informado por Mercado Pago"
      : settlement?.payment_status === "refunded" ||
          (reservation.commercial_refund_status === "aprobado" &&
            reservation.state === "reembolsada")
        ? "Pago reembolsado"
        : "Pago aprobado";
  const sourceDate = settlement?.synced_at
    ? new Date(settlement.synced_at).toLocaleString("es-CL", {
        timeZone: "America/Santiago",
      })
    : undefined;
  const money = (amount: number) => `${formatBookingClp(amount)} CLP`;
  const netLabel =
    confirmedNetHostClp === undefined
      ? "Importe antes de tarifa Mercado Pago"
      : "Neto informado por Mercado Pago";
  const releaseLabel =
    settlement?.release_status === "released"
      ? "Fondos liberados en Mercado Pago"
      : settlement?.release_status === "pending"
        ? "Fondos pendientes de liberación"
        : "Disponibilidad de fondos sin confirmar";
  const releaseDate = settlement?.release_at
    ? new Date(settlement.release_at).toLocaleString("es-CL", {
        timeZone: "America/Santiago",
      })
    : undefined;
  const releaseDescription =
    releaseLabel + (releaseDate ? ` · ${releaseDate}` : "");
  const reconciliationNote =
    settlement?.reconciliation === "discrepancy"
      ? "El desglose de Mercado Pago presenta una diferencia pendiente de conciliar."
      : settlement?.reconciliation === "reconciled"
        ? "Importes conciliados con el pago de Mercado Pago."
        : "Conciliación de cargos de Mercado Pago pendiente.";
  const settlementNote = sourceDate
    ? `${reconciliationNote} Consulta de Mercado Pago: ${sourceDate}.`
    : reconciliationNote;
  const hostRows = [
    ["Valor del arriendo", money(baseClp)],
    [
      "Cargo EspaciGo al arrendatario (3% + IVA)",
      `+${money(buyerFeeClp)} (${money(buyerCommissionClp)} + ${money(buyerVatClp)} de IVA)`,
    ],
    ["Total pagado por el arrendatario", money(totalChargedClp)],
    [
      "Cargo EspaciGo al anfitrión (3% + IVA)",
      `−${money(hostFeeClp)} (${money(hostCommissionClp)} + ${money(hostVatClp)} de IVA)`,
    ],
    [
      "Comisión EspaciGo en el split",
      `${money(marketplaceFeeClp)} en total · ${money(buyerFeeClp)} arrendatario + ${money(hostFeeClp)} anfitrión`,
    ],
    ...(settlement?.marketplace_fee_clp !== undefined &&
    settlement.marketplace_fee_clp !== marketplaceFeeClp
      ? [
          [
            "Comisión del split informada por Mercado Pago",
            money(settlement.marketplace_fee_clp),
          ],
        ]
      : []),
    [
      "Comisión de procesamiento de Mercado Pago",
      processingFeeClp === undefined
        ? "Pendiente de confirmar"
        : `−${money(processingFeeClp)}`,
    ],
    ...(settlement?.refunded_clp
      ? [["Reembolso informado", money(settlement.refunded_clp)]]
      : []),
  ];

  const handleDownloadTxt = () => {
    const economicLines = isHost
      ? [
          ...hostRows.map(([label, value]) => `${label}: ${value}`),
          `${netLabel}: ${money(netHostClp)}`,
        ]
      : [
          `Precio Original del Espacio: ${formatBookingClp(baseClp)} CLP`,
          `Comisión de Servicio (3%):  +${formatBookingClp(buyerCommissionClp)} CLP`,
          `IVA sobre Comisión (19%):   +${formatBookingClp(buyerVatClp)} CLP`,
          `Total Pagado:                ${formatBookingClp(totalChargedClp)} CLP`,
        ];

    const footerLines = isHost
      ? [settlementNote, releaseDescription]
      : [
          "Nota: Este comprobante refleja el cobro total confirmado",
          "en EspaciGo y procesado a través de Mercado Pago Chile.",
        ];

    const lines = [
      "============================================================",
      isHost
        ? "           ESPACIGO — LIQUIDACIÓN / INVOICE ANFITRIÓN       "
        : "           ESPACIGO — COMPROBANTE DE PAGO CLIENTE           ",
      "============================================================",
      `N° Comprobante:      ${invoiceNumber}`,
      `Fecha de Emisión:    ${issuedAtFormatted}`,
      `Estado:              ${paymentLabel}`,
      "------------------------------------------------------------",
      "DATOS DEL ESPACIO Y RESERVA",
      `Espacio:             ${reservation.space_title || reservation.space_id}`,
      `ID Espacio:          ${reservation.space_id}`,
      `ID Reserva:          ${reservation.id}`,
      `ID Cotización:       ${reservation.quote_id}`,
      `Inicio Acordado:     ${startAtFormatted}`,
      `Término Acordado:    ${endAtFormatted}`,
      `Cantidad:            ${unitsLabel}`,
      ...(reservation.intervals && reservation.intervals.length > 1
        ? [
            "------------------------------------------------------------",
            "DESGLOSE DE DÍAS Y HORARIOS RESERVADOS",
            ...groupReservationIntervalsByDay(reservation).map(
              (day) =>
                `  ${day.dateLabel}: ${day.slots.map((s) => `${s.timeLabel} (${s.units}h)`).join(", ")} -> ${formatBookingClp(day.subtotalClp)} CLP`,
            ),
          ]
        : []),
      "------------------------------------------------------------",
      "DATOS DE TRANSACCIÓN MERCADO PAGO",
      `Método:              ${paymentMethodLabel}`,
      `N° Operación MP:     ${mpOperationLabel}`,
      `Estado Checkout:     ${reservation.checkout_status || "approved"}`,
      "------------------------------------------------------------",
      "DESGLOSE ECONÓMICO (CLP)",
      ...economicLines,
      "============================================================",
      ...footerLines,
      "============================================================",
    ].join("\n");

    const blob = new Blob([lines], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${invoiceNumber}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const handlePrintPdf = () => {
    const printWindow = window.open("", "_blank", "width=920,height=1050");
    if (!printWindow) {
      window.print();
      return;
    }

    const docTitle = isHost
      ? `Liquidación Anfitrión ${invoiceNumber} — EspaciGo`
      : `Comprobante de Pago ${invoiceNumber} — EspaciGo`;

    const badgeText = isHost
      ? "LIQUIDACIÓN / INVOICE ANFITRIÓN"
      : "COMPROBANTE DE PAGO CLIENTE";

    const rowsHtml = isHost
      ? hostRows
          .map(
            ([label, value]) =>
              `<tr><td><div class="row-title">${escapeHtml(label)}</div></td><td class="amount">${escapeHtml(value)}</td></tr>`,
          )
          .join("")
      : `
        <tr>
          <td>
            <div class="row-title">Precio original del espacio</div>
            <div class="row-sub">${escapeHtml(unitsLabel)}</div>
          </td>
          <td class="amount">${escapeHtml(formatBookingClp(baseClp))} CLP</td>
        </tr>
        <tr>
          <td>
            <div class="row-title">Comisión de servicio EspaciGo (3%)</div>
            <div class="row-sub">Uso y respaldo de plataforma EspaciGo</div>
          </td>
          <td class="amount">+${escapeHtml(formatBookingClp(buyerCommissionClp))} CLP</td>
        </tr>
        <tr>
          <td>
            <div class="row-title">IVA sobre comisión (19%)</div>
            <div class="row-sub">Impuesto legal sobre servicio</div>
          </td>
          <td class="amount">+${escapeHtml(formatBookingClp(buyerVatClp))} CLP</td>
        </tr>
      `;

    const summaryLabel = isHost ? netLabel : "Total Pagado";
    const summarySub = isHost
      ? releaseDescription
      : "Cobro confirmado vía Mercado Pago Chile";
    const summaryAmount = isHost
      ? `${formatBookingClp(netHostClp)} CLP`
      : `${formatBookingClp(totalChargedClp)} CLP`;

    const legalNote = isHost
      ? settlementNote
      : "Este comprobante refleja los importes confirmados de tu pago en EspaciGo tras la aprobación en Mercado Pago Chile.";

    const groupedDays = groupReservationIntervalsByDay(reservation);
    const schedulePrintHtml =
      reservation.intervals && reservation.intervals.length > 1
        ? `
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Días y Horarios Reservados</th>
                <th>Subtotal</th>
              </tr>
            </thead>
            <tbody>
              ${groupedDays
                .map(
                  (day) => `
                <tr>
                  <td>
                    <div class="row-title" style="text-transform: capitalize;">${escapeHtml(day.dateLabel)}</div>
                    <div class="row-sub mono">${escapeHtml(
                      day.slots
                        .map((s) => `${s.timeLabel} (${s.units}h)`)
                        .join(" · "),
                    )}</div>
                  </td>
                  <td class="amount">${escapeHtml(formatBookingClp(day.subtotalClp))} CLP</td>
                </tr>`,
                )
                .join("")}
            </tbody>
          </table>
        </div>`
        : "";

    const html = `<!doctype html>
<html lang="es">
  <head>
    <meta charset="utf-8" />
    <title>${escapeHtml(docTitle)}</title>
    <style>
      @page {
        size: A4;
        margin: 14mm;
      }
      * {
        box-sizing: border-box;
      }
      body {
        margin: 0;
        padding: 0;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, Helvetica, Arial, sans-serif;
        color: #18181b;
        background: #ffffff;
        -webkit-print-color-adjust: exact;
        print-color-adjust: exact;
      }
      .sheet {
        max-width: 760px;
        margin: 0 auto;
        border: 1px solid #e4e4e7;
        border-radius: 14px;
        overflow: hidden;
        background: #ffffff;
      }
      .accent-bar {
        height: 6px;
        background: linear-gradient(90deg, #ee8953 0%, #d46236 52%, #b94628 100%);
      }
      .content {
        padding: 28px 32px;
      }
      .header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        gap: 20px;
        padding-bottom: 20px;
        border-bottom: 1px solid #e4e4e7;
      }
      .brand {
        display: flex;
        align-items: center;
        gap: 12px;
      }
      .brand-mark {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 48px;
        height: 48px;
        border-radius: 12px;
        background: #fff7f3;
        border: 1px solid #fed7c3;
      }
      .brand-name {
        font-size: 20px;
        font-weight: 800;
        letter-spacing: -0.02em;
        color: #18181b;
        line-height: 1.1;
      }
      .brand-sub {
        margin-top: 3px;
        font-size: 11px;
        font-weight: 600;
        color: #71717a;
        text-transform: uppercase;
        letter-spacing: 0.06em;
      }
      .doc-meta {
        text-align: right;
      }
      .doc-type {
        font-size: 10px;
        font-weight: 700;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: #c2410c;
      }
      .doc-number {
        margin-top: 4px;
        font-size: 18px;
        font-weight: 800;
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        color: #18181b;
      }
      .status-pill {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        margin-top: 8px;
        padding: 4px 10px;
        border-radius: 999px;
        background: #ecfdf5;
        border: 1px solid #a7f3d0;
        color: #065f46;
        font-size: 11px;
        font-weight: 700;
      }
      .status-dot {
        width: 7px;
        height: 7px;
        border-radius: 999px;
        background: #10b981;
      }
      .issued {
        margin-top: 6px;
        font-size: 11px;
        color: #71717a;
      }
      .grid {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 14px;
        margin-top: 20px;
      }
      .card {
        border: 1px solid #e4e4e7;
        border-radius: 10px;
        padding: 14px 16px;
        background: #fafafa;
      }
      .card-title {
        font-size: 10px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: #71717a;
        margin-bottom: 8px;
      }
      .card-main {
        font-size: 14px;
        font-weight: 700;
        color: #18181b;
        margin-bottom: 6px;
      }
      .meta-line {
        font-size: 11px;
        color: #52525b;
        line-height: 1.55;
        word-break: break-word;
      }
      .meta-line strong {
        color: #18181b;
      }
      .mono {
        font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      }
      .table-wrap {
        margin-top: 20px;
        border: 1px solid #e4e4e7;
        border-radius: 10px;
        overflow: hidden;
      }
      table {
        width: 100%;
        border-collapse: collapse;
      }
      thead th {
        background: #f4f4f5;
        padding: 10px 16px;
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.05em;
        color: #52525b;
        text-align: left;
        border-bottom: 1px solid #e4e4e7;
      }
      thead th:last-child {
        text-align: right;
      }
      tbody td {
        padding: 12px 16px;
        border-bottom: 1px solid #f4f4f5;
        vertical-align: top;
      }
      tbody tr:last-child td {
        border-bottom: none;
      }
      .row-title {
        font-size: 13px;
        font-weight: 600;
        color: #18181b;
      }
      .row-sub {
        margin-top: 2px;
        font-size: 11px;
        color: #71717a;
      }
      .amount {
        text-align: right;
        font-size: 13px;
        font-weight: 700;
        font-variant-numeric: tabular-nums;
        white-space: nowrap;
        color: #18181b;
      }
      .deduction {
        color: #b45309;
      }
      .summary-box {
        margin-top: 16px;
        padding: 16px 18px;
        border-radius: 10px;
        background: #ecfdf5;
        border: 1px solid #a7f3d0;
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 16px;
      }
      .summary-label {
        font-size: 13px;
        font-weight: 700;
        color: #064e3b;
      }
      .summary-sub {
        margin-top: 2px;
        font-size: 11px;
        color: #047857;
      }
      .summary-amount {
        font-size: 20px;
        font-weight: 800;
        font-variant-numeric: tabular-nums;
        color: #065f46;
        white-space: nowrap;
      }
      .note {
        margin-top: 18px;
        padding: 12px 14px;
        border-radius: 8px;
        background: #fafafa;
        border: 1px solid #e4e4e7;
        font-size: 11px;
        line-height: 1.55;
        color: #52525b;
      }
      .doc-footer {
        margin-top: 18px;
        padding-top: 12px;
        border-top: 1px dashed #e4e4e7;
        display: flex;
        justify-content: space-between;
        font-size: 10px;
        color: #71717a;
      }
    </style>
  </head>
  <body>
    <div class="sheet">
      <div class="accent-bar"></div>
      <div class="content">
        <div class="header">
          <div class="brand">
            <div class="brand-mark">${ESPACIGO_MARK_SVG}</div>
            <div>
              <div class="brand-name">EspaciGo</div>
              <div class="brand-sub">Marketplace de Espacios · Chile</div>
            </div>
          </div>
          <div class="doc-meta">
            <div class="doc-type">${escapeHtml(badgeText)}</div>
            <div class="doc-number">${escapeHtml(invoiceNumber)}</div>
            <div class="status-pill">
              <span class="status-dot"></span>
              ${escapeHtml(paymentLabel)}
            </div>
            <div class="issued">Emitido: ${escapeHtml(issuedAtFormatted)}</div>
          </div>
        </div>

        <div class="grid">
          <div class="card">
            <div class="card-title">Espacio y Horario</div>
            <div class="card-main">${escapeHtml(reservation.space_title || "Espacio EspaciGo")}</div>
            <div class="meta-line mono">ID Espacio: ${escapeHtml(reservation.space_id)}</div>
            <div class="meta-line" style="margin-top: 6px;">
              <strong>Inicio:</strong> ${escapeHtml(startAtFormatted)}
            </div>
            <div class="meta-line">
              <strong>Término:</strong> ${escapeHtml(endAtFormatted)}
            </div>
          </div>
          <div class="card">
            <div class="card-title">Referencia de Transacción</div>
            <div class="meta-line mono"><strong>Reserva:</strong> ${escapeHtml(reservation.id)}</div>
            <div class="meta-line mono"><strong>Cotización:</strong> ${escapeHtml(reservation.quote_id)}</div>
            <div class="meta-line" style="margin-top: 6px;">
              <strong>Pasarela:</strong> ${escapeHtml(paymentMethodLabel)}
            </div>
            <div class="meta-line mono">
              <strong>Operación MP:</strong> ${escapeHtml(mpOperationLabel)}
            </div>
          </div>
        </div>

        ${schedulePrintHtml}

        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Concepto y Detalle</th>
                <th>Monto (CLP)</th>
              </tr>
            </thead>
            <tbody>
              ${rowsHtml}
            </tbody>
          </table>
        </div>

        <div class="summary-box">
          <div>
            <div class="summary-label">${escapeHtml(summaryLabel)}</div>
            <div class="summary-sub">${escapeHtml(summarySub)}</div>
          </div>
          <div class="summary-amount">${escapeHtml(summaryAmount)}</div>
        </div>

        <div class="note">${escapeHtml(legalNote)}</div>

        <div class="doc-footer">
          <span>Documento electrónico generado por EspaciGo</span>
          <span>Ref: ${escapeHtml(invoiceNumber)}</span>
        </div>
      </div>
    </div>
    <script>
      window.addEventListener("load", function () {
        setTimeout(function () {
          window.focus();
          window.print();
        }, 120);
      });
    </script>
  </body>
</html>`;

    printWindow.document.open();
    printWindow.document.write(html);
    printWindow.document.close();
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="espacigo-invoice-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-3 backdrop-blur-xs sm:p-4"
    >
      <div className="relative max-h-[92vh] w-full max-w-2xl overflow-y-auto rounded-2xl border border-border bg-card text-foreground shadow-2xl">
        {/* Top brand accent bar */}
        <div className="h-1.5 w-full bg-gradient-to-r from-[#EE8953] via-[#D46236] to-[#B94628]" />

        <div className="p-5 sm:p-6">
          {/* Brand & Invoice Header */}
          <div className="flex flex-col justify-between gap-4 border-b border-border pb-5 sm:flex-row sm:items-start">
            <div className="flex items-center gap-3.5">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-[#D46236]/25 bg-[#D46236]/10 p-1.5">
                <img
                  src="/espacigo-mark.svg"
                  alt="EspaciGo"
                  width={36}
                  height={36}
                  className="h-9 w-9 object-contain"
                />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-lg font-extrabold tracking-tight text-foreground">
                    EspaciGo
                  </span>
                  <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    Chile
                  </span>
                </div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[#D46236] dark:text-[#EE8953]">
                  {isHost
                    ? "Liquidación / Invoice Anfitrión"
                    : "Comprobante de Pago Cliente"}
                </p>
                <h2
                  id="espacigo-invoice-title"
                  className="mt-0.5 font-mono text-sm font-bold text-foreground"
                >
                  {invoiceNumber}
                </h2>
              </div>
            </div>

            <div className="flex items-start justify-between gap-3 sm:flex-col sm:items-end">
              <button
                type="button"
                onClick={onClose}
                aria-label="Cerrar comprobante"
                className="order-2 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:order-1 print:hidden"
              >
                <X className="h-5 w-5" />
              </button>
              <div className="order-1 text-left sm:order-2 sm:text-right">
                <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-1 text-xs font-semibold text-emerald-700 dark:text-emerald-300">
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  {paymentLabel}
                </span>
                <p className="mt-1.5 font-mono text-[11px] text-muted-foreground">
                  Emitido: {issuedAtFormatted}
                </p>
              </div>
            </div>
          </div>

          {/* Body */}
          <div className="mt-5 space-y-4 text-sm">
            {/* Espacio & Transacción */}
            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <div className="rounded-xl border border-border bg-muted/25 p-4 text-xs">
                <p className="flex items-center gap-1.5 font-semibold uppercase tracking-wider text-muted-foreground">
                  <MapPin className="h-3.5 w-3.5 text-[#D46236]" />
                  Espacio y Horario
                </p>
                <p className="mt-2 text-sm font-bold text-foreground">
                  {reservation.space_title || "Espacio EspaciGo"}
                </p>
                <p className="mt-0.5 break-all font-mono text-[11px] text-muted-foreground">
                  ID: {reservation.space_id}
                </p>
                <div className="mt-2.5 space-y-1 border-t border-border/70 pt-2.5 text-muted-foreground">
                  <p className="flex items-center gap-1.5">
                    <CalendarClock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span>
                      Inicio:{" "}
                      <strong className="text-foreground">
                        {startAtFormatted}
                      </strong>
                    </span>
                  </p>
                  <p className="pl-5">
                    Término:{" "}
                    <strong className="text-foreground">
                      {endAtFormatted}
                    </strong>
                  </p>
                </div>
              </div>

              <div className="rounded-xl border border-border bg-muted/25 p-4 text-xs">
                <p className="flex items-center gap-1.5 font-semibold uppercase tracking-wider text-muted-foreground">
                  <Receipt className="h-3.5 w-3.5 text-[#D46236]" />
                  Referencia de Transacción
                </p>
                <div className="mt-2 space-y-1.5">
                  <p className="break-all font-mono text-[11px] text-foreground">
                    <span className="text-muted-foreground">Reserva: </span>
                    {reservation.id}
                  </p>
                  <p className="break-all font-mono text-[11px] text-muted-foreground">
                    <span>Cotización: </span>
                    {reservation.quote_id}
                  </p>
                  <p className="pt-1 text-muted-foreground">
                    Pasarela:{" "}
                    <strong className="text-foreground">
                      {paymentMethodLabel}
                    </strong>
                  </p>
                  <p className="font-mono font-semibold text-emerald-700 dark:text-emerald-400">
                    Operación MP {mpOperationLabel}
                  </p>
                </div>
              </div>
            </div>

            {reservation.intervals && reservation.intervals.length > 1 && (
              <ReservationScheduleBreakdown reservation={reservation} />
            )}

            {/* Desglose financiero por rol */}
            <div className="overflow-hidden rounded-xl border border-border">
              <table className="w-full text-left text-xs">
                <thead className="bg-muted/60 text-muted-foreground">
                  <tr>
                    <th className="px-4 py-2.5 font-semibold uppercase tracking-wider">
                      Concepto
                    </th>
                    <th className="hidden px-4 py-2.5 text-right font-semibold uppercase tracking-wider sm:table-cell">
                      Detalle
                    </th>
                    <th className="px-4 py-2.5 text-right font-semibold uppercase tracking-wider">
                      Monto (CLP)
                    </th>
                  </tr>
                </thead>
                {isHost ? (
                  <tbody className="divide-y divide-border bg-background">
                    {hostRows.map(([label, value]) => (
                      <tr key={label}>
                        <td
                          className="px-4 py-3 text-foreground sm:table-cell"
                          colSpan={2}
                        >
                          {label}
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-foreground">
                          {value}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                ) : (
                  <tbody className="divide-y divide-border bg-background">
                    <tr>
                      <td className="px-4 py-3 font-medium text-foreground">
                        Precio original del espacio
                        <span className="mt-0.5 block text-[11px] font-normal text-muted-foreground sm:hidden">
                          {unitsLabel}
                        </span>
                      </td>
                      <td className="hidden px-4 py-3 text-right text-muted-foreground sm:table-cell">
                        {unitsLabel}
                      </td>
                      <td className="px-4 py-3 text-right font-semibold tabular-nums text-foreground">
                        {formatBookingClp(baseClp)} CLP
                      </td>
                    </tr>
                    <tr>
                      <td className="px-4 py-3 text-muted-foreground">
                        Comisión de servicio EspaciGo (3%)
                      </td>
                      <td className="hidden px-4 py-3 text-right text-muted-foreground sm:table-cell">
                        Servicio plataforma EspaciGo
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-foreground">
                        +{formatBookingClp(buyerCommissionClp)} CLP
                      </td>
                    </tr>
                    <tr>
                      <td className="px-4 py-3 text-muted-foreground">
                        IVA sobre comisión (19%)
                      </td>
                      <td className="hidden px-4 py-3 text-right text-muted-foreground sm:table-cell">
                        Impuesto legal sobre servicio
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-foreground">
                        +{formatBookingClp(buyerVatClp)} CLP
                      </td>
                    </tr>
                  </tbody>
                )}
              </table>
            </div>

            {/* Highlighted Total / Net Box */}
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3.5">
              <div>
                <p className="text-xs font-bold uppercase tracking-wider text-emerald-800 dark:text-emerald-300">
                  {isHost ? netLabel : "Total Pagado"}
                </p>
                <p className="mt-0.5 text-[11px] text-emerald-700/90 dark:text-emerald-300/80">
                  {isHost
                    ? releaseDescription
                    : "Cobro confirmado vía Mercado Pago Chile"}
                </p>
              </div>
              <p className="text-xl font-extrabold tabular-nums text-emerald-700 dark:text-emerald-300">
                {isHost
                  ? `${formatBookingClp(netHostClp)} CLP`
                  : `${formatBookingClp(totalChargedClp)} CLP`}
              </p>
            </div>

            {/* Legal / Note */}
            <div className="flex items-start gap-2.5 rounded-xl border border-border/80 bg-muted/30 p-3 text-[11px] leading-relaxed text-muted-foreground">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-[#D46236]" />
              <p>
                {isHost
                  ? settlementNote
                  : "Este comprobante refleja los importes confirmados de tu pago en EspaciGo tras la aprobación en Mercado Pago."}
              </p>
            </div>
          </div>

          {/* Footer actions */}
          <div className="mt-6 flex flex-wrap items-center justify-end gap-2.5 border-t border-border pt-4 print:hidden">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleDownloadTxt}
            >
              <Download className="mr-1.5 h-4 w-4" />
              Descargar (.txt)
            </Button>
            <Button
              type="button"
              variant="default"
              size="sm"
              onClick={handlePrintPdf}
              className="bg-[#D46236] text-white hover:bg-[#B94628]"
            >
              <Printer className="mr-1.5 h-4 w-4" />
              Imprimir / Guardar PDF
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              Cerrar
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
