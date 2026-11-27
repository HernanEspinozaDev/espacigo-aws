"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  CheckCircle2,
  Clock3,
  FileCheck2,
  Loader2,
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

type Compliance = {
  compliance_status: string;
  sii_giro_code?: string;
  sii_status?: string;
  grace_started_at?: string;
  grace_due_at?: string;
  disabled_at?: string;
  disabled_reason?: string;
};
type Space = { id: string; title: string; organization_id?: string };
type Eligibility = {
  space_verified?: boolean;
  compliance_status?: string;
  verification_status?: string;
  grace_due_at?: string;
};
const allowedTypes = new Set(["application/pdf", "image/jpeg", "image/png"]);
const maxBytes = 10 * 1024 * 1024;
const api = "/api/backend/kyb";

async function jsonRequest<T>(
  token: string,
  path: string,
  init: RequestInit = {},
): Promise<T> {
  if (!token) throw new Error("Inicia sesión para continuar.");
  const response = await fetch(path, { ...init, credentials: "same-origin" });
  const data = await response.json().catch(() => ({}));
  if (!response.ok)
    throw Object.assign(
      new Error("No fue posible consultar la documentación."),
      { status: response.status },
    );
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
function statusText(status?: string) {
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
    "Pendiente"
  );
}

export function PersonalSpaceCompliance({
  organizations,
}: {
  organizations: Array<{ id: string; my_role?: string }>;
}) {
  const { snapshot, getAccessToken } = useAuth();
  const token = snapshot.status === "authenticated" ? getAccessToken() : null;
  const [compliance, setCompliance] = useState<Compliance | null>(null);
  const [spaces, setSpaces] = useState<Space[]>([]);
  const [eligibility, setEligibility] = useState<Record<string, Eligibility>>(
    {},
  );
  const [giro, setGiro] = useState("");
  const [siiFile, setSiiFile] = useState<File | null>(null);
  const [spaceFiles, setSpaceFiles] = useState<Record<string, File | null>>({});
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const refresh = useCallback(async () => {
    if (!token) return;
    const [ownResult, spacesResult] = await Promise.all([
      jsonRequest<Compliance>(token, `${api}/personal/compliance`).catch(
        (error) => {
          if (error.status === 404) return null;
          throw error;
        },
      ),
      jsonRequest<{ items?: Space[] }>(token, "/api/backend/spaces"),
    ]);
    setCompliance(ownResult);
    const hostSpaces = spacesResult.items ?? [];
    setSpaces(hostSpaces);
    const pairs = await Promise.all(
      hostSpaces.map(async (space) => {
        try {
          return [
            space.id,
            await jsonRequest<Eligibility>(
              token,
              `${api}/spaces/${encodeURIComponent(space.id)}/eligibility`,
            ),
          ] as const;
        } catch {
          return [space.id, {} as Eligibility] as const;
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
        if (active)
          setError(
            cause instanceof Error
              ? cause.message
              : "No fue posible consultar el cumplimiento.",
          );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [token, refresh]);

  if (!token) return null;
  const days = remainingDays(compliance?.grace_due_at);
  const disabled = compliance?.compliance_status === "disabled_missing_docs";

  async function submitSii(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setNotice("");
    if (!token) return;
    const invalid = fileError(siiFile);
    if (invalid) {
      setError(invalid);
      return;
    }
    if (!/^\d{6}$/.test(giro)) {
      setError("El código de giro SII debe tener 6 dígitos.");
      return;
    }
    setSubmitting(true);
    try {
      const form = new FormData();
      form.append("sii_giro_code", giro);
      form.append(
        "sii_start_activities",
        siiFile as File,
        (siiFile as File).name,
      );
      await jsonRequest(token, `${api}/personal/sii`, {
        method: "POST",
        body: form,
      });
      setNotice("Certificado SII enviado para revisión.");
      setSiiFile(null);
      await refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "No fue posible enviar el certificado SII.",
      );
    } finally {
      setSubmitting(false);
    }
  }
  async function submitSpace(
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
    setSubmitting(true);
    try {
      const form = new FormData();
      form.append(
        "property_or_sublease_authorization",
        file as File,
        (file as File).name,
      );
      await jsonRequest(
        token,
        `${api}/spaces/${encodeURIComponent(spaceId)}/verification`,
        { method: "POST", body: form },
      );
      setNotice("Respaldo del espacio enviado para revisión.");
      setSpaceFiles((previous) => ({ ...previous, [spaceId]: null }));
      await refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "No fue posible enviar el respaldo del espacio.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Clock3 className="size-5" />
                Cumplimiento Persona Natural
              </CardTitle>
              <CardDescription>
                La verificación se revisa por perfil y por espacio. Mientras
                estés dentro del plazo de gracia puedes operar sin insignia.
              </CardDescription>
            </div>
            {compliance ? (
              <Badge variant="outline">
                {statusText(compliance.compliance_status)}
              </Badge>
            ) : null}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Consultando estado…
            </p>
          ) : null}
          {days !== null && !disabled ? (
            <p className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
              Plazo de gracia: quedan <strong>{days} días</strong> para
              completar la documentación requerida.
            </p>
          ) : null}
          {disabled ? (
            <div
              role="alert"
              className="space-y-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
            >
              <p>
                Tu perfil está inhabilitado por falta de documentación. Tus
                publicaciones y administración comercial están pausadas hasta
                regularizar los documentos pendientes; tu acceso como
                arrendatario sigue habilitado.
                {compliance?.disabled_reason
                  ? ` Motivo: ${compliance.disabled_reason}`
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
          {error ? (
            <p
              role="alert"
              className="flex gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
            >
              <AlertCircle className="size-4 shrink-0" />
              {error}
            </p>
          ) : null}
          {notice ? (
            <p
              role="status"
              className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900"
            >
              {notice}
            </p>
          ) : null}
          <form
            onSubmit={submitSii}
            className="grid gap-4 rounded-lg border p-4"
          >
            <div>
              <h3 className="font-medium">Inicio de Actividades SII</h3>
              <p className="text-sm text-muted-foreground">
                Adjunta el certificado vigente y el código de giro de 6 dígitos.
              </p>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="personal-sii-giro">Código de giro SII</Label>
                <Input
                  id="personal-sii-giro"
                  value={giro}
                  onChange={(event) =>
                    setGiro(event.target.value.replace(/\D/g, "").slice(0, 6))
                  }
                  inputMode="numeric"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="personal-sii-file">
                  Certificado de Inicio de Actividades
                </Label>
                <Input
                  id="personal-sii-file"
                  type="file"
                  accept="application/pdf,image/jpeg,image/png"
                  onChange={(event) =>
                    setSiiFile(event.target.files?.[0] ?? null)
                  }
                  required
                />
              </div>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm text-muted-foreground">
                Estado: {statusText(compliance?.sii_status)}
              </span>
              <Button type="submit" disabled={submitting}>
                {submitting ? (
                  <Loader2 className="mr-2 size-4 animate-spin" />
                ) : (
                  <FileCheck2 className="mr-2 size-4" />
                )}
                Enviar SII
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold">Respaldo por espacio</h2>
          <p className="text-sm text-muted-foreground">
            Cada inmueble obtiene su propia insignia al aprobarse el documento
            de dominio o autorización.
          </p>
        </div>
        {spaces.length === 0 ? (
          <Card>
            <CardContent className="py-6 text-sm text-muted-foreground">
              Aún no tienes espacios para verificar.
            </CardContent>
          </Card>
        ) : (
          spaces.map((space) => {
            const state = eligibility[space.id];
            const role = organizations.find(
              (org) => org.id === space.organization_id,
            )?.my_role;
            const canUpload =
              !space.organization_id ||
              role === "representante_legal" ||
              role === "administrador";
            return (
              <Card key={space.id}>
                <CardHeader>
                  <div className="flex items-center justify-between gap-3">
                    <CardTitle className="text-base">{space.title}</CardTitle>
                    <Badge variant="outline" className="gap-1">
                      {state?.space_verified ? (
                        <CheckCircle2 className="size-3.5" />
                      ) : (
                        <Clock3 className="size-3.5" />
                      )}
                      {state?.space_verified
                        ? "Espacio verificado"
                        : statusText(state?.verification_status)}
                    </Badge>
                  </div>
                  <CardDescription>
                    Solo el respaldo aprobado de este espacio habilita su
                    insignia pública.
                    {state?.grace_due_at && !state.space_verified
                      ? ` Vencimiento compartido: ${new Date(state.grace_due_at).toLocaleDateString("es-CL", { timeZone: "America/Santiago" })}.`
                      : ""}
                  </CardDescription>
                </CardHeader>
                {canUpload ? (
                  <CardContent>
                    <form
                      onSubmit={(event) => void submitSpace(event, space.id)}
                      className="flex flex-wrap items-end gap-3"
                    >
                      <div className="min-w-64 flex-1 space-y-2">
                        <Label htmlFor={`space-doc-${space.id}`}>
                          Dominio vigente o autorización de arriendo/subarriendo
                        </Label>
                        <Input
                          id={`space-doc-${space.id}`}
                          type="file"
                          accept="application/pdf,image/jpeg,image/png"
                          onChange={(event) =>
                            setSpaceFiles((previous) => ({
                              ...previous,
                              [space.id]: event.target.files?.[0] ?? null,
                            }))
                          }
                          required
                        />
                      </div>
                      <Button type="submit" disabled={submitting}>
                        {submitting ? (
                          <Loader2 className="mr-2 size-4 animate-spin" />
                        ) : (
                          <FileCheck2 className="mr-2 size-4" />
                        )}
                        Enviar respaldo
                      </Button>
                    </form>
                  </CardContent>
                ) : null}
              </Card>
            );
          })
        )}
      </section>
    </div>
  );
}
