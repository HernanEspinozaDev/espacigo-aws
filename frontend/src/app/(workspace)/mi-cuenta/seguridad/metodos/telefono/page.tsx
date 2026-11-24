"use client";

import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { PhoneChannel } from "@/features/account/phone-verification-api";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { usePhoneVerification } from "@/features/account/ui/use-phone-verification";

const channelLabels = { sms: "SMS", whatsapp: "WhatsApp" } satisfies Record<
  PhoneChannel,
  string
>;

export default function TelefonoMetodoPage() {
  const {
    status,
    isLoading,
    isSending,
    isVerifying,
    error,
    clearError,
    success,
    cooldownSeconds,
    refresh,
    sendCode,
    verifyCode,
  } = usePhoneVerification();
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [channel, setChannel] = useState<PhoneChannel>("sms");
  const [editPendingPhone, setEditPendingPhone] = useState(false);
  const channels = status?.available_channels ?? [];
  const selectedChannel = channels.includes(channel) ? channel : channels[0];
  const awaitingCode = status?.pending_verification && !editPendingPhone;
  const busy = isSending || isVerifying;

  async function send(target: string) {
    if (!selectedChannel || busy || cooldownSeconds > 0) return;
    if (await sendCode(target, selectedChannel)) {
      setPhone("");
      setCode("");
      setEditPendingPhone(false);
    }
  }

  function submitPhone(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void send(phone.trim());
  }

  function submitCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void verifyCode(code);
  }

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Teléfono"
        description="Confirma que tienes acceso a tu número móvil chileno."
        overview
      />

      <section
        className="space-y-5 rounded-2xl border bg-card p-5 sm:p-6"
        aria-busy={isLoading}
      >
        {error && (
          <p
            role="alert"
            className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
          >
            {error}
          </p>
        )}
        {success && !error && (
          <p role="status" className="rounded-lg bg-primary/5 p-3 text-sm">
            {success}
          </p>
        )}

        {isLoading ? (
          <p role="status" className="text-sm">
            Consultando tu teléfono…
          </p>
        ) : !status ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => void refresh()}
          >
            Reintentar
          </Button>
        ) : status.verified ? (
          <div className="space-y-1">
            <h2 className="font-semibold">Teléfono verificado</h2>
            <p className="text-sm text-muted-foreground">
              {status.masked_phone}
            </p>
          </div>
        ) : (
          <>
            {awaitingCode && (
              <form className="max-w-sm space-y-3" onSubmit={submitCode}>
                <h2 className="font-semibold">
                  Ingresa el código de verificación
                </h2>
                <p className="text-sm text-muted-foreground">
                  Número de destino {status.masked_phone}
                </p>
                <Label htmlFor="phone-code">Código de seis dígitos</Label>
                <Input
                  id="phone-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={code}
                  onChange={(event) =>
                    setCode(event.target.value.replace(/\D/g, ""))
                  }
                  required
                />
                <Button type="submit" disabled={busy || code.length !== 6}>
                  {isVerifying ? "Verificando…" : "Verificar teléfono"}
                </Button>
              </form>
            )}

            {selectedChannel ? (
              <form
                className="max-w-sm space-y-4"
                onSubmit={
                  awaitingCode
                    ? (event) => {
                        event.preventDefault();
                        void send("");
                      }
                    : submitPhone
                }
              >
                {!awaitingCode && (
                  <div className="space-y-2">
                    <Label htmlFor="mobile-phone">Número de celular</Label>
                    <Input
                      id="mobile-phone"
                      type="tel"
                      autoComplete="tel"
                      placeholder="+56 9 1234 5678"
                      value={phone}
                      onChange={(event) => setPhone(event.target.value)}
                      required
                    />
                  </div>
                )}
                {channels.length > 1 ? (
                  <fieldset className="space-y-2" disabled={busy}>
                    <legend className="text-sm font-medium">
                      Recibir código por
                    </legend>
                    <div className="flex flex-wrap gap-3">
                      {channels.map((option) => (
                        <label
                          key={option}
                          className="inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 text-sm"
                        >
                          <input
                            type="radio"
                            name="phone-channel"
                            value={option}
                            checked={option === selectedChannel}
                            onChange={() => setChannel(option)}
                            className="accent-primary"
                          />
                          {channelLabels[option]}
                        </label>
                      ))}
                    </div>
                  </fieldset>
                ) : null}
                <div className="flex flex-wrap items-center gap-3">
                  <Button
                    type="submit"
                    variant={awaitingCode ? "outline" : "default"}
                    disabled={
                      busy ||
                      cooldownSeconds > 0 ||
                      (!awaitingCode && !phone.trim())
                    }
                  >
                    {isSending
                      ? "Enviando…"
                      : `${awaitingCode ? "Reenviar" : "Enviar"} por ${channelLabels[selectedChannel]}`}
                  </Button>
                  {awaitingCode && (
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => {
                        clearError();
                        setEditPendingPhone(true);
                      }}
                    >
                      Corregir número
                    </Button>
                  )}
                </div>
                {cooldownSeconds > 0 && (
                  <p className="text-sm text-muted-foreground">
                    Podrás pedir otro código en {cooldownSeconds} segundos.
                  </p>
                )}
              </form>
            ) : (
              <p className="text-sm text-muted-foreground">
                El envío de códigos no está disponible en este momento.
              </p>
            )}
          </>
        )}
      </section>
    </div>
  );
}
