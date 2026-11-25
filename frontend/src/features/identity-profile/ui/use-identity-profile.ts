"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { identityProfileStore } from "../identity-profile-runtime";

export function useIdentityProfile() {
  const { snapshot: auth } = useAuth();
  const snapshot = useSyncExternalStore(
    identityProfileStore.subscribe,
    identityProfileStore.getSnapshot,
    identityProfileStore.getServerSnapshot,
  );
  useEffect(() => {
    void identityProfileStore.setAccount(
      auth.status === "authenticated" ? auth.accountId : null,
    );
  }, [auth.accountId, auth.status]);
  const save = useCallback(
    (input: Parameters<typeof identityProfileStore.save>[0]) =>
      identityProfileStore.save(input),
    [],
  );
  const saveUsername = useCallback(
    (username: string) => identityProfileStore.saveUsername(username),
    [],
  );
  const saveDraft = useCallback(
    (draft: Parameters<typeof identityProfileStore.saveDraft>[0]) =>
      identityProfileStore.saveDraft(draft),
    [],
  );
  const restoreDraft = useCallback(
    (password: string) => identityProfileStore.restoreDraft(password),
    [],
  );
  const reload = useCallback(() => identityProfileStore.reload(), []);
  const startVerification = useCallback(
    (returnTo?: string | null) =>
      identityProfileStore.startVerification(returnTo),
    [],
  );
  const refreshVerification = useCallback(
    () => identityProfileStore.refreshVerification(),
    [],
  );
  return {
    snapshot,
    save,
    saveUsername,
    saveDraft,
    restoreDraft,
    reload,
    startVerification,
    refreshVerification,
  };
}
