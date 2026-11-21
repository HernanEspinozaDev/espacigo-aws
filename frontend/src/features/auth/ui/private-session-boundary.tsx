"use client";

import { Fragment, useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./use-auth";

export function PrivateSessionBoundary({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { snapshot } = useAuth();
  const { status, accountId, roles, capabilities, restrictedMode, expiresAt } =
    snapshot;
  const hasIdentity = status === "authenticated" && accountId !== null;

  useEffect(() => {
    if (status === "anonymous") {
      router.replace("/");
    }
  }, [status, router]);

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
