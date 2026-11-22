"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { AlertTriangle, Loader2, ShieldCheck } from "lucide-react";
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
import { ApiError } from "@/lib/api/errors";
import { contractTemplateService } from "../contract-template-runtime";
import { commercialAuthorizationService } from "../commercial-authorization-runtime";
import {
  isCommercialAuthorization,
  validAuthorizationAmount,
  type CommercialAuthorization,
  type CommercialAuthorizationEvent,
} from "../domain/commercial-authorization";

interface Props {
  readonly spaceId: string;
}

function localDateTime(date: Date): string {
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
function fromLocalDateTime(value: string): string {
  return new Date(value).toISOString();
}
function errorText(error: unknown): string {
  if (error instanceof ApiError && error.status === 409)
    return "La plantilla publicada cambió o ya existe una autorización vigente. Recarga el panel.";
  if (error instanceof ApiError && error.status === 404)
    return "No se encontró el espacio, la plantilla publicada o la autorización.";
  return "No se pudo guardar la autorización. Recarga el panel e inténtalo otra vez.";
}

export function CommercialAuthorizationPanel({ spaceId }: Props) {
  const [authorization, setAuthorization] =
    useState<CommercialAuthorization | null>(null);
  const [history, setHistory] = useState<CommercialAuthorizationEvent[]>([]);
  const [publishedVersion, setPublishedVersion] = useState(0);
  const [amount, setAmount] = useState("100000");
  const [validFrom, setValidFrom] = useState(() =>
    localDateTime(new Date(Date.now() + 5 * 60_000)),
  );
  const [expiresAt, setExpiresAt] = useState(() =>
    localDateTime(new Date(Date.now() + 14 * 24 * 60 * 60_000)),
  );
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const [template, events, current] = await Promise.all([
          contractTemplateService.read(spaceId, signal),
          commercialAuthorizationService.history(spaceId, signal),
          commercialAuthorizationService
            .current(spaceId, signal)
            .catch((cause: unknown) => {
              if (cause instanceof ApiError && cause.status === 404)
                return null;
              throw cause;
            }),
        ]);
        setPublishedVersion(template.draft.published_version);
        setHistory(events);
        setAuthorization(
          current && isCommercialAuthorization(current) ? current : null,
        );
      } catch (cause) {
        if (!signal?.aborted) setError(errorText(cause));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [spaceId],
  );

  useEffect(() => {
    const controller = new AbortController();
    const task = window.setTimeout(() => {
      void load(controller.signal);
    }, 0);
    return () => {
      window.clearTimeout(task);
      controller.abort();
    };
  }, [load]);

  async function activate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const maxAmount = validAuthorizationAmount(amount);
    if (!publishedVersion || maxAmount === null || !validFrom || !expiresAt) {
      setError(
        "Publica una plantilla y completa un monto CLP y una vigencia válidos.",
      );
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await commercialAuthorizationService.activate(spaceId, {
        template_version: publishedVersion,
        max_amount_clp: maxAmount,
        valid_from: fromLocalDateTime(validFrom),
        expires_at: fromLocalDateTime(expiresAt),
      });
      setAuthorization(result);
      setNotice(
        "Autorización guardada. No registra una firma humana ni habilita reservas automáticas.",
      );
      await load();
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    if (!authorization) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await commercialAuthorizationService.revoke(
        spaceId,
        authorization.id,
        "owner_request",
      );
      setAuthorization(null);
      setNotice("Autorización revocada y registrada en el historial.");
      await load();
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card className="mt-6" aria-labelledby="commercial-authorization-title">
      <CardHeader>
        <CardTitle
          id="commercial-authorization-title"
          className="flex items-center gap-2"
        >
          <ShieldCheck className="size-5" aria-hidden="true" /> Autorización
          acotada de reservas
        </CardTitle>
        <CardDescription>
          Se administra por separado de la plantilla y requiere una versión
          publicada, un monto máximo y una vigencia de hasta 90 días.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div
          className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950"
          role="note"
        >
          <p className="flex gap-2 font-medium">
            <AlertTriangle
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />{" "}
            La reserva automática está deshabilitada mientras espera revisión
            jurídica.
          </p>
          <p className="mt-1">
            Esta autorización de límites no reemplaza ni registra la firma de
            ninguna persona.
          </p>
        </div>
        {loading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />{" "}
            Cargando estado privado…
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {notice ? (
          <p role="status" className="text-sm text-green-700">
            {notice}
          </p>
        ) : null}
        {authorization ? (
          <div className="space-y-3 rounded-md border p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  Autorización registrada
                </p>
                <p className="font-medium">
                  Versión {authorization.template_version} · máximo $
                  {authorization.max_amount_clp.toLocaleString("es-CL")} CLP
                </p>
                <p className="text-sm text-muted-foreground">
                  Vigencia:{" "}
                  {new Date(authorization.valid_from).toLocaleString("es-CL")} a{" "}
                  {new Date(authorization.expires_at).toLocaleString("es-CL")}
                </p>
                <p className="text-xs text-muted-foreground">
                  Solo evaluación de límites; aprobación manual obligatoria.
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={revoke}
              >
                Revocar autorización
              </Button>
            </div>
          </div>
        ) : (
          <form onSubmit={activate} className="grid gap-4 md:grid-cols-2">
            <p className="md:col-span-2 text-sm">
              Plantilla publicada:{" "}
              {publishedVersion > 0 ? `versión ${publishedVersion}` : "ninguna"}
              . Publicar otra versión invalida la autorización y exige volver a
              activarla.
            </p>
            <div className="space-y-2">
              <Label htmlFor="authorization-max-amount">
                Monto máximo por reserva (CLP)
              </Label>
              <Input
                id="authorization-max-amount"
                inputMode="numeric"
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                required
                aria-describedby="authorization-amount-help"
              />
              <p
                id="authorization-amount-help"
                className="text-xs text-muted-foreground"
              >
                Desde $1 hasta $100.000.000 CLP.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="authorization-valid-from">Válida desde</Label>
              <Input
                id="authorization-valid-from"
                type="datetime-local"
                value={validFrom}
                onChange={(event) => setValidFrom(event.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="authorization-expires-at">Vence</Label>
              <Input
                id="authorization-expires-at"
                type="datetime-local"
                value={expiresAt}
                onChange={(event) => setExpiresAt(event.target.value)}
                required
              />
            </div>
            <div className="flex items-end">
              <Button
                type="submit"
                disabled={
                  busy ||
                  loading ||
                  publishedVersion < 1 ||
                  validAuthorizationAmount(amount) === null
                }
              >
                {busy ? "Guardando…" : "Activar límites"}
              </Button>
            </div>
          </form>
        )}
        <div className="border-t pt-4">
          <h3 className="text-sm font-semibold">Historial de autorización</h3>
          {history.length ? (
            <ul className="mt-2 space-y-2 text-sm">
              {history.slice(0, 8).map((item) => (
                <li
                  key={`${item.grant_id}-${item.action}`}
                  className="flex justify-between gap-3"
                >
                  <span>
                    {item.action === "activated" ? "Activada" : "Revocada"} ·{" "}
                    {item.reason}
                  </span>
                  <time
                    dateTime={item.occurred_at}
                    className="text-muted-foreground"
                  >
                    {new Date(item.occurred_at).toLocaleString("es-CL")}
                  </time>
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-2 text-sm text-muted-foreground">
              Sin eventos todavía.
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
