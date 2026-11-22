"use client";

import { Fragment, useEffect, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { resolveReturnTarget } from "../domain/return-target";
import { useAuth } from "./use-auth";

export function PrivateSessionBoundary({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { snapshot } = useAuth();
  const { status, accountId, roles, capabilities, restrictedMode, expiresAt } =
    snapshot;
  const hasIdentity = status === "authenticated" && accountId !== null;

  useEffect(() => {
    if (status !== "anonymous") return;
    if (pathname === "/reservas/nueva") {
      const search =
        typeof window !== "undefined" ? window.location.search : "";
      const target = resolveReturnTarget(`${pathname}${search}`);
      if (target !== "/") {
        router.replace(`/acceso?return_to=${encodeURIComponent(target)}`);
        return;
      }
    }
    router.replace("/");
  }, [pathname, status, router]);

  if (status === "anonymous") {
    return null;
  }

  const sessionKey = hasIdentity
    ? JSON.stringify([
        status,
        accountId,
        roles,
        capabilities,
        restrictedMode,
        expiresAt,
      ])
    : "guest";

  return <Fragment key={sessionKey}>{children}</Fragment>;
}
