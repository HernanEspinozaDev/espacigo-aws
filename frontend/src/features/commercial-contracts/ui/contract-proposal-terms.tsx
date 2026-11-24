import type { ContractProposal } from "../domain/agreements";
import { formatRentalCLP } from "@/features/public-catalog/domain/contract-preview-format";
import { stripPostalCodeFromAddress } from "@/features/host-listings/domain/commercial-location";

export function contractDate(value: string, timeZone: string) {
  return new Intl.DateTimeFormat("es-CL", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone,
  }).format(new Date(value));
}
export function ContractProposalTerms({
  proposal,
}: {
  proposal: ContractProposal;
}) {
  return (
    <div className="space-y-5 text-sm">
      <div>
        <h3 className="font-semibold">{proposal.space_title}</h3>
        <p>{stripPostalCodeFromAddress(proposal.location.address)}</p>
        <p>
          {contractDate(proposal.start_at, proposal.time_zone)} —{" "}
          {contractDate(proposal.end_at, proposal.time_zone)}
        </p>
        <p className="mt-2 font-semibold">
          Total: {formatRentalCLP(proposal.subtotal_clp)} CLP
        </p>
      </div>
      <p className="rounded-lg border border-border bg-muted/30 p-3">
        {proposal.contact_notice}
      </p>
      <dl className="grid gap-3 sm:grid-cols-2">
        {proposal.parties.map((p) => (
          <div className="rounded-lg border border-border p-3" key={p.role}>
            <dt className="font-medium">
              {p.role === "arrendador" ? "Anfitrión" : "Arrendatario"}
              {p.self ? " (tú)" : ""}
            </dt>
            <dd>
              {p.self ? (
                <>
                  {p.legal_name}
                  <br />
                  RUT: {p.rut}
                </>
              ) : (
                "Identidad verificada. Datos personales reservados hasta confirmar el pago."
              )}
            </dd>
          </div>
        ))}
      </dl>
      <div className="space-y-4">
        {proposal.sections.map((section) => (
          <section key={section.key}>
            <h4 className="font-semibold">{section.title}</h4>
            <p className="whitespace-pre-wrap break-words">{section.body}</p>
          </section>
        ))}
      </div>
      <section>
        <h4 className="font-semibold">Política de cancelación</h4>
        <p className="whitespace-pre-wrap break-words">
          {proposal.cancellation_policy.text}
        </p>
      </section>
      <p className="text-xs text-muted-foreground">
        Acuerdo versión {proposal.version} · Plantilla publicada{" "}
        {proposal.template_version}. Esta propuesta no retiene el horario ni
        confirma una reserva.
      </p>
    </div>
  );
}
