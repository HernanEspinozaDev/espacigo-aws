"use client";

import { useEffect, useState } from "react";
import { AlertCircle, CheckCircle2, Eye, Loader2, RefreshCw, ShieldCheck, XCircle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/workspace/ui/card";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isAbortError } from "@/lib/api/errors";
import { KYBOpenApiAdapter } from "../infrastructure/kyb-openapi-adapter";
import { kybStatusLabel, type KYBDocument, type KYBVerification } from "../domain/kyb";

const adapter = new KYBOpenApiAdapter();
const documentLabels: Record<KYBDocument["kind"], string> = {
  erut: "e-RUT",
  power_certificate: "Certificado de Vigencia de Poderes",
  property_or_sublease_authorization: "Dominio Vigente o autorización de subarriendo",
};

export function KYBAdminView() {
  const { snapshot: auth, getAccessToken } = useAuth();
  const [items, setItems] = useState<KYBVerification[]>([]);
  const [selected, setSelected] = useState<KYBVerification | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [decision, setDecision] = useState<"APPROVE" | "REJECT" | null>(null);
  const [reason, setReason] = useState("");
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [previewName, setPreviewName] = useState("");

  async function loadList(signal?: AbortSignal) {
    const token = getAccessToken();
    if (!token) return;
    setLoading(true);
    setError(null);
    try {
      const value = await adapter.listPending(token, signal ?? new AbortController().signal);
      setItems(value);
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible cargar las solicitudes KYB.");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (auth.status !== "authenticated") {
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    void loadList(controller.signal);
    return () => controller.abort();
  }, [auth.status, auth.accountId, auth.roles, getAccessToken]);

  async function selectItem(item: KYBVerification) {
    const token = getAccessToken();
    if (!token) return;
    setWorking(true);
    setError(null);
    setPreviewUrl(null);
    try {
      const detail = await adapter.getForReview(token, item.id, new AbortController().signal);
      setSelected(detail);
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible cargar el detalle KYB.");
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
      const updated = await adapter.startReview(token, selected.id, new AbortController().signal);
      setSelected(updated);
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
      const result = await adapter.getDocumentReviewURL(token, selected.id, document.id, new AbortController().signal);
      const signedPath = `/api/backend${result.download_url.replace(/^\/api\/v1/, "")}`;
      const pdfResponse = await fetch(signedPath);
      if (!pdfResponse.ok) throw new Error("pdf_fetch_failed");
      const blob = await pdfResponse.blob();
      if (previewUrl?.startsWith("blob:")) URL.revokeObjectURL(previewUrl);
      setPreviewUrl(URL.createObjectURL(blob));
      setPreviewName(documentLabels[document.kind]);
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible emitir el enlace temporal del documento.");
    } finally {
      setWorking(false);
    }
  }

  async function submitDecision() {
    const token = getAccessToken();
    if (!token || !selected || !decision) return;
    if (decision === "REJECT" && (reason.trim().length < 5 || reason.trim().length > 500)) {
      setError("El motivo de rechazo debe tener entre 5 y 500 caracteres.");
      return;
    }
    setWorking(true);
    setError(null);
    try {
      const updated = await adapter.decide(token, selected.id, decision, reason.trim(), new AbortController().signal);
      setSelected(updated);
      setDecision(null);
      setReason("");
      setPreviewUrl(null);
      await loadList();
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible registrar la resolución KYB.");
    } finally {
      setWorking(false);
    }
  }

  if (auth.status !== "authenticated") {
    return <Card><CardContent className="py-10 text-sm text-muted-foreground">Inicia sesión con una cuenta administrativa.</CardContent></Card>;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Solicitudes KYB</h2>
          <p className="mt-1 text-sm text-muted-foreground">Revisión documental de empresas arrendadoras.</p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => void loadList()} disabled={loading || working}><RefreshCw className={`mr-2 size-4 ${loading ? "animate-spin" : ""}`} />Actualizar</Button>
      </div>

      {error ? <div role="alert" className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"><AlertCircle className="mt-0.5 size-4 shrink-0" />{error}</div> : null}

      <div className="grid gap-4 lg:grid-cols-[minmax(260px,0.7fr)_minmax(0,1.3fr)]">
        <Card>
          <CardHeader><CardTitle className="text-base">Cola pendiente</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {loading ? <p className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />Consultando…</p> : null}
            {!loading && items.length === 0 ? <p className="text-sm text-muted-foreground">No hay solicitudes pendientes.</p> : null}
            {items.map((item) => (
              <button key={item.id} type="button" onClick={() => void selectItem(item)} className={`w-full rounded-lg border p-3 text-left transition-colors ${selected?.id === item.id ? "border-primary bg-primary/5" : "hover:bg-muted/40"}`}>
                <div className="flex items-center justify-between gap-2"><span className="font-semibold">{item.legal_name}</span><Badge variant="outline">{kybStatusLabel(item.status)}</Badge></div>
                <p className="mt-1 text-xs text-muted-foreground">Giro {item.sii_giro_code} · {new Date(item.submitted_at).toLocaleDateString("es-CL")}</p>
              </button>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-base">Detalle de revisión</CardTitle></CardHeader>
          <CardContent>
            {!selected ? <p className="text-sm text-muted-foreground">Selecciona una solicitud para revisar su información.</p> : (
              <div className="space-y-5">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Info label="Razón social" value={selected.legal_name} />
                  <Info label="RUT empresa" value={selected.company_rut ?? "No disponible"} />
                  <Info label="Representante" value={selected.representative_name} />
                  <Info label="RUT representante" value={selected.representative_rut ?? "No disponible"} />
                  <Info label="Giro SII" value={selected.sii_giro_code} />
                  <Info label="Estado" value={kybStatusLabel(selected.status)} />
                </div>

                <div className="space-y-2">
                  <p className="text-sm font-semibold">Documentos</p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {(selected.documents ?? []).map((document) => (
                      <Button key={document.id} type="button" variant="outline" className="justify-between" onClick={() => void preview(document)}>
                        <span className="truncate text-xs">{documentLabels[document.kind]}</span><Eye className="size-4 shrink-0" />
                      </Button>
                    ))}
                  </div>
                </div>

                {previewUrl ? (
                  <div className="space-y-2">
                    <p className="text-sm font-semibold">Vista previa: {previewName}</p>
                    <iframe title={previewName} src={previewUrl} className="h-[420px] w-full rounded-lg border bg-muted/20" />
                  </div>
                ) : null}

                {selected.status === "PENDING" ? (
                  <Button type="button" onClick={() => void startReview()} disabled={working}><ClockIcon />Pasar a revisión</Button>
                ) : null}

                {selected.status === "IN_REVIEW" ? (
                  <div className="space-y-3 border-t pt-4">
                    <div className="flex flex-wrap gap-2">
                      <Button type="button" variant="default" onClick={() => setDecision("APPROVE")} disabled={working}><CheckCircle2 className="mr-2 size-4" />Aprobar</Button>
                      <Button type="button" variant="outline" onClick={() => setDecision("REJECT")} disabled={working}><XCircle className="mr-2 size-4" />Rechazar</Button>
                    </div>
                    {decision === "REJECT" ? (
                      <div className="space-y-2">
                        <Label htmlFor="kyb-rejection">Motivo de rechazo</Label>
                        <textarea id="kyb-rejection" value={reason} onChange={(event) => setReason(event.target.value)} minLength={5} maxLength={500} className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring" />
                      </div>
                    ) : null}
                    {decision ? <Button type="button" onClick={() => void submitDecision()} disabled={working}>{working ? <Loader2 className="mr-2 size-4 animate-spin" /> : <ShieldCheck className="mr-2 size-4" />}Confirmar {decision === "APPROVE" ? "aprobación" : "rechazo"}</Button> : null}
                  </div>
                ) : null}

                {selected.status === "APPROVED" || selected.status === "REJECTED" ? <Badge variant="outline" className="gap-1.5"><CheckCircle2 className="size-3.5" />{kybStatusLabel(selected.status)}</Badge> : null}
                {selected.status === "REJECTED" && selected.rejection_reason ? <p className="text-sm text-red-700">Motivo: {selected.rejection_reason}</p> : null}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return <div className="rounded-lg border p-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 break-words text-sm font-medium">{value}</p></div>;
}

function ClockIcon() {
  return <Loader2 className="mr-2 size-4" aria-hidden="true" />;
}
