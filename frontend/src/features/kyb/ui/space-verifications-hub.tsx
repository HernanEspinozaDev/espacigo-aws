"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  Building2,
  CheckCircle2,
  Clock3,
  FileCheck2,
  Loader2,
  Plus,
  User,
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
import {
  CompanyKYBCard,
  type CompanyKYBOrganization,
} from "./company-kyb-card";

type PersonalCompliance = {
  compliance_status: string;
  sii_giro_code?: string;
  sii_status?: string;
  grace_started_at?: string;
  grace_due_at?: string;
  disabled_at?: string;
  disabled_reason?: string;
};

type SpaceItem = {
  id: string;
  title: string;
  state?: string;
  status?: string;
  organization_id?: string;
};

type SpaceEligibility = {
  can_publish?: boolean;
  can_book?: boolean;
  space_verified?: boolean;
  compliance_status?: string;
  verification_status?: string;
  grace_due_at?: string;
  property_approved?: boolean;
  property_status?: string;
  property_rejection_reason?: string;
  company_verified?: boolean;
  requires_category_sii?: boolean;
  category_sii_approved?: boolean;
  category_sii_status?: string;
  category_sii_rejection_reason?: string;
  category_code?: string;
  category_name?: string;
};

const allowedTypes = new Set(["application/pdf", "image/jpeg", "image/png"]);
const maxBytes = 10 * 1024 * 1024;
const kybApi = "/api/backend/kyb";

async function jsonRequest<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw Object.assign(
      new Error("No fue posible consultar la documentación."),
      { status: response.status },
    );
  }
  return data as T;
}

function fileError(file: File | null) {
  if (!file) return "Adjunta un documento para continuar.";
  if (!allowedTypes.has(file.type)) return "Solo se aceptan PDF, JPG o PNG.";
  if (file.size < 1 || file.size > maxBytes)
    return "El archivo debe tener entre 1 byte y 10 MiB.";
  return null;
}

function remainingDays(dueAt?: string) {
  if (!dueAt) return null;
  return Math.max(0, Math.ceil((Date.parse(dueAt) - Date.now()) / 86_400_000));
}

function spaceStateLabel(space: SpaceItem): string {
  const raw = space.state || space.status || "borrador";
  if (raw === "activa" || raw === "published") return "Publicada";
  if (raw === "oculta" || raw === "hidden") return "Oculta";
  if (raw === "borrador" || raw === "draft") return "Borrador";
  return raw;
}

function verificationStatusLabel(status?: string): string {
  return (
    (
      {
        grace_active: "En plazo de gracia",
        in_review: "En revisión",
        compliant: "Documentación aprobada",
        disabled_missing_docs: "Inhabilitada por falta de documentación",
        pending: "Pendiente",
        PENDING: "Pendiente de revisión",
        IN_REVIEW: "En revisión administrativa",
        APPROVED: "Aprobado",
        REJECTED: "Requiere correcciones",
        approved: "Aprobada",
        rejected: "Requiere correcciones",
      } as Record<string, string>
    )[status || ""] ||
    status ||
    "Pendiente de revisión"
  );
}

function documentStatusLabel(status?: string): string {
  if (!status) return "Sin enviar";
  return verificationStatusLabel(status);
}

