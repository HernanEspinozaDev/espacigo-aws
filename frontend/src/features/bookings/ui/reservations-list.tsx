"use client";

import { useEffect, useState, useMemo, useCallback } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/features/auth/ui/use-auth";
import { useBookings } from "./use-bookings";
import { HostCommissionCalculator } from "./host-commission-calculator";
import {
  getRecordedProcessingFee,
  getReservationFinancialBreakdown,
  getReservationFinancialMovement,
  type ReservationFinancialAdjustment,
} from "../domain/financial-breakdown";
import { formatBookingClp } from "./format";
import { HostSpaceSchedulePanel } from "./host-space-schedule-panel";
import { ReservationScheduleBreakdown } from "./reservation-schedule-breakdown";
import {
  buildInvoiceNumber,
  ReservationInvoiceModal,
} from "./reservation-invoice-modal";
import { ReservationPassesModal } from "./reservation-passes-modal";
import { HostReceptionModal } from "./host-reception-modal";
import type { BookingReservation, BookingState } from "../domain/types";
import { ReservationCountdown } from "@/features/rental-checkout/ui/reservation-countdown";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import {
  BarChart3,
  Building2,
  Calendar,
  ChevronRight,
  Clock,
  ArrowRight,
  Search,
  FileText,
  AlertCircle,
  CheckCircle,
  Clock3,
  DollarSign,
  ListChecks,
  QrCode,
  XCircle,
  HelpCircle,
  Receipt,
  RefreshCw,
  Wallet,
} from "lucide-react";

export type ReservationFilterGroup =
  "todas" | "solicitada" | "confirmada" | "activa" | "completada" | "cancelada";

const processingRateFormatter = new Intl.NumberFormat("es-CL", {
  style: "percent",
  maximumFractionDigits: 2,
});

export type ReservationSectionTab = "general" | "finanzas" | "lista";

interface ReservationsListProps {
  mode?: "renter" | "host";
  initialSpaceId?: string;
  initialSection?: ReservationSectionTab;
  title?: string;
  description?: string;
}

export function getBookingStateConfig(state: BookingState): {
  label: string;
  group: ReservationFilterGroup;
  badgeClass: string;
  icon: React.ComponentType<{ className?: string }>;
} {
  switch (state) {
    case "pendiente_de_pago":
      return {
        label: "Pendiente de Pago",
        group: "solicitada",
        badgeClass:
          "bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20",
        icon: Clock3,
      };
    case "pagada":
      return {
        label: "Pago Aprobado · Reserva Guardada",
        group: "confirmada",
        badgeClass:
          "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20",
        icon: CheckCircle,
      };
    case "aprobada_host":
      return {
        label: "Aprobada por Anfitrión",
        group: "confirmada",
        badgeClass:
          "bg-indigo-500/10 text-indigo-700 dark:text-indigo-400 border-indigo-500/20",
        icon: CheckCircle,
      };
    case "firma_parcial":
      return {
        label: "Firma Parcial de Contrato",
        group: "confirmada",
        badgeClass:
          "bg-indigo-500/10 text-indigo-700 dark:text-indigo-400 border-indigo-500/20",
        icon: FileText,
      };
    case "lista_para_checkin":
      return {
        label: "Lista para Check-in",
        group: "confirmada",
        badgeClass:
          "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20",
        icon: CheckCircle,
      };
    case "en_curso":
      return {
        label: "Arriendo en Curso",
        group: "activa",
        badgeClass:
          "bg-emerald-600/15 text-emerald-800 dark:text-emerald-300 border-emerald-600/30 animate-pulse",
        icon: Clock,
      };
    case "finalizada":
      return {
        label: "Finalizada (Check-out)",
        group: "completada",
        badgeClass:
          "bg-teal-500/10 text-teal-700 dark:text-teal-400 border-teal-500/20",
        icon: CheckCircle,
      };
    case "cerrada":
      return {
        label: "Cerrada y Liquidada",
        group: "completada",
        badgeClass:
          "bg-zinc-500/10 text-zinc-700 dark:text-zinc-400 border-zinc-500/20",
        icon: CheckCircle,
      };
    case "en_disputa":
      return {
        label: "En Disputa",
        group: "cancelada",
        badgeClass:
          "bg-rose-500/10 text-rose-700 dark:text-rose-400 border-rose-500/20",
        icon: AlertCircle,
      };
    case "reconciliation_required":
      return {
        label: "Conciliación Requerida",
        group: "cancelada",
        badgeClass:
          "bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20",
        icon: AlertCircle,
      };
    case "cancelada_por_firma":
    case "cancelada_por_pago":
      return {
        label: "Reserva cancelada por no pago",
        group: "cancelada",
        badgeClass: "bg-destructive/10 text-destructive border-destructive/20",
        icon: XCircle,
      };
    case "cancelada":
      return {
        label: "Reserva cancelada",
        group: "cancelada",
        badgeClass: "bg-destructive/10 text-destructive border-destructive/20",
        icon: XCircle,
      };
    case "vencida_pago":
      return {
        label: "Reserva cancelada por tiempo (no pagada)",
        group: "cancelada",
        badgeClass: "bg-destructive/10 text-destructive border-destructive/20",
        icon: XCircle,
      };
    case "rechazada_arrendador":
    case "vencida_host":
    case "cancelada_arrendatario":
      return {
        label: state.replace(/_/g, " "),
        group: "cancelada",
        badgeClass: "bg-destructive/10 text-destructive border-destructive/20",
        icon: XCircle,
      };
    case "reembolsada":
      return {
        label: "Reserva reembolsada",
        group: "cancelada",
        badgeClass:
          "bg-sky-500/10 text-sky-700 dark:text-sky-300 border-sky-500/20",
        icon: CheckCircle,
      };
    default:
      return {
        label: state,
        group: "todas",
        badgeClass: "bg-muted text-muted-foreground border-border",
        icon: HelpCircle,
      };
  }
}

const PAID_OR_APPROVED_STATES: ReadonlySet<BookingState> = new Set([
  "pagada",
  "aprobada_host",
  "lista_para_checkin",
  "en_curso",
  "finalizada",
  "cerrada",
]);

const FINANCE_ADJUSTMENT_LABEL: Record<ReservationFinancialAdjustment, string> =
  {
    none: "Sin reembolso",
    refund_pending: "Reembolso pendiente",
    refund_confirmed: "Reembolso confirmado",
    refund_failed: "Reembolso no confirmado",
    chargeback: "Contracargo",
  };

function isReservationPaid(reservation: BookingReservation): boolean {
  return (
    reservation.state === "pagada" || reservation.checkout_status === "approved"
  );
}

