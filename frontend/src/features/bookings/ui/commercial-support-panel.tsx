"use client";

import { useEffect, useId, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { Button } from "@/components/workspace/ui/button";
import {
  commercialAftercare,
  type CommercialSupportCase,
} from "../infrastructure/commercial-aftercare-openapi-adapter";
import { useAftercareRequest } from "./use-aftercare-request";

const categories = {
  problemas_acceso: "Problemas de acceso",
  condiciones_diferentes: "Condiciones distintas a las acordadas",
  incumplimiento_normas: "Incumplimiento de normas",
  espacio_no_disponible: "Espacio no disponible",
  problema_pago_reembolso: "Problema de pago o reembolso",
  mediacion_directa: "Solicitar mediación de EspaciGo",
  dano_inmueble: "Reclamar daños del inmueble",
};
const actions: Record<string, string> = {
  abrir: "Caso abierto",
  responder: "Responder",
  escalar: "Escalar a mediación de EspaciGo",
  descargo: "Presentar descargo",
  resolver: "Registrar resolución",
  cerrar: "Cerrar caso",
  adjuntar: "Adjuntar evidencia",
};
const statuses: Record<string, string> = {
  esperando_contraparte: "En conversación con la contraparte",
  en_mediacion_espacigo: "En mediación de EspaciGo",
  resuelto: "Resuelto",
  cerrado: "Cerrado",
};
const inputStyle = "w-full rounded-md border bg-background p-2";

function labelCategory(category: string) {
  return categories[category as keyof typeof categories] ?? category;
}

export function CommercialSupportPanel({
  reservationId,
}: {
  reservationId: string;
}) {
  const { snapshot } = useAuth();
  if (snapshot.status !== "authenticated") return null;
  return (
    <CommercialCases
      key={`${snapshot.accountId}:${reservationId}`}
      reservationId={reservationId}
    />
  );
}

export function CommercialCases({ reservationId }: { reservationId?: string }) {
  const [data, setData] =
    useState<Awaited<ReturnType<typeof commercialAftercare.cases>>>();
  const [loadError, setLoadError] = useState(false);
  const [category, setCategory] = useState("problemas_acceso");
  const [description, setDescription] = useState("");
  const [revision, setRevision] = useState(0);
  const mutation = useAftercareRequest();
  const formID = useId();

  useEffect(() => {
    let current = true;
    commercialAftercare
      .cases(reservationId)
      .then((response) => {
        if (current) {
          setData(response);
          setLoadError(false);
        }
      })
      .catch(() => {
        if (current) setLoadError(true);
      });
    return () => {
      current = false;
    };
  }, [reservationId, revision]);

  const update = (item: CommercialSupportCase) => {
    setData((current) => {
      if (!current) return current;
      return {
        ...current,
        can_claim_damage:
          current.can_claim_damage && item.category !== "dano_inmueble",
        items: current.items.some((existing) => existing.id === item.id)
          ? current.items.map((existing) =>
              existing.id === item.id ? item : existing,
            )
          : [item, ...current.items],
      };
    });
  };

  return (
    <section
      aria-label="Casos de soporte y reclamos"
      className="space-y-4 rounded-lg border p-4"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">
          {reservationId
            ? "Casos de soporte"
            : "Mediaciones de reservas comerciales"}
        </h2>
        <Button
          variant="outline"
          onClick={() => setRevision((value) => value + 1)}
        >
          Actualizar
        </Button>
      </div>
      {loadError && <p role="alert">No se pudieron cargar los casos.</p>}
      {data?.items.length === 0 && <p>No hay casos registrados.</p>}
      {data?.items.map((item) => (
        <CaseCard
          key={item.id}
          item={item}
          reservationId={reservationId}
          update={update}
          reload={() => setRevision((value) => value + 1)}
        />
      ))}
      {data && reservationId && (
        <form
          className="space-y-2 border-t pt-4"
          onSubmit={(event) => {
            event.preventDefault();
            void mutation.run(`${category}:${description}`, async (key) => {
              update(
                await commercialAftercare.openCase(
                  reservationId,
                  category,
                  description,
                  key,
                ),
              );
              setCategory("problemas_acceso");
              setDescription("");
            });
          }}
        >
          <label htmlFor={`${formID}-category`}>Motivo del caso</label>
          <select
            id={`${formID}-category`}
            value={category}
            className={inputStyle}
            onChange={(event) => setCategory(event.target.value)}
          >
            {Object.entries(categories)
              .filter(
                ([value]) => value !== "dano_inmueble" || data.can_claim_damage,
              )
              .map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
          </select>
          <label htmlFor={`${formID}-description`}>Describe lo ocurrido</label>
          <textarea
            id={`${formID}-description`}
            rows={3}
            maxLength={3000}
            required
            className={inputStyle}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
          <Button type="submit" disabled={mutation.busy || !description.trim()}>
            Abrir caso
          </Button>
        </form>
      )}
      {mutation.error && <p role="alert">{mutation.error}</p>}
    </section>
  );
}

function CaseCard({
  item,
  reservationId,
  update,
  reload,
}: {
  item: CommercialSupportCase;
  reservationId?: string;
  update: (item: CommercialSupportCase) => void;
  reload: () => void;
}) {
  const [form, setForm] = useState("");
  const [description, setDescription] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState(false);
  const formID = useId();
  const mutation = useAftercareRequest();
  const requiresDescription = ["responder", "descargo", "resolver"].includes(
    form,
  );

  return (
    <article className="space-y-3 rounded-md border p-3">
      <div>
        <h3 className="font-semibold">{labelCategory(item.category)}</h3>
        <p className="text-sm">{statuses[item.status] ?? item.status}</p>
        {!reservationId && (
          <p className="break-all text-sm">Reserva {item.reservation_id}</p>
        )}
      </div>
      <p className="whitespace-pre-wrap break-words">{item.description}</p>
      {item.defense_deadline && (
        <p className="text-sm">
          Plazo de descargo{" "}
          {new Date(item.defense_deadline).toLocaleString("es-CL", {
            timeZone: "America/Santiago",
          })}
        </p>
      )}
      {item.events.length > 1 && (
        <ol aria-label="Historial del caso" className="space-y-2">
          {item.events
            .filter((event) => event.action !== "abrir")
            .map((event, index) => (
              <li
                key={`${event.created_at}:${index}`}
                className="rounded bg-muted p-2 text-sm"
              >
                <p className="font-medium">
                  {actions[event.action] ?? event.action}
                </p>
                <p className="whitespace-pre-wrap break-words">
                  {event.description}
                </p>
              </li>
            ))}
        </ol>
      )}
      {item.evidence.length > 0 && (
        <ul aria-label="Evidencias" className="space-y-1">
          {item.evidence.map((evidence, index) => (
            <li key={evidence.id}>
              <Button
                type="button"
                variant="outline"
                disabled={mutation.busy}
                onClick={() => {
                  void mutation.run(
                    `download:${evidence.id}`,
                    async (_key, signal) => {
                      const blob = await commercialAftercare.evidence(
                        item.id,
                        evidence.id,
                        reservationId,
                        signal,
                      );
                      signal.throwIfAborted();
                      const url = URL.createObjectURL(blob);
                      try {
                        const link = document.createElement("a");
                        const extension =
                          evidence.mime === "image/jpeg"
                            ? "jpg"
                            : evidence.mime === "image/png"
                              ? "png"
                              : "pdf";
                        link.href = url;
                        link.download = `evidencia-${index + 1}.${extension}`;
                        link.click();
                      } finally {
                        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
                      }
                    },
                  );
                }}
              >
                Descargar evidencia {index + 1}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap gap-2">
        {item.actions.map((action) => (
          <Button
            type="button"
            key={action}
            variant="outline"
            disabled={mutation.busy}
            aria-expanded={form === action}
            onClick={() => {
              setForm(form === action ? "" : action);
              setDescription("");
              setFile(null);
              setFileError(false);
            }}
          >
            {actions[action] ?? action}
          </Button>
        ))}
      </div>
      {form && item.actions.includes(form) && (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void mutation.run(`${form}:${description}`, async (key, signal) => {
              if (form === "adjuntar" && reservationId && file) {
                await commercialAftercare.attach(
                  reservationId,
                  item.id,
                  file,
                  signal,
                );
                reload();
              } else {
                update(
                  await commercialAftercare.caseAction(
                    item.id,
                    form,
                    description,
                    key,
                    reservationId,
                  ),
                );
              }
              setForm("");
            });
          }}
        >
          {form === "adjuntar" ? (
            <>
              <label htmlFor={formID}>
                Archivo PDF, JPG o PNG de hasta 5 MB
              </label>
              <input
                id={formID}
                type="file"
                required
                accept="application/pdf,image/jpeg,image/png"
                className={inputStyle}
                onChange={(event) => {
                  const selected = event.target.files?.[0];
                  const invalid =
                    !!selected &&
                    (selected.size === 0 || selected.size > 5 * 1024 * 1024);
                  setFileError(invalid);
                  setFile(invalid ? null : (selected ?? null));
                }}
              />
              {fileError && (
                <p role="alert">El archivo debe tener entre 1 byte y 5 MB.</p>
              )}
            </>
          ) : requiresDescription ? (
            <>
              <label htmlFor={formID}>Detalle</label>
              <textarea
                id={formID}
                value={description}
                required
                rows={3}
                maxLength={3000}
                className={inputStyle}
                onChange={(event) => setDescription(event.target.value)}
              />
            </>
          ) : null}
          <Button
            type="submit"
            disabled={
              mutation.busy ||
              (form === "adjuntar" && !file) ||
              (requiresDescription && !description.trim())
            }
          >
            Confirmar {actions[form]?.toLowerCase() ?? "acción"}
          </Button>
        </form>
      )}
      {mutation.error && <p role="alert">{mutation.error}</p>}
    </article>
  );
}
