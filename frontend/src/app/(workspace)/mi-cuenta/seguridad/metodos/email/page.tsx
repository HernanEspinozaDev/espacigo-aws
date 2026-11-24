"use client";

import { Button } from "@/components/ui/button";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

export default function EmailMetodoPage() {
  const { summary, isLoading, error, refresh } = useSecurityMethodsSummary();
  const email = summary?.methods.email;

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Correo electrónico"
        description="Tu correo principal recibe avisos de seguridad y permite recuperar el acceso a tu cuenta."
        overview
      />
      <section
        className="space-y-3 rounded-2xl border bg-card p-5"
        aria-busy={isLoading}
      >
        <h2 className="font-semibold">Correo principal</h2>
        {isLoading ? (
          <p role="status" className="text-sm text-muted-foreground">
            Consultando el estado…
          </p>
        ) : error || !email ? (
          <>
            <p role="alert" className="text-sm">
              No pudimos consultar el estado de tu correo.
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void refresh()}
            >
              Reintentar
            </Button>
          </>
        ) : email.enabled ? (
          <>
            <p className="break-all text-sm">{email.masked_email}</p>
            <p className="text-sm font-medium text-emerald-800 dark:text-emerald-300">
              Verificado
            </p>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            Sin correo verificado.
          </p>
        )}
      </section>
    </div>
  );
}
