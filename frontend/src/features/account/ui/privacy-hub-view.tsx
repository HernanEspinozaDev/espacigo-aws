"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { DocumentReviewView } from "@/features/document-review/ui/document-review-view";
import { DocumentVerificationView } from "@/features/document-verification/ui/document-verification-view";
import { cn } from "@/lib/utils";
import { PrivacyCard } from "./privacy-card";
import { useAccount } from "./use-account";

export type PrivacyTab = "privacidad" | "documentos" | "revision";

const TABS: ReadonlyArray<{ id: PrivacyTab; label: string }> = [
  { id: "privacidad", label: "Privacidad" },
  { id: "documentos", label: "Documentos" },
  { id: "revision", label: "Revisión documental" },
];

export function resolvePrivacyTab(raw?: string | null): PrivacyTab {
  if (raw === "documentos") return "documentos";
  if (raw === "revision") return "revision";
  return "privacidad";
}

export function PrivacyHubView({ initialTab }: { initialTab?: string }) {
  const resolvedInitial = resolvePrivacyTab(initialTab);
  return (
    <PrivacyHubContent key={resolvedInitial} initialTab={resolvedInitial} />
  );
}

function PrivacyHubContent({ initialTab }: { initialTab: PrivacyTab }) {
  const [activeTab, setActiveTab] = useState<PrivacyTab>(initialTab);
  const { snapshot: authSnapshot } = useAuth();
  const { loadOverview } = useAccount();

  useEffect(() => {
    if (authSnapshot.status === "authenticated") {
      loadOverview().catch(() => {});
    }
  }, [authSnapshot.status, loadOverview]);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-1.5 text-sm text-muted-foreground"
      >
        <Link
          href="/mi-cuenta"
          className="font-medium text-primary hover:underline"
        >
          General
        </Link>
        <ChevronRight className="size-4 shrink-0" aria-hidden="true" />
        <span aria-current="page" className="font-medium text-foreground">
          Privacidad
        </span>
      </nav>

      <div>
        <h2 className="text-2xl font-bold tracking-tight text-foreground">
          Privacidad
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Gestiona tus derechos ARCO, expedientes documentales privados y
          revisiones asignadas.
        </p>
      </div>

      <div
        role="tablist"
        aria-label="Secciones de privacidad"
        className="flex flex-wrap gap-2 border-b border-border pb-3"
      >
        {TABS.map((tab) => {
          const selected = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              id={`privacy-tab-${tab.id}`}
              type="button"
              role="tab"
              aria-selected={selected}
              aria-controls={`privacy-panel-${tab.id}`}
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                "inline-flex min-h-10 items-center rounded-lg px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                selected
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted/60 text-foreground/80 hover:bg-muted hover:text-foreground",
              )}
            >
              {tab.label}
            </button>
          );
        })}
      </div>

      <div
        id={`privacy-panel-${activeTab}`}
        role="tabpanel"
        aria-labelledby={`privacy-tab-${activeTab}`}
      >
        {activeTab === "privacidad" ? <PrivacyCard /> : null}
        {activeTab === "documentos" ? <DocumentVerificationView /> : null}
        {activeTab === "revision" ? <DocumentReviewView /> : null}
      </div>
    </div>
  );
}
