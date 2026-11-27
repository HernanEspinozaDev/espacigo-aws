"use client";

import Link from "next/link";
import { useState, type FormEvent, type KeyboardEvent } from "react";
import {
  AlertCircle,
  ArrowRight,
  CalendarClock,
  Check,
  CheckCircle2,
  Clock,
  Copy,
  FileCheck2,
  Loader2,
  Lock,
  Search,
  Send,
  ShieldCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import {
  EVENT_TYPE_LABELS,
  PUBLIC_PRIVACY_RIGHT_OPTIONS,
  REPRESENTATIVE_RELATION_OPTIONS,
  REQUESTER_TYPE_LABELS,
  REQUEST_STATE_LABELS,
  RIGHT_TYPE_LABELS,
  type PublicPrivacyRepresentativeRelation,
  type PublicPrivacyRequestReceipt,
  type PublicPrivacyRequestState,
  type PublicPrivacyRequestStatus,
  type PublicPrivacyRequesterType,
  type PublicPrivacyRightType,
  type RightsRequestEvent,
} from "../domain/types";
import {
  lookupPublicPrivacyRequest,
  submitPublicPrivacyRequest,
} from "../infrastructure/privacy-public-client";

type PortalTab = "submit" | "lookup";

const DETAIL_MIN_LENGTH = 10;
const DETAIL_MAX_LENGTH = 1000;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function formatDateTimeCL(isoString: string | undefined): string | null {
  if (!isoString) return null;
  const parsed = new Date(isoString);
  if (Number.isNaN(parsed.getTime())) return isoString;
  return parsed.toLocaleString("es-CL", {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function stateLabel(state: string): string {
  return REQUEST_STATE_LABELS[state as PublicPrivacyRequestState] ?? state;
}

function stateBadgeVariant(
  state: PublicPrivacyRequestState,
): "default" | "secondary" | "destructive" | "outline" {
  switch (state) {
    case "resuelta":
      return "default";
    case "denegada_fundada":
      return "destructive";
    case "en_revision":
    case "identidad_pendiente":
      return "outline";
    case "recibida":
    default:
      return "secondary";
  }
}

function sortTimelineEvents(
  events: readonly RightsRequestEvent[] | undefined,
): RightsRequestEvent[] {
  if (!events || events.length === 0) return [];
  return [...events].sort((a, b) => a.sequence - b.sequence);
}

export function PublicPrivacyPortal() {
  const [activeTab, setActiveTab] = useState<PortalTab>("submit");

  // Formulario de ingreso público
  const [rightType, setRightType] = useState<PublicPrivacyRightType>("acceso");
  const [requesterType, setRequesterType] =
    useState<PublicPrivacyRequesterType>("titular");
  const [requesterName, setRequesterName] = useState("");
  const [requesterEmail, setRequesterEmail] = useState("");
  const [representativeName, setRepresentativeName] = useState("");
  const [representativeRelation, setRepresentativeRelation] =
    useState<PublicPrivacyRepresentativeRelation>("representante_legal");
  const [subjectReference, setSubjectReference] = useState("");
  const [detail, setDetail] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [receipt, setReceipt] = useState<PublicPrivacyRequestReceipt | null>(
    null,
  );
  const [copiedCredentials, setCopiedCredentials] = useState(false);

  // Formulario de consulta
  const [lookupFolio, setLookupFolio] = useState("");
  const [lookupCode, setLookupCode] = useState("");
  const [lookingUp, setLookingUp] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [lookupStatus, setLookupStatus] =
    useState<PublicPrivacyRequestStatus | null>(null);

  const handleTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "ArrowRight" || event.key === "ArrowLeft") {
      event.preventDefault();
      setActiveTab((current) => (current === "submit" ? "lookup" : "submit"));
    }
  };

  const validateIntakeForm = (): string | null => {
    if (requesterName.trim().length < 2) {
      return "Ingresa el nombre completo del solicitante (mínimo 2 caracteres).";
    }
    const trimmedEmail = requesterEmail.trim();
    if (trimmedEmail.length < 5 || !EMAIL_PATTERN.test(trimmedEmail)) {
      return "Ingresa un correo electrónico de contacto válido.";
    }
    if (requesterType === "representante") {
      if (representativeName.trim().length < 2) {
        return "Ingresa el nombre completo del representante (mínimo 2 caracteres).";
      }
      if (!representativeRelation) {
        return "Selecciona la relación de representación.";
      }
    }
    const trimmedReference = subjectReference.trim();
    if (trimmedReference.length > 0 && trimmedReference.length < 2) {
      return "La referencia opcional del titular debe tener al menos 2 caracteres si la informas.";
    }
    if (detail.trim().length < DETAIL_MIN_LENGTH) {
      return `Describe claramente tu requerimiento (mínimo ${DETAIL_MIN_LENGTH} caracteres).`;
    }
    if (detail.trim().length > DETAIL_MAX_LENGTH) {
      return `El detalle no puede superar los ${DETAIL_MAX_LENGTH} caracteres.`;
    }
    return null;
  };

  const handleIntakeSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitError(null);
    setCopiedCredentials(false);

    const validationError = validateIntakeForm();
    if (validationError) {
      setSubmitError(validationError);
      return;
    }

    setSubmitting(true);
    try {
      const trimmedReference = subjectReference.trim();
      const createdReceipt = await submitPublicPrivacyRequest({
        right_type: rightType,
        requester_type: requesterType,
        requester_name: requesterName.trim(),
        requester_email: requesterEmail.trim(),
        ...(requesterType === "representante"
          ? {
              representative_name: representativeName.trim(),
              representative_relation: representativeRelation,
            }
          : {}),
        ...(trimmedReference ? { subject_reference: trimmedReference } : {}),
        detail: detail.trim(),
      });
      setReceipt(createdReceipt);
      setLookupFolio(createdReceipt.folio);
      setLookupCode(createdReceipt.tracking_code);
      setLookupError(null);
      setLookupStatus(null);
    } catch (error) {
      setSubmitError(
        error instanceof Error
          ? error.message
          : "No pudimos registrar tu solicitud en este momento.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  const handleCopyCredentials = async () => {
    if (!receipt) return;
    const textToCopy = `Folio: ${receipt.folio}\nCódigo de seguimiento: ${receipt.tracking_code}`;
    try {
      await navigator.clipboard.writeText(textToCopy);
      setCopiedCredentials(true);
    } catch {
      setCopiedCredentials(false);
    }
  };

  const handleGoToLookupWithReceipt = () => {
    if (receipt) {
      setLookupFolio(receipt.folio);
      setLookupCode(receipt.tracking_code);
    }
    setLookupError(null);
    setActiveTab("lookup");
  };

  const handleResetForNewRequest = () => {
    setReceipt(null);
    setSubmitError(null);
    setCopiedCredentials(false);
    setRightType("acceso");
    setRequesterType("titular");
    setRequesterName("");
    setRequesterEmail("");
    setRepresentativeName("");
    setRepresentativeRelation("representante_legal");
    setSubjectReference("");
    setDetail("");
  };

  const handleLookupSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLookupError(null);
    setLookupStatus(null);

    const folio = lookupFolio.trim();
    const trackingCode = lookupCode.trim();
    if (folio.length < 6) {
      setLookupError(
        "Ingresa un folio válido (por ejemplo, PRIV-2026-XXXXXXXX).",
      );
      return;
    }
    if (trackingCode.length < 16) {
      setLookupError("Ingresa el código de seguimiento completo.");
      return;
    }

    setLookingUp(true);
    try {
      const result = await lookupPublicPrivacyRequest({
        folio,
        tracking_code: trackingCode,
      });
      setLookupStatus(result);
    } catch (error) {
      setLookupError(
        error instanceof Error
          ? error.message
          : "No pudimos consultar el estado de la solicitud.",
      );
    } finally {
      setLookingUp(false);
    }
  };

  const timelineEvents = sortTimelineEvents(lookupStatus?.events);

  return (
    <article className="mx-auto max-w-5xl px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
      <nav
        aria-label="Breadcrumb"
        className="mb-8 text-sm text-muted-foreground"
      >
        <Link href="/" className="hover:text-foreground">
          Inicio
        </Link>
        <span aria-hidden="true"> / </span>
        <Link href="/politica-de-privacidad" className="hover:text-foreground">
          Política de privacidad
        </Link>
        <span aria-hidden="true"> / </span>
        <span aria-current="page">Solicitudes ARCO+</span>
      </nav>

      <header className="rounded-3xl border border-border bg-[var(--brand-surface-subtle)] p-6 sm:p-10">
        <p className="inline-flex items-center gap-2 rounded-full bg-[var(--brand-primary-soft)] px-3.5 py-1 text-xs font-semibold uppercase tracking-[0.16em] text-[var(--brand-accent)]">
          <ShieldCheck className="size-3.5" aria-hidden="true" />
          Canal público sin sesión
        </p>
        <h1 className="mt-4 text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
          Solicitudes de privacidad y derechos ARCO+
        </h1>
        <p className="mt-4 max-w-3xl text-base leading-relaxed text-[var(--brand-muted-on-subtle)]">
          Ejerce tus derechos de acceso, rectificación, supresión, oposición,
          portabilidad o bloqueo temporal de datos personales sin necesidad de
          tener una cuenta activa ni iniciar sesión en EspaciGo.
        </p>

        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          <div className="rounded-2xl border border-border bg-background p-4 text-sm">
            <p className="font-medium text-foreground">
              Marco normativo chileno
            </p>
            <p className="mt-1.5 leading-relaxed text-muted-foreground">
              Operamos conforme a la <strong>Ley N° 19.628</strong> vigente y en
              adecuación a la <strong>Ley N° 21.719</strong> (modificada por la{" "}
              <strong>Ley N° 21.855</strong>, con entrada en vigor el{" "}
              <strong>15 de octubre de 2027</strong>).
            </p>
          </div>
          <div
            role="note"
            className="rounded-2xl border border-border bg-background p-4 text-sm"
          >
            <p className="flex items-center gap-1.5 font-medium text-foreground">
              <Lock className="size-4 text-primary" aria-hidden="true" />
              Aviso de respuesta neutral
            </p>
            <p className="mt-1.5 leading-relaxed text-muted-foreground">
              Por seguridad y protección de datos, este canal{" "}
              <strong>no confirma ni revela la existencia de cuentas</strong>{" "}
              asociadas a una persona o correo electrónico al ingresar o
              consultar una solicitud.
            </p>
          </div>
        </div>
      </header>

      <div className="mt-8">
        <div
          role="tablist"
          aria-label="Opciones del portal público de privacidad"
          className="inline-flex w-full flex-col gap-2 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-1.5 sm:w-auto sm:flex-row"
        >
          <button
            type="button"
            role="tab"
            id="privacy-tab-submit"
            aria-selected={activeTab === "submit"}
            aria-controls="privacy-panel-submit"
            onClick={() => setActiveTab("submit")}
            onKeyDown={handleTabKeyDown}
            className={cn(
              "inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors",
              activeTab === "submit"
                ? "bg-background text-foreground shadow-xs"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Send className="size-4" aria-hidden="true" />
            Ingresar solicitud sin sesión
          </button>
          <button
            type="button"
            role="tab"
            id="privacy-tab-lookup"
            aria-selected={activeTab === "lookup"}
            aria-controls="privacy-panel-lookup"
            onClick={() => setActiveTab("lookup")}
            onKeyDown={handleTabKeyDown}
            className={cn(
              "inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors",
              activeTab === "lookup"
                ? "bg-background text-foreground shadow-xs"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Search className="size-4" aria-hidden="true" />
            Consultar solicitud por folio y código
          </button>
        </div>
      </div>

      {/* Pestaña 1: Ingresar solicitud sin sesión */}
      <section
        id="privacy-panel-submit"
        role="tabpanel"
        aria-labelledby="privacy-tab-submit"
        hidden={activeTab !== "submit"}
        className="mt-6"
      >
        {receipt ? (
          <div
            role="status"
            aria-labelledby="privacy-receipt-title"
            className="rounded-3xl border border-emerald-300 bg-background p-6 shadow-xs sm:p-8 dark:border-emerald-800"
          >
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="flex items-start gap-3">
                <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-xl bg-emerald-100 text-emerald-800 dark:bg-emerald-950/60 dark:text-emerald-300">
                  <FileCheck2 className="size-5" aria-hidden="true" />
                </span>
                <div>
                  <h2
                    id="privacy-receipt-title"
                    className="text-xl font-semibold text-foreground"
                  >
                    Acuse de recepción registrado
                  </h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {receipt.notice}
                  </p>
                </div>
              </div>
              <Badge variant={stateBadgeVariant(receipt.state)}>
                {REQUEST_STATE_LABELS[receipt.state]}
              </Badge>
            </div>

            <div className="mt-6 rounded-2xl border border-amber-300/80 bg-amber-50/70 p-4 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
              <p className="font-medium">
                Guarda tu código de seguimiento ahora: se muestra una sola vez.
              </p>
              <p className="mt-1 text-xs leading-relaxed opacity-90">
                Por protección de tus datos no reenviamos ni volvemos a mostrar
                el código de seguimiento. Necesitarás tanto el folio como el
                código para consultar el estado de tu expediente.
              </p>
            </div>

            <dl className="mt-6 grid gap-4 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-5 sm:grid-cols-2">
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Folio público
                </dt>
                <dd
                  data-testid="receipt-folio"
                  className="mt-1 font-mono text-base font-semibold text-foreground"
                >
                  {receipt.folio}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Código de seguimiento (única exhibición)
                </dt>
                <dd
                  data-testid="receipt-tracking-code"
                  className="mt-1 break-all font-mono text-base font-semibold text-foreground"
                >
                  {receipt.tracking_code}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Derecho ejercido
                </dt>
                <dd className="mt-1 text-sm font-medium text-foreground">
                  {RIGHT_TYPE_LABELS[receipt.right_type]}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Tipo de solicitante
                </dt>
                <dd className="mt-1 text-sm text-foreground">
                  {REQUESTER_TYPE_LABELS[receipt.requester_type]}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Plazo de respuesta
                </dt>
                <dd className="mt-1 flex items-center gap-1.5 text-sm font-medium text-foreground">
                  <CalendarClock
                    className="size-4 text-primary"
                    aria-hidden="true"
                  />
                  <span>
                    {formatDateTimeCL(receipt.due_at) ?? receipt.due_at} (
                    {receipt.right_type === "bloqueo"
                      ? "2 días hábiles"
                      : "30 días corridos"}
                    )
                  </span>
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Fecha de registro
                </dt>
                <dd className="mt-1 text-sm text-foreground">
                  {formatDateTimeCL(receipt.created_at) ?? receipt.created_at}
                </dd>
              </div>
            </dl>

            <div className="mt-6 flex flex-wrap gap-3">
              <Button
                type="button"
                variant="outline"
                onClick={() => void handleCopyCredentials()}
              >
                {copiedCredentials ? (
                  <>
                    <Check className="size-4" aria-hidden="true" />
                    Folio y código copiados
                  </>
                ) : (
                  <>
                    <Copy className="size-4" aria-hidden="true" />
                    Copiar folio y código
                  </>
                )}
              </Button>
              <Button type="button" onClick={handleGoToLookupWithReceipt}>
                Consultar este folio en seguimiento
                <ArrowRight className="size-4" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={handleResetForNewRequest}
              >
                Ingresar otra solicitud
              </Button>
            </div>
          </div>
        ) : (
          <form
            onSubmit={(event) => void handleIntakeSubmit(event)}
            noValidate
            className="space-y-8 rounded-3xl border border-border bg-background p-6 sm:p-8"
          >
            {submitError && (
              <div
                role="alert"
                className="flex items-start gap-2.5 rounded-2xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive"
              >
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span>{submitError}</span>
              </div>
            )}

            <fieldset className="space-y-3">
              <legend className="text-base font-semibold text-foreground">
                1. Selecciona el derecho ARCO+ que deseas ejercer
              </legend>
              <p className="text-sm text-muted-foreground">
                Las solicitudes generales se responden dentro de{" "}
                <strong>30 días corridos</strong>. El derecho de{" "}
                <strong>Bloqueo temporal</strong> cuenta con atención
                prioritaria en <strong>2 días hábiles</strong>.
              </p>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {PUBLIC_PRIVACY_RIGHT_OPTIONS.map((option) => {
                  const selected = rightType === option.value;
                  return (
                    <label
                      key={option.value}
                      className={cn(
                        "flex cursor-pointer flex-col justify-between rounded-2xl border p-4 transition-all focus-within:ring-2 focus-within:ring-ring",
                        selected
                          ? "border-primary bg-[var(--brand-primary-soft)]/50"
                          : "border-border bg-background hover:border-foreground/30",
                        option.isUrgentSla && !selected
                          ? "border-amber-300/80 dark:border-amber-800/70"
                          : "",
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <input
                          type="radio"
                          name="right_type"
                          value={option.value}
                          checked={selected}
                          onChange={() => setRightType(option.value)}
                          disabled={submitting}
                          className="mt-1 size-4 shrink-0 accent-primary"
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="font-semibold text-foreground">
                              {option.label}
                            </span>
                            <Badge
                              variant={
                                option.isUrgentSla ? "default" : "secondary"
                              }
                            >
                              <Clock className="size-3" aria-hidden="true" />
                              {option.slaLabel}
                            </Badge>
                          </div>
                          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
                            {option.description}
                          </p>
                        </div>
                      </div>
                    </label>
                  );
                })}
              </div>
            </fieldset>

            <fieldset className="space-y-3">
              <legend className="text-base font-semibold text-foreground">
                2. Calidad del solicitante
              </legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {(["titular", "representante"] as const).map((kind) => {
                  const selected = requesterType === kind;
                  return (
                    <label
                      key={kind}
                      className={cn(
                        "flex cursor-pointer items-center justify-between rounded-2xl border px-4 py-3 text-sm transition-all focus-within:ring-2 focus-within:ring-ring",
                        selected
                          ? "border-primary bg-[var(--brand-primary-soft)]/50 font-medium text-foreground"
                          : "border-border bg-background text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <span className="flex items-center gap-2.5">
                        <input
                          type="radio"
                          name="requester_type"
                          value={kind}
                          checked={selected}
                          onChange={() => setRequesterType(kind)}
                          disabled={submitting}
                          className="size-4 shrink-0 accent-primary"
                        />
                        <span>{REQUESTER_TYPE_LABELS[kind]}</span>
                      </span>
                      {selected && (
                        <CheckCircle2
                          className="size-4 text-primary"
                          aria-hidden="true"
                        />
                      )}
                    </label>
                  );
                })}
              </div>
            </fieldset>

            {requesterType === "representante" && (
              <div className="grid gap-4 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-5 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="representative-name">
                    Nombre del representante
                  </Label>
                  <Input
                    id="representative-name"
                    type="text"
                    required
                    maxLength={160}
                    value={representativeName}
                    onChange={(event) =>
                      setRepresentativeName(event.target.value)
                    }
                    disabled={submitting}
                    placeholder="Ej. Camila Rojas Soto"
                    className="rounded-xl bg-background"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="representative-relation">
                    Relación de representación
                  </Label>
                  <select
                    id="representative-relation"
                    required
                    value={representativeRelation}
                    onChange={(event) =>
                      setRepresentativeRelation(
                        event.target
                          .value as PublicPrivacyRepresentativeRelation,
                      )
                    }
                    disabled={submitting}
                    className="h-9 w-full rounded-xl border border-input bg-background px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  >
                    {REPRESENTATIVE_RELATION_OPTIONS.map((item) => (
                      <option key={item.value} value={item.value}>
                        {item.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            )}

            <div className="space-y-4">
              <h2 className="text-base font-semibold text-foreground">
                3. Datos de contacto y detalle de la solicitud
              </h2>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="requester-name">
                    Nombre completo del solicitante (titular)
                  </Label>
                  <Input
                    id="requester-name"
                    type="text"
                    required
                    maxLength={160}
                    value={requesterName}
                    onChange={(event) => setRequesterName(event.target.value)}
                    disabled={submitting}
                    placeholder="Nombre y apellidos del titular"
                    className="rounded-xl"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="requester-email">
                    Correo electrónico de contacto
                  </Label>
                  <Input
                    id="requester-email"
                    type="email"
                    required
                    maxLength={254}
                    value={requesterEmail}
                    onChange={(event) => setRequesterEmail(event.target.value)}
                    disabled={submitting}
                    placeholder="correo@ejemplo.cl"
                    className="rounded-xl"
                  />
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="subject-reference">
                  Referencia del titular (opcional)
                </Label>
                <Input
                  id="subject-reference"
                  type="text"
                  maxLength={200}
                  value={subjectReference}
                  onChange={(event) => setSubjectReference(event.target.value)}
                  disabled={submitting}
                  placeholder="Opcional: contexto o referencia para ubicar el registro (2 a 200 caracteres)"
                  className="rounded-xl"
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="request-detail">
                  Detalle del requerimiento
                </Label>
                <textarea
                  id="request-detail"
                  required
                  maxLength={DETAIL_MAX_LENGTH}
                  value={detail}
                  onChange={(event) => setDetail(event.target.value)}
                  disabled={submitting}
                  placeholder="Describe con claridad qué datos deseas acceder, rectificar, suprimir, portar, oponerte o bloquear temporalmente (mínimo 10 caracteres)."
                  aria-describedby="request-detail-help"
                  className="min-h-28 w-full rounded-xl border border-input bg-input/30 px-3 py-2 text-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
                />
                <p
                  id="request-detail-help"
                  className="flex justify-between text-xs text-muted-foreground"
                >
                  <span>
                    Mínimo {DETAIL_MIN_LENGTH} caracteres. No compartas
                    contraseñas ni datos bancarios.
                  </span>
                  <span>
                    {detail.length}/{DETAIL_MAX_LENGTH}
                  </span>
                </p>
              </div>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-4 border-t border-border pt-6">
              <p className="text-xs text-muted-foreground">
                Recibirás un folio público y un código de seguimiento único.
              </p>
              <Button type="submit" size="lg" disabled={submitting}>
                {submitting && (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                )}
                Enviar solicitud ARCO+
              </Button>
            </div>
          </form>
        )}
      </section>

      {/* Pestaña 2: Consultar solicitud por folio y código */}
      <section
        id="privacy-panel-lookup"
        role="tabpanel"
        aria-labelledby="privacy-tab-lookup"
        hidden={activeTab !== "lookup"}
        className="mt-6 space-y-6"
      >
        <form
          onSubmit={(event) => void handleLookupSubmit(event)}
          noValidate
          className="space-y-5 rounded-3xl border border-border bg-background p-6 sm:p-8"
        >
          <div>
            <h2 className="text-lg font-semibold text-foreground">
              Seguimiento de solicitud por folio y código
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Ingresa el folio público y el código de seguimiento de una sola
              exhibición entregado al registrar tu solicitud.
            </p>
          </div>

          {lookupError && (
            <div
              role="alert"
              className="flex items-start gap-2.5 rounded-2xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive"
            >
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <span>{lookupError}</span>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="lookup-folio">Folio</Label>
              <Input
                id="lookup-folio"
                type="text"
                required
                maxLength={40}
                value={lookupFolio}
                onChange={(event) => setLookupFolio(event.target.value)}
                disabled={lookingUp}
                placeholder="PRIV-2026-XXXXXXXX"
                className="rounded-xl font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="lookup-tracking-code">
                Código de seguimiento
              </Label>
              <Input
                id="lookup-tracking-code"
                type="text"
                required
                maxLength={128}
                value={lookupCode}
                onChange={(event) => setLookupCode(event.target.value)}
                disabled={lookingUp}
                placeholder="Código entregado en el acuse"
                className="rounded-xl font-mono"
              />
            </div>
          </div>

          <div className="flex justify-end">
            <Button type="submit" size="lg" disabled={lookingUp}>
              {lookingUp ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <Search className="size-4" aria-hidden="true" />
              )}
              Consultar estado
            </Button>
          </div>
        </form>

        {lookupStatus && (
          <div
            role="status"
            aria-labelledby="lookup-status-title"
            className="space-y-6 rounded-3xl border border-border bg-background p-6 sm:p-8"
          >
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
                  Folio {lookupStatus.folio}
                </p>
                <h3
                  id="lookup-status-title"
                  className="mt-1 text-xl font-semibold text-foreground"
                >
                  {RIGHT_TYPE_LABELS[lookupStatus.right_type]}
                </h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  Solicitante:{" "}
                  {REQUESTER_TYPE_LABELS[lookupStatus.requester_type]}
                </p>
              </div>
              <Badge variant={stateBadgeVariant(lookupStatus.state)}>
                {REQUEST_STATE_LABELS[lookupStatus.state]}
              </Badge>
            </div>

            <dl className="grid gap-4 rounded-2xl border border-border bg-[var(--brand-surface-subtle)] p-5 sm:grid-cols-2">
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Fecha de creación
                </dt>
                <dd className="mt-1 text-sm text-foreground">
                  {formatDateTimeCL(lookupStatus.created_at) ??
                    lookupStatus.created_at}
                </dd>
              </div>
              {lookupStatus.due_at && (
                <div>
                  <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Plazo de respuesta
                  </dt>
                  <dd className="mt-1 text-sm font-medium text-foreground">
                    {formatDateTimeCL(lookupStatus.due_at) ??
                      lookupStatus.due_at}
                  </dd>
                </div>
              )}
              {lookupStatus.extended_due_at && (
                <div className="sm:col-span-2">
                  <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Plazo prorrogado
                  </dt>
                  <dd className="mt-1 text-sm font-medium text-foreground">
                    {formatDateTimeCL(lookupStatus.extended_due_at) ??
                      lookupStatus.extended_due_at}
                    {lookupStatus.extension_reason && (
                      <span className="mt-1 block font-normal text-muted-foreground">
                        Motivo de prórroga: {lookupStatus.extension_reason}
                      </span>
                    )}
                  </dd>
                </div>
              )}
              {lookupStatus.resolved_at && (
                <div>
                  <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    Fecha de resolución
                  </dt>
                  <dd className="mt-1 text-sm text-foreground">
                    {formatDateTimeCL(lookupStatus.resolved_at) ??
                      lookupStatus.resolved_at}
                  </dd>
                </div>
              )}
            </dl>

            {lookupStatus.response && (
              <div className="rounded-2xl border border-border bg-[var(--brand-primary-soft)]/35 p-5">
                <h4 className="text-sm font-semibold text-foreground">
                  Respuesta publicada
                </h4>
                <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
                  {lookupStatus.response}
                </p>
              </div>
            )}

            <div className="border-t border-border pt-5">
              <h4 className="text-sm font-semibold text-foreground">
                Línea de tiempo del expediente
              </h4>
              {timelineEvents.length === 0 ? (
                <p className="mt-2 text-sm text-muted-foreground">
                  No hay eventos adicionales registrados hasta el momento.
                </p>
              ) : (
                <ol className="mt-4 space-y-4">
                  {timelineEvents.map((ev) => (
                    <li
                      key={ev.id}
                      className="relative border-l-2 border-border pl-4"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="text-sm font-medium text-foreground">
                          {EVENT_TYPE_LABELS[ev.event_type] ?? ev.event_type}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {formatDateTimeCL(ev.created_at) ?? ev.created_at}
                        </span>
                      </div>
                      {ev.state_to && (
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          Estado:{" "}
                          {ev.state_from
                            ? `${stateLabel(ev.state_from)} → `
                            : ""}
                          {stateLabel(ev.state_to)}
                        </p>
                      )}
                      {ev.new_due_at && (
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          Nuevo plazo:{" "}
                          {formatDateTimeCL(ev.new_due_at) ?? ev.new_due_at}
                        </p>
                      )}
                      {ev.detail && (
                        <p className="mt-1 whitespace-pre-wrap text-xs text-muted-foreground">
                          {ev.detail}
                        </p>
                      )}
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        )}
      </section>
    </article>
  );
}
