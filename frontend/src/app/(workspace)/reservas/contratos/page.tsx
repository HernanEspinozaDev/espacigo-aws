import Link from "next/link";
import type { Metadata } from "next";
import { ContractsWorkspace } from "@/features/commercial-contracts/ui/contracts-workspace";

export const metadata: Metadata = {
  title: "Mis contratos | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function ContractRequestsPage({
  searchParams,
}: {
  searchParams: Promise<{
    rol?: string | string[];
    solicitud?: string | string[];
  }>;
}) {
  const { rol, solicitud } = await searchParams;
  const initialRole = typeof rol === "string" ? rol : undefined;
  const initialAgreementId =
    typeof solicitud === "string" ? solicitud : undefined;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header className="space-y-2">
        <h1 className="text-2xl font-semibold">Mis contratos</h1>
        <p className="text-sm text-muted-foreground">
          Revisa y firma las solicitudes que recibes como arrendador o haz
          seguimiento a las solicitudes que enviaste como arrendatario.
        </p>
      </header>
      <ContractsWorkspace
        initialRole={initialRole}
        initialAgreementId={initialAgreementId}
      />
      <Link
        href="/explorar"
        className="inline-flex min-h-11 items-center text-sm underline underline-offset-4"
      >
        Explorar espacios
      </Link>
    </div>
  );
}
