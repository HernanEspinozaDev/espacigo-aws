"use client";

import { useEffect, useState, type FormEvent } from "react";
import { AlertCircle, Building2, CheckCircle2, Clock3, FileCheck2, Loader2, RefreshCw, ShieldCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isValidRut, normalizeRut } from "@/features/verifications/rut-utils";
import { isAbortError } from "@/lib/api/errors";
import { KYBOpenApiAdapter } from "../infrastructure/kyb-openapi-adapter";
import { kybStatusLabel, kybStatusTone, type KYBVerification } from "../domain/kyb";

const adapter = new KYBOpenApiAdapter();
const allowedTypes = new Set(["application/pdf", "image/jpeg", "image/png"]);
const maxBytes = 10 * 1024 * 1024;

function StatusBadge({ verification }: { verification: KYBVerification }) {
  const tone = kybStatusTone(verification.status);
  const icons = {
    pending: Clock3,
    review: Clock3,
    approved: CheckCircle2,
    rejected: AlertCircle,
  };
  const Icon = icons[tone];
  const classes = {
    pending: "border-amber-300 bg-amber-50 text-amber-900",
    review: "border-sky-300 bg-sky-50 text-sky-900",
    approved: "border-emerald-300 bg-emerald-50 text-emerald-900",
    rejected: "border-red-300 bg-red-50 text-red-900",
  };
  return (
    <Badge variant="outline" className={`gap-1.5 ${classes[tone]}`}>
      <Icon className="size-3.5" aria-hidden="true" />
      {kybStatusLabel(verification.status)}
    </Badge>
  );
}

