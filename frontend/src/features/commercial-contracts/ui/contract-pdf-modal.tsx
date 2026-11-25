"use client";

import { Download, FileText, X } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { Button } from "@/components/workspace/ui/button";
import { formatRentalCLP } from "@/features/public-catalog/domain/contract-preview-format";
import { stripPostalCodeFromAddress } from "@/features/host-listings/domain/commercial-location";
import type { ContractSection } from "@/features/host-listings/domain/contract-template";
import {
  agreementStateLabel,
  type CommercialAgreement,
  type ContractProposal,
} from "../domain/agreements";
import { contractDate } from "./contract-proposal-terms";
import {
  ContractDocumentView,
  contractOrdinalTitle,
  formatLegalDate,
  formatTimeZoneLabel,
  rateUnitWord,
  type TemplatePreviewInput,
} from "./contract-document-view";

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

const ESPACIGO_LOGO_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="154 130 512 512" width="38" height="38" aria-hidden="true"><defs><linearGradient id="terracotta" x1="295" y1="175" x2="535" y2="553" gradientUnits="userSpaceOnUse"><stop stop-color="#EE8953"/><stop offset="0.48" stop-color="#D46236"/><stop offset="1" stop-color="#B94628"/></linearGradient><linearGradient id="fold" x1="328" y1="516" x2="516" y2="433" gradientUnits="userSpaceOnUse"><stop stop-color="#C04C2B"/><stop offset="1" stop-color="#E67B46"/></linearGradient></defs><path fill="url(#terracotta)" d="M592 204C548 166 489 150 433 156C315 164 218 255 217 363C214 418 233 460 272 491L402 612Q409 622 416 612C456 566 519 535 561 488C595 450 605 404 603 356Q603 335 582 335H511Q500 335 491 328L439 286Q426 275 414 284L329 352Q317 362 317 377V410Q325 431 340 444C310 420 298 392 298 369C298 293 358 231 433 231C466 230 496 243 532 265Q540 270 548 262L592 218Q599 211 592 204Z"/><path fill="url(#fold)" d="M272 491C340 537 425 532 489 475C524 444 550 442 578 451C556 501 499 538 455 572L416 612Q409 622 402 612Z"/><path fill="#FFF" d="M340 444V401Q340 389 350 381L407 340Q417 332 427 340L491 389Q501 398 514 400V428C514 441 508 450 498 458C456 492 393 499 350 474Q344 470 340 466Z"/><g fill="#CA5732"><rect x="394" y="387" width="21" height="20" rx="4"/><rect x="421" y="387" width="21" height="20" rx="4"/><rect x="394" y="414" width="21" height="21" rx="4"/><rect x="421" y="414" width="21" height="21" rx="4"/></g></svg>`;

function partyDescription(
  party: ContractProposal["parties"][number],
  label: string,
) {
  if (party.legal_name && party.rut) {
    return `<strong>${escapeHtml(party.legal_name)}</strong>, cédula de identidad / RUT N.º <strong>${escapeHtml(party.rut)}</strong>, en adelante «${label}»`;
  }
  return `<strong>Parte con identidad verificada electrónicamente</strong> (datos completos se revelan al confirmar el pago), en adelante «${label}»`;
}

interface ContractPdfShellOptions {
  documentTitle: string;
  metaHtml: string;
  bannerHtml?: string;
  issueDate: string;
  landlordComparecenciaHtml: string;
  renterComparecenciaHtml: string;
  spaceTitle: string;
  address: string;
  vigenciaHtml: string;
  canonHtml: string;
  sections: ContractSection[];
  cancellationText: string;
  landlordSigHtml: string;
  renterSigHtml: string;
  footerHtml: string;
}

/**
 * Builds the printable HTML/CSS legal contract shell shared between
 * live reservation proposals and template sample previews.
 */
function renderContractPdfShell({
  documentTitle,
  metaHtml,
  bannerHtml,
  issueDate,
  landlordComparecenciaHtml,
  renterComparecenciaHtml,
  spaceTitle,
  address,
  vigenciaHtml,
  canonHtml,
  sections,
  cancellationText,
  landlordSigHtml,
  renterSigHtml,
  footerHtml,
}: ContractPdfShellOptions): string {
  const cancelOrdinal = contractOrdinalTitle(sections.length);
  const finalOrdinal = contractOrdinalTitle(sections.length + 1);

  return `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8" />
