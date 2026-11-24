"use client";

import { useEffect, useState } from "react";
import { Eye, EyeOff, ShieldAlert } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiError } from "@/lib/api/errors";
import { isValidAdminReason } from "../domain/admin-rules";
import type {
  AdminSensitiveAccessResult,
  AdminSensitiveAccessScope,
} from "../domain/types";
import { useAdmin } from "./use-admin";

const timeFormatter = new Intl.DateTimeFormat("es-CL", {
  dateStyle: "short",
  timeStyle: "medium",
  timeZone: "America/Santiago",
});

export function AdminSensitiveEmailControl({
  accountId,
  maskedEmail,
  defaultScope = "account_email",
}: {
  accountId: string;
  maskedEmail: string;
  defaultScope?: AdminSensitiveAccessScope;
}) {
  const { requestGlobalAccountSensitiveAccess } = useAdmin();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [caseId, setCaseId] = useState("");
  const [scope, setScope] = useState<AdminSensitiveAccessScope>(defaultScope);
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<AdminSensitiveAccessResult | null>(
    null,
  );

  useEffect(() => {
    if (!revealed?.expires_at) return;
    const expiresMs = Date.parse(revealed.expires_at);
    if (Number.isNaN(expiresMs)) return;
    const remainingMs = Math.max(0, expiresMs - Date.now());
    const timer = window.setTimeout(() => {
      setRevealed(null);
    }, remainingMs);
    return () => window.clearTimeout(timer);
  }, [revealed]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isValidAdminReason(reason)) {
      setLocalError(
        "El motivo es obligatorio y debe tener al menos 5 caracteres.",
      );
      return;
    }
    setSubmitting(true);
    setLocalError(null);
    try {
      const result = await requestGlobalAccountSensitiveAccess(accountId, {
        field: "email",
        reason: reason.trim(),
        case_id: caseId.trim() || undefined,
        scope,
      });
      if (result) {
        setRevealed(result);
        setOpen(false);
      }
    } catch (err) {
      if (
        err instanceof ApiError &&
        (err.status === 428 || err.code === "recent_authentication_required")
      ) {
        setLocalError(
          "Se requiere verificación MFA reciente. Completa el código en la ventana emergente y reintenta.",
        );
      } else {
        setLocalError("No pudimos revelar el correo en este momento.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  if (revealed) {
    return (
      <span className="inline-flex flex-wrap items-center gap-2">
        <span className="font-mono font-semibold text-foreground">
          {revealed.value}
        </span>
        <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-sans text-amber-900 dark:bg-amber-950/60 dark:text-amber-200">
          Expira: {timeFormatter.format(new Date(revealed.expires_at))}
        </span>
        <button
          type="button"
          onClick={() => setRevealed(null)}
          className="inline-flex items-center gap-1 font-sans text-xs font-medium text-muted-foreground underline underline-offset-2 hover:text-foreground"
        >
          <EyeOff className="size-3.5" aria-hidden="true" />
          Ocultar
        </button>
      </span>
    );
  }

  return (
    <>
      <span className="inline-flex flex-wrap items-center gap-2">
        <span>{maskedEmail}</span>
        <button
          type="button"
          onClick={() => {
            setLocalError(null);
            setOpen(true);
          }}
          className="inline-flex items-center gap-1 font-sans text-xs font-medium text-primary underline underline-offset-2 hover:text-primary/80"
        >
          <Eye className="size-3.5" aria-hidden="true" />
          Revelar correo
        </button>
      </span>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ShieldAlert
                className="size-5 text-amber-600"
                aria-hidden="true"
              />
              <span>Acceso auditado a correo</span>
            </DialogTitle>
            <DialogDescription>
              Esta consulta quedará registrada en auditoría con acceso acotado
              en el tiempo.
            </DialogDescription>
          </DialogHeader>

          <form className="space-y-4" onSubmit={handleSubmit}>
            {localError && (
              <p
                role="alert"
                className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
              >
                {localError}
              </p>
            )}
            <div className="space-y-1.5">
              <Label htmlFor={`sensitive-reason-${accountId}`}>
                Motivo (obligatorio, mínimo 5 caracteres)
              </Label>
              <textarea
                id={`sensitive-reason-${accountId}`}
                className="flex min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Ej: Verificación de titularidad solicitada en ticket de soporte"
                rows={3}
                required
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor={`sensitive-case-${accountId}`}>
                Caso / Ticket (opcional)
              </Label>
              <Input
                id={`sensitive-case-${accountId}`}
                value={caseId}
                onChange={(e) => setCaseId(e.target.value)}
                placeholder="Ej: SUP-2026-1042"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor={`sensitive-scope-${accountId}`}>Alcance</Label>
              <Select
                value={scope}
                onValueChange={(value) =>
                  setScope(value as AdminSensitiveAccessScope)
                }
              >
                <SelectTrigger id={`sensitive-scope-${accountId}`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="account_email">
                    Correo de cuenta (account_email)
                  </SelectItem>
                  <SelectItem value="account_summary">
                    Resumen de cuenta (account_summary)
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting ? "Revelando…" : "Confirmar y revelar"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
