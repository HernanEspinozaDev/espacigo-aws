"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { approveAccountQRTicket } from "@/features/account/qr-verification-api";

export default function ConfirmarVinculacionPage() {
  const token = useRef("");
  const [pending, setPending] = useState(false);
  const [approved, setApproved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!window.location.hash) return;
    token.current = window.location.hash.slice(1);
    window.history.replaceState(
      window.history.state,
      "",
      window.location.pathname,
    );
  }, []);

  async function approve() {
    if (!/^[A-Za-z0-9_-]{43}$/.test(token.current)) {
      setError("Escanea un nuevo código desde Seguridad e inténtalo de nuevo.");
      return;
    }
    setPending(true);
    setError(null);
    try {
      await approveAccountQRTicket({ token: token.current });
      token.current = "";
      setApproved(true);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "No pudimos vincular el dispositivo.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Confirmar vinculación"
        description="Confirma solo si acabas de generar el código desde Seguridad en tu otro dispositivo."
        overview
      />
      <section className="space-y-4 rounded-2xl border bg-card p-5 sm:p-6">
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {approved ? (
          <p role="status" className="text-sm">
            Dispositivo vinculado.
          </p>
        ) : (
          <Button
            type="button"
            onClick={() => void approve()}
            disabled={pending}
          >
            {pending ? "Confirmando…" : "Vincular este dispositivo"}
          </Button>
        )}
      </section>
    </div>
  );
}
