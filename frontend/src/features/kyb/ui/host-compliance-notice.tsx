"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { AlertCircle, Clock3 } from "lucide-react";
import { useAuth } from "@/features/auth/ui/use-auth";

type Compliance = {
  compliance_status?: string;
  grace_started_at?: string;
  grace_due_at?: string;
};
type OrganizationCompliance = {
  id: string;
  legal_name: string;
  trade_name?: string;
  compliance_status?: string;
  grace_due_at?: string;
};

export type HostComplianceProfile = Compliance;

export function useHostCompliance() {
  const { snapshot, accessToken } = useAuth();
  const accountId =
    snapshot.status === "authenticated" ? snapshot.accountId : null;
  const [result, setResult] = useState<{
    accountId: string;
    profile: Compliance;
  } | null>(null);
  const profile =
    result?.accountId === accountId ? (result?.profile ?? null) : null;

  useEffect(() => {
    if (!accountId || !accessToken) return;
    const controller = new AbortController();
    void fetch("/api/backend/kyb/personal/compliance", {
      credentials: "same-origin",
      cache: "no-store",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) return null;
        return (await response.json()) as Compliance;
      })
      .then((value) => {
        if (!controller.signal.aborted)
          setResult(value ? { accountId, profile: value } : null);
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult(null);
      });
    return () => controller.abort();
  }, [accountId, accessToken]);

  return {
    profile,
    disabled: profile?.compliance_status === "disabled_missing_docs",
  };
}

function daysRemaining(dueAt?: string) {
  if (!dueAt) return null;
  return Math.max(0, Math.ceil((Date.parse(dueAt) - Date.now()) / 86_400_000));
}

/** Optional account-level notice. A missing or unavailable endpoint must not
 * block host navigation, draft creation, uploads, or tenant activity. */
export function HostComplianceNotice({
  profile,
}: {
  profile: HostComplianceProfile | null;
}) {
  const { snapshot, accessToken } = useAuth();
  const accountId =
    snapshot.status === "authenticated" ? snapshot.accountId : null;
  const [result, setResult] = useState<{
    accountId: string;
    organizations: OrganizationCompliance[];
  } | null>(null);
  const organizations =
    result?.accountId === accountId ? (result?.organizations ?? []) : [];
  useEffect(() => {
    if (!accountId || !accessToken) return;
    const controller = new AbortController();
    void fetch("/api/backend/host/organizations", {
      credentials: "same-origin",
      cache: "no-store",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) return { items: [] as OrganizationCompliance[] };
        return (await response.json()) as { items?: OrganizationCompliance[] };
      })
      .then((result) => {
        if (!controller.signal.aborted)
          setResult({ accountId, organizations: result.items ?? [] });
      })
      .catch(() => {
        if (!controller.signal.aborted) setResult(null);
      });
    return () => controller.abort();
  }, [accountId, accessToken]);

  const notices = [
    ...(profile ? [{ id: "personal", label: "Perfil personal", profile }] : []),
    ...organizations.map((organization) => ({
      id: organization.id,
      label: organization.trade_name || organization.legal_name || "Empresa",
      profile: organization,
    })),
  ];
  const visible = notices.filter(({ profile: item }) => {
    const disabled = item.compliance_status === "disabled_missing_docs";
    const days = daysRemaining(item.grace_due_at);
    return (
      disabled || (item.compliance_status !== "compliant" && days !== null)
    );
  });
  if (visible.length === 0) return null;

  return (
    <div className="space-y-3">
      {visible.map(({ id, label, profile: item }) => (
        <ComplianceNoticeCard key={id} label={label} profile={item} />
      ))}
    </div>
  );
}

function ComplianceNoticeCard({
  label,
  profile,
}: {
  label: string;
  profile: Compliance;
}) {
  const disabled = profile.compliance_status === "disabled_missing_docs";
  const days = daysRemaining(profile.grace_due_at);
  return (
    <section
      role={disabled ? "alert" : "status"}
      className={`flex items-start gap-3 rounded-lg border p-4 text-sm ${disabled ? "border-red-200 bg-red-50 text-red-950" : "border-amber-200 bg-amber-50 text-amber-950"}`}
    >
      {disabled ? (
        <AlertCircle className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
      ) : (
        <Clock3 className="mt-0.5 size-5 shrink-0" aria-hidden="true" />
      )}
      <div className="flex-1 space-y-1">
        <p className="font-semibold">
          {label}:{" "}
          {disabled
            ? "publicaciones y administración pausadas por documentación pendiente"
            : `quedan ${days} ${days === 1 ? "día" : "días"} del plazo de gracia`}
        </p>
        <p>
          {disabled
            ? "Envía los documentos pendientes para solicitar la reactivación. Tu acceso como arrendatario sigue habilitado."
            : "Completa la documentación antes de la fecha límite para mantener habilitadas las publicaciones."}
        </p>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          <Link
            className="font-medium underline underline-offset-2"
            href="/anfitrion/kyb"
          >
            Revisar documentación
          </Link>
          {disabled ? (
            <>
              <Link
                className="font-medium underline underline-offset-2"
                href="/reservas"
              >
                Seguir usando EspaciGo como arrendatario
              </Link>
              <Link
                className="font-medium underline underline-offset-2"
                href="/mi-cuenta/privacidad"
              >
                Solicitar cierre de cuenta
              </Link>
            </>
          ) : null}
        </div>
      </div>
    </section>
  );
}
