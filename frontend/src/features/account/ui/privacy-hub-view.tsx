"use client";

import { useEffect, useSyncExternalStore } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Tabs } from "radix-ui";
import { useAuth } from "@/features/auth/ui/use-auth";
import { LegalAcceptancesCard } from "./legal-acceptances-card";
import { PrivacyCard, type PrivacySection } from "./privacy-card";
import { useAccount } from "./use-account";

const sections: ReadonlyArray<{ id: PrivacySection | "legal"; label: string }> =
  [
    { id: "privacidad", label: "Solicitudes de privacidad" },
    { id: "exportacion", label: "Exportación de datos personales" },
    { id: "preferencias", label: "Preferencias de finalidad" },
    { id: "bloqueos", label: "Bloqueos temporales" },
    { id: "legal", label: "Tus documentos legales" },
  ];

const desktop = "(min-width: 48rem)";
function subscribeToViewport(onChange: () => void) {
  const media = window.matchMedia(desktop);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}

export function PrivacyHubView({ initialTab }: { initialTab?: string }) {
  const firstTab = sections.some((section) => section.id === initialTab)
    ? initialTab
    : "privacidad";
  const vertical = useSyncExternalStore(
    subscribeToViewport,
    () => window.matchMedia(desktop).matches,
    () => false,
  );
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

      <h2 className="text-2xl font-bold tracking-tight">Privacidad</h2>

      <Tabs.Root
        defaultValue={firstTab}
        orientation={vertical ? "vertical" : "horizontal"}
        className="grid min-w-0 gap-4 md:grid-cols-[12rem_minmax(0,1fr)] lg:grid-cols-[14rem_minmax(0,1fr)]"
      >
        <Tabs.List
          aria-label="Secciones de privacidad"
          className="flex min-w-0 gap-1 overflow-x-auto rounded-xl border p-1 md:flex-col md:self-start"
        >
          {sections.map((section) => (
            <Tabs.Trigger
              key={section.id}
              value={section.id}
              className="min-h-10 shrink-0 rounded-lg px-3 py-2 text-left text-sm font-medium text-muted-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring data-[state=active]:bg-primary data-[state=active]:text-primary-foreground"
            >
              {section.label}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
        {sections.map((section) => (
          <Tabs.Content key={section.id} value={section.id} className="min-w-0">
            {section.id === "legal" ? (
              authSnapshot.status === "authenticated" &&
              authSnapshot.accountId && (
                <LegalAcceptancesCard accountId={authSnapshot.accountId} />
              )
            ) : (
              <PrivacyCard section={section.id} />
            )}
          </Tabs.Content>
        ))}
      </Tabs.Root>
    </div>
  );
}
