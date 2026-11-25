import { redirect } from "next/navigation";
import { safeHostReturnPath } from "@/features/host-listings/domain/publish-guide";

export default async function VerificationReturnPage({
  searchParams,
}: {
  searchParams: Promise<{ return_to?: string | string[] }>;
}) {
  const { return_to } = await searchParams;
  const returnTo = safeHostReturnPath(return_to);
  const destination = returnTo
    ? "/mi-cuenta/perfil?refresh_identity=1&return_to=" +
      encodeURIComponent(returnTo) +
      "#identity-verification"
    : "/mi-cuenta/perfil?refresh_identity=1#identity-verification";
  redirect(destination);
}
