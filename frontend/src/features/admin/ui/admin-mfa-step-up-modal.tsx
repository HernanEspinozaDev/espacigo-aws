"use client";

import { useState } from "react";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAdmin } from "./use-admin";

export function AdminMfaStepUpModal() {
  const { snapshot, verifyAdminSessionMfa, closeMfaStepUp, clearError } =
    useAdmin();
  const [code, setCode] = useState("");

  const open =
    snapshot.adminSessionStatus === "authenticated" &&
    snapshot.mfaStepUpRequired;

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setCode("");
          closeMfaStepUp();
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="size-5 text-amber-600" aria-hidden="true" />
            <span>Confirmar autenticación reciente (MFA)</span>
          </DialogTitle>
          <DialogDescription>
            Esta operación sensible requiere verificar nuevamente tu código TOTP
            o de respaldo (HTTP 428) para continuar sin perder el contexto.
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            const ok = await verifyAdminSessionMfa(code);
            if (ok) {
              setCode("");
            }
          }}
        >
          {snapshot.error && (
            <p
              role="alert"
              className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-xs text-red-900 dark:border-red-800 dark:bg-red-950/50 dark:text-red-200"
            >
              {snapshot.error}
            </p>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="admin-stepup-code">Código TOTP o de respaldo</Label>
            <Input
              id="admin-stepup-code"
              name="code"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="123456"
              value={code}
              onChange={(e) => {
                clearError();
                setCode(e.target.value);
              }}
              required
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setCode("");
                closeMfaStepUp();
              }}
            >
              Cancelar
            </Button>
            <Button
              type="submit"
              disabled={snapshot.actionLoading || !code.trim()}
            >
              {snapshot.actionLoading ? "Validando…" : "Confirmar código"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
