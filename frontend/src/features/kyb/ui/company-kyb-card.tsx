"use client";

import { useEffect, useId, useState, type FormEvent } from "react";
import {
  AlertCircle,
  Building2,
  CheckCircle2,
  Clock3,
  FileCheck2,
  Loader2,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isValidRut, normalizeRut } from "@/features/verifications/rut-utils";
import { isAbortError } from "@/lib/api/errors";
import { KYBOpenApiAdapter } from "../infrastructure/kyb-openapi-adapter";
import {
  kybStatusLabel,
  kybStatusTone,
  type KYBVerification,
} from "../domain/kyb";

const adapter = new KYBOpenApiAdapter();
const allowedTypes = new Set(["application/pdf", "image/jpeg", "image/png"]);
const maxBytes = 10 * 1024 * 1024;

export type CompanyKYBOrganization = {
  id: string;
  legal_name?: string;
  trade_name?: string;
  razon_social?: string;
  my_role?: string;
  compliance_status?: string;
  grace_due_at?: string;
};

function remainingDays(dueAt?: string) {
  return dueAt
    ? Math.max(0, Math.ceil((Date.parse(dueAt) - Date.now()) / 86_400_000))
    : null;
}

function complianceLabel(status: string) {
  return (
    (
      {
        grace_active: "En plazo de gracia",
        in_review: "Documentación en revisión",
        compliant: "Habilitada",
        disabled_missing_docs: "Inhabilitada por falta de documentación",
      } as Record<string, string>
    )[status] ?? status
  );
}

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

function FileInput({
  id,
  label,
  file,
  onChange,
}: {
  id: string;
  label: string;
  file: File | null;
  onChange: (file: File | null) => void;
}) {
  return (
    <div className="space-y-2 rounded-lg border p-3">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="file"
        accept="application/pdf,image/jpeg,image/png"
        onChange={(event) => onChange(event.target.files?.[0] ?? null)}
      />
      {file ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <FileCheck2 className="size-3.5" />
          {file.name}
        </p>
      ) : null}
    </div>
  );
}

function validateFile(file: File | null): string | null {
  if (!file) return "Debes adjuntar este documento.";
  if (!allowedTypes.has(file.type)) return "Solo se aceptan PDF, JPG o PNG.";
  if (file.size < 1 || file.size > maxBytes)
    return "El archivo debe tener entre 1 byte y 10 MiB.";
  return null;
}

export function CompanyKYBCard({
  organization,
  onVerifiedSubmitted,
  variant = "card",
}: {
  organization: CompanyKYBOrganization;
  onVerifiedSubmitted?: () => void;
  variant?: "card" | "embedded";
}) {
  return (
    <CompanyKYBCardContent
      key={organization.id}
      organization={organization}
      onVerifiedSubmitted={onVerifiedSubmitted}
      variant={variant}
    />
  );
}

