"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { authFacade } from "@/features/auth/auth-runtime";
import { apiClient } from "@/lib/api/client";
import { normalizeApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";

type Documents = components["schemas"]["CommercialReservationDocuments"];
interface Props {
  reservationId: string;
}
const money = (amount: number | string) =>
  new Intl.NumberFormat("es-CL", {
    style: "currency",
    currency: "CLP",
    maximumFractionDigits: 0,
  }).format(Number(amount));
const date = (value: string) =>
  new Intl.DateTimeFormat("es-CL", {
    dateStyle: "long",
    timeStyle: "short",
    timeZone: "America/Santiago",
  }).format(new Date(value));

export function CommercialReservationDocuments(props: Props) {
  const { snapshot, accessToken } = useAuth();
  const stateKey = JSON.stringify([
    props.reservationId,
    snapshot.accountId,
    snapshot.expiresAt,
    accessToken,
  ]);
  return <ReservationDocumentsPanel key={stateKey} {...props} />;
}

function ReservationDocumentsPanel({ reservationId }: Props) {
  const { snapshot, accessToken, getAccessToken } = useAuth();
  const [loaded, setLoaded] = useState<{
    documents: Documents;
    token: typeof accessToken;
  } | null>(null);
  const documents =
    !snapshot.restrictedMode &&
    accessToken &&
    loaded?.token === accessToken &&
    loaded.documents.reservation_id === reservationId
      ? loaded.documents
      : null;
  const downloadPath = `/commercial-reservations/${encodeURIComponent(reservationId)}/contract-document`;
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [taxFile, setTaxFile] = useState<File | null>(null);
  const isCurrent = useCallback(
    () => Boolean(accessToken) && getAccessToken() === accessToken,
    [accessToken, getAccessToken],
  );
  const load = useCallback(async () => {
    if (!accessToken || snapshot.restrictedMode) return;
    const result = await authFacade.runAuthenticatedOperation(
      (_token, signal) =>
        apiClient.GET("/commercial-reservations/{reservationID}/documents", {
          params: { path: { reservationID: reservationId } },
          signal,
        }),
    );
    if (!result.response.ok)
      throw normalizeApiError(result.response, result.error);
    const payload: unknown = result.data;
    if (typeof payload !== "object" || payload === null)
      throw new TypeError("Respuesta de documentos inválida");
    const payloadRecord = payload as Record<string, unknown>;
    if (
      !Object.hasOwn(payloadRecord, "reservation_id") ||
      payloadRecord.reservation_id !== reservationId ||
      !Object.hasOwn(payloadRecord, "access_instructions") ||
      payloadRecord.access_instructions !== null
    )
      throw new TypeError("Respuesta de documentos inválida");
    if (isCurrent())
      setLoaded({ documents: payload as Documents, token: accessToken });
  }, [accessToken, snapshot.restrictedMode, reservationId, isCurrent]);

  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (!active) return;
      load().catch((cause: unknown) => {
        if (active && isCurrent()) {
          // This endpoint intentionally hides unpaid and non-participant records.
          const status = (cause as { status?: number }).status;
          if (status !== 404 && status !== 409)
            setError("No pudimos cargar los documentos de esta reserva.");
        }
      });
    });
    return () => {
      active = false;
    };
  }, [load, isCurrent]);

  const download = async () => {
    if (
      documents?.contract.download_path !== `/api/v1${downloadPath}` ||
      !isCurrent()
    )
      return;
    setBusy(true);
    setError("");
    let objectUrl: string | undefined;
    try {
      const response = await authFacade.runAuthenticatedOperation(
        async (_token, signal) =>
          fetch(`/api/backend${downloadPath}`, {
            method: "GET",
            credentials: "same-origin",
            cache: "no-store",
            signal,
          }),
      );
      if (!response.ok) {
        if (response.status === 409) {
          await load();
          setError(
            "El contrato sigue procesándose. Puedes volver a intentar en unos segundos.",
          );
          return;
        }
        throw new Error("Descarga no disponible");
      }
      if (
        response.headers.get("Content-Type")?.split(";")[0] !==
        "application/pdf"
      )
        throw new Error("Tipo de archivo inesperado");
      const blob = await response.blob();
      if (!isCurrent()) return;
      objectUrl = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = objectUrl;
      link.download = `contrato-reserva-${reservationId}.pdf`;
      document.body.append(link);
      link.click();
      link.remove();
    } catch {
      if (isCurrent())
        setError(
          "No pudimos descargar el contrato. Actualiza el estado e inténtalo de nuevo.",
        );
    } finally {
      if (objectUrl) URL.revokeObjectURL(objectUrl);
      if (isCurrent()) setBusy(false);
    }
  };

  const taxDocumentURL = `/api/backend/commercial-reservations/${encodeURIComponent(reservationId)}/rental-tax-document`;
  const uploadTaxDocument = async () => {
    if (!taxFile || !isCurrent() || documents?.participant_role !== "anfitrion")
      return;
    if (
      taxFile.type !== "application/pdf" ||
      taxFile.size < 1 ||
      taxFile.size > 10 * 1024 * 1024
    ) {
      setError("Selecciona un archivo PDF de hasta 10 MiB.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const response = await authFacade.runAuthenticatedOperation(
        (_token, signal) =>
          fetch(taxDocumentURL, {
            method: "PUT",
            credentials: "same-origin",
            cache: "no-store",
            headers: { "Content-Type": "application/pdf" },
            body: taxFile,
            signal,
          }),
      );
      if (!response.ok) {
        if (response.status === 409)
          throw new Error(
            "Ya existe un documento tributario adjunto para esta reserva.",
          );
        if (response.status === 503)
          throw new Error(
            "La inspección del archivo no está disponible. Inténtalo más tarde.",
          );
        throw new Error(
          "No se pudo adjuntar el PDF. Comprueba el archivo e inténtalo de nuevo.",
        );
      }
      const result: unknown = await response.json();
      if (typeof result !== "object" || result === null)
        throw new TypeError("Respuesta de adjunto inválida");
      const attachment = result as Record<string, unknown>;
      if (
        attachment.reservation_id !== reservationId ||
        attachment.state !== "ready" ||
        typeof attachment.sha256 !== "string" ||
        !/^[a-f0-9]{64}$/u.test(attachment.sha256) ||
        !Number.isSafeInteger(attachment.size_bytes) ||
        Number(attachment.size_bytes) < 1 ||
        typeof attachment.uploaded_at !== "string" ||
        !Number.isFinite(Date.parse(attachment.uploaded_at))
      )
        throw new TypeError("Respuesta de adjunto inválida");
      await load();
      if (isCurrent()) setTaxFile(null);
    } catch (cause) {
      if (isCurrent())
        setError(
          cause instanceof Error
            ? cause.message
            : "No se pudo adjuntar el PDF.",
        );
    } finally {
      if (isCurrent()) setBusy(false);
    }
  };

  const downloadTaxDocument = async () => {
    if (documents?.rental_tax_document.state !== "ready" || !isCurrent())
      return;
    setBusy(true);
    setError("");
    let objectUrl: string | undefined;
    try {
      const response = await authFacade.runAuthenticatedOperation(
        (_token, signal) =>
          fetch(taxDocumentURL, {
            method: "GET",
            credentials: "same-origin",
            cache: "no-store",
            signal,
          }),
      );
      if (!response.ok)
        throw new Error("No pudimos descargar el documento tributario.");
      if (
        response.headers.get("Content-Type")?.split(";")[0] !==
        "application/pdf"
      )
        throw new Error("Tipo de archivo inesperado");
      const blob = await response.blob();
      if (!isCurrent()) return;
      objectUrl = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = objectUrl;
      link.download = `documento-tributario-arriendo-${reservationId}.pdf`;
      document.body.append(link);
      link.click();
      link.remove();
    } catch (cause) {
      if (isCurrent())
        setError(
          cause instanceof Error
            ? cause.message
            : "No pudimos descargar el PDF.",
        );
    } finally {
      if (objectUrl) URL.revokeObjectURL(objectUrl);
      if (isCurrent()) setBusy(false);
    }
  };

  if (!documents) return error ? <p role="alert">{error}</p> : null;
  const counterpart = documents.counterpart_contact;
  const contract = documents.contract;
  const retryable = ["pending", "processing", "retry"].includes(contract.state);
  return (
    <section
      aria-labelledby="commercial-reservation-documents-title"
      className="space-y-4 rounded-xl border border-border bg-background p-5"
    >
      <div>
        <h2
          id="commercial-reservation-documents-title"
          className="text-lg font-semibold"
        >
          Tu reserva pagada
        </h2>
        <p className="text-sm text-muted-foreground">
          Documentos y datos de contacto disponibles para las partes.
        </p>
      </div>
      {counterpart && (
        <div className="space-y-1 text-sm">
          <h3 className="font-medium">
            Contacto del{" "}
            {counterpart.role === "anfitrion" ? "anfitrión" : "arrendatario"}
          </h3>
          {counterpart.display_name && <p>{counterpart.display_name}</p>}
          {counterpart.email && (
            <p>
              <a
                className="underline underline-offset-4"
                href={`mailto:${counterpart.email}`}
              >
                {counterpart.email}
              </a>
            </p>
          )}
          {counterpart.phone && (
            <p>
              <a
                className="underline underline-offset-4"
                href={`tel:${counterpart.phone}`}
              >
                {counterpart.phone}
              </a>
            </p>
          )}
        </div>
      )}
      <div className="space-y-1 text-sm">
        <h3 className="font-medium">Comprobante interno de pago</h3>
        <p>Estado: Pago aprobado</p>
        <p>
          Monto: {money(documents.receipt.amount_clp)}{" "}
          {documents.receipt.currency}
        </p>
        <p>Fecha de confirmación: {date(documents.receipt.verified_at)}</p>
        <p>
          Referencia de Mercado Pago: {documents.receipt.provider_reference}
        </p>
        <p className="text-muted-foreground">
          Este comprobante informa el pago procesado; no es una boleta o factura
          tributaria del arriendo.
        </p>
      </div>
      <div className="space-y-2 text-sm">
        <h3 className="font-medium">Contrato</h3>
        {contract.state === "ready" && contract.download_path ? (
          <Button type="button" disabled={busy} onClick={() => void download()}>
            {busy ? "Preparando descarga…" : "Descargar contrato PDF"}
          </Button>
        ) : retryable ? (
          <div className="space-y-2">
            <p role="status">El contrato se está preparando.</p>
            <Button
              variant="outline"
              type="button"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                void load()
                  .catch(() =>
                    setError("No pudimos actualizar el estado del contrato."),
                  )
                  .finally(() => setBusy(false));
              }}
            >
              {busy ? "Actualizando…" : "Actualizar estado"}
            </Button>
          </div>
        ) : (
          <p role="status">
            El contrato no está disponible. Contacta a soporte si necesitas
            asistencia.
          </p>
        )}
      </div>
      <div className="space-y-1 text-sm">
        <h3 className="font-medium">Documento tributario del arriendo</h3>
        <p>{documents.rental_tax_document.notice}</p>
        {documents.rental_tax_document.state === "ready" ? (
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => void downloadTaxDocument()}
          >
            {busy ? "Preparando descarga…" : "Descargar PDF tributario"}
          </Button>
        ) : documents.participant_role === "anfitrion" ? (
          <div className="space-y-2">
            <label
              className="block text-sm font-medium"
              htmlFor="rental-tax-document"
            >
              Adjuntar boleta o factura del arriendo (PDF)
            </label>
            <input
              id="rental-tax-document"
              type="file"
              accept="application/pdf,.pdf"
              disabled={busy}
              onChange={(event) =>
                setTaxFile(event.currentTarget.files?.[0] ?? null)
              }
            />
            <Button
              type="button"
              variant="outline"
              disabled={busy || !taxFile}
              onClick={() => void uploadTaxDocument()}
            >
              {busy ? "Inspeccionando y adjuntando…" : "Adjuntar PDF"}
            </Button>
            <p className="text-muted-foreground">
              Se permite un archivo de hasta 10 MiB. Debe ser un PDF
              estructuralmente seguro y superar la inspección antimalware.
              EspaciGo no emite ni valida DTE.
            </p>
          </div>
        ) : null}
      </div>
      {error && (
        <p role="alert" className="text-sm text-red-800">
          {error}
        </p>
      )}
    </section>
  );
}
