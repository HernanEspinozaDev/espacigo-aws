"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import { contractTemplateService } from "../contract-template-runtime";
import {
  contractTemplateExamples,
  validContractSections,
  type ContractSection,
  type ContractTemplateRead,
  type ContractTemplateSource,
  type ContractTemplateVersion,
} from "../domain/contract-template";

interface Props {
  readonly spaceId: string;
  readonly sessionScope: string;
}

export function ContractTemplateEditor({ spaceId, sessionScope }: Props) {
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
  isCurrent,
}: {
  spaceId: string;
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
      setSources(loaded.sources);
      setNeedsReload(false);
    },
    [],
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

  return (
    <section
      aria-labelledby="contract-template-title"
      className="space-y-5 rounded-xl border border-border bg-card p-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="contract-template-title" className="text-lg font-semibold">
            Plantilla de contrato
          </h2>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            Define las reglas de este espacio por secciones. EspaciGo completará
            las partes, la ubicación confirmada, el horario, el precio y la
            política de cancelación al generar cada acuerdo.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void reload()}
          disabled={busy || loading}
        >
          Recargar plantilla
        </Button>
      </div>
      {loading && <p role="status">Cargando plantilla…</p>}
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
        <>
          <p className="text-sm">
            {read.draft.published_version
              ? `Versión publicada: ${read.draft.published_version}.`
              : "Aún no hay una versión publicada."}{" "}
            Las nuevas versiones se aplican a solicitudes nuevas.
          </p>
          <p className="text-sm text-muted-foreground">
            Hasta 12 secciones, 100 caracteres por título y 4.000 por sección;
            máximo 20.000 caracteres en total. Usa texto simple. No incluyas
            contactos ni códigos de acceso.
          </p>
          <fieldset disabled={disabled} className="space-y-5">
            <legend className="sr-only">Secciones del contrato</legend>
            {sections.map((section, index) => (
              <div
                key={section.key}
                className="space-y-3 rounded-lg border border-border p-4"
              >
                <div className="flex items-end gap-3">
                  <div className="flex-1 space-y-2">
                    <Label htmlFor={`clause-title-${section.key}`}>
                      Título de la sección {index + 1}
                    </Label>
                    <Input
                      id={`clause-title-${section.key}`}
                      value={section.title}
                      maxLength={100}
                      onChange={(e) => update(index, "title", e.target.value)}
                    />
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={sections.length <= 1}
                    aria-label={`Quitar sección ${index + 1}`}
                    onClick={() =>
                      setSections((previous) =>
                        previous.filter((_, i) => i !== index),
                      )
                    }
                  >
                    Quitar
                  </Button>
                </div>
                <Label htmlFor={`clause-body-${section.key}`}>
                  Reglas y condiciones
                </Label>
                <textarea
                  id={`clause-body-${section.key}`}
                  className="min-h-28 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
                  value={section.body}
                  maxLength={4000}
                  onChange={(e) => update(index, "body", e.target.value)}
                />
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              disabled={sections.length >= 12}
              onClick={() =>
                setSections((previous) => [
                  ...previous,
                  {
                    key: `seccion_${crypto.randomUUID().slice(0, 8)}`,
                    title: "Nueva sección",
                    body: "",
                  },
                ])
              }
            >
              Agregar sección
            </Button>
          </fieldset>
          {!valid && (
            <p className="text-sm text-muted-foreground">
              Completa todos los títulos y reglas con texto simple antes de
              guardar.
            </p>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant="outline"
              disabled={disabled || !dirty || !valid}
              onClick={() =>
                void mutate(async () => {
                  if (!read) return;
                  const saved = await contractTemplateService.save(
                    spaceId,
                    sections,
                    read.etag,
                  );
                  if (permitted()) setRead(saved);
                }, "Borrador guardado.")
              }
            >
              Guardar borrador
            </Button>
            <Button
              type="button"
              disabled={
                disabled || dirty || !valid || read.draft.revision === 0
              }
              onClick={() =>
                void mutate(async () => {
                  if (!read) return;
                  await contractTemplateService.publish(spaceId, read.etag);
                  const loaded = await contractTemplateService.load(spaceId);
                  if (permitted()) apply(loaded);
                }, "Versión publicada. Las solicitudes anteriores conservan sus condiciones.")
              }
            >
              Publicar versión
            </Button>
            {dirty && (
              <span className="text-sm text-muted-foreground">
                Guarda el borrador antes de publicarlo.
              </span>
            )}
            {busy && (
              <span role="status" className="text-sm">
                Guardando cambios…
              </span>
            )}
          </div>
          {sources.length > 0 && (
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer font-medium">
                Copiar reglas de otro espacio propio
              </summary>
              <p className="my-3 text-sm text-muted-foreground">
                La copia reemplaza este borrador y mantiene intacta la versión
                publicada. Elige una versión de origen.
              </p>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="template-copy-space">Espacio de origen</Label>
                  <select
                    id="template-copy-space"
                    value={source}
                    disabled={disabled}
                    className="h-10 w-full rounded-md border border-input bg-background px-3"
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
                <div className="space-y-2">
                  <Label htmlFor="template-copy-version">
                    Versión publicada
                  </Label>
                  <select
                    id="template-copy-version"
                    value={sourceVersion}
                    disabled={
                      disabled || sourceLoading || !sourceVersions.length
                    }
                    className="h-10 w-full rounded-md border border-input bg-background px-3"
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
                <p className="mt-2 text-sm">
                  Este espacio aún no tiene una plantilla publicada.
                </p>
              )}
              <Button
                className="mt-3"
                type="button"
                variant="outline"
                disabled={
                  disabled || sourceLoading || !source || !sourceVersion
                }
                onClick={() =>
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
                    }
                  }, "Reglas copiadas al borrador. Revísalas antes de publicar.")
                }
              >
                Copiar al borrador
              </Button>
            </details>
          )}
          {versions.length > 0 && (
            <details className="rounded-lg border border-border p-4">
              <summary className="cursor-pointer font-medium">
                Versiones publicadas ({versions.length})
              </summary>
              <ul className="mt-3 space-y-4">
                {versions.map((v) => (
                  <li key={v.id}>
                    <details>
                      <summary className="cursor-pointer text-sm">
                        Versión {v.version} ·{" "}
                        {new Date(v.published_at).toLocaleDateString("es-CL")}
                      </summary>
                      <div className="mt-2 space-y-3">
                        {v.sections.map((s) => (
                          <div key={s.key}>
                            <h3 className="font-medium">{s.title}</h3>
                            <p className="whitespace-pre-wrap text-sm">
                              {s.body}
                            </p>
                          </div>
                        ))}
                      </div>
                    </details>
                  </li>
                ))}
              </ul>
            </details>
          )}
        </>
      )}
    </section>
  );
}
