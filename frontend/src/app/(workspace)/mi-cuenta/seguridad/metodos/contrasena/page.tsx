"use client";

import { useState } from "react";
import Link from "next/link";
import {
  CheckCircle2,
  ChevronRight,
  KeyRound,
  Lock,
  PenLine,
  ShieldCheck,
} from "lucide-react";
import { SecurityCard } from "@/features/account/ui/security-card";

export default function ContrasenaMetodoPage() {
  const [isEditing, setIsEditing] = useState(false);

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground flex-wrap"
      >
        <Link
          href="/mi-cuenta"
          className="hover:text-primary transition-colors font-medium"
        >
          Mi perfil
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad"
          className="hover:text-primary transition-colors font-medium"
        >
          Seguridad
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad/metodos"
          className="hover:text-primary transition-colors font-medium"
        >
          Métodos de verificación
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="text-foreground font-semibold">Contraseña</span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Contraseña
        </h1>
        <p className="text-sm text-muted-foreground">
          Gestiona tu clave principal de acceso a EspaciGo.
        </p>
      </div>

      {/* Tarjeta de estado estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 border-b border-border/60">
          <div className="flex items-center gap-4">
            <div className="size-12 rounded-2xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
              <Lock className="size-6" />
            </div>
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <h2 className="text-lg font-bold text-foreground">
                  Contraseña de acceso
                </h2>
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3.5" />
                  Activa
                </span>
              </div>
              <p className="text-base font-mono tracking-widest text-foreground font-semibold">
                ••••••••••••••••••
              </p>
            </div>
          </div>

          <div>
            <button
              type="button"
              onClick={() => setIsEditing((prev) => !prev)}
              className="inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 font-semibold text-xs sm:text-sm transition-colors w-full sm:w-auto shadow-sm"
            >
              <PenLine className="size-4" />
              {isEditing ? "Cerrar edición" : "Modificar"}
            </button>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs sm:text-sm text-muted-foreground">
          <div className="flex items-start gap-3">
            <ShieldCheck className="size-5 text-emerald-600 shrink-0 mt-0.5" />
            <p>
              Tu contraseña está protegida con cifrado bcrypt y hash con sal en
              PostgreSQL de acuerdo con las políticas A.8.11 y A.8.24.
            </p>
          </div>
          <div className="flex items-start gap-3">
            <KeyRound className="size-5 text-primary shrink-0 mt-0.5" />
            <p>
              Recomendamos actualizar tu clave si sospechas que pudo ser
              comprometida o compartida en un dispositivo público.
            </p>
          </div>
        </div>
      </div>

      {/* Formulario real SecurityCard */}
      <section
        aria-label="Formulario de actualización de contraseña"
        className={isEditing ? "block" : "space-y-4"}
      >
        <div className="flex items-center justify-between pb-2">
          <h3 className="text-base font-bold text-foreground">
            Formulario de cambio de contraseña
          </h3>
          {!isEditing && (
            <span className="text-xs text-muted-foreground">
              Haz clic en &ldquo;Modificar&rdquo; arriba o utiliza los campos a
              continuación.
            </span>
          )}
        </div>
        <SecurityCard />
      </section>
    </div>
  );
}
