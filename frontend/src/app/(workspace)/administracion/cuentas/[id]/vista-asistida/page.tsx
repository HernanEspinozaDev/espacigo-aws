import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { isValidUUID } from "@/features/admin/domain/admin-rules";
import { AdminAssistedViewPanel } from "@/features/admin/ui/admin-assisted-view-panel";

export const metadata: Metadata = {
  title: "Vista asistida | Administración | EspaciGo",
  robots: { index: false, follow: false },
};

export default async function AdminAssistedViewPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  if (!isValidUUID(id)) notFound();
  return <AdminAssistedViewPanel accountId={id} />;
}