<title>${escapeHtml(documentTitle)}</title>
<style>
  @page { size: letter; margin: 18mm; }
  body { font-family: Georgia, "Times New Roman", serif; color: #1f2937; line-height: 1.55; margin: 0; font-size: 11pt; }
  .header { border-bottom: 2px solid #702a1b; padding-bottom: 12px; margin-bottom: 18px; display: flex; justify-content: space-between; align-items: center; }
  .brand-wrap { display: flex; align-items: center; gap: 10px; }
  .brand { font-family: system-ui, sans-serif; font-size: 17pt; font-weight: 700; color: #702a1b; line-height: 1.1; }
  .subbrand { font-family: system-ui, sans-serif; font-size: 8.5pt; color: #4b5563; text-transform: uppercase; letter-spacing: 0.06em; }
  .meta { font-family: system-ui, sans-serif; font-size: 8.5pt; color: #4b5563; text-align: right; }
  h1 { font-family: system-ui, sans-serif; font-size: 13.5pt; margin: 0 0 14px; text-align: center; text-transform: uppercase; letter-spacing: 0.05em; color: #111827; }
  .clause { margin-bottom: 11px; text-align: justify; white-space: pre-wrap; }
  .clause strong.title { font-family: system-ui, sans-serif; font-size: 10.5pt; color: #111827; }
  .box { border: 1px solid #d1d5db; border-radius: 6px; padding: 10px 14px; margin: 12px 0; background: #f9fafb; font-family: system-ui, sans-serif; font-size: 9.5pt; }
  .note { border-left: 3px solid #a44731; background: #fbf7f2; padding: 8px 12px; font-family: system-ui, sans-serif; font-size: 8.8pt; color: #4b5563; margin: 14px 0; }
  .sigs { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-top: 32px; page-break-inside: avoid; }
  .sig-box { border-top: 1.5px solid #111827; padding-top: 8px; font-family: system-ui, sans-serif; font-size: 9pt; }
  .hash { margin-top: 18px; font-family: monospace; font-size: 7.5pt; color: #6b7280; word-break: break-all; }
</style>
</head>
<body>
  <div class="header">
    <div class="brand-wrap">
      ${ESPACIGO_LOGO_SVG}
      <div>
        <div class="brand">EspaciGo</div>
        <div class="subbrand">Contrato de Arrendamiento Simple</div>
      </div>
    </div>
    <div class="meta">${metaHtml}</div>
  </div>

  <h1>Contrato de Arrendamiento de Espacio</h1>
  ${bannerHtml ?? ""}

  <p class="clause">En Chile, a ${escapeHtml(issueDate)}, comparecen: por una parte, ${landlordComparecenciaHtml}; y por la otra, ${renterComparecenciaHtml}; ambos mayores de edad, con identidad verificada electrónicamente en la plataforma EspaciGo, quienes exponen que han convenido en celebrar el siguiente Contrato de Arrendamiento Simple de espacio, el cual se regirá por las normas de la Ley N.º 18.101 sobre Arrendamiento de Predios Urbanos, supletoriamente por el Código Civil de Chile, y bajo modalidad de Firma Electrónica Simple (FES) conforme a la Ley N.º 19.799:</p>

  <p class="clause"><strong class="title">PRIMERO. INDIVIDUALIZACIÓN DEL INMUEBLE:</strong> EL ARRENDADOR declara ser propietario o estar legítimamente autorizado para dar en arrendamiento el espacio denominado «${escapeHtml(spaceTitle)}», ubicado en ${escapeHtml(address)}, el cual entrega en arrendamiento a EL ARRENDATARIO en condiciones aptas para el uso acordado.</p>

  <p class="clause"><strong class="title">SEGUNDO. OBJETO Y USO PERMITIDO:</strong> Por el presente instrumento, EL ARRENDADOR entrega en arrendamiento a EL ARRENDATARIO el espacio individualizado en la cláusula primera. EL ARRENDATARIO no podrá subarrendar, ceder ni destinar el espacio a un uso distinto o ilícito sin autorización previa y escrita de EL ARRENDADOR.</p>

  <p class="clause"><strong class="title">TERCERO. VIGENCIA, HORARIO Y RESTITUCIÓN:</strong> ${vigenciaHtml} Al término de dicho período, EL ARRENDATARIO se obliga a restituir el inmueble de inmediato, desocupado de personas y bienes propios, en el mismo estado de conservación, orden y aseo en que lo recibió, respondiendo por cualquier daño o deterioro imputable a su uso o al de sus acompañantes.</p>

  <p class="clause"><strong class="title">CUARTO. CANON DE ARRIENDO Y FORMA DE PAGO:</strong> ${canonHtml}</p>

  ${sections
    .map(
      (s, idx) =>
        `<p class="clause"><strong class="title">${contractOrdinalTitle(idx)}. ${escapeHtml((s.title || "SIN TÍTULO").toUpperCase())}:</strong> ${escapeHtml(s.body || "[Sin contenido]")}</p>`,
    )
    .join("")}

  <p class="clause"><strong class="title">${cancelOrdinal}. POLÍTICA DE CANCELACIÓN:</strong> ${escapeHtml(cancellationText)}</p>

  <p class="clause"><strong class="title">${finalOrdinal}. DOMICILIO, JURISDICCIÓN Y NATURALEZA DEL INSTRUMENTO:</strong> Para todos los efectos legales derivados de este contrato, las partes fijan domicilio en la comuna de ubicación del inmueble y se someten a la competencia de los Tribunales Ordinarios de Justicia de Chile. Las partes aceptan expresamente que este contrato se perfecciona mediante Firma Electrónica Simple (FES) conforme a la Ley N.º 19.799.</p>

  <div class="sigs">
    <div class="sig-box">${landlordSigHtml}</div>
    <div class="sig-box">${renterSigHtml}</div>
  </div>
  ${footerHtml}
</body>
</html>`;
}

function printHtmlDocument(html: string) {
  const iframe = document.createElement("iframe");
  iframe.style.position = "fixed";
  iframe.style.right = "0";
  iframe.style.bottom = "0";
  iframe.style.width = "0";
  iframe.style.height = "0";
  iframe.style.border = "0";
  iframe.setAttribute("aria-hidden", "true");
  document.body.appendChild(iframe);
  const doc = iframe.contentDocument;
  if (!doc) {
    document.body.removeChild(iframe);
    return;
  }
  doc.open();
  doc.write(html);
  doc.close();
  iframe.contentWindow?.focus();
  iframe.contentWindow?.print();
  window.setTimeout(() => {
    if (iframe.parentNode) iframe.parentNode.removeChild(iframe);
  }, 2000);
}

function printProposalAsPdf(
  proposal: ContractProposal,
  agreement?: CommercialAgreement | null,
) {
  const renterSig = agreement?.signatures.find(
    (s) => s.role === "arrendatario",
  );
  const landlordSig = agreement?.signatures.find(
    (s) => s.role === "arrendador",
  );
  const landlordParty = proposal.parties.find((p) => p.role === "arrendador");
  const renterParty = proposal.parties.find((p) => p.role === "arrendatario");
  const docId = agreement?.id ?? proposal.id;
  const issueDate = formatLegalDate(proposal.generated_at, proposal.time_zone);
  const tzLabel = formatTimeZoneLabel(proposal.time_zone);

  const html = renderContractPdfShell({
    documentTitle: `Contrato de Arrendamiento EspaciGo ${docId.slice(0, 8)}`,
    metaHtml: `
      <div>Folio: ${escapeHtml(docId.slice(0, 8))} · v${proposal.version} (Plantilla ${proposal.template_version})</div>
      <div>Emitido: ${escapeHtml(issueDate)}</div>
      ${agreement ? `<div>Estado: ${escapeHtml(agreementStateLabel(agreement.state))}</div>` : ""}
    `,
    issueDate,
    landlordComparecenciaHtml: landlordParty
      ? partyDescription(landlordParty, "EL ARRENDADOR")
      : "EL ARRENDADOR",
    renterComparecenciaHtml: renterParty
      ? partyDescription(renterParty, "EL ARRENDATARIO")
      : "EL ARRENDATARIO",
    spaceTitle: proposal.space_title,
    address: stripPostalCodeFromAddress(proposal.location.address),
    vigenciaHtml: `El arrendamiento regirá desde el ${escapeHtml(contractDate(proposal.start_at, proposal.time_zone))} hasta el ${escapeHtml(contractDate(proposal.end_at, proposal.time_zone))} (${escapeHtml(tzLabel)}).`,
    canonHtml: `La renta de arrendamiento se fija en la suma total de <strong>${escapeHtml(formatRentalCLP(proposal.subtotal_clp))}</strong> (correspondiente a ${escapeHtml(String(proposal.units))} ${escapeHtml(rateUnitWord(proposal.rate_unit, proposal.units))} a razón de ${escapeHtml(formatRentalCLP(proposal.unit_price_clp))} por ${escapeHtml(proposal.rate_unit)}), pagadera en línea mediante la plataforma EspaciGo / Mercado Pago una vez suscrito el presente contrato por ambas partes, aplicándose además la comisión de servicio vigente informada en el resumen de la operación.`,
    sections: proposal.sections,
    cancellationText: proposal.cancellation_policy.text,
    landlordSigHtml: `
      <strong>EL ARRENDADOR</strong><br/>
      ${landlordParty?.legal_name && landlordParty?.rut ? `${escapeHtml(landlordParty.legal_name)}<br/>RUT: ${escapeHtml(landlordParty.rut)}<br/>` : ""}
      ${
        landlordSig
          ? `Firmado electrónicamente el ${escapeHtml(contractDate(landlordSig.signed_at, proposal.time_zone))} (v${landlordSig.proposal_version})<br/>Modalidad: ${escapeHtml(landlordSig.signature_type ?? "FES")} · ${landlordSig.auth_method === "email_otp" ? "Verificada por correo (OTP)" : "Sesión autenticada"}`
          : "Pendiente de firma electrónica"
      }
    `,
    renterSigHtml: `
      <strong>EL ARRENDATARIO</strong><br/>
      ${renterParty?.legal_name && renterParty?.rut ? `${escapeHtml(renterParty.legal_name)}<br/>RUT: ${escapeHtml(renterParty.rut)}<br/>` : ""}
      ${
        renterSig
          ? `Firmado electrónicamente el ${escapeHtml(contractDate(renterSig.signed_at, proposal.time_zone))} (v${renterSig.proposal_version})<br/>Modalidad: ${escapeHtml(renterSig.signature_type ?? "FES")} · ${renterSig.auth_method === "email_otp" ? "Verificada por correo (OTP)" : "Sesión autenticada"}`
          : "Pendiente de firma electrónica"
      }
    `,
    footerHtml: `<div class="hash">Integridad SHA-256 del documento: ${escapeHtml(proposal.sha256)}</div>`,
  });

  printHtmlDocument(html);
}

/**
 * Generates and opens the browser print/PDF dialog for a sample preview of a
 * host contract template before a booking exists.
 */
export function printTemplatePreviewAsPdf(preview: TemplatePreviewInput) {
  const issueDate = formatLegalDate(
    new Date().toISOString(),
    "America/Santiago",
  );
  const spaceTitle = preview.spaceTitle || "Espacio publicado";
  const address =
    stripPostalCodeFromAddress(preview.address || "") ||
    "Dirección confirmada del espacio";
  const cancellationText =
    preview.cancellationPolicyText ||
    "Política de cancelación configurada para el espacio.";
  const hasTariff = Boolean(preview.basePriceClp && preview.basePriceClp > 0);
  const tariffSummary = hasTariff
    ? `la suma total en CLP según duración seleccionada (a razón de <strong>$${escapeHtml(preview.basePriceClp!.toLocaleString("es-CL"))} CLP por ${escapeHtml(rateUnitWord(preview.rateUnit ?? "hora", 1))}</strong>)`
    : `[monto total en CLP según tarifa y duración seleccionada]`;

  const html = renderContractPdfShell({
    documentTitle: `Vista previa de plantilla — ${spaceTitle}`,
    metaHtml: `
      <div><strong>Vista previa de plantilla (borrador de muestra)</strong></div>
      <div>${preview.templateVersion ? `Versión publicada v${preview.templateVersion}` : "Borrador sin publicar"}</div>
      <div>Fecha de emisión: ${escapeHtml(issueDate)}</div>
    `,
    bannerHtml: `<div class="note"><strong>Vista previa de plantilla (borrador de muestra):</strong> Este documento ilustra el formato oficial que recibirán las partes al confirmar una reserva. Las identidades, fechas de arriendo y montos totales se completan automáticamente en cada operación.</div>`,
    issueDate,
    landlordComparecenciaHtml: `<strong>[Identidad verificada del Arrendador]</strong>, en adelante «EL ARRENDADOR»`,
    renterComparecenciaHtml: `<strong>[Identidad verificada del Arrendatario]</strong>, en adelante «EL ARRENDATARIO»`,
    spaceTitle,
    address,
    vigenciaHtml: `El arrendamiento regirá [desde fecha/hora de inicio hasta fecha/hora de término de la reserva (hora de Chile continental)].`,
    canonHtml: `La renta de arrendamiento se fija en ${tariffSummary}, pagadera en línea mediante la plataforma EspaciGo / Mercado Pago una vez suscrito el presente contrato por ambas partes, aplicándose además la comisión de servicio vigente informada en el resumen de la operación.`,
    sections: preview.sections,
    cancellationText,
    landlordSigHtml: `<strong>EL ARRENDADOR</strong><br/>Se completará con la firma electrónica simple (FES) del arrendador`,
    renterSigHtml: `<strong>EL ARRENDATARIO</strong><br/>Se completará con la firma electrónica simple (FES) del arrendatario`,
    footerHtml: `<div class="hash">Vista previa de plantilla (borrador de muestra) · Documento referencial sin validez contractual hasta su emisión y firma electrónica en una reserva confirmada.</div>`,
  });

  printHtmlDocument(html);
}

export function ContractPdfModal({
  open,
  onClose,
  proposal,
  agreement,
  title = "Documento de Contrato",
  footer,
}: {
  open: boolean;
  onClose: () => void;
  proposal: ContractProposal | null;
  agreement?: CommercialAgreement | null;
  title?: string;
  footer?: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open, onClose]);

  if (!open || !proposal) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-2 sm:p-4"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-border bg-white shadow-2xl">
        {/* Top bar */}
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-white px-4 py-3 sm:px-6">
          <div className="flex items-center gap-2.5 min-w-0">
            <FileText
              className="size-5 shrink-0 text-[#a44731]"
              aria-hidden="true"
            />
            <div className="min-w-0">
              <h2 className="truncate text-base font-semibold text-gray-900">
                {title}
              </h2>
              {agreement && (
                <p className="text-xs text-muted-foreground">
                  {agreementStateLabel(agreement.state)} · Folio{" "}
                  {agreement.id.slice(0, 8)}
                </p>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => printProposalAsPdf(proposal, agreement)}
            >
              <Download className="size-4" aria-hidden="true" />
              <span>Descargar PDF</span>
            </Button>
            <button
              type="button"
              aria-label="Cerrar contrato"
              onClick={onClose}
              className="inline-flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            >
              <X className="size-5" aria-hidden="true" />
            </button>
          </div>
        </div>

        {/* Paper PDF viewport */}
        <div className="flex-1 overflow-y-auto bg-neutral-100 p-3 sm:p-6">
          <ContractDocumentView proposal={proposal} agreement={agreement} />
        </div>

        {/* Sticky footer */}
        {footer && (
          <div className="border-t border-border bg-white px-4 py-3 sm:px-6">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}
