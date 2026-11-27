"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Eye,
  Loader2,
  RefreshCw,
  ShieldCheck,
  X,
  XCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isAbortError } from "@/lib/api/errors";
import {
  KYBOpenApiAdapter,
  type KYBComplianceDocument,
  type KYBComplianceProfile,
} from "../infrastructure/kyb-openapi-adapter";
import {
  kybStatusLabel,
  type KYBDocumentDecision,
  type KYBDocument,
  type KYBVerification,
} from "../domain/kyb";

const adapter = new KYBOpenApiAdapter();
const documentLabels: Record<KYBDocument["kind"], string> = {
  erut: "e-RUT",
  sii_start_activities: "Certificado de Inicio de Actividades del SII",
  power_certificate: "Certificado de Vigencia de Poderes",
  property_or_sublease_authorization:
    "Dominio Vigente o autorización de subarriendo",
};
function complianceStatusLabel(
  status: KYBComplianceProfile["compliance_status"],
) {
  return (
    {
      grace_active: "En gracia / por vencer",
      in_review: "Documentación en revisión",
      compliant: "Habilitada",
      disabled_missing_docs: "Inhabilitada por falta de documentación",
    } as const
  )[status];
}
function graceDaysRemaining(dueAt?: string) {
  if (!dueAt) return null;
  return Math.max(0, Math.ceil((Date.parse(dueAt) - Date.now()) / 86_400_000));
}
function initialDocumentDecisions(
  documents: KYBDocument[] = [],
): Record<
  string,
  { decision?: "APPROVE" | "REJECT"; rejection_reason: string }
> {
  return Object.fromEntries(
    documents.map((document) => [
      document.id,
      {
        decision:
          document.review_status === "APPROVED"
            ? "APPROVE"
            : document.review_status === "REJECTED"
              ? "REJECT"
              : undefined,
        rejection_reason: document.rejection_reason ?? "",
      },
    ]),
  );
}
function documentReviewStatusLabel(status?: KYBDocument["review_status"]) {
  return (
    {
      PENDING: "Pendiente",
      IN_REVIEW: "En revisión",
      APPROVED: "Aprobado",
      REJECTED: "Rechazado",
    } as const
  )[status ?? "PENDING"];
}

