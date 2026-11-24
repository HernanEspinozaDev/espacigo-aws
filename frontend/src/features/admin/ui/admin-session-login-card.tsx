"use client";

import { useState } from "react";
import { KeyRound, Lock, ShieldCheck } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAdmin } from "./use-admin";

export function AdminSessionLoginCard() {
  const {
    snapshot,
    startAdminSession,
    verifyAdminSessionMfa,
    logoutAdminSession,
    clearError,
  } = useAdmin();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mfaCode, setMfaCode] = useState("");

  if (
    snapshot.adminSessionStatus === "unknown" ||
    snapshot.adminSessionStatus === "checking"
  ) {
    return (
      <Card className="max-w-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <ShieldCheck className="size-5 text-primary" aria-hidden="true" />
            <span>Verificando sesión administrativa…</span>
          </CardTitle>
          <CardDescription>
            Comprobando la sesión aislada del personal de administración.
          </CardDescription>
        </CardHeader>
      </Card>
    );
  }

  if (snapshot.adminSessionStatus === "mfa_required") {
    const methods = snapshot.mfaChallenge?.methods ?? ["totp"];
    return (
      <Card className="max-w-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <KeyRound className="size-5 text-primary" aria-hidden="true" />
            <span>Verificación MFA administrativa</span>
          </CardTitle>
          <CardDescription>
            Ingresa tu código TOTP o código de respaldo para completar el acceso
            ({methods.join(", ")}).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={async (e) => {
              e.preventDefault();
              await verifyAdminSessionMfa(mfaCode);
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
              <Label htmlFor="admin-mfa-code">Código de verificación</Label>
              <Input
                id="admin-mfa-code"
                name="code"
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                placeholder="123456"
                value={mfaCode}
                onChange={(e) => {
                  clearError();
                  setMfaCode(e.target.value);
                }}
                required
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="submit"
                disabled={snapshot.actionLoading || !mfaCode.trim()}
              >
                {snapshot.actionLoading ? "Verificando…" : "Verificar MFA"}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={snapshot.actionLoading}
                onClick={() => {
                  setMfaCode("");
                  void logoutAdminSession();
                }}
              >
                Usar otra cuenta
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="max-w-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <Lock className="size-5 text-primary" aria-hidden="true" />
          <span>Acceso exclusivo de Administración</span>
        </CardTitle>
        <CardDescription>
          Inicia sesión con tus credenciales administrativas. Esta sesión está
          completamente aislada de tu cuenta comercial.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            await startAdminSession(email, password);
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
            <Label htmlFor="admin-login-email">Correo administrativo</Label>
            <Input
              id="admin-login-email"
              name="email"
              type="email"
              autoComplete="username"
              placeholder="admin@espacigo.cl"
              value={email}
              onChange={(e) => {
                clearError();
                setEmail(e.target.value);
              }}
              required
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="admin-login-password">Contraseña</Label>
            <Input
              id="admin-login-password"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => {
                clearError();
                setPassword(e.target.value);
              }}
              required
            />
          </div>
          <Button
            type="submit"
            className="w-full"
            disabled={snapshot.actionLoading || !email.trim() || !password}
          >
            {snapshot.actionLoading
              ? "Iniciando sesión…"
              : "Iniciar sesión administrativa"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
