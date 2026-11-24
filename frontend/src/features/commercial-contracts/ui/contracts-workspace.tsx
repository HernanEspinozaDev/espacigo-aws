"use client";

import { useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { HostAgreementRequests } from "@/features/host-agreements/ui/host-agreement-requests";
import { hostAgreements } from "@/features/host-agreements/host-agreements-runtime";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import { RenterAgreementList } from "./renter-agreement-list";
import { cn } from "@/lib/utils";

type RoleTab = "arrendador" | "arrendatario";

export function ContractsWorkspace({
  initialRole,
  initialAgreementId,
}: {
  initialRole?: string;
  initialAgreementId?: string;
}) {
  const { accessToken, snapshot } = useAuth();
  const explicitTab: RoleTab | null =
    initialRole === "arrendador" ||
    initialRole === "anfitrion" ||
    Boolean(initialAgreementId)
      ? "arrendador"
      : initialRole === "arrendatario"
        ? "arrendatario"
        : null;

  const [selectedTab, setSelectedTab] = useState<RoleTab>(
    explicitTab ?? "arrendador",
  );
  const [userPickedTab, setUserPickedTab] = useState(false);
  const tab = userPickedTab ? selectedTab : (explicitTab ?? selectedTab);
  const [counts, setCounts] = useState<{
    landlordPending: number;
    landlordTotal: number;
    renterTotal: number;
  }>({ landlordPending: 0, landlordTotal: 0, renterTotal: 0 });

  useEffect(() => {
    if (!accessToken || snapshot.restrictedMode) return;
    const controller = new AbortController();
    Promise.allSettled([
      hostAgreements.list(controller.signal),
      commercialAgreements.list("renter"),
    ]).then(([landlordRes, renterRes]) => {
      if (controller.signal.aborted) return;
      const landlordItems =
        landlordRes.status === "fulfilled" ? landlordRes.value : [];
      const renterItems =
        renterRes.status === "fulfilled" ? renterRes.value : [];
      const landlordPending = landlordItems.filter(
        (item) => item.state === "pending_landlord",
      ).length;

      setCounts({
        landlordPending,
        landlordTotal: landlordItems.length,
        renterTotal: renterItems.length,
      });

      if (!explicitTab && !userPickedTab) {
        if (landlordPending > 0 || landlordItems.length > 0) {
          setSelectedTab("arrendador");
        } else if (renterItems.length > 0) {
          setSelectedTab("arrendatario");
        }
      }
    });
    return () => controller.abort();
  }, [
    accessToken,
    snapshot.restrictedMode,
    snapshot.accountId,
    explicitTab,
    userPickedTab,
  ]);

  return (
    <div className="space-y-6">
      <div
        role="tablist"
        aria-label="Rol en contratos"
        className="flex flex-wrap gap-2 border-b border-border pb-3"
      >
        <button
          type="button"
          role="tab"
          aria-selected={tab === "arrendador"}
          onClick={() => {
            setUserPickedTab(true);
            setSelectedTab("arrendador");
          }}
          className={cn(
            "inline-flex min-h-10 items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors",
            tab === "arrendador"
              ? "bg-primary text-primary-foreground"
              : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground",
          )}
        >
          <span>Como arrendador (recibidas)</span>
          {counts.landlordPending > 0 ? (
            <span className="inline-flex min-w-5 h-5 items-center justify-center rounded-full bg-red-600 px-1.5 text-xs font-semibold tabular-nums text-white">
              {counts.landlordPending}
            </span>
          ) : counts.landlordTotal > 0 ? (
            <span className="text-xs tabular-nums opacity-80">
              ({counts.landlordTotal})
            </span>
          ) : null}
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={tab === "arrendatario"}
          onClick={() => {
            setUserPickedTab(true);
            setSelectedTab("arrendatario");
          }}
          className={cn(
            "inline-flex min-h-10 items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors",
            tab === "arrendatario"
              ? "bg-primary text-primary-foreground"
              : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground",
          )}
        >
          <span>Como arrendatario (enviadas)</span>
          {counts.renterTotal > 0 && (
            <span className="text-xs opacity-80">({counts.renterTotal})</span>
          )}
        </button>
      </div>

      {tab === "arrendador" ? (
        <HostAgreementRequests initialAgreementId={initialAgreementId} />
      ) : (
        <RenterAgreementList />
      )}
    </div>
  );
}
