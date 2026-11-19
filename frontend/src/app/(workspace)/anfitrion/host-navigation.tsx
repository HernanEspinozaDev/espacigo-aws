"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Building2,
  PlusCircle,
  CalendarCheck,
  User,
  WalletCards,
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
  const isReservas = pathname.includes("/anfitrion/reservas");
  const isPayments = pathname === "/anfitrion/mercado-pago";
  const isListado =
    (pathname === "/anfitrion/espacios" || pathname === "/anfitrion") &&
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
            ) : isPayments ? (
              <BreadcrumbLink href="/anfitrion/espacios">
                Mis Espacios
              </BreadcrumbLink>
            ) : (
              <BreadcrumbLink href="/mi-cuenta/espacios">
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
              ? "border-primary text-primary bg-muted/40 font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <Building2 className="size-4" aria-hidden="true" />
          <span>Mis Espacios</span>
        </Link>

        <Link
          href="/anfitrion/reservas"
          aria-current={isReservas ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isReservas
              ? "border-primary text-primary bg-muted/40 font-semibold"
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
              ? "border-primary text-primary bg-muted/40 font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
          )}
        >
          <WalletCards className="size-4" aria-hidden="true" />
          <span>Pagos</span>
        </Link>

        <Link
          href="/anfitrion/espacios/nuevo"
          aria-current={isNuevo ? "page" : undefined}
          className={cn(
            "inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 whitespace-nowrap transition-colors rounded-t-md",
            isNuevo
              ? "border-primary text-primary bg-muted/40 font-semibold"
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
