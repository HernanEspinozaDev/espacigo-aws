"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  createContext,
  useContext,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  getCookiePreferences,
  saveCookiePreferences,
  subscribeToCookiePreferences,
} from "./preferences";

const getServerPreference = () => undefined;
const OpenPreferences = createContext<(trigger: HTMLButtonElement) => void>(
  () => {},
);

export function CookiePreferencesProvider({
  children,
}: {
  children: ReactNode;
}) {
  const choice = useSyncExternalStore(
    subscribeToCookiePreferences,
    getCookiePreferences,
    getServerPreference,
  );
  const pathname = usePathname();
  const viewingPolicy =
    pathname === "/politica-de-privacidad" ||
    pathname === "/terminos-y-condiciones";
  const awaitingChoice = choice === null && !viewingPolicy;
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [visitedSpaces, setVisitedSpaces] = useState(false);
  const opener = useRef<HTMLButtonElement | null>(null);

  function openPreferences(trigger: HTMLButtonElement) {
    opener.current = trigger;
    setVisitedSpaces(choice === "visited-spaces");
    setSettingsOpen(true);
  }

  function save(allowVisitedSpaces: boolean) {
    saveCookiePreferences(allowVisitedSpaces);
    setSettingsOpen(false);
  }

  return (
    <OpenPreferences.Provider value={openPreferences}>
      {children}
      <Dialog
        open={awaitingChoice || settingsOpen}
        onOpenChange={(open) => {
          if (!awaitingChoice) setSettingsOpen(open);
        }}
        disablePointerDismissal={awaitingChoice}
      >
        <DialogContent
          showCloseButton={!awaitingChoice}
          className="top-auto right-4 bottom-4 left-auto max-h-[calc(100dvh-2rem)] translate-x-0 translate-y-0 overflow-y-auto"
          finalFocus={() =>
            opener.current?.isConnected
              ? opener.current
              : document.getElementById("main-content")
          }
        >
          <DialogHeader>
            <DialogTitle>
              {awaitingChoice
                ? "Privacidad y cookies"
                : "Preferencias de cookies"}
            </DialogTitle>
            <DialogDescription>
              El sitio usa almacenamiento necesario para funcionar. Tú decides
              si esta pestaña puede recordar los espacios que visitas.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {awaitingChoice && (
              <div className="flex flex-wrap gap-x-4 gap-y-1">
                <Link
                  className="underline underline-offset-2"
                  href="/politica-de-privacidad"
                >
                  Política de privacidad
                </Link>
                <Link
                  className="underline underline-offset-2"
                  href="/terminos-y-condiciones"
                >
                  Términos y condiciones
                </Link>
              </div>
            )}
            <label className="flex items-start gap-3">
              <input
                className="mt-1 size-4 accent-primary"
                type="checkbox"
                checked={visitedSpaces}
                onChange={(event) => setVisitedSpaces(event.target.checked)}
              />
              <span>
                <span className="block font-medium">
                  Recordar espacios visitados
                </span>
                <span className="text-muted-foreground">
                  Puedes cambiar esta elección desde el pie de página.
                </span>
              </span>
            </label>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => save(false)}>
              Rechazar opcionales
            </Button>
            <Button type="button" onClick={() => save(visitedSpaces)}>
              {visitedSpaces ? "Aceptar opcionales" : "Guardar preferencias"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </OpenPreferences.Provider>
  );
}

export function CookiePreferencesButton() {
  const openPreferences = useContext(OpenPreferences);
  return (
    <button
      type="button"
      className="text-sm text-[var(--brand-muted-on-subtle)] hover:text-primary"
      onClick={(event) => openPreferences(event.currentTarget)}
    >
      Preferencias de cookies
    </button>
  );
}
