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
  const [loading, setLoading] = useState(true);
  const [submittingKey, setSubmittingKey] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [spaceFiles, setSpaceFiles] = useState<Record<string, File | null>>({});
  const [siiGiros, setSiiGiros] = useState<Record<string, string>>({});
  const [siiFiles, setSiiFiles] = useState<Record<string, File | null>>({});

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

  async function submitPersonalSii(
    event: FormEvent<HTMLFormElement>,
    spaceId: string,
  ) {
    event.preventDefault();
    setError("");
    setNotice("");
    if (!token) return;
    const giro = siiGiros[spaceId] ?? "";
    const file = siiFiles[spaceId] ?? null;
    if (!/^\d{6}$/.test(giro)) {
      setError("El código de giro SII debe tener 6 dígitos.");
      return;
    }
    const invalid = fileError(file);
    if (invalid) {
      setError(invalid);
      return;
    }
    setSubmittingKey(`sii:${spaceId}`);
    try {
      const form = new FormData();
      form.append("sii_giro_code", giro);
      form.append("sii_start_activities", file as File, (file as File).name);
      await jsonRequest(`${kybApi}/personal/sii`, {
        method: "POST",
        body: form,
      });
      setNotice("Certificado SII enviado para revisión.");
      setSiiFiles((prev) => ({ ...prev, [spaceId]: null }));
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

  return (
    <div className="space-y-6">
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

      {spaces.map((space) => {
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
        const spaceDisabled =
          spaceElig?.compliance_status === "disabled_missing_docs" ||
          (!space.organization_id && personalDisabled) ||
          (Boolean(space.organization_id) &&
            org?.compliance_status === "disabled_missing_docs");

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
                  : graceDays !== null && !spaceElig?.space_verified
                    ? `Plazo de gracia: quedan ${graceDays} días para completar la documentación pendiente.`
                    : "Al aprobarse el respaldo del inmueble y la documentación del titular, el espacio obtiene la insignia verificada."}
              </CardDescription>
            </CardHeader>

            <CardContent className="space-y-5">
              {/* 1. Respaldo del espacio */}
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
                      Respaldo del espacio
                    </h2>
                    <p className="text-xs text-muted-foreground">
                      Estado:{" "}
                      {spaceElig?.space_verified
                        ? "Aprobado"
                        : verificationStatusLabel(
                            spaceElig?.verification_status,
                          )}
                    </p>
                  </div>
                </div>

                {canUploadSpaceDoc ? (
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

              {/* 2. Documentación del titular (Empresa vs Persona Natural) */}
              {space.organization_id ? (
                org ? (
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
              ) : (
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
                        {verificationStatusLabel(
                          personalCompliance?.sii_status,
                        )}
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
                      void submitPersonalSii(event, space.id)
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
              )}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
