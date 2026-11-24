"use client";

import {
  ArrowDown,
  ArrowUp,
  Copy,
  Download,
  FileText,
  History,
  Plus,
  RefreshCw,
  Sparkles,
  X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  ContractDocumentView,
  contractOrdinalTitle,
  rateUnitWord,
  type TemplatePreviewInput,
} from "@/features/commercial-contracts/ui/contract-document-view";
import { printTemplatePreviewAsPdf } from "@/features/commercial-contracts/ui/contract-pdf-modal";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";
import { contractTemplateService } from "../contract-template-runtime";
import {
  containsContractContact,
  contractTemplateExamples,
  validContractSections,
  type ContractSection,
  type ContractTemplateRead,
  type ContractTemplateSource,
  type ContractTemplateVersion,
} from "../domain/contract-template";

export interface SpaceContractPreviewContext {
  readonly spaceTitle: string;
  readonly address: string;
  readonly cancellationPolicyText: string;
  readonly rateUnit?: string;
  readonly basePriceClp?: number | null;
}

const SUGGESTED_CLAUSES: Array<{ label: string; title: string; body: string }> =
  [
    {
      label: "Equipamiento e inventario",
      title: "Equipamiento e inventario incluido",
      body: "El arrendamiento incluye el mobiliario y equipamiento declarado en la publicación. El arrendatario se obliga a utilizarlo conforme a su destino y restituirlo en perfecto estado de funcionamiento.",
    },
    {
      label: "Ruido y convivencia",
      title: "Normas de ruido y convivencia",
      body: "El arrendatario deberá respetar los decibeles máximos permitidos, los reglamentos internos del recinto o comunidad y abstenerse de causar molestias a vecinos o terceros.",
    },
    {
      label: "Estacionamiento y accesos",
      title: "Accesos y estacionamientos",
      body: "El ingreso de personas y vehículos deberá ajustarse exclusivamente a los accesos habilitados y al aforo máximo autorizado para el espacio.",
    },
    {
      label: "Daños y aseo",
      title: "Responsabilidad por daños y limpieza",
      body: "Cualquier deterioro, pérdida de equipamiento o necesidad de limpieza extraordinaria imputable al arrendatario será de su exclusivo cargo.",
    },
  ];

interface Props {
  readonly spaceId: string;
  readonly sessionScope: string;
  readonly spacePreview?: SpaceContractPreviewContext;
  readonly onPublishedVersionChange?: (version: number) => void;
}

export function ContractTemplateEditor({
  spaceId,
  sessionScope,
  spacePreview,
  onPublishedVersionChange,
}: Props) {
  const { accessToken, getAccessToken, snapshot } = useAuth();
  const isCurrent = useCallback(
    () => getAccessToken() === accessToken,
    [getAccessToken, accessToken],
  );
  if (!accessToken || snapshot.restrictedMode) return null;
  return (
    <TemplateEditor
      key={`${spaceId}:${sessionScope}:${accessToken}`}
      spaceId={spaceId}
      spacePreview={spacePreview}
      onPublishedVersionChange={onPublishedVersionChange}
      isCurrent={isCurrent}
    />
  );
}

function errorMessage(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 412)
      return "La plantilla cambió en otra sesión. Recarga antes de guardar o publicar.";
    if (error.status === 401 || error.status === 403)
      return "Tu sesión no permite editar esta plantilla. Vuelve a ingresar.";
    if (error.status === 404)
      return "El espacio o la versión de origen ya no están disponibles para tu cuenta.";
    if (error.status === 422)
      return "Revisa las secciones: usa texto simple y respeta los límites indicados.";
  }
  return "No se pudo completar la operación. Recarga la plantilla para comprobar su estado antes de continuar.";
}

