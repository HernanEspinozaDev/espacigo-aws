"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Building2,
  PlusCircle,
  CalendarCheck,
  FileText,
  User,
  WalletCards,
  Users,
  ShieldCheck,
} from "lucide-react";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { cn } from "@/lib/utils";

export function HostNavigation() {
  const pathname = usePathname();

  const isNuevo = pathname.includes("/espacios/nuevo");
  const isEditar = pathname.includes("/editar");
  const isContratos = pathname.includes("/anfitrion/contratos");
  const isReservas = pathname.includes("/anfitrion/reservas");
  const isPayments = pathname === "/anfitrion/mercado-pago";
  const isEmpresas = pathname.startsWith("/anfitrion/empresas");
  const isKyb = pathname.startsWith("/anfitrion/kyb");
  const isListado =
    (pathname === "/anfitrion/espacios" || pathname === "/anfitrion") &&
    !isContratos &&
    !isReservas &&
    !isPayments;

  return (
    <div className="space-y-4">
      {/* Migas de pan accesibles */}
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink href="/mi-cuenta">Mi cuenta</BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            {isListado ? (
              <BreadcrumbPage>Mis Espacios</BreadcrumbPage>
            ) : (
              <BreadcrumbLink href="/anfitrion/espacios">
                Mis Espacios
              </BreadcrumbLink>
            )}
          </BreadcrumbItem>
          {isNuevo && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Publicar Nuevo Espacio</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isEditar && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Editar Espacio</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isContratos && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Contratos</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isReservas && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Reservas</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isPayments && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Pagos</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isEmpresas && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Empresas</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
          {isKyb && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>Verificación y documentos</BreadcrumbPage>
              </BreadcrumbItem>
            </>
          )}
        </BreadcrumbList>
      </Breadcrumb>

      {/* Barra de navegación de anfitrión */}
      <nav
        aria-label="Navegación del panel de anfitrión"
        className="border-b border-border flex items-center gap-1 overflow-x-auto pb-px"
      >
        <Link
          href="/mi-cuenta"
          className="inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md border-transparent text-muted-foreground hover:text-foreground hover:border-border"
        >
          <User className="size-4" aria-hidden="true" />
          <span>Mi Cuenta</span>
        </Link>

        <Link
          href="/anfitrion/espacios"
          aria-current={isListado ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isListado
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <Building2 className="size-4" aria-hidden="true" />
          <span>Mis Espacios</span>
        </Link>

        <Link
          href="/anfitrion/contratos"
          aria-current={isContratos ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isContratos
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <FileText className="size-4" aria-hidden="true" />
          <span>Contratos</span>
        </Link>

        <Link
          href="/anfitrion/reservas"
          aria-current={isReservas ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isReservas
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <CalendarCheck className="size-4" aria-hidden="true" />
          <span>Reservas</span>
        </Link>

        <Link
          href="/anfitrion/mercado-pago"
          aria-current={isPayments ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isPayments
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <WalletCards className="size-4" aria-hidden="true" />
          <span>Pagos</span>
        </Link>

        <Link
          href="/anfitrion/empresas"
          aria-current={isEmpresas ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isEmpresas
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <Users className="size-4" aria-hidden="true" />
          <span>Empresas</span>
        </Link>

        <Link
          href="/anfitrion/kyb"
          aria-current={isKyb ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isKyb
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <ShieldCheck className="size-4" aria-hidden="true" />
          <span>Verificación</span>
        </Link>

        <Link
          href="/anfitrion/espacios/nuevo"
          aria-current={isNuevo ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isNuevo
              ? "border-primary text-[var(--brand-primary-soft-ink)] bg-[var(--brand-primary-soft)] font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <PlusCircle className="size-4" aria-hidden="true" />
          <span>Publicar Espacio</span>
        </Link>
      </nav>
    </div>
  );
}
