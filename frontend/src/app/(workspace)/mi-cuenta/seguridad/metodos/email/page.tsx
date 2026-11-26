"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  confirmAccountEmailChange,
  securityConfirmationError,
  startAccountEmailChange,
} from "@/features/account/security-confirmation-api";
import { Button } from "@/components/ui/button";
import { SecurityMethodHeader } from "@/features/account/ui/security-method-header";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";

export default function EmailMetodoPage() {
  const { summary, isLoading, error, refresh } = useSecurityMethodsSummary();
  const email = summary?.methods.email;
  const [editing, setEditing] = useState(false);
  const [address, setAddress] = useState("");
  const [password, setPassword] = useState("");
  const [authenticator, setAuthenticator] = useState("");
  const [code, setCode] = useState("");
  const [destination, setDestination] = useState("");
  const [message, setMessage] = useState("");
  const [failure, setFailure] = useState("");
  const [busy, setBusy] = useState(false);

  function cancel() {
    setEditing(false);
    setAddress("");
    setPassword("");
    setAuthenticator("");
    setCode("");
    setDestination("");
    setFailure("");
  }

  async function handleChange(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setFailure("");
    try {
      if (destination) {
        await confirmAccountEmailChange(code);
        cancel();
        setMessage("Correo actualizado.");
        void refresh();
      } else {
        const result = await startAccountEmailChange(
          address,
          password,
          authenticator,
        );
        if (result) setDestination(result.masked_email);
        setPassword("");
        setAuthenticator("");
      }
    } catch (cause) {
      setFailure(securityConfirmationError(cause));
      if (destination) setCode("");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <SecurityMethodHeader
        title="Correo electrónico"
        description="Tu correo principal recibe avisos de seguridad y permite recuperar el acceso a tu cuenta."
        overview
      />
      <section
        className="space-y-3 rounded-2xl border bg-card p-5"
        aria-busy={isLoading}
      >
        <h2 className="font-semibold">Correo principal</h2>
        {isLoading ? (
          <p role="status" className="text-sm text-muted-foreground">
            Consultando el estado…
          </p>
        ) : error || !email ? (
          <>
            <p role="alert" className="text-sm">
              No pudimos consultar el estado de tu correo.
            </p>
            <Button
              type="button"
              variant="outline"
              onClick={() => void refresh()}
            >
              Reintentar
            </Button>
          </>
        ) : email.enabled ? (
          <>
            <p className="break-all text-sm">{email.masked_email}</p>
            <p className="text-sm font-medium text-emerald-800 dark:text-emerald-300">
              Verificado
            </p>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            Sin correo verificado.
          </p>
        )}
        {message && (
          <p
            role="status"
            className="text-sm text-emerald-800 dark:text-emerald-300"
          >
            {message}
          </p>
        )}
        {email?.enabled && !editing && (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setMessage("");
              setEditing(true);
            }}
          >
            Cambiar correo
          </Button>
        )}
        {editing && (
          <form onSubmit={handleChange} className="max-w-md space-y-4">
            {destination ? (
              <div className="space-y-2">
                <Label htmlFor="new-email-code">Código del nuevo correo</Label>
                <p className="text-sm text-muted-foreground">
                  Enviamos el código a {destination}.
                </p>
                <Input
                  id="new-email-code"
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  required
                />
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <Label htmlFor="new-account-email">
                    Nuevo correo electrónico
                  </Label>
                  <Input
                    id="new-account-email"
                    type="email"
                    autoComplete="email"
                    maxLength={254}
                    value={address}
                    onChange={(e) => setAddress(e.target.value)}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="email-change-password">
                    Contraseña actual
                  </Label>
                  <Input
                    id="email-change-password"
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="email-change-authenticator">
                    Código del autenticador
                  </Label>
                  <Input
                    id="email-change-authenticator"
                    inputMode="numeric"
                    maxLength={6}
                    value={authenticator}
                    onChange={(e) =>
                      setAuthenticator(e.target.value.replace(/\D/g, ""))
                    }
                    disabled={!summary?.methods.totp.enabled}
                    required
                  />
                  {!summary?.methods.totp.enabled && (
                    <Link
                      href="/mi-cuenta/seguridad/metodos/google-authenticator"
                      className="text-sm text-primary underline"
                    >
                      Configurar autenticador
                    </Link>
                  )}
                </div>
              </>
            )}
            {failure && (
              <p role="alert" className="text-sm text-destructive">
                {failure}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={cancel}
              >
                Cancelar
              </Button>
              <Button
                type="submit"
                disabled={
                  busy ||
                  (destination
                    ? !/^\d{6}$/.test(code)
                    : !address || !password || !/^\d{6}$/.test(authenticator))
                }
              >
                {destination
                  ? "Confirmar cambio"
                  : "Enviar código al nuevo correo"}
              </Button>
            </div>
          </form>
        )}
      </section>
    </div>
  );
}
