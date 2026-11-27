"use client";

import { useState } from "react";
import {
  AlertCircle,
  Archive,
  CheckCircle2,
  Download,
  FileJson,
  FileText,
  Loader2,
  Lock,
  Shield,
  SlidersHorizontal,
  Trash2,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Label } from "@/components/workspace/ui/label";
import { Button } from "@/components/workspace/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/features/auth/ui/use-auth";
import { useAccount } from "./use-account";
import {
  PRIVACY_PURPOSES,
  PRIVACY_PURPOSE_DESCRIPTIONS,
  PRIVACY_PURPOSE_LABELS,
  PRIVACY_PURPOSE_STATE_LABELS,
  PRIVACY_TREATMENT_BLOCK_SCOPE_LABELS,
  PRIVACY_TREATMENT_BLOCK_STATE_LABELS,
  type PrivacyPurpose,
  type PrivacyPurposeState,
  type RightsRequestKind,
} from "../domain/privacy";
import { privateBlobUrls } from "@/lib/api/private-blob-urls";

const DETAIL_MAX_LENGTH = 1000;
const MIN_REQUIRED_DETAIL_LENGTH = 10;

const DETAILED_KIND_LABELS: Record<RightsRequestKind, string> = {
  acceso: "Acceso",
  rectificacion: "Rectificación",
  supresion: "Supresión",
  oposicion: "Oposición",
  portabilidad: "Portabilidad",
  bloqueo: "Bloqueo temporal",
};

const EVENT_LABELS: Record<string, string> = {
  recepcion: "Solicitud registrada",
  identidad_verificada: "Identidad verificada",
  en_revision: "En revisión",
  aclaracion_solicitada: "Aclaración solicitada",
  prorroga: "Prórroga operativa registrada",
  resolucion: "Respuesta publicada",
  ejecucion_derecho: "Derecho ejecutado",
  despacho_registrado: "Despacho registrado",
};

const PURPOSE_STATE_ACTIONS: ReadonlyArray<{
  state: PrivacyPurposeState;
  label: string;
}> = [
  { state: "permitido", label: "Permitir" },
  { state: "opuesto", label: "Oponerme" },
  { state: "retirado", label: "Retirar" },
];

export type PrivacySection =
  "privacidad" | "exportacion" | "preferencias" | "bloqueos";

