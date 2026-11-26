import { redirect } from "next/navigation";
import { verificationReturnHref } from "@/features/host-listings/domain/publish-guide";

export default async function VerificationReturnPage({
  searchParams,
}: {
  searchParams: Promise<{ return_to?: string | string[] }>;
}) {
  const { return_to } = await searchParams;
  redirect(verificationReturnHref(return_to, { refresh: true }));
}