function CompanyKYBCardContent({
  organization,
  onVerifiedSubmitted,
  variant,
}: {
  organization: CompanyKYBOrganization;
  onVerifiedSubmitted?: () => void;
  variant: "card" | "embedded";
}) {
  const fieldId = useId();
  const { snapshot: auth, getAccessToken } = useAuth();
  const [verification, setVerification] = useState<KYBVerification | null>(
    null,
  );
  const [resolvedKey, setResolvedKey] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const initialLegalName =
    organization.legal_name || organization.razon_social || "";
  const [legalName, setLegalName] = useState(initialLegalName);
  const [companyRut, setCompanyRut] = useState("");
  const [representativeName, setRepresentativeName] = useState("");
  const [representativeRut, setRepresentativeRut] = useState("");
  const [siiGiroCode, setSiiGiroCode] = useState("");
  const [erut, setErut] = useState<File | null>(null);
  const [siiCertificate, setSiiCertificate] = useState<File | null>(null);
  const [powerCertificate, setPowerCertificate] = useState<File | null>(null);

  const token = auth.status === "authenticated" ? getAccessToken() : null;
  const fetchKey =
    auth.status === "authenticated" &&
    token &&
    (organization.my_role === "representante_legal" ||
      organization.my_role === "administrador")
      ? `${auth.accountId ?? ""}:${auth.roles.join(",")}:${organization.id}:${reloadKey}`
      : null;
  const loading = fetchKey !== null && resolvedKey !== fetchKey;

  useEffect(() => {
    if (!fetchKey || !token) return;
    const controller = new AbortController();
    let active = true;
    void adapter
      .getOwn(token, controller.signal, organization.id)
      .then((value) => {
        if (!active) return;
        setVerification(value);
        setError(null);
      })
      .catch((cause) => {
        if (active && !isAbortError(cause)) {
          setError("No fue posible consultar el estado KYB.");
        }
      })
      .finally(() => {
        if (active) setResolvedKey(fetchKey);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [fetchKey, token, organization.id]);

  if (auth.status !== "authenticated") {
    return (
      <Card>
        <CardContent className="py-10 text-sm text-muted-foreground">
          Inicia sesión para completar la verificación empresarial.
        </CardContent>
      </Card>
    );
  }

  const companyDaysRemaining = remainingDays(organization.grace_due_at);
  const companyDisabled =
    organization.compliance_status === "disabled_missing_docs";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setNotice(null);
    if (!legalName.trim()) {
      setError("Ingresa la razón social de la empresa.");
      return;
    }
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
    for (const file of [erut, siiCertificate, powerCertificate]) {
      const fileError = validateFile(file);
      if (fileError) {
        setError(fileError);
        return;
      }
    }
    const activeToken = getAccessToken();
    if (!activeToken) {
      setError("Tu sesión expiró. Inicia sesión nuevamente.");
      return;
    }
    setSubmitting(true);
    try {
      const created = await adapter.submit(
        activeToken,
        {
          organizationId: organization.id,
          legalName: legalName.trim(),
          companyRut: normalizeRut(companyRut),
          representativeName: representativeName.trim(),
          representativeRut: normalizeRut(representativeRut),
          siiGiroCode,
          erut: erut as File,
          siiStartActivities: siiCertificate as File,
          powerCertificate: powerCertificate as File,
        },
        new AbortController().signal,
      );
      setVerification(created);
      setNotice("Solicitud de verificación empresarial KYB enviada.");
      onVerifiedSubmitted?.();
    } catch (cause) {
      if (!isAbortError(cause)) {
        setError("No fue posible enviar la solicitud KYB.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  const body = (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Building2 className="size-4 text-primary" aria-hidden="true" />
          <h3 className="text-base font-semibold">
            Verificación empresarial (KYB)
          </h3>
        </div>
        {verification ? <StatusBadge verification={verification} /> : null}
      </div>

      <p className="text-sm text-muted-foreground">
        Valida a{" "}
        <strong className="text-foreground">
          {organization.trade_name ||
            organization.legal_name ||
            organization.razon_social ||
            "tu empresa"}
        </strong>{" "}
        con e-RUT, Certificado de Inicio de Actividades del SII y Vigencia de
        Poderes. Al aprobarse la verificación empresarial junto con el respaldo
        de cada inmueble, tanto la empresa como el espacio aparecen verificados.
      </p>

      {organization.compliance_status ? (
        <div
          className={`rounded-lg border p-3 text-sm ${
            companyDisabled
              ? "border-red-200 bg-red-50 text-red-900"
              : organization.compliance_status === "compliant"
                ? "border-emerald-200 bg-emerald-50 text-emerald-900"
                : organization.compliance_status === "in_review"
                  ? "border-sky-200 bg-sky-50 text-sky-900"
                  : companyDaysRemaining !== null
                    ? "border-amber-200 bg-amber-50 text-amber-950"
                    : "border-border bg-muted/30"
          }`}
        >
          <p className="font-medium">
            Estado comercial: {complianceLabel(organization.compliance_status)}
          </p>
          {companyDisabled ? (
            <p className="mt-1">
              Las publicaciones y administración comercial de esta empresa están
              inhabilitadas hasta regularizar la documentación. Puedes continuar
              enviando sus documentos.
            </p>
          ) : organization.compliance_status === "compliant" ? (
            <p className="mt-1">
              La empresa cuenta con verificación empresarial KYB aprobada.
            </p>
          ) : companyDaysRemaining !== null ? (
            <p className="mt-1">
              Plazo compartido de la empresa: quedan {companyDaysRemaining} días
              para completar los documentos pendientes.
            </p>
          ) : null}
        </div>
      ) : null}

      {notice ? (
        <p
          role="status"
          className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900"
        >
          {notice}
        </p>
      ) : null}

      {loading ? (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Consultando estado KYB…
        </p>
      ) : verification &&
        verification.status !== "REJECTED" &&
        !companyDisabled ? (
        <div
          className={`rounded-lg border p-4 text-sm ${
            verification.status === "APPROVED"
              ? "border-emerald-200 bg-emerald-50 text-emerald-900"
              : "border-border bg-muted/30"
          }`}
        >
          <p className="font-medium">{kybStatusLabel(verification.status)}</p>
          <p
            className={`mt-1 ${
              verification.status === "APPROVED"
                ? "text-emerald-800"
                : "text-muted-foreground"
            }`}
          >
            Empresa: {verification.legal_name}. Giro SII:{" "}
            {verification.sii_giro_code}.
          </p>
        </div>
      ) : organization.compliance_status ===
        "compliant" ? null : organization.my_role !== "representante_legal" ? (
        <p className="text-sm text-muted-foreground">
          La carpeta de la empresa la presenta un representante legal.
        </p>
      ) : (
        <form onSubmit={submit} className="grid gap-5">
          {verification?.status === "REJECTED" ? (
            <div
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900"
            >
              <p className="font-semibold">Solicitud rechazada</p>
              {verification.rejection_reason ? (
                <p className="mt-1">Motivo: {verification.rejection_reason}</p>
              ) : null}
            </div>
          ) : null}
          {error ? (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
            >
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              {error}
            </div>
          ) : null}
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor={`${fieldId}-legal-name`}>Razón social</Label>
              <Input
                id={`${fieldId}-legal-name`}
                value={legalName}
                onChange={(event) => setLegalName(event.target.value)}
                required
                maxLength={200}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${fieldId}-company-rut`}>RUT empresa</Label>
              <Input
                id={`${fieldId}-company-rut`}
                value={companyRut}
                onChange={(event) => setCompanyRut(event.target.value)}
                placeholder="76.123.456-0"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${fieldId}-rep-name`}>Representante legal</Label>
              <Input
                id={`${fieldId}-rep-name`}
                value={representativeName}
                onChange={(event) => setRepresentativeName(event.target.value)}
                required
                maxLength={200}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${fieldId}-rep-rut`}>RUT representante</Label>
              <Input
                id={`${fieldId}-rep-rut`}
                value={representativeRut}
                onChange={(event) => setRepresentativeRut(event.target.value)}
                placeholder="12.345.678-5"
                required
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor={`${fieldId}-giro`}>Giro SII (6 dígitos)</Label>
              <Input
                id={`${fieldId}-giro`}
                value={siiGiroCode}
                onChange={(event) =>
                  setSiiGiroCode(
                    event.target.value.replace(/\D/g, "").slice(0, 6),
                  )
                }
                placeholder="620100"
                inputMode="numeric"
                required
              />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <FileInput
              id={`${fieldId}-erut`}
              label="e-RUT"
              file={erut}
              onChange={setErut}
            />
            <FileInput
              id={`${fieldId}-sii`}
              label="Certificado de Inicio de Actividades del SII"
              file={siiCertificate}
              onChange={setSiiCertificate}
            />
            <FileInput
              id={`${fieldId}-powers`}
              label="Vigencia de Poderes"
              file={powerCertificate}
              onChange={setPowerCertificate}
            />
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={submitting}>
              {submitting ? (
                <Loader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <ShieldCheck className="mr-2 size-4" />
              )}
              Enviar verificación KYB
            </Button>
          </div>
        </form>
      )}

      {verification && error ? (
        <div role="alert" className="text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {verification ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setReloadKey((value) => value + 1)}
        >
          <RefreshCw className="mr-2 size-4" />
          Actualizar estado
        </Button>
      ) : null}
    </div>
  );

  if (variant === "embedded") {
    return (
      <div className="space-y-4 rounded-lg border border-border/80 bg-muted/15 p-4">
        {body}
      </div>
    );
  }

  return (
    <Card>
      <CardHeader className="sr-only">
        <CardTitle>Verificación empresarial (KYB)</CardTitle>
        <CardDescription>
          Carpeta empresarial KYB de la organización seleccionada.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-6">{body}</CardContent>
    </Card>
  );
}
