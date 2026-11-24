"use client";

import { IdentityDeclarationPanel } from "@/features/identity-declaration/ui/identity-declaration-panel";
import { useAuth } from "@/features/auth/ui/use-auth";
import { DocumentVerificationView } from "./document-verification-view";

const steps = [
  {
    number: "1",
    title: "Declaración",
    description: "Guarda tus datos legales",
  },
  { number: "2", title: "Expediente", description: "Confirma formato y aviso" },
  {
    number: "3",
    title: "Evidencias",
    description: "Carga anverso, reverso y selfie",
  },
  {
    number: "4",
    title: "Resultado",
    description: "Consulta el estado actualizado",
  },
] as const;

export function IdentityVerificationFlow() {
  const { accessToken } = useAuth();

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          Verificación de identidad
        </h1>
        <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
          Sigue las cuatro etapas. La declaración y el expediente se consultan
          desde el servidor; la revisión la realiza una persona autorizada.
        </p>
      </header>

      <nav aria-label="Etapas de verificación" className="overflow-hidden">
        <ol className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
          {steps.map((step) => (
            <li
              key={step.number}
              className="flex min-w-0 items-start gap-3 rounded-lg border bg-card p-3"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary">
                {step.number}
              </span>
              <span className="min-w-0">
                <span className="block text-sm font-semibold">
                  {step.title}
                </span>
                <span className="block text-xs leading-snug text-muted-foreground">
                  {step.description}
                </span>
              </span>
            </li>
          ))}
        </ol>
      </nav>

      <section aria-label="Etapa 1: declaración legal">
        <IdentityDeclarationPanel accessToken={accessToken} />
      </section>

      <section aria-label="Etapas 2 a 4: expediente y resultado">
        <DocumentVerificationView identityOnly />
      </section>
    </div>
  );
}
