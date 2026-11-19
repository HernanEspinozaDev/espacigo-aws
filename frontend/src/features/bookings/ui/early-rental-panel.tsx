"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { ApiError } from "@/lib/api/errors";
import {
  earlyRentalAdapter as api,
  type DemoRental,
  type DemoRentalAction,
  type DemoRentalSelection,
} from "../infrastructure/early-rental-adapter";

const labels: Record<DemoRental["state"], string> = {
  pendiente_identidad: "Solicitud pendiente de aprobación",
  pendiente_contrato: "Contrato pendiente de firma",
  pendiente_envio_pago: "Contrato firmado · pago pendiente",
  pendiente_recepcion_pago: "Confirmando pago",
  confirmado: "Pago aprobado · reserva confirmada",
  rechazado: "Solicitud rechazada",
  cancelado: "Solicitud cancelada",
  vencido: "Solicitud vencida",
};

const money = new Intl.NumberFormat("es-CL", {
  style: "currency",
  currency: "CLP",
});
const rentalHref = (id: string) => "/reservas?demo=" + encodeURIComponent(id);

function errorMessage(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 409)
      return "La solicitud cambió o el horario ya no está disponible.";
    if (error.status === 412)
      return "El estado cambió. Revisa la solicitud actualizada y vuelve a intentar.";
    if (error.status === 404)
      return "La solicitud o el espacio no está disponible para esta cuenta.";
    if (error.status === 403) return "Esta cuenta no puede realizar la acción.";
  }
  return "No pudimos completar la operación. Intenta nuevamente.";
}

export function EarlyRentalPanel(props: {
  selection?: DemoRentalSelection;
  selectedId?: string;
  fallback?: React.ReactNode;
}) {
  const { snapshot } = useAuth();
  if (snapshot.status !== "authenticated") {
    return props.selection ? (
      <p>Inicia sesión para enviar una solicitud de arriendo.</p>
    ) : (
      (props.fallback ?? null)
    );
  }
  return (
    <SessionPanel
      key={snapshot.accountId + ":" + snapshot.expiresAt}
      {...props}
    />
  );
}

