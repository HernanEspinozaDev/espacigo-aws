import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { isValidUUID } from "@/features/admin/domain/admin-rules";
import { AdminCustomer360 } from "@/features/admin/ui/admin-customer-360";

export const metadata: Metadata = {
  title: "Ficha 360 | Administración | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function AdminAccountPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  if (!isValidUUID(id)) notFound();
  return <AdminCustomer360 accountId={id} />;
}