export function KYBAdminView() {
  const { snapshot: auth, getAccessToken } = useAuth();
  const [items, setItems] = useState<KYBVerification[]>([]);
  const [selected, setSelected] = useState<KYBVerification | null>(null);
  const [resolvedKey, setResolvedKey] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [decision, setDecision] = useState<"APPROVE" | "REJECT" | null>(null);
  const [reviewMode, setReviewMode] = useState<"documents" | "packet">(
    "documents",
  );
  const [documentDecisions, setDocumentDecisions] = useState<
    Record<
      string,
      { decision?: "APPROVE" | "REJECT"; rejection_reason: string }
    >
  >({});
  const [reason, setReason] = useState("");
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const previewUrlRef = useRef<string | null>(null);
  const mountedRef = useRef(false);
  const [previewName, setPreviewName] = useState("");
  const [compliance, setCompliance] = useState<KYBComplianceProfile[]>([]);
  const [complianceFilter, setComplianceFilter] = useState("");
  const [complianceBusy, setComplianceBusy] = useState<string | null>(null);
  const [complianceDocuments, setComplianceDocuments] = useState<
    KYBComplianceDocument[]
  >([]);
  const [selectedComplianceDocument, setSelectedComplianceDocument] =
    useState<KYBComplianceDocument | null>(null);
  const [complianceDecision, setComplianceDecision] = useState<
    "APPROVE" | "REJECT" | null
  >(null);

  function clearPreviewUrl() {
    if (previewUrlRef.current?.startsWith("blob:")) {
      URL.revokeObjectURL(previewUrlRef.current);
    }
    previewUrlRef.current = null;
    setPreviewUrl(null);
  }

  function showPreviewBlob(blob: Blob) {
    if (!mountedRef.current) return;
    if (previewUrlRef.current?.startsWith("blob:")) {
      URL.revokeObjectURL(previewUrlRef.current);
    }
    const url = URL.createObjectURL(blob);
    previewUrlRef.current = url;
    setPreviewUrl(url);
  }

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (previewUrlRef.current?.startsWith("blob:")) {
        URL.revokeObjectURL(previewUrlRef.current);
        previewUrlRef.current = null;
      }
    };
  }, []);

  const activeToken = auth.status === "authenticated" ? getAccessToken() : null;
  const fetchKey =
    auth.status === "authenticated" && activeToken
      ? `${auth.accountId ?? ""}:${auth.roles.join(",")}`
      : null;
  const loading = refreshing || (fetchKey !== null && resolvedKey !== fetchKey);

  async function loadList(signal?: AbortSignal) {
    const token = getAccessToken();
    if (!token) return;
    setRefreshing(true);
    setError(null);
    try {
      const value = await adapter.listPending(
        token,
        signal ?? new AbortController().signal,
      );
      setItems(value);
      setCompliance(
        await adapter.listCompliance(
          token,
          complianceFilter,
          signal ?? new AbortController().signal,
        ),
      );
      setComplianceDocuments(
        await adapter.listPendingComplianceDocuments(
          token,
          signal ?? new AbortController().signal,
        ),
      );
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible cargar las solicitudes KYB.");
    } finally {
      setRefreshing(false);
    }
  }

  async function reviewComplianceDocument(item: KYBComplianceDocument) {
    const token = getAccessToken();
    if (!token) return;
    setWorking(true);
    setError(null);
    try {
      if (item.status === "PENDING")
        await adapter.startComplianceDocumentReview(
          token,
          item.id,
          new AbortController().signal,
        );
      const updated = { ...item, status: "IN_REVIEW" as const };
      setSelectedComplianceDocument(updated);
      setComplianceDocuments(
        await adapter.listPendingComplianceDocuments(
          token,
          new AbortController().signal,
        ),
      );
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible iniciar la revisión del documento.");
    } finally {
      setWorking(false);
    }
  }

  async function previewComplianceDocument(item: KYBComplianceDocument) {
    const token = getAccessToken();
    if (!token) return;
    setWorking(true);
    setError(null);
    try {
      const link = await adapter.complianceDocumentURL(
        token,
        item.id,
        new AbortController().signal,
      );
      const response = await fetch(
        `/api/backend${link.download_url.replace(/^\/api\/v1/, "")}`,
      );
      if (!response.ok) throw new Error("document_fetch_failed");
      const blob = await response.blob();
      showPreviewBlob(blob);
      setPreviewName(
        item.document_kind === "sii_start_activities"
          ? item.space_id
            ? "Certificado SII por categoría"
            : "Certificado SII personal"
          : "Respaldo de inmueble",
      );
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible abrir el documento privado.");
    } finally {
      setWorking(false);
    }
  }

  async function decideComplianceDocument() {
    const token = getAccessToken();
    if (!token || !selectedComplianceDocument || !complianceDecision) return;
    const rejectionReason =
      complianceDecision === "REJECT"
        ? (window.prompt("Motivo del rechazo (mínimo 5 caracteres):")?.trim() ??
          "")
        : "";
    if (complianceDecision === "REJECT" && rejectionReason.length < 5) return;
    setWorking(true);
    setError(null);
    try {
      await adapter.decideComplianceDocument(
        token,
        selectedComplianceDocument.id,
        complianceDecision,
        rejectionReason,
        new AbortController().signal,
      );
      setSelectedComplianceDocument(null);
      setComplianceDecision(null);
      clearPreviewUrl();
      setComplianceDocuments(
        await adapter.listPendingComplianceDocuments(
          token,
          new AbortController().signal,
        ),
      );
      setCompliance(
        await adapter.listCompliance(
          token,
          complianceFilter,
          new AbortController().signal,
        ),
      );
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible guardar la resolución documental.");
    } finally {
      setWorking(false);
    }
  }

  async function changeCompliance(item: KYBComplianceProfile) {
    const token = getAccessToken();
    if (!token) return;
    const disabled = item.compliance_status !== "disabled_missing_docs";
    const reason = disabled
      ? (window
          .prompt("Motivo para inhabilitar este perfil (mínimo 5 caracteres):")
          ?.trim() ?? "")
      : "";
    if (disabled && reason.length < 5) return;
    const id = item.organization_id ?? item.owner_usuario_id;
    setComplianceBusy(id);
    setError(null);
    try {
      await adapter.setCompliance(
        token,
        item,
        disabled,
        reason,
        new AbortController().signal,
      );
      setCompliance(
        await adapter.listCompliance(
          token,
          complianceFilter,
          new AbortController().signal,
        ),
      );
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible actualizar el estado de cumplimiento.");
    } finally {
      setComplianceBusy(null);
    }
  }

  useEffect(() => {
    if (!fetchKey || !activeToken) return;
    const controller = new AbortController();
    let active = true;
    void Promise.all([
      adapter.listPending(activeToken, controller.signal),
      adapter.listCompliance(activeToken, complianceFilter, controller.signal),
      adapter.listPendingComplianceDocuments(activeToken, controller.signal),
    ])
      .then(([value, profiles, documents]) => {
        if (!active) return;
        setItems(value);
        setCompliance(profiles);
        setComplianceDocuments(documents);
        setError(null);
      })
      .catch((cause) => {
        if (active && !isAbortError(cause)) {
          setError("No fue posible cargar las solicitudes KYB.");
        }
      })
      .finally(() => {
        if (active) setResolvedKey(fetchKey);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [fetchKey, activeToken, complianceFilter]);

  async function selectItem(item: KYBVerification) {
    const token = getAccessToken();
    if (!token) return;
    setWorking(true);
    setError(null);
    clearPreviewUrl();
    try {
      const detail = await adapter.getForReview(
        token,
        item.id,
        new AbortController().signal,
      );
      setSelected(detail);
      setReviewMode("documents");
      setDecision(null);
      setReason("");
      setDocumentDecisions(initialDocumentDecisions(detail.documents ?? []));
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible cargar el detalle KYB.");
    } finally {
      setWorking(false);
    }
  }

  async function startReview() {
    const token = getAccessToken();
    if (!token || !selected) return;
    setWorking(true);
    setError(null);
    try {
      const updated = await adapter.startReview(
        token,
        selected.id,
        new AbortController().signal,
      );
      setSelected(updated);
      setReviewMode("documents");
      setDocumentDecisions(initialDocumentDecisions(updated.documents ?? []));
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible iniciar la revisión.");
    } finally {
      setWorking(false);
    }
  }

  async function preview(document: KYBDocument) {
    const token = getAccessToken();
    if (!token || !selected) return;
    setWorking(true);
    setError(null);
    try {
      const result = await adapter.getDocumentReviewURL(
        token,
        selected.id,
        document.id,
        new AbortController().signal,
      );
      const signedPath = `/api/backend${result.download_url.replace(/^\/api\/v1/, "")}`;
      const pdfResponse = await fetch(signedPath);
      if (!pdfResponse.ok) throw new Error("pdf_fetch_failed");
      const blob = await pdfResponse.blob();
      showPreviewBlob(blob);
      setPreviewName(documentLabels[document.kind]);
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible emitir el enlace temporal del documento.");
    } finally {
      setWorking(false);
    }
  }

  async function submitDecision() {
    const token = getAccessToken();
    if (!token || !selected || !decision) return;
    if (
      decision === "REJECT" &&
      (reason.trim().length < 5 || reason.trim().length > 500)
    ) {
      setError("El motivo de rechazo debe tener entre 5 y 500 caracteres.");
      return;
    }
    setWorking(true);
    setError(null);
    try {
      const updated = await adapter.decide(
        token,
        selected.id,
        decision,
        reason.trim(),
        new AbortController().signal,
      );
      setSelected(updated);
      setDocumentDecisions(initialDocumentDecisions(updated.documents ?? []));
      setDecision(null);
      setReason("");
      clearPreviewUrl();
      await loadList();
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible registrar la resolución KYB.");
    } finally {
      setWorking(false);
    }
  }

  function chooseDocumentDecision(
    documentId: string,
    value: "APPROVE" | "REJECT",
  ) {
    setDocumentDecisions((current) => ({
      ...current,
      [documentId]: {
        decision: value,
        rejection_reason:
          value === "REJECT"
            ? (current[documentId]?.rejection_reason ?? "")
            : "",
      },
    }));
  }

  async function submitDocumentDecisions() {
    const token = getAccessToken();
    if (!token || !selected) return;
    const documents = selected.documents ?? [];
    if (documents.length === 0) {
      setError("Este expediente no incluye documentos revisables.");
      return;
    }
    const missing = documents.some(
      (document) => !documentDecisions[document.id]?.decision,
    );
    if (missing) {
      setError(
        "Selecciona aprobar o rechazar cada documento antes de continuar.",
      );
      return;
    }
    const invalidReason = documents.some((document) => {
      const row = documentDecisions[document.id];
      return (
        row?.decision === "REJECT" &&
        (row.rejection_reason.trim().length < 5 ||
          row.rejection_reason.trim().length > 500)
      );
    });
    if (invalidReason) {
      setError("Cada rechazo necesita un motivo de entre 5 y 500 caracteres.");
      return;
    }
    const decisions: KYBDocumentDecision[] = documents.map((document) => {
      const row = documentDecisions[document.id];
      return {
        document_id: document.id,
        decision: row.decision as "APPROVE" | "REJECT",
        ...(row.decision === "REJECT"
          ? { rejection_reason: row.rejection_reason.trim() }
          : {}),
      };
    });
    setWorking(true);
    setError(null);
    try {
      const updated = await adapter.decide(
        token,
        selected.id,
        null,
        "",
        new AbortController().signal,
        decisions,
      );
      setSelected(updated);
      setDocumentDecisions(initialDocumentDecisions(updated.documents ?? []));
      clearPreviewUrl();
      await loadList();
    } catch (cause) {
      if (!isAbortError(cause))
        setError("No fue posible guardar las decisiones por documento.");
    } finally {
      setWorking(false);
    }
  }

  if (auth.status !== "authenticated") {
    return (
      <Card>
        <CardContent className="py-10 text-sm text-muted-foreground">
          Inicia sesión con una cuenta administrativa.
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Solicitudes KYB</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Revisión documental de empresas arrendadoras.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void loadList()}
          disabled={loading || working}
        >
          <RefreshCw
            className={`mr-2 size-4 ${loading ? "animate-spin" : ""}`}
          />
          Actualizar
        </Button>
      </div>

      {error ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" />
          {error}
        </div>
      ) : null}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle className="text-base">
            Cuentas y empresas por cumplimiento
          </CardTitle>
          <select
            aria-label="Filtrar cumplimiento"
            value={complianceFilter}
            onChange={(event) => setComplianceFilter(event.target.value)}
            className="h-9 rounded-md border bg-background px-2 text-sm"
          >
            <option value="">Todas</option>
            <option value="grace_active">En gracia / por vencer</option>
            <option value="in_review">En revisión</option>
            <option value="compliant">Habilitadas</option>
            <option value="disabled_missing_docs">
              Inhabilitadas por falta de documentación
            </option>
          </select>
        </CardHeader>
        <CardContent className="space-y-2">
          {compliance.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No hay perfiles para este filtro.
            </p>
          ) : (
            compliance.map((item) => {
              const id = item.organization_id ?? item.owner_usuario_id;
              const label = item.organization_id
                ? item.legal_name || "Empresa"
                : "Persona natural";
              return (
                <div
                  key={`${item.organization_id ?? "personal"}:${id}`}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"
                >
                  <div>
                    <p className="font-medium">{label}</p>
                    <p className="text-xs text-muted-foreground">
                      {complianceStatusLabel(item.compliance_status)} · SII{" "}
                      {item.sii_status ?? "sin documento"}
                      {item.grace_due_at
                        ? ` · vence ${new Date(item.grace_due_at).toLocaleDateString("es-CL")}${item.compliance_status === "grace_active" || item.compliance_status === "in_review" ? ` (${graceDaysRemaining(item.grace_due_at)} días)` : ""}`
                        : ""}
                    </p>
                    {item.disabled_reason ? (
                      <p className="mt-1 text-xs text-red-700">
                        Motivo: {item.disabled_reason}
                      </p>
                    ) : null}
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant={
                      item.compliance_status === "disabled_missing_docs"
                        ? "outline"
                        : "destructive"
                    }
                    onClick={() => void changeCompliance(item)}
                    disabled={complianceBusy === id}
                  >
                    {complianceBusy === id ? (
                      <Loader2 className="mr-2 size-4 animate-spin" />
                    ) : null}
                    {item.compliance_status === "disabled_missing_docs"
                      ? "Rehabilitar"
                      : "Inhabilitar"}
                  </Button>
                </div>
              );
            })
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            Certificados SII y respaldos de inmueble
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {complianceDocuments.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No hay documentos pendientes de revisión.
            </p>
          ) : (
            complianceDocuments.map((doc) => {
              const owner = compliance.find((profile) =>
                doc.organization_id
                  ? profile.organization_id === doc.organization_id
                  : profile.owner_usuario_id === doc.owner_usuario_id,
              );
              const isCategorySii =
                doc.document_kind === "sii_start_activities" &&
                Boolean(doc.space_id);
              const isPersonalSii =
                doc.document_kind === "sii_start_activities" && !doc.space_id;
              const documentTitle = isCategorySii
                ? "Certificado SII por categoría"
                : isPersonalSii
                  ? "Certificado SII personal"
                  : "Respaldo de inmueble";
              const contextParts = [
                doc.organization_id
                  ? `Empresa · ${owner?.legal_name || doc.organization_id}`
                  : !doc.space_id
                    ? "Persona natural"
                    : null,
                doc.space_id ? `Espacio ${doc.space_id}` : null,
              ].filter(Boolean);
              const detailParts = [
                isCategorySii && doc.category_code
                  ? `Categoría ${doc.category_code}`
                  : null,
                doc.sii_giro_code ? `Giro ${doc.sii_giro_code}` : null,
                kybStatusLabel(doc.status),
                new Date(doc.submitted_at).toLocaleDateString("es-CL"),
              ].filter(Boolean);

              return (
                <div
                  key={doc.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3"
                >
                  <div>
                    <p className="text-xs text-muted-foreground">
                      {contextParts.join(" · ")}
                    </p>
                    <p className="font-medium">{documentTitle}</p>
                    <p className="text-xs text-muted-foreground">
                      {detailParts.join(" · ")}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => void previewComplianceDocument(doc)}
                    >
                      <Eye className="mr-2 size-4" />
                      Abrir
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      onClick={() => void reviewComplianceDocument(doc)}
                      disabled={working}
                    >
                      {doc.status === "PENDING"
                        ? "Iniciar revisión"
                        : "Revisar"}
                    </Button>
                  </div>
                </div>
              );
            })
          )}
          {selectedComplianceDocument?.status === "IN_REVIEW" ? (
            <div className="space-y-3 border-t pt-3">
              {previewUrl ? (
                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-semibold">
                      Vista previa: {previewName}
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={clearPreviewUrl}
                    >
                      <X className="mr-1 size-4" />
                      Cerrar vista previa
                    </Button>
                  </div>
                  <iframe
                    title={previewName}
                    src={previewUrl}
                    className="h-[360px] w-full rounded-lg border bg-muted/20"
                  />
                </div>
              ) : null}
              <div className="flex gap-2">
                <Button
                  type="button"
                  onClick={() => setComplianceDecision("APPROVE")}
                >
                  <CheckCircle2 className="mr-2 size-4" />
                  Aprobar documento
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setComplianceDecision("REJECT")}
                >
                  <XCircle className="mr-2 size-4" />
                  Rechazar documento
                </Button>
              </div>
              {complianceDecision ? (
                <Button
                  type="button"
                  onClick={() => void decideComplianceDocument()}
                  disabled={working}
                >
                  Confirmar{" "}
                  {complianceDecision === "APPROVE" ? "aprobación" : "rechazo"}
                </Button>
              ) : null}
            </div>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-[minmax(260px,0.7fr)_minmax(0,1.3fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Cola pendiente</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Consultando…
              </p>
            ) : null}
            {!loading && items.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay solicitudes pendientes.
              </p>
            ) : null}
            {items.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => void selectItem(item)}
                className={`w-full rounded-lg border p-3 text-left transition-colors ${selected?.id === item.id ? "border-primary bg-primary/5" : "hover:bg-muted/40"}`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-semibold">{item.legal_name}</span>
                  <Badge variant="outline">{kybStatusLabel(item.status)}</Badge>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  Giro {item.sii_giro_code} ·{" "}
                  {new Date(item.submitted_at).toLocaleDateString("es-CL")}
                </p>
              </button>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Detalle de revisión</CardTitle>
          </CardHeader>
          <CardContent>
            {!selected ? (
              <p className="text-sm text-muted-foreground">
                Selecciona una solicitud para revisar su información.
              </p>
            ) : (
              <div className="space-y-5">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Info label="Razón social" value={selected.legal_name} />
                  <Info
                    label="RUT empresa"
                    value={selected.company_rut ?? "No disponible"}
                  />
                  <Info
                    label="Representante"
                    value={selected.representative_name}
                  />
                  <Info
                    label="RUT representante"
                    value={selected.representative_rut ?? "No disponible"}
                  />
                  <Info label="Giro SII" value={selected.sii_giro_code} />
                  <Info
                    label="Estado"
                    value={kybStatusLabel(selected.status)}
                  />
                </div>

                <div className="space-y-2">
                  <p className="text-sm font-semibold">Documentos</p>
                  <div className="space-y-3">
                    {(selected.documents ?? []).map((document) => {
                      const row = documentDecisions[document.id];
                      return (
                        <div
                          key={document.id}
                          className="space-y-3 rounded-lg border p-3"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-3">
                            <div>
                              <p className="font-medium">
                                {documentLabels[document.kind]}
                              </p>
                              <p className="text-xs text-muted-foreground">
                                {document.size_bytes.toLocaleString("es-CL")}{" "}
                                bytes · {document.mime_type}
                              </p>
                            </div>
                            <div className="flex items-center gap-2">
                              <Badge variant="outline">
                                {documentReviewStatusLabel(
                                  document.review_status,
                                )}
                              </Badge>
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => void preview(document)}
                              >
                                <Eye className="mr-2 size-4" />
                                Abrir documento
                              </Button>
                            </div>
                          </div>
                          {document.reviewed_at ? (
                            <p className="text-xs text-muted-foreground">
                              Revisado{" "}
                              {new Date(document.reviewed_at).toLocaleString(
                                "es-CL",
                              )}
                            </p>
                          ) : null}
                          {document.review_status === "REJECTED" &&
                          document.rejection_reason ? (
                            <p className="text-sm text-red-700">
                              Motivo anterior: {document.rejection_reason}
                            </p>
                          ) : null}
                          {selected.status === "IN_REVIEW" &&
                          reviewMode === "documents" ? (
                            <div className="space-y-2 border-t pt-3">
                              <div className="flex flex-wrap gap-2">
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={
                                    row?.decision === "APPROVE"
                                      ? "default"
                                      : "outline"
                                  }
                                  disabled={working}
                                  onClick={() =>
                                    chooseDocumentDecision(
                                      document.id,
                                      "APPROVE",
                                    )
                                  }
                                >
                                  <CheckCircle2 className="mr-2 size-4" />
                                  Aprobar este documento
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={
                                    row?.decision === "REJECT"
                                      ? "destructive"
                                      : "outline"
                                  }
                                  disabled={working}
                                  onClick={() =>
                                    chooseDocumentDecision(
                                      document.id,
                                      "REJECT",
                                    )
                                  }
                                >
                                  <XCircle className="mr-2 size-4" />
                                  Rechazar este documento
                                </Button>
                              </div>
                              {row?.decision === "REJECT" ? (
                                <div className="space-y-2">
                                  <Label
                                    htmlFor={`document-reason-${document.id}`}
                                  >
                                    Motivo del rechazo (5 a 500 caracteres)
                                  </Label>
                                  <textarea
                                    id={`document-reason-${document.id}`}
                                    value={row.rejection_reason}
                                    onChange={(event) =>
                                      setDocumentDecisions((current) => ({
                                        ...current,
                                        [document.id]: {
                                          ...current[document.id],
                                          decision: "REJECT",
                                          rejection_reason: event.target.value,
                                        },
                                      }))
                                    }
                                    minLength={5}
                                    maxLength={500}
                                    className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                  />
                                </div>
                              ) : null}
                            </div>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                </div>

                {previewUrl ? (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-semibold">
                        Vista previa: {previewName}
                      </p>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={clearPreviewUrl}
                      >
                        <X className="mr-1 size-4" />
                        Cerrar vista previa
                      </Button>
                    </div>
                    <iframe
                      title={previewName}
                      src={previewUrl}
                      className="h-[420px] w-full rounded-lg border bg-muted/20"
                    />
                  </div>
                ) : null}

                {selected.status === "PENDING" ? (
                  <Button
                    type="button"
                    onClick={() => void startReview()}
                    disabled={working}
                  >
                    <ClockIcon />
                    Pasar a revisión
                  </Button>
                ) : null}

                {selected.status === "IN_REVIEW" ? (
                  <div className="space-y-3 border-t pt-4">
                    <div>
                      <p className="text-sm font-semibold">
                        Tipo de resolución
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Puedes resolver cada documento por separado o aplicar
                        una misma decisión al expediente completo.
                      </p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant={
                          reviewMode === "documents" ? "default" : "outline"
                        }
                        onClick={() => {
                          setReviewMode("documents");
                          setDecision(null);
                        }}
                        disabled={working}
                      >
                        Decidir por documento
                      </Button>
                      <Button
                        type="button"
                        variant={
                          reviewMode === "packet" ? "default" : "outline"
                        }
                        onClick={() => {
                          setReviewMode("packet");
                          setDocumentDecisions(
                            initialDocumentDecisions(selected.documents ?? []),
                          );
                        }}
                        disabled={working}
                      >
                        Resolver expediente completo
                      </Button>
                    </div>
                    {reviewMode === "documents" ? (
                      <div className="space-y-3">
                        <p className="text-sm text-muted-foreground">
                          Se requiere una decisión explícita para cada
                          documento. Rechaza cada archivo con su propio motivo.
                        </p>
                        <Button
                          type="button"
                          onClick={() => void submitDocumentDecisions()}
                          disabled={
                            working ||
                            !(selected.documents ?? []).length ||
                            !(selected.documents ?? []).every((document) =>
                              Boolean(documentDecisions[document.id]?.decision),
                            )
                          }
                        >
                          {working ? (
                            <Loader2 className="mr-2 size-4 animate-spin" />
                          ) : (
                            <ShieldCheck className="mr-2 size-4" />
                          )}
                          Confirmar decisiones por documento
                        </Button>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        <div className="flex flex-wrap gap-2">
                          <Button
                            type="button"
                            variant={
                              decision === "APPROVE" ? "default" : "outline"
                            }
                            onClick={() => setDecision("APPROVE")}
                            disabled={working}
                          >
                            <CheckCircle2 className="mr-2 size-4" />
                            Aprobar todo el expediente
                          </Button>
                          <Button
                            type="button"
                            variant={
                              decision === "REJECT" ? "destructive" : "outline"
                            }
                            onClick={() => setDecision("REJECT")}
                            disabled={working}
                          >
                            <XCircle className="mr-2 size-4" />
                            Rechazar todo el expediente
                          </Button>
                        </div>
                        {decision === "REJECT" ? (
                          <div className="space-y-2">
                            <Label htmlFor="kyb-rejection">
                              Motivo general (5 a 500 caracteres)
                            </Label>
                            <textarea
                              id="kyb-rejection"
                              value={reason}
                              onChange={(event) =>
                                setReason(event.target.value)
                              }
                              minLength={5}
                              maxLength={500}
                              className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            />
                          </div>
                        ) : null}
                        {decision ? (
                          <Button
                            type="button"
                            onClick={() => void submitDecision()}
                            disabled={
                              working ||
                              (decision === "REJECT" &&
                                (reason.trim().length < 5 ||
                                  reason.trim().length > 500))
                            }
                          >
                            {working ? (
                              <Loader2 className="mr-2 size-4 animate-spin" />
                            ) : (
                              <ShieldCheck className="mr-2 size-4" />
                            )}
                            Confirmar decisión para todos los documentos
                          </Button>
                        ) : null}
                      </div>
                    )}
                  </div>
                ) : null}

                {selected.status === "APPROVED" ||
                selected.status === "REJECTED" ? (
                  <Badge variant="outline" className="gap-1.5">
                    <CheckCircle2 className="size-3.5" />
                    {kybStatusLabel(selected.status)}
                  </Badge>
                ) : null}
                {selected.status === "REJECTED" && selected.rejection_reason ? (
                  <p className="text-sm text-red-700">
                    Motivo: {selected.rejection_reason}
                  </p>
                ) : null}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

function ClockIcon() {
  return <Loader2 className="mr-2 size-4" aria-hidden="true" />;
}
