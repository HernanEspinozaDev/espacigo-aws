"use client";

import { useState } from "react";
import Link from "next/link";
import {
  CheckCircle2,
  ChevronRight,
  Info,
  Plus,
  ShieldCheck,
  Trash2,
  UserCheck,
  Users,
} from "lucide-react";
import { maskEmail, maskPhone } from "@/components/reveal-email";

interface TrustedContact {
  id: string;
  name: string;
  email: string;
  phone: string;
  relationship: string;
}

export default function PersonasConfianzaPage() {
  const [contacts, setContacts] = useState<TrustedContact[]>([
    {
      id: "tc-1",
      name: "Contacto de respaldo",
      email: "familiar@espacigo.cl",
      phone: "+56987654321",
      relationship: "Familiar directo",
    },
  ]);

  const [showAddModal, setShowAddModal] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [relationship, setRelationship] = useState("");
  const [notification, setNotification] = useState<string | null>(null);

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !email.trim()) return;

    const newContact: TrustedContact = {
      id: `tc-${Date.now()}`,
      name: name.trim(),
      email: email.trim(),
      phone: phone.trim() || "+56900000000",
      relationship: relationship.trim() || "Contacto de confianza",
    };

    setContacts((prev) => [...prev, newContact]);
    setName("");
    setEmail("");
    setPhone("");
    setRelationship("");
    setShowAddModal(false);
    setNotification("Contacto de confianza agregado exitosamente.");
    setTimeout(() => setNotification(null), 4000);
  };

  const handleRemove = (id: string) => {
    setContacts((prev) => prev.filter((c) => c.id !== id));
    setNotification("El contacto de confianza fue retirado.");
    setTimeout(() => setNotification(null), 4000);
  };

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground"
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
        <span className="text-foreground font-semibold">
          Personas de confianza
        </span>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          Personas de confianza
        </h1>
        <p className="text-sm text-muted-foreground">
          Contactos de respaldo que pueden ayudarte a recuperar el acceso a tu
          cuenta en caso de emergencia o pérdida de tus dispositivos.
        </p>
      </div>

      {notification && (
        <div
          role="status"
          className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs sm:text-sm flex items-center gap-2"
        >
          <CheckCircle2 className="size-4 shrink-0" />
          <span>{notification}</span>
        </div>
      )}

      {/* Card principal estilo Mercado Pago */}
      <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 shadow-sm space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 border-b border-border/60">
          <div className="flex items-center gap-4">
            <div className="size-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
              <Users className="size-6" />
            </div>
            <div>
              <h2 className="text-base font-bold text-foreground">
                Tus contactos asignados ({contacts.length}/3)
              </h2>
              <p className="text-xs text-muted-foreground">
                Máximo 3 personas de confianza autorizadas para validar tu identidad.
              </p>
            </div>
          </div>

          {contacts.length < 3 && (
            <button
              type="button"
              onClick={() => setShowAddModal(true)}
              className="inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 font-semibold text-xs sm:text-sm transition-colors shadow-sm"
            >
              <Plus className="size-4" />
              Agregar persona
            </button>
          )}
        </div>

        {contacts.length === 0 ? (
          <p className="text-xs sm:text-sm text-muted-foreground text-center py-6">
            Aún no has configurado personas de confianza.
          </p>
        ) : (
          <div className="divide-y divide-border/60">
            {contacts.map((contact) => (
              <div
                key={contact.id}
                className="py-4 flex items-center justify-between gap-4"
              >
                <div className="flex items-center gap-3.5 min-w-0">
                  <div className="size-10 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                    <UserCheck className="size-5" />
                  </div>
                  <div className="space-y-0.5 min-w-0">
                    <p className="text-sm font-semibold text-foreground">
                      {contact.name}{" "}
                      <span className="text-xs font-normal text-muted-foreground">
                        ({contact.relationship})
                      </span>
                    </p>
                    <p className="text-xs font-mono text-muted-foreground">
                      E-mail: {maskEmail(contact.email)} • Tel:{" "}
                      {maskPhone(contact.phone)}
                    </p>
                  </div>
                </div>

                <button
                  type="button"
                  onClick={() => handleRemove(contact.id)}
                  aria-label={`Eliminar contacto ${contact.name}`}
                  className="text-muted-foreground hover:text-destructive p-2 rounded-lg hover:bg-muted transition-colors"
                >
                  <Trash2 className="size-4" />
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="pt-2 border-t border-border/50 text-xs text-muted-foreground flex items-start gap-2">
          <ShieldCheck className="size-4 text-emerald-600 shrink-0 mt-0.5" />
          <p>
            Tus personas de confianza nunca tendrán acceso a ver tus fondos,
            espacios, mensajes ni contraseñas. Solo recibirán un código de
            recuperación si tú lo solicitas expresamente.
          </p>
        </div>
      </div>

      {/* Modal para agregar contacto */}
      {showAddModal && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-md w-full p-6 shadow-xl space-y-4">
            <h3 className="text-lg font-bold text-foreground">
              Nueva persona de confianza
            </h3>
            <p className="text-xs text-muted-foreground">
              Ingresa los datos de la persona que te asistirá en caso de pérdida
              de acceso.
            </p>

            <form onSubmit={handleAdd} className="space-y-3">
              <div>
                <label
                  htmlFor="contactName"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Nombre completo
                </label>
                <input
                  id="contactName"
                  type="text"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Ej. Constanza Silva"
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>

              <div>
                <label
                  htmlFor="contactRel"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Parentesco o relación
                </label>
                <input
                  id="contactRel"
                  type="text"
                  value={relationship}
                  onChange={(e) => setRelationship(e.target.value)}
                  placeholder="Ej. Hermano, Cónyuge, Socio"
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>

              <div>
                <label
                  htmlFor="contactEmail"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Correo electrónico
                </label>
                <input
                  id="contactEmail"
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="correo@ejemplo.cl"
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>

              <div>
                <label
                  htmlFor="contactPhone"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Teléfono celular
                </label>
                <input
                  id="contactPhone"
                  type="tel"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  placeholder="+56 9 1234 5678"
                  className="w-full px-3.5 py-2 rounded-xl bg-background border border-input text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-4 py-2 text-xs font-semibold rounded-xl border border-input hover:bg-muted transition-colors text-foreground"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
                >
                  Guardar contacto
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Nota de fase */}
      <div className="p-4 rounded-xl bg-muted/40 border border-border/60 text-xs text-muted-foreground flex items-start gap-3">
        <Info className="size-4 text-primary shrink-0 mt-0.5" />
        <p>
          <strong>Plan de seguridad EspaciGo (Fase 7):</strong> Las personas de
          confianza se registrarán con desafíos criptográficos de doble
          confirmación para evitar suplantaciones.
        </p>
      </div>
    </div>
  );
}
