"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { mercadoPagoFacade } from "../mercado-pago-runtime";

export function useMercadoPago() {
  const { snapshot: auth } = useAuth();
  const snapshot = useSyncExternalStore(
    mercadoPagoFacade.subscribe,
    mercadoPagoFacade.getSnapshot,
    mercadoPagoFacade.getServerSnapshot,
  );

  useEffect(() => {
    if (auth.status !== "authenticated") {
      mercadoPagoFacade.clear();
      return;
    }
    if (!auth.roles.includes("arrendador")) {
      mercadoPagoFacade.clear();
      return;
    }
    void mercadoPagoFacade.load();
    return () => mercadoPagoFacade.clear();
  }, [auth.status, auth.accountId, auth.roles]);

  const startAuthorization = useCallback(
    () => mercadoPagoFacade.startAuthorization(),
    [],
  );
  const reload = useCallback(() => mercadoPagoFacade.load(), []);
  return { auth, snapshot, startAuthorization, reload };
}