export function KYBOwnerView() {
  const { snapshot: auth, getAccessToken } = useAuth();
  const [verification, setVerification] = useState<KYBVerification | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const [legalName, setLegalName] = useState("");
  const [companyRut, setCompanyRut] = useState("");
  const [representativeName, setRepresentativeName] = useState("");
  const [representativeRut, setRepresentativeRut] = useState("");
  const [siiGiroCode, setSiiGiroCode] = useState("");
  const [erut, setErut] = useState<File | null>(null);
  const [powerCertificate, setPowerCertificate] = useState<File | null>(null);
  const [propertyAuthorization, setPropertyAuthorization] = useState<File | null>(null);

  useEffect(() => {
    if (auth.status !== "authenticated") {
      setLoading(false);
      return;
    }
    const token = getAccessToken();
    if (!token) {
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    void adapter.getOwn(token, controller.signal).then((value) => {
      setVerification(value);
      setError(null);
    }).catch((cause) => {
      if (!isAbortError(cause)) setError("No fue posible consultar el estado KYB.");
    }).finally(() => setLoading(false));
    return () => controller.abort();
  }, [auth.status, auth.accountId, auth.roles, getAccessToken, reloadKey]);

  if (auth.status !== "authenticated") {
    return <Card><CardContent className="py-10 text-sm text-muted-foreground">Inicia sesión para completar la verificación empresarial.</CardContent></Card>;
  }
  if (!auth.roles.includes("arrendador")) {
    return <Card><CardContent className="py-10 text-sm text-muted-foreground">Esta sección está disponible para cuentas arrendadoras.</CardContent></Card>;
  }

  function validateFile(file: File | null): string | null {
    if (!file) return "Debes adjuntar este documento.";
    if (!allowedTypes.has(file.type)) return "Solo se aceptan PDF, JPG o PNG.";
    if (file.size < 1 || file.size > maxBytes) return "El archivo debe tener entre 1 byte y 10 MiB.";
    return null;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (!isValidRut(companyRut)) {
      setError("Ingresa un RUT empresarial válido.");
      return;
    }
    if (!isValidRut(representativeRut)) {
      setError("Ingresa un RUT representante válido.");
      return;
    }
    if (!/^\d{6}$/.test(siiGiroCode)) {
      setError("El código de giro SII debe tener 6 dígitos.");
      return;
    }
    for (const file of [erut, powerCertificate, propertyAuthorization]) {
      const fileError = validateFile(file);
      if (fileError) {
        setError(fileError);
        return;
      }
    }
    const token = getAccessToken();
    if (!token) {
      setError("Tu sesión expiró. Inicia sesión nuevamente.");
      return;
    }
    setSubmitting(true);
    try {
      const created = await adapter.submit(token, {
        legalName: legalName.trim(),
        companyRut: normalizeRut(companyRut),
        representativeName: representativeName.trim(),
        representativeRut: normalizeRut(representativeRut),
        siiGiroCode,
        erut: erut as File,
        powerCertificate: powerCertificate as File,
        propertyOrSubleaseAuthorization: propertyAuthorization as File,
      }, new AbortController().signal);
      setVerification(created);
    } catch (cause) {
      if (!isAbortError(cause)) setError("No fue posible enviar la solicitud KYB.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <Building2 className="size-5 text-primary" aria-hidden="true" />
              <CardTitle>Verificación empresarial KYB</CardTitle>
            </div>
            {verification ? <StatusBadge verification={verification} /> : null}
          </div>
          <CardDescription>
            Valida tu empresa arrendadora antes de publicar espacios comerciales B2B.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />Consultando estado…</p>
          ) : verification && verification.status !== "REJECTED" ? (
            <div className="rounded-lg border border-border bg-muted/30 p-4 text-sm">
              <p className="font-medium">{kybStatusLabel(verification.status)}</p>
              <p className="mt-1 text-muted-foreground">
                Empresa: {verification.legal_name}. Giro SII: {verification.sii_giro_code}.
              </p>
            </div>
          ) : (
            <form onSubmit={submit} className="grid gap-5">
              {verification?.status === "REJECTED" ? (
                <div role="alert" className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900">
                  <p className="font-semibold">Solicitud rechazada</p>
                  {verification.rejection_reason ? <p className="mt-1">Motivo: {verification.rejection_reason}</p> : null}
                </div>
              ) : null}
              {error ? <div role="alert" className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"><AlertCircle className="mt-0.5 size-4 shrink-0" />{error}</div> : null}
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="kyb-legal-name">Razón social</Label><Input id="kyb-legal-name" value={legalName} onChange={(event) => setLegalName(event.target.value)} required maxLength={200} /></div>
                <div className="space-y-2"><Label htmlFor="kyb-company-rut">RUT empresa</Label><Input id="kyb-company-rut" value={companyRut} onChange={(event) => setCompanyRut(event.target.value)} placeholder="76.123.456-0" required /></div>
                <div className="space-y-2"><Label htmlFor="kyb-representative-name">Representante legal</Label><Input id="kyb-representative-name" value={representativeName} onChange={(event) => setRepresentativeName(event.target.value)} required maxLength={200} /></div>
                <div className="space-y-2"><Label htmlFor="kyb-representative-rut">RUT representante</Label><Input id="kyb-representative-rut" value={representativeRut} onChange={(event) => setRepresentativeRut(event.target.value)} placeholder="12.345.678-5" required /></div>
                <div className="space-y-2 sm:col-span-2"><Label htmlFor="kyb-giro">Código de giro SII</Label><Input id="kyb-giro" value={siiGiroCode} onChange={(event) => setSiiGiroCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder="620100" required /></div>
              </div>
              <div className="grid gap-4 sm:grid-cols-3">
                <FileInput id="kyb-erut" label="e-RUT" file={erut} onChange={setErut} />
                <FileInput id="kyb-powers" label="Vigencia de Poderes" file={powerCertificate} onChange={setPowerCertificate} />
                <FileInput id="kyb-domain" label="Dominio Vigente o subarriendo" file={propertyAuthorization} onChange={setPropertyAuthorization} />
              </div>
              <div className="flex justify-end"><Button type="submit" disabled={submitting}>{submitting ? <Loader2 className="mr-2 size-4 animate-spin" /> : <ShieldCheck className="mr-2 size-4" />}Enviar solicitud KYB</Button></div>
            </form>
          )}
          {verification && error ? <div role="alert" className="text-sm text-destructive">{error}</div> : null}
          {verification ? <Button type="button" variant="outline" size="sm" onClick={() => setReloadKey((value) => value + 1)}><RefreshCw className="mr-2 size-4" />Actualizar estado</Button> : null}
        </CardContent>
      </Card>
    </div>
  );
}

function FileInput({ id, label, file, onChange }: { id: string; label: string; file: File | null; onChange: (file: File | null) => void }) {
  return (
    <div className="space-y-2 rounded-lg border p-3">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} type="file" accept="application/pdf,image/jpeg,image/png" onChange={(event) => onChange(event.target.files?.[0] ?? null)} />
      {file ? <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><FileCheck2 className="size-3.5" />{file.name}</p> : null}
    </div>
  );
}
