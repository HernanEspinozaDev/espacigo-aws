"use client";

import Image from "next/image";
import { formatRentalCLP } from "@/features/public-catalog/domain/contract-preview-format";
import { stripPostalCodeFromAddress } from "@/features/host-listings/domain/commercial-location";
import type { ContractSection } from "@/features/host-listings/domain/contract-template";
import type {
  CommercialAgreement,
  ContractProposal,
} from "../domain/agreements";
import { contractDate } from "./contract-proposal-terms";

const ORDINALS = [
  "QUINTO",
  "SEXTO",
  "SÉPTIMO",
  "OCTAVO",
  "NOVENO",
  "DÉCIMO",
  "DÉCIMO PRIMERO",
  "DÉCIMO SEGUNDO",
  "DÉCIMO TERCERO",
  "DÉCIMO CUARTO",
  "DÉCIMO QUINTO",
  "DÉCIMO SEXTO",
];

export function contractOrdinalTitle(index: number) {
  return ORDINALS[index] ?? `CLÁUSULA ${index + 5}`;
}

export function rateUnitWord(unit: string, units: number | string) {
  const count = Number(units);
  const plural = count !== 1;
  if (unit === "hora") return plural ? "horas" : "hora";
  if (unit === "dia") return plural ? "días" : "día";
  if (unit === "mes") return plural ? "meses" : "mes";
  return unit;
}

export function formatLegalDate(value: string, timeZone: string) {
  return new Intl.DateTimeFormat("es-CL", {
    dateStyle: "long",
    timeZone,
  }).format(new Date(value));
}

export function formatTimeZoneLabel(timeZone: string) {
  return timeZone === "America/Santiago"
    ? "hora de Chile continental"
    : `zona horaria ${timeZone}`;
}

export interface TemplatePreviewInput {
  spaceTitle: string;
  address: string;
  cancellationPolicyText: string;
  sections: ContractSection[];
  templateVersion?: number;
  rateUnit?: string;
  basePriceClp?: number | null;
}

