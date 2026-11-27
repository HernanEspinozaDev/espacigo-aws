"use client";

import Link from "next/link";
import {
  createContext,
  useContext,
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
const OpenPreferences = createContext<() => void>(() => {});

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
  const [open, setOpen] = useState(false);
  const [visitedSpaces, setVisitedSpaces] = useState(false);

  function openPreferences() {
    setVisitedSpaces(choice === "visited-spaces");
    setOpen(true);
  }

  function save(allowVisitedSpaces: boolean) {
    saveCookiePreferences(allowVisitedSpaces);
    setOpen(false);
  }

  return (
    <OpenPreferences.Provider value={openPreferences}>
      {choice === null && !open && (
        <aside
          aria-label="Aviso de privacidad y cookies"
          className="border-b border-border bg-[var(--brand-surface-subtle)] text-foreground"
        >
          <div className="mx-auto max-w-7xl px-4 py-3 sm:px-6 lg:px-8">
            <h2 className="text-base font-semibold">Privacidad y cookies</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              El sitio usa almacenamiento necesario para funcionar. Si lo
              permites, recordaremos en esta pestaña los espacios que visitas.
            </p>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
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
            <div className="mt-3 flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => save(true)}
              >
                Aceptar opcionales
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => save(false)}
              >
                Rechazar opcionales
              </Button>
              <Button type="button" variant="ghost" onClick={openPreferences}>
                Configurar
              </Button>
            </div>
          </div>
        </aside>
      )}
      {children}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Preferencias de cookies</DialogTitle>
            <DialogDescription>
              Esta elección se aplica en este navegador. Puedes cambiarla cuando
              quieras.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <p className="font-medium">Necesarias · siempre activas</p>
              <p className="text-muted-foreground">
                Mantienen la sesión, la seguridad y la continuidad de una
                gestión.
              </p>
            </div>
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
                  EspaciGo guarda esta lista en la pestaña mientras está
                  abierta. Puedes borrarla desactivando la opción.
                </span>
              </span>
            </label>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => save(false)}>
              Rechazar opcionales
            </Button>
            <Button type="button" onClick={() => save(visitedSpaces)}>
              Guardar preferencias
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
      onClick={openPreferences}
    >
      Preferencias de cookies
    </button>
  );
}
