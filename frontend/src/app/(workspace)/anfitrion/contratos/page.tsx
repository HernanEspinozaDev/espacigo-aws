import { HostAgreementRequests } from "@/features/host-agreements/ui/host-agreement-requests";

export default async function HostContratosPage({
  searchParams,
}: {
  searchParams: Promise<{ solicitud?: string | string[] }>;
}) {
  const { solicitud } = await searchParams;
  const initialAgreementId =
    typeof solicitud === "string" ? solicitud : undefined;

  return (
    <div className="space-y-6">
      <HostAgreementRequests initialAgreementId={initialAgreementId} />
    </div>
  );
}