function SessionPanel({
  selection,
  selectedId,
  fallback,
}: {
  selection?: DemoRentalSelection;
  selectedId?: string;
  fallback?: React.ReactNode;
}) {
  const router = useRouter();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [checked, setChecked] = useState(false);
  const [preview, setPreview] = useState(false);
  const [observedAt, setObservedAt] = useState(0);
  const [page, setPage] = useState<Awaited<ReturnType<typeof api.list>>>();
  const [cursor, setCursor] = useState<string>();
  const [detail, setDetail] = useState<Awaited<ReturnType<typeof api.read>>>();
  const mutation = useRef<AbortController | null>(null);
  const createKey = useRef<string | null>(null);

  useEffect(() => {
    const request = new AbortController();
    void api
      .enabled(request.signal)
      .then((value) => {
        if (!request.signal.aborted) setEnabled(value);
      })
      .catch((err: unknown) => {
        if (!request.signal.aborted) setError(errorMessage(err));
      });
    return () => {
      request.abort();
      mutation.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (!enabled || selection) return;
    const request = new AbortController();
    let inFlight = false;
    async function refresh() {
      if (inFlight || mutation.current) return;
      inFlight = true;
      setObservedAt(Date.now());
      try {
        if (selectedId) {
          const value = await api.read(selectedId, request.signal);
          if (!request.signal.aborted)
            setDetail((current) =>
              current?.rental.id === value.rental.id &&
              current.rental.revision > value.rental.revision
                ? current
                : value,
            );
        } else {
          const value = await api.list(request.signal, cursor);
          if (!request.signal.aborted) setPage(value);
        }
      } catch (err) {
        if (!request.signal.aborted) setError(errorMessage(err));
      } finally {
        inFlight = false;
      }
    }
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => {
      request.abort();
      window.clearInterval(timer);
    };
  }, [enabled, selection, selectedId, cursor]);

  async function act(action: DemoRentalAction | "create") {
    if (mutation.current) return;
    const request = new AbortController();
    mutation.current = request;
    setBusy(true);
    setError("");
    try {
      if (action === "create" && selection) {
        createKey.current ??= crypto.randomUUID();
        const created = await api.create(
          selection,
          createKey.current,
          request.signal,
        );
        if (!request.signal.aborted) router.push(rentalHref(created.id));
      } else if (action !== "create" && detail) {
        const value = await api.act(
          action,
          detail.rental,
          detail.revision,
          request.signal,
        );
        if (!request.signal.aborted) {
          setDetail(value);
          setChecked(false);
        }
      }
    } catch (err) {
      if (!request.signal.aborted) setError(errorMessage(err));
    } finally {
      mutation.current = null;
      if (!request.signal.aborted) setBusy(false);
    }
  }

  if (enabled === false) {
    return (
      fallback ?? <p>La demo de arriendo está desactivada en este entorno.</p>
    );
  }

  const rental = detail?.rental.id === selectedId ? detail?.rental : undefined;
  const invoice = rental?.invoice;
  const actions = rental?.allowed_actions ?? [];
  const host = rental?.actor_role === "arrendador";
  const identity = actions.includes("confirmar_identidad_simulada");
  const contract = actions.includes("aceptar_contrato");
  const pay = actions.includes("marcar_pago_enviado");
  const reject = actions.includes("rechazar");
  const expired =
    rental &&
    !invoice &&
    observedAt >= Date.parse(rental.expires_at) &&
    !["rechazado", "cancelado", "vencido"].includes(rental.state);
  return (
    <section
      aria-label="Arriendo de prueba"
      className="space-y-4 rounded-xl border bg-card p-5"
    >
      <p className="text-sm text-muted-foreground">
        Demo · pago y contrato de prueba
      </p>
      <h2 className="text-lg font-semibold">
        {selection
          ? "Solicitar arriendo de prueba"
          : selectedId
            ? "Solicitud de arriendo"
            : "Mis reservas"}
      </h2>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {enabled === null && !error && (
        <p role="status">Consultando disponibilidad de la demo…</p>
      )}
      {enabled && selection && (
        <Button disabled={busy} onClick={() => void act("create")}>
          Enviar solicitud al arrendador
        </Button>
      )}
      {enabled && !selection && !selectedId && page && (
        <div aria-live="polite" className="space-y-3">
          {page.items.length === 0 && (
            <p>Aún no tienes solicitudes ni reservas de prueba.</p>
          )}
          {page.items.map((item) => (
            <Link
              key={item.id}
              className="block rounded-lg border p-3 hover:bg-muted"
              href={rentalHref(item.id)}
            >
              <span className="block font-medium">{item.space_title}</span>
              <span className="block text-sm">
                {item.actor_role === "arrendador" ? "Recibida" : "Enviada"} ·{" "}
                {item.state !== "confirmado" &&
                item.state.startsWith("pendiente_") &&
                observedAt >= Date.parse(item.expires_at)
                  ? labels.vencido
                  : labels[item.state]}
              </span>
            </Link>
          ))}
          {page.next_cursor && (
            <Button
              variant="outline"
              onClick={() => setCursor(page.next_cursor ?? undefined)}
            >
              Siguientes reservas
            </Button>
          )}
          {cursor && (
            <Button variant="outline" onClick={() => setCursor(undefined)}>
              Volver al inicio
            </Button>
          )}
        </div>
      )}
      {rental && (
        <>
          <Link href="/reservas" className="underline">
            Volver a Mis reservas
          </Link>
          <p className="font-medium">{rental.space_title}</p>
          <p role="status">{expired ? labels.vencido : labels[rental.state]}</p>
          <p>
            {new Intl.DateTimeFormat("es-CL", {
              dateStyle: "short",
              timeStyle: "short",
              timeZone: rental.time_zone,
            }).formatRange(new Date(rental.start_at), new Date(rental.end_at))}
          </p>
          {!expired &&
            identity &&
            (!host || rental.identity_confirmations.arrendatario) &&
            (host ? (
              <Button disabled={busy} onClick={() => void act("identity")}>
                Aprobar solicitud
              </Button>
            ) : (
              <div className="space-y-2">
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={checked}
                    disabled={busy}
                    onChange={(event) => setChecked(event.target.checked)}
                  />{" "}
                  Confirmo mi identidad de prueba
                </label>
                <Button
                  disabled={busy || !checked}
                  onClick={() => void act("identity")}
                >
                  Guardar verificación
                </Button>
              </div>
            ))}
          {!expired && reject && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void act("reject")}
            >
              Rechazar solicitud
            </Button>
          )}
          {[
            "pendiente_contrato",
            "pendiente_envio_pago",
            "confirmado",
          ].includes(rental.state) && (
            <p>
              Contrato · Arrendatario{" "}
              {rental.contract.acceptances.arrendatario ? "firmó" : "pendiente"}{" "}
              · Arrendador{" "}
              {rental.contract.acceptances.arrendador ? "firmó" : "pendiente"}
            </p>
          )}
          {!expired && contract && (
            <div className="space-y-2">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={busy}
                  onChange={(event) => setChecked(event.target.checked)}
                />{" "}
                Apruebo y firmo el contrato de prueba
              </label>
              <Button
                disabled={busy || !checked}
                onClick={() => void act("contract")}
              >
                Guardar firma
              </Button>
            </div>
          )}
          {!expired && pay && !preview && (
            <Button onClick={() => setPreview(true)}>Proceder al pago</Button>
          )}
          {((!expired && pay && preview) || invoice) && (
            <section
              className="space-y-2 rounded-lg border p-4"
              aria-label="Invoice de prueba"
            >
              <h3 className="font-semibold">
                {invoice
                  ? "Invoice de prueba"
                  : "Vista previa del invoice de prueba"}
              </h3>
              <p>Arriendo {rental.id}</p>
              <p>
                {rental.units} h × {money.format(BigInt(rental.unit_price_clp))}
              </p>
              <p>
                Total{" "}
                {money.format(
                  BigInt(invoice?.amount_clp ?? rental.subtotal_clp),
                )}
              </p>
              {invoice ? (
                <>
                  <p>Número {invoice.number}</p>
                  <p>Pago {invoice.payment_id}</p>
                  <p>
                    Aprobado el{" "}
                    {new Date(invoice.approved_at).toLocaleString("es-CL", {
                      timeZone: rental.time_zone,
                    })}
                  </p>
                </>
              ) : (
                <Button disabled={busy} onClick={() => void act("pay")}>
                  Aprobar pago simulado
                </Button>
              )}
            </section>
          )}
        </>
      )}
    </section>
  );
}