function TemplateEditor({
  spaceId,
  spacePreview,
  onPublishedVersionChange,
  isCurrent,
}: {
  spaceId: string;
  spacePreview?: SpaceContractPreviewContext;
  onPublishedVersionChange?: (version: number) => void;
  isCurrent(): boolean;
}) {
  const [read, setRead] = useState<ContractTemplateRead | null>(null);
  const [sections, setSections] = useState<ContractSection[]>([]);
  const [versions, setVersions] = useState<ContractTemplateVersion[]>([]);
  const [sources, setSources] = useState<ContractTemplateSource[]>([]);
  const [source, setSource] = useState("");
  const [sourceVersions, setSourceVersions] = useState<
    ContractTemplateVersion[]
  >([]);
  const [sourceVersion, setSourceVersion] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [sourceLoading, setSourceLoading] = useState(false);
  const [needsReload, setNeedsReload] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [editorOpen, setEditorOpen] = useState(false);
  const [mobileTab, setMobileTab] = useState<"edit" | "preview">("edit");
  const [footerPanel, setFooterPanel] = useState<"none" | "versions" | "copy">(
    "none",
  );
  const [selectedVersionNum, setSelectedVersionNum] = useState("");
  const alive = useRef(true);
  const selectedSource = useRef("");
  const permitted = () => alive.current && isCurrent();
  const apply = useCallback(
    (loaded: Awaited<ReturnType<typeof contractTemplateService.load>>) => {
      setRead(loaded);
      setSections(
        loaded.draft.sections.length
          ? loaded.draft.sections.map((s) => ({ ...s }))
          : contractTemplateExamples.map((s) => ({ ...s })),
      );
      setVersions(loaded.versions);
      setSelectedVersionNum(
        loaded.versions[0] ? String(loaded.versions[0].version) : "",
      );
      setSources(loaded.sources);
      setNeedsReload(false);
      onPublishedVersionChange?.(loaded.draft.published_version ?? 0);
    },
    [onPublishedVersionChange],
  );
  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    contractTemplateService
      .load(spaceId, controller.signal)
      .then((loaded) => {
        if (alive.current && isCurrent()) apply(loaded);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted && alive.current && isCurrent()) {
          setError(errorMessage(e));
          setNeedsReload(true);
        }
      })
      .finally(() => {
        if (alive.current && isCurrent()) setLoading(false);
      });
    return () => {
      alive.current = false;
      controller.abort();
    };
  }, [spaceId, apply, isCurrent]);

  const dirty =
    read !== null &&
    JSON.stringify(sections) !== JSON.stringify(read.draft.sections);
  const valid = validContractSections(sections);
  const disabled = loading || busy || needsReload || read === null;
  const mutate = async (operation: () => Promise<void>, success: string) => {
    if (disabled || !permitted()) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await operation();
      if (permitted()) setNotice(success);
    } catch (e) {
      if (permitted()) {
        setError(errorMessage(e));
        setNeedsReload(true);
      }
    } finally {
      if (permitted()) setBusy(false);
    }
  };
  const reload = async () => {
    if (busy || loading || !permitted()) return;
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const loaded = await contractTemplateService.load(spaceId);
      if (permitted()) apply(loaded);
    } catch (e) {
      if (permitted()) {
        setError(errorMessage(e));
        setNeedsReload(true);
      }
    } finally {
      if (permitted()) setLoading(false);
    }
  };
  const selectSource = async (id: string) => {
    setSource(id);
    selectedSource.current = id;
    setSourceVersion("");
    setSourceVersions([]);
    setError("");
    setSourceLoading(false);
    if (!id) return;
    setSourceLoading(true);
    try {
      const items = await contractTemplateService.versions(id);
      if (permitted() && selectedSource.current === id) {
        setSourceVersions(items);
        setSourceVersion(items[0] ? String(items[0].version) : "");
      }
    } catch (e) {
      if (permitted() && selectedSource.current === id)
        setError(errorMessage(e));
    } finally {
      if (permitted() && selectedSource.current === id) setSourceLoading(false);
    }
  };
  const update = (index: number, field: "title" | "body", value: string) =>
    setSections((previous) =>
      previous.map((section, i) =>
        i === index ? { ...section, [field]: value } : section,
      ),
    );

  const moveSection = (index: number, delta: -1 | 1) => {
    const target = index + delta;
    if (target < 0 || target >= sections.length) return;
    setSections((previous) => {
      const next = [...previous];
      const [moved] = next.splice(index, 1);
      next.splice(target, 0, moved);
      return next;
    });
  };

  const closeEditor = useCallback(() => {
    setEditorOpen(false);
    setFooterPanel("none");
  }, []);

  useEffect(() => {
    if (!editorOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeEditor();
    };
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = prevOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [editorOpen, closeEditor]);

  const saveDraft = () =>
    void mutate(async () => {
      if (!read) return;
      const saved = await contractTemplateService.save(
        spaceId,
        sections,
        read.etag,
      );
      if (permitted()) setRead(saved);
    }, "Borrador guardado.");

  const publishDraft = () =>
    void mutate(async () => {
      if (!read) return;
      await contractTemplateService.publish(spaceId, read.etag);
      const loaded = await contractTemplateService.load(spaceId);
      if (permitted()) apply(loaded);
    }, "Versión publicada. Las solicitudes anteriores conservan sus condiciones.");

  const copyFromSource = () =>
    void mutate(async () => {
      if (!read) return;
      const copied = await contractTemplateService.copy(
        spaceId,
        read.etag,
        source,
        Number(sourceVersion),
      );
      if (permitted()) {
        setRead(copied);
        setSections(copied.draft.sections.map((s) => ({ ...s })));
        setFooterPanel("none");
      }
    }, "Reglas copiadas al borrador. Revísalas antes de publicar.");

  const loadVersionIntoDraft = (versionItem: ContractTemplateVersion) => {
    setSections(versionItem.sections.map((s) => ({ ...s })));
    setError("");
    setNotice(
      `Cláusulas de la versión v${versionItem.version} cargadas en el borrador. Guarda para conservar los cambios.`,
    );
    setFooterPanel("none");
  };

  const previewInput: TemplatePreviewInput = {
    spaceTitle: spacePreview?.spaceTitle || "Espacio publicado",
    address: spacePreview?.address || "Dirección confirmada del espacio",
    cancellationPolicyText:
      spacePreview?.cancellationPolicyText ||
      "Política de cancelación configurada en el espacio.",
    sections,
    templateVersion: read?.draft.published_version || undefined,
    rateUnit: spacePreview?.rateUnit,
    basePriceClp: spacePreview?.basePriceClp,
  };

  const tariffDisplay =
    previewInput.basePriceClp && previewInput.basePriceClp > 0
      ? `$${previewInput.basePriceClp.toLocaleString("es-CL")} CLP por ${rateUnitWord(previewInput.rateUnit ?? "hora", 1)}`
      : "Por definir";

  const inspectedVersion =
    versions.find((v) => String(v.version) === selectedVersionNum) ??
    versions[0] ??
    null;

  const publishedVersion = read?.draft.published_version ?? 0;

  return (
    <section
      aria-labelledby="contract-template-title"
      className="space-y-5 rounded-xl border border-border bg-card p-5"
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <h2
                id="contract-template-title"
                className="text-lg font-semibold"
              >
                Plantilla de contrato
              </h2>
              {read && (
                <>
                  {publishedVersion > 0 ? (
                    <span className="inline-flex items-center rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-0.5 text-xs font-medium text-emerald-800">
                      Publicada · Versión {publishedVersion}
                    </span>
                  ) : (
                    <span className="inline-flex items-center rounded-full border border-amber-200 bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-800">
                      Borrador sin publicar
                    </span>
                  )}
                  {dirty && (
                    <span className="inline-flex items-center rounded-full border border-amber-200 bg-amber-50/70 px-2.5 py-0.5 text-xs font-medium text-amber-700">
                      Cambios sin guardar
                    </span>
                  )}
                </>
              )}
            </div>
            <p className="max-w-3xl text-sm text-muted-foreground">
              Define las cláusulas particulares de este espacio con vista previa
              en tiempo real del contrato oficial de EspaciGo. Las partes,
              ubicación, horario, precio y política de cancelación se completan
              automáticamente en cada reserva.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            onClick={() => setEditorOpen(true)}
            disabled={!read || loading}
          >
            <FileText className="size-4" aria-hidden="true" />
            <span>Editar contrato y vista previa</span>
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => printTemplatePreviewAsPdf(previewInput)}
            disabled={!read || loading}
          >
            <Download className="size-4" aria-hidden="true" />
            <span>Descargar PDF de muestra</span>
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => void reload()}
            disabled={busy || loading}
          >
            <RefreshCw
              className={cn("size-4", loading && "animate-spin")}
              aria-hidden="true"
            />
            <span>Recargar</span>
          </Button>
        </div>
      </div>

      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          Cargando plantilla…
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-red-800">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="text-sm text-green-800">
          {notice}
        </p>
      )}
      {needsReload && (
        <p className="text-sm text-muted-foreground">
          Recargar reemplaza las secciones visibles con el último borrador
          guardado.
        </p>
      )}

      {read && (
        <div className="space-y-4">
          <dl className="grid gap-x-4 gap-y-3 rounded-lg border border-border/80 bg-muted/30 p-3.5 text-xs sm:grid-cols-3">
            <div>
              <dt className="font-medium text-muted-foreground">Inmueble</dt>
              <dd className="mt-0.5 font-semibold text-foreground">
                {previewInput.spaceTitle}
              </dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Dirección</dt>
              <dd className="mt-0.5 font-semibold text-foreground">
                {previewInput.address}
              </dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Tarifa</dt>
              <dd className="mt-0.5 font-semibold text-foreground">
                {tariffDisplay}
              </dd>
            </div>
            <div className="border-t border-border/60 pt-2.5 sm:col-span-3">
              <dt className="font-medium text-muted-foreground">
                Política de cancelación
              </dt>
              <dd className="mt-0.5 text-foreground">
                {previewInput.cancellationPolicyText}
              </dd>
            </div>
          </dl>

          <div className="space-y-2.5">
            <div className="flex items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-foreground">
                Cláusulas particulares configuradas ({sections.length}/12)
              </h3>
            </div>
            <ul className="grid gap-2.5 sm:grid-cols-2">
              {sections.map((section, index) => (
                <li
                  key={section.key}
                  className="rounded-lg border border-border/80 bg-background/60 p-3"
                >
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] font-bold uppercase tracking-wider text-[#702a1b]">
                      {contractOrdinalTitle(index)}
                    </span>
                    <span className="text-xs text-muted-foreground">·</span>
                    <p className="truncate text-sm font-medium text-foreground">
                      {section.title || "Sin título"}
                    </p>
                  </div>
                  <p className="mt-1 whitespace-pre-wrap text-xs leading-relaxed text-muted-foreground line-clamp-2">
                    {section.body || "Sin reglas definidas."}
                  </p>
                </li>
              ))}
            </ul>
            <p className="text-xs text-muted-foreground">
              La edición de cláusulas, el guardado de borradores, la publicación
              de versiones, el historial y la copia desde otro espacio propio se
              gestionan desde el editor dinámico.
            </p>
          </div>
        </div>
      )}

      {editorOpen && read && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Editor dinámico de contrato"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-2 sm:p-4"
        >
          <div className="flex h-[92vh] w-full max-w-7xl flex-col overflow-hidden rounded-2xl border border-border bg-white shadow-2xl">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-white px-4 py-3 sm:px-6">
              <div className="flex items-center gap-2.5 min-w-0">
                <FileText
                  className="size-5 shrink-0 text-[#a44731]"
                  aria-hidden="true"
                />
                <div className="min-w-0">
                  <h3 className="truncate text-base font-semibold text-gray-900">
                    Editor dinámico de contrato — {previewInput.spaceTitle}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {read.draft.published_version
                      ? `Versión publicada: v${read.draft.published_version}`
                      : "Borrador sin publicar"}{" "}
                    · {sections.length}/12 cláusulas particulares
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <div className="flex rounded-lg border border-border p-0.5 lg:hidden">
                  <button
                    type="button"
                    onClick={() => setMobileTab("edit")}
                    className={cn(
                      "rounded-md px-2.5 py-1 text-xs font-medium",
                      mobileTab === "edit"
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground",
                    )}
                  >
                    Editar cláusulas
                  </button>
                  <button
                    type="button"
                    onClick={() => setMobileTab("preview")}
                    className={cn(
                      "rounded-md px-2.5 py-1 text-xs font-medium",
                      mobileTab === "preview"
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground",
                    )}
                  >
                    Vista previa
                  </button>
                </div>
                <button
                  type="button"
                  aria-label="Cerrar editor de contrato"
                  onClick={closeEditor}
                  className="inline-flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                >
                  <X className="size-5" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div className="grid flex-1 min-h-0 lg:grid-cols-[42%_58%]">
              {/* Left panel: Dynamic form */}
              <div
                className={cn(
                  "overflow-y-auto border-r border-border p-4 sm:p-5 space-y-5",
                  mobileTab !== "edit" && "hidden lg:block",
                )}
              >
                {error && (
                  <p role="alert" className="text-sm text-red-800">
                    {error}
                  </p>
                )}
                {notice && (
                  <p role="status" className="text-sm text-green-800">
                    {notice}
                  </p>
                )}
                <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs space-y-2">
                  <p className="font-semibold text-foreground">
                    Resumen autocompletado del inmueble
                  </p>
                  <dl className="space-y-1 text-muted-foreground">
                    <div>
                      <dt className="inline font-semibold text-foreground">
                        Espacio:
                      </dt>{" "}
                      <dd className="inline">{previewInput.spaceTitle}</dd>
                    </div>
                    <div>
                      <dt className="inline font-semibold text-foreground">
                        Dirección:
                      </dt>{" "}
                      <dd className="inline">{previewInput.address}</dd>
                    </div>
                    <div>
                      <dt className="inline font-semibold text-foreground">
                        Tarifa:
                      </dt>{" "}
                      <dd className="inline">{tariffDisplay}</dd>
                    </div>
                    <div>
                      <dt className="inline font-semibold text-foreground">
                        Política de cancelación:
                      </dt>{" "}
                      <dd className="inline">
                        {previewInput.cancellationPolicyText}
                      </dd>
                    </div>
                  </dl>
                  <details className="rounded-md border border-border/80 bg-white/80 px-2.5 py-2">
                    <summary className="cursor-pointer font-medium text-foreground select-none">
                      Cláusulas base protegidas (PRIMERO–CUARTO)
                    </summary>
                    <ul className="mt-2 space-y-1 text-[11px] leading-relaxed text-muted-foreground">
                      <li>
                        <strong>PRIMERO (Inmueble):</strong> identifica el
                        espacio y su dirección confirmada.
                      </li>
                      <li>
                        <strong>SEGUNDO (Objeto y uso):</strong> entrega el
                        espacio en arriendo y prohíbe subarrendar o darle uso
                        ilícito.
                      </li>
                      <li>
                        <strong>TERCERO (Vigencia y restitución):</strong>{" "}
                        define fecha/hora de inicio y término de la reserva y la
                        obligación de restitución.
                      </li>
                      <li>
                        <strong>CUARTO (Canon y pago):</strong> fija la renta en
                        CLP según tarifa y duración, pagadera vía EspaciGo /
                        Mercado Pago.
                      </li>
                    </ul>
                    <p className="mt-1.5 text-[11px] text-muted-foreground">
                      La Comparecencia, Política de cancelación y Jurisdicción
                      (FES Ley N.º 19.799) también se completan automáticamente.
                    </p>
                  </details>
                </div>

                <div className="space-y-2">
                  <p className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                    <Sparkles
                      className="size-3.5 text-[#a44731]"
                      aria-hidden="true"
                    />
                    <span>Catálogo rápido de cláusulas sugeridas</span>
                  </p>
                  <div className="flex flex-wrap gap-1.5">
                    {SUGGESTED_CLAUSES.map((clause) => (
                      <button
                        key={clause.label}
                        type="button"
                        disabled={disabled || sections.length >= 12}
                        onClick={() =>
                          setSections((previous) => [
                            ...previous,
                            {
                              key: `clausula_${crypto.randomUUID().slice(0, 8)}`,
                              title: clause.title,
                              body: clause.body,
                            },
                          ])
                        }
                        className="inline-flex items-center gap-1 rounded-full border border-border bg-white px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-secondary disabled:opacity-50"
                      >
                        <Plus className="size-3" aria-hidden="true" />
                        <span>{clause.label}</span>
                      </button>
                    ))}
                  </div>
                </div>

                <div className="space-y-3">
                  {sections.map((section, index) => {
                    const hasContact =
                      containsContractContact(section.title) ||
                      containsContractContact(section.body);
                    return (
                      <div
                        key={section.key}
                        className="space-y-2.5 rounded-lg border border-border bg-white p-3.5"
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-xs font-bold uppercase tracking-wider text-[#702a1b]">
                            {contractOrdinalTitle(index)}
                          </span>
                          <div className="flex items-center gap-1">
                            <button
                              type="button"
                              aria-label={`Subir cláusula ${index + 1}`}
                              disabled={disabled || index === 0}
                              onClick={() => moveSection(index, -1)}
                              className="inline-flex size-7 items-center justify-center rounded border border-border text-muted-foreground hover:bg-secondary disabled:opacity-40"
                            >
                              <ArrowUp
                                className="size-3.5"
                                aria-hidden="true"
                              />
                            </button>
                            <button
                              type="button"
                              aria-label={`Bajar cláusula ${index + 1}`}
                              disabled={
                                disabled || index === sections.length - 1
                              }
                              onClick={() => moveSection(index, 1)}
                              className="inline-flex size-7 items-center justify-center rounded border border-border text-muted-foreground hover:bg-secondary disabled:opacity-40"
                            >
                              <ArrowDown
                                className="size-3.5"
                                aria-hidden="true"
                              />
                            </button>
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              disabled={disabled || sections.length <= 1}
                              onClick={() =>
                                setSections((previous) =>
                                  previous.filter((_, i) => i !== index),
                                )
                              }
                            >
                              Quitar
                            </Button>
                          </div>
                        </div>
                        <Input
                          value={section.title}
                          maxLength={100}
                          disabled={disabled}
                          placeholder="Título de la cláusula"
                          onChange={(e) =>
                            update(index, "title", e.target.value)
                          }
                        />
                        <textarea
                          className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
                          value={section.body}
                          maxLength={4000}
                          disabled={disabled}
                          placeholder="Redacta las condiciones de esta cláusula..."
                          onChange={(e) =>
                            update(index, "body", e.target.value)
                          }
                        />
                        {hasContact && (
                          <p className="text-xs font-medium text-destructive">
                            No incluyas teléfonos, correos electrónicos ni
                            enlaces en las cláusulas.
                          </p>
                        )}
                      </div>
                    );
                  })}
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full"
                    disabled={disabled || sections.length >= 12}
                    onClick={() =>
                      setSections((previous) => [
                        ...previous,
                        {
                          key: `seccion_${crypto.randomUUID().slice(0, 8)}`,
                          title: "Nueva cláusula",
                          body: "",
                        },
                      ])
                    }
                  >
                    <Plus className="size-4" aria-hidden="true" />
                    <span>Agregar cláusula personalizada</span>
                  </Button>
                </div>
              </div>

              {/* Right panel: Live A4/Letter preview */}
              <div
                className={cn(
                  "overflow-y-auto bg-neutral-100 p-3 sm:p-6",
                  mobileTab !== "preview" && "hidden lg:block",
                )}
              >
                <ContractDocumentView templatePreview={previewInput} />
              </div>
            </div>

            {/* Sticky modal footer */}
            <div className="border-t border-border bg-white">
              {footerPanel === "versions" && versions.length > 0 && (
                <div className="max-h-60 overflow-y-auto border-b border-border bg-muted/20 px-4 py-3 sm:px-6 space-y-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Label
                        htmlFor="modal-version-select"
                        className="text-xs font-semibold"
                      >
                        Versión publicada a consultar
                      </Label>
                      <select
                        id="modal-version-select"
                        value={
                          inspectedVersion
                            ? String(inspectedVersion.version)
                            : ""
                        }
                        onChange={(e) => setSelectedVersionNum(e.target.value)}
                        className="h-8 rounded-md border border-input bg-background px-2.5 text-xs"
                      >
                        {versions.map((v) => (
                          <option key={v.id} value={v.version}>
                            Versión {v.version} ·{" "}
                            {new Date(v.published_at).toLocaleDateString(
                              "es-CL",
                            )}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="flex items-center gap-2">
                      {inspectedVersion && (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={disabled}
                          onClick={() => loadVersionIntoDraft(inspectedVersion)}
                        >
                          Cargar en borrador
                        </Button>
                      )}
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => setFooterPanel("none")}
                      >
                        Ocultar
                      </Button>
                    </div>
                  </div>
                  {inspectedVersion && (
                    <div className="grid gap-2 sm:grid-cols-2">
                      {inspectedVersion.sections.map((s, idx) => (
                        <div
                          key={s.key}
                          className="rounded-md border border-border bg-white p-2.5 text-xs"
                        >
                          <p className="font-semibold text-foreground">
                            {contractOrdinalTitle(idx)}. {s.title}
                          </p>
                          <p className="mt-1 whitespace-pre-wrap text-muted-foreground line-clamp-4">
                            {s.body}
                          </p>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {footerPanel === "copy" && sources.length > 0 && (
                <div className="border-b border-border bg-muted/20 px-4 py-3 sm:px-6 space-y-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="text-xs text-muted-foreground">
                      La copia reemplaza este borrador y mantiene intacta la
                      versión publicada.
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setFooterPanel("none")}
                    >
                      Ocultar
                    </Button>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label htmlFor="modal-template-copy-space">
                        Espacio de origen
                      </Label>
                      <select
                        id="modal-template-copy-space"
                        value={source}
                        disabled={disabled}
                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                        onChange={(e) => void selectSource(e.target.value)}
                      >
                        <option value="">Elige un espacio</option>
                        {sources.map((s) => (
                          <option key={s.id} value={s.id}>
                            {s.title}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="modal-template-copy-version">
                        Versión publicada
                      </Label>
                      <select
                        id="modal-template-copy-version"
                        value={sourceVersion}
                        disabled={
                          disabled || sourceLoading || !sourceVersions.length
                        }
                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                        onChange={(e) => setSourceVersion(e.target.value)}
                      >
                        <option value="">
                          {sourceLoading
                            ? "Cargando versiones…"
                            : "Elige una versión"}
                        </option>
                        {sourceVersions.map((v) => (
                          <option key={v.id} value={v.version}>
                            Versión {v.version}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>
                  {source && !sourceLoading && sourceVersions.length === 0 && (
                    <p className="mt-2 text-xs text-muted-foreground">
                      Este espacio aún no tiene una plantilla publicada.
                    </p>
                  )}
                  <div className="flex justify-end">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={
                        disabled || sourceLoading || !source || !sourceVersion
                      }
                      onClick={copyFromSource}
                    >
                      Copiar al borrador
                    </Button>
                  </div>
                </div>
              )}

              <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-6">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-xs text-muted-foreground mr-1">
                    {!valid
                      ? "Completa todos los títulos y cláusulas sin datos de contacto para guardar."
                      : dirty
                        ? "Tienes cambios sin guardar en el borrador."
                        : "Borrador al día."}
                  </p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => printTemplatePreviewAsPdf(previewInput)}
                  >
                    <Download className="size-3.5" aria-hidden="true" />
                    <span>Descargar PDF de muestra</span>
                  </Button>
                  {versions.length > 0 && (
                    <Button
                      type="button"
                      size="sm"
                      variant={
                        footerPanel === "versions" ? "secondary" : "outline"
                      }
                      aria-expanded={footerPanel === "versions"}
                      onClick={() =>
                        setFooterPanel((prev) =>
                          prev === "versions" ? "none" : "versions",
                        )
                      }
                    >
                      <History className="size-3.5" aria-hidden="true" />
                      <span>Versiones ({versions.length})</span>
                    </Button>
                  )}
                  {sources.length > 0 && (
                    <Button
                      type="button"
                      size="sm"
                      variant={footerPanel === "copy" ? "secondary" : "outline"}
                      aria-expanded={footerPanel === "copy"}
                      onClick={() =>
                        setFooterPanel((prev) =>
                          prev === "copy" ? "none" : "copy",
                        )
                      }
                    >
                      <Copy className="size-3.5" aria-hidden="true" />
                      <span>Copiar de otro espacio</span>
                    </Button>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="button" variant="outline" onClick={closeEditor}>
                    Cerrar
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={disabled || !dirty || !valid}
                    onClick={saveDraft}
                  >
                    Guardar borrador
                  </Button>
                  <Button
                    type="button"
                    disabled={
                      disabled || dirty || !valid || read.draft.revision === 0
                    }
                    onClick={publishDraft}
                  >
                    Publicar versión
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
