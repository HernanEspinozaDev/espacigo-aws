"use client";

import { useEffect, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Building2,
  MapPin,
  DollarSign,
  Calendar,
  Eye,
  Loader2,
  AlertCircle,
  Info,
  ListChecks,
  CheckCircle2,
  Clock,
  Lock,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/features/auth/ui/use-auth";
import { authFacade } from "@/features/auth/auth-runtime";
import { listingErrorMessage } from "../domain/listing-error-message";
import {
  mercadoPagoReturnHref,
  verificationReturnHref,
} from "../domain/publish-guide";
import {
  PublishGuideModal,
  usePublishPrerequisites,
} from "./publish-guide-modal";
import {
  cancellationPolicies,
  cancellationPolicyVersion,
} from "../domain/cancellation-policy";
import type { CancellationPolicyVersion } from "../domain/cancellation-policy";
import { CHILE_COMUNAS, CHILE_REGIONS } from "../domain/chile-dpa";
import { validateLocation } from "../domain/commercial-location";
import { CommercialLocationFields } from "./commercial-location-fields";
import { validateWizardLocation } from "../domain/wizard-location";
import { reverseGeocodeCommercialPin } from "../commercial-location-runtime";
import { useCommercialPinResolution } from "./use-commercial-pin-resolution";
import { CHILE_TIME_ZONE_OPTIONS } from "../domain/chile-time-zones";
import {
  cleanAttributesForPayload,
  formatAttributeLabel,
  formatAttributeValue,
} from "../domain/attribute-formatting";
import { useHostListings } from "./use-host-listings";
import {
  clearPartialDraftRecoveryPending,
  hasPendingPartialDraftRecovery,
  markPartialDraftRecoveryPending,
} from "./partial-draft-recovery-state";
import { WeeklyScheduleSlots } from "./weekly-schedule-slots";
import { HostAvailabilityCalendar } from "./host-availability-calendar";
import {
  validateCategoryAttributes,
  validateListingDraftInput,
  validateScheduleSlots,
  isValidIanaTimeZone,
  isValidAreaM2,
  isValidCapacity,
} from "../domain/validation";
import type {
  Category,
  ListingCreateInput,
  RateUnit,
  ScheduleSlot,
} from "../domain/types";

const STEPS = [
  { number: 1, title: "Tipo y Resumen", icon: Building2 },
  { number: 2, title: "Ubicación", icon: MapPin },
  { number: 3, title: "Atributos", icon: Building2 },
  { number: 4, title: "Tarifas y Reglas", icon: DollarSign },
  { number: 5, title: "Disponibilidad", icon: Calendar },
  { number: 6, title: "Confirmación", icon: Eye },
];

export function ListingWizard() {
  const router = useRouter();
  const {
    snapshot: authSnapshot,
    checkSession,
    accessToken,
    getAccessToken,
  } = useAuth();
  const recoveryScope = authFacade.getSessionIdentityScope();
  const {
    snapshot,
    loadCategories,
    loadCategorySchema,
    createDraft,
    updateWeeklySchedule,
    configureTimeZone,
  } = useHostListings();

  const [currentStep, setCurrentStep] = useState<number>(1);
  const [maxVisitedStep, setMaxVisitedStep] = useState<number>(1);
  const [returnToConfirmation, setReturnToConfirmation] = useState(false);
  const [stepError, setStepError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [guideOpen, setGuideOpen] = useState(false);
  const isAuthenticated =
    authSnapshot.status === "authenticated" && Boolean(accessToken);
  const isHost =
    authSnapshot.status === "authenticated" &&
    authSnapshot.roles.includes("arrendador");
  const prerequisites = usePublishPrerequisites({
    accessToken: isAuthenticated ? accessToken : null,
    isHost,
    enabled: isAuthenticated,
  });
  const [partialDraft, setPartialDraft] = useState<{
    id: string | null;
    accountId: string | null;
  } | null>(() =>
    hasPendingPartialDraftRecovery(recoveryScope)
      ? { id: null, accountId: null }
      : null,
  );
  const [, startTransition] = useTransition();

  // Paso 1: Tipo y resumen
  const [categoryCode, setCategoryCode] = useState<string>("");
  const [title, setTitle] = useState<string>("");
  const [description, setDescription] = useState<string>("");

  // Paso 2: Ubicación, dimensiones y atributos dinámicos
  const pinResolution = useCommercialPinResolution({
    initial: {
      address: "",
      region_code: "",
      comuna_code: "",
      latitude: null,
      longitude: null,
      public_confirmed: false,
    },
    resolvePin: reverseGeocodeCommercialPin,
    onChange: () => {},
  });
  const {
    location: pin,
    onLocationChange,
    selectPin,
    isResolved,
  } = pinResolution;
  const { address, region_code: regionCode, comuna_code: comunaCode } = pin;
  const [areaM2, setAreaM2] = useState<number>(25);
  const [capacity, setCapacity] = useState<number>(4);
  const [dynamicAttributes, setDynamicAttributes] = useState<
    Record<string, unknown>
  >({});
  const [schemaLoadError, setSchemaLoadError] = useState<{
    categoryCode: string;
  } | null>(null);
  const [schemaRetryToken, setSchemaRetryToken] = useState(0);
  const currentSchema = categoryCode
    ? (snapshot.categorySchemas[categoryCode] ?? null)
    : null;
  const schemaFailed = Boolean(
    categoryCode &&
    !currentSchema &&
    schemaLoadError?.categoryCode === categoryCode,
  );
  const loadingSchema = Boolean(
    categoryCode && !currentSchema && !schemaFailed,
  );

  // Paso 3: Tarifas
  const [rateUnit, setRateUnit] = useState<RateUnit>("hora");
  const [basePriceCLP, setBasePriceCLP] = useState<number>(15000);
  const [cancellationPolicy, setCancellationPolicy] =
    useState<CancellationPolicyVersion>("flexible_v1");
  const [usageRules, setUsageRules] = useState<string>(
    "No fumar. Cuidar el orden y limpieza de las instalaciones.",
  );

  // Paso 5: Disponibilidad semanal
  const [timeZone, setTimeZone] = useState<string>("America/Santiago");
  const [scheduleEnabled, setScheduleEnabled] = useState<boolean>(true);
  const [slots, setSlots] = useState<ScheduleSlot[]>([
    { day_of_week: 1, start_time: "09:00", end_time: "18:00" },
    { day_of_week: 2, start_time: "09:00", end_time: "18:00" },
    { day_of_week: 3, start_time: "09:00", end_time: "18:00" },
    { day_of_week: 4, start_time: "09:00", end_time: "18:00" },
    { day_of_week: 5, start_time: "09:00", end_time: "18:00" },
  ]);

  // Cargar categorías dinámicamente de Go
  useEffect(() => {
    if (!accessToken) return;
    loadCategories(accessToken).catch(() => {});
  }, [loadCategories, accessToken]);

  // Si se selecciona o cambia categoría, cargar esquema de Go.
  // El wizard conserva el bloqueo del paso 2 hasta que exista el perfil vigente.
  useEffect(() => {
    if (!accessToken || !categoryCode || currentSchema) return;

    let active = true;
    loadCategorySchema(accessToken, categoryCode)
      .then((schema) => {
        if (active && !schema) setSchemaLoadError({ categoryCode });
      })
      .catch(() => {
        if (active) setSchemaLoadError({ categoryCode });
      });

    return () => {
      active = false;
    };
  }, [
    categoryCode,
    currentSchema,
    loadCategorySchema,
    schemaRetryToken,
    accessToken,
  ]);

  const handleAttributeChange = (code: string, value: unknown) => {
    setDynamicAttributes((prev) => {
      const next = { ...prev, [code]: value };
      if (
        categoryCode === "quincho" &&
        code === "tipo_parrilla" &&
        value === "sin_parrilla" &&
        (next.parrillas_disponibles === undefined ||
          (typeof next.parrillas_disponibles === "number" &&
            next.parrillas_disponibles > 0))
      ) {
        next.parrillas_disponibles = 0;
      }
      return next;
    });
  };

  const clearScheduleError = () => {
    const scheduleError = fieldErrors.schedule;
    setFieldErrors((prev) => {
      if (!prev.schedule) return prev;
      const next = { ...prev };
      delete next.schedule;
      return next;
    });
    if (scheduleError) {
      setStepError((prev) => (prev === scheduleError ? null : prev));
    }
  };

  const validateStep = (step: number): boolean => {
    setStepError(null);
    setFieldErrors({});
    const errors: Record<string, string> = {};
    if (step === 1) {
      if (!categoryCode)
        errors.category = "Debes seleccionar un tipo de espacio.";
      if (!title.trim()) errors.title = "El título es obligatorio.";
      else if (Array.from(title).length > 70)
        errors.title = "El título no puede superar los 70 caracteres.";
    } else if (step === 2) {
      const location = validateWizardLocation(regionCode, comunaCode);
      if (!address.trim()) errors.address = "La dirección es obligatoria.";
      else if (Array.from(address).length > 500)
        errors.address = "La dirección no puede superar los 500 caracteres.";
      if (!location.valid) errors.comuna = location.message;
      if (!isResolved)
        errors.location =
          "Ajusta el pin y espera a que se confirme la región y comuna.";
    } else if (step === 3) {
      if (!isValidAreaM2(areaM2))
        errors.area =
          "La superficie debe estar entre 0,01 y 99.999.999,99 m², con hasta 2 decimales.";
      if (!isValidCapacity(capacity))
        errors.capacity = "La capacidad debe ser un entero positivo.";
      if (currentSchema) {
        const attrValidation = validateCategoryAttributes(
          cleanAttributesForPayload(dynamicAttributes),
          currentSchema,
          Number(areaM2),
        );
        if (!attrValidation.valid)
          Object.entries(attrValidation.errors).forEach(([code, message]) => {
            errors[`attribute:${code}`] = message;
          });
      }
    } else if (step === 4) {
      if (basePriceCLP < 1000 || !Number.isSafeInteger(basePriceCLP))
        errors.price =
          "El precio base debe ser un monto entero de al menos 1.000 CLP.";
      if (!usageRules.trim())
        errors.usageRules = "Las reglas de uso son obligatorias.";
      else if (Array.from(usageRules.trim()).length > 250)
        errors.usageRules =
          "Las reglas de uso no pueden superar 250 caracteres.";
      if (!["hora", "dia", "mes"].includes(rateUnit)) {
        setStepError("Selecciona una unidad de cobro válida.");
        return false;
      }
      if (Object.keys(errors).length) {
        setFieldErrors(errors);
        setStepError(Object.values(errors)[0]);
        requestAnimationFrame(() =>
          document
            .getElementById(
              `wizard-${Object.keys(errors)[0] === "usageRules" ? "usage-rules" : Object.keys(errors)[0]}`,
            )
            ?.focus(),
        );
        return false;
      }
      return true;
    } else if (step === 5) {
      let message: string | null = null;
      if (!isValidIanaTimeZone(timeZone)) {
        message =
          "La zona horaria debe ser un identificador IANA válido (ej: America/Santiago).";
        errors.timezone = message;
      } else if (scheduleEnabled && rateUnit === "hora" && slots.length > 0) {
        const v = validateScheduleSlots(slots);
        if (!v.valid) {
          message = v.errors[0] ?? "Horarios con tramos inválidos.";
          errors.schedule = message;
        }
      }
      setFieldErrors(errors);
      setStepError(message);
      if (errors.timezone || errors.schedule) {
        const targetId = errors.timezone
          ? "wizard-timezone"
          : "wizard-schedule-group";
        requestAnimationFrame(() => document.getElementById(targetId)?.focus());
      }
      return message === null;
    }
    if (Object.keys(errors).length) {
      setFieldErrors(errors);
      const first = Object.keys(errors)[0];
      setStepError(errors[first]);
      requestAnimationFrame(() => {
        if (first.startsWith("attribute:")) {
          const code = first.slice("attribute:".length);
          const definition = currentSchema?.attributes.find(
            (attribute) => attribute.code === code,
          );
          const targetId =
            definition?.type === "enum_list"
              ? `attr-${code}-option-0`
              : `attr-${code}`;
          document.getElementById(targetId)?.focus();
          return;
        }
        document.getElementById(`wizard-${first}`)?.focus();
      });
      return false;
    }
    return true;
  };

  const fieldFeedback = (name: string) => ({
    "aria-invalid": fieldErrors[name] ? true : undefined,
    "aria-describedby":
      [
        name === "description"
          ? "wizard-description-count"
          : name === "address"
            ? "wizard-address-count"
            : name === "usageRules"
              ? "wizard-usage-rules-count"
              : undefined,
        fieldErrors[name]
          ? `wizard-${name === "usageRules" ? "usage-rules" : name}-error`
          : undefined,
      ]
        .filter(Boolean)
        .join(" ") || undefined,
  });
  const clearFieldError = (name: string) => {
    setFieldErrors((prev) => {
      const next = { ...prev };
      delete next[name];
      return next;
    });
    setStepError(null);
  };

  const handleNext = () => {
    if (validateStep(currentStep)) {
      const next = returnToConfirmation ? 6 : Math.min(6, currentStep + 1);
      setReturnToConfirmation(false);
      setCurrentStep(next);
      setMaxVisitedStep((prev) => Math.max(prev, next));
    }
  };

  const handleBack = () => {
    setStepError(null);
    setCurrentStep((prev) => Math.max(1, prev - 1));
  };

  const handleCreateDraft = async () => {
    if (isSubmitting || partialDraft) return;
    if (
      authSnapshot.status !== "authenticated" ||
      !accessToken ||
      !recoveryScope
    ) {
      setStepError("Sesión no disponible. Inicia sesión nuevamente.");
      return;
    }

    for (let step = 1; step <= 5; step += 1) {
      if (!validateStep(step)) {
        setCurrentStep(step);
        return;
      }
    }
    const location = validateWizardLocation(regionCode, comunaCode);
    if (!location.valid) {
      setStepError(location.message);
      return;
    }
    if (!isResolved || pin.latitude === null || pin.longitude === null) {
      setCurrentStep(2);
      setStepError(
        "Ajusta el pin y espera a que se confirme la región y comuna.",
      );
      return;
    }

    // Validación final completa
    if (!currentSchema) {
      setStepError("Espera a que cargue el perfil de la categoría.");
      return;
    }
    const cleanAttributes = cleanAttributesForPayload(dynamicAttributes);
    const draftPayload: ListingCreateInput = {
      category_code: categoryCode,
      title: title.trim(),
      description: description.trim(),
      area_m2: Number(areaM2),
      capacity: Number(capacity),
      usage_rules: usageRules.trim(),
      cancellation_policy_version: cancellationPolicy,
      rate_unit: rateUnit,
      base_price_clp: Number(basePriceCLP),
      location: {
        ...pin,
        latitude: pin.latitude,
        longitude: pin.longitude,
      },
      attribute_schema_version: currentSchema.schema_version,
      attributes: cleanAttributes,
    };

    const overallVal = validateListingDraftInput(draftPayload);
    if (!overallVal.valid) {
      const firstErr = Object.values(overallVal.errors)[0];
      setStepError(
        firstErr ?? "Por favor completa todos los campos requeridos.",
      );
      return;
    }

    try {
      validateLocation(pin);
    } catch (error) {
      setStepError(
        error instanceof Error ? error.message : "Revisa la ubicación.",
      );
      setCurrentStep(2);
      return;
    }

    setIsSubmitting(true);
    setStepError(null);

    let createdDraftId: string | null = null;
    try {
      const created = await createDraft(accessToken, draftPayload);
      createdDraftId = created.id;
      markPartialDraftRecoveryPending(recoveryScope);

      if (!authSnapshot.roles.includes("arrendador")) {
        await checkSession().catch(() => false);
      }

      const currentScope = getAccessToken();
      if (
        !currentScope ||
        authFacade.getSnapshot().accountId !== authSnapshot.accountId
      ) {
        throw new DOMException("La sesión cambió.", "AbortError");
      }

      // Configure timezone with the create response ETag.
      const createdEtag = created.etag;
      if (!createdEtag) {
        throw new Error("No se pudo verificar la versión del borrador creado.");
      }
      const configured = await configureTimeZone(
        currentScope,
        created.id,
        timeZone,
        createdEtag,
      );
      if (currentScope !== getAccessToken())
        throw new DOMException("La sesión cambió.", "AbortError");
      if (scheduleEnabled && rateUnit === "hora" && slots.length > 0) {
        await updateWeeklySchedule(
          currentScope,
          created.id,
          {
            timezone: timeZone,
            enabled: true,
            weekly_slots: slots,
          },
          configured.etag,
        );
      }

      // Redirigir al editor tras el alta dentro de mi cuenta
      clearPartialDraftRecoveryPending();
      startTransition(() => {
        router.push(`/anfitrion/espacios/${created.id}/editar?tab=fotos`);
      });
    } catch (err) {
      if (createdDraftId) {
        setPartialDraft({
          id: createdDraftId,
          accountId: authSnapshot.accountId,
        });
        setStepError(null);
      } else {
        setStepError(listingErrorMessage(err));
      }
      setIsSubmitting(false);
    }
  };

  const selectedCatObj = snapshot.categories.find(
    (c: Category) => c.code === categoryCode,
  );
  const canRecoverPartialDraft =
    Boolean(partialDraft?.id && getAccessToken()) &&
    authFacade.getSnapshot().accountId === partialDraft?.accountId;

  return (
    <div className="max-w-4xl mx-auto space-y-8 pb-12">
      {/* Cabecera del Asistente */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-2 border-b border-border">
        <div>
          <h1 className="text-2xl lg:text-3xl font-bold tracking-tight text-foreground">
            Publicar un nuevo espacio
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Completa los 6 pasos para dar de alta tu propiedad en EspaciGo.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 shrink-0">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => {
              setGuideOpen(true);
              prerequisites.reload();
            }}
          >
            <ListChecks className="size-4 text-primary" aria-hidden="true" />
            Guía para publicar
          </Button>
          <Button asChild variant="outline" size="sm">
            <Link href="/anfitrion/espacios" className="gap-1.5">
              <ArrowLeft className="size-4" aria-hidden="true" />
              Mis espacios
            </Link>
          </Button>
        </div>
      </div>

      {/* Guía compacta de requisitos previos para publicar */}
      {prerequisites.loaded &&
        !prerequisites.error &&
        (!prerequisites.kycVerified || (isHost && !prerequisites.mpConnected)) && (
          <section
            aria-label="Requisitos previos para publicar"
            className="rounded-xl border border-amber-300 bg-amber-50/70 p-4 space-y-3"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="space-y-1">
                <h2 className="text-sm font-semibold text-amber-950">
                  Antes de publicar tu espacio, necesitarás completar estos pasos
                </h2>
                <p className="text-xs text-amber-900">
                  Puedes crear tu borrador ahora, pero para activarlo en el
                  catálogo deberás confirmar tu identidad, vincular Mercado Pago
                  y publicar la plantilla de contrato en el editor.
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="border-amber-400 bg-white text-amber-950 hover:bg-amber-100"
                onClick={() => setGuideOpen(true)}
              >
                Ver guía completa
              </Button>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-4 gap-2 text-xs">
              <div className="flex items-center justify-between gap-2 rounded-lg border border-amber-200 bg-white px-3 py-2">
                <span className="font-medium">1. Identidad (KYC)</span>
                {prerequisites.kycVerified ? (
                  <Badge className="bg-emerald-600 text-white gap-1">
                    <CheckCircle2 className="size-3" aria-hidden="true" />
                    Listo
                  </Badge>
                ) : (
                  <Link
                    href={verificationReturnHref("/anfitrion/espacios/nuevo")}
                    className="font-semibold text-primary hover:underline inline-flex items-center gap-1"
                  >
                    Completar
                    <ArrowRight className="size-3" aria-hidden="true" />
                  </Link>
                )}
              </div>

              <div className="flex items-center justify-between gap-2 rounded-lg border border-amber-200 bg-white px-3 py-2">
                <span className="font-medium">2. Mercado Pago</span>
                {prerequisites.mpConnected ? (
                  <Badge className="bg-emerald-600 text-white gap-1">
                    <CheckCircle2 className="size-3" aria-hidden="true" />
                    Listo
                  </Badge>
                ) : isHost ? (
                  <Link
                    href={mercadoPagoReturnHref("/anfitrion/espacios/nuevo")}
                    className="font-semibold text-primary hover:underline inline-flex items-center gap-1"
                  >
                    Vincular
                    <ArrowRight className="size-3" aria-hidden="true" />
                  </Link>
                ) : (
                  <span className="text-muted-foreground inline-flex items-center gap-1">
                    <Lock className="size-3" aria-hidden="true" />
                    Tras borrador
                  </span>
                )}
              </div>

              <div className="flex items-center justify-between gap-2 rounded-lg border border-amber-200 bg-white px-3 py-2">
                <span className="font-medium">3. Borrador</span>
                <Badge
                  variant="outline"
                  className="border-amber-400 text-amber-800 gap-1"
                >
                  <Clock className="size-3" aria-hidden="true" />
                  En curso
                </Badge>
              </div>

              <div className="flex items-center justify-between gap-2 rounded-lg border border-amber-200 bg-white px-3 py-2">
                <span className="font-medium">4. Contrato</span>
                <span className="text-muted-foreground inline-flex items-center gap-1">
                  <Lock className="size-3" aria-hidden="true" />
                  En editor
                </span>
              </div>
            </div>
          </section>
        )}

      <PublishGuideModal
        open={guideOpen}
        onOpenChange={setGuideOpen}
        mode="wizard"
        isHost={isHost}
        prerequisites={prerequisites}
        onRefresh={prerequisites.reload}
        returnPath="/anfitrion/espacios/nuevo"
      />

      {/* Indicador de progreso de 6 pasos accesibles */}
      <nav aria-label="Pasos del asistente de publicación">
        <ol className="grid grid-cols-2 sm:grid-cols-6 gap-2 text-xs font-medium">
          {STEPS.map((s) => {
            const isCompleted = s.number < currentStep;
            const isCurrent = s.number === currentStep;
            const canVisit = s.number <= maxVisitedStep && !isSubmitting;
            const StepIcon = s.icon;

            return (
              <li
                key={s.number}
                aria-current={isCurrent ? "step" : undefined}
                className={`flex items-center gap-2 p-2.5 rounded-lg border transition-colors ${
                  isCurrent
                    ? "border-primary bg-primary/10 text-primary font-semibold shadow-xs"
                    : isCompleted
                      ? "border-border bg-muted/60 text-foreground"
                      : "border-transparent bg-muted/20 text-muted-foreground"
                }`}
              >
                <button
                  type="button"
                  disabled={!canVisit}
                  aria-label={`${s.title}${isCurrent ? ", paso actual" : ""}`}
                  onClick={() => {
                    if (s.number < currentStep) {
                      setStepError(null);
                      setCurrentStep(s.number);
                      return;
                    }
                    if (s.number > currentStep) {
                      for (let step = 1; step < s.number; step += 1) {
                        if (!validateStep(step)) {
                          setCurrentStep(step);
                          return;
                        }
                      }
                      setCurrentStep(s.number);
                      setMaxVisitedStep((prev) => Math.max(prev, s.number));
                    }
                  }}
                  className="flex w-full items-center gap-2 text-left disabled:cursor-not-allowed"
                >
                  <div
                    className={`flex size-6 items-center justify-center rounded-full text-xs font-bold ${
                      isCurrent
                        ? "bg-primary text-primary-foreground"
                        : isCompleted
                          ? "bg-emerald-600 text-white"
                          : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {isCompleted ? (
                      <Check className="size-3.5" aria-hidden="true" />
                    ) : (
                      <StepIcon className="size-3.5" aria-hidden="true" />
                    )}
                  </div>
                  <span className="truncate">{s.title}</span>
                </button>
              </li>
            );
          })}
        </ol>
      </nav>

      {/* Mensaje de alerta / error */}
      {(stepError || partialDraft) && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-sm text-destructive flex items-start gap-3"
        >
          <AlertCircle className="size-5 shrink-0 mt-0.5" aria-hidden="true" />
          <div className="flex-1">
            {partialDraft ? (
              <>
                {partialDraft.id
                  ? "El borrador fue creado, pero no se pudo completar su configuración inicial. Continúa desde el borrador guardado antes de crear otro."
                  : "La sesión cambió después de iniciar el alta. Revisa Mis espacios antes de volver a intentarlo."}{" "}
                <Link
                  className="font-semibold underline"
                  onClick={() => {
                    clearPartialDraftRecoveryPending();
                  }}
                  href={
                    canRecoverPartialDraft
                      ? `/anfitrion/espacios/${partialDraft.id}/editar`
                      : "/anfitrion/espacios"
                  }
                >
                  {canRecoverPartialDraft
                    ? "Ir al editor del borrador"
                    : "Ir a Mis espacios"}
                </Link>
              </>
            ) : (
              stepError
            )}
          </div>
        </div>
      )}

      {/* Contenido del paso actual */}
      <Card className="shadow-sm">
        {/* PASO 1: Tipo de espacio, título y resumen */}
        {currentStep === 1 && (
          <>
            <CardHeader>
              <CardTitle className="text-xl">
                Paso 1: Tipo de espacio y resumen
              </CardTitle>
              <CardDescription>
                Selecciona la categoría que mejor describa tu inmueble y define
                un título atractivo.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-2">
                <Label htmlFor="wizard-category" className="font-semibold">
                  Categoría del espacio *
                </Label>
                <select
                  id="wizard-category"
                  value={categoryCode}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  required
                  {...fieldFeedback("category")}
                  onChange={(e) => {
                    setCategoryCode(e.target.value);
                    setDynamicAttributes({});
                    setMaxVisitedStep(1);
                    setFieldErrors((previous) =>
                      Object.fromEntries(
                        Object.entries(previous).filter(
                          ([field]) =>
                            field !== "category" &&
                            !field.startsWith("attribute:"),
                        ),
                      ),
                    );
                    setStepError(null);
                  }}
                >
                  <option value="">-- Selecciona una categoría --</option>
                  {snapshot.categories.map((cat: Category) => (
                    <option key={cat.code} value={cat.code}>
                      {cat.name}
                    </option>
                  ))}
                </select>
                {fieldErrors.category && (
                  <p
                    id="wizard-category-error"
                    className="text-sm text-red-800"
                  >
                    {fieldErrors.category}
                  </p>
                )}
                <p className="text-xs text-muted-foreground">
                  Las categorías se obtienen en tiempo real desde el catálogo
                  oficial.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="wizard-title" className="font-semibold">
                  Título de la publicación *
                </Label>
                <Input
                  id="wizard-title"
                  value={title}
                  onChange={(e) => {
                    setTitle(e.target.value);
                    clearFieldError("title");
                  }}
                  {...fieldFeedback("title")}
                  placeholder="Ej. Oficina privada ejecutiva en Las Condes"
                  required
                />
                {fieldErrors.title && (
                  <p id="wizard-title-error" className="text-sm text-red-800">
                    {fieldErrors.title}
                  </p>
                )}
                <p className="text-xs text-muted-foreground flex justify-between">
                  <span>Sé específico e incluye detalles atractivos.</span>
                  <span>{Array.from(title).length}/70</span>
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="wizard-description" className="font-semibold">
                  Resumen o descripción (opcional)
                </Label>
                <textarea
                  id="wizard-description"
                  rows={4}
                  value={description}
                  onChange={(e) => {
                    setDescription(e.target.value);
                    clearFieldError("description");
                  }}
                  {...fieldFeedback("description")}
                  placeholder="Describe las comodidades, accesibilidad, iluminación y entorno del espacio..."
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                />
                {fieldErrors.description && (
                  <p
                    id="wizard-description-error"
                    className="text-sm text-red-800"
                  >
                    {fieldErrors.description}
                  </p>
                )}
                <p
                  id="wizard-description-count"
                  className="text-xs text-muted-foreground text-right"
                >
                  {Array.from(description.trim()).length}/500 caracteres
                </p>
              </div>
            </CardContent>
          </>
        )}

        {/* PASOS 2 y 3: ubicación y atributos */}
        {(currentStep === 2 || currentStep === 3) && (
          <>
            <CardHeader>
              <CardTitle className="text-xl">
                {currentStep === 2 ? "Paso 2: Ubicación" : "Paso 3: Atributos"}
              </CardTitle>
              <CardDescription>
                {currentStep === 2
                  ? "Indica la dirección física y coloca el pin del espacio."
                  : `Completa las dimensiones y características de ${selectedCatObj?.name || "la categoría"}.`}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {currentStep === 2 && (
                <div className="space-y-4">
                  <CommercialLocationFields
                    idPrefix="wizard"
                    location={pin}
                    onLocationChange={(next) => {
                      onLocationChange(next);
                      if (next.address !== address) clearFieldError("address");
                    }}
                    onSelectPin={selectPin}
                    resolution={pinResolution.resolution}
                    onRestoreAddress={
                      pinResolution.restoredAddressAvailable
                        ? pinResolution.restoreAddress
                        : undefined
                    }
                    addressErrorId={
                      fieldErrors.address ? "wizard-address-error" : undefined
                    }
                    addressInvalid={Boolean(fieldErrors.address)}
                    disabled={isSubmitting}
                  />
                  {fieldErrors.location && (
                    <p
                      id="wizard-location-error"
                      role="alert"
                      className="text-sm text-destructive"
                    >
                      {fieldErrors.location}
                    </p>
                  )}
                  {fieldErrors.address && (
                    <p
                      id="wizard-address-error"
                      role="alert"
                      className="text-sm text-destructive"
                    >
                      {fieldErrors.address}
                    </p>
                  )}
                </div>
              )}
              {currentStep === 3 && (
                <div className="space-y-6">
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="wizard-area" className="font-semibold">
                        Superficie (m²) *
                      </Label>
                      <Input
                        id="wizard-area"
                        type="number"
                        min={0.01}
                        max={99999999.99}
                        step={0.01}
                        value={areaM2}
                        onChange={(e) => {
                          setAreaM2(Number(e.target.value));
                          clearFieldError("area");
                        }}
                        {...fieldFeedback("area")}
                        required
                      />
                      {fieldErrors.area && (
                        <p
                          id="wizard-area-error"
                          className="text-sm text-red-800"
                        >
                          {fieldErrors.area}
                        </p>
                      )}
                    </div>

                    <div className="space-y-2">
                      <Label
                        htmlFor="wizard-capacity"
                        className="font-semibold"
                      >
                        Capacidad (personas) *
                      </Label>
                      <Input
                        id="wizard-capacity"
                        type="number"
                        min={1}
                        max={2147483647}
                        step={1}
                        value={capacity}
                        onChange={(e) => {
                          setCapacity(Number(e.target.value));
                          clearFieldError("capacity");
                        }}
                        {...fieldFeedback("capacity")}
                        required
                      />
                      {fieldErrors.capacity && (
                        <p
                          id="wizard-capacity-error"
                          className="text-sm text-red-800"
                        >
                          {fieldErrors.capacity}
                        </p>
                      )}
                    </div>
                  </div>

                  {/* Atributos dinámicos del CategoryProfileSchema */}
                  <div className="pt-4 border-t border-border space-y-4">
                    <div className="flex items-center justify-between">
                      <h2 className="text-base font-semibold text-foreground">
                        Atributos del perfil (
                        {selectedCatObj?.name || categoryCode})
                      </h2>
                      {currentSchema && (
                        <Badge variant="outline" className="text-xs font-mono">
                          v{currentSchema.schema_version}
                        </Badge>
                      )}
                    </div>

                    {loadingSchema && (
                      <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
                        <Loader2
                          className="size-4 animate-spin"
                          aria-hidden="true"
                        />
                        <span>
                          Cargando esquema de atributos desde el backend...
                        </span>
                      </div>
                    )}

                    {schemaFailed && (
                      <div
                        role="alert"
                        className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
                      >
                        <span>
                          No se pudo cargar el perfil de esta categoría. Vuelve
                          a intentarlo antes de completar sus atributos.
                        </span>
                        <Button
                          type="button"
                          variant="outline"
                          onClick={() => {
                            setSchemaLoadError(null);
                            setSchemaRetryToken((current) => current + 1);
                          }}
                        >
                          Reintentar perfil
                        </Button>
                      </div>
                    )}

                    {!loadingSchema &&
                    currentSchema?.attributes &&
                    currentSchema.attributes.length > 0 ? (
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 bg-muted/20 p-4 rounded-xl border">
                        {currentSchema.attributes.map((def) => {
                          const label = formatAttributeLabel(
                            def.code,
                            currentSchema,
                          );
                          const attrVal = dynamicAttributes[def.code];
                          const fieldId = `attr-${def.code}`;

                          if (def.type === "boolean") {
                            return (
                              <div
                                key={def.code}
                                className="flex items-center justify-between p-3 rounded-lg bg-background border"
                              >
                                <Label
                                  htmlFor={fieldId}
                                  className="text-sm cursor-pointer"
                                >
                                  {label} {def.required && "*"}
                                </Label>
                                <input
                                  id={fieldId}
                                  type="checkbox"
                                  checked={Boolean(attrVal)}
                                  onChange={(e) => {
                                    handleAttributeChange(
                                      def.code,
                                      e.target.checked,
                                    );
                                    clearFieldError(`attribute:${def.code}`);
                                  }}
                                  aria-invalid={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? true
                                      : undefined
                                  }
                                  aria-describedby={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? `wizard-attribute-${def.code}-error`
                                      : undefined
                                  }
                                  className="size-4 rounded text-primary focus:ring-primary"
                                />
                              </div>
                            );
                          }

                          if (def.type === "integer" || def.type === "number") {
                            return (
                              <div key={def.code} className="space-y-1.5">
                                <Label
                                  htmlFor={fieldId}
                                  className="text-xs font-semibold"
                                >
                                  {label} {def.unit ? `(${def.unit})` : ""}{" "}
                                  {def.required && "*"}
                                </Label>
                                <Input
                                  id={fieldId}
                                  type="number"
                                  min={def.minimum}
                                  max={def.maximum}
                                  step={
                                    def.step ??
                                    (def.type === "integer" ? 1 : 0.1)
                                  }
                                  value={
                                    attrVal !== undefined ? String(attrVal) : ""
                                  }
                                  onChange={(e) => {
                                    handleAttributeChange(
                                      def.code,
                                      e.target.value === ""
                                        ? undefined
                                        : Number(e.target.value),
                                    );
                                    clearFieldError(`attribute:${def.code}`);
                                  }}
                                  aria-invalid={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? true
                                      : undefined
                                  }
                                  aria-describedby={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? `wizard-attribute-${def.code}-error`
                                      : undefined
                                  }
                                  placeholder={
                                    def.description || `Ingrese ${label}`
                                  }
                                />
                                {fieldErrors[`attribute:${def.code}`] && (
                                  <p
                                    id={`wizard-attribute-${def.code}-error`}
                                    className="text-sm text-red-800"
                                  >
                                    {fieldErrors[`attribute:${def.code}`]}
                                  </p>
                                )}
                              </div>
                            );
                          }

                          if (def.type === "enum_list" && def.options) {
                            const options = def.options;
                            const selected = Array.isArray(attrVal)
                              ? attrVal.filter(
                                  (value): value is string =>
                                    typeof value === "string" &&
                                    options.includes(value),
                                )
                              : [];
                            return (
                              <fieldset
                                key={def.code}
                                tabIndex={-1}
                                aria-invalid={
                                  fieldErrors[`attribute:${def.code}`]
                                    ? true
                                    : undefined
                                }
                                aria-describedby={
                                  fieldErrors[`attribute:${def.code}`]
                                    ? `wizard-attribute-${def.code}-error`
                                    : undefined
                                }
                                className="space-y-2 rounded-lg border bg-background p-3"
                              >
                                <legend className="px-1 text-xs font-semibold">
                                  {label} {def.required && "*"}
                                </legend>
                                <div className="space-y-2">
                                  {def.options.map((option, optionIndex) => {
                                    const optionId = `${fieldId}-option-${optionIndex}`;
                                    return (
                                      <div
                                        key={option}
                                        className="flex items-center gap-2"
                                      >
                                        <input
                                          id={optionId}
                                          type="checkbox"
                                          checked={selected.includes(option)}
                                          aria-invalid={
                                            fieldErrors[`attribute:${def.code}`]
                                              ? true
                                              : undefined
                                          }
                                          aria-describedby={
                                            fieldErrors[`attribute:${def.code}`]
                                              ? `wizard-attribute-${def.code}-error`
                                              : undefined
                                          }
                                          onChange={(event) => {
                                            const next = event.target.checked
                                              ? [...selected, option]
                                              : selected.filter(
                                                  (value) => value !== option,
                                                );
                                            const selectedSet = new Set(next);
                                            handleAttributeChange(
                                              def.code,
                                              options.filter((value) =>
                                                selectedSet.has(value),
                                              ),
                                            );
                                            clearFieldError(
                                              `attribute:${def.code}`,
                                            );
                                          }}
                                          className="size-4 rounded text-primary focus:ring-primary"
                                        />
                                        <Label
                                          htmlFor={optionId}
                                          className="cursor-pointer text-sm"
                                        >
                                          {formatAttributeValue(option)}
                                        </Label>
                                      </div>
                                    );
                                  })}
                                </div>
                                {fieldErrors[`attribute:${def.code}`] && (
                                  <p
                                    id={`wizard-attribute-${def.code}-error`}
                                    className="text-sm text-red-800"
                                  >
                                    {fieldErrors[`attribute:${def.code}`]}
                                  </p>
                                )}
                              </fieldset>
                            );
                          }

                          if (def.type === "enum" && def.options) {
                            return (
                              <div key={def.code} className="space-y-1.5">
                                <Label
                                  htmlFor={fieldId}
                                  className="text-xs font-semibold"
                                >
                                  {label} {def.required && "*"}
                                </Label>
                                <select
                                  id={fieldId}
                                  aria-required={def.required || undefined}
                                  value={
                                    typeof attrVal === "string" ? attrVal : ""
                                  }
                                  onChange={(e) => {
                                    handleAttributeChange(
                                      def.code,
                                      e.target.value,
                                    );
                                    clearFieldError(`attribute:${def.code}`);
                                  }}
                                  aria-invalid={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? true
                                      : undefined
                                  }
                                  aria-describedby={
                                    fieldErrors[`attribute:${def.code}`]
                                      ? `wizard-attribute-${def.code}-error`
                                      : undefined
                                  }
                                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                                >
                                  <option value="">-- Selecciona --</option>
                                  {def.options.map((opt) => (
                                    <option key={opt} value={opt}>
                                      {formatAttributeValue(opt)}
                                    </option>
                                  ))}
                                </select>
                                {fieldErrors[`attribute:${def.code}`] && (
                                  <p
                                    id={`wizard-attribute-${def.code}-error`}
                                    className="text-sm text-red-800"
                                  >
                                    {fieldErrors[`attribute:${def.code}`]}
                                  </p>
                                )}
                              </div>
                            );
                          }

                          return null;
                        })}
                      </div>
                    ) : (
                      !loadingSchema && (
                        <p className="text-xs text-muted-foreground italic">
                          No hay atributos dinámicos adicionales requeridos para
                          esta categoría.
                        </p>
                      )
                    )}
                  </div>
                </div>
              )}
            </CardContent>
          </>
        )}

        {/* PASO 4: Tarifas iniciales y reglas */}
        {currentStep === 4 && (
          <>
            <CardHeader>
              <CardTitle className="text-xl">
                Paso 4: Tarifas iniciales y condiciones
              </CardTitle>
              <CardDescription>
                Establece el precio base en pesos chilenos (CLP) y las políticas
                de cancelación.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="wizard-rate-unit" className="font-semibold">
                    Unidad de cobro *
                  </Label>
                  <select
                    id="wizard-rate-unit"
                    value={rateUnit}
                    onChange={(e) => setRateUnit(e.target.value as RateUnit)}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  >
                    <option value="hora">Por Hora</option>
                    <option value="dia">Por Día</option>
                    <option value="mes">Por Mes</option>
                  </select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="wizard-price" className="font-semibold">
                    Precio base (CLP) *
                  </Label>
                  <Input
                    id="wizard-price"
                    type="number"
                    min={1000}
                    step={1}
                    value={basePriceCLP}
                    onChange={(e) => {
                      setBasePriceCLP(Number(e.target.value));
                      clearFieldError("price");
                    }}
                    {...fieldFeedback("price")}
                    required
                  />
                  {fieldErrors.price && (
                    <p
                      id="wizard-price-error"
                      className="text-sm text-destructive"
                    >
                      {fieldErrors.price}
                    </p>
                  )}
                  <p className="text-xs text-muted-foreground">
                    Monto en pesos chilenos por cada {rateUnit}.
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="wizard-deposit">
                    Garantía (no disponible)
                  </Label>
                  <Input
                    id="wizard-deposit"
                    type="number"
                    value={0}
                    disabled
                    aria-describedby="wizard-deposit-help"
                  />
                  <p
                    id="wizard-deposit-help"
                    className="text-xs text-muted-foreground"
                  >
                    La garantía no está disponible en este flujo.
                  </p>
                </div>

                <div className="space-y-2">
                  <Label
                    htmlFor="wizard-cancellation"
                    className="font-semibold"
                  >
                    Política de cancelación
                  </Label>
                  <select
                    id="wizard-cancellation"
                    value={cancellationPolicy}
                    onChange={(e) => {
                      const version = cancellationPolicyVersion(e.target.value);
                      if (version) setCancellationPolicy(version);
                    }}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  >
                    {cancellationPolicies.map((policy) => (
                      <option key={policy.version} value={policy.version}>
                        {policy.label}
                      </option>
                    ))}
                  </select>
                  <p
                    className="text-xs text-muted-foreground"
                    aria-live="polite"
                  >
                    {
                      cancellationPolicies.find(
                        (policy) => policy.version === cancellationPolicy,
                      )?.summary
                    }
                  </p>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="wizard-usage-rules" className="font-semibold">
                  Reglas y normas de uso
                </Label>
                <textarea
                  id="wizard-usage-rules"
                  rows={3}
                  value={usageRules}
                  onChange={(e) => {
                    setUsageRules(e.target.value);
                    clearFieldError("usageRules");
                  }}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  placeholder="Indicaciones sobre ruido, acceso de mascotas, horarios de cierre..."
                  {...fieldFeedback("usageRules")}
                />
                {fieldErrors.usageRules && (
                  <p
                    id="wizard-usage-rules-error"
                    className="text-sm text-destructive"
                  >
                    {fieldErrors.usageRules}
                  </p>
                )}
                <p
                  id="wizard-usage-rules-count"
                  className="text-xs text-muted-foreground text-right"
                  aria-live="polite"
                >
                  {Array.from(usageRules.trim()).length}/250 caracteres
                </p>
              </div>
            </CardContent>
          </>
        )}

        {/* PASO 5: Disponibilidad semanal */}
        {currentStep === 5 && (
          <>
            <CardHeader>
              <CardTitle className="text-xl">
                Paso 5: Disponibilidad semanal y zona horaria
              </CardTitle>
              <CardDescription>
                El horario semanal describe el funcionamiento habitual del
                espacio; no confirma disponibilidad ni garantiza una reserva.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {rateUnit === "hora" && (
                <div className="flex items-center justify-between p-4 rounded-xl bg-muted/40 border">
                  <div>
                    <Label
                      htmlFor="schedule-enabled"
                      className="text-sm font-semibold text-foreground cursor-pointer"
                    >
                      Habilitar horario semanal
                    </Label>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Actívalo para registrar los tramos habituales del espacio.
                      No confirma disponibilidad para fechas.
                    </p>
                  </div>
                  <input
                    type="checkbox"
                    id="schedule-enabled"
                    aria-label="Habilitar calendario de reservas semanal"
                    checked={scheduleEnabled}
                    onChange={(e) => setScheduleEnabled(e.target.checked)}
                    className="size-5 rounded text-primary focus:ring-primary"
                  />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="wizard-timezone" className="font-semibold">
                  Zona horaria de Chile *
                </Label>
                <select
                  id="wizard-timezone"
                  value={timeZone}
                  onChange={(e) => {
                    setTimeZone(e.target.value);
                    clearFieldError("timezone");
                  }}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  required
                  {...fieldFeedback("timezone")}
                >
                  {CHILE_TIME_ZONE_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
                {fieldErrors.timezone && (
                  <p
                    id="wizard-timezone-error"
                    className="text-sm text-destructive"
                  >
                    {fieldErrors.timezone}
                  </p>
                )}
                <p className="text-xs text-muted-foreground">
                  Elige la zona que corresponde a la ubicación del espacio. Se
                  guardará respetando los cambios horarios oficiales.
                </p>
              </div>

              {scheduleEnabled && rateUnit === "hora" && (
                <div className="space-y-4 pt-2">
                  <h2
                    id="wizard-schedule-label"
                    className="text-sm font-semibold text-foreground"
                  >
                    Tramos horarios por día de la semana
                  </h2>
                  <div
                    id="wizard-schedule-group"
                    role="group"
                    tabIndex={-1}
                    aria-labelledby="wizard-schedule-label"
                    aria-describedby={
                      fieldErrors.schedule ? "wizard-schedule-error" : undefined
                    }
                    className="space-y-4 outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  >
                    {fieldErrors.schedule && (
                      <p
                        id="wizard-schedule-error"
                        className="text-sm text-red-800"
                      >
                        {fieldErrors.schedule}
                      </p>
                    )}
                    <div
                      aria-invalid={fieldErrors.schedule ? true : undefined}
                      aria-describedby={
                        fieldErrors.schedule
                          ? "wizard-schedule-error"
                          : undefined
                      }
                    >
                      <WeeklyScheduleSlots
                        slots={slots}
                        timeZone={timeZone}
                        disabled={isSubmitting || !scheduleEnabled}
                        invalid={Boolean(fieldErrors.schedule)}
                        errorId="wizard-schedule-error"
                        onChange={(nextSlots) => {
                          setSlots(nextSlots);
                          clearScheduleError();
                        }}
                      />
                    </div>
                  </div>
                </div>
              )}
              <div className="space-y-3 pt-4 border-t border-border">
                <div>
                  <h2 className="text-base font-semibold">
                    Vista previa del calendario
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Selecciona fechas para agregar excepciones o bloqueos y
                    revisa cómo quedará la disponibilidad.
                  </p>
                </div>
                <HostAvailabilityCalendar
                  rateUnit={rateUnit}
                  basePriceCLP={basePriceCLP}
                  timeZone={timeZone}
                  onTimeZoneChange={(tz) => setTimeZone(tz)}
                  initialSlots={slots}
                  onScheduleChange={(nextSlots) => {
                    setSlots(nextSlots);
                    clearScheduleError();
                  }}
                  disabled={isSubmitting}
                />
              </div>
              {rateUnit === "mes" && (
                <div
                  role="note"
                  className="rounded-xl border border-sky-200 bg-sky-50/80 p-4 text-xs text-sky-950 space-y-1"
                >
                  <p className="font-semibold text-sm text-sky-900">
                    Disponibilidad para arriendo mensual
                  </p>
                  <p>
                    Para publicaciones por mes, la fecha de inicio disponible
                    (&quot;Disponible desde&quot;) y la estancia mínima opcional
                    en meses se configuran en el editor de disponibilidad de tu
                    espacio una vez creado el borrador.
                  </p>
                </div>
              )}
              {rateUnit !== "hora" && (
                <p className="text-sm text-muted-foreground" role="note">
                  Horario semanal no aplicable a tarifas por día o mes; se
                  guardará la zona horaria.
                </p>
              )}
            </CardContent>
          </>
        )}

        {/* PASO 6: Vista previa y confirmación */}
        {currentStep === 6 && (
          <>
            <CardHeader>
              <CardTitle className="text-xl">
                Paso 6: Resumen y confirmación de alta
              </CardTitle>
              <CardDescription>
                Revisa tus datos antes de crear el borrador privado. Después
                podrás agregar las fotografías para publicarlo.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="flex flex-wrap gap-2" aria-label="Editar sección">
                {[
                  [1, "Tipo y resumen"],
                  [2, "Ubicación"],
                  [3, "Atributos"],
                  [4, "Tarifas y reglas"],
                  [5, "Disponibilidad"],
                ].map(([step, label]) => (
                  <Button
                    key={step}
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setStepError(null);
                      setReturnToConfirmation(true);
                      setCurrentStep(Number(step));
                    }}
                  >
                    Editar {label}
                  </Button>
                ))}
              </div>
              <div className="rounded-xl border bg-muted/20 p-5 space-y-4 text-sm">
                <div className="flex items-start justify-between gap-4 border-b pb-3">
                  <div>
                    <Badge variant="outline" className="mb-1">
                      {selectedCatObj?.name || categoryCode}
                    </Badge>
                    <h2 className="text-lg font-bold text-foreground">
                      {title}
                    </h2>
                    <p className="text-xs text-muted-foreground mt-1">
                      {description}
                    </p>
                  </div>
                  <div className="text-right shrink-0">
                    <div className="text-xs text-muted-foreground">Tarifa</div>
                    <div className="text-lg font-bold text-primary">
                      ${new Intl.NumberFormat("es-CL").format(basePriceCLP)} CLP
                      <span className="text-xs font-normal text-muted-foreground">
                        /{rateUnit}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
                  <div>
                    <span className="text-muted-foreground font-semibold">
                      Ubicación:
                    </span>
                    <p className="text-foreground mt-0.5">
                      {address},{" "}
                      {CHILE_COMUNAS.find(({ code }) => code === comunaCode)
                        ?.name || "Chile"}
                      {regionCode &&
                        `, ${CHILE_REGIONS.find(({ code }) => code === regionCode)?.name ?? ""}`}
                    </p>
                  </div>
                  <div>
                    <span className="text-muted-foreground font-semibold">
                      Capacidad y Área:
                    </span>
                    <p className="text-foreground mt-0.5">
                      {areaM2} m² · Hasta {capacity} personas
                    </p>
                  </div>
                  <div>
                    <span className="text-muted-foreground font-semibold">
                      Política de cancelación:
                    </span>
                    <p className="text-foreground mt-0.5">
                      {
                        cancellationPolicies.find(
                          (policy) => policy.version === cancellationPolicy,
                        )?.label
                      }
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {
                        cancellationPolicies.find(
                          (policy) => policy.version === cancellationPolicy,
                        )?.summary
                      }
                    </p>
                  </div>
                  <div>
                    <span className="text-muted-foreground font-semibold">
                      Reglas de uso:
                    </span>
                    <p className="text-foreground mt-0.5">{usageRules}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground font-semibold">
                      Disponibilidad:
                    </span>
                    <p className="text-foreground mt-0.5">
                      {rateUnit === "mes"
                        ? "Modalidad mensual: disponible desde y estancia mínima se configuran en el editor tras el alta"
                        : rateUnit === "dia"
                          ? "Modalidad diaria: jornada completa sin horarios"
                          : scheduleEnabled
                            ? `${slots.length} tramos semanales`
                            : "Sin horario activo"}{" "}
                      ({timeZone})
                    </p>
                  </div>
                </div>

                {Object.keys(dynamicAttributes).length > 0 && (
                  <div className="border-t pt-3 text-xs">
                    <span className="text-muted-foreground font-semibold">
                      Atributos configurados:
                    </span>
                    <ul className="mt-1 grid grid-cols-2 gap-2 text-foreground">
                      {Object.entries(
                        cleanAttributesForPayload(dynamicAttributes),
                      ).map(([k, v]) => {
                        const definition = currentSchema?.attributes.find(
                          (attribute) => attribute.code === k,
                        );
                        return (
                          <li key={k} className="truncate">
                            <span className="font-medium">
                              {formatAttributeLabel(
                                k,
                                currentSchema ?? undefined,
                              )}
                              :
                            </span>{" "}
                            {formatAttributeValue(v, definition)}
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 text-xs text-blue-900 flex items-start gap-2.5">
                <Info
                  className="size-4 shrink-0 text-blue-600 mt-0.5"
                  aria-hidden="true"
                />
                <div>
                  Al confirmar, se generará un borrador privado. Serás
                  redirigido al editor donde podrás cargar fotografías y
                  gestionar la publicación.
                </div>
              </div>
            </CardContent>
          </>
        )}

        {/* Navegación y botones de pie de tarjeta */}
        <CardFooter className="flex items-center justify-between gap-4 border-t border-border pt-4">
          {currentStep > 1 ? (
            <Button
              type="button"
              variant="outline"
              onClick={handleBack}
              disabled={isSubmitting}
              className="gap-1.5"
            >
              <ArrowLeft className="size-4" aria-hidden="true" />
              Atrás
            </Button>
          ) : (
            <div />
          )}

          {currentStep < 6 ? (
            <Button
              type="button"
              onClick={handleNext}
              disabled={
                isSubmitting ||
                (currentStep === 2 && !isResolved) ||
                (currentStep === 3 && Boolean(categoryCode) && !currentSchema)
              }
              className="gap-1.5 font-semibold"
            >
              Siguiente
              <ArrowRight className="size-4" aria-hidden="true" />
            </Button>
          ) : (
            <Button
              type="button"
              onClick={handleCreateDraft}
              disabled={isSubmitting || Boolean(partialDraft)}
              className="gap-2 font-bold bg-primary text-primary-foreground shadow-sm"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                  Creando borrador...
                </>
              ) : (
                <>
                  <Check className="size-4" aria-hidden="true" />
                  Crear espacio y continuar a edición
                </>
              )}
            </Button>
          )}
        </CardFooter>
      </Card>
    </div>
  );
}