export function ContractDocumentView({
  proposal,
  agreement,
  templatePreview,
}: {
  proposal?: ContractProposal | null;
  agreement?: CommercialAgreement | null;
  templatePreview?: TemplatePreviewInput;
}) {
  if (!proposal && !templatePreview) return null;

  const isPreview = !proposal && Boolean(templatePreview);
  const spaceTitle =
    proposal?.space_title ??
    (templatePreview?.spaceTitle || "Nombre del espacio");
  const address =
    stripPostalCodeFromAddress(
      proposal?.location.address ?? templatePreview?.address ?? "",
    ) || "Dirección confirmada del inmueble";
  const sections = proposal?.sections ?? templatePreview?.sections ?? [];
  const cancellationText =
    proposal?.cancellation_policy.text ??
    (templatePreview?.cancellationPolicyText ||
      "Política de cancelación configurada para el espacio.");
  const issueDate = proposal
    ? formatLegalDate(proposal.generated_at, proposal.time_zone)
    : formatLegalDate(new Date().toISOString(), "America/Santiago");
  const tzLabel = proposal
    ? formatTimeZoneLabel(proposal.time_zone)
    : "hora de Chile continental";

  const renterSig = agreement?.signatures.find(
    (s) => s.role === "arrendatario",
  );
  const landlordSig = agreement?.signatures.find(
    (s) => s.role === "arrendador",
  );
  const landlordParty = proposal?.parties.find((p) => p.role === "arrendador");
  const renterParty = proposal?.parties.find((p) => p.role === "arrendatario");
  const cancelOrdinal = contractOrdinalTitle(sections.length);
  const finalOrdinal = contractOrdinalTitle(sections.length + 1);

  return (
    <article className="mx-auto max-w-3xl space-y-5 rounded-lg border border-neutral-200 bg-white p-5 shadow-sm sm:p-10 font-serif text-neutral-800">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b-2 border-[#702a1b] pb-4 font-sans">
        <div className="flex items-center gap-3">
          <Image
            src="/espacigo-mark.svg"
            width={36}
            height={36}
            alt="EspaciGo"
            className="shrink-0"
          />
          <div>
            <p className="text-xl font-bold tracking-tight text-[#702a1b] leading-none">
              EspaciGo
            </p>
            <p className="mt-1 text-[11px] font-semibold uppercase tracking-wider text-neutral-500">
              Contrato de Arrendamiento Simple
            </p>
          </div>
        </div>
        <div className="text-right text-xs text-neutral-600">
          {proposal ? (
            <>
              <p>
                Folio: {(agreement?.id ?? proposal.id).slice(0, 8)} · v
                {proposal.version} (Plantilla v{proposal.template_version})
              </p>
              <p>Emitido: {issueDate}</p>
            </>
          ) : (
            <>
              <p>
                Vista previa de plantilla{" "}
                {templatePreview?.templateVersion
                  ? `(v${templatePreview.templateVersion})`
                  : "(Borrador)"}
              </p>
              <p>Fecha de emisión: {issueDate}</p>
            </>
          )}
        </div>
      </header>

      <h3 className="text-center font-sans text-base font-bold uppercase tracking-wider text-neutral-900">
        Contrato de Arrendamiento de Espacio
      </h3>

      {/* Comparecencia */}
      <p className="text-sm leading-relaxed text-justify">
        En Chile, a {issueDate}, comparecen: por una parte,{" "}
        {isPreview ? (
          <span className="rounded bg-amber-100/80 px-1.5 py-0.5 font-sans text-xs font-medium text-amber-950">
            [Identidad verificada del Arrendador]
          </span>
        ) : landlordParty?.legal_name && landlordParty?.rut ? (
          <>
            <strong>{landlordParty.legal_name}</strong>, cédula de identidad /
            RUT N.º <strong>{landlordParty.rut}</strong>
          </>
        ) : (
          <strong>
            Parte con identidad verificada (datos completos se completan al
            confirmar el pago)
          </strong>
        )}
        , en adelante «<strong>EL ARRENDADOR</strong>»; y por la otra,{" "}
        {isPreview ? (
          <span className="rounded bg-amber-100/80 px-1.5 py-0.5 font-sans text-xs font-medium text-amber-950">
            [Identidad verificada del Arrendatario]
          </span>
        ) : renterParty?.legal_name && renterParty?.rut ? (
          <>
            <strong>{renterParty.legal_name}</strong>, cédula de identidad / RUT
            N.º <strong>{renterParty.rut}</strong>
          </>
        ) : (
          <strong>
            Parte con identidad verificada (datos completos se completan al
            confirmar el pago)
          </strong>
        )}
        , en adelante «<strong>EL ARRENDATARIO</strong>»; ambos mayores de edad,
        con identidad verificada en EspaciGo, quienes convienen en celebrar el
        siguiente Contrato de Arrendamiento Simple, regido por la Ley N.º
        18.101, supletoriamente por el Código Civil de Chile, y suscrito
        mediante Firma Electrónica Simple (FES) conforme a la Ley N.º 19.799:
      </p>

      {/* Cláusula 1 */}
      <p className="text-sm leading-relaxed text-justify">
        <strong className="font-sans text-neutral-900">
          PRIMERO. INDIVIDUALIZACIÓN DEL INMUEBLE:
        </strong>{" "}
        EL ARRENDADOR declara ser propietario o estar legítimamente autorizado
        para dar en arrendamiento el espacio denominado «{spaceTitle}», ubicado
        en {address}, el cual entrega a EL ARRENDATARIO en condiciones aptas
        para el uso acordado.
      </p>

      {/* Cláusula 2 */}
      <p className="text-sm leading-relaxed text-justify">
        <strong className="font-sans text-neutral-900">
          SEGUNDO. OBJETO Y USO PERMITIDO:
        </strong>{" "}
        Por el presente instrumento, EL ARRENDADOR entrega en arrendamiento a EL
        ARRENDATARIO el espacio individualizado en la cláusula primera. EL
        ARRENDATARIO no podrá subarrendar, ceder ni destinar el espacio a un uso
        distinto o ilícito sin autorización previa y escrita de EL ARRENDADOR.
      </p>

      {/* Cláusula 3 */}
      <p className="text-sm leading-relaxed text-justify">
        <strong className="font-sans text-neutral-900">
          TERCERO. VIGENCIA, HORARIO Y RESTITUCIÓN:
        </strong>{" "}
        El arrendamiento regirá{" "}
        {proposal ? (
          <>
            desde el {contractDate(proposal.start_at, proposal.time_zone)} hasta
            el {contractDate(proposal.end_at, proposal.time_zone)} ({tzLabel})
          </>
        ) : (
          <span className="rounded bg-amber-100/80 px-1.5 py-0.5 font-sans text-xs font-medium text-amber-950">
            [desde fecha/hora de inicio hasta fecha/hora de término de la
            reserva ({tzLabel})]
          </span>
        )}
        . Al término de dicho período, EL ARRENDATARIO deberá restituir el
        inmueble de inmediato, desocupado de personas y bienes propios, en el
        mismo estado de conservación, orden y aseo en que lo recibió,
        respondiendo por cualquier daño imputable.
      </p>

      {/* Cláusula 4 */}
      <p className="text-sm leading-relaxed text-justify">
        <strong className="font-sans text-neutral-900">
          CUARTO. CANON DE ARRIENDO Y FORMA DE PAGO:
        </strong>{" "}
        La renta de arrendamiento se fija en{" "}
        {proposal ? (
          <>
            <strong>{formatRentalCLP(proposal.subtotal_clp)}</strong> (
            {proposal.units} {rateUnitWord(proposal.rate_unit, proposal.units)}{" "}
            a razón de {formatRentalCLP(proposal.unit_price_clp)} por{" "}
            {proposal.rate_unit})
          </>
        ) : templatePreview?.basePriceClp &&
          templatePreview.basePriceClp > 0 ? (
          <>
            <span className="rounded bg-amber-100/80 px-1.5 py-0.5 font-sans text-xs font-medium text-amber-950">
              [monto total en CLP según duración seleccionada]
            </span>{" "}
            (a razón de{" "}
            <strong>
              ${templatePreview.basePriceClp.toLocaleString("es-CL")} CLP por{" "}
              {rateUnitWord(templatePreview.rateUnit ?? "hora", 1)}
            </strong>
            )
          </>
        ) : (
          <span className="rounded bg-amber-100/80 px-1.5 py-0.5 font-sans text-xs font-medium text-amber-950">
            [monto total en CLP según tarifa y duración seleccionada]
          </span>
        )}
        , pagadera en línea mediante EspaciGo / Mercado Pago una vez suscrito
        este contrato por ambas partes, aplicándose la comisión de servicio
        informada en la operación.
      </p>

      {/* Cláusulas particulares del arrendador */}
      {sections.map((section, idx) => (
        <p
          key={section.key}
          className={
            isPreview
              ? "rounded-md border border-dashed border-[#a44731]/40 bg-[#fbf7f2]/60 px-3 py-2 text-sm leading-relaxed text-justify whitespace-pre-wrap break-words"
              : "text-sm leading-relaxed text-justify whitespace-pre-wrap break-words"
          }
        >
          <strong className="font-sans text-neutral-900">
            {contractOrdinalTitle(idx)}.{" "}
            {(section.title || "SIN TÍTULO").toUpperCase()}:
          </strong>{" "}
          {section.body || (
            <span className="italic text-neutral-400">
              [Escribe aquí las reglas de esta cláusula…]
            </span>
          )}
        </p>
      ))}

      {/* Cancelación */}
      <p className="text-sm leading-relaxed text-justify whitespace-pre-wrap break-words">
        <strong className="font-sans text-neutral-900">
          {cancelOrdinal}. POLÍTICA DE CANCELACIÓN:
        </strong>{" "}
        {cancellationText}
      </p>

      {/* Jurisdicción */}
      <p className="text-sm leading-relaxed text-justify">
        <strong className="font-sans text-neutral-900">
          {finalOrdinal}. DOMICILIO, JURISDICCIÓN Y NATURALEZA DEL INSTRUMENTO:
        </strong>{" "}
        Para todos los efectos legales derivados de este contrato, las partes
        fijan domicilio en la comuna del inmueble y se someten a la competencia
        de los Tribunales Ordinarios de Justicia de Chile, perfeccionándose
        mediante Firma Electrónica Simple (FES) conforme a la Ley N.º 19.799.
      </p>

      <section className="grid gap-4 border-t border-neutral-300 pt-6 sm:grid-cols-2 font-sans text-xs">
        <div className="rounded-lg border border-neutral-200 p-3.5 space-y-1">
          <p className="font-bold text-neutral-900">EL ARRENDADOR</p>
          {landlordParty?.legal_name && landlordParty?.rut && (
            <p className="text-neutral-700">
              {landlordParty.legal_name} · RUT {landlordParty.rut}
            </p>
          )}
          <p className="text-neutral-600">
            {isPreview
              ? "Se completará con la firma electrónica (FES) del arrendador"
              : landlordSig
                ? `Firmado el ${contractDate(landlordSig.signed_at, proposal!.time_zone)} · v${landlordSig.proposal_version}`
                : "Pendiente de firma electrónica"}
          </p>
          {landlordSig && (
            <p className="text-[11px] text-neutral-500">
              Modalidad: {landlordSig.signature_type ?? "FES"} ·{" "}
              {landlordSig.auth_method === "email_otp"
                ? "Verificada por correo (OTP)"
                : landlordSig.auth_method === "auto_authorization"
                  ? "Firma automática acotada"
                  : "Sesión autenticada"}
            </p>
          )}
        </div>
        <div className="rounded-lg border border-neutral-200 p-3.5 space-y-1">
          <p className="font-bold text-neutral-900">EL ARRENDATARIO</p>
          {renterParty?.legal_name && renterParty?.rut && (
            <p className="text-neutral-700">
              {renterParty.legal_name} · RUT {renterParty.rut}
            </p>
          )}
          <p className="text-neutral-600">
            {isPreview
              ? "Se completará con la firma electrónica (FES) del arrendatario"
              : renterSig
                ? `Firmado el ${contractDate(renterSig.signed_at, proposal!.time_zone)} · v${renterSig.proposal_version}`
                : "Pendiente de firma electrónica"}
          </p>
          {renterSig && (
            <p className="text-[11px] text-neutral-500">
              Modalidad: {renterSig.signature_type ?? "FES"} ·{" "}
              {renterSig.auth_method === "email_otp"
                ? "Verificada por correo (OTP)"
                : "Sesión autenticada"}
            </p>
          )}
        </div>
      </section>

      {proposal && (
        <p className="break-all font-mono text-[11px] text-neutral-400">
          SHA-256: {proposal.sha256}
        </p>
      )}
    </article>
  );
}
