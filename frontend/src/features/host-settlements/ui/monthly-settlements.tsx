"use client";

import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { hostSettlementsFacade } from "../host-settlements-runtime";
import type { SettlementPage } from "../domain/settlement";

function clp(value: string | null) {
  if (value === null) return "No disponible";
  return `$${BigInt(value).toLocaleString("es-CL")} CLP`;
}
function currentMonth() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "America/Santiago",
    year: "numeric",
    month: "2-digit",
  }).format(new Date());
}

export function MonthlySettlements() {
  const { snapshot: auth } = useAuth();
  const [month, setMonth] = useState(currentMonth);
  const [mode, setMode] = useState<"live" | "test">("live");
  const [page, setPage] = useState<SettlementPage | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [history, setHistory] = useState<Array<string | null>>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const authorized =
    auth.status === "authenticated" && auth.roles.includes("arrendador");
  const canLoad = authorized && /^\d{4}-(0[1-9]|1[0-2])$/.test(month);

  useEffect(() => {
    if (!canLoad) return;
    let alive = true;
    queueMicrotask(() => {
      if (alive) {
        setLoading(true);
        setError(false);
      }
    });
    void hostSettlementsFacade
      .list(month, mode, cursor)
      .then((result) => {
        if (alive) setPage(result);
      })
      .catch((cause: unknown) => {
        if (
          alive &&
          !(cause instanceof DOMException && cause.name === "AbortError")
        ) {
          setError(true);
          setPage(null);
        }
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
      hostSettlementsFacade.cancel();
    };
  }, [canLoad, month, mode, cursor, reloadKey]);

  const summary = page?.summary;
  const monthLabel = useMemo(() => {
    if (!/^\d{4}-\d{2}$/.test(month)) return "";
    return new Intl.DateTimeFormat("es-CL", {
      month: "long",
      year: "numeric",
      timeZone: "America/Santiago",
    }).format(new Date(`${month}-01T12:00:00-03:00`));
  }, [month]);

  if (!authorized)
    return (
      <section className="rounded-xl border p-6">
        <h2 className="text-lg font-semibold">Acceso restringido</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Esta liquidación está disponible para la cuenta anfitriona.
        </p>
      </section>
    );

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <label className="grid gap-2 text-sm font-medium">
          Mes
          <input
            aria-label="Mes del resumen"
            className="rounded-md border bg-background px-3 py-2"
            type="month"
            value={month}
            onChange={(event) => {
              setMonth(event.target.value);
              setCursor(null);
              setHistory([]);
            }}
          />
        </label>
        <label className="grid gap-2 text-sm font-medium">
          Entorno de pago
          <select
            aria-label="Entorno de pago"
            className="rounded-md border bg-background px-3 py-2"
            value={mode}
            onChange={(event) => {
              setMode(event.target.value as "live" | "test");
              setCursor(null);
              setHistory([]);
            }}
          >
            <option value="live">Pagos reales</option>
            <option value="test">Pruebas</option>
          </select>
        </label>
        <button
          className="rounded-md border px-4 py-2 text-sm font-medium"
          type="button"
          onClick={() => setReloadKey((value) => value + 1)}
          disabled={loading || !canLoad}
        >
          Actualizar
        </button>
      </div>
      <p className="text-sm text-muted-foreground">
        Resumen de {monthLabel || "…"}. Los pagos de prueba y los pagos reales
        se muestran por separado.
      </p>
      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          Cargando liquidaciones…
        </p>
      )}
      {error && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/40 p-4 text-sm"
        >
          No pudimos cargar el resumen.{" "}
          <button
            className="underline"
            onClick={() => setReloadKey((value) => value + 1)}
          >
            Reintentar
          </button>
        </div>
      )}
      {summary && !loading && (
        <>
          <section
            aria-label="Resumen mensual"
            className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
          >
            <Metric
              title="Arriendos pagados"
              value={String(summary.payment_count)}
            />
            <Metric title="Bruto" value={clp(summary.gross_clp)} />
            <Metric title="Base del arriendo" value={clp(summary.base_clp)} />
            <Metric
              title="Neto tras comisión"
              value={clp(summary.after_marketplace_fee_clp)}
            />
          </section>
          <section className="rounded-xl border p-5">
            <h2 className="font-semibold">Detalle de importes</h2>
            <dl className="mt-4 grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
              <Amount
                title="Comisión EspaciGo"
                value={clp(summary.commission_clp)}
              />
              <Amount
                title="IVA de la comisión"
                value={clp(summary.commission_vat_clp)}
              />
              <Amount
                title="Comisión total descontada"
                value={clp(summary.marketplace_fee_clp)}
              />
              <Amount
                title="Cargo del procesador"
                value={clp(summary.processor_fee_clp)}
              />
              <Amount title="Ajustes" value={clp(summary.adjustments_clp)} />
              <Amount
                title="Neto conciliado"
                value={clp(summary.reconciled_net_clp)}
              />
            </dl>
            <p className="mt-4 text-xs text-muted-foreground">
              Los cargos del procesador, ajustes y neto conciliado sólo aparecen
              cuando existe un registro verificado para esos importes.
            </p>
          </section>
          <section className="space-y-3">
            <h2 className="text-lg font-semibold">Pagos incluidos</h2>
            {page.items.length === 0 ? (
              <p className="rounded-lg border p-5 text-sm text-muted-foreground">
                No hay pagos verificados para este mes y entorno.
              </p>
            ) : (
              <div className="overflow-x-auto rounded-xl border">
                <table className="w-full min-w-[700px] text-left text-sm">
                  <thead className="bg-muted/50">
                    <tr>
                      <th className="p-3">Espacio y fecha</th>
                      <th className="p-3">Bruto</th>
                      <th className="p-3">Comisión</th>
                      <th className="p-3">IVA</th>
                      <th className="p-3">Neto tras comisión</th>
                    </tr>
                  </thead>
                  <tbody>
                    {page.items.map((item) => (
                      <tr key={item.reservation_id} className="border-t">
                        <td className="p-3">
                          <a
                            className="font-medium underline"
                            href={`/anfitrion/reservas/${encodeURIComponent(item.reservation_id)}`}
                          >
                            {item.space_title || "Espacio"}
                          </a>
                          <div className="mt-1 text-xs text-muted-foreground">
                            {new Intl.DateTimeFormat("es-CL", {
                              dateStyle: "medium",
                              timeStyle: "short",
                              timeZone: "America/Santiago",
                            }).format(new Date(item.verified_at))}
                          </div>
                        </td>
                        <td className="p-3">{clp(item.gross_clp)}</td>
                        <td className="p-3">{clp(item.commission_clp)}</td>
                        <td className="p-3">{clp(item.commission_vat_clp)}</td>
                        <td className="p-3">
                          {clp(item.after_marketplace_fee_clp)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <div className="flex justify-between">
              <button
                type="button"
                className="rounded-md border px-4 py-2 text-sm disabled:opacity-50"
                disabled={history.length === 0 || loading}
                onClick={() => {
                  const prev = history[history.length - 1] ?? null;
                  setHistory(history.slice(0, -1));
                  setCursor(prev);
                }}
              >
                Anterior
              </button>
              <button
                type="button"
                className="rounded-md border px-4 py-2 text-sm disabled:opacity-50"
                disabled={!page.next_cursor || loading}
                onClick={() => {
                  setHistory([...history, cursor]);
                  setCursor(page.next_cursor);
                }}
              >
                Siguiente
              </button>
            </div>
          </section>
        </>
      )}
    </div>
  );
}

function Metric({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-xl border p-4">
      <p className="text-sm text-muted-foreground">{title}</p>
      <p className="mt-2 text-xl font-semibold">{value}</p>
    </div>
  );
}
function Amount({ title, value }: { title: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 border-b pb-2">
      <dt className="text-muted-foreground">{title}</dt>
      <dd className="text-right font-medium">{value}</dd>
    </div>
  );
}
