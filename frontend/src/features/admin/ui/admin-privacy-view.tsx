"use client";

import { useCallback, useEffect, useState } from "react";
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Loader2,
  Lock,
  Plus,
  RefreshCw,
  Scale,
  ShieldCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import {
  isForbiddenError,
  isRecentAuthRequired,
  isValidAdminReason,
  isValidUUID,
} from "../domain/admin-rules";
import { AdminFeedbackBanners } from "./admin-feedback-banners";
import { useAdmin } from "./use-admin";
import type {
  AdminPrivacyNotificationSummary,
  AdminPrivacyPurpose,
  AdminPrivacyPurposeState,
  AdminPrivacyTreatmentScope,
  CreateGlobalAdminPrivacyIncidentInput,
  CreateGlobalAdminPrivacyLegalHoldInput,
  GlobalAdminKycCase,
  GlobalAdminPrivacyChannel,
  GlobalAdminPrivacyExecutionAction,
  GlobalAdminPrivacyExecutionInput,
  GlobalAdminPrivacyIncident,
  GlobalAdminPrivacyIncidentAction,
  GlobalAdminPrivacyIncidentDetail,
  GlobalAdminPrivacyIncidentPage,
  GlobalAdminPrivacyIncidentQuery,
  GlobalAdminPrivacyIncidentSeverity,
  GlobalAdminPrivacyLegalHold,
  GlobalAdminPrivacyLegalHoldPage,
  GlobalAdminPrivacyLegalHoldQuery,
  GlobalAdminPrivacyLegalHoldScope,
  GlobalAdminPrivacyRepresentativeRelation,
  GlobalAdminPrivacyRequest,
  GlobalAdminPrivacyRequestDetail,
  GlobalAdminPrivacyRequestStatus,
  GlobalAdminPrivacyRequestType,
  GlobalAdminPrivacyWorkflowAction,
  ReleaseGlobalAdminPrivacyLegalHoldInput,
  TransitionGlobalAdminPrivacyIncidentInput,
} from "../domain/types";

const MIN_RESPONSE_LENGTH = 10;
const MAX_RESPONSE_LENGTH = 4000;
const MIN_EXECUTION_SUMMARY_LENGTH = 10;
const MAX_EXECUTION_SUMMARY_LENGTH = 1000;

const WORKFLOW_EVENT_LABELS: Record<string, string> = {
  recepcion: "Recepción",
  asignacion: "Asignación",
  identidad_verificada: "Identidad verificada",
  en_revision: "En revisión",
  aclaracion_solicitada: "Aclaración solicitada",
  nota_interna: "Nota interna",
  prorroga: "Prórroga",
  ejecucion_derecho: "Ejecución de derecho",
  resolucion: "Resolución",
  despacho_registrado: "Despacho registrado",
};

const TREATMENT_SCOPE_OPTIONS: ReadonlyArray<{
  value: AdminPrivacyTreatmentScope;
  label: string;
}> = [
  {
    value: "comunicaciones_comerciales",
    label: "Comunicaciones comerciales",
  },
  {
    value: "analitica_y_perfilado",
    label: "Analítica y perfilado",
  },
  {
    value: "publicaciones_y_visibilidad",
    label: "Publicaciones y visibilidad",
  },
  {
    value: "exportaciones_y_terceros",
    label: "Exportaciones y terceros",
  },
  {
    value: "tratamiento_general",
    label: "Tratamiento general",
  },
];

const PURPOSE_OPTIONS: ReadonlyArray<{
  value: AdminPrivacyPurpose;
  label: string;
}> = [
  {
    value: "comunicaciones_comerciales",
    label: "Comunicaciones comerciales",
  },
  {
    value: "analitica_producto",
    label: "Analítica de producto",
  },
  {
    value: "personalizacion_recomendaciones",
    label: "Personalización y recomendaciones",
  },
  {
    value: "comparticion_encargados_opcionales",
    label: "Compartición con encargados opcionales",
  },
];

const EXECUTION_ACTION_LABELS: Record<
  Exclude<GlobalAdminPrivacyExecutionAction, "lift_block">,
  string
> = {
  rectify_profile: "Rectificar perfil de cuenta",
  apply_opposition: "Registrar oposición por finalidad",
  apply_block: "Aplicar bloqueo temporal de tratamiento",
  issue_portability: "Emitir paquete de portabilidad",
};

function getAvailableExecutionActions(
  rightType: GlobalAdminPrivacyRequestType,
): Array<Exclude<GlobalAdminPrivacyExecutionAction, "lift_block">> {
  switch (rightType) {
    case "rectificacion":
      return ["rectify_profile", "apply_block"];
    case "oposicion":
      return ["apply_opposition", "apply_block"];
    case "bloqueo":
      return ["apply_block"];
    case "portabilidad":
      return ["issue_portability"];
    case "supresion":
      return ["apply_block"];
    default:
      return [];
  }
}

interface ExecutionDraft {
  action?: Exclude<GlobalAdminPrivacyExecutionAction, "lift_block">;
  summary: string;
  displayName: string;
  phone: string;
  clearPhone: boolean;
  purpose: AdminPrivacyPurpose;
  purposeState: Extract<AdminPrivacyPurposeState, "opuesto" | "retirado">;
  scope: AdminPrivacyTreatmentScope;
  targetReference: string;
}

const DEFAULT_EXECUTION_DRAFT: ExecutionDraft = {
  summary: "",
  displayName: "",
  phone: "",
  clearPhone: false,
  purpose: "comunicaciones_comerciales",
  purposeState: "opuesto",
  scope: "tratamiento_general",
  targetReference: "",
};

function getScopeLabel(scope: string): string {
  return (
    TREATMENT_SCOPE_OPTIONS.find((item) => item.value === scope)?.label ?? scope
  );
}

function getPurposeLabel(purpose: string): string {
  return PURPOSE_OPTIONS.find((item) => item.value === purpose)?.label ?? purpose;
}

function formatRepresentativeRelation(
  relation?: GlobalAdminPrivacyRepresentativeRelation,
): string {
  if (relation === "mandatario") return "Mandatario";
  if (relation === "representante_legal") return "Representante legal";
  if (relation === "tutor_nna") return "Tutor NNA";
  return "";
}

