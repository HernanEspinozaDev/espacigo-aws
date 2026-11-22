"use client";
import { useEffect, useState, type ReactNode } from "react";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import { RenterAgreementPanel } from "./renter-agreement-panel";
export function ContractCheckoutGate({
  quoteId,
  checkoutApiEnabled,
  walletPurchaseEnabled,
  children,
}: {
  quoteId: string;
  checkoutApiEnabled: boolean;
  walletPurchaseEnabled: boolean;
  children: ReactNode;
}) {
  const [mode, setMode] = useState<"loading" | "manual" | "legacy" | "error">(
    "loading",
  );
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    commercialAgreements
      .capabilities(controller.signal)
      .then((enabled) => {
        if (!controller.signal.aborted) setMode(enabled ? "manual" : "legacy");
      })
      .catch(() => {
        if (!controller.signal.aborted) setMode("error");
      });
    return () => controller.abort();
  }, [attempt]);
  if (mode === "loading")
    return (
      <p role="status" className="text-sm">
        Comprobando los pasos del arriendo…
      </p>
    );
  if (mode === "error")
    return (
      <div role="alert" className="space-y-2 text-sm">
        <p>
          No pudimos comprobar los requisitos del contrato. El pago permanece
          pendiente.
        </p>
        <button
          type="button"
          className="min-h-11 underline underline-offset-4"
          onClick={() => {
            setMode("loading");
            setAttempt((v) => v + 1);
          }}
        >
          Volver a comprobar
        </button>
      </div>
    );
  if (mode === "legacy") return children;
  return (
    <RenterAgreementPanel
      quoteId={quoteId}
      checkoutApiEnabled={checkoutApiEnabled}
      walletPurchaseEnabled={walletPurchaseEnabled}
    />
  );
}
