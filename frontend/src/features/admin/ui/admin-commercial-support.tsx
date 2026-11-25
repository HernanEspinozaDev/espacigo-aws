"use client";

import { CommercialCases } from "@/features/bookings/ui/commercial-support-panel";
import { useAdmin } from "./use-admin";

export function AdminCommercialSupport() {
  const { snapshot, hasAdminCapability } = useAdmin();
  if (
    snapshot.adminSessionStatus !== "authenticated" ||
    !hasAdminCapability("support.tickets.manage")
  )
    return null;
  return <CommercialCases key={snapshot.me?.account_id ?? "admin"} />;
}
