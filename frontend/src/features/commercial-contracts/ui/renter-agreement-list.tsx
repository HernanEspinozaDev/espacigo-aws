"use client";
import Link from "next/link";
import { FileText } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { useAuth } from "@/features/auth/ui/use-auth";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import {
  agreementStateLabel,
  type AgreementRead,
  type CommercialAgreement,
  type ContractProposal,
} from "../domain/agreements";
import { agreementErrorMessage } from "./renter-agreement-panel";
import { contractDate } from "./contract-proposal-terms";
import { ContractPdfModal } from "./contract-pdf-modal";
import {
  ContractListToolbar,
  ContractPagination,
  agreementBadgeClass,
  matchesAgreementFilter,
  type AgreementFilter,
} from "./contract-list-controls";
import { cn } from "@/lib/utils";

export function RenterAgreementList() {
  const { accessToken, snapshot, getAccessToken } = useAuth();
  if (!accessToken || snapshot.restrictedMode)
    return <p>Inicia sesión para consultar tus solicitudes de contrato.</p>;
  return (
    <AgreementList
      key={`${accessToken}:${snapshot.accountId}`}
      isCurrent={() => getAccessToken() === accessToken}
    />
  );
}
function AgreementList({ isCurrent }: { isCurrent(): boolean }) {
  const [items, setItems] = useState<CommercialAgreement[] | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [filter, setFilter] = useState<AgreementFilter>("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [pdfDetail, setPdfDetail] = useState<{
    read: AgreementRead;
    proposal: ContractProposal;
  } | null>(null);
  const [pdfOpen, setPdfOpen] = useState(false);
  const [loadingPdf, setLoadingPdf] = useState(false);

  useEffect(() => {
    let active = true;
    commercialAgreements
      .list("renter")
      .then((loaded) => {
        if (active && isCurrent()) {
          setItems(loaded);
          setError("");
        }
      })
      .catch((e: unknown) => {
        if (active && isCurrent()) setError(agreementErrorMessage(e));
      });
    return () => {
      active = false;
    };
  }, [attempt, isCurrent]);

  async function openPdf(agreementId: string) {
    setLoadingPdf(true);
    setPdfOpen(true);
    setPdfDetail(null);
    setError("");
    try {
      const detail = await commercialAgreements.resume(agreementId);
      if (isCurrent()) {
        setPdfDetail({ read: detail, proposal: detail.proposal });
      }
    } catch (e: unknown) {
      if (isCurrent()) {
        setError(agreementErrorMessage(e));
        setPdfOpen(false);
      }
    } finally {
      if (isCurrent()) setLoadingPdf(false);
    }
  }

  const allItems = items ?? [];
  const counts = {
    all: allItems.length,
    pending: allItems.filter((i) => i.state === "pending_landlord").length,
    ready: allItems.filter((i) => i.state === "ready_for_payment").length,
    closed: allItems.filter((i) =>
      ["rejected", "withdrawn", "expired"].includes(i.state),
    ).length,
  };
  const query = search.trim().toLowerCase();
  const filtered = allItems.filter((item) => {
    if (!matchesAgreementFilter(item.state, filter)) return false;
    if (!query) return true;
    return (
      item.id.toLowerCase().includes(query) ||
      agreementStateLabel(item.state).toLowerCase().includes(query)
    );
  });
  const pageSize = 10;
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageItems = filtered.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>Solicitudes enviadas (Arrendatario)</CardTitle>
        <CardDescription>
          Consulta el estado de tus firmas, descarga el PDF del contrato y
          retoma el pago cuando el arrendador acepte.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <ContractListToolbar
          filter={filter}
          onFilterChange={(next) => {
            setFilter(next);
            setPage(1);
          }}
          search={search}
          onSearchChange={(next) => {
            setSearch(next);
            setPage(1);
          }}
          counts={counts}
          busy={!items || loadingPdf}
          onRefresh={() => setAttempt((v) => v + 1)}
        />
        {!items && !error && <p role="status">Cargando solicitudes…</p>}
        {items?.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Aún no enviaste solicitudes de contrato. Elige un espacio y revisa
            sus condiciones antes de firmar.
          </p>
        )}
        {items && items.length > 0 && filtered.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No hay solicitudes que coincidan con el filtro seleccionado.
          </p>
        )}
        <ul className="divide-y divide-border rounded-lg border border-border">
          {pageItems.map((a) => {
            const renterSigned = a.signatures.some(
              (s) => s.role === "arrendatario",
            );
            const landlordSigned = a.signatures.some(
              (s) => s.role === "arrendador",
            );
            return (
              <li
                key={a.id}
                className="flex flex-wrap items-center justify-between gap-3 p-3.5 hover:bg-muted/20"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-sm font-medium">
                      Solicitud {a.id.slice(0, 8)}
                    </p>
                    <span
                      className={cn(
                        "inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium",
                        agreementBadgeClass(a.state),
                      )}
                    >
                      {agreementStateLabel(a.state)}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Enviada {contractDate(a.created_at, "America/Santiago")} ·
                    Arrendatario: {renterSigned ? "Firmado" : "Pendiente"} ·
                    Arrendador: {landlordSigned ? "Firmado" : "Pendiente"}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={loadingPdf}
                    onClick={() => void openPdf(a.id)}
                  >
                    <FileText className="size-4" aria-hidden="true" />
                    <span>Ver PDF</span>
                  </Button>
                  <Button asChild size="sm" variant={a.state === "ready_for_payment" ? "default" : "outline"}>
                    <Link
                      href={`/reservas/contratos/${encodeURIComponent(a.id)}`}
                    >
                      {a.state === "ready_for_payment"
                        ? "Ir a pagar"
                        : "Ver detalle"}
                    </Link>
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
        <ContractPagination
          page={currentPage}
          totalPages={totalPages}
          onPageChange={setPage}
        />
        {pdfOpen && loadingPdf && (
          <p role="status" className="text-sm text-muted-foreground">
            Cargando documento PDF…
          </p>
        )}
        <ContractPdfModal
          open={pdfOpen && Boolean(pdfDetail)}
          onClose={() => setPdfOpen(false)}
          proposal={pdfDetail?.proposal ?? null}
          agreement={pdfDetail?.read.agreement}
          title="Contrato de arriendo (Arrendatario)"
          footer={
            pdfDetail ? (
              <div className="flex flex-wrap items-center justify-end gap-2">
                {pdfDetail.read.agreement.state === "ready_for_payment" && (
                  <Button asChild>
                    <Link
                      href={`/reservas/contratos/${encodeURIComponent(pdfDetail.read.agreement.id)}`}
                    >
                      Continuar al pago
                    </Link>
                  </Button>
                )}
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setPdfOpen(false)}
                >
                  Cerrar
                </Button>
              </div>
            ) : undefined
          }
        />
      </CardContent>
    </Card>
  );
}