export function SpaceVerificationsHub() {
  const { snapshot: auth, getAccessToken } = useAuth();
  const token = auth.status === "authenticated" ? getAccessToken() : null;

  const [spaces, setSpaces] = useState<SpaceItem[]>([]);
  const [organizations, setOrganizations] = useState<CompanyKYBOrganization[]>(
    [],
  );
  const [personalCompliance, setPersonalCompliance] =
    useState<PersonalCompliance | null>(null);
  const [eligibility, setEligibility] = useState<
    Record<string, SpaceEligibility>
  >({});
  const [filterOrgId, setFilterOrgId] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [submittingKey, setSubmittingKey] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [spaceFiles, setSpaceFiles] = useState<Record<string, File | null>>({});
  const [siiGiros, setSiiGiros] = useState<Record<string, string>>({});
  const [siiFiles, setSiiFiles] = useState<Record<string, File | null>>({});
  const [categorySiiGiros, setCategorySiiGiros] = useState<
    Record<string, string>
  >({});
  const [categorySiiFiles, setCategorySiiFiles] = useState<
    Record<string, File | null>
  >({});

  useEffect(() => {
    void Promise.resolve().then(() => {
      const orgParam = new URLSearchParams(window.location.search).get(
        "organizationId",
      );
      if (orgParam) {
        setFilterOrgId(orgParam);
      }
    });
  }, []);

  const refresh = useCallback(async () => {
    if (!token) return;
    const [spacesResult, orgsResult, personalResult] = await Promise.all([
      jsonRequest<{ items?: SpaceItem[] }>("/api/backend/spaces"),
      jsonRequest<{ items?: CompanyKYBOrganization[] }>(
        "/api/backend/host/organizations",
      ).catch(() => ({ items: [] as CompanyKYBOrganization[] })),
      jsonRequest<PersonalCompliance>(`${kybApi}/personal/compliance`).catch(
        (cause: { status?: number }) => {
          if (cause.status === 404) return null;
          throw cause;
        },
      ),
    ]);

    const hostSpaces = spacesResult.items ?? [];
    setSpaces(hostSpaces);
    setOrganizations(orgsResult.items ?? []);
    setPersonalCompliance(personalResult);

    if (hostSpaces.length === 0) {
      setEligibility({});
      return;
    }

    const pairs = await Promise.all(
      hostSpaces.map(async (space) => {
        try {
          const item = await jsonRequest<SpaceEligibility>(
            `${kybApi}/spaces/${encodeURIComponent(space.id)}/eligibility`,
          );
          return [space.id, item] as const;
        } catch {
          return [space.id, {} as SpaceEligibility] as const;
        }
      }),
    );
    setEligibility(Object.fromEntries(pairs));
  }, [token]);

  useEffect(() => {
    if (!token) return;
    let active = true;
    void Promise.resolve()
      .then(() => {
        if (active) setLoading(true);
        return refresh();
      })
      .catch((cause) => {
        if (active) {
          setError(
            cause instanceof Error
              ? cause.message
              : "No fue posible cargar las verificaciones de espacio.",
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [token, refresh]);

  async function submitSpaceDocument(
    event: FormEvent<HTMLFormElement>,
    spaceId: string,
  ) {
    event.preventDefault();
    setError("");
    setNotice("");
    if (!token) return;
    const file = spaceFiles[spaceId] ?? null;
    const invalid = fileError(file);
    if (invalid) {
      setError(invalid);
      return;
    }
    setSubmittingKey(`space:${spaceId}`);
    try {
      const form = new FormData();
      form.append(
        "property_or_sublease_authorization",
        file as File,
        (file as File).name,
      );
      await jsonRequest(
        `${kybApi}/spaces/${encodeURIComponent(spaceId)}/verification`,
        {
          method: "POST",
          body: form,
        },
      );
      setNotice("Respaldo del espacio enviado para revisión.");
      setSpaceFiles((prev) => ({ ...prev, [spaceId]: null }));
      await refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "No fue posible enviar el respaldo del espacio.",
      );
    } finally {
      setSubmittingKey(null);
    }
  }

  async function submitSiiDocument(
    event: FormEvent<HTMLFormElement>,
    options: {
      spaceId: string;
      endpoint: string;
      submittingId: string;
      giro: string;
      file: File | null;
      successNotice: string;
      onClearFile: () => void;
    },
  ) {
    event.preventDefault();
    setError("");
    setNotice("");
    if (!token) return;
    if (!/^\d{6}$/.test(options.giro)) {
      setError("El código de giro SII debe tener 6 dígitos.");
      return;
    }
    const invalid = fileError(options.file);
    if (invalid) {
      setError(invalid);
      return;
    }
    setSubmittingKey(options.submittingId);
    try {
      const form = new FormData();
      form.append("sii_giro_code", options.giro);
      form.append(
        "sii_start_activities",
        options.file as File,
        (options.file as File).name,
      );
      await jsonRequest(options.endpoint, {
        method: "POST",
        body: form,
      });
      setNotice(options.successNotice);
      options.onClearFile();
      await refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "No fue posible enviar el certificado SII.",
      );
    } finally {
      setSubmittingKey(null);
    }
  }

  function clearOrganizationFilter() {
    setFilterOrgId("");
    const url = new URL(window.location.href);
    url.searchParams.delete("organizationId");
    window.history.replaceState({}, "", `${url.pathname}${url.search}`);
  }

  if (auth.status === "checking") {
    return (
      <p role="status" className="text-sm text-muted-foreground">
        Comprobando sesión…
      </p>
    );
  }

  if (auth.status !== "authenticated") {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Inicia sesión para ver tus espacios</CardTitle>
          <CardDescription>
            Las verificaciones documentales y empresariales son privadas para la
            cuenta titular.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild>
            <Link href="/acceso?returnTo=%2Fmi-cuenta%2Fverificaciones%2Fespacios">
              Ir a iniciar sesión
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Consultando verificaciones de espacio…
        </CardContent>
      </Card>
    );
  }

  if (spaces.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Aún no tienes espacios publicados</CardTitle>
          <CardDescription>
            Las verificaciones documentales (respaldo del inmueble, Certificado
            de Inicio de Actividades SII y verificación empresarial KYB) están
            asignadas a cada espacio publicado.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/anfitrion/espacios/nuevo">
              <Plus className="mr-2 size-4" aria-hidden="true" />
              Publicar un espacio
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/mi-cuenta/empresas">
              <Building2 className="mr-2 size-4" aria-hidden="true" />
              Ir a Perfil empresas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const personalDisabled =
    personalCompliance?.compliance_status === "disabled_missing_docs";
  const filteredOrg = filterOrgId
    ? organizations.find((item) => item.id === filterOrgId)
    : undefined;
  const filteredOrgName = filteredOrg
    ? filteredOrg.trade_name ||
      filteredOrg.legal_name ||
      filteredOrg.razon_social ||
      "esta empresa"
    : "la empresa seleccionada";
  const visibleSpaces = filterOrgId
    ? spaces.filter((space) => space.organization_id === filterOrgId)
    : spaces;

  return (
    <div className="space-y-6">
      {filterOrgId ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/25 bg-primary/5 px-4 py-3 text-sm">
          <div className="flex items-center gap-2">
            <Building2 className="size-4 text-primary" aria-hidden="true" />
            <span>
              Mostrando espacios de{" "}
              <strong className="font-semibold">{filteredOrgName}</strong>
            </span>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={clearOrganizationFilter}
          >
            Ver todos los espacios
          </Button>
        </div>
      ) : null}

      {error ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>{error}</span>
        </div>
      ) : null}

      {notice ? (
        <div
          role="status"
          className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900"
        >
          {notice}
        </div>
      ) : null}

      {visibleSpaces.length === 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>
              No hay espacios registrados para {filteredOrgName}
            </CardTitle>
            <CardDescription>
              Puedes publicar un espacio asociado a esta empresa o volver a ver
              todos tus espacios.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-3">
            <Button type="button" onClick={clearOrganizationFilter}>
              Ver todos los espacios
            </Button>
            <Button asChild variant="outline">
              <Link href="/anfitrion/espacios/nuevo">
                <Plus className="mr-2 size-4" aria-hidden="true" />
                Publicar un espacio
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {visibleSpaces.map((space) => {
        const spaceElig = eligibility[space.id];
        const org = space.organization_id
          ? organizations.find((item) => item.id === space.organization_id)
          : undefined;
        const modalityLabel = space.organization_id
          ? `Empresa: ${
              org
                ? org.trade_name ||
                  org.legal_name ||
                  org.razon_social ||
                  "Empresa"
                : "Empresa vinculada"
            }`
          : "Persona Natural";
        const canUploadSpaceDoc =
          !space.organization_id ||
          org?.my_role === "representante_legal" ||
          org?.my_role === "administrador";
        const graceDays = remainingDays(
          spaceElig?.grace_due_at ??
            (space.organization_id
              ? org?.grace_due_at
              : personalCompliance?.grace_due_at),
        );
        const isCompanyVerified =
          Boolean(space.organization_id) &&
          (spaceElig?.company_verified === true ||
            org?.compliance_status === "compliant");
        const spaceDisabled =
          spaceElig?.compliance_status === "disabled_missing_docs" ||
          (!space.organization_id && personalDisabled) ||
          (Boolean(space.organization_id) &&
            org?.compliance_status === "disabled_missing_docs");

        const propertyStatus =
          spaceElig?.property_status || spaceElig?.verification_status || "";
        const isPropertyApproved =
          spaceElig?.property_approved === true ||
          propertyStatus === "APPROVED" ||
          (!spaceElig?.property_status && spaceElig?.space_verified === true);
        const isPropertyInReview =
          propertyStatus === "PENDING" || propertyStatus === "IN_REVIEW";

        const catSiiStatus = spaceElig?.category_sii_status || "";
        const isCatSiiApproved =
          spaceElig?.category_sii_approved === true ||
          catSiiStatus === "APPROVED";
        const isCatSiiInReview =
          catSiiStatus === "PENDING" || catSiiStatus === "IN_REVIEW";

        return (
          <Card
            key={space.id}
            data-space-verification-card={space.id}
            aria-labelledby={`space-card-title-${space.id}`}
          >
            <CardHeader>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="space-y-1">
                  <CardTitle
                    id={`space-card-title-${space.id}`}
                    className="text-lg"
                  >
                    {space.title}
                  </CardTitle>
                  <div className="flex flex-wrap items-center gap-2 text-xs">
                    <Badge variant="secondary">{spaceStateLabel(space)}</Badge>
                    {spaceElig?.category_name ? (
                      <Badge variant="secondary">
                        {spaceElig.category_name}
                      </Badge>
                    ) : null}
                    <Badge variant="outline" className="gap-1">
                      {space.organization_id ? (
                        <Building2 className="size-3" aria-hidden="true" />
                      ) : (
                        <User className="size-3" aria-hidden="true" />
                      )}
                      {modalityLabel}
                    </Badge>
                  </div>
                </div>
                <Badge
                  variant="outline"
                  className={
                    spaceElig?.space_verified
                      ? "gap-1 border-emerald-300 bg-emerald-50 text-emerald-900"
                      : "gap-1"
                  }
                >
                  {spaceElig?.space_verified ? (
                    <CheckCircle2 className="size-3.5" aria-hidden="true" />
                  ) : (
                    <Clock3 className="size-3.5" aria-hidden="true" />
                  )}
                  {spaceElig?.space_verified
                    ? "Espacio verificado"
                    : verificationStatusLabel(spaceElig?.verification_status)}
                </Badge>
              </div>
              <CardDescription>
                {spaceDisabled
                  ? "Publicación o administración comercial inhabilitada por documentación pendiente."
                  : isCompanyVerified && !spaceElig?.space_verified
                    ? `${
                        graceDays !== null
                          ? `Plazo de gracia: quedan ${graceDays} días. `
                          : ""
                      }La empresa ya está verificada y tus espacios aprobados siguen habilitados. Para verificar y habilitar este espacio, completa su documentación pendiente.`
                    : graceDays !== null && !spaceElig?.space_verified
                      ? `Plazo de gracia: quedan ${graceDays} días para completar la documentación pendiente.`
                      : "Al aprobarse el respaldo del inmueble y la documentación del titular, el espacio obtiene la insignia verificada."}
              </CardDescription>
            </CardHeader>

            <CardContent className="space-y-5">
              {/* Paso 1: Documentación de empresa (KYB) o aviso compacto si ya está verificada */}
              {space.organization_id ? (
                isCompanyVerified ? (
                  <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
                    <div className="flex items-start gap-2.5">
                      <CheckCircle2
                        className="mt-0.5 size-4 shrink-0 text-emerald-700"
                        aria-hidden="true"
                      />
                      <p>
                        Empresa verificada (KYB aprobado): no necesitas volver a
                        subir e-RUT ni Vigencia de Poderes para este espacio.
                      </p>
                    </div>
                    <Link
                      href={`/mi-cuenta/empresas?organizationId=${encodeURIComponent(space.organization_id)}`}
                      className="text-xs font-medium underline underline-offset-2"
                    >
                      Ver perfil de empresa
                    </Link>
                  </div>
                ) : org ? (
                  <CompanyKYBCard
                    organization={org}
                    variant="embedded"
                    onVerifiedSubmitted={() => void refresh()}
                  />
                ) : (
                  <div className="space-y-3 rounded-lg border border-border/80 bg-muted/20 p-4 text-sm">
                    <p className="font-medium">
                      Verificación empresarial KYB vinculada
                    </p>
                    <p className="text-muted-foreground">
                      No encontramos esta empresa en tus perfiles activos.
                      Revisa o gestiona tu acceso en Perfil empresas.
                    </p>
                    <Button asChild variant="outline" size="sm">
                      <Link href="/mi-cuenta/empresas">
                        Ir a Perfil empresas
                      </Link>
                    </Button>
                  </div>
                )
              ) : null}

              {/* Paso 2: Respaldo del inmueble (obligatorio por cada espacio) */}
              <section
                aria-labelledby={`space-doc-heading-${space.id}`}
                className="space-y-3 rounded-lg border p-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <h2
                      id={`space-doc-heading-${space.id}`}
                      className="text-sm font-semibold"
                    >
                      Respaldo del inmueble
                    </h2>
                    <p className="text-xs text-muted-foreground">
                      Estado:{" "}
                      {isPropertyApproved
                        ? "Aprobado"
                        : documentStatusLabel(propertyStatus)}
                    </p>
                  </div>
                </div>

                {propertyStatus === "REJECTED" &&
                spaceElig?.property_rejection_reason ? (
                  <div
                    role="alert"
                    className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
                  >
                    <AlertCircle
                      className="mt-0.5 size-4 shrink-0"
                      aria-hidden="true"
                    />
                    <span>
                      Motivo del rechazo: {spaceElig.property_rejection_reason}
                    </span>
                  </div>
                ) : null}

                {isPropertyApproved ? (
                  <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900">
                    <CheckCircle2
                      className="size-4 shrink-0 text-emerald-700"
                      aria-hidden="true"
                    />
                    <span>Respaldo del inmueble aprobado.</span>
                  </div>
                ) : isPropertyInReview ? (
                  <div className="flex items-center gap-2 rounded-lg border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">
                    <Clock3
                      className="size-4 shrink-0 text-sky-700"
                      aria-hidden="true"
                    />
                    <span>
                      Respaldo del inmueble enviado y en proceso de revisión.
                    </span>
                  </div>
                ) : canUploadSpaceDoc ? (
                  <form
                    onSubmit={(event) =>
                      void submitSpaceDocument(event, space.id)
                    }
                    className="flex flex-wrap items-end gap-3"
                  >
                    <div className="min-w-60 flex-1 space-y-2">
                      <Label htmlFor={`space-doc-${space.id}`}>
                        Dominio vigente o autorización de arriendo/subarriendo
                      </Label>
                      <Input
                        id={`space-doc-${space.id}`}
                        type="file"
                        accept="application/pdf,image/jpeg,image/png"
                        onChange={(event) =>
                          setSpaceFiles((prev) => ({
                            ...prev,
                            [space.id]: event.target.files?.[0] ?? null,
                          }))
                        }
                        required
                      />
                    </div>
                    <Button type="submit" disabled={submittingKey !== null}>
                      {submittingKey === `space:${space.id}` ? (
                        <Loader2 className="mr-2 size-4 animate-spin" />
                      ) : (
                        <FileCheck2 className="mr-2 size-4" />
                      )}
                      Enviar respaldo
                    </Button>
                  </form>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Solo un representante legal o administrador de la empresa
                    puede cargar el respaldo de este espacio.
                  </p>
                )}
              </section>

              {/* Paso 3: Certificado SII por nueva categoría (solo espacios de empresa que lo requieran) */}
              {space.organization_id &&
              spaceElig?.requires_category_sii === true ? (
                <section
                  aria-labelledby={`category-sii-heading-${space.id}`}
                  className="space-y-4 rounded-lg border p-4"
                >
                  <div>
                    <h2
                      id={`category-sii-heading-${space.id}`}
                      className="text-sm font-semibold"
                    >
                      Certificado SII por categoría (
                      {spaceElig.category_name ||
                        spaceElig.category_code ||
                        "nueva categoría"}
                      )
                    </h2>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Este espacio pertenece a una categoría distinta a las ya
                      acreditadas por tu empresa. Sube el Certificado de Inicio
                      de Actividades del SII y su código de giro (6 dígitos)
                      para habilitar y verificar esta categoría.
                    </p>
                    <p className="mt-1 text-xs font-medium text-muted-foreground">
                      Estado:{" "}
                      {isCatSiiApproved
                        ? "Aprobado"
                        : documentStatusLabel(catSiiStatus)}
                    </p>
                  </div>

                  {catSiiStatus === "REJECTED" &&
                  spaceElig.category_sii_rejection_reason ? (
                    <div
                      role="alert"
                      className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
                    >
                      <AlertCircle
                        className="mt-0.5 size-4 shrink-0"
                        aria-hidden="true"
                      />
                      <span>
                        Motivo del rechazo:{" "}
                        {spaceElig.category_sii_rejection_reason}
                      </span>
                    </div>
                  ) : null}

                  {isCatSiiApproved ? (
                    <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900">
                      <CheckCircle2
                        className="size-4 shrink-0 text-emerald-700"
                        aria-hidden="true"
                      />
                      <span>Certificado SII por categoría aprobado.</span>
                    </div>
                  ) : isCatSiiInReview ? (
                    <div className="flex items-center gap-2 rounded-lg border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">
                      <Clock3
                        className="size-4 shrink-0 text-sky-700"
                        aria-hidden="true"
                      />
                      <span>
                        Certificado SII por categoría enviado y en proceso de
                        revisión.
                      </span>
                    </div>
                  ) : canUploadSpaceDoc ? (
                    <form
                      onSubmit={(event) =>
                        void submitSiiDocument(event, {
                          spaceId: space.id,
                          endpoint: `${kybApi}/spaces/${encodeURIComponent(space.id)}/sii`,
                          submittingId: `category-sii:${space.id}`,
                          giro: categorySiiGiros[space.id] ?? "",
                          file: categorySiiFiles[space.id] ?? null,
                          successNotice:
                            "Certificado SII por categoría enviado para revisión.",
                          onClearFile: () =>
                            setCategorySiiFiles((prev) => ({
                              ...prev,
                              [space.id]: null,
                            })),
                        })
                      }
                      className="grid gap-4"
                    >
                      <div className="grid gap-4 sm:grid-cols-2">
                        <div className="space-y-2">
                          <Label htmlFor={`cat-sii-giro-${space.id}`}>
                            Código de giro SII (6 dígitos)
                          </Label>
                          <Input
                            id={`cat-sii-giro-${space.id}`}
                            value={categorySiiGiros[space.id] ?? ""}
                            onChange={(event) =>
                              setCategorySiiGiros((prev) => ({
                                ...prev,
                                [space.id]: event.target.value
                                  .replace(/\D/g, "")
                                  .slice(0, 6),
                              }))
                            }
                            placeholder="620100"
                            inputMode="numeric"
                            required
                          />
                        </div>
                        <div className="space-y-2">
                          <Label htmlFor={`cat-sii-file-${space.id}`}>
                            Certificado de Inicio de Actividades del SII
                          </Label>
                          <Input
                            id={`cat-sii-file-${space.id}`}
                            type="file"
                            accept="application/pdf,image/jpeg,image/png"
                            onChange={(event) =>
                              setCategorySiiFiles((prev) => ({
                                ...prev,
                                [space.id]: event.target.files?.[0] ?? null,
                              }))
                            }
                            required
                          />
                        </div>
                      </div>
                      <div className="flex justify-end">
                        <Button type="submit" disabled={submittingKey !== null}>
                          {submittingKey === `category-sii:${space.id}` ? (
                            <Loader2 className="mr-2 size-4 animate-spin" />
                          ) : (
                            <FileCheck2 className="mr-2 size-4" />
                          )}
                          Enviar SII por categoría
                        </Button>
                      </div>
                    </form>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      Solo un representante legal o administrador de la empresa
                      puede cargar este certificado.
                    </p>
                  )}
                </section>
              ) : null}

              {/* Documentación personal SII (Persona Natural) */}
              {!space.organization_id ? (
                <section
                  aria-labelledby={`personal-sii-heading-${space.id}`}
                  className="space-y-4 rounded-lg border p-4"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <h2
                        id={`personal-sii-heading-${space.id}`}
                        className="text-sm font-semibold"
                      >
                        Certificado de Inicio de Actividades del SII
                      </h2>
                      <p className="text-xs text-muted-foreground">
                        Estado SII:{" "}
                        {documentStatusLabel(personalCompliance?.sii_status)}
                        {personalCompliance?.sii_giro_code
                          ? ` · Giro ${personalCompliance.sii_giro_code}`
                          : ""}
                      </p>
                    </div>
                  </div>

                  {personalDisabled ? (
                    <div
                      role="alert"
                      className="space-y-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
                    >
                      <p>
                        Tu perfil está inhabilitado por falta de documentación.
                        {personalCompliance?.disabled_reason
                          ? ` Motivo: ${personalCompliance.disabled_reason}`
                          : ""}
                      </p>
                      <div className="flex flex-wrap gap-x-4 gap-y-1">
                        <Link
                          className="font-medium underline underline-offset-2"
                          href="/reservas"
                        >
                          Seguir usando EspaciGo como arrendatario
                        </Link>
                        <Link
                          className="font-medium underline underline-offset-2"
                          href="/mi-cuenta/privacidad"
                        >
                          Privacidad y cierre de cuenta
                        </Link>
                      </div>
                    </div>
                  ) : null}

                  <form
                    onSubmit={(event) =>
                      void submitSiiDocument(event, {
                        spaceId: space.id,
                        endpoint: `${kybApi}/personal/sii`,
                        submittingId: `sii:${space.id}`,
                        giro: siiGiros[space.id] ?? "",
                        file: siiFiles[space.id] ?? null,
                        successNotice: "Certificado SII enviado para revisión.",
                        onClearFile: () =>
                          setSiiFiles((prev) => ({
                            ...prev,
                            [space.id]: null,
                          })),
                      })
                    }
                    className="grid gap-4"
                  >
                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor={`sii-giro-${space.id}`}>
                          Código de giro SII (6 dígitos)
                        </Label>
                        <Input
                          id={`sii-giro-${space.id}`}
                          value={siiGiros[space.id] ?? ""}
                          onChange={(event) =>
                            setSiiGiros((prev) => ({
                              ...prev,
                              [space.id]: event.target.value
                                .replace(/\D/g, "")
                                .slice(0, 6),
                            }))
                          }
                          placeholder="620100"
                          inputMode="numeric"
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor={`sii-file-${space.id}`}>
                          Certificado de Inicio de Actividades del SII
                        </Label>
                        <Input
                          id={`sii-file-${space.id}`}
                          type="file"
                          accept="application/pdf,image/jpeg,image/png"
                          onChange={(event) =>
                            setSiiFiles((prev) => ({
                              ...prev,
                              [space.id]: event.target.files?.[0] ?? null,
                            }))
                          }
                          required
                        />
                      </div>
                    </div>
                    <div className="flex justify-end">
                      <Button type="submit" disabled={submittingKey !== null}>
                        {submittingKey === `sii:${space.id}` ? (
                          <Loader2 className="mr-2 size-4 animate-spin" />
                        ) : (
                          <FileCheck2 className="mr-2 size-4" />
                        )}
                        Enviar SII
                      </Button>
                    </div>
                  </form>
                </section>
              ) : null}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
