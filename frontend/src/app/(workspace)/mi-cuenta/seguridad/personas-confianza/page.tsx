"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Info,
  Loader2,
  Mail,
  Phone,
  Plus,
  ShieldCheck,
  Trash2,
  UserCheck,
  Users,
} from "lucide-react";
import { useTrustedContacts } from "@/features/account/ui/use-trusted-contacts";

export default function PersonasConfianzaPage() {
  const {
    contacts,
    isLoading,
    isAdding,
    removingContactId,
    error,
    notification,
    addContact,
    removeContact,
    clearError,
    clearNotification,
  } = useTrustedContacts();

  const [showAddModal, setShowAddModal] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [relationship, setRelationship] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const [contactToDelete, setContactToDelete] = useState<{
    id: string;
    name: string;
  } | null>(null);

  const resetForm = () => {
    setName("");
    setEmail("");
    setPhone("");
    setRelationship("");
    setFormError(null);
  };

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    clearError();
    clearNotification();
    setFormError(null);

    const trimmedName = name.trim();
    const trimmedEmail = email.trim();
    const trimmedRelationship = relationship.trim();
    const trimmedPhone = phone.trim();

    if (trimmedName.length < 2 || trimmedName.length > 80) {
      setFormError("El nombre completo debe tener entre 2 y 80 caracteres.");
      return;
    }

    if (trimmedRelationship.length < 2 || trimmedRelationship.length > 40) {
      setFormError("El parentesco o relación debe tener entre 2 y 40 caracteres.");
      return;
    }

    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(trimmedEmail)) {
      setFormError("Ingresa un correo electrónico válido.");
      return;
    }

    if (trimmedPhone && trimmedPhone.length < 8) {
      setFormError("Ingresa un número telefónico chileno válido (ej. +56912345678).");
      return;
    }

    const success = await addContact({
      full_name: trimmedName,
      email: trimmedEmail,
      relationship: trimmedRelationship,
      phone: trimmedPhone || undefined,
    });

    if (success) {
      resetForm();
      setShowAddModal(false);
    }
  };

  const handleConfirmDelete = async () => {
    if (!contactToDelete) return;
    clearError();
    clearNotification();
    const id = contactToDelete.id;
    setContactToDelete(null);
    await removeContact(id);
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
          className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2">
            <CheckCircle2 className="size-4 shrink-0" />
            <span>{notification}</span>
          </div>
          <button
            type="button"
            onClick={clearNotification}
            className="text-xs hover:underline"
          >
            Cerrar
          </button>
        </div>
      )}

      {error && (
        <div
          role="alert"
          className="p-4 rounded-xl bg-destructive/10 border border-destructive/20 text-destructive text-xs sm:text-sm flex items-center justify-between gap-2"
        >
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" />
            <span>{error}</span>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="text-xs hover:underline"
          >
            Cerrar
          </button>
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
                Máximo 3 personas de confianza autorizadas para asistir en la recuperación.
              </p>
            </div>
          </div>

          {contacts.length < 3 ? (
            <button
              type="button"
              onClick={() => {
                resetForm();
                setShowAddModal(true);
              }}
              className="inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 font-semibold text-xs sm:text-sm transition-colors shadow-sm"
            >
              <Plus className="size-4" />
              Agregar persona
            </button>
          ) : (
            <span className="text-xs font-medium text-amber-600 bg-amber-500/10 px-3 py-1 rounded-full border border-amber-500/20">
              Límite de 3 contactos alcanzado
            </span>
          )}
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-10 gap-2 text-muted-foreground text-sm">
            <Loader2 className="size-5 animate-spin text-primary" />
            <span>Cargando personas de confianza...</span>
          </div>
        ) : contacts.length === 0 ? (
          <div className="text-center py-8 space-y-2">
            <p className="text-sm font-medium text-foreground">
              Aún no has configurado personas de confianza.
            </p>
            <p className="text-xs text-muted-foreground max-w-md mx-auto">
              Te recomendamos agregar al menos un contacto cercano para ayudarte a
              recuperar tu acceso en situaciones imprevistas.
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border/60">
            {contacts.map((contact) => {
              const isDeletingThis = removingContactId === contact.id;
              return (
                <div
                  key={contact.id}
                  className="py-4 flex items-center justify-between gap-4"
                >
                  <div className="flex items-center gap-3.5 min-w-0">
                    <div className="size-10 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                      <UserCheck className="size-5" />
                    </div>
                    <div className="space-y-0.5 min-w-0">
                      <p className="text-sm font-semibold text-foreground truncate">
                        {contact.full_name}{" "}
                        <span className="text-xs font-normal text-muted-foreground">
                          ({contact.relationship})
                        </span>
                      </p>
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-mono text-muted-foreground">
                        <span className="inline-flex items-center gap-1">
                          <Mail className="size-3 text-muted-foreground/70" />
                          {contact.masked_email}
                        </span>
                        {contact.masked_phone && (
                          <span className="inline-flex items-center gap-1">
                            <Phone className="size-3 text-muted-foreground/70" />
                            {contact.masked_phone}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>

                  <button
                    type="button"
                    onClick={() =>
                      setContactToDelete({
                        id: contact.id,
                        name: contact.full_name,
                      })
                    }
                    disabled={Boolean(removingContactId)}
                    aria-label={`Eliminar contacto ${contact.full_name}`}
                    className="text-muted-foreground hover:text-destructive p-2 rounded-lg hover:bg-muted transition-colors disabled:opacity-50"
                  >
                    {isDeletingThis ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Trash2 className="size-4" />
                    )}
                  </button>
                </div>
              );
            })}
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

      {/* Modal de confirmación de eliminación */}
      {contactToDelete && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
        >
          <div className="bg-card border border-border rounded-2xl max-w-sm w-full p-6 shadow-xl space-y-4">
            <h3 className="text-base font-bold text-foreground">
              ¿Eliminar persona de confianza?
            </h3>
            <p className="text-xs text-muted-foreground">
              ¿Estás seguro de que deseas revocar a{" "}
              <strong className="text-foreground">{contactToDelete.name}</strong>{" "}
              como persona de confianza? Ya no podrá validar tu identidad en
              recuperaciones de emergencia.
            </p>
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setContactToDelete(null)}
                className="px-4 py-2 text-xs font-semibold rounded-xl border border-input hover:bg-muted transition-colors text-foreground"
              >
                Cancelar
              </button>
              <button
                type="button"
                onClick={handleConfirmDelete}
                className="px-4 py-2 text-xs font-semibold rounded-xl bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors"
              >
                Eliminar contacto
              </button>
            </div>
          </div>
        </div>
      )}

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

            {formError && (
              <div
                role="alert"
                className="p-3 rounded-xl bg-destructive/10 border border-destructive/20 text-destructive text-xs flex items-center gap-2"
              >
                <AlertCircle className="size-4 shrink-0" />
                <span>{formError}</span>
              </div>
            )}

            <form onSubmit={handleAdd} className="space-y-3">
              <div>
                <label
                  htmlFor="contactName"
                  className="block text-xs font-semibold text-foreground mb-1"
                >
                  Nombre completo <span className="text-destructive">*</span>
                </label>
                <input
                  id="contactName"
                  type="text"
                  required
                  maxLength={80}
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
                  Parentesco o relación <span className="text-destructive">*</span>
                </label>
                <input
                  id="contactRel"
                  type="text"
                  required
                  maxLength={40}
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
                  Correo electrónico <span className="text-destructive">*</span>
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
                  Teléfono celular (opcional)
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
                  disabled={isAdding}
                  className="px-4 py-2 text-xs font-semibold rounded-xl border border-input hover:bg-muted transition-colors text-foreground disabled:opacity-50"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  disabled={isAdding}
                  className="inline-flex items-center justify-center gap-1.5 px-4 py-2 text-xs font-semibold rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                >
                  {isAdding ? (
                    <>
                      <Loader2 className="size-3.5 animate-spin" />
                      <span>Guardando...</span>
                    </>
                  ) : (
                    <span>Guardar contacto</span>
                  )}
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
          confianza se registran de forma segura y sus datos de contacto se
          mantienen enmascarados criptográficamente.
        </p>
      </div>
    </div>
  );
}
