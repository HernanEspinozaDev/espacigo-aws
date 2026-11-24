"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BarChart3,
  Building2,
  CheckSquare,
  FileText,
  Landmark,
  Lock,
  LogOut,
  Scale,
  Shield,
  UserCheck,
  Users,
} from "lucide-react";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import { cn } from "@/lib/utils";
import { useAdmin } from "./use-admin";
import type { AdminSection } from "../domain/types";

interface AdminNavItem {
  label: string;
  href: string;
  section: AdminSection;
  icon: typeof BarChart3;
  description: string;
  exact: boolean;
}

const NAV_ITEMS: readonly AdminNavItem[] = [
  {
    label: "Resumen / KPIs",
    href: "/administracion",
    section: "overview",
    icon: BarChart3,
    description: "Métricas y estado general",
    exact: true,
  },
  {
    label: "Verificaciones KYC",
    href: "/administracion/verificaciones",
    section: "verifications",
    icon: UserCheck,
    description: "Validación de identidad",
    exact: false,
  },
  {
    label: "Espacios",
    href: "/administracion/espacios",
    section: "spaces",
    icon: Building2,
    description: "Moderación de catálogo",
    exact: false,
  },
  {
    label: "Disputas y Reclamos",
    href: "/administracion/disputas",
    section: "disputes",
    icon: Scale,
    description: "Gestión de mediación",
    exact: false,
  },
  {
    label: "Cuentas y Roles",
    href: "/administracion/cuentas",
    section: "accounts",
    icon: Users,
    description: "Control de accesos RBAC",
    exact: false,
  },
  {
    label: "Auditoría y Reportes",
    href: "/administracion/auditoria",
    section: "audit",
    icon: FileText,
    description: "Eventos y exportación",
    exact: false,
  },
  {
    label: "Finanzas y Reservas",
    href: "/administracion/finanzas",
    section: "finance",
    icon: Landmark,
    description: "Conciliación, reservas y contratos",
    exact: false,
  },
  {
    label: "Privacidad y KYC",
    href: "/administracion/privacidad",
    section: "privacy",
    icon: Lock,
    description: "ARCO+ (Ley 21.719) y KYC",
    exact: false,
  },
  {
    label: "Aprobaciones y Soporte",
    href: "/administracion/aprobaciones",
    section: "approvals",
    icon: CheckSquare,
    description: "Doble control y tickets",
    exact: false,
  },
];

export function formatAdminRole(role: string | null): string {
  switch (role) {
    case "administrador_global":
      return "Administrador Global";
    case "administrador":
      return "Administrador";
    case "arrendador":
      return "Arrendador";
    case "arrendatario":
      return "Arrendatario";
    default:
      return "Sesión Restringida";
  }
}

export function AdminNavigation({
  onNavigate,
  showSessionCard = true,
  className,
}: {
  onNavigate?: () => void;
  showSessionCard?: boolean;
  className?: string;
}) {
  const pathname = usePathname();
  const { snapshot, canAccess, activeRole, logoutAdminSession } = useAdmin();

  const accessibleItems = NAV_ITEMS.filter((item) => canAccess(item.section));
  const isRoot = pathname === "/administracion";
  const currentItem = accessibleItems.find((item) =>
    item.exact ? pathname === item.href : pathname.startsWith(item.href),
  );

  return (
    <aside
      aria-label="Navegación de administración"
      className={cn("w-full space-y-4 min-w-0", className)}
    >
      <div className="hidden md:block">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink href="/">Inicio</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              {isRoot ? (
                <BreadcrumbPage>Administración</BreadcrumbPage>
              ) : (
                <BreadcrumbLink href="/administracion">
                  Administración
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
            {!isRoot && currentItem && (
              <>
                <BreadcrumbSeparator />
                <BreadcrumbItem>
                  <BreadcrumbPage>{currentItem.label}</BreadcrumbPage>
                </BreadcrumbItem>
              </>
            )}
          </BreadcrumbList>
        </Breadcrumb>
      </div>

      {showSessionCard && (
        <div className="flex items-center justify-between gap-2 p-2.5 rounded-lg bg-muted/40 border border-border/60">
          <Badge
            variant="secondary"
            className="gap-1.5 px-3 py-1 text-xs font-semibold tracking-wide uppercase shadow-none border border-slate-300 dark:border-slate-700"
          >
            <Shield className="size-3.5 text-primary" aria-hidden="true" />
            <span>{formatAdminRole(activeRole)}</span>
          </Badge>
          {snapshot.adminSessionStatus === "authenticated" && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => void logoutAdminSession()}
              disabled={snapshot.actionLoading}
              className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
            >
              <LogOut className="size-3.5 mr-1" aria-hidden="true" />
              Salir
              <span className="sr-only"> de la sesión administrativa</span>
            </Button>
          )}
        </div>
      )}

      <nav
        aria-label="Navegación de secciones de administración"
        className="flex flex-col gap-1.5 min-w-0"
      >
        {accessibleItems.map((item) => {
          const isActive = item.exact
            ? pathname === item.href
            : pathname.startsWith(item.href);
          const Icon = item.icon;

          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={onNavigate}
              aria-current={isActive ? "page" : undefined}
              className={cn(
                "flex items-center gap-3 rounded-lg px-3.5 py-2.5 text-sm font-medium transition-colors min-w-0",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                isActive
                  ? "bg-[var(--brand-primary-soft)] text-[var(--brand-primary-soft-ink)] font-semibold border-l-2 border-primary"
                  : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
              )}
            >
              <Icon className="size-4 shrink-0" aria-hidden="true" />
              <div className="flex flex-col min-w-0 text-left">
                <span className="truncate">{item.label}</span>
                <span
                  aria-hidden="true"
                  className={cn(
                    "text-xs font-normal hidden lg:block truncate",
                    isActive
                      ? "text-[var(--brand-primary-soft-ink)]/85"
                      : "text-muted-foreground",
                  )}
                >
                  {item.description}
                </span>
              </div>
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
