"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { SecurityQrCode } from "@/features/account/ui/security-qr-code";
import { useQRVerification } from "@/features/account/ui/use-qr-verification";

export default function CodigoQRPage() {
  const {
    status,
    activeTicket,
    ticketStatus,
    isLoading,
    isGeneratingTicket,
    isDisabling,
    error,
    notification,
    refresh,
    generateTicket,
    disableQR,
  } = useQRVerification();
  const [pairingLink, setPairingLink] = useState("");
  const [confirmDisable, setConfirmDisable] = useState(false);

  async function beginPairing() {
    const ticket = await generateTicket();
    if (!ticket) return;
    const url = new URL(
      "/mi-cuenta/seguridad/metodos/codigo-qr/confirmar",
      window.location.origin,
    );
    url.hash = ticket.qr_payload;
    setPairingLink(url.href);
  }

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Código QR"
        description="Vincula un dispositivo desde Seguridad."
        overview
      />
      <section
        className="space-y-4 rounded-2xl border bg-card p-5 sm:p-6"
        aria-busy={isLoading}
      >
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {notification && (
          <p role="status" className="text-sm">
            {notification}
          </p>
        )}
        {isLoading ? (
          <p role="status" className="text-sm">
            Consultando la vinculación…
          </p>
        ) : !status ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => void refresh()}
          >
            Reintentar
          </Button>
        ) : status.enabled ? (
          <>
            <h2 className="font-semibold">Dispositivo vinculado</h2>
            {status.trusted_device_label && (
              <p className="text-sm text-muted-foreground">
                {status.trusted_device_label}
              </p>
            )}
            {confirmDisable ? (
              <div className="flex flex-wrap gap-3">
                <Button
                  type="button"
                  variant="destructive"
                  disabled={isDisabling}
                  onClick={async () => {
                    if (await disableQR()) setConfirmDisable(false);
                  }}
                >
                  Confirmar desvinculación
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={isDisabling}
                  onClick={() => setConfirmDisable(false)}
                >
                  Cancelar
                </Button>
              </div>
            ) : (
              <Button
                type="button"
                variant="outline"
                onClick={() => setConfirmDisable(true)}
              >
                Desvincular dispositivo
              </Button>
            )}
          </>
        ) : activeTicket &&
          pairingLink &&
          ticketStatus?.status === "pending" ? (
          <>
            <h2 className="font-semibold">Escanea con tu celular</h2>
            <p className="text-sm text-muted-foreground">
              Abre el enlace con la misma cuenta de EspaciGo iniciada en el
              celular y confirma la vinculación. El código dura dos minutos.
            </p>
            <SecurityQrCode
              payload={pairingLink}
              label="Código QR para vincular dispositivo"
            />
            <p role="status" className="text-sm">
              Esperando confirmación…
            </p>
          </>
        ) : (
          <>
            <h2 className="font-semibold">Vincular dispositivo</h2>
            <p className="text-sm text-muted-foreground">
              Antes de continuar, inicia sesión con tu cuenta de EspaciGo en el
              celular que vas a vincular.
            </p>
            <Button
              type="button"
              onClick={() => void beginPairing()}
              disabled={isGeneratingTicket}
            >
              {isGeneratingTicket ? "Generando…" : "Generar código QR"}
            </Button>
          </>
        )}
      </section>
    </div>
  );
}
