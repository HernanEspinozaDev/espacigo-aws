"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Clock, LogOut, Menu, Shield, ShieldAlert, ShieldCheck, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import { AdminAssistedViewBanner } from "./admin-assisted-view-banner";
import { AdminMfaStepUpModal } from "./admin-mfa-step-up-modal";
import { AdminNavigation, formatAdminRole } from "./admin-navigation";
import { AdminSessionLoginCard } from "./admin-session-login-card";
import { useAdmin } from "./use-admin";

const timeFormatter = new Intl.DateTimeFormat("es-CL", {
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "America/Santiago",
});

function formatSessionTime(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return null;
  return timeFormatter.format(parsed);
}

export function AdminWorkspaceShell({ children }: { children: ReactNode }) {
  const { snapshot, activeRole, logoutAdminSession } = useAdmin();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMobileNavOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [mobileNavOpen]);

  if (snapshot.adminSessionStatus !== "authenticated") {
    return <AdminSessionLoginCard />;
  }

  const mfaPending =
    snapshot.mfaStepUpRequired || !snapshot.adminSession?.mfa_verified_at;
  const expiresLabel = formatSessionTime(snapshot.adminSession?.expires_at);
  const recentAuthLabel = formatSessionTime(
    snapshot.adminSession?.recent_auth_expires_at,
  );

  return (
    <div className="space-y-6 min-w-0">
      <header className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-card px-3 py-2.5 sm:px-4 shadow-xs min-w-0">
        <div className="flex flex-wrap items-center gap-2 min-w-0">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="md:hidden min-h-9"
            aria-expanded={mobileNavOpen}
            aria-controls="admin-mobile-drawer"
            onClick={() => setMobileNavOpen((prev) => !prev)}
          >
            <Menu className="size-4 mr-1.5" aria-hidden="true" />
            <span>Menú</span>
          </Button>

          <Badge
            variant="secondary"
            className="gap-1.5 px-2.5 py-1 text-xs font-semibold tracking-wide uppercase shadow-none border border-slate-300 dark:border-slate-700"
          >
            <Shield className="size-3.5 text-primary" aria-hidden="true" />
            <span>{formatAdminRole(activeRole)}</span>
          </Badge>

          <Badge
            variant={mfaPending ? "outline" : "secondary"}
            className={
              mfaPending
                ? "gap-1.5 px-2.5 py-1 text-xs border-amber-400 bg-amber-50 text-amber-900 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-200"
                : "gap-1.5 px-2.5 py-1 text-xs border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200"
            }
          >
            {mfaPending ? (
              <ShieldAlert className="size-3.5" aria-hidden="true" />
            ) : (
              <ShieldCheck className="size-3.5" aria-hidden="true" />
            )}
            <span>{mfaPending ? "MFA pendiente" : "MFA verificado"}</span>
          </Badge>
        </div>

        <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground min-w-0">
          {(expiresLabel || recentAuthLabel) && (
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              {expiresLabel && (
                <span className="inline-flex items-center gap-1 tabular-nums">
                  <Clock className="size-3.5" aria-hidden="true" />
                  Sesión hasta {expiresLabel}
                </span>
              )}
              {recentAuthLabel && (
                <span className="inline-flex items-center gap-1 tabular-nums">
                  Auth reciente hasta {recentAuthLabel}
                </span>
              )}
            </div>
          )}

          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => void logoutAdminSession()}
            disabled={snapshot.actionLoading}
            className="h-8 px-2.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <LogOut className="size-3.5 mr-1" aria-hidden="true" />
            Salir
            <span className="sr-only"> de la sesión administrativa</span>
          </Button>
        </div>
      </header>

      <AdminAssistedViewBanner />

      {mobileNavOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div
            className="fixed inset-0 bg-black/40"
            aria-hidden="true"
            onClick={() => setMobileNavOpen(false)}
          />
          <div
            id="admin-mobile-drawer"
            role="dialog"
            aria-modal="true"
            aria-label="Navegación de administración"
            className="relative z-10 flex h-full w-72 max-w-[85vw] flex-col gap-4 overflow-y-auto border-r bg-background p-4 shadow-xl"
          >
            <div className="flex items-center justify-between border-b pb-3">
              <span className="text-sm font-semibold">Administración</span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setMobileNavOpen(false)}
              >
                <X className="size-4 mr-1" aria-hidden="true" />
                Cerrar
              </Button>
            </div>
            <AdminNavigation
              showSessionCard={false}
              onNavigate={() => setMobileNavOpen(false)}
            />
          </div>
        </div>
      )}

      <div className="flex flex-col md:flex-row gap-6 lg:gap-8 items-start min-w-0">
        <div className="hidden md:block w-64 shrink-0 border-r border-border/60 pr-6">
          <AdminNavigation showSessionCard={false} />
        </div>
        <section
          aria-label="Área operativa"
          className="flex-1 w-full min-w-0 space-y-6"
        >
          {children}
        </section>
      </div>

      <AdminMfaStepUpModal />
    </div>
  );
}