function NotificationSummaryBox({
  summary,
  retrying,
  error,
  showMfaHint,
  onRetry,
}: {
  summary: AdminPrivacyNotificationSummary;
  retrying: boolean;
  error: string | null;
  showMfaHint: boolean;
  onRetry: () => void;
}) {
  return (
    <div className="space-y-2 rounded border border-border bg-background p-2.5 text-xs">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium text-foreground">
          Estado de notificaciones
        </span>
        {summary.terminal > 0 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={retrying}
            onClick={onRetry}
          >
            {retrying ? "Reintentando…" : "Reintentar notificación"}
          </Button>
        )}
      </div>
      <div className="flex flex-wrap gap-3 text-muted-foreground">
        <span>
          Pendientes:{" "}
          <strong className="text-foreground">{summary.pending}</strong>
        </span>
        <span>
          Entregadas:{" "}
          <strong className="text-foreground">{summary.delivered}</strong>
        </span>
        <span>
          Fallidas/Terminales:{" "}
          <strong className="text-foreground">{summary.terminal}</strong>
        </span>
      </div>
      {summary.last_error_code && (
        <p className="text-destructive">
          Último error:{" "}
          <span className="font-mono">{summary.last_error_code}</span>
        </p>
      )}
      {summary.last_delivered_at && (
        <p className="text-muted-foreground">
          Última entrega:{" "}
          {new Date(summary.last_delivered_at).toLocaleString("es-CL")}
        </p>
      )}
      {summary.terminal > 0 && showMfaHint && (
        <p className="text-muted-foreground">
          Se solicitará MFA reciente antes de reintentar la notificación.
        </p>
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

function isRecentAuthActive(
  mfaStepUpRequired: boolean,
  expiresAt?: string | null,
): boolean {
  return Boolean(
    !mfaStepUpRequired && expiresAt && Date.parse(expiresAt) > Date.now(),
  );
}

const INCIDENT_SEVERITY_OPTIONS: ReadonlyArray<{
  value: GlobalAdminPrivacyIncidentSeverity;
  label: string;
}> = [
  { value: "baja", label: "Baja" },
  { value: "media", label: "Media" },
  { value: "alta", label: "Alta" },
  { value: "critica", label: "Crítica" },
];

const INCIDENT_EVENT_LABELS: Record<string, string> = {
  apertura: "Apertura",
  contencion: "Contención",
  evaluacion: "Evaluación",
  notificacion_agencia: "Notificación a Agencia",
  notificacion_titulares: "Notificación a titulares",
  nota: "Nota",
  cierre: "Cierre",
};

const INCIDENT_ACTION_OPTIONS: ReadonlyArray<{
  value: GlobalAdminPrivacyIncidentAction;
  label: string;
}> = [
  { value: "contain", label: "Contener incidente (contain)" },
  { value: "evaluate", label: "Evaluar impacto (evaluate)" },
  { value: "notify_agency", label: "Notificar a la Agencia (notify_agency)" },
  { value: "notify_subjects", label: "Notificar a titulares (notify_subjects)" },
  { value: "add_note", label: "Agregar nota operativa (add_note)" },
  { value: "close", label: "Cerrar incidente (close)" },
];

const LEGAL_HOLD_SCOPE_OPTIONS: ReadonlyArray<{
  value: GlobalAdminPrivacyLegalHoldScope;
  label: string;
}> = [
  { value: "expediente_privacidad", label: "Expediente de privacidad" },
  { value: "cuenta_completa", label: "Cuenta completa" },
  { value: "evidencia_kyc", label: "Evidencia KYC" },
  { value: "contratos_y_pagos", label: "Contratos y pagos" },
  { value: "incidente_seguridad", label: "Incidente de seguridad" },
];

function getLegalHoldScopeLabel(scope: string): string {
  return (
    LEGAL_HOLD_SCOPE_OPTIONS.find((item) => item.value === scope)?.label ??
    scope
  );
}

function getSeverityBadgeVariant(
  severity: GlobalAdminPrivacyIncidentSeverity,
): "destructive" | "secondary" | "outline" {
  if (severity === "critica" || severity === "alta") return "destructive";
  if (severity === "media") return "secondary";
  return "outline";
}

function parseCommaList(raw: string): string[] {
  return raw
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}

interface IncidentTransitionDraft {
  action: GlobalAdminPrivacyIncidentAction;
  detail: string;
  containmentMeasures: string;
  agencyReference: string;
  requiresAgencyNotice: boolean;
  requiresSubjectNotice: boolean;
}

interface AdminPrivacyPhase7SectionProps {
  mfaStepUpRequired: boolean;
  recentAuthExpiresAt?: string | null;
  showMfaHint: boolean;
  openMfaStepUp: () => void;
  listGlobalPrivacyIncidents: (
    query?: GlobalAdminPrivacyIncidentQuery,
  ) => Promise<GlobalAdminPrivacyIncidentPage>;
  getGlobalPrivacyIncident: (
    incidentId: string,
  ) => Promise<GlobalAdminPrivacyIncidentDetail>;
  createGlobalPrivacyIncident: (
    input: CreateGlobalAdminPrivacyIncidentInput,
  ) => Promise<GlobalAdminPrivacyIncidentDetail>;
  transitionGlobalPrivacyIncident: (
    incidentId: string,
    input: TransitionGlobalAdminPrivacyIncidentInput,
  ) => Promise<GlobalAdminPrivacyIncidentDetail>;
  listGlobalPrivacyLegalHolds: (
    query?: GlobalAdminPrivacyLegalHoldQuery,
  ) => Promise<GlobalAdminPrivacyLegalHoldPage>;
  createGlobalPrivacyLegalHold: (
    input: CreateGlobalAdminPrivacyLegalHoldInput,
  ) => Promise<GlobalAdminPrivacyLegalHold>;
  releaseGlobalPrivacyLegalHold: (
    holdId: string,
    input: ReleaseGlobalAdminPrivacyLegalHoldInput,
  ) => Promise<GlobalAdminPrivacyLegalHold>;
}

function AdminPrivacyPhase7Section({
  mfaStepUpRequired,
  recentAuthExpiresAt,
  showMfaHint,
  openMfaStepUp,
  listGlobalPrivacyIncidents,
  getGlobalPrivacyIncident,
  createGlobalPrivacyIncident,
  transitionGlobalPrivacyIncident,
  listGlobalPrivacyLegalHolds,
  createGlobalPrivacyLegalHold,
  releaseGlobalPrivacyLegalHold,
}: AdminPrivacyPhase7SectionProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [hasLoadedOnce, setHasLoadedOnce] = useState(false);
  const [loadingPhase7, setLoadingPhase7] = useState(false);

  // Incidents state
  const [incidents, setIncidents] = useState<GlobalAdminPrivacyIncident[]>([]);
  const [incidentsNextCursor, setIncidentsNextCursor] = useState<string | null>(
    null,
  );
  const [loadingMoreIncidents, setLoadingMoreIncidents] = useState(false);
  const [incidentsForbidden, setIncidentsForbidden] = useState(false);
  const [incidentsError, setIncidentsError] = useState<string | null>(null);
  const [expandedIncidentId, setExpandedIncidentId] = useState<string | null>(
    null,
  );
  const [incidentDetails, setIncidentDetails] = useState<
    Record<string, GlobalAdminPrivacyIncidentDetail>
  >({});
  const [loadingIncidentDetailId, setLoadingIncidentDetailId] = useState<
    string | null
  >(null);
  const [incidentDetailError, setIncidentDetailError] = useState<string | null>(
    null,
  );

  // Create incident form state
  const [createIncidentOpen, setCreateIncidentOpen] = useState(false);
  const [incTitle, setIncTitle] = useState("");
  const [incSeverity, setIncSeverity] =
    useState<GlobalAdminPrivacyIncidentSeverity>("media");
  const [incCategories, setIncCategories] = useState("");
  const [incSystems, setIncSystems] = useState("");
  const [incEstimatedSubjects, setIncEstimatedSubjects] = useState("0");
  const [incLeaderId, setIncLeaderId] = useState("");
  const [incSummary, setIncSummary] = useState("");
  const [incRequiresAgency, setIncRequiresAgency] = useState(false);
  const [incRequiresSubjects, setIncRequiresSubjects] = useState(false);
  const [createIncidentError, setCreateIncidentError] = useState<string | null>(
    null,
  );
  const [creatingIncident, setCreatingIncident] = useState(false);

  // Transition incident state
  const [transitionDrafts, setTransitionDrafts] = useState<
    Record<string, IncidentTransitionDraft>
  >({});
  const [transitioningId, setTransitioningId] = useState<string | null>(null);
  const [transitionErrors, setTransitionErrors] = useState<
    Record<string, string | null>
  >({});

  // Legal holds state
  const [legalHolds, setLegalHolds] = useState<GlobalAdminPrivacyLegalHold[]>(
    [],
  );
  const [legalHoldsNextCursor, setLegalHoldsNextCursor] = useState<
    string | null
  >(null);
  const [loadingMoreLegalHolds, setLoadingMoreLegalHolds] = useState(false);
  const [legalHoldsForbidden, setLegalHoldsForbidden] = useState(false);
  const [legalHoldsError, setLegalHoldsError] = useState<string | null>(null);

  // Create legal hold form state
  const [createHoldOpen, setCreateHoldOpen] = useState(false);
  const [holdScope, setHoldScope] = useState<GlobalAdminPrivacyLegalHoldScope>(
    "expediente_privacidad",
  );
  const [holdAuthority, setHoldAuthority] = useState("");
  const [holdLegalBasis, setHoldLegalBasis] = useState("");
  const [holdAccountId, setHoldAccountId] = useState("");
  const [holdRequestId, setHoldRequestId] = useState("");
  const [holdIncidentId, setHoldIncidentId] = useState("");
  const [createHoldError, setCreateHoldError] = useState<string | null>(null);
  const [creatingHold, setCreatingHold] = useState(false);

  // Release legal hold state
  const [releaseReasonDrafts, setReleaseReasonDrafts] = useState<
    Record<string, string>
  >({});
  const [releasingHoldId, setReleasingHoldId] = useState<string | null>(null);
  const [releaseHoldErrors, setReleaseHoldErrors] = useState<
    Record<string, string | null>
  >({});

  const loadPhase7Data = useCallback(async () => {
    setLoadingPhase7(true);
    setIncidentsForbidden(false);
    setIncidentsError(null);
    setLegalHoldsForbidden(false);
    setLegalHoldsError(null);

    const [incidentsResult, holdsResult] = await Promise.allSettled([
      listGlobalPrivacyIncidents({ page_size: 25 }),
      listGlobalPrivacyLegalHolds({ page_size: 25 }),
    ]);

    if (incidentsResult.status === "fulfilled") {
      setIncidents([...incidentsResult.value.items]);
      setIncidentsNextCursor(incidentsResult.value.next_cursor ?? null);
    } else if (isForbiddenError(incidentsResult.reason)) {
      setIncidentsForbidden(true);
      setIncidents([]);
      setIncidentsNextCursor(null);
    } else {
      setIncidentsError(
        incidentsResult.reason instanceof Error
          ? incidentsResult.reason.message
          : "No pudimos cargar los incidentes de privacidad.",
      );
    }

    if (holdsResult.status === "fulfilled") {
      setLegalHolds([...holdsResult.value.items]);
      setLegalHoldsNextCursor(holdsResult.value.next_cursor ?? null);
    } else if (isForbiddenError(holdsResult.reason)) {
      setLegalHoldsForbidden(true);
      setLegalHolds([]);
      setLegalHoldsNextCursor(null);
    } else {
      setLegalHoldsError(
        holdsResult.reason instanceof Error
          ? holdsResult.reason.message
          : "No pudimos cargar las retenciones legales.",
      );
    }

    setHasLoadedOnce(true);
    setLoadingPhase7(false);
  }, [listGlobalPrivacyIncidents, listGlobalPrivacyLegalHolds]);

  const handleToggleSection = () => {
    const nextOpen = !isOpen;
    setIsOpen(nextOpen);
    if (nextOpen && !hasLoadedOnce) {
      void loadPhase7Data();
    }
  };

  const handleLoadMoreIncidents = async () => {
    if (!incidentsNextCursor || loadingMoreIncidents) return;
    setLoadingMoreIncidents(true);
    try {
      const page = await listGlobalPrivacyIncidents({
        cursor: incidentsNextCursor,
        page_size: 25,
      });
      setIncidents((prev) => [...prev, ...page.items]);
      setIncidentsNextCursor(page.next_cursor ?? null);
    } catch (err) {
      if (isForbiddenError(err)) {
        setIncidentsForbidden(true);
      } else {
        setIncidentsError(
          err instanceof Error
            ? err.message
            : "No pudimos cargar más incidentes.",
        );
      }
    } finally {
      setLoadingMoreIncidents(false);
    }
  };

  const handleLoadMoreLegalHolds = async () => {
    if (!legalHoldsNextCursor || loadingMoreLegalHolds) return;
    setLoadingMoreLegalHolds(true);
    try {
      const page = await listGlobalPrivacyLegalHolds({
        cursor: legalHoldsNextCursor,
        page_size: 25,
      });
      setLegalHolds((prev) => [...prev, ...page.items]);
      setLegalHoldsNextCursor(page.next_cursor ?? null);
    } catch (err) {
      if (isForbiddenError(err)) {
        setLegalHoldsForbidden(true);
      } else {
        setLegalHoldsError(
          err instanceof Error
            ? err.message
            : "No pudimos cargar más retenciones legales.",
        );
      }
    } finally {
      setLoadingMoreLegalHolds(false);
    }
  };

  const handleToggleIncidentDetail = async (incident: GlobalAdminPrivacyIncident) => {
    setIncidentDetailError(null);
    if (expandedIncidentId === incident.id) {
      setExpandedIncidentId(null);
      return;
    }
    setExpandedIncidentId(incident.id);
    setTransitionDrafts((prev) =>
      prev[incident.id]
        ? prev
        : {
            ...prev,
            [incident.id]: {
              action: "contain",
              detail: "",
              containmentMeasures: incident.containment_measures ?? "",
              agencyReference: incident.agency_reference ?? "",
              requiresAgencyNotice: incident.requires_agency_notice,
              requiresSubjectNotice: incident.requires_subject_notice,
            },
          },
    );
    setLoadingIncidentDetailId(incident.id);
    try {
      const detail = await getGlobalPrivacyIncident(incident.id);
      setIncidentDetails((prev) => ({ ...prev, [incident.id]: detail }));
    } catch (err) {
      setIncidentDetailError(
        isForbiddenError(err)
          ? "No cuentas con permisos suficientes para ver el detalle del incidente (HTTP 403)."
          : err instanceof Error
            ? err.message
            : "No pudimos cargar el detalle del incidente.",
      );
    } finally {
      setLoadingIncidentDetailId(null);
    }
  };

  const updateTransitionDraft = <K extends keyof IncidentTransitionDraft>(
    incident: GlobalAdminPrivacyIncident,
    field: K,
    value: IncidentTransitionDraft[K],
  ) => {
    setTransitionDrafts((prev) => {
      const current = prev[incident.id] ?? {
        action: "contain" as GlobalAdminPrivacyIncidentAction,
        detail: "",
        containmentMeasures: incident.containment_measures ?? "",
        agencyReference: incident.agency_reference ?? "",
        requiresAgencyNotice: incident.requires_agency_notice,
        requiresSubjectNotice: incident.requires_subject_notice,
      };
      return {
        ...prev,
        [incident.id]: {
          ...current,
          [field]: value,
        },
      };
    });
  };

  const handleCreateIncident = async (event: React.FormEvent) => {
    event.preventDefault();
    setCreateIncidentError(null);

    const trimmedTitle = incTitle.trim();
    const categories = parseCommaList(incCategories);
    const systems = parseCommaList(incSystems);
    const trimmedLeader = incLeaderId.trim();
    const trimmedSummary = incSummary.trim();
    const trimmedSubjects = incEstimatedSubjects.trim();

    if (trimmedTitle.length < 5 || trimmedTitle.length > 200) {
      setCreateIncidentError(
        "El título del incidente debe tener entre 5 y 200 caracteres.",
      );
      return;
    }
    if (categories.length === 0) {
      setCreateIncidentError(
        "Ingresa al menos una categoría de datos afectada (separadas por coma).",
      );
      return;
    }
    if (systems.length === 0) {
      setCreateIncidentError(
        "Ingresa al menos un sistema afectado (separados por coma).",
      );
      return;
    }
    let parsedSubjects: number | undefined;
    if (trimmedSubjects !== "") {
      parsedSubjects = Number(trimmedSubjects);
      if (!Number.isInteger(parsedSubjects) || parsedSubjects < 0) {
        setCreateIncidentError(
          "Los titulares estimados deben ser un número entero mayor o igual a 0.",
        );
        return;
      }
    }
    if (trimmedLeader && !isValidUUID(trimmedLeader)) {
      setCreateIncidentError("Ingresa un UUID válido para el líder asignado.");
      return;
    }
    if (trimmedSummary.length < 10 || trimmedSummary.length > 2000) {
      setCreateIncidentError(
        "El resumen de hechos debe tener entre 10 y 2000 caracteres.",
      );
      return;
    }

    if (!isRecentAuthActive(mfaStepUpRequired, recentAuthExpiresAt)) {
      openMfaStepUp();
      return;
    }

    setCreatingIncident(true);
    try {
      const created = await createGlobalPrivacyIncident({
        title: trimmedTitle,
        severity: incSeverity,
        data_categories: categories,
        affected_systems: systems,
        ...(parsedSubjects !== undefined
          ? { estimated_subjects: parsedSubjects }
          : {}),
        summary: trimmedSummary,
        requires_agency_notice: incRequiresAgency,
        requires_subject_notice: incRequiresSubjects,
        ...(trimmedLeader ? { leader_id: trimmedLeader } : {}),
      });
      setIncidents((prev) => [created, ...prev]);
      setIncidentDetails((prev) => ({ ...prev, [created.id]: created }));
      setIncTitle("");
      setIncSeverity("media");
      setIncCategories("");
      setIncSystems("");
      setIncEstimatedSubjects("0");
      setIncLeaderId("");
      setIncSummary("");
      setIncRequiresAgency(false);
      setIncRequiresSubjects(false);
      setCreateIncidentOpen(false);
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      } else {
        setCreateIncidentError(
          err instanceof Error
            ? err.message
            : "No pudimos registrar el incidente de datos personales.",
        );
      }
    } finally {
      setCreatingIncident(false);
    }
  };

  const handleTransitionIncident = async (
    event: React.FormEvent,
    incident: GlobalAdminPrivacyIncident,
  ) => {
    event.preventDefault();
    setTransitionErrors((prev) => ({ ...prev, [incident.id]: null }));

    const draft = transitionDrafts[incident.id] ?? {
      action: "contain" as GlobalAdminPrivacyIncidentAction,
      detail: "",
      containmentMeasures: incident.containment_measures ?? "",
      agencyReference: incident.agency_reference ?? "",
      requiresAgencyNotice: incident.requires_agency_notice,
      requiresSubjectNotice: incident.requires_subject_notice,
    };

    const trimmedDetail = draft.detail.trim();
    const trimmedContainment = draft.containmentMeasures.trim();
    const trimmedAgencyRef = draft.agencyReference.trim();

    if (draft.action === "add_note" && trimmedDetail.length < 5) {
      setTransitionErrors((prev) => ({
        ...prev,
        [incident.id]: "La nota debe tener al menos 5 caracteres.",
      }));
      return;
    }
    if (trimmedDetail && (trimmedDetail.length < 5 || trimmedDetail.length > 2000)) {
      setTransitionErrors((prev) => ({
        ...prev,
        [incident.id]: "El detalle debe tener entre 5 y 2000 caracteres.",
      }));
      return;
    }
    if (
      draft.action === "contain" &&
      trimmedContainment &&
      (trimmedContainment.length < 10 || trimmedContainment.length > 2000)
    ) {
      setTransitionErrors((prev) => ({
        ...prev,
        [incident.id]:
          "Las medidas de contención deben tener entre 10 y 2000 caracteres.",
      }));
      return;
    }
    if (
      draft.action === "notify_agency" &&
      (trimmedAgencyRef.length < 2 || trimmedAgencyRef.length > 160)
    ) {
      setTransitionErrors((prev) => ({
        ...prev,
        [incident.id]:
          "Ingresa la referencia de notificación a la Agencia (2 a 160 caracteres).",
      }));
      return;
    }

    if (!isRecentAuthActive(mfaStepUpRequired, recentAuthExpiresAt)) {
      openMfaStepUp();
      return;
    }

    const detailRecord = incidentDetails[incident.id];
    const expectedVersion = detailRecord?.version ?? incident.version;

    setTransitioningId(incident.id);
    try {
      const updated = await transitionGlobalPrivacyIncident(incident.id, {
        action: draft.action,
        expected_version: expectedVersion,
        ...(trimmedDetail ? { detail: trimmedDetail } : {}),
        ...(draft.action === "contain" && trimmedContainment
          ? { containment_measures: trimmedContainment }
          : {}),
        ...(draft.action === "notify_agency"
          ? { agency_reference: trimmedAgencyRef }
          : {}),
        ...(draft.action === "evaluate"
          ? {
              requires_agency_notice: draft.requiresAgencyNotice,
              requires_subject_notice: draft.requiresSubjectNotice,
            }
          : {}),
      });
      setIncidentDetails((prev) => ({ ...prev, [incident.id]: updated }));
      setIncidents((prev) =>
        prev.map((item) => (item.id === updated.id ? updated : item)),
      );
      setTransitionDrafts((prev) => ({
        ...prev,
        [incident.id]: {
          ...draft,
          detail: "",
          containmentMeasures: updated.containment_measures ?? "",
          agencyReference: updated.agency_reference ?? "",
          requiresAgencyNotice: updated.requires_agency_notice,
          requiresSubjectNotice: updated.requires_subject_notice,
        },
      }));
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      } else {
        setTransitionErrors((prev) => ({
          ...prev,
          [incident.id]:
            err instanceof Error
              ? err.message
              : "No pudimos ejecutar la transición del incidente.",
        }));
      }
    } finally {
      setTransitioningId(null);
    }
  };

  const handleCreateLegalHold = async (event: React.FormEvent) => {
    event.preventDefault();
    setCreateHoldError(null);

    const trimmedAuthority = holdAuthority.trim();
    const trimmedBasis = holdLegalBasis.trim();
    const trimmedAccountId = holdAccountId.trim();
    const trimmedRequestId = holdRequestId.trim();
    const trimmedIncidentId = holdIncidentId.trim();

    if (trimmedAuthority.length < 3 || trimmedAuthority.length > 200) {
      setCreateHoldError("La autoridad debe tener entre 3 y 200 caracteres.");
      return;
    }
    if (trimmedBasis.length < 10 || trimmedBasis.length > 1000) {
      setCreateHoldError(
        "El fundamento legal debe tener entre 10 y 1000 caracteres.",
      );
      return;
    }
    if (trimmedAccountId && !isValidUUID(trimmedAccountId)) {
      setCreateHoldError("Ingresa un UUID válido para la cuenta vinculada.");
      return;
    }
    if (trimmedRequestId && !isValidUUID(trimmedRequestId)) {
      setCreateHoldError("Ingresa un UUID válido para la solicitud vinculada.");
      return;
    }
    if (trimmedIncidentId && !isValidUUID(trimmedIncidentId)) {
      setCreateHoldError("Ingresa un UUID válido para el incidente vinculado.");
      return;
    }

    if (!isRecentAuthActive(mfaStepUpRequired, recentAuthExpiresAt)) {
      openMfaStepUp();
      return;
    }

    setCreatingHold(true);
    try {
      const created = await createGlobalPrivacyLegalHold({
        scope: holdScope,
        authority: trimmedAuthority,
        legal_basis: trimmedBasis,
        ...(trimmedAccountId ? { account_id: trimmedAccountId } : {}),
        ...(trimmedRequestId ? { request_id: trimmedRequestId } : {}),
        ...(trimmedIncidentId ? { incident_id: trimmedIncidentId } : {}),
      });
      setLegalHolds((prev) => [created, ...prev]);
      setHoldAuthority("");
      setHoldLegalBasis("");
      setHoldAccountId("");
      setHoldRequestId("");
      setHoldIncidentId("");
      setCreateHoldOpen(false);
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      } else {
        setCreateHoldError(
          err instanceof Error
            ? err.message
            : "No pudimos aplicar la retención legal.",
        );
      }
    } finally {
      setCreatingHold(false);
    }
  };

  const handleReleaseLegalHold = async (hold: GlobalAdminPrivacyLegalHold) => {
    setReleaseHoldErrors((prev) => ({ ...prev, [hold.id]: null }));
    const trimmedReason = (releaseReasonDrafts[hold.id] ?? "").trim();
    if (trimmedReason.length < 10 || trimmedReason.length > 1000) {
      setReleaseHoldErrors((prev) => ({
        ...prev,
        [hold.id]:
          "El motivo de liberación debe tener entre 10 y 1000 caracteres.",
      }));
      return;
    }

    if (!isRecentAuthActive(mfaStepUpRequired, recentAuthExpiresAt)) {
      openMfaStepUp();
      return;
    }

    setReleasingHoldId(hold.id);
    try {
      const updated = await releaseGlobalPrivacyLegalHold(hold.id, {
        release_reason: trimmedReason,
      });
      setLegalHolds((prev) =>
        prev.map((item) => (item.id === updated.id ? updated : item)),
      );
      setReleaseReasonDrafts((prev) => {
        const next = { ...prev };
        delete next[hold.id];
        return next;
      });
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      } else {
        setReleaseHoldErrors((prev) => ({
          ...prev,
          [hold.id]:
            err instanceof Error
              ? err.message
              : "No pudimos liberar la retención legal.",
        }));
      }
    } finally {
      setReleasingHoldId(null);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          <button
            type="button"
            aria-expanded={isOpen}
            aria-controls="privacy-phase7-content"
            onClick={handleToggleSection}
            className="flex w-full items-center justify-between gap-3 text-left"
          >
            <span className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4" />
              Incidentes y Retención Legal (Fase 7)
            </span>
            <span className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground">
              {isOpen ? (
                <>
                  Ocultar
                  <ChevronUp className="h-4 w-4" />
                </>
              ) : (
                <>
                  Desplegar
                  <ChevronDown className="h-4 w-4" />
                </>
              )}
            </span>
          </button>
        </CardTitle>
      </CardHeader>

      {isOpen && (
        <CardContent id="privacy-phase7-content" className="space-y-6">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
            <p className="text-xs text-muted-foreground">
              Gestión de brechas de datos personales y retenciones legales de
              conservación (Ley 21.719).
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={loadingPhase7}
              onClick={() => void loadPhase7Data()}
            >
              {loadingPhase7 ? (
                <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              )}
              Recargar Fase 7
            </Button>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            {/* Subsección Incidentes de datos personales */}
            <section className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                  <AlertTriangle className="h-4 w-4" />
                  Incidentes de datos personales
                </h3>
                {!incidentsForbidden && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-expanded={createIncidentOpen}
                    onClick={() => setCreateIncidentOpen((prev) => !prev)}
                  >
                    <Plus className="mr-1 h-3.5 w-3.5" />
                    Registrar incidente
                  </Button>
                )}
              </div>

              {createIncidentOpen && !incidentsForbidden && (
                <form
                  onSubmit={(e) => void handleCreateIncident(e)}
                  className="space-y-3 rounded-lg border border-border p-3"
                >
                  <p className="text-sm font-medium text-foreground">
                    Nuevo incidente de privacidad
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-title">Título</Label>
                      <Input
                        id="incident-create-title"
                        value={incTitle}
                        onChange={(e) => setIncTitle(e.target.value)}
                        maxLength={200}
                        placeholder="Ej. Exposición temporal de registros"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-severity">Severidad</Label>
                      <select
                        id="incident-create-severity"
                        value={incSeverity}
                        onChange={(e) =>
                          setIncSeverity(
                            e.target.value as GlobalAdminPrivacyIncidentSeverity,
                          )
                        }
                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                      >
                        {INCIDENT_SEVERITY_OPTIONS.map((opt) => (
                          <option key={opt.value} value={opt.value}>
                            {opt.label}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-categories">
                        Categorías (separadas por coma)
                      </Label>
                      <Input
                        id="incident-create-categories"
                        value={incCategories}
                        onChange={(e) => setIncCategories(e.target.value)}
                        placeholder="identificacion, contacto"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-systems">
                        Sistemas afectados (separados por coma)
                      </Label>
                      <Input
                        id="incident-create-systems"
                        value={incSystems}
                        onChange={(e) => setIncSystems(e.target.value)}
                        placeholder="api-core, s3-kyc"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-subjects">
                        Titulares estimados
                      </Label>
                      <Input
                        id="incident-create-subjects"
                        type="number"
                        min={0}
                        value={incEstimatedSubjects}
                        onChange={(e) =>
                          setIncEstimatedSubjects(e.target.value)
                        }
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="incident-create-leader">
                        UUID de líder (opcional)
                      </Label>
                      <Input
                        id="incident-create-leader"
                        value={incLeaderId}
                        onChange={(e) => setIncLeaderId(e.target.value)}
                        placeholder="110e8400-e29b-41d4-a716-446655440000"
                        autoComplete="off"
                      />
                    </div>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="incident-create-summary">
                      Resumen de hechos
                    </Label>
                    <textarea
                      id="incident-create-summary"
                      value={incSummary}
                      onChange={(e) => setIncSummary(e.target.value)}
                      maxLength={2000}
                      className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      placeholder="Describe los hechos detectados y alcance preliminar (mín. 10 caracteres)"
                    />
                  </div>
                  <div className="flex flex-wrap gap-4 text-xs">
                    <label
                      htmlFor="incident-create-agency-notice"
                      className="flex items-center gap-2"
                    >
                      <input
                        id="incident-create-agency-notice"
                        type="checkbox"
                        checked={incRequiresAgency}
                        onChange={(e) => setIncRequiresAgency(e.target.checked)}
                      />
                      Requiere notificación a Agencia
                    </label>
                    <label
                      htmlFor="incident-create-subject-notice"
                      className="flex items-center gap-2"
                    >
                      <input
                        id="incident-create-subject-notice"
                        type="checkbox"
                        checked={incRequiresSubjects}
                        onChange={(e) =>
                          setIncRequiresSubjects(e.target.checked)
                        }
                      />
                      Requiere notificación a titulares
                    </label>
                  </div>
                  {createIncidentError && (
                    <p role="alert" className="text-xs text-destructive">
                      {createIncidentError}
                    </p>
                  )}
                  {showMfaHint && (
                    <p className="text-xs text-muted-foreground">
                      Se solicitará verificación MFA reciente antes de registrar
                      el incidente.
                    </p>
                  )}
                  <Button
                    type="submit"
                    size="sm"
                    disabled={creatingIncident}
                  >
                    {creatingIncident
                      ? "Registrando…"
                      : "Confirmar registro de incidente"}
                  </Button>
                </form>
              )}

              {loadingPhase7 && incidents.length === 0 && !incidentsForbidden ? (
                <p className="text-sm text-muted-foreground">
                  Cargando incidentes de privacidad…
                </p>
              ) : incidentsForbidden ? (
                <p className="rounded-md border border-border p-3 text-sm text-muted-foreground">
                  No cuentas con permisos para consultar incidentes de datos
                  personales (HTTP 403).
                </p>
              ) : incidentsError ? (
                <p role="alert" className="text-xs text-destructive">
                  {incidentsError}
                </p>
              ) : incidents.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No hay incidentes de datos personales registrados.
                </p>
              ) : (
                <div className="space-y-3">
                  {incidents.map((inc) => {
                    const detail = incidentDetails[inc.id];
                    const transitionDraft = transitionDrafts[inc.id] ?? {
                      action: "contain" as GlobalAdminPrivacyIncidentAction,
                      detail: "",
                      containmentMeasures: inc.containment_measures ?? "",
                      agencyReference: inc.agency_reference ?? "",
                      requiresAgencyNotice: inc.requires_agency_notice,
                      requiresSubjectNotice: inc.requires_subject_notice,
                    };

                    return (
                      <div
                        key={inc.id}
                        className="space-y-3 rounded-lg border border-border p-3"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-mono text-xs font-medium text-muted-foreground">
                              {inc.folio}
                            </span>
                            <Badge variant={getSeverityBadgeVariant(inc.severity)}>
                              {inc.severity}
                            </Badge>
                            <Badge variant="outline">{inc.status}</Badge>
                          </div>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            aria-expanded={expandedIncidentId === inc.id}
                            onClick={() => void handleToggleIncidentDetail(inc)}
                          >
                            {expandedIncidentId === inc.id
                              ? "Ocultar detalle"
                              : "Ver detalle"}
                          </Button>
                        </div>

                        <p className="text-sm font-medium text-foreground">
                          {inc.title}
                        </p>

                        <div className="grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
                          <span>
                            Titulares estimados:{" "}
                            <strong className="text-foreground">
                              {inc.estimated_subjects}
                            </strong>
                          </span>
                          <span>
                            Detectado:{" "}
                            {new Date(inc.detected_at).toLocaleString("es-CL")}
                          </span>
                          <span>
                            Notif. Agencia:{" "}
                            {inc.requires_agency_notice
                              ? inc.agency_notified_at
                                ? "Notificada"
                                : "Requerida"
                              : "No requerida"}
                            {inc.agency_reference
                              ? ` (${inc.agency_reference})`
                              : ""}
                          </span>
                          <span>
                            Notif. Titulares:{" "}
                            {inc.requires_subject_notice
                              ? inc.subjects_notified_at
                                ? "Notificados"
                                : "Requerida"
                              : "No requerida"}
                          </span>
                        </div>

                        {expandedIncidentId === inc.id && (
                          <div className="space-y-3 border-t border-border pt-3">
                            {loadingIncidentDetailId === inc.id ? (
                              <p className="text-xs text-muted-foreground">
                                Cargando línea de tiempo del incidente…
                              </p>
                            ) : incidentDetailError ? (
                              <p role="alert" className="text-xs text-destructive">
                                {incidentDetailError}
                              </p>
                            ) : (
                              <>
                                <div className="space-y-1 text-xs text-muted-foreground">
                                  <p className="text-foreground">{inc.summary}</p>
                                  <p>
                                    Categorías:{" "}
                                    {inc.data_categories.join(", ") || "—"}
                                  </p>
                                  <p>
                                    Sistemas:{" "}
                                    {inc.affected_systems.join(", ") || "—"}
                                  </p>
                                  {inc.containment_measures && (
                                    <p>
                                      Contención: {inc.containment_measures}
                                    </p>
                                  )}
                                  <p className="font-mono">
                                    Líder: {inc.leader_id} · Versión:{" "}
                                    {detail?.version ?? inc.version}
                                  </p>
                                </div>

                                <div className="space-y-1.5 rounded-md bg-muted/40 p-2.5">
                                  <p className="text-xs font-medium text-foreground">
                                    Línea de tiempo del incidente
                                  </p>
                                  {!detail || detail.events.length === 0 ? (
                                    <p className="text-xs text-muted-foreground">
                                      Sin eventos registrados.
                                    </p>
                                  ) : (
                                    <div className="space-y-1.5">
                                      {detail.events.map((ev) => (
                                        <div
                                          key={ev.id}
                                          className="border-l-2 border-border pl-2 text-xs"
                                        >
                                          <p className="font-medium text-foreground">
                                            #{ev.sequence} ·{" "}
                                            {INCIDENT_EVENT_LABELS[
                                              ev.event_type
                                            ] ?? ev.event_type}
                                          </p>
                                          <p className="text-muted-foreground">
                                            {ev.detail}
                                          </p>
                                          <p className="text-muted-foreground">
                                            {new Date(
                                              ev.created_at,
                                            ).toLocaleString("es-CL")}
                                          </p>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>

                                <form
                                  onSubmit={(e) =>
                                    void handleTransitionIncident(e, inc)
                                  }
                                  className="space-y-2 rounded-md border border-border p-3"
                                >
                                  <p className="text-sm font-medium text-foreground">
                                    Transicionar incidente
                                  </p>
                                  <div className="space-y-1">
                                    <Label
                                      htmlFor={`incident-transition-action-${inc.id}`}
                                    >
                                      Acción de transición
                                    </Label>
                                    <select
                                      id={`incident-transition-action-${inc.id}`}
                                      value={transitionDraft.action}
                                      onChange={(e) =>
                                        updateTransitionDraft(
                                          inc,
                                          "action",
                                          e.target
                                            .value as GlobalAdminPrivacyIncidentAction,
                                        )
                                      }
                                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                                    >
                                      {INCIDENT_ACTION_OPTIONS.map((opt) => (
                                        <option
                                          key={opt.value}
                                          value={opt.value}
                                        >
                                          {opt.label}
                                        </option>
                                      ))}
                                    </select>
                                  </div>

                                  {transitionDraft.action === "notify_agency" && (
                                    <div className="space-y-1">
                                      <Label
                                        htmlFor={`incident-agency-ref-${inc.id}`}
                                      >
                                        Referencia de notificación a Agencia
                                      </Label>
                                      <Input
                                        id={`incident-agency-ref-${inc.id}`}
                                        value={transitionDraft.agencyReference}
                                        onChange={(e) =>
                                          updateTransitionDraft(
                                            inc,
                                            "agencyReference",
                                            e.target.value,
                                          )
                                        }
                                        maxLength={160}
                                        placeholder="Ej. AGPD-2026-0091"
                                        autoComplete="off"
                                      />
                                    </div>
                                  )}

                                  {transitionDraft.action === "contain" && (
                                    <div className="space-y-1">
                                      <Label
                                        htmlFor={`incident-containment-${inc.id}`}
                                      >
                                        Medidas de contención (opcional, mín. 10
                                        caracteres)
                                      </Label>
                                      <textarea
                                        id={`incident-containment-${inc.id}`}
                                        value={
                                          transitionDraft.containmentMeasures
                                        }
                                        onChange={(e) =>
                                          updateTransitionDraft(
                                            inc,
                                            "containmentMeasures",
                                            e.target.value,
                                          )
                                        }
                                        maxLength={2000}
                                        className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                                      />
                                    </div>
                                  )}

                                  {transitionDraft.action === "evaluate" && (
                                    <div className="flex flex-wrap gap-4 text-xs">
                                      <label
                                        htmlFor={`incident-eval-agency-${inc.id}`}
                                        className="flex items-center gap-2"
                                      >
                                        <input
                                          id={`incident-eval-agency-${inc.id}`}
                                          type="checkbox"
                                          checked={
                                            transitionDraft.requiresAgencyNotice
                                          }
                                          onChange={(e) =>
                                            updateTransitionDraft(
                                              inc,
                                              "requiresAgencyNotice",
                                              e.target.checked,
                                            )
                                          }
                                        />
                                        Requiere notificación a Agencia
                                      </label>
                                      <label
                                        htmlFor={`incident-eval-subjects-${inc.id}`}
                                        className="flex items-center gap-2"
                                      >
                                        <input
                                          id={`incident-eval-subjects-${inc.id}`}
                                          type="checkbox"
                                          checked={
                                            transitionDraft.requiresSubjectNotice
                                          }
                                          onChange={(e) =>
                                            updateTransitionDraft(
                                              inc,
                                              "requiresSubjectNotice",
                                              e.target.checked,
                                            )
                                          }
                                        />
                                        Requiere notificación a titulares
                                      </label>
                                    </div>
                                  )}

                                  <div className="space-y-1">
                                    <Label
                                      htmlFor={`incident-transition-detail-${inc.id}`}
                                    >
                                      Detalle o respaldo
                                    </Label>
                                    <textarea
                                      id={`incident-transition-detail-${inc.id}`}
                                      value={transitionDraft.detail}
                                      onChange={(e) =>
                                        updateTransitionDraft(
                                          inc,
                                          "detail",
                                          e.target.value,
                                        )
                                      }
                                      maxLength={2000}
                                      className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                                      placeholder="Detalle de la acción realizada"
                                    />
                                  </div>

                                  {transitionErrors[inc.id] && (
                                    <p
                                      role="alert"
                                      className="text-xs text-destructive"
                                    >
                                      {transitionErrors[inc.id]}
                                    </p>
                                  )}
                                  {showMfaHint && (
                                    <p className="text-xs text-muted-foreground">
                                      Se solicitará MFA reciente antes de
                                      ejecutar la transición.
                                    </p>
                                  )}
                                  <Button
                                    type="submit"
                                    size="sm"
                                    disabled={transitioningId === inc.id}
                                  >
                                    {transitioningId === inc.id
                                      ? "Actualizando…"
                                      : "Ejecutar transición"}
                                  </Button>
                                </form>
                              </>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}

                  {incidentsNextCursor && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={loadingMoreIncidents}
                      onClick={() => void handleLoadMoreIncidents()}
                    >
                      {loadingMoreIncidents
                        ? "Cargando…"
                        : "Cargar más incidentes"}
                    </Button>
                  )}
                </div>
              )}
            </section>

            {/* Subsección Retenciones legales (Legal Hold) */}
            <section className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                  <Scale className="h-4 w-4" />
                  Retenciones legales (Legal Hold)
                </h3>
                {!legalHoldsForbidden && (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-expanded={createHoldOpen}
                    onClick={() => setCreateHoldOpen((prev) => !prev)}
                  >
                    <Plus className="mr-1 h-3.5 w-3.5" />
                    Aplicar retención legal
                  </Button>
                )}
              </div>

              {createHoldOpen && !legalHoldsForbidden && (
                <form
                  onSubmit={(e) => void handleCreateLegalHold(e)}
                  className="space-y-3 rounded-lg border border-border p-3"
                >
                  <p className="text-sm font-medium text-foreground">
                    Nueva retención legal
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1">
                      <Label htmlFor="legal-hold-scope">Ámbito</Label>
                      <select
                        id="legal-hold-scope"
                        value={holdScope}
                        onChange={(e) =>
                          setHoldScope(
                            e.target.value as GlobalAdminPrivacyLegalHoldScope,
                          )
                        }
                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                      >
                        {LEGAL_HOLD_SCOPE_OPTIONS.map((opt) => (
                          <option key={opt.value} value={opt.value}>
                            {opt.label}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="legal-hold-authority">Autoridad</Label>
                      <Input
                        id="legal-hold-authority"
                        value={holdAuthority}
                        onChange={(e) => setHoldAuthority(e.target.value)}
                        maxLength={200}
                        placeholder="Ej. Juzgado Civil / Fiscalía / DPO"
                      />
                    </div>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="legal-hold-basis">
                      Fundamento legal (mín. 10 caracteres)
                    </Label>
                    <textarea
                      id="legal-hold-basis"
                      value={holdLegalBasis}
                      onChange={(e) => setHoldLegalBasis(e.target.value)}
                      maxLength={1000}
                      className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      placeholder="Indica la resolución, requerimiento o fundamento normativo"
                    />
                  </div>
                  <div className="grid gap-3 sm:grid-cols-3">
                    <div className="space-y-1">
                      <Label htmlFor="legal-hold-account-id">
                        Cuenta (UUID, opcional)
                      </Label>
                      <Input
                        id="legal-hold-account-id"
                        value={holdAccountId}
                        onChange={(e) => setHoldAccountId(e.target.value)}
                        autoComplete="off"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="legal-hold-request-id">
                        Solicitud ARCO+ (UUID, opcional)
                      </Label>
                      <Input
                        id="legal-hold-request-id"
                        value={holdRequestId}
                        onChange={(e) => setHoldRequestId(e.target.value)}
                        autoComplete="off"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="legal-hold-incident-id">
                        Incidente (UUID, opcional)
                      </Label>
                      <Input
                        id="legal-hold-incident-id"
                        value={holdIncidentId}
                        onChange={(e) => setHoldIncidentId(e.target.value)}
                        autoComplete="off"
                      />
                    </div>
                  </div>
                  {createHoldError && (
                    <p role="alert" className="text-xs text-destructive">
                      {createHoldError}
                    </p>
                  )}
                  {showMfaHint && (
                    <p className="text-xs text-muted-foreground">
                      Se solicitará verificación MFA reciente antes de aplicar la
                      retención legal.
                    </p>
                  )}
                  <Button type="submit" size="sm" disabled={creatingHold}>
                    {creatingHold ? "Aplicando…" : "Confirmar retención legal"}
                  </Button>
                </form>
              )}

              {loadingPhase7 && legalHolds.length === 0 && !legalHoldsForbidden ? (
                <p className="text-sm text-muted-foreground">
                  Cargando retenciones legales…
                </p>
              ) : legalHoldsForbidden ? (
                <p className="rounded-md border border-border p-3 text-sm text-muted-foreground">
                  No cuentas con permisos para consultar retenciones legales
                  (HTTP 403).
                </p>
              ) : legalHoldsError ? (
                <p role="alert" className="text-xs text-destructive">
                  {legalHoldsError}
                </p>
              ) : legalHolds.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No hay retenciones legales registradas.
                </p>
              ) : (
                <div className="space-y-3">
                  {legalHolds.map((hold) => (
                    <div
                      key={hold.id}
                      className="space-y-2 rounded-lg border border-border p-3"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-xs font-medium text-muted-foreground">
                            {hold.folio}
                          </span>
                          <Badge
                            variant={
                              hold.status === "activa"
                                ? "destructive"
                                : "outline"
                            }
                          >
                            {hold.status}
                          </Badge>
                          <Badge variant="secondary">
                            {getLegalHoldScopeLabel(hold.scope)}
                          </Badge>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {new Date(hold.applied_at).toLocaleString("es-CL")}
                        </span>
                      </div>

                      <p className="text-sm font-medium text-foreground">
                        Autoridad: {hold.authority}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Fundamento: {hold.legal_basis}
                      </p>

                      {(hold.account_id ||
                        hold.request_id ||
                        hold.incident_id) && (
                        <div className="space-y-0.5 font-mono text-xs text-muted-foreground">
                          {hold.account_id && (
                            <p>Cuenta: {hold.account_id}</p>
                          )}
                          {hold.request_id && (
                            <p>Solicitud ARCO+: {hold.request_id}</p>
                          )}
                          {hold.incident_id && (
                            <p>Incidente: {hold.incident_id}</p>
                          )}
                        </div>
                      )}

                      {hold.released_at && (
                        <div className="text-xs text-muted-foreground">
                          <p>
                            Liberada:{" "}
                            {new Date(hold.released_at).toLocaleString("es-CL")}
                          </p>
                          {hold.release_reason && (
                            <p>Motivo de liberación: {hold.release_reason}</p>
                          )}
                        </div>
                      )}

                      {hold.status === "activa" && (
                        <div className="space-y-2 border-t border-border pt-2">
                          <div className="space-y-1">
                            <Label
                              htmlFor={`legal-hold-release-reason-${hold.id}`}
                            >
                              Motivo de liberación (mín. 10 caracteres)
                            </Label>
                            <Input
                              id={`legal-hold-release-reason-${hold.id}`}
                              value={releaseReasonDrafts[hold.id] ?? ""}
                              onChange={(e) =>
                                setReleaseReasonDrafts((prev) => ({
                                  ...prev,
                                  [hold.id]: e.target.value,
                                }))
                              }
                              maxLength={1000}
                              placeholder="Indica el respaldo para levantar la retención"
                            />
                          </div>
                          {releaseHoldErrors[hold.id] && (
                            <p
                              role="alert"
                              className="text-xs text-destructive"
                            >
                              {releaseHoldErrors[hold.id]}
                            </p>
                          )}
                          {showMfaHint && (
                            <p className="text-xs text-muted-foreground">
                              Se solicitará MFA reciente antes de liberar la
                              retención.
                            </p>
                          )}
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            disabled={releasingHoldId === hold.id}
                            onClick={() => void handleReleaseLegalHold(hold)}
                          >
                            {releasingHoldId === hold.id
                              ? "Liberando…"
                              : "Liberar retención"}
                          </Button>
                        </div>
                      )}
                    </div>
                  ))}

                  {legalHoldsNextCursor && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={loadingMoreLegalHolds}
                      onClick={() => void handleLoadMoreLegalHolds()}
                    >
                      {loadingMoreLegalHolds
                        ? "Cargando…"
                        : "Cargar más retenciones"}
                    </Button>
                  )}
                </div>
              )}
            </section>
          </div>
        </CardContent>
      )}
    </Card>
  );
}

export function AdminPrivacyView() {
  const {
    snapshot,
    hasAdminCapability,
    listGlobalPrivacyRequests,
    getGlobalPrivacyRequest,
    createGlobalPrivacyRequest,
    transitionGlobalPrivacyRequest,
    executeGlobalPrivacyRight,
    resolveGlobalPrivacyRequest,
    recordGlobalPrivacyDispatchReceipt,
    retryGlobalPrivacyNotifications,
    openMfaStepUp,
    listGlobalKycCases,
    listGlobalPrivacyIncidents,
    getGlobalPrivacyIncident,
    createGlobalPrivacyIncident,
    transitionGlobalPrivacyIncident,
    listGlobalPrivacyLegalHolds,
    createGlobalPrivacyLegalHold,
    releaseGlobalPrivacyLegalHold,
  } = useAdmin();

  const canManagePrivacy = hasAdminCapability("privacy.requests.manage");
  const canReadKyc = hasAdminCapability("kyc.case.read");

  const [privacyRequests, setPrivacyRequests] = useState<
    GlobalAdminPrivacyRequest[]
  >([]);
  const [privacyStatusFilter, setPrivacyStatusFilter] = useState<
    "" | GlobalAdminPrivacyRequestStatus
  >("");
  const [privacyRightTypeFilter, setPrivacyRightTypeFilter] = useState<
    "" | GlobalAdminPrivacyRequestType
  >("");
  const [privacyChannelFilter, setPrivacyChannelFilter] = useState<
    "" | GlobalAdminPrivacyChannel
  >("");
  const [privacyOverdueFilter, setPrivacyOverdueFilter] = useState<
    "" | "true" | "false"
  >("");
  const [kycCases, setKycCases] = useState<GlobalAdminKycCase[]>([]);
  const [kycStatusFilter, setKycStatusFilter] = useState("");
  const [loading, setLoading] = useState(false);
  const [privacyNextCursor, setPrivacyNextCursor] = useState<string | null>(
    null,
  );
  const [loadingMorePrivacy, setLoadingMorePrivacy] = useState(false);

  // New ARCO+ request form state
  const [accountId, setAccountId] = useState("");
  const [rightType, setRightType] =
    useState<GlobalAdminPrivacyRequestType>("acceso");
  const [reason, setReason] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [caseDetails, setCaseDetails] = useState<
    Record<string, GlobalAdminPrivacyRequestDetail>
  >({});
  const [loadingDetailId, setLoadingDetailId] = useState<string | null>(null);
  const [workflowDrafts, setWorkflowDrafts] = useState<
    Record<
      string,
      {
        action: GlobalAdminPrivacyWorkflowAction;
        assignedToId: string;
        accountId: string;
        note: string;
        extraDays: string;
      }
    >
  >({});
  const [workflowSubmittingId, setWorkflowSubmittingId] = useState<string | null>(
    null,
  );
  const [workflowError, setWorkflowError] = useState<string | null>(null);
  const [retryingNotificationId, setRetryingNotificationId] = useState<
    string | null
  >(null);
  const [notificationRetryError, setNotificationRetryError] = useState<
    string | null
  >(null);
  const [resolutionDrafts, setResolutionDrafts] = useState<
    Record<
      string,
      { decision: "resuelta" | "denegada_fundada"; response: string }
    >
  >({});
  const [resolvingId, setResolvingId] = useState<string | null>(null);
  const [resolutionError, setResolutionError] = useState<string | null>(null);
  const [dispatchDrafts, setDispatchDrafts] = useState<
    Record<string, { channel: "email" | "postal"; destination: string; sentAt: string; evidenceRef: string }>
  >({});
  const [dispatchingId, setDispatchingId] = useState<string | null>(null);
  const [dispatchError, setDispatchError] = useState<string | null>(null);
  const [executionDrafts, setExecutionDrafts] = useState<
    Record<string, ExecutionDraft>
  >({});
  const [executingRequestId, setExecutingRequestId] = useState<string | null>(
    null,
  );
  const [executionErrors, setExecutionErrors] = useState<
    Record<string, string | null>
  >({});
  const [liftReasonDrafts, setLiftReasonDrafts] = useState<
    Record<string, string>
  >({});
  const [liftingBlockId, setLiftingBlockId] = useState<string | null>(null);
  const [liftBlockErrors, setLiftBlockErrors] = useState<
    Record<string, string | null>
  >({});

  const showMfaHint =
    snapshot.mfaStepUpRequired ||
    !snapshot.adminSession?.mfa_verified_at ||
    !snapshot.adminSession?.recent_auth_expires_at;
  const canResolvePortalAccess = (request: GlobalAdminPrivacyRequest) =>
    request.can_publish_response === true;

  const toggleCaseDetail = async (requestId: string) => {
    setWorkflowError(null);
    setNotificationRetryError(null);
    if (expandedId === requestId) {
      setExpandedId(null);
      return;
    }
    setExpandedId(requestId);
    setLoadingDetailId(requestId);
    try {
      const detail = await getGlobalPrivacyRequest(requestId);
      setCaseDetails((prev) => ({ ...prev, [requestId]: detail }));
    } catch {
      // Surfaced via AdminFeedbackBanners
    } finally {
      setLoadingDetailId(null);
    }
  };

  const updateWorkflowDraft = (
    requestId: string,
    field: "action" | "assignedToId" | "accountId" | "note" | "extraDays",
    value: string,
  ) => {
    setWorkflowDrafts((drafts) => ({
      ...drafts,
      [requestId]: {
        action: drafts[requestId]?.action ?? "add_internal_note",
        assignedToId: drafts[requestId]?.assignedToId ?? "",
        accountId: drafts[requestId]?.accountId ?? "",
        note: drafts[requestId]?.note ?? "",
        extraDays: drafts[requestId]?.extraDays ?? "15",
        [field]: value,
      },
    }));
  };

  const handleWorkflowSubmit = async (
    event: React.FormEvent,
    request: GlobalAdminPrivacyRequest,
  ) => {
    event.preventDefault();
    setWorkflowError(null);
    const draft = workflowDrafts[request.id] ?? {
      action: "add_internal_note" as GlobalAdminPrivacyWorkflowAction,
      assignedToId: "",
      accountId: "",
      note: "",
      extraDays: "15",
    };
    const trimmedNote = draft.note.trim();
    const trimmedAssignee = draft.assignedToId.trim();
    const trimmedAccountId = draft.accountId.trim();
    if (draft.action === "assign" && !isValidUUID(trimmedAssignee)) {
      setWorkflowError("Ingresa un UUID válido para asignar responsable.");
      return;
    }
    if (
      draft.action === "verify_identity" &&
      request.account_id === "" &&
      trimmedAccountId !== "" &&
      !isValidUUID(trimmedAccountId)
    ) {
      setWorkflowError("Ingresa un UUID válido para vincular la cuenta.");
      return;
    }
    if (
      (draft.action === "request_clarification" ||
        draft.action === "extend_deadline") &&
      trimmedNote.length < 10
    ) {
      setWorkflowError(
        "Esta acción requiere un detalle o motivo de al menos 10 caracteres.",
      );
      return;
    }
    if (draft.action === "add_internal_note" && trimmedNote.length < 5) {
      setWorkflowError("La nota interna debe tener al menos 5 caracteres.");
      return;
    }
    let parsedExtraDays: number | undefined;
    if (draft.action === "extend_deadline") {
      parsedExtraDays = Number.parseInt(draft.extraDays, 10);
      if (
        !Number.isInteger(parsedExtraDays) ||
        parsedExtraDays < 1 ||
        parsedExtraDays > 30
      ) {
        setWorkflowError("La prórroga debe ser entre 1 y 30 días corridos.");
        return;
      }
    }
    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }
    const detail = caseDetails[request.id];
    const expectedVersion = detail?.version ?? request.version ?? 1;
    setWorkflowSubmittingId(request.id);
    try {
      const updated = await transitionGlobalPrivacyRequest(request.id, {
        action: draft.action,
        expected_version: expectedVersion,
        ...(draft.action === "assign" ? { assigned_to_id: trimmedAssignee } : {}),
        ...(draft.action === "verify_identity" &&
        request.account_id === "" &&
        trimmedAccountId !== ""
          ? { account_id: trimmedAccountId }
          : {}),
        ...(trimmedNote ? { note: trimmedNote } : {}),
        ...(parsedExtraDays ? { extra_days: parsedExtraDays } : {}),
      });
      setCaseDetails((prev) => ({ ...prev, [request.id]: updated }));
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      setWorkflowDrafts((drafts) => ({
        ...drafts,
        [request.id]: {
          ...draft,
          accountId: "",
          note: "",
        },
      }));
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      }
      // Surfaced via AdminFeedbackBanners
    } finally {
      setWorkflowSubmittingId(null);
    }
  };

  const updateExecutionDraft = <K extends keyof ExecutionDraft>(
    requestId: string,
    field: K,
    value: ExecutionDraft[K],
  ) => {
    setExecutionDrafts((drafts) => ({
      ...drafts,
      [requestId]: {
        ...(drafts[requestId] ?? DEFAULT_EXECUTION_DRAFT),
        [field]: value,
      },
    }));
  };

  const handleExecuteRight = async (
    event: React.FormEvent,
    request: GlobalAdminPrivacyRequest,
  ) => {
    event.preventDefault();
    setExecutionErrors((prev) => ({ ...prev, [request.id]: null }));
    const availableActions = getAvailableExecutionActions(request.right_type);
    if (availableActions.length === 0) return;

    const draft = executionDrafts[request.id] ?? DEFAULT_EXECUTION_DRAFT;
    const action =
      draft.action && availableActions.includes(draft.action)
        ? draft.action
        : availableActions[0];
    const trimmedSummary = draft.summary.trim();
    if (
      trimmedSummary.length < MIN_EXECUTION_SUMMARY_LENGTH ||
      trimmedSummary.length > MAX_EXECUTION_SUMMARY_LENGTH
    ) {
      setExecutionErrors((prev) => ({
        ...prev,
        [request.id]: `El resumen de ejecución debe tener entre ${MIN_EXECUTION_SUMMARY_LENGTH} y ${MAX_EXECUTION_SUMMARY_LENGTH} caracteres.`,
      }));
      return;
    }

    const detail = caseDetails[request.id];
    const expectedVersion = detail?.version ?? request.version ?? 1;
    let payload: GlobalAdminPrivacyExecutionInput;

    if (action === "rectify_profile") {
      const trimmedName = draft.displayName.trim();
      const trimmedPhone = draft.phone.trim();
      if (!trimmedName && !draft.clearPhone && !trimmedPhone) {
        setExecutionErrors((prev) => ({
          ...prev,
          [request.id]:
            "Ingresa un nombre visible, un teléfono de 9 dígitos o marca limpiar teléfono.",
        }));
        return;
      }
      if (trimmedName && !/^[\p{L} ]{1,120}$/u.test(trimmedName)) {
        setExecutionErrors((prev) => ({
          ...prev,
          [request.id]:
            "El nombre visible solo puede contener letras y espacios (máx. 120 caracteres).",
        }));
        return;
      }
      if (!draft.clearPhone && trimmedPhone && !/^\d{9}$/.test(trimmedPhone)) {
        setExecutionErrors((prev) => ({
          ...prev,
          [request.id]:
            "El teléfono debe tener exactamente 9 dígitos (o marca limpiar teléfono).",
        }));
        return;
      }
      payload = {
        action: "rectify_profile",
        expected_version: expectedVersion,
        summary: trimmedSummary,
        ...(trimmedName ? { display_name: trimmedName } : {}),
        ...(draft.clearPhone
          ? { phone: "" }
          : trimmedPhone
            ? { phone: trimmedPhone }
            : {}),
      };
    } else if (action === "apply_opposition") {
      payload = {
        action: "apply_opposition",
        expected_version: expectedVersion,
        summary: trimmedSummary,
        purpose: draft.purpose,
        purpose_state: draft.purposeState,
      };
    } else if (action === "apply_block") {
      payload = {
        action: "apply_block",
        expected_version: expectedVersion,
        summary: trimmedSummary,
        scope: draft.scope,
      };
    } else {
      const trimmedRef = draft.targetReference.trim();
      if (trimmedRef && (trimmedRef.length < 2 || trimmedRef.length > 200)) {
        setExecutionErrors((prev) => ({
          ...prev,
          [request.id]:
            "La referencia de destino debe tener entre 2 y 200 caracteres.",
        }));
        return;
      }
      payload = {
        action: "issue_portability",
        expected_version: expectedVersion,
        summary: trimmedSummary,
        ...(trimmedRef ? { target_reference: trimmedRef } : {}),
      };
    }

    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }

    setExecutingRequestId(request.id);
    try {
      const updated = await executeGlobalPrivacyRight(request.id, payload);
      setCaseDetails((prev) => ({ ...prev, [request.id]: updated }));
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      setExecutionDrafts((drafts) => ({
        ...drafts,
        [request.id]: {
          ...DEFAULT_EXECUTION_DRAFT,
          action,
        },
      }));
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      }
      // Surfaced via AdminFeedbackBanners
    } finally {
      setExecutingRequestId(null);
    }
  };

  const handleLiftBlock = async (
    request: GlobalAdminPrivacyRequest,
    blockId: string,
  ) => {
    setLiftBlockErrors((prev) => ({ ...prev, [blockId]: null }));
    const trimmedReason = (liftReasonDrafts[blockId] ?? "").trim();
    if (
      trimmedReason.length < MIN_EXECUTION_SUMMARY_LENGTH ||
      trimmedReason.length > MAX_EXECUTION_SUMMARY_LENGTH
    ) {
      setLiftBlockErrors((prev) => ({
        ...prev,
        [blockId]: `El motivo de levantamiento debe tener entre ${MIN_EXECUTION_SUMMARY_LENGTH} y ${MAX_EXECUTION_SUMMARY_LENGTH} caracteres.`,
      }));
      return;
    }
    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }

    const detail = caseDetails[request.id];
    const expectedVersion = detail?.version ?? request.version ?? 1;
    setLiftingBlockId(blockId);
    try {
      const updated = await executeGlobalPrivacyRight(request.id, {
        action: "lift_block",
        expected_version: expectedVersion,
        block_id: blockId,
        summary: trimmedReason,
      });
      setCaseDetails((prev) => ({ ...prev, [request.id]: updated }));
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      setLiftReasonDrafts((prev) => {
        const next = { ...prev };
        delete next[blockId];
        return next;
      });
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      }
      // Surfaced via AdminFeedbackBanners
    } finally {
      setLiftingBlockId(null);
    }
  };

  const handleRetryNotifications = async (requestId: string) => {
    setNotificationRetryError(null);
    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }
    setRetryingNotificationId(requestId);
    try {
      const updated = await retryGlobalPrivacyNotifications(requestId);
      setCaseDetails((prev) => ({ ...prev, [requestId]: updated }));
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
    } catch (err) {
      if (isRecentAuthRequired(err)) {
        openMfaStepUp();
      } else {
        setNotificationRetryError(
          err instanceof Error
            ? err.message
            : "No pudimos reintentar las notificaciones.",
        );
      }
    } finally {
      setRetryingNotificationId(null);
    }
  };

  const updateResolutionDraft = (
    requestId: string,
    field: "decision" | "response",
    value: string,
  ) => {
    setResolutionDrafts((drafts) => ({
      ...drafts,
      [requestId]: {
        decision: drafts[requestId]?.decision ?? "resuelta",
        response: drafts[requestId]?.response ?? "",
        [field]: value,
      },
    }));
  };

  const handleResolve = async (request: GlobalAdminPrivacyRequest) => {
    const draft = resolutionDrafts[request.id];
    const response = draft?.response.trim() ?? "";
    setResolutionError(null);
    if (!canResolvePortalAccess(request)) return;
    if (
      response.length < MIN_RESPONSE_LENGTH ||
      response.length > MAX_RESPONSE_LENGTH
    ) {
      setResolutionError(
        `La respuesta debe tener entre ${MIN_RESPONSE_LENGTH} y ${MAX_RESPONSE_LENGTH} caracteres.`,
      );
      return;
    }
    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }
    setResolvingId(request.id);
    try {
      const updated = await resolveGlobalPrivacyRequest(request.id, {
        decision: draft?.decision ?? "resuelta",
        response,
      });
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      if (expandedId === request.id) {
        const refreshed = await getGlobalPrivacyRequest(request.id);
        setCaseDetails((prev) => ({ ...prev, [request.id]: refreshed }));
      }
      setResolutionDrafts((drafts) => {
        const next = { ...drafts };
        delete next[request.id];
        return next;
      });
    } catch {
      // The facade presents authorization and API errors in the shared banner.
    } finally {
      setResolvingId(null);
    }
  };

  const handleRecordDispatch = async (
    event: React.FormEvent,
    request: GlobalAdminPrivacyRequest,
  ) => {
    event.preventDefault();
    setDispatchError(null);
    const draft = dispatchDrafts[request.id];
    if (!draft?.destination.trim() || !draft.sentAt || !draft.evidenceRef.trim()) {
      setDispatchError("Completa el destino, la fecha y la referencia de evidencia.");
      return;
    }
    const sentAt = new Date(draft.sentAt);
    if (Number.isNaN(sentAt.getTime())) {
      setDispatchError("Ingresa una fecha de despacho válida.");
      return;
    }
    if (
      !isRecentAuthActive(
        snapshot.mfaStepUpRequired,
        snapshot.adminSession?.recent_auth_expires_at,
      )
    ) {
      openMfaStepUp();
      return;
    }
    setDispatchingId(request.id);
    try {
      const updated = await recordGlobalPrivacyDispatchReceipt(request.id, {
        channel: draft.channel,
        destination: draft.destination.trim(),
        sent_at: sentAt.toISOString(),
        evidence_ref: draft.evidenceRef.trim(),
      });
      setPrivacyRequests((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
      if (expandedId === request.id) {
        const refreshed = await getGlobalPrivacyRequest(request.id);
        setCaseDetails((prev) => ({ ...prev, [request.id]: refreshed }));
      }
      setDispatchDrafts((drafts) => {
        const next = { ...drafts };
        delete next[request.id];
        return next;
      });
    } catch {
      // The facade presents authorization and API errors in the shared banner.
    } finally {
      setDispatchingId(null);
    }
  };

  const updateDispatchDraft = (
    requestId: string,
    field: "channel" | "destination" | "sentAt" | "evidenceRef",
    value: string,
  ) => {
    setDispatchDrafts((drafts) => ({
      ...drafts,
      [requestId]: {
        channel: drafts[requestId]?.channel ?? "email",
        destination: drafts[requestId]?.destination ?? "",
        sentAt: drafts[requestId]?.sentAt ?? "",
        evidenceRef: drafts[requestId]?.evidenceRef ?? "",
        [field]: value,
      },
    }));
  };

  const fetchPrivacyData = useCallback(
    async (cursor?: string) => {
      const tasks: Promise<void>[] = [];
      if (canManagePrivacy) {
        tasks.push(
          listGlobalPrivacyRequests({
            status: privacyStatusFilter || undefined,
            right_type: privacyRightTypeFilter || undefined,
            channel: privacyChannelFilter || undefined,
            overdue:
              privacyOverdueFilter === ""
                ? undefined
                : privacyOverdueFilter === "true",
            cursor,
            page_size: 25,
          }).then((res) => {
            setPrivacyRequests((current) =>
              cursor ? [...current, ...res.items] : res.items,
            );
            setPrivacyNextCursor(res.next_cursor ?? null);
          }),
        );
      }
      if (canReadKyc) {
        tasks.push(
          listGlobalKycCases({
            status: kycStatusFilter || undefined,
          }).then((res) => setKycCases(res.items)),
        );
      }
      await Promise.all(tasks);
    },
    [
      canManagePrivacy,
      canReadKyc,
      kycStatusFilter,
      privacyStatusFilter,
      privacyRightTypeFilter,
      privacyChannelFilter,
      privacyOverdueFilter,
      listGlobalPrivacyRequests,
      listGlobalKycCases,
    ],
  );

  const loadData = useCallback(async () => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    setLoading(true);
    try {
      setPrivacyNextCursor(null);
      await fetchPrivacyData();
    } catch {
      // Surfaced via AdminFeedbackBanners
    } finally {
      setLoading(false);
    }
  }, [snapshot.adminSessionStatus, fetchPrivacyData]);

  const loadMorePrivacy = async () => {
    if (!privacyNextCursor || loadingMorePrivacy) return;
    setLoadingMorePrivacy(true);
    try {
      await listGlobalPrivacyRequests({
        status: privacyStatusFilter || undefined,
        right_type: privacyRightTypeFilter || undefined,
        channel: privacyChannelFilter || undefined,
        overdue:
          privacyOverdueFilter === ""
            ? undefined
            : privacyOverdueFilter === "true",
        cursor: privacyNextCursor,
        page_size: 25,
      }).then((res) => {
        setPrivacyRequests((current) => [...current, ...res.items]);
        setPrivacyNextCursor(res.next_cursor ?? null);
      });
    } catch {
      // Surfaced via AdminFeedbackBanners.
    } finally {
      setLoadingMorePrivacy(false);
    }
  };

  useEffect(() => {
    if (snapshot.adminSessionStatus !== "authenticated") return;
    void fetchPrivacyData().catch(() => {
      // Surfaced via AdminFeedbackBanners
    });
  }, [snapshot.adminSessionStatus, fetchPrivacyData]);

  const handleCreatePrivacyRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!isValidUUID(accountId.trim())) {
      setFormError("Ingresa un UUID válido de cuenta titular.");
      return;
    }
    if (!isValidAdminReason(reason)) {
      setFormError("El motivo debe tener al menos 5 caracteres válidos.");
      return;
    }
    setSubmitting(true);
    try {
      const created = await createGlobalPrivacyRequest({
        account_id: accountId.trim(),
        right_type: rightType,
        reason: reason.trim(),
      });
      setPrivacyRequests((prev) => [created, ...prev]);
      setAccountId("");
      setReason("");
    } catch {
      // Handled by facade / AdminFeedbackBanners
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Privacidad ARCO+ y Verificaciones KYC
          </h1>
          <p className="text-sm text-muted-foreground">
            Gestión de derechos Ley 21.719 (acceso, rectificación, supresión,
            oposición, portabilidad y bloqueo) y resumen de casos KYC.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void loadData()}
          disabled={loading}
        >
          {loading ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 h-4 w-4" />
          )}
          Actualizar
        </Button>
      </div>

      <AdminFeedbackBanners />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Lock className="h-4 w-4" />
              Solicitudes ARCO+ (Ley 21.719)
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {canManagePrivacy && (
              <form
                onSubmit={(e) => void handleCreatePrivacyRequest(e)}
                className="space-y-3 rounded-lg border border-border p-3"
              >
                <p className="text-sm font-medium text-foreground">
                  Registrar nueva solicitud ARCO+
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="privacy-account-id">
                      Cuenta titular (UUID)
                    </Label>
                    <Input
                      id="privacy-account-id"
                      value={accountId}
                      onChange={(e) => setAccountId(e.target.value)}
                      placeholder="110e8400-e29b-41d4-a716-446655440000"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="privacy-request-type">Derecho ARCO+</Label>
                    <select
                      id="privacy-request-type"
                      value={rightType}
                      onChange={(e) =>
                        setRightType(
                          e.target.value as GlobalAdminPrivacyRequestType,
                        )
                      }
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="acceso">Acceso</option>
                      <option value="rectificacion">Rectificación</option>
                      <option value="supresion">Supresión</option>
                      <option value="oposicion">Oposición</option>
                      <option value="portabilidad">Portabilidad</option>
                      <option value="bloqueo">Bloqueo</option>
                    </select>
                  </div>
                </div>
                <div className="space-y-1">
                  <Label htmlFor="privacy-reason">Motivo o respaldo</Label>
                  <Input
                    id="privacy-reason"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="Detalle de la solicitud del titular (mín. 5 caracteres)"
                  />
                </div>
                {formError && (
                  <p role="alert" className="text-xs text-destructive">
                    {formError}
                  </p>
                )}
                <Button type="submit" size="sm" disabled={submitting}>
                  <Plus className="mr-1 h-4 w-4" />
                  {submitting ? "Registrando..." : "Registrar solicitud"}
                </Button>
              </form>
            )}

            {!canManagePrivacy ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>privacy.requests.manage</code>.
              </p>
            ) : (
              <>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="privacy-status-filter">
                      Filtrar por estado
                    </Label>
                    <select
                      id="privacy-status-filter"
                      value={privacyStatusFilter}
                      onChange={(e) => {
                        setPrivacyNextCursor(null);
                        setPrivacyStatusFilter(
                          e.target.value as
                            | ""
                            | GlobalAdminPrivacyRequestStatus,
                        );
                      }}
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="">Todos los estados</option>
                      <option value="pendiente">Pendiente</option>
                      <option value="en_revision">En revisión</option>
                      <option value="aprobada">Aprobada</option>
                      <option value="rechazada">Rechazada</option>
                      <option value="completada">Completada</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="privacy-right-type-filter">
                      Filtrar por derecho
                    </Label>
                    <select
                      id="privacy-right-type-filter"
                      value={privacyRightTypeFilter}
                      onChange={(e) => {
                        setPrivacyNextCursor(null);
                        setPrivacyRightTypeFilter(
                          e.target.value as "" | GlobalAdminPrivacyRequestType,
                        );
                      }}
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="">Todos los derechos</option>
                      <option value="acceso">Acceso</option>
                      <option value="rectificacion">Rectificación</option>
                      <option value="supresion">Supresión</option>
                      <option value="oposicion">Oposición</option>
                      <option value="portabilidad">Portabilidad</option>
                      <option value="bloqueo">Bloqueo</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="privacy-channel-filter">
                      Filtrar por canal
                    </Label>
                    <select
                      id="privacy-channel-filter"
                      value={privacyChannelFilter}
                      onChange={(e) => {
                        setPrivacyNextCursor(null);
                        setPrivacyChannelFilter(
                          e.target.value as "" | GlobalAdminPrivacyChannel,
                        );
                      }}
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="">Todos los canales</option>
                      <option value="web">Portal web (titular)</option>
                      <option value="api">API / Mesa administrativa</option>
                      <option value="publico">Formulario público</option>
                      <option value="correo">Correo electrónico</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="privacy-overdue-filter">
                      Vencimiento operativo
                    </Label>
                    <select
                      id="privacy-overdue-filter"
                      value={privacyOverdueFilter}
                      onChange={(e) => {
                        setPrivacyNextCursor(null);
                        setPrivacyOverdueFilter(
                          e.target.value as "" | "true" | "false",
                        );
                      }}
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="">Todos los plazos</option>
                      <option value="true">Vencidas</option>
                      <option value="false">En plazo o cerradas</option>
                    </select>
                  </div>
                </div>
                {privacyRequests.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No hay solicitudes ARCO+ registradas para los filtros seleccionados.
                  </p>
                ) : (
              <div className="space-y-2">
                {privacyRequests.map((req) => (
                  <div
                    key={req.id}
                    className="rounded-lg border border-border p-3 space-y-1"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="default">{req.right_type}</Badge>
                        <Badge variant="outline">{req.status}</Badge>
                        {req.folio && (
                          <Badge variant="outline" className="font-mono">
                            {req.folio}
                          </Badge>
                        )}
                        {req.channel && (
                          <Badge variant="secondary">{req.channel}</Badge>
                        )}
                        {req.requester_type === "representante" && (
                          <Badge variant="secondary">
                            Representante
                            {req.representative_relation
                              ? ` · ${formatRepresentativeRelation(req.representative_relation)}`
                              : ""}
                          </Badge>
                        )}
                        {req.requester_type === "titular" && (
                          <Badge variant="outline">Titular</Badge>
                        )}
                        {req.is_overdue && (
                          <Badge variant="destructive">Vencida</Badge>
                        )}
                        {req.version && (
                          <span className="text-xs text-muted-foreground font-mono">
                            v{req.version}
                          </span>
                        )}
                      </div>
                      <span className="font-mono text-xs text-muted-foreground">
                        {req.account_id || "Sin cuenta vinculada"}
                      </span>
                    </div>
                    <p className="text-xs text-foreground">{req.reason}</p>
                    <p className="text-xs text-muted-foreground">
                      Folio:{" "}
                      <span className="font-mono">
                        {req.folio || req.id}
                      </span>
                      {req.folio && (
                        <span className="ml-2">
                          · ID: <span className="font-mono">{req.id}</span>
                        </span>
                      )}
                    </p>
                    {(req.requester_name ||
                      req.representative_name ||
                      req.requester_email_masked ||
                      req.subject_reference) && (
                      <div className="space-y-0.5 text-xs text-muted-foreground">
                        {req.requester_name && (
                          <p>Titular: {req.requester_name}</p>
                        )}
                        {req.requester_type === "representante" &&
                          (req.representative_name ||
                            req.representative_relation) && (
                            <p>
                              Representante:{" "}
                              {req.representative_name || "No informado"}
                              {req.representative_relation
                                ? ` (${formatRepresentativeRelation(req.representative_relation)})`
                                : ""}
                            </p>
                          )}
                        {req.requester_email_masked && (
                          <p>
                            Correo:{" "}
                            <span className="font-mono">
                              {req.requester_email_masked}
                            </span>
                          </p>
                        )}
                        {req.subject_reference && (
                          <p>
                            Referencia titular:{" "}
                            <span className="font-mono">
                              {req.subject_reference}
                            </span>
                          </p>
                        )}
                      </div>
                    )}
                    {(req.extended_due_at || req.due_at) && (
                      <p className="text-xs text-muted-foreground">
                        {req.extended_due_at
                          ? `Plazo prorrogado: ${new Date(req.extended_due_at).toLocaleString("es-CL")}`
                          : `Plazo objetivo: ${new Date(req.due_at as string).toLocaleString("es-CL")}`}
                      </p>
                    )}
                    {req.assigned_to_id && (
                      <p className="text-xs text-muted-foreground">
                        Responsable:{" "}
                        <span className="font-mono">{req.assigned_to_id}</span>
                      </p>
                    )}
                    {req.resolved_at && (
                      <p className="text-xs text-muted-foreground">
                        Actualizada el{" "}
                        {new Date(req.resolved_at).toLocaleString("es-CL")}
                      </p>
                    )}
                    {req.detail && (
                      <p className="text-xs text-foreground">{req.detail}</p>
                    )}
                    {req.dispatch_recorded_at && (
                      <p className="text-xs text-muted-foreground">
                        Despacho registrado el{" "}
                        {new Date(req.dispatch_recorded_at).toLocaleString("es-CL")}
                      </p>
                    )}
                    {canManagePrivacy && (
                      <div className="pt-1">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => void toggleCaseDetail(req.id)}
                          disabled={loadingDetailId === req.id}
                        >
                          {loadingDetailId === req.id
                            ? "Cargando expediente…"
                            : expandedId === req.id
                              ? "Cerrar expediente"
                              : "Gestionar expediente"}
                        </Button>
                      </div>
                    )}
                    {canManagePrivacy && expandedId === req.id && (
                      <div className="space-y-3 rounded-md border border-border bg-muted/20 p-3">
                        <p className="text-xs font-medium text-foreground">
                          Historial y gestión del expediente
                        </p>
                        {caseDetails[req.id]?.notification_summary && (
                          <NotificationSummaryBox
                            summary={caseDetails[req.id].notification_summary!}
                            retrying={retryingNotificationId === req.id}
                            error={notificationRetryError}
                            showMfaHint={showMfaHint}
                            onRetry={() => void handleRetryNotifications(req.id)}
                          />
                        )}
                        {caseDetails[req.id]?.events?.length ? (
                          <ul className="space-y-1.5 text-xs">
                            {caseDetails[req.id].events.map((ev) => (
                              <li
                                key={ev.id}
                                className="rounded border border-border/60 bg-background p-2"
                              >
                                <div className="flex flex-wrap items-center justify-between gap-2">
                                  <span className="font-medium">
                                    #{ev.sequence} ·{" "}
                                    {WORKFLOW_EVENT_LABELS[ev.event_type] ??
                                      ev.event_type}
                                  </span>
                                  <Badge
                                    variant={
                                      ev.visibility === "interna"
                                        ? "secondary"
                                        : "outline"
                                    }
                                  >
                                    {ev.visibility === "interna"
                                      ? "Interna"
                                      : "Titular"}
                                  </Badge>
                                </div>
                                <p className="text-muted-foreground">
                                  {new Date(ev.created_at).toLocaleString(
                                    "es-CL",
                                  )}
                                  {ev.state_from && ev.state_to
                                    ? ` · ${ev.state_from} → ${ev.state_to}`
                                    : ""}
                                </p>
                                {ev.detail && (
                                  <p className="mt-1 whitespace-pre-wrap text-foreground">
                                    {ev.detail}
                                  </p>
                                )}
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <p className="text-xs text-muted-foreground">
                            Sin eventos registrados en el expediente.
                          </p>
                        )}
                        <form
                          onSubmit={(event) =>
                            void handleWorkflowSubmit(event, req)
                          }
                          className="space-y-2 border-t border-border pt-2"
                        >
                          <div className="grid gap-2 sm:grid-cols-2">
                            <div className="space-y-1">
                              <Label htmlFor={`privacy-wf-action-${req.id}`}>
                                Acción de expediente
                              </Label>
                              <select
                                id={`privacy-wf-action-${req.id}`}
                                value={
                                  workflowDrafts[req.id]?.action ??
                                  "add_internal_note"
                                }
                                onChange={(e) =>
                                  updateWorkflowDraft(
                                    req.id,
                                    "action",
                                    e.target.value,
                                  )
                                }
                                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                              >
                                <option value="add_internal_note">
                                  Agregar nota interna
                                </option>
                                <option value="assign">
                                  Asignar responsable
                                </option>
                                <option value="verify_identity">
                                  Verificar identidad
                                </option>
                                <option value="start_review">
                                  Pasar a revisión
                                </option>
                                <option value="request_clarification">
                                  Solicitar aclaración al titular
                                </option>
                                {!req.extended_due_at && (
                                  <option value="extend_deadline">
                                    Prorrogar plazo operativo
                                  </option>
                                )}
                              </select>
                            </div>
                            {(workflowDrafts[req.id]?.action ??
                              "add_internal_note") === "assign" && (
                              <div className="space-y-1">
                                <Label
                                  htmlFor={`privacy-wf-assignee-${req.id}`}
                                >
                                  UUID de responsable
                                </Label>
                                <Input
                                  id={`privacy-wf-assignee-${req.id}`}
                                  value={
                                    workflowDrafts[req.id]?.assignedToId ?? ""
                                  }
                                  onChange={(e) =>
                                    updateWorkflowDraft(
                                      req.id,
                                      "assignedToId",
                                      e.target.value,
                                    )
                                  }
                                  placeholder="UUID de usuario responsable"
                                />
                              </div>
                            )}
                            {(workflowDrafts[req.id]?.action ??
                              "add_internal_note") === "verify_identity" &&
                              req.account_id === "" && (
                                <div className="space-y-1">
                                  <Label
                                    htmlFor={`privacy-wf-account-${req.id}`}
                                  >
                                    ID de cuenta a vincular (UUID opcional)
                                  </Label>
                                  <Input
                                    id={`privacy-wf-account-${req.id}`}
                                    value={
                                      workflowDrafts[req.id]?.accountId ?? ""
                                    }
                                    onChange={(e) =>
                                      updateWorkflowDraft(
                                        req.id,
                                        "accountId",
                                        e.target.value,
                                      )
                                    }
                                    placeholder="UUID de cuenta titular (opcional)"
                                  />
                                </div>
                              )}
                            {(workflowDrafts[req.id]?.action ??
                              "add_internal_note") === "extend_deadline" && (
                              <div className="space-y-1">
                                <Label htmlFor={`privacy-wf-days-${req.id}`}>
                                  Días corridos adicionales (1–30)
                                </Label>
                                <Input
                                  id={`privacy-wf-days-${req.id}`}
                                  type="number"
                                  min={1}
                                  max={30}
                                  value={
                                    workflowDrafts[req.id]?.extraDays ?? "15"
                                  }
                                  onChange={(e) =>
                                    updateWorkflowDraft(
                                      req.id,
                                      "extraDays",
                                      e.target.value,
                                    )
                                  }
                                />
                              </div>
                            )}
                          </div>
                          <div className="space-y-1">
                            <Label htmlFor={`privacy-wf-note-${req.id}`}>
                              Detalle, motivo o nota
                            </Label>
                            <textarea
                              id={`privacy-wf-note-${req.id}`}
                              value={workflowDrafts[req.id]?.note ?? ""}
                              onChange={(e) =>
                                updateWorkflowDraft(
                                  req.id,
                                  "note",
                                  e.target.value,
                                )
                              }
                              maxLength={2000}
                              className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                              placeholder="Nota interna (no visible al titular) o detalle visible según la acción."
                            />
                          </div>
                          {workflowError && (
                            <p role="alert" className="text-xs text-destructive">
                              {workflowError}
                            </p>
                          )}
                          {showMfaHint && (
                            <p className="text-xs text-muted-foreground">
                              Se solicitará MFA reciente antes de actualizar el
                              expediente.
                            </p>
                          )}
                          <Button
                            type="submit"
                            size="sm"
                            disabled={workflowSubmittingId === req.id}
                          >
                            {workflowSubmittingId === req.id
                              ? "Aplicando…"
                              : "Aplicar acción"}
                          </Button>
                        </form>
                        {caseDetails[req.id] &&
                          (() => {
                            const detail = caseDetails[req.id];
                            const availableActions =
                              getAvailableExecutionActions(req.right_type);
                            const execDraft =
                              executionDrafts[req.id] ?? DEFAULT_EXECUTION_DRAFT;
                            const selectedAction =
                              execDraft.action &&
                              availableActions.includes(execDraft.action)
                                ? execDraft.action
                                : availableActions[0];
                            const canExecuteRight =
                              (detail.account_id || req.account_id) !== "" &&
                              (detail.can_publish_response ??
                                req.can_publish_response) === true &&
                              availableActions.length > 0;

                            return (
                              <div className="space-y-3 border-t border-border pt-3">
                                <p className="text-xs font-semibold text-foreground">
                                  Ejecución efectiva de derechos (Fase 4)
                                </p>

                                <div className="space-y-1.5">
                                  <p className="text-xs font-medium text-foreground">
                                    Ejecuciones registradas
                                  </p>
                                  {detail.executions?.length ? (
                                    <ul className="space-y-1.5 text-xs">
                                      {detail.executions.map((exec) => (
                                        <li
                                          key={exec.id}
                                          className="space-y-1 rounded border border-border/60 bg-background p-2"
                                        >
                                          <div className="flex flex-wrap items-center justify-between gap-2">
                                            <div className="flex flex-wrap items-center gap-1.5">
                                              <Badge variant="secondary">
                                                {exec.execution_type}
                                              </Badge>
                                              <Badge variant="outline">
                                                {exec.status}
                                              </Badge>
                                            </div>
                                            <span className="text-muted-foreground">
                                              {new Date(
                                                exec.executed_at,
                                              ).toLocaleString("es-CL")}
                                            </span>
                                          </div>
                                          <p className="text-muted-foreground">
                                            Ámbito:{" "}
                                            <span className="text-foreground">
                                              {getScopeLabel(exec.scope)}
                                            </span>
                                          </p>
                                          <p className="text-foreground">
                                            {exec.summary}
                                          </p>
                                          {exec.target_reference && (
                                            <p className="text-muted-foreground">
                                              Destino / referencia:{" "}
                                              <span className="font-mono text-foreground">
                                                {exec.target_reference}
                                              </span>
                                            </p>
                                          )}
                                          {exec.artifact_sha256 && (
                                            <p className="text-muted-foreground">
                                              SHA-256:{" "}
                                              <span
                                                className="font-mono text-foreground"
                                                title={exec.artifact_sha256}
                                              >
                                                {exec.artifact_sha256.slice(
                                                  0,
                                                  16,
                                                )}
                                                …
                                              </span>
                                            </p>
                                          )}
                                        </li>
                                      ))}
                                    </ul>
                                  ) : (
                                    <p className="text-xs text-muted-foreground">
                                      Sin ejecuciones registradas.
                                    </p>
                                  )}
                                </div>

                                <div className="space-y-1.5">
                                  <p className="text-xs font-medium text-foreground">
                                    Bloqueos temporales de tratamiento
                                  </p>
                                  {detail.treatment_blocks?.length ? (
                                    <ul className="space-y-2 text-xs">
                                      {detail.treatment_blocks.map((block) => (
                                        <li
                                          key={block.id}
                                          className="space-y-1.5 rounded border border-border/60 bg-background p-2"
                                        >
                                          <div className="flex flex-wrap items-center justify-between gap-2">
                                            <span className="font-medium text-foreground">
                                              {getScopeLabel(block.scope)}
                                            </span>
                                            <Badge
                                              variant={
                                                block.state === "activo"
                                                  ? "destructive"
                                                  : "outline"
                                              }
                                            >
                                              {block.state}
                                            </Badge>
                                          </div>
                                          <p className="text-muted-foreground">
                                            Motivo:{" "}
                                            <span className="text-foreground">
                                              {block.reason}
                                            </span>
                                          </p>
                                          {block.lift_reason && (
                                            <p className="text-muted-foreground">
                                              Motivo de levantamiento:{" "}
                                              <span className="text-foreground">
                                                {block.lift_reason}
                                              </span>
                                            </p>
                                          )}
                                          {block.state === "activo" && (
                                            <div className="space-y-1.5 border-t border-border/50 pt-2">
                                              <Label
                                                htmlFor={`privacy-lift-reason-${block.id}`}
                                              >
                                                Motivo de levantamiento
                                              </Label>
                                              <div className="flex flex-col gap-2 sm:flex-row">
                                                <Input
                                                  id={`privacy-lift-reason-${block.id}`}
                                                  value={
                                                    liftReasonDrafts[
                                                      block.id
                                                    ] ?? ""
                                                  }
                                                  onChange={(e) =>
                                                    setLiftReasonDrafts(
                                                      (prev) => ({
                                                        ...prev,
                                                        [block.id]:
                                                          e.target.value,
                                                      }),
                                                    )
                                                  }
                                                  placeholder="Motivo del levantamiento (mín. 10 caracteres)"
                                                />
                                                <Button
                                                  type="button"
                                                  variant="outline"
                                                  size="sm"
                                                  disabled={
                                                    liftingBlockId === block.id
                                                  }
                                                  onClick={() =>
                                                    void handleLiftBlock(
                                                      req,
                                                      block.id,
                                                    )
                                                  }
                                                >
                                                  {liftingBlockId === block.id
                                                    ? "Levantando…"
                                                    : "Levantar bloqueo"}
                                                </Button>
                                              </div>
                                              {liftBlockErrors[block.id] && (
                                                <p
                                                  role="alert"
                                                  className="text-xs text-destructive"
                                                >
                                                  {liftBlockErrors[block.id]}
                                                </p>
                                              )}
                                            </div>
                                          )}
                                        </li>
                                      ))}
                                    </ul>
                                  ) : (
                                    <p className="text-xs text-muted-foreground">
                                      Sin bloqueos temporales registrados.
                                    </p>
                                  )}
                                </div>

                                <div className="space-y-1.5">
                                  <p className="text-xs font-medium text-foreground">
                                    Preferencias de finalidad de la cuenta
                                  </p>
                                  {detail.purpose_preferences?.length ? (
                                    <ul className="space-y-1 text-xs">
                                      {detail.purpose_preferences.map((pref) => (
                                        <li
                                          key={pref.purpose}
                                          className="flex flex-wrap items-center justify-between gap-2 rounded border border-border/60 bg-background p-2"
                                        >
                                          <span className="text-foreground">
                                            {getPurposeLabel(pref.purpose)}
                                          </span>
                                          <Badge
                                            variant={
                                              pref.state === "permitido"
                                                ? "outline"
                                                : "secondary"
                                            }
                                          >
                                            {pref.state}
                                          </Badge>
                                        </li>
                                      ))}
                                    </ul>
                                  ) : (
                                    <p className="text-xs text-muted-foreground">
                                      Sin preferencias de finalidad registradas.
                                    </p>
                                  )}
                                </div>

                                {canExecuteRight && selectedAction && (
                                  <form
                                    onSubmit={(event) =>
                                      void handleExecuteRight(event, req)
                                    }
                                    className="space-y-2 rounded border border-border bg-background p-2.5"
                                  >
                                    <p className="text-xs font-medium text-foreground">
                                      Ejecutar acción sobre el derecho
                                    </p>
                                    <div className="space-y-1">
                                      <Label
                                        htmlFor={`privacy-exec-action-${req.id}`}
                                      >
                                        Acción sobre el derecho
                                      </Label>
                                      <select
                                        id={`privacy-exec-action-${req.id}`}
                                        value={selectedAction}
                                        onChange={(e) =>
                                          updateExecutionDraft(
                                            req.id,
                                            "action",
                                            e.target.value as Exclude<
                                              GlobalAdminPrivacyExecutionAction,
                                              "lift_block"
                                            >,
                                          )
                                        }
                                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                                      >
                                        {availableActions.map((act) => (
                                          <option key={act} value={act}>
                                            {EXECUTION_ACTION_LABELS[act]}
                                          </option>
                                        ))}
                                      </select>
                                    </div>

                                    {selectedAction === "rectify_profile" && (
                                      <div className="space-y-2">
                                        <div className="grid gap-2 sm:grid-cols-2">
                                          <div className="space-y-1">
                                            <Label
                                              htmlFor={`privacy-exec-display-name-${req.id}`}
                                            >
                                              Nombre visible (opcional)
                                            </Label>
                                            <Input
                                              id={`privacy-exec-display-name-${req.id}`}
                                              value={execDraft.displayName}
                                              onChange={(e) =>
                                                updateExecutionDraft(
                                                  req.id,
                                                  "displayName",
                                                  e.target.value,
                                                )
                                              }
                                              placeholder="Nombre visible corregido"
                                              autoComplete="off"
                                            />
                                          </div>
                                          <div className="space-y-1">
                                            <Label
                                              htmlFor={`privacy-exec-phone-${req.id}`}
                                            >
                                              Teléfono 9 dígitos (opcional)
                                            </Label>
                                            <Input
                                              id={`privacy-exec-phone-${req.id}`}
                                              value={execDraft.phone}
                                              disabled={execDraft.clearPhone}
                                              onChange={(e) =>
                                                updateExecutionDraft(
                                                  req.id,
                                                  "phone",
                                                  e.target.value,
                                                )
                                              }
                                              placeholder="912345678"
                                              autoComplete="off"
                                            />
                                          </div>
                                        </div>
                                        <div className="flex items-center gap-2">
                                          <input
                                            id={`privacy-exec-clear-phone-${req.id}`}
                                            type="checkbox"
                                            checked={execDraft.clearPhone}
                                            onChange={(e) =>
                                              updateExecutionDraft(
                                                req.id,
                                                "clearPhone",
                                                e.target.checked,
                                              )
                                            }
                                            className="h-4 w-4 rounded border border-input"
                                          />
                                          <Label
                                            htmlFor={`privacy-exec-clear-phone-${req.id}`}
                                          >
                                            Limpiar teléfono (enviar vacío)
                                          </Label>
                                        </div>
                                      </div>
                                    )}

                                    {selectedAction === "apply_opposition" && (
                                      <div className="grid gap-2 sm:grid-cols-2">
                                        <div className="space-y-1">
                                          <Label
                                            htmlFor={`privacy-exec-purpose-${req.id}`}
                                          >
                                            Finalidad
                                          </Label>
                                          <select
                                            id={`privacy-exec-purpose-${req.id}`}
                                            value={execDraft.purpose}
                                            onChange={(e) =>
                                              updateExecutionDraft(
                                                req.id,
                                                "purpose",
                                                e.target
                                                  .value as AdminPrivacyPurpose,
                                              )
                                            }
                                            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                                          >
                                            {PURPOSE_OPTIONS.map((opt) => (
                                              <option
                                                key={opt.value}
                                                value={opt.value}
                                              >
                                                {opt.label}
                                              </option>
                                            ))}
                                          </select>
                                        </div>
                                        <div className="space-y-1">
                                          <Label
                                            htmlFor={`privacy-exec-purpose-state-${req.id}`}
                                          >
                                            Estado de finalidad
                                          </Label>
                                          <select
                                            id={`privacy-exec-purpose-state-${req.id}`}
                                            value={execDraft.purposeState}
                                            onChange={(e) =>
                                              updateExecutionDraft(
                                                req.id,
                                                "purposeState",
                                                e.target.value as Extract<
                                                  AdminPrivacyPurposeState,
                                                  "opuesto" | "retirado"
                                                >,
                                              )
                                            }
                                            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                                          >
                                            <option value="opuesto">
                                              Opuesto
                                            </option>
                                            <option value="retirado">
                                              Retirado
                                            </option>
                                          </select>
                                        </div>
                                      </div>
                                    )}

                                    {selectedAction === "apply_block" && (
                                      <div className="space-y-1">
                                        <Label
                                          htmlFor={`privacy-exec-scope-${req.id}`}
                                        >
                                          Ámbito del bloqueo
                                        </Label>
                                        <select
                                          id={`privacy-exec-scope-${req.id}`}
                                          value={execDraft.scope}
                                          onChange={(e) =>
                                            updateExecutionDraft(
                                              req.id,
                                              "scope",
                                              e.target
                                                .value as AdminPrivacyTreatmentScope,
                                            )
                                          }
                                          className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                                        >
                                          {TREATMENT_SCOPE_OPTIONS.map(
                                            (opt) => (
                                              <option
                                                key={opt.value}
                                                value={opt.value}
                                              >
                                                {opt.label}
                                              </option>
                                            ),
                                          )}
                                        </select>
                                      </div>
                                    )}

                                    {selectedAction === "issue_portability" && (
                                      <div className="space-y-1">
                                        <Label
                                          htmlFor={`privacy-exec-target-ref-${req.id}`}
                                        >
                                          Referencia de destino (opcional)
                                        </Label>
                                        <Input
                                          id={`privacy-exec-target-ref-${req.id}`}
                                          value={execDraft.targetReference}
                                          onChange={(e) =>
                                            updateExecutionDraft(
                                              req.id,
                                              "targetReference",
                                              e.target.value,
                                            )
                                          }
                                          placeholder="Ej. entrega_titular_portal o destinatario"
                                          autoComplete="off"
                                        />
                                      </div>
                                    )}

                                    <div className="space-y-1">
                                      <Label
                                        htmlFor={`privacy-exec-summary-${req.id}`}
                                      >
                                        Resumen de ejecución
                                      </Label>
                                      <textarea
                                        id={`privacy-exec-summary-${req.id}`}
                                        value={execDraft.summary}
                                        onChange={(e) =>
                                          updateExecutionDraft(
                                            req.id,
                                            "summary",
                                            e.target.value,
                                          )
                                        }
                                        maxLength={MAX_EXECUTION_SUMMARY_LENGTH}
                                        className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                                        placeholder="Describe la ejecución aplicada (mín. 10 caracteres)"
                                      />
                                    </div>
                                    {executionErrors[req.id] && (
                                      <p
                                        role="alert"
                                        className="text-xs text-destructive"
                                      >
                                        {executionErrors[req.id]}
                                      </p>
                                    )}
                                    {showMfaHint && (
                                      <p className="text-xs text-muted-foreground">
                                        Se solicitará MFA reciente antes de
                                        ejecutar el derecho.
                                      </p>
                                    )}
                                    <Button
                                      type="submit"
                                      size="sm"
                                      disabled={executingRequestId === req.id}
                                    >
                                      {executingRequestId === req.id
                                        ? "Ejecutando…"
                                        : "Ejecutar derecho"}
                                    </Button>
                                  </form>
                                )}
                              </div>
                            );
                          })()}
                      </div>
                    )}
                    {canManagePrivacy && req.can_record_dispatch === true && (
                      <form
                        onSubmit={(event) => void handleRecordDispatch(event, req)}
                        className="space-y-2 rounded-md border border-border p-3"
                      >
                        <p className="text-sm font-medium">Registrar comprobante de despacho</p>
                        <p className="text-xs text-muted-foreground">
                          Registra un envío por email o correo postal que ya realizaste manualmente. Este formulario solo deja constancia; no envía mensajes ni correspondencia.
                        </p>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <div className="space-y-1">
                            <Label htmlFor={`privacy-dispatch-channel-${req.id}`}>Canal</Label>
                            <select
                              id={`privacy-dispatch-channel-${req.id}`}
                              value={dispatchDrafts[req.id]?.channel ?? "email"}
                              onChange={(event) => updateDispatchDraft(req.id, "channel", event.target.value)}
                              className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                            >
                              <option value="email">Email</option>
                              <option value="postal">Correo postal</option>
                            </select>
                          </div>
                          <div className="space-y-1">
                            <Label htmlFor={`privacy-dispatch-destination-${req.id}`}>Destino</Label>
                            <Input
                              id={`privacy-dispatch-destination-${req.id}`}
                              value={dispatchDrafts[req.id]?.destination ?? ""}
                              onChange={(event) => updateDispatchDraft(req.id, "destination", event.target.value)}
                              autoComplete="off"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label htmlFor={`privacy-dispatch-sent-at-${req.id}`}>Fecha y hora del envío</Label>
                            <Input
                              id={`privacy-dispatch-sent-at-${req.id}`}
                              type="datetime-local"
                              value={dispatchDrafts[req.id]?.sentAt ?? ""}
                              onChange={(event) => updateDispatchDraft(req.id, "sentAt", event.target.value)}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label htmlFor={`privacy-dispatch-evidence-${req.id}`}>Referencia de evidencia</Label>
                            <Input
                              id={`privacy-dispatch-evidence-${req.id}`}
                              value={dispatchDrafts[req.id]?.evidenceRef ?? ""}
                              onChange={(event) => updateDispatchDraft(req.id, "evidenceRef", event.target.value)}
                              autoComplete="off"
                            />
                          </div>
                        </div>
                        {dispatchError && (
                          <p role="alert" className="text-xs text-destructive">{dispatchError}</p>
                        )}
                        {showMfaHint && (
                          <p className="text-xs text-muted-foreground">Se solicitará MFA reciente antes de registrar el comprobante.</p>
                        )}
                        <Button type="submit" size="sm" disabled={dispatchingId === req.id}>
                          {dispatchingId === req.id ? "Registrando…" : "Registrar despacho realizado"}
                        </Button>
                      </form>
                    )}
                    {canManagePrivacy && canResolvePortalAccess(req) && (
                      <div className="space-y-2 rounded-md border border-border p-3">
                        <Label htmlFor={`privacy-resolution-${req.id}`}>
                          Respuesta para el titular
                        </Label>
                        <textarea
                          id={`privacy-resolution-${req.id}`}
                          value={resolutionDrafts[req.id]?.response ?? ""}
                          maxLength={MAX_RESPONSE_LENGTH}
                          onChange={(event) =>
                            updateResolutionDraft(
                              req.id,
                              "response",
                              event.target.value,
                            )
                          }
                          className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                          aria-describedby={`privacy-resolution-help-${req.id}`}
                        />
                        <p
                          id={`privacy-resolution-help-${req.id}`}
                          className="text-xs text-muted-foreground"
                        >
                          Registra la respuesta en EspaciGo. Esto no confirma
                          una ejecución de acciones en servicios externos.{" "}
                          {resolutionDrafts[req.id]?.response.length ?? 0}/
                          {MAX_RESPONSE_LENGTH}
                        </p>
                        <div className="flex flex-wrap items-center gap-2">
                          <select
                            aria-label="Decisión de privacidad"
                            value={
                              resolutionDrafts[req.id]?.decision ?? "resuelta"
                            }
                            onChange={(event) =>
                              updateResolutionDraft(
                                req.id,
                                "decision",
                                event.target.value,
                              )
                            }
                            className="h-9 rounded-md border border-input bg-background px-3 text-sm"
                          >
                            <option value="resuelta">Solicitud resuelta</option>
                            <option value="denegada_fundada">
                              Denegada con fundamento
                            </option>
                          </select>
                          <Button
                            size="sm"
                            disabled={resolvingId === req.id}
                            onClick={() => void handleResolve(req)}
                          >
                            {resolvingId === req.id
                              ? "Guardando…"
                              : "Registrar respuesta"}
                          </Button>
                        </div>
                        {resolutionError && (
                          <p role="alert" className="text-xs text-destructive">
                            {resolutionError}
                          </p>
                        )}
                        {showMfaHint && (
                          <p className="text-xs text-muted-foreground">
                            Se solicitará una verificación MFA reciente antes de
                            registrar la respuesta.
                          </p>
                        )}
                      </div>
                    )}
                  </div>
                ))}
                {privacyNextCursor && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => void loadMorePrivacy()}
                    disabled={loadingMorePrivacy}
                  >
                    {loadingMorePrivacy
                      ? "Cargando…"
                      : "Cargar más solicitudes"}
                  </Button>
                )}
              </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldCheck className="h-4 w-4" />
              Resumen de casos KYC (estado primero)
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1">
              <Label htmlFor="kyc-status-filter">Filtrar por estado KYC</Label>
              <select
                id="kyc-status-filter"
                value={kycStatusFilter}
                onChange={(e) => setKycStatusFilter(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="">Todos los estados</option>
                <option value="pendiente">Pendiente</option>
                <option value="en_revision">En revisión</option>
                <option value="aprobado">Aprobado</option>
                <option value="rechazado">Rechazado</option>
              </select>
            </div>

            {!canReadKyc ? (
              <p className="text-sm text-muted-foreground">
                No cuentas con el permiso <code>kyc.case.read</code>.
              </p>
            ) : kycCases.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No hay casos KYC para los filtros seleccionados.
              </p>
            ) : (
              <div className="space-y-2">
                {kycCases.map((item) => (
                  <div
                    key={item.id}
                    className="rounded-lg border border-border p-3 space-y-1"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <Badge
                          variant={
                            item.status === "aprobado"
                              ? "default"
                              : item.status === "rechazado"
                                ? "destructive"
                                : "secondary"
                          }
                        >
                          {item.status}
                        </Badge>
                        <Badge variant="outline">
                          Decisión: {item.decision_status}
                        </Badge>
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {item.environment} · {item.id_country}
                      </span>
                    </div>
                    <p className="font-mono text-xs text-muted-foreground">
                      Cuenta: {item.account_id}
                    </p>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <AdminPrivacyPhase7Section
        mfaStepUpRequired={snapshot.mfaStepUpRequired}
        recentAuthExpiresAt={snapshot.adminSession?.recent_auth_expires_at}
        showMfaHint={showMfaHint}
        openMfaStepUp={openMfaStepUp}
        listGlobalPrivacyIncidents={listGlobalPrivacyIncidents}
        getGlobalPrivacyIncident={getGlobalPrivacyIncident}
        createGlobalPrivacyIncident={createGlobalPrivacyIncident}
        transitionGlobalPrivacyIncident={transitionGlobalPrivacyIncident}
        listGlobalPrivacyLegalHolds={listGlobalPrivacyLegalHolds}
        createGlobalPrivacyLegalHold={createGlobalPrivacyLegalHold}
        releaseGlobalPrivacyLegalHold={releaseGlobalPrivacyLegalHold}
      />
    </div>
  );
}
