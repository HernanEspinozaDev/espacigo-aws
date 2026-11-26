"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PINPASS_SETTINGS_HREF } from "@/features/account/pinpass-api";

import type { ProfileConfirmationMethod } from "../domain/identity-profile";

const methods = {
  current_password: "Contraseña",
  pin: "Pinpass",
  otp_code: "Autenticador",
} satisfies Record<ProfileConfirmationMethod, string>;

export function ProfileConfirmationDialog({
  purpose,
  pinConfigured,
  otpConfigured,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  purpose: "edit" | "draft" | "reveal";
  pinConfigured: boolean;
  otpConfigured: boolean;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: (method: ProfileConfirmationMethod, value: string) => void;
}) {
  const [method, setMethod] = useState<ProfileConfirmationMethod>(
    purpose === "reveal" ? "pin" : "current_password",
  );
  const [value, setValue] = useState("");
  const password = method === "current_password";
  const choices = Object.entries(methods) as [
    ProfileConfirmationMethod,
    string,
  ][];
  const valid = password ? value.length > 0 : /^\d{6}$/.test(value);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!valid || busy) return;
    onConfirm(method, value);
    setValue("");
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Confirma tu identidad</DialogTitle>
          <DialogDescription>
            {purpose === "reveal"
              ? "Ingresa tu Pinpass para ver este dato."
              : purpose === "draft"
                ? "Elige cómo identificarte para continuar tu perfil."
                : "Elige cómo identificarte para editar tus datos."}
          </DialogDescription>
        </DialogHeader>
        {purpose !== "reveal" && (
          <fieldset className="flex flex-wrap gap-2">
            <legend className="sr-only">Método de confirmación</legend>
            {choices.map(([key, label]) => {
              const available =
                key === "current_password" ||
                (key === "pin" ? pinConfigured : otpConfigured);
              return (
                <Button
                  key={key}
                  type="button"
                  variant={key === method ? "default" : "outline"}
                  aria-pressed={key === method}
                  disabled={busy || !available}
                  onClick={() => {
                    setMethod(key);
                    setValue("");
                  }}
                >
                  {label}
                  {!available && " · Sin configurar"}
                </Button>
              );
            })}
          </fieldset>
        )}
        {purpose === "reveal" && !pinConfigured ? (
          <Link href={PINPASS_SETTINGS_HREF} className={buttonVariants()}>
            Configurar Pinpass
          </Link>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="profile-confirmation-value">
                {password
                  ? "Contraseña actual"
                  : method === "pin"
                    ? "Ingresa tu Pinpass"
                    : "Código del autenticador"}
              </Label>
              <Input
                id="profile-confirmation-value"
                type={method === "otp_code" ? "text" : "password"}
                autoComplete={password ? "current-password" : "one-time-code"}
                inputMode={password ? undefined : "numeric"}
                maxLength={password ? 72 : 6}
                value={value}
                disabled={busy}
                onChange={(event) =>
                  setValue(
                    password
                      ? event.target.value
                      : event.target.value.replace(/\D/g, ""),
                  )
                }
                required
              />
            </div>
            {error && (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={onClose}
                disabled={busy}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={!valid || busy}>
                Confirmar
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
