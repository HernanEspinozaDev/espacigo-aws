import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { isPublicUuid } from "@/features/public-catalog/domain/media";
import { RenterAgreementPanel } from "@/features/commercial-contracts/ui/renter-agreement-panel";
export const metadata: Metadata = {
  title: "Mi solicitud de contrato | EspaciGo",
  robots: { index: false, follow: false },
};
export default async function ContractRequestPage({
  params,
}: {
  params: Promise<{ agreementID: string }>;
}) {
  const { agreementID } = await params;
  if (!isPublicUuid(agreementID)) notFound();
  const checkoutApiEnabled =
    process.env.NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED === "1" &&
    Boolean(process.env.NEXT_PUBLIC_MP_PUBLIC_KEY?.startsWith("TEST-"));
  const walletPurchaseEnabled =
    process.env.NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED === "1";
  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <h1 className="text-2xl font-semibold">Mi solicitud de contrato</h1>
      <RenterAgreementPanel
        agreementId={agreementID}
        checkoutApiEnabled={checkoutApiEnabled}
        walletPurchaseEnabled={walletPurchaseEnabled}
      />
      <Link
        href="/reservas/contratos"
        className="inline-flex min-h-11 items-center text-sm underline underline-offset-4"
      >
        Volver a mis solicitudes
      </Link>
    </div>
  );
}