export function ReservationsList({
  mode = "renter",
  initialSpaceId,
  initialSection,
  title,
  description,
}: ReservationsListProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { snapshot: authSnapshot } = useAuth();
  const isAuthenticated = authSnapshot.status === "authenticated";
  const accountId = authSnapshot.accountId;
  const expiresAt = authSnapshot.expiresAt;

  const { reservations, loading, error, loadReservations } = useBookings();
  const [activeSection, setActiveSection] = useState<ReservationSectionTab>(
    initialSection ?? (initialSpaceId ? "lista" : "general"),
  );
  const [selectedSpaceId, setSelectedSpaceId] = useState<string>(
    initialSpaceId || "all",
  );
  const [activeTab, setActiveTab] = useState<ReservationFilterGroup>("todas");
  const [searchQuery, setSearchQuery] = useState("");
  const [page, setPage] = useState(1);
  const [invoiceReservation, setInvoiceReservation] =
    useState<BookingReservation | null>(null);
  const [passesReservation, setPassesReservation] =
    useState<BookingReservation | null>(null);
  const [receptionOpen, setReceptionOpen] = useState(false);

  const identityKey =
    authSnapshot.status === "checking"
      ? null
      : `${authSnapshot.status}:${accountId}:${expiresAt}`;
  const [prevIdentityKey, setPrevIdentityKey] = useState<string | null>(
    identityKey,
  );

  if (identityKey !== null && prevIdentityKey !== identityKey) {
    const wasChecking = prevIdentityKey === null;
    setPrevIdentityKey(identityKey);
    if (!wasChecking) {
      setActiveSection(
        initialSection ?? (initialSpaceId ? "lista" : "general"),
      );
      setSelectedSpaceId(initialSpaceId || "all");
      setActiveTab("todas");
      setSearchQuery("");
      setInvoiceReservation(null);
    }
  }

  const effectiveMode: "renter" | "host" = mode;

  const syncRouteParams = useCallback(
    (nextSection: ReservationSectionTab, nextSpaceId: string) => {
      if (effectiveMode !== "host") return;
      const params = new URLSearchParams();
      params.set("tab", nextSection);
      if (nextSpaceId && nextSpaceId !== "all") {
        params.set("space_id", nextSpaceId);
      }
      router.replace(`${pathname}?${params.toString()}`, { scroll: false });
    },
    [effectiveMode, pathname, router],
  );

  const handleSectionChange = useCallback(
    (nextSection: ReservationSectionTab) => {
      setActiveSection(nextSection);
      syncRouteParams(nextSection, selectedSpaceId);
    },
    [selectedSpaceId, syncRouteParams],
  );

  const handleSpaceChange = useCallback(
    (nextSpaceId: string, switchToList = false) => {
      const targetSection = switchToList ? "lista" : activeSection;
      setSelectedSpaceId(nextSpaceId);
      if (switchToList) {
        setActiveSection("lista");
      }
      syncRouteParams(targetSection, nextSpaceId);
    },
    [activeSection, syncRouteParams],
  );

  useEffect(() => {
    if (isAuthenticated) {
      loadReservations().catch(() => {});
    }
  }, [isAuthenticated, accountId, expiresAt, loadReservations]);

  const roleCounts = useMemo(() => {
    let renter = 0;
    let host = 0;
    for (const r of reservations) {
      if (r.participant_role === "anfitrion") host += 1;
      else renter += 1;
    }
    return { renter, host };
  }, [reservations]);

  const roleReservations = useMemo(() => {
    return reservations.filter((reservation) => {
      if (effectiveMode === "host") {
        if (
          reservation.participant_role &&
          reservation.participant_role !== "anfitrion"
        ) {
          return false;
        }
        if (
          reservation.host_id &&
          accountId &&
          reservation.host_id !== "me" &&
          reservation.host_id !== accountId
        ) {
          return false;
        }
        return true;
      }
      if (
        reservation.participant_role &&
        reservation.participant_role !== "arrendatario"
      ) {
        return false;
      }
      if (
        reservation.renter_id &&
        accountId &&
        reservation.renter_id !== "me" &&
        reservation.renter_id !== accountId
      ) {
        return false;
      }
      return true;
    });
  }, [reservations, effectiveMode, accountId]);

  const spacesDirectory = useMemo(() => {
    if (effectiveMode !== "host") return [];
    const map = new Map<
      string,
      {
        spaceId: string;
        spaceTitle: string;
        total: number;
        approved: number;
        pending: number;
        rejectedOrExpired: number;
        grossClp: number;
        netClp: number;
      }
    >();
    for (const r of roleReservations) {
      const entry = map.get(r.space_id) ?? {
        spaceId: r.space_id,
        spaceTitle: r.space_title || `Espacio ${r.space_id.slice(0, 8)}`,
        total: 0,
        approved: 0,
        pending: 0,
        rejectedOrExpired: 0,
        grossClp: 0,
        netClp: 0,
      };
      entry.total += 1;
      if (PAID_OR_APPROVED_STATES.has(r.state)) {
        const financial = getReservationFinancialBreakdown(r);
        entry.approved += 1;
        entry.grossClp += financial.baseClp;
        entry.netClp += financial.netHostClp;
      } else if (r.state === "pendiente_de_pago") {
        entry.pending += 1;
      } else {
        entry.rejectedOrExpired += 1;
      }
      map.set(r.space_id, entry);
    }
    return Array.from(map.values());
  }, [effectiveMode, roleReservations]);

  const scopedReservations = useMemo(() => {
    if (effectiveMode !== "host" || selectedSpaceId === "all") {
      return roleReservations;
    }
    return roleReservations.filter((r) => r.space_id === selectedSpaceId);
  }, [effectiveMode, roleReservations, selectedSpaceId]);

  const kpiStats = useMemo(() => {
    let approved = 0;
    let pending = 0;
    let rejected = 0;
    let grossClp = 0;
    let financeGrossClp = 0;
    let espacigoCommissionClp = 0;
    let espacigoVatClp = 0;
    let hostFeeClp = 0;
    let confirmedRefundClp = 0;
    let confirmedRefundCount = 0;
    let refundAmountPendingCount = 0;
    let chargebackCount = 0;
    let netClp = 0;
    let pendingSettlementCount = 0;
    let mercadoPagoPaymentCount = 0;
    let processingFeeCount = 0;
    let processingFeeClp = 0;
    let processingFeeChargedClp = 0;
    const paidList: BookingReservation[] = [];

    for (const r of scopedReservations) {
      const paid = PAID_OR_APPROVED_STATES.has(r.state);
      if (paid) {
        approved += 1;
        grossClp += getReservationFinancialBreakdown(r).baseClp;
      } else if (r.state === "pendiente_de_pago") {
        pending += 1;
      } else {
        rejected += 1;
      }

      const hasCommercialPayment =
        r.is_commercial &&
        (r.checkout_status === "approved" ||
          r.state === "reembolsada" ||
          r.commercial_refund_status === "aprobado" ||
          r.provider_settlement?.payment_status !== undefined);
      if (!paid && !hasCommercialPayment) continue;

      const financial = getReservationFinancialBreakdown(r);
      const movement = getReservationFinancialMovement(r, financial);
      financeGrossClp += financial.baseClp;
      espacigoCommissionClp += financial.hostCommissionClp;
      espacigoVatClp += financial.hostVatClp;
      hostFeeClp += financial.hostFeeClp;
      if (movement.reconciledNetClp === undefined) {
        pendingSettlementCount += 1;
      } else {
        netClp += movement.reconciledNetClp;
      }
      if (movement.adjustment === "chargeback") {
        chargebackCount += 1;
      }
      if (movement.confirmedRefundClp !== undefined) {
        confirmedRefundCount += 1;
        confirmedRefundClp += movement.confirmedRefundClp;
      } else if (movement.adjustment === "refund_confirmed") {
        refundAmountPendingCount += 1;
      }
      if (r.provider_payment_id) mercadoPagoPaymentCount += 1;
      const processing = getRecordedProcessingFee(r, financial);
      if (processing) {
        processingFeeCount += 1;
        processingFeeClp += processing.amountClp;
        processingFeeChargedClp += processing.chargedClp;
      }
      paidList.push(r);
    }

    return {
      total: scopedReservations.length,
      approved,
      pending,
      rejected,
      grossClp,
      financeGrossClp,
      espacigoCommissionClp,
      espacigoVatClp,
      hostFeeClp,
      confirmedRefundClp,
      confirmedRefundCount,
      refundAmountPendingCount,
      chargebackCount,
      netClp,
      pendingSettlementCount,
      mercadoPagoPaymentCount,
      processingFeeCount,
      processingFeeClp,
      processingFeeChargedClp,
      paidList,
    };
  }, [scopedReservations]);

  const filteredReservations = useMemo(() => {
    return scopedReservations.filter((reservation) => {
      const stateConfig = getBookingStateConfig(reservation.state);
      const matchesTab =
        activeTab === "todas" || stateConfig.group === activeTab;

      const query = searchQuery.trim().toLowerCase();
      const matchesSearch =
        !query ||
        reservation.id.toLowerCase().includes(query) ||
        reservation.space_id.toLowerCase().includes(query) ||
        (reservation.provider_payment_id &&
          reservation.provider_payment_id.toLowerCase().includes(query)) ||
        (reservation.space_title &&
          reservation.space_title.toLowerCase().includes(query));

      return matchesTab && matchesSearch;
    });
  }, [scopedReservations, activeTab, searchQuery]);

  const pageSize = 10;
  const totalPages = Math.max(
    1,
    Math.ceil(filteredReservations.length / pageSize),
  );
  const currentPage = Math.min(page, totalPages);
  const paginatedReservations = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredReservations.slice(start, start + pageSize);
  }, [filteredReservations, currentPage]);

  const renterCounts = useMemo(
    () => ({
      todas: scopedReservations.length,
      solicitada: scopedReservations.filter(
        (r) => getBookingStateConfig(r.state).group === "solicitada",
      ).length,
      confirmada: scopedReservations.filter(
        (r) => getBookingStateConfig(r.state).group === "confirmada",
      ).length,
      cancelada: scopedReservations.filter(
        (r) => getBookingStateConfig(r.state).group === "cancelada",
      ).length,
    }),
    [scopedReservations],
  );

  const detailBaseHref =
    effectiveMode === "host" ? "/anfitrion/reservas" : "/reservas";

  const renterFilterTabs: Array<{
    id: ReservationFilterGroup;
    label: string;
  }> = [
    { id: "todas", label: "Todas" },
    { id: "solicitada", label: "Solicitadas" },
    { id: "confirmada", label: "Confirmadas" },
    { id: "cancelada", label: "Canceladas / Cierre" },
  ];

  const hostFilterTabs: Array<{ id: ReservationFilterGroup; label: string }> = [
    { id: "todas", label: "Todas" },
    { id: "solicitada", label: "Solicitadas" },
    { id: "confirmada", label: "Confirmadas" },
    { id: "activa", label: "En Curso" },
    { id: "completada", label: "Completadas" },
    { id: "cancelada", label: "Canceladas / Cierre" },
  ];

  const filterTabs =
    effectiveMode === "renter" ? renterFilterTabs : hostFilterTabs;

  // =========================================================================
  // VISTA ESTRICTA DE ARRENDATARIO (/reservas)
  // =========================================================================
  if (effectiveMode === "renter") {
    return (
      <div className="space-y-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
              {title ?? "Mis Reservas"}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {description ??
                "Monitorea tus arriendos, pagos confirmados con Mercado Pago y comprobantes."}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2.5">
            {isAuthenticated && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => loadReservations()}
                disabled={loading}
                className="inline-flex items-center gap-2"
              >
                <RefreshCw
                  className={`h-4 w-4 ${loading ? "animate-spin" : ""}`}
                />
                Actualizar
              </Button>
            )}
            <Button asChild size="sm">
              <Link href="/explorar">Explorar Espacios</Link>
            </Button>
          </div>
        </div>

        {/* Selector claro de rol cuando el usuario también tiene reservas como anfitrión */}
        {roleCounts.host > 0 && (
          <div className="flex flex-col gap-3 rounded-2xl border border-primary/25 bg-primary/5 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-xs">
                <Building2 className="h-5 w-5" />
              </div>
              <div>
                <p className="text-sm font-semibold text-foreground">
                  Tienes {roleCounts.host} reserva
                  {roleCounts.host === 1 ? "" : "s"} registrada
                  {roleCounts.host === 1 ? "" : "s"} en tus espacios publicados
                </p>
                <p className="text-xs text-muted-foreground">
                  Aquí estás viendo tus reservas como cliente. Para revisar
                  KPIs, finanzas, agenda horaria e invoices de tus propiedades,
                  abre el panel de anfitrión.
                </p>
              </div>
            </div>
            <Button
              asChild
              size="sm"
              className="shrink-0 font-semibold shadow-xs"
            >
              <Link
                href="/anfitrion/reservas"
                className="inline-flex items-center gap-1.5"
              >
                Ver reservas de mis espacios ({roleCounts.host})
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          </div>
        )}

        <Card>
          <CardHeader>
            <CardTitle>Listado de reservas</CardTitle>
            <CardDescription>
              Filtra por estado, busca por espacio o folio y revisa tus
              comprobantes o pases QR.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {error && (
              <div
                role="alert"
                className="flex items-start gap-2.5 rounded-xl border border-destructive/20 bg-destructive/10 p-3.5 text-sm text-red-800 dark:text-red-300"
              >
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                <div>
                  <p className="font-medium">
                    No pudimos sincronizar las reservas
                  </p>
                  <p className="mt-0.5">{error}</p>
                </div>
              </div>
            )}

            <div className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <nav
                  aria-label="Filtro por estado de reserva"
                  className="flex flex-wrap gap-1.5"
                >
                  {filterTabs.map((tab) => {
                    const isSelected = activeTab === tab.id;
                    const count =
                      renterCounts[tab.id as keyof typeof renterCounts] ?? 0;
                    return (
                      <button
                        key={tab.id}
                        type="button"
                        onClick={() => {
                          setActiveTab(tab.id);
                          setPage(1);
                        }}
                        aria-current={isSelected ? "true" : undefined}
                        className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors ${
                          isSelected
                            ? "border-primary bg-primary text-primary-foreground"
                            : "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground"
                        }`}
                      >
                        <span>{tab.label}</span>
                        <span className="tabular-nums opacity-75">
                          ({count})
                        </span>
                      </button>
                    );
                  })}
                </nav>
              </div>

              <div className="relative">
                <Search
                  className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
                  aria-hidden="true"
                />
                <input
                  type="search"
                  value={searchQuery}
                  onChange={(e) => {
                    setSearchQuery(e.target.value);
                    setPage(1);
                  }}
                  placeholder="Buscar por espacio, folio o ID de operación…"
                  className="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm outline-none focus:border-primary"
                />
              </div>
            </div>

            {loading && reservations.length === 0 ? (
              <div className="space-y-3">
                {[1, 2, 3].map((n) => (
                  <div
                    key={n}
                    className="h-20 animate-pulse rounded-lg border border-border bg-muted/30"
                  />
                ))}
              </div>
            ) : filteredReservations.length === 0 ? (
              <div className="rounded-xl border border-dashed border-border p-10 text-center">
                <Calendar className="mx-auto h-10 w-10 text-muted-foreground/50" />
                <h2 className="mt-3 text-base font-medium text-foreground">
                  No se encontraron reservas
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {searchQuery || activeTab !== "todas"
                    ? "No hay reservas que coincidan con los filtros seleccionados."
                    : "Aún no tienes reservas activas ni solicitudes en proceso."}
                </p>
                {!searchQuery && activeTab === "todas" && (
                  <div className="mt-5">
                    <Button asChild size="sm">
                      <Link href="/explorar">
                        Explorar Espacios Disponibles
                      </Link>
                    </Button>
                  </div>
                )}
              </div>
            ) : (
              <>
                <ul
                  className="divide-y divide-border rounded-lg border border-border"
                  aria-label="Listado de reservas"
                >
                  {paginatedReservations.map((reservation) => {
                    const config = getBookingStateConfig(reservation.state);
                    const Icon = config.icon;
                    const financial =
                      getReservationFinancialBreakdown(reservation);
                    const totalPaidClp =
                      reservation.total_charged_clp ?? reservation.subtotal_clp;
                    const canViewReceipt = isReservationPaid(reservation);

                    return (
                      <li
                        key={reservation.id}
                        className="flex flex-wrap items-center justify-between gap-3 p-3.5 transition-colors hover:bg-muted/20"
                      >
                        <div className="min-w-0 space-y-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <Link
                              href={`${detailBaseHref}/${encodeURIComponent(reservation.id)}`}
                              className="text-sm font-semibold text-foreground hover:underline"
                            >
                              {reservation.space_title ||
                                `Reserva ${reservation.id.slice(0, 8)}`}
                            </Link>
                            <span
                              className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium ${config.badgeClass}`}
                            >
                              <Icon className="h-3 w-3" aria-hidden="true" />
                              {config.label}
                            </span>
                            {reservation.unread_count > 0 && (
                              <span className="rounded-full bg-primary px-2 py-0.5 text-[10px] font-semibold text-primary-foreground">
                                {reservation.unread_count} nuevo
                                {reservation.unread_count > 1 ? "s" : ""}
                              </span>
                            )}
                          </div>
                          <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                            <span className="font-mono">
                              #{reservation.id.slice(0, 8)}
                            </span>
                            <span>·</span>
                            <span>
                              {reservation.units} {reservation.rate_unit}
                              {reservation.units > 1 ? "s" : ""}
                            </span>
                            <span>·</span>
                            <span className="font-semibold tabular-nums text-foreground">
                              {formatBookingClp(totalPaidClp)} CLP
                            </span>
                            {financial.buyerFeeClp > 0 && (
                              <span className="text-[11px]">
                                (incl. servicio)
                              </span>
                            )}
                            {reservation.provider_payment_id && (
                              <>
                                <span>·</span>
                                <span className="font-mono text-emerald-700 dark:text-emerald-300">
                                  MP #{reservation.provider_payment_id}
                                </span>
                              </>
                            )}
                          </p>
                          {reservation.state === "pendiente_de_pago" &&
                            reservation.pay_expires_at && (
                              <div className="pt-0.5">
                                <ReservationCountdown
                                  expiresAt={reservation.pay_expires_at}
                                />
                              </div>
                            )}
                        </div>

                        <div className="flex flex-wrap items-center gap-2">
                          {canViewReceipt && (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => setPassesReservation(reservation)}
                            >
                              <QrCode
                                className="h-3.5 w-3.5"
                                aria-hidden="true"
                              />
                              Pases / QR
                            </Button>
                          )}
                          {canViewReceipt && (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() =>
                                setInvoiceReservation({
                                  ...reservation,
                                  participant_role: "arrendatario",
                                })
                              }
                            >
                              <FileText
                                className="h-3.5 w-3.5"
                                aria-hidden="true"
                              />
                              Comprobante
                            </Button>
                          )}
                          {reservation.state === "pendiente_de_pago" &&
                            reservation.quote_id && (
                              <Button asChild variant="outline" size="sm">
                                <Link
                                  href={`/reservas/checkout/${encodeURIComponent(reservation.quote_id)}`}
                                >
                                  Estado del pago
                                </Link>
                              </Button>
                            )}
                          <Button
                            asChild
                            size="sm"
                            variant={
                              reservation.state === "pendiente_de_pago"
                                ? "default"
                                : "outline"
                            }
                          >
                            <Link
                              href={`${detailBaseHref}/${encodeURIComponent(reservation.id)}`}
                            >
                              Ver reserva
                            </Link>
                          </Button>
                        </div>
                      </li>
                    );
                  })}
                </ul>

                {totalPages > 1 && (
                  <div className="flex items-center justify-between border-t border-border pt-3 text-sm">
                    <span className="text-xs tabular-nums text-muted-foreground">
                      Página {currentPage} de {totalPages}
                    </span>
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={currentPage <= 1}
                        onClick={() => setPage((p) => Math.max(1, p - 1))}
                      >
                        Anterior
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={currentPage >= totalPages}
                        onClick={() =>
                          setPage((p) => Math.min(totalPages, p + 1))
                        }
                      >
                        Siguiente
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        <ReservationInvoiceModal
          reservation={invoiceReservation}
          onClose={() => setInvoiceReservation(null)}
        />
        <ReservationPassesModal
          reservation={passesReservation}
          onClose={() => setPassesReservation(null)}
        />
      </div>
    );
  }

  // =========================================================================
  // VISTA DE ANFITRIÓN (/anfitrion/reservas)
  // =========================================================================
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
            {title ?? "Reservas y Pagos de Mis Espacios"}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {description ??
              "Revisa los pagos confirmados con Mercado Pago, horarios reservados, desglose de comisiones y disponibilidad libre de tus espacios."}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2.5">
          {isAuthenticated && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => loadReservations()}
              disabled={loading}
              className="inline-flex items-center gap-2"
            >
              <RefreshCw
                className={`h-4 w-4 ${loading ? "animate-spin" : ""}`}
              />
              Actualizar
            </Button>
          )}
          <Button asChild size="sm">
            <Link href="/anfitrion/espacios">Mis Espacios</Link>
          </Button>
        </div>
      </div>

      {/* Ruta de navegación (Breadcrumb con space_id y pestaña activa) */}
      <nav
        aria-label="Ruta de navegación de reservas"
        className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground"
      >
        <button
          type="button"
          onClick={() => handleSpaceChange("all", false)}
          className="font-medium underline-offset-4 hover:text-foreground hover:underline"
        >
          Reservas de Mis Espacios
        </button>
        {selectedSpaceId !== "all" && (
          <>
            <ChevronRight className="h-3.5 w-3.5" />
            <span className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 font-mono text-foreground">
              <Building2 className="h-3 w-3 text-primary" />
              Espacio: {selectedSpaceId}
            </span>
          </>
        )}
        <ChevronRight className="h-3.5 w-3.5" />
        <span className="font-semibold text-foreground">
          {activeSection === "general"
            ? "General Reservas"
            : activeSection === "finanzas"
              ? "Finanzas"
              : "Lista de Reservas"}
        </span>
      </nav>

      {/* Subnavegador interno: General Reservas | Finanzas | Lista de Reservas */}
      <div
        role="tablist"
        aria-label="Secciones de reservas"
        className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-2"
      >
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            role="tab"
            aria-selected={activeSection === "general"}
            onClick={() => handleSectionChange("general")}
            className={`inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
              activeSection === "general"
                ? "border-primary font-semibold text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <BarChart3 className="h-4 w-4" />
            General Reservas
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSection === "finanzas"}
            onClick={() => handleSectionChange("finanzas")}
            className={`inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
              activeSection === "finanzas"
                ? "border-primary font-semibold text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <Wallet className="h-4 w-4" />
            Finanzas
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSection === "lista"}
            onClick={() => handleSectionChange("lista")}
            className={`inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
              activeSection === "lista"
                ? "border-primary font-semibold text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <ListChecks className="h-4 w-4" />
            Lista de Reservas ({filteredReservations.length})
          </button>
        </div>
        <Button
          type="button"
          size="sm"
          onClick={() => setReceptionOpen(true)}
          className="inline-flex items-center gap-1.5 bg-[#D46236] text-white hover:bg-[#B94628]"
        >
          <QrCode className="h-4 w-4" />
          Validar Ingreso (Recepción)
        </Button>
      </div>

      {/* Global Error Banner */}
      {error && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-xl border border-destructive/20 bg-destructive/10 p-4 text-sm text-red-800 dark:text-red-300"
        >
          <AlertCircle className="mt-0.5 h-5 w-5 shrink-0" />
          <div>
            <p className="font-medium">No pudimos sincronizar las reservas</p>
            <p className="mt-0.5">{error}</p>
          </div>
        </div>
      )}

      {/* SECCIÓN 1: GENERAL RESERVAS */}
      {activeSection === "general" && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div className="rounded-2xl border border-emerald-500/30 bg-emerald-500/10 p-4 shadow-xs">
              <p className="text-xs font-medium text-emerald-800 dark:text-emerald-300">
                Reservas Aprobadas / Pagadas
              </p>
              <p className="mt-2 text-3xl font-bold text-emerald-900 dark:text-emerald-200">
                {kpiStats.approved}
              </p>
              <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-400">
                Tarifa base: {formatBookingClp(kpiStats.grossClp)} CLP
              </p>
            </div>

            <div className="rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 shadow-xs">
              <p className="text-xs font-medium text-amber-800 dark:text-amber-300">
                Pendientes de Pago
              </p>
              <p className="mt-2 text-3xl font-bold text-amber-900 dark:text-amber-200">
                {kpiStats.pending}
              </p>
              <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
                Retención temporal activa
              </p>
            </div>

            <div className="rounded-2xl border border-destructive/25 bg-destructive/10 p-4 shadow-xs">
              <p className="text-xs font-medium text-destructive">
                Rechazadas / Vencidas
              </p>
              <p className="mt-2 text-3xl font-bold text-destructive">
                {kpiStats.rejected}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Horarios ya liberados
              </p>
            </div>

            <div className="rounded-2xl border border-border bg-card p-4 shadow-xs">
              <p className="text-xs font-medium text-muted-foreground">
                Espacios con Reservas
              </p>
              <p className="mt-2 text-3xl font-bold text-foreground">
                {spacesDirectory.length}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Total operaciones: {kpiStats.total}
              </p>
            </div>
          </div>

          {roleCounts.host > 0 ? (
            <HostSpaceSchedulePanel
              reservations={reservations}
              selectedSpaceId={selectedSpaceId}
              onSelectSpaceId={(spaceId) => handleSpaceChange(spaceId, false)}
              detailBaseHref={detailBaseHref}
            />
          ) : (
            <div className="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-border bg-card p-5">
              <div>
                <h2 className="text-base font-semibold text-foreground">
                  Sin reservas recibidas aún
                </h2>
                <p className="text-xs text-muted-foreground">
                  Cuando recibas solicitudes o pagos en tus espacios publicados,
                  verás aquí su agenda y resumen.
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                onClick={() => handleSectionChange("lista")}
              >
                Ver Lista de Reservas
                <ArrowRight className="ml-1.5 h-4 w-4" />
              </Button>
            </div>
          )}
        </div>
      )}

      {/* SECCIÓN 2: FINANZAS (Liquidación Anfitrión) */}
      {activeSection === "finanzas" && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
            <div className="rounded-2xl border border-border bg-card p-5 shadow-xs">
              <p className="text-xs font-medium text-muted-foreground">
                Tarifa Base de Espacios
              </p>
              <p className="mt-2 text-2xl font-bold text-foreground">
                {formatBookingClp(kpiStats.financeGrossClp)} CLP
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {kpiStats.paidList.length} cobro
                {kpiStats.paidList.length === 1 ? "" : "s"} registrado
                {kpiStats.paidList.length === 1 ? "" : "s"}
              </p>
            </div>

            <div className="rounded-2xl border border-amber-500/30 bg-amber-500/10 p-5 shadow-xs">
              <p className="text-xs font-medium text-amber-800 dark:text-amber-300">
                Comisión original EspaciGo (3% + IVA)
              </p>
              <p className="mt-2 text-2xl font-bold text-amber-900 dark:text-amber-200">
                -{formatBookingClp(kpiStats.hostFeeClp)} CLP
              </p>
              <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
                Comisión {formatBookingClp(kpiStats.espacigoCommissionClp)} +
                IVA {formatBookingClp(kpiStats.espacigoVatClp)}
              </p>
            </div>

            <div className="rounded-2xl border border-sky-500/30 bg-sky-500/10 p-5 shadow-xs">
              <p className="text-xs font-medium text-sky-800 dark:text-sky-300">
                Comisión Mercado Pago
              </p>
              <p className="mt-2 text-2xl font-bold text-sky-900 dark:text-sky-200">
                {kpiStats.processingFeeCount === 0
                  ? "Sin datos"
                  : `${formatBookingClp(kpiStats.processingFeeClp)} CLP`}
              </p>
              <p className="mt-1 text-xs text-sky-800 dark:text-sky-300">
                {kpiStats.processingFeeCount === 0
                  ? "Sin cargos informados para los pagos del filtro"
                  : `${processingRateFormatter.format(kpiStats.processingFeeClp / kpiStats.processingFeeChargedClp)} del total cobrado · ${kpiStats.processingFeeCount} de ${kpiStats.mercadoPagoPaymentCount} pagos con cargo informado`}
              </p>
            </div>

            <div className="rounded-2xl border border-border bg-card p-5 shadow-xs">
              <p className="text-xs font-medium text-muted-foreground">
                Reembolsos confirmados
              </p>
              <p className="mt-2 text-2xl font-bold text-foreground">
                {kpiStats.confirmedRefundCount > 0
                  ? `${formatBookingClp(kpiStats.confirmedRefundClp)} CLP`
                  : "Sin montos confirmados"}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {kpiStats.refundAmountPendingCount > 0
                  ? `${kpiStats.confirmedRefundCount} con monto informado · ${kpiStats.refundAmountPendingCount} por confirmar`
                  : "Montos devueltos al arrendatario"}
              </p>
            </div>

            <div className="rounded-2xl border border-emerald-500/30 bg-emerald-500/10 p-5 shadow-xs">
              <p className="text-xs font-medium text-emerald-800 dark:text-emerald-300">
                Neto informado para el anfitrión
              </p>
              <p className="mt-2 text-2xl font-bold text-emerald-900 dark:text-emerald-200">
                {kpiStats.paidList.length > 0 &&
                kpiStats.pendingSettlementCount === kpiStats.paidList.length
                  ? "Por confirmar"
                  : `${formatBookingClp(kpiStats.netClp)} CLP`}
              </p>
              <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-400">
                {kpiStats.pendingSettlementCount > 0
                  ? `${kpiStats.paidList.length - kpiStats.pendingSettlementCount} de ${kpiStats.paidList.length} pagos con neto informado`
                  : "Importes de las transacciones registradas"}
              </p>
            </div>
          </div>

          {kpiStats.chargebackCount > 0 && (
            <div
              role="status"
              className="flex items-start gap-2.5 rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-sm text-rose-900 dark:text-rose-200"
            >
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-rose-600 dark:text-rose-400" />
              <div>
                <p className="font-semibold">
                  Contracargo en curso ({kpiStats.chargebackCount}{" "}
                  {kpiStats.chargebackCount === 1 ? "operación" : "operaciones"}
                  )
                </p>
                <p className="mt-0.5 text-xs">
                  Los contracargos informados por Mercado Pago se registran como
                  incidencia independiente y su neto queda pendiente de
                  conciliar hasta la resolución del proveedor.
                </p>
              </div>
            </div>
          )}

          <HostCommissionCalculator reservations={kpiStats.paidList} />

          <div className="space-y-4 rounded-2xl border border-border bg-card p-5 shadow-xs">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h2 className="flex items-center gap-2 text-base font-semibold text-foreground">
                  <DollarSign className="h-4 w-4 text-primary" />
                  Detalle de cobros y reembolsos
                </h2>
                <p className="text-xs text-muted-foreground">
                  Consulta el cobro original, sus comisiones y los ajustes
                  confirmados de cada operación.
                </p>
              </div>
            </div>

            {kpiStats.paidList.length === 0 ? (
              <p className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
                Aún no hay transacciones aprobadas para mostrar en Finanzas.
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead className="border-b border-border text-muted-foreground">
                    <tr>
                      <th className="py-2.5 pr-3 font-semibold">Comprobante</th>
                      <th className="py-2.5 pr-3 font-semibold">Espacio</th>
                      <th className="py-2.5 pr-3 font-semibold">
                        Operación MP
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Tarifa Base
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Comisión EspaciGo
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Comisión Mercado Pago
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Cobro original
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Reembolso
                      </th>
                      <th className="py-2.5 pr-3 text-right font-semibold">
                        Neto anfitrión
                      </th>
                      <th className="py-2.5 text-right font-semibold">
                        Acción
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {kpiStats.paidList.map((tx) => {
                      const financial = getReservationFinancialBreakdown(tx);
                      const processing = getRecordedProcessingFee(
                        tx,
                        financial,
                      );
                      const movement = getReservationFinancialMovement(
                        tx,
                        financial,
                      );
                      return (
                        <tr key={tx.id} className="hover:bg-muted/30">
                          <td className="py-3 pr-3 font-mono font-semibold text-foreground">
                            {buildInvoiceNumber(tx.id)}
                          </td>
                          <td className="py-3 pr-3">
                            <span className="block font-medium text-foreground">
                              {tx.space_title || tx.space_id.slice(0, 8)}
                            </span>
                            <span className="font-mono text-[11px] text-muted-foreground">
                              {new Date(tx.start_at).toLocaleDateString(
                                "es-CL",
                              )}
                            </span>
                          </td>
                          <td className="py-3 pr-3 font-mono text-muted-foreground">
                            {tx.provider_payment_id
                              ? `#${tx.provider_payment_id}`
                              : "Confirmada"}
                            <span className="block text-[10px]">
                              {tx.checkout_method === "payments_api"
                                ? "Checkout API"
                                : "Checkout Pro"}
                            </span>
                          </td>
                          <td className="py-3 pr-3 text-right font-medium tabular-nums text-foreground">
                            {formatBookingClp(financial.baseClp)}
                          </td>
                          <td className="py-3 pr-3 text-right tabular-nums text-amber-700 dark:text-amber-400">
                            -{formatBookingClp(financial.hostFeeClp)}
                            {financial.marketplaceFeeClp >
                              financial.hostFeeClp && (
                              <span className="block text-[10px] text-muted-foreground">
                                Split{" "}
                                {formatBookingClp(financial.marketplaceFeeClp)}
                              </span>
                            )}
                          </td>
                          <td className="py-3 pr-3 text-right tabular-nums text-sky-800 dark:text-sky-300">
                            {processing ? (
                              <>
                                {formatBookingClp(processing.amountClp)}
                                <span className="block text-[10px] text-muted-foreground">
                                  {processingRateFormatter.format(
                                    processing.amountClp /
                                      processing.chargedClp,
                                  )}
                                </span>
                              </>
                            ) : (
                              <span className="text-muted-foreground">
                                Sin datos
                              </span>
                            )}
                          </td>
                          <td className="py-3 pr-3 text-right font-medium tabular-nums text-foreground">
                            {movement.originalChargeClp === undefined
                              ? "Sin registro"
                              : formatBookingClp(movement.originalChargeClp)}
                          </td>
                          <td className="py-3 pr-3 text-right tabular-nums">
                            {movement.confirmedRefundClp !== undefined && (
                              <span className="block font-semibold text-foreground">
                                {formatBookingClp(movement.confirmedRefundClp)}
                              </span>
                            )}
                            <span
                              className={
                                movement.adjustment === "chargeback"
                                  ? "inline-block rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[11px] font-medium text-rose-700 dark:text-rose-300"
                                  : "text-muted-foreground"
                              }
                            >
                              {FINANCE_ADJUSTMENT_LABEL[movement.adjustment]}
                            </span>
                            {movement.adjustment === "refund_confirmed" &&
                              movement.confirmedRefundClp === undefined && (
                                <span className="block text-muted-foreground">
                                  Monto por confirmar
                                </span>
                              )}
                          </td>
                          <td className="py-3 pr-3 text-right font-bold tabular-nums">
                            {movement.reconciledNetClp === undefined ? (
                              <span className="text-muted-foreground">
                                Pendiente de conciliar
                              </span>
                            ) : (
                              <span className="text-emerald-700 dark:text-emerald-400">
                                {formatBookingClp(movement.reconciledNetClp)}
                              </span>
                            )}
                          </td>
                          <td className="py-3 text-right">
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() =>
                                setInvoiceReservation({
                                  ...tx,
                                  participant_role: "anfitrion",
                                })
                              }
                              className="h-7 px-2.5 text-xs"
                            >
                              <FileText className="mr-1 h-3 w-3" />
                              Invoice
                            </Button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      )}

      {/* SECCIÓN 3: LISTA DE RESERVAS */}
      {activeSection === "lista" && (
        <div className="space-y-6">
          {spacesDirectory.length > 0 && (
            <div className="space-y-3 rounded-2xl border border-border bg-card p-4 shadow-xs">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  <Building2 className="h-3.5 w-3.5 text-primary" />
                  Selecciona un espacio para ver su lista de reservas
                </p>
                {selectedSpaceId !== "all" && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => handleSpaceChange("all", true)}
                    className="h-7 text-xs"
                  >
                    Ver todos los espacios ({spacesDirectory.length})
                  </Button>
                )}
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {spacesDirectory.map((sp) => {
                  const isSelected = selectedSpaceId === sp.spaceId;
                  return (
                    <button
                      key={sp.spaceId}
                      type="button"
                      onClick={() =>
                        handleSpaceChange(isSelected ? "all" : sp.spaceId, true)
                      }
                      className={`rounded-xl border p-3.5 text-left transition-all ${
                        isSelected
                          ? "border-primary bg-primary/5 shadow-xs ring-1 ring-primary"
                          : "border-border bg-background hover:border-primary/40"
                      }`}
                    >
                      <p className="line-clamp-1 text-sm font-semibold text-foreground">
                        {sp.spaceTitle}
                      </p>
                      <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
                        ID: {sp.spaceId}
                      </p>
                      <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px]">
                        <span className="rounded-full bg-emerald-500/10 px-2 py-0.5 font-medium text-emerald-700 dark:text-emerald-400">
                          {sp.approved} aprobadas
                        </span>
                        <span className="rounded-full bg-amber-500/10 px-2 py-0.5 font-medium text-amber-700 dark:text-amber-400">
                          {sp.pending} pend.
                        </span>
                        <span className="text-muted-foreground">
                          {sp.total} total
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {/* Filter and Search Controls */}
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <nav
              aria-label="Filtro por estado de reserva"
              className="flex flex-wrap gap-1.5"
            >
              {filterTabs.map((tab) => {
                const isSelected = activeTab === tab.id;
                return (
                  <button
                    key={tab.id}
                    type="button"
                    onClick={() => setActiveTab(tab.id)}
                    aria-current={isSelected ? "true" : undefined}
                    className={`rounded-full px-3.5 py-1.5 text-xs font-medium transition-colors ${
                      isSelected
                        ? "bg-primary text-primary-foreground shadow-sm"
                        : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                  >
                    {tab.label}
                  </button>
                );
              })}
            </nav>

            <div className="relative w-full sm:w-64">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                type="search"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Buscar ID de reserva..."
                className="w-full rounded-lg border border-input bg-background py-1.5 pl-9 pr-3 text-sm shadow-sm placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
          </div>

          {/* List Content */}
          {loading && reservations.length === 0 ? (
            <div className="space-y-4">
              {[1, 2, 3].map((n) => (
                <div
                  key={n}
                  className="h-32 animate-pulse rounded-2xl border border-border bg-card/50 p-6"
                />
              ))}
            </div>
          ) : filteredReservations.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border p-12 text-center">
              <Calendar className="mx-auto h-12 w-12 text-muted-foreground/50" />
              <h2 className="mt-4 text-base font-medium text-foreground">
                No se encontraron reservas
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {searchQuery || activeTab !== "todas"
                  ? "No hay reservas que coincidan con los filtros seleccionados."
                  : "Aún no has recibido solicitudes de reserva para tus espacios publicados."}
              </p>
            </div>
          ) : (
            <>
              <div
                className="space-y-4"
                role="feed"
                aria-label="Listado de reservas"
              >
                {paginatedReservations.map((reservation) => {
                  const config = getBookingStateConfig(reservation.state);
                  const Icon = config.icon;
                  const financial =
                    getReservationFinancialBreakdown(reservation);
                  const movement = getReservationFinancialMovement(
                    reservation,
                    financial,
                  );
                  const canViewInvoice =
                    isReservationPaid(reservation) ||
                    movement.adjustment !== "none";
                  const netPending =
                    movement.reconciledNetClp === undefined &&
                    (movement.adjustment !== "none" ||
                      reservation.provider_settlement?.reconciliation ===
                        "discrepancy");

                  return (
                    <article
                      key={reservation.id}
                      className="group relative rounded-2xl border border-border bg-card p-5 transition-shadow hover:shadow-md sm:p-6"
                    >
                      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                        <div className="space-y-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <span
                              className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium ${config.badgeClass}`}
                            >
                              <Icon className="h-3.5 w-3.5" />
                              {config.label}
                            </span>
                            {reservation.unread_count > 0 && (
                              <span className="rounded-full bg-primary px-2 py-0.5 text-[10px] font-semibold text-primary-foreground">
                                {reservation.unread_count} nuevo
                                {reservation.unread_count > 1 ? "s" : ""}
                              </span>
                            )}
                            <span className="font-mono text-xs text-muted-foreground">
                              ID Reserva: {reservation.id}
                            </span>
                            {reservation.provider_payment_id && (
                              <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 font-mono text-xs font-medium text-emerald-800 dark:text-emerald-300">
                                <Receipt className="h-3 w-3" />
                                Operación MP #{reservation.provider_payment_id}
                              </span>
                            )}
                          </div>

                          <div className="flex flex-wrap items-baseline gap-2">
                            <h2 className="text-lg font-semibold tracking-tight text-foreground">
                              <Link
                                href={`${detailBaseHref}/${encodeURIComponent(reservation.id)}`}
                                className="underline-offset-4 hover:underline"
                              >
                                {reservation.space_title
                                  ? reservation.space_title
                                  : `Espacio: ${reservation.space_id.slice(0, 8)}...`}
                              </Link>
                            </h2>
                            <span className="text-sm text-muted-foreground">
                              · {reservation.units} {reservation.rate_unit}
                              {reservation.units > 1 ? "s" : ""}
                            </span>
                            {reservation.checkout_status && (
                              <span className="text-xs text-muted-foreground">
                                · Checkout:{" "}
                                {reservation.checkout_method === "payments_api"
                                  ? `Tarjeta (${reservation.checkout_status})`
                                  : `Checkout Pro (${reservation.checkout_status})`}
                              </span>
                            )}
                          </div>

                          <p className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground">
                            <Building2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                            <span>Espacio ID: {reservation.space_id}</span>
                          </p>

                          <ReservationScheduleBreakdown
                            reservation={reservation}
                            compact
                            allowHostControls
                          />
                        </div>

                        <div className="flex flex-row items-center justify-between gap-4 sm:flex-col sm:items-end">
                          <div className="text-left sm:text-right">
                            <p className="text-xs text-muted-foreground">
                              Tarifa Base
                            </p>
                            <p className="text-lg font-bold text-foreground">
                              {formatBookingClp(financial.baseClp)} CLP
                            </p>
                            {canViewInvoice && netPending && (
                              <p className="text-xs font-medium text-muted-foreground">
                                Neto pendiente de conciliar
                              </p>
                            )}
                            {canViewInvoice && !netPending && (
                              <p className="text-xs font-medium text-emerald-700 dark:text-emerald-400">
                                {reservation.is_commercial &&
                                financial.confirmedNetHostClp === undefined
                                  ? "Importe antes de tarifa Mercado Pago:"
                                  : "Neto anfitrión:"}{" "}
                                {formatBookingClp(financial.netHostClp)} CLP
                              </p>
                            )}
                          </div>

                          <div className="flex flex-wrap items-center gap-2">
                            {canViewInvoice && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                  setInvoiceReservation({
                                    ...reservation,
                                    participant_role: "anfitrion",
                                  })
                                }
                                className="inline-flex items-center gap-1.5"
                              >
                                <FileText className="h-3.5 w-3.5" />
                                Invoice
                              </Button>
                            )}
                            <Button
                              asChild
                              size="sm"
                              className="inline-flex items-center gap-1.5"
                            >
                              <Link
                                href={`${detailBaseHref}/${encodeURIComponent(reservation.id)}`}
                              >
                                Ver reserva
                                <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
                              </Link>
                            </Button>
                          </div>
                        </div>
                      </div>
                    </article>
                  );
                })}
              </div>
              {totalPages > 1 && (
                <div className="flex items-center justify-between border-t border-border pt-3 text-sm">
                  <span className="text-xs tabular-nums text-muted-foreground">
                    Página {currentPage} de {totalPages}
                  </span>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={currentPage <= 1}
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                    >
                      Anterior
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={currentPage >= totalPages}
                      onClick={() =>
                        setPage((p) => Math.min(totalPages, p + 1))
                      }
                    >
                      Siguiente
                    </Button>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}

      <ReservationInvoiceModal
        reservation={invoiceReservation}
        onClose={() => setInvoiceReservation(null)}
      />
      <HostReceptionModal
        open={receptionOpen}
        spaceId={selectedSpaceId !== "all" ? selectedSpaceId : undefined}
        onClose={() => setReceptionOpen(false)}
        onUpdated={() => {
          void loadReservations();
        }}
      />
    </div>
  );
}