export function PrivacyCard({ section }: { section: PrivacySection }) {
  const { snapshot: authSnapshot } = useAuth();
  const {
    snapshot,
    requestDataRight,
    exportIdentityBlob,
    downloadIdentityArchive,
    updatePrivacyPreference,
    exportPortabilityBlob,
  } = useAccount();
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [prefMessage, setPrefMessage] = useState<string | null>(null);
  const [prefError, setPrefError] = useState<string | null>(null);
  const [portabilitySha256, setPortabilitySha256] = useState<string | null>(
    null,
  );
  const [selectedKind, setSelectedKind] =
    useState<RightsRequestKind>("rectificacion");
  const [detail, setDetail] = useState("");
  const [preferenceChoices, setPreferenceChoices] = useState<
    Partial<Record<PrivacyPurpose, PrivacyPurposeState>>
  >({});
  const [preferenceReasons, setPreferenceReasons] = useState<
    Partial<Record<PrivacyPurpose, string>>
  >({});

  const handleRequest = async (kind: RightsRequestKind) => {
    if (authSnapshot.status !== "authenticated") return;
    setError(null);
    setMessage(null);
    setLoadingAction(kind);
    try {
      await requestDataRight(kind);
      setMessage(
        kind === "supresion"
          ? "Solicitud de supresión de datos registrada exitosamente."
          : "Solicitud de acceso a datos registrada exitosamente.",
      );
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Error al enviar la solicitud",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const handleDetailedRequest = async (event: React.FormEvent) => {
    event.preventDefault();
    if (authSnapshot.status !== "authenticated") return;
    setError(null);
    setMessage(null);
    const trimmedDetail = detail.trim();
    if (
      (selectedKind === "rectificacion" ||
        selectedKind === "oposicion" ||
        selectedKind === "bloqueo") &&
      trimmedDetail.length < MIN_REQUIRED_DETAIL_LENGTH
    ) {
      setError(
        `Las solicitudes de ${DETAILED_KIND_LABELS[selectedKind].toLowerCase()} requieren un detalle de al menos ${MIN_REQUIRED_DETAIL_LENGTH} caracteres.`,
      );
      return;
    }
    if (trimmedDetail.length > DETAIL_MAX_LENGTH) {
      setError(
        `El detalle no puede superar los ${DETAIL_MAX_LENGTH} caracteres.`,
      );
      return;
    }
    setLoadingAction("detailed");
    try {
      await requestDataRight(selectedKind, trimmedDetail);
      setDetail("");
      setMessage(
        `Solicitud de ${DETAILED_KIND_LABELS[selectedKind].toLowerCase()} registrada exitosamente para revisión.`,
      );
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Error al enviar la solicitud",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const downloadBlobInBrowser = (
    blob: Blob,
    filename: string,
    resourceKey: string,
  ) => {
    const registered = privateBlobUrls.create(resourceKey, blob);
    const link = document.createElement("a");
    link.href = registered.url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    setTimeout(() => {
      registered.revoke();
    }, 2000);
  };

  const handleExportJson = async () => {
    if (authSnapshot.status !== "authenticated") return;
    setError(null);
    setMessage(null);
    setLoadingAction("export-json");
    try {
      const blob = await exportIdentityBlob();
      if (!blob) {
        setError("No se pudo obtener el archivo de exportación JSON.");
        return;
      }
      downloadBlobInBrowser(
        blob,
        "espacigo-identity-export.json",
        "export-json",
      );
      setMessage("Exportación JSON descargada correctamente.");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Error al descargar la exportación JSON",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const handleDownloadArchive = async () => {
    if (authSnapshot.status !== "authenticated") return;
    setError(null);
    setMessage(null);
    setLoadingAction("export-archive");
    try {
      const blob = await downloadIdentityArchive();
      if (!blob) {
        setError("No se pudo obtener el archivo comprimido ZIP.");
        return;
      }
      downloadBlobInBrowser(
        blob,
        "espacigo-identity-archive.zip",
        "export-zip",
      );
      setMessage("Archivo ZIP descargado correctamente.");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Error al descargar el archivo ZIP",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const handleExportPortability = async () => {
    if (authSnapshot.status !== "authenticated") return;
    setError(null);
    setMessage(null);
    setPortabilitySha256(null);
    setLoadingAction("export-portability");
    try {
      const result = await exportPortabilityBlob();
      if (!result) {
        setError("No se pudo obtener el paquete de portabilidad estructurada.");
        return;
      }
      downloadBlobInBrowser(
        result.blob,
        "espacigo-portabilidad-v1.json",
        "export-portability",
      );
      setPortabilitySha256(result.sha256);
      setMessage(
        "Portabilidad estructurada (JSON v1) descargada correctamente.",
      );
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Error al descargar la portabilidad estructurada",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const handleUpdatePreference = async (
    purpose: PrivacyPurpose,
    state: PrivacyPurposeState,
  ) => {
    if (authSnapshot.status !== "authenticated") return;
    setPrefError(null);
    setPrefMessage(null);
    const reason = (preferenceReasons[purpose] ?? "").trim();
    const reasonLength = Array.from(reason).length;
    if (reason && (reasonLength < 5 || reasonLength > 500)) {
      setPrefError(
        "El motivo debe tener entre 5 y 500 caracteres, o quedar vacío.",
      );
      return;
    }
    setLoadingAction(`pref-${purpose}`);
    try {
      await updatePrivacyPreference({
        purpose,
        state,
        ...(reason ? { reason } : {}),
      });
      setPreferenceReasons((prev) => ({ ...prev, [purpose]: "" }));
      setPreferenceChoices((prev) => ({ ...prev, [purpose]: undefined }));
      setPrefMessage(
        `Preferencia de ${PRIVACY_PURPOSE_LABELS[purpose]} actualizada a ${PRIVACY_PURPOSE_STATE_LABELS[state]}.`,
      );
    } catch (err) {
      setPrefError(
        err instanceof Error
          ? err.message
          : "Error al actualizar la preferencia de finalidad",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const requests = snapshot.rightsRequests;
  const preferencesByPurpose = new Map(
    snapshot.privacyPreferences
      .filter((pref) => pref.updated_at)
      .map((pref) => [pref.purpose, pref]),
  );
  const restrictions = snapshot.privacyRestrictions;

  const feedback = (
    <>
      {error && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded-md bg-destructive/15 p-3 text-sm text-destructive border border-destructive/20"
        >
          <AlertCircle className="size-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {message && (
        <div
          role="status"
          className="flex items-center gap-2 rounded-md bg-emerald-50 p-3 text-sm text-emerald-800 border border-emerald-200 dark:bg-emerald-950/30 dark:border-emerald-800 dark:text-emerald-300"
        >
          <CheckCircle2 className="size-4 shrink-0" />
          <span>{message}</span>
        </div>
      )}
    </>
  );

  if (section === "privacidad") {
    return (
      <Card aria-labelledby="privacy-rights-title">
        <CardHeader>
          <div className="flex items-center gap-2">
            <Shield className="size-5 text-primary" aria-hidden="true" />
            <CardTitle id="privacy-rights-title">
              Derechos ARCO y Privacidad de Datos
            </CardTitle>
          </div>
          <CardDescription>
            Registra tus solicitudes de acceso, rectificación, supresión,
            oposición o portabilidad conforme a la Ley 19.628 vigente y en
            preparación operativa para la Ley 21.719. El envío registra tu folio
            para revisión del equipo; no ejecuta cambios automáticos.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {feedback}
          <div className="flex flex-wrap gap-3">
            <Button
              type="button"
              variant="outline"
              onClick={() => handleRequest("acceso")}
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "acceso" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <FileText className="mr-2 size-4" aria-hidden="true" />
              )}
              Solicitar derecho de acceso
            </Button>

            <Button
              type="button"
              variant="outline"
              className="border-destructive/40 text-destructive hover:bg-destructive/10"
              onClick={() => handleRequest("supresion")}
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "supresion" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <Trash2 className="mr-2 size-4" aria-hidden="true" />
              )}
              Solicitar derecho de supresión
            </Button>
          </div>

          <form
            onSubmit={(event) => void handleDetailedRequest(event)}
            className="space-y-3 rounded-md border p-4"
          >
            <h3 className="text-sm font-medium">
              Otra solicitud o solicitud con detalle
            </h3>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="privacy-detailed-kind">Tipo de derecho</Label>
                <select
                  id="privacy-detailed-kind"
                  value={selectedKind}
                  onChange={(event) =>
                    setSelectedKind(event.target.value as RightsRequestKind)
                  }
                  disabled={
                    Boolean(loadingAction) ||
                    authSnapshot.status !== "authenticated"
                  }
                  className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                >
                  <option value="acceso">Acceso</option>
                  <option value="rectificacion">Rectificación</option>
                  <option value="supresion">Supresión</option>
                  <option value="oposicion">Oposición</option>
                  <option value="portabilidad">Portabilidad</option>
                  <option value="bloqueo">Bloqueo temporal</option>
                </select>
              </div>
            </div>
            <div className="space-y-1">
              <Label htmlFor="privacy-detailed-detail">
                Detalle de la solicitud
              </Label>
              <textarea
                id="privacy-detailed-detail"
                value={detail}
                maxLength={DETAIL_MAX_LENGTH}
                onChange={(event) => setDetail(event.target.value)}
                disabled={
                  Boolean(loadingAction) ||
                  authSnapshot.status !== "authenticated"
                }
                placeholder="Indica qué datos deseas revisar, corregir, oponerte, portar o bloquear (mínimo 10 caracteres para rectificación, oposición y bloqueo)."
                className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                aria-describedby="privacy-detailed-help"
              />
              <p
                id="privacy-detailed-help"
                className="text-xs text-muted-foreground"
              >
                {selectedKind === "rectificacion" ||
                selectedKind === "oposicion" ||
                selectedKind === "bloqueo"
                  ? `Obligatorio (mínimo ${MIN_REQUIRED_DETAIL_LENGTH} caracteres). `
                  : "Opcional. "}
                {detail.length}/{DETAIL_MAX_LENGTH}
              </p>
            </div>
            <Button
              type="submit"
              variant="outline"
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "detailed" && (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              )}
              Registrar solicitud con detalle
            </Button>
          </form>

          {requests.length > 0 && (
            <div className="pt-4 space-y-2">
              <h3 className="text-sm font-medium">Solicitudes registradas</h3>
              <div className="divide-y rounded-md border text-sm">
                {requests.map((r, i) => (
                  <div key={r.id ?? i} className="space-y-2 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <span className="font-medium capitalize">{r.kind}</span>
                        {r.created_at && (
                          <span className="text-xs text-muted-foreground ml-2">
                            {new Date(r.created_at).toLocaleDateString("es-CL")}
                          </span>
                        )}
                        {r.id && (
                          <p className="text-xs text-muted-foreground font-mono">
                            Folio: {r.id}
                          </p>
                        )}
                        {r.resolved_at && (
                          <p className="text-xs text-muted-foreground">
                            Actualizada el{" "}
                            {new Date(r.resolved_at).toLocaleDateString(
                              "es-CL",
                            )}
                          </p>
                        )}
                        {(r.extended_due_at || r.due_at) && (
                          <p className="text-xs text-muted-foreground">
                            {r.extended_due_at
                              ? `Plazo operativo prorrogado hasta: ${new Date(r.extended_due_at).toLocaleDateString("es-CL")}`
                              : `Plazo operativo estimado: ${new Date(r.due_at as string).toLocaleDateString("es-CL")}`}
                          </p>
                        )}
                      </div>
                      <Badge variant="secondary">{r.state}</Badge>
                    </div>
                    {r.detail && (
                      <p className="whitespace-pre-wrap text-xs text-muted-foreground">
                        {r.detail}
                      </p>
                    )}
                    {r.extension_reason && (
                      <p className="text-xs text-muted-foreground">
                        Motivo de prórroga: {r.extension_reason}
                      </p>
                    )}
                    {r.events && r.events.length > 0 && (
                      <div className="space-y-1 border-t pt-2">
                        <p className="text-xs font-medium text-muted-foreground">
                          Historial de seguimiento
                        </p>
                        <ul className="space-y-1 text-xs text-muted-foreground">
                          {r.events.map((ev) => (
                            <li key={ev.id}>
                              <span className="font-medium text-foreground">
                                {EVENT_LABELS[ev.event_type] ?? ev.event_type}
                              </span>{" "}
                              ·{" "}
                              {new Date(ev.created_at).toLocaleString("es-CL")}
                              {ev.detail ? ` — ${ev.detail}` : ""}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                    {r.response && (
                      <div className="rounded-md bg-muted/60 p-3">
                        <p className="text-xs font-medium text-muted-foreground">
                          Respuesta registrada en EspaciGo
                        </p>
                        <p className="mt-1 whitespace-pre-wrap text-sm">
                          {r.response}
                        </p>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    );
  }

  if (section === "exportacion") {
    return (
      <Card aria-labelledby="privacy-export-title">
        <CardHeader>
          <div className="flex items-center gap-2">
            <Download className="size-5 text-primary" aria-hidden="true" />
            <CardTitle id="privacy-export-title">
              Exportación de Datos Personales
            </CardTitle>
          </div>
          <CardDescription>
            Descarga directa de tu copia de datos en formato JSON estructurado o
            archivo ZIP completo.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {feedback}
          <div className="flex flex-wrap gap-3">
            <Button
              type="button"
              variant="outline"
              onClick={handleExportJson}
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "export-json" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <FileJson className="mr-2 size-4" aria-hidden="true" />
              )}
              Descargar exportación JSON
            </Button>

            <Button
              type="button"
              variant="outline"
              onClick={handleDownloadArchive}
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "export-archive" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <Archive className="mr-2 size-4" aria-hidden="true" />
              )}
              Descargar archivo ZIP
            </Button>

            <Button
              type="button"
              variant="outline"
              onClick={handleExportPortability}
              disabled={
                Boolean(loadingAction) ||
                authSnapshot.status !== "authenticated"
              }
            >
              {loadingAction === "export-portability" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <FileJson className="mr-2 size-4" aria-hidden="true" />
              )}
              Descargar portabilidad estructurada (JSON v1)
            </Button>
          </div>
          {portabilitySha256 && (
            <p className="text-xs text-muted-foreground break-all font-mono">
              SHA-256 (portabilidad v1): {portabilitySha256}
            </p>
          )}
        </CardContent>
      </Card>
    );
  }

  if (section === "preferencias") {
    return (
      <Card aria-labelledby="privacy-preferences-title">
        <CardHeader>
          <div className="flex items-center gap-2">
            <SlidersHorizontal
              className="size-5 text-primary"
              aria-hidden="true"
            />
            <CardTitle id="privacy-preferences-title">
              Preferencias de finalidad y oposición/retiro
            </CardTitle>
          </div>
          <CardDescription>
            Gestiona tu consentimiento u oposición para tratamientos opcionales
            de datos personales conforme a la Ley 21.719.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {prefError && (
            <div
              role="alert"
              className="flex items-center gap-2 rounded-md bg-destructive/15 p-3 text-sm text-destructive border border-destructive/20"
            >
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              <span>{prefError}</span>
            </div>
          )}

          {prefMessage && (
            <div
              role="status"
              className="flex items-center gap-2 rounded-md bg-emerald-50 p-3 text-sm text-emerald-800 border border-emerald-200 dark:bg-emerald-950/30 dark:border-emerald-800 dark:text-emerald-300"
            >
              <CheckCircle2 className="size-4 shrink-0" aria-hidden="true" />
              <span>{prefMessage}</span>
            </div>
          )}

          <div className="space-y-3">
            {PRIVACY_PURPOSES.map((purpose) => {
              const current = preferencesByPurpose.get(purpose);
              const currentState = current?.state;
              const selectedState = preferenceChoices[purpose] ?? currentState;
              const reasonInputValue = preferenceReasons[purpose] ?? "";
              const isBusy = loadingAction === `pref-${purpose}`;
              const titleId = `privacy-pref-title-${purpose}`;
              const reasonId = `privacy-pref-reason-${purpose}`;

              return (
                <section
                  key={purpose}
                  aria-labelledby={titleId}
                  className="space-y-3 rounded-md border p-4"
                >
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0 space-y-1">
                      <h3 id={titleId} className="text-sm font-medium">
                        {PRIVACY_PURPOSE_LABELS[purpose]}
                      </h3>
                      <p className="text-xs text-muted-foreground">
                        {PRIVACY_PURPOSE_DESCRIPTIONS[purpose]}
                      </p>
                    </div>
                    <Badge
                      variant={
                        currentState === "permitido" ? "secondary" : "outline"
                      }
                    >
                      {currentState
                        ? PRIVACY_PURPOSE_STATE_LABELS[currentState]
                        : "Sin preferencia registrada"}
                    </Badge>
                  </div>

                  {current?.reason && (
                    <p className="text-xs text-muted-foreground break-words">
                      Motivo registrado: {current.reason}
                    </p>
                  )}

                  {current?.updated_at && (
                    <p className="text-xs text-muted-foreground">
                      Última actualización:{" "}
                      {new Date(current.updated_at).toLocaleString("es-CL")}
                    </p>
                  )}

                  <div className="space-y-1">
                    <Label htmlFor={reasonId} className="text-xs">
                      Motivo o causal (opcional, 5 a 500 caracteres)
                    </Label>
                    <input
                      id={reasonId}
                      type="text"
                      maxLength={500}
                      value={reasonInputValue}
                      onChange={(event) =>
                        setPreferenceReasons((prev) => ({
                          ...prev,
                          [purpose]: event.target.value,
                        }))
                      }
                      disabled={
                        Boolean(loadingAction) ||
                        authSnapshot.status !== "authenticated"
                      }
                      placeholder="Indica opcionalmente el motivo de tu cambio de preferencia"
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    />
                  </div>

                  <fieldset
                    disabled={
                      Boolean(loadingAction) ||
                      authSnapshot.status !== "authenticated"
                    }
                    className="flex flex-wrap gap-x-5 gap-y-2"
                  >
                    <legend className="mb-2 text-xs font-medium">
                      Tu elección
                    </legend>
                    {PURPOSE_STATE_ACTIONS.map((action) => (
                      <label
                        key={action.state}
                        className="inline-flex cursor-pointer items-center gap-2 text-sm"
                      >
                        <input
                          type="radio"
                          name={`privacy-choice-${purpose}`}
                          className="size-4 accent-primary"
                          checked={selectedState === action.state}
                          onChange={() =>
                            setPreferenceChoices((prev) => ({
                              ...prev,
                              [purpose]: action.state,
                            }))
                          }
                        />
                        {action.label}
                      </label>
                    ))}
                  </fieldset>
                  <Button
                    type="button"
                    size="sm"
                    disabled={
                      !selectedState ||
                      Boolean(loadingAction) ||
                      authSnapshot.status !== "authenticated"
                    }
                    onClick={() => {
                      if (selectedState)
                        void handleUpdatePreference(purpose, selectedState);
                    }}
                  >
                    {isBusy && (
                      <Loader2
                        className="mr-1.5 size-3.5 animate-spin"
                        aria-hidden="true"
                      />
                    )}
                    Guardar preferencia
                  </Button>
                </section>
              );
            })}
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card aria-labelledby="privacy-restrictions-title">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Lock className="size-5 text-primary" aria-hidden="true" />
          <CardTitle id="privacy-restrictions-title">
            Bloqueos temporales de tratamiento
          </CardTitle>
        </div>
        <CardDescription>
          Restricciones o bloqueos temporales vigentes e históricos sobre el
          tratamiento de tus datos personales.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {restrictions.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No hay bloqueos temporales de tratamiento registrados en tu cuenta.
          </p>
        ) : (
          <div className="divide-y rounded-md border text-sm">
            {restrictions.map((block) => (
              <div key={block.id} className="space-y-2 p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="font-medium">
                    {PRIVACY_TREATMENT_BLOCK_SCOPE_LABELS[block.scope] ??
                      block.scope}
                  </span>
                  <Badge
                    variant={block.state === "activo" ? "default" : "secondary"}
                  >
                    {PRIVACY_TREATMENT_BLOCK_STATE_LABELS[block.state] ??
                      block.state}
                  </Badge>
                </div>
                <p className="text-xs text-muted-foreground break-words">
                  Motivo: {block.reason}
                </p>
                <p className="text-xs text-muted-foreground">
                  Aplicado el{" "}
                  {new Date(block.applied_at).toLocaleString("es-CL")}
                </p>
                {block.review_at && (
                  <p className="text-xs text-muted-foreground">
                    Revisión programada:{" "}
                    {new Date(block.review_at).toLocaleDateString("es-CL")}
                  </p>
                )}
                {block.lifted_at && (
                  <p className="text-xs text-muted-foreground">
                    Levantado el{" "}
                    {new Date(block.lifted_at).toLocaleString("es-CL")}
                  </p>
                )}
                {block.lift_reason && (
                  <p className="text-xs text-muted-foreground break-words">
                    Motivo de levantamiento: {block.lift_reason}
                  </p>
                )}
                <p className="text-xs text-muted-foreground font-mono break-all">
                  Solicitud asociada: {block.request_id}
                </p>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
