"use client";

import Link from "next/link";
import Image from "next/image";
import { useRouter } from "next/navigation";
import { Building2, ChevronDown, FileText, LogOut, User } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { useAccount } from "@/features/account/ui/use-account";
import { NotificationBell } from "@/features/notifications/ui/notification-bell";
import { useAuth } from "./use-auth";
import { useSessionIdentity } from "./use-session-identity";
import { useIdentityProfile } from "@/features/identity-profile/ui/use-identity-profile";

export interface SessionControlsProps {
  variant: "workspace" | "public";
  anonymousOnly?: boolean;
  onNavigate?: () => void;
}

export function SessionControls({
  variant,
  anonymousOnly = false,
  onNavigate,
}: SessionControlsProps) {
  const router = useRouter();
  const { status, accountId } = useSessionIdentity();
  const { snapshot: identityProfile } = useIdentityProfile();
  const { snapshot: accountSnapshot, loadProfilePhoto } = useAccount();
  const { logout } = useAuth();
  const [loggingOut, setLoggingOut] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!anonymousOnly && status === "authenticated" && accountId) {
      void loadProfilePhoto();
    }
  }, [accountId, anonymousOnly, loadProfilePhoto, status]);

  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuOpen]);

  if (anonymousOnly && status !== "anonymous") return null;

  const handleLogout = async () => {
    if (loggingOut) return;
    setMenuOpen(false);
    setLoggingOut(true);
    try {
      await logout();
    } catch {
      // useAuth.logout handles local clear and sets pending without false remote success
    } finally {
      setLoggingOut(false);
      router.replace("/");
    }
  };

  if (status === "checking") {
    if (variant === "workspace") {
      return (
        <div
          data-testid="session-controls-checking"
          className="inline-flex h-9 w-20 items-center justify-center rounded-full border border-border bg-secondary/60 text-xs font-medium text-muted-foreground opacity-75"
          aria-busy="true"
        >
          Acceso
        </div>
      );
    }
    return (
      <Link
        href="/acceso"
        data-testid="session-controls-checking"
        onClick={onNavigate}
        aria-busy="true"
        className="inline-flex min-h-11 items-center rounded-full border border-border bg-background px-4 text-sm font-medium text-foreground/80 transition-colors hover:bg-secondary"
      >
        Iniciar sesión
      </Link>
    );
  }

  if (status === "authenticated") {
    const ownIdentity =
      identityProfile.accountId === accountId ? identityProfile : null;
    const displayIdentity =
      ownIdentity?.summary?.username ??
      (ownIdentity?.loading || !ownIdentity
        ? "Cargando perfil…"
        : "Completa tu perfil");
    const profilePhoto = accountSnapshot.photoUrl;
    const avatar = (
      <span
        aria-hidden="true"
        className="relative inline-flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted text-xs font-semibold text-muted-foreground"
      >
        {profilePhoto ? (
          <Image
            src={profilePhoto}
            alt=""
            fill
            sizes="28px"
            unoptimized
            className="object-cover"
          />
        ) : (
          (Array.from(displayIdentity.trim())[0]?.toLocaleUpperCase("es") ??
          "?")
        )}
      </span>
    );

    return (
      <div className="flex shrink-0 items-center gap-3">
        <NotificationBell />
        <div ref={menuRef} className="relative">
          <button
            type="button"
            aria-expanded={menuOpen}
            aria-haspopup="menu"
            onClick={() => setMenuOpen((prev) => !prev)}
            className="inline-flex min-h-10 items-center gap-2 rounded-full border border-border bg-white px-2.5 py-1.5 text-sm font-medium text-foreground shadow-xs transition-colors hover:bg-secondary"
          >
            {avatar}
            <span
              className="sr-only sm:not-sr-only sm:inline-block sm:max-w-[140px] sm:truncate"
              title={displayIdentity || undefined}
            >
              {displayIdentity || "Menú de cuenta"}
            </span>
            <ChevronDown
              className="size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          </button>

          {menuOpen && (
            <div
              role="menu"
              aria-label="Menú de cuenta"
              className="absolute right-0 top-full z-50 mt-2 w-60 rounded-xl border border-border bg-white p-1.5 shadow-lg"
            >
              <div className="border-b border-border/60 px-3 py-2">
                <p className="truncate text-xs font-medium text-muted-foreground">
                  Sesión iniciada
                </p>
                <p className="truncate text-sm font-semibold text-foreground">
                  {displayIdentity}
                </p>
              </div>
              <div className="py-1">
                <Link
                  role="menuitem"
                  href="/mi-cuenta"
                  onClick={() => {
                    setMenuOpen(false);
                    onNavigate?.();
                  }}
                  className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-secondary"
                >
                  <User
                    className="size-4 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span>Mi cuenta</span>
                </Link>
                <Link
                  role="menuitem"
                  href="/reservas/contratos"
                  onClick={() => {
                    setMenuOpen(false);
                    onNavigate?.();
                  }}
                  className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-secondary"
                >
                  <FileText
                    className="size-4 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span>Mis contratos</span>
                </Link>
                <Link
                  role="menuitem"
                  href="/anfitrion/espacios"
                  onClick={() => {
                    setMenuOpen(false);
                    onNavigate?.();
                  }}
                  className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-secondary"
                >
                  <Building2
                    className="size-4 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span>Mis espacios</span>
                </Link>
              </div>
              <div className="border-t border-border/60 pt-1">
                <button
                  type="button"
                  role="menuitem"
                  disabled={loggingOut}
                  onClick={() => void handleLogout()}
                  className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm font-medium text-destructive transition-colors hover:bg-destructive/10 disabled:opacity-50"
                >
                  <LogOut className="size-4" aria-hidden="true" />
                  <span>
                    {loggingOut ? "Cerrando sesión…" : "Cerrar sesión"}
                  </span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  // status === "anonymous" (guest)
  if (variant === "workspace") {
    return (
      <Button asChild className="rounded-full">
        <Link href="/acceso">Acceso</Link>
      </Button>
    );
  }

  return (
    <Link
      href="/acceso"
      onClick={onNavigate}
      className="inline-flex min-h-11 items-center rounded-full bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-[var(--brand-primary-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      Iniciar sesión
    </Link>
  );
}
