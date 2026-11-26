"use client";

import Link from "next/link";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Loader2,
  Pencil,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError } from "@/lib/api/errors";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  formatChileanRutAtCursor,
  normalizeChileanE164,
  normalizeChileanRut,
  normalizeUsername,
  validateIdentityProfile,
  type IdentityProfileErrors,
  type IdentityProfileInput,
  type IdentityProfileState,
} from "../domain/identity-profile";
import { useIdentityProfile } from "./use-identity-profile";
import { PinpassAccountSection } from "@/features/account/ui/pinpass-card";
import type { components } from "@/lib/api/generated/openapi";

const EMPTY: IdentityProfileInput = {
  username: "",
  first_name: "",
  first_last_name: "",
  second_last_name: "",
  no_second_last_name: false,
  rut: "",
  date_of_birth: "",
  document_type: "cl_id",
  phone: "",
  consent: false,
};

const STATE_LABEL: Record<IdentityProfileState, string> = {
  incomplete: "Perfil incompleto",
  ready: "Listo para verificar",
  in_progress: "Verificación en curso",
  in_review: "Identidad en revisión",
  needs_correction: "Requiere corrección",
  verified: "Identidad verificada",
  expired: "Verificación vencida",
  revoked: "Verificación revocada",
};

function errorCode(error: unknown): string | null {
  return error instanceof ApiError ? error.code : null;
}

interface IdentityProfileCardProps {
  readonly refreshOnMount?: boolean;
  readonly returnTo?: string | null;
}

export function IdentityProfileCard(props: IdentityProfileCardProps) {
  const { snapshot: auth } = useAuth();
  return (
    <IdentityProfileCardSession
      key={auth.status + ":" + (auth.accountId ?? "none")}
      {...props}
    />
  );
}

function IdentityProfileCardSession({
  refreshOnMount = false,
  returnTo,
}: IdentityProfileCardProps) {
  const {
    snapshot,
    save,
    saveUsername,
    saveDraft,
    restoreDraft,
    reload,
    startVerification,
    refreshVerification,
  } = useIdentityProfile();
  const [form, setForm] = useState<IdentityProfileInput>(EMPTY);
  const [username, setUsername] = useState("");
  const [usernameTouched, setUsernameTouched] = useState(false);
  const [draftPassword, setDraftPassword] = useState("");
  const [draftRestored, setDraftRestored] = useState(false);
  const [editor, setEditor] = useState<"identity" | "username" | null>(null);
  const [errors, setErrors] = useState<IdentityProfileErrors>({});
  const [touched, setTouched] = useState<Set<string>>(new Set());
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const firstInvalid = useRef<HTMLElement | null>(null);
  const rutInput = useRef<HTMLInputElement>(null);
  const autoRefreshStarted = useRef(false);

  const status = snapshot.summary?.state ?? "incomplete";
  const rutLocked =
    snapshot.summary?.rut_locked ??
    ["verified", "expired", "revoked"].includes(status);
  const canStartVerification =
    snapshot.summary?.complete &&
    ["ready", "in_progress", "needs_correction", "expired", "revoked"].includes(
      status,
    );
  const usernameValue = usernameTouched
    ? username
    : (snapshot.summary?.username ?? form.username);

  useEffect(() => {
    if (firstInvalid.current) {
      firstInvalid.current.focus();
      firstInvalid.current = null;
    }
  }, [errors]);

  useEffect(() => {
    if (!refreshOnMount || !snapshot.accountId || autoRefreshStarted.current)
      return;
    autoRefreshStarted.current = true;
    const currentUrl = new URL(window.location.href);
    currentUrl.searchParams.delete("refresh_identity");
    window.history.replaceState(
      window.history.state,
      "",
      currentUrl.pathname + currentUrl.search + currentUrl.hash,
    );
    setBusy(true);
    void refreshVerification()
      .catch(() =>
        setSummaryError("No pudimos actualizar el resultado de verificación."),
      )
      .finally(() => setBusy(false));
  }, [refreshOnMount, refreshVerification, snapshot.accountId]);

  function closeEditor() {
    setEditor(null);
    setForm(EMPTY);
    setUsername("");
    setUsernameTouched(false);
    setDraftPassword("");
    setDraftRestored(false);
    setTouched(new Set());
    setErrors({});
    setSummaryError(null);
  }

  function update<K extends keyof IdentityProfileInput>(
    field: K,
    value: IdentityProfileInput[K],
  ) {
    setForm((current) => ({ ...current, [field]: value }));
    setMessage(null);
    setSummaryError(null);
    if (touched.has(field)) {
      const next = { ...form, [field]: value };
      setErrors(validateIdentityProfile(next, new Date(), rutLocked));
    }
  }

  function touch(field: keyof IdentityProfileInput) {
    setTouched((current) => new Set(current).add(field));
    const nextErrors = validateIdentityProfile(
      { ...form, username: usernameValue },
      new Date(),
      rutLocked,
    );
    setErrors((current) => ({ ...current, [field]: nextErrors[field] }));
  }

  function onRutChange(value: string) {
    const cursor = rutInput.current?.selectionStart ?? value.length;
    const formatted = formatChileanRutAtCursor(value, cursor);
    update("rut", formatted.value);
    requestAnimationFrame(() =>
      rutInput.current?.setSelectionRange(formatted.cursor, formatted.cursor),
    );
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage(null);
    setSummaryError(null);
    const input = {
      ...form,
      username: usernameTouched
        ? username
        : (snapshot.summary?.username ?? form.username),
    };
    const nextErrors = validateIdentityProfile(input, new Date(), rutLocked);
    setErrors(nextErrors);
    setTouched(new Set(Object.keys(EMPTY)));
    if (Object.keys(nextErrors).length > 0) {
      const firstField = Object.keys(EMPTY).find(
        (field) => nextErrors[field as keyof IdentityProfileInput],
      );
      firstInvalid.current = firstField
        ? document.getElementById("identity-profile-" + firstField)
        : null;
      setSummaryError("Revisa los campos marcados antes de guardar tu perfil.");
      return;
    }

    setBusy(true);
    try {
      const saved = await save({
        ...input,
        username: normalizeUsername(input.username),
        rut: normalizeChileanRut(input.rut),
        second_last_name: input.no_second_last_name
          ? ""
          : input.second_last_name,
        phone: normalizeChileanE164(input.phone) ?? input.phone,
      });
      closeEditor();
      setMessage(
        saved.complete
          ? "Datos guardados. Ya puedes iniciar la verificación de identidad."
          : "Guardamos tus datos; aún faltan requisitos para verificar.",
      );
    } catch (error) {
      if (error instanceof ApiError) {
        const mapped: IdentityProfileErrors = {};
        for (const fieldError of error.fields) {
          if (fieldError.field in EMPTY) {
            const field = fieldError.field as keyof IdentityProfileInput;
            mapped[field] =
              fieldError.code === "username_taken"
                ? "Ese nombre de usuario ya está ocupado. Prueba con otro."
                : fieldError.code === "invalid_rut"
                  ? "Revisa el RUN y su dígito verificador."
                  : fieldError.code === "rut_locked"
                    ? "El RUN verificado ya está asociado a esta cuenta."
                    : "Revisa este dato e inténtalo nuevamente.";
          }
        }
        if (Object.keys(mapped).length) {
          setErrors((current) => ({ ...current, ...mapped }));
          const firstField = Object.keys(EMPTY).find(
            (field) => mapped[field as keyof IdentityProfileInput],
          );
          firstInvalid.current = firstField
            ? document.getElementById("identity-profile-" + firstField)
            : null;
          setSummaryError(
            "No se guardaron los cambios. Corrige los campos indicados.",
          );
        } else if (
          error.code === "username_taken" ||
          error.code === "username_unavailable"
        ) {
          setErrors((current) => ({
            ...current,
            username: "Ese nombre de usuario ya está ocupado. Prueba con otro.",
          }));
          firstInvalid.current = document.getElementById(
            "identity-profile-username",
          );
        } else {
          setSummaryError(
            "No pudimos guardar los datos. Inténtalo nuevamente.",
          );
        }
      } else {
        setSummaryError("No pudimos guardar los datos. Inténtalo nuevamente.");
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleVerification() {
    setBusy(true);
    setSummaryError(null);
    try {
      const result = await startVerification(returnTo);
      const url = new URL(result.url);
      if (url.protocol !== "https:")
        throw new Error("URL de verificación inválida");
      window.location.assign(url.toString());
    } catch {
      setSummaryError(
        "No pudimos iniciar la verificación. Revisa tu conexión e inténtalo nuevamente.",
      );
      setBusy(false);
    }
  }

  async function handleRefreshVerification() {
    setBusy(true);
    setSummaryError(null);
    try {
      await refreshVerification();
      setMessage("Estado de verificación actualizado.");
    } catch {
      setSummaryError(
        "No pudimos actualizar el resultado. Inténtalo nuevamente.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleUsernameSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const usernameToSave = usernameTouched
      ? username
      : (snapshot.summary?.username ?? username);
    const usernameError = validateIdentityProfile(
      { ...form, username: usernameToSave },
      new Date(),
      rutLocked,
    ).username;
    if (usernameError) {
      setErrors((current) => ({ ...current, username: usernameError }));
      firstInvalid.current = document.getElementById(
        "identity-profile-username",
      );
      setSummaryError("Revisa el nombre de usuario antes de guardar.");
      return;
    }
    setBusy(true);
    setSummaryError(null);
    try {
      await saveUsername(normalizeUsername(usernameToSave));
      closeEditor();
      setMessage("Nombre de usuario actualizado.");
    } catch (error) {
      const code = errorCode(error);
      const text =
        code === "username_taken" || code === "username_unavailable"
          ? "Ese nombre de usuario ya está ocupado. Prueba con otro."
          : "No pudimos actualizar el nombre de usuario. Inténtalo nuevamente.";
      setErrors((current) => ({ ...current, username: text }));
      firstInvalid.current = document.getElementById(
        "identity-profile-username",
      );
      setSummaryError(text);
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveDraft() {
    const draft: components["schemas"]["IdentityProfileDraft"] = {};
    if (form.username.trim()) draft.username = form.username;
    if (form.first_name.trim()) draft.first_name = form.first_name;
    if (form.first_last_name.trim())
      draft.first_last_name = form.first_last_name;
    if (form.second_last_name?.trim())
      draft.second_last_name = form.second_last_name;
    if (form.no_second_last_name) draft.no_second_last_name = true;
    if (form.rut.trim()) draft.rut = form.rut;
    if (form.date_of_birth) draft.date_of_birth = form.date_of_birth;
    if (form.phone.trim()) draft.phone = form.phone;
    if (form.consent) draft.consent = true;
    if (Object.keys(draft).length === 0) {
      setSummaryError("Ingresa al menos un dato para guardar un borrador.");
      return;
    }
    setBusy(true);
    setSummaryError(null);
    try {
      await saveDraft({ ...draft, document_type: "cl_id" });
      closeEditor();
      setMessage(
        "Borrador cifrado guardado. Para retomarlo, confirma tu contraseña actual.",
      );
    } catch {
      setSummaryError("No pudimos guardar el borrador. Inténtalo nuevamente.");
    } finally {
      setBusy(false);
    }
  }

  async function handleRestoreDraft(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draftPassword) {
      setSummaryError(
        "Ingresa tu contraseña actual para restaurar el borrador.",
      );
      return;
    }
    setBusy(true);
    setSummaryError(null);
    try {
      const draft = await restoreDraft(draftPassword);
      setForm({
        ...EMPTY,
        username: draft.username ?? "",
        first_name: draft.first_name ?? "",
        first_last_name: draft.first_last_name ?? "",
        second_last_name: draft.second_last_name ?? "",
        no_second_last_name: draft.no_second_last_name ?? false,
        rut: draft.rut ?? "",
        date_of_birth: draft.date_of_birth ?? "",
        document_type: "cl_id",
        phone: draft.phone ?? "",
        consent: draft.consent ?? false,
      });
      setDraftRestored(true);
      setUsername(draft.username ?? "");
      setUsernameTouched(true);
      setErrors({});
      setTouched(new Set());
      setMessage(
        "Borrador restaurado. Completa los datos pendientes para guardar el perfil.",
      );
    } catch (error) {
      const code = errorCode(error);
      setSummaryError(
        code === "reauthentication_failed" || code === "invalid_credentials"
          ? "La contraseña no coincide. Inténtalo nuevamente."
          : "No pudimos restaurar el borrador. Confirma tu contraseña y vuelve a intentarlo.",
      );
    } finally {
      setDraftPassword("");
      setBusy(false);
    }
  }

  const fieldState = (field: keyof IdentityProfileInput) =>
    touched.has(field) &&
    !errors[field] &&
    Boolean(field === "username" ? usernameValue : form[field]);

  return (
    <div aria-labelledby="identity-profile-title" className="space-y-6">
      <Card>
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <UserRound className="size-5" aria-hidden="true" />
            </div>
            <div className="space-y-1">
              <CardTitle id="identity-profile-title">
                Datos del perfil
              </CardTitle>
              <CardDescription>
                Tu nombre público y tus datos de contacto.
              </CardDescription>
            </div>
          </div>
          {snapshot.summary ? (
            <Button
              variant="outline"
              size="sm"
              className="w-fit gap-2"
              type="button"
              aria-expanded={editor === "identity"}
              aria-controls="identity-profile-editor"
              disabled={busy}
              onClick={() => {
                if (editor === "identity") closeEditor();
                else {
                  closeEditor();
                  setMessage(null);
                  setEditor("identity");
                }
              }}
            >
              <Pencil className="size-4" aria-hidden="true" />
              {editor === "identity"
                ? "Cerrar edición"
                : snapshot.summary.complete
                  ? "Editar datos"
                  : snapshot.summary.draft_saved
                    ? "Continuar perfil"
                    : "Completar perfil"}
            </Button>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-4">
          {snapshot.loading && !snapshot.summary ? (
            <p className="text-sm text-muted-foreground">
              Cargando datos del perfil…
            </p>
          ) : snapshot.error ? (
            <div className="flex flex-wrap items-center gap-3">
              <p className="text-sm text-muted-foreground">
                No pudimos cargar tus datos.
              </p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void reload()}
              >
                Reintentar
              </Button>
            </div>
          ) : snapshot.summary ? (
            <dl className="grid gap-3 sm:grid-cols-2">
              <ProfileValue
                label="Nombre de usuario"
                value={snapshot.summary.username}
              >
                {snapshot.summary.complete && editor !== "username" ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      closeEditor();
                      setMessage(null);
                      setEditor("username");
                    }}
                  >
                    Editar
                  </Button>
                ) : null}
              </ProfileValue>
              <ProfileValue
                label="Correo electrónico"
                value={snapshot.summary.masked_email}
              />
              <ProfileValue label="RUN" value={snapshot.summary.masked_rut} />
              <ProfileValue
                label="Teléfono móvil"
                value={snapshot.summary.masked_phone}
              />
            </dl>
          ) : null}

          {message ? (
            <p
              role="status"
              className="flex items-start gap-2 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900"
            >
              <CheckCircle2
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              {message}
            </p>
          ) : null}
          {summaryError ? (
            <p
              role="alert"
              className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
            >
              <AlertCircle
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              {summaryError}
            </p>
          ) : null}

          {editor === "identity" &&
          snapshot.summary?.draft_saved &&
          !snapshot.summary.complete &&
          !draftRestored ? (
            <form
              className="space-y-3 rounded-lg border border-border p-4"
              onSubmit={(event) => void handleRestoreDraft(event)}
            >
              <div>
                <h3 className="font-semibold">Continuar un borrador</h3>
                <p className="text-sm text-muted-foreground">
                  Tu borrador está cifrado. Confirma la contraseña actual para
                  cargar sus datos en este formulario.
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="identity-profile-draft-password">
                  Contraseña actual
                </Label>
                <Input
                  id="identity-profile-draft-password"
                  type="password"
                  autoComplete="current-password"
                  value={draftPassword}
                  onChange={(event) => setDraftPassword(event.target.value)}
                  disabled={busy}
                  required
                />
              </div>
              <Button
                type="submit"
                variant="outline"
                disabled={busy || !draftPassword}
              >
                Restaurar borrador
              </Button>
            </form>
          ) : null}

          {snapshot.summary?.complete && editor === "username" ? (
            <form
              className="space-y-3 rounded-lg border border-border p-4"
              onSubmit={(event) => void handleUsernameSubmit(event)}
              noValidate
            >
              <div>
                <h3 className="font-semibold">Nombre público</h3>
                <p className="text-sm text-muted-foreground">
                  Este nombre aparece en el navbar y en tus publicaciones.
                </p>
              </div>
              <Field
                id="username"
                label="Nombre de usuario"
                example="3–30 caracteres"
                required
                error={errors.username}
                valid={fieldState("username")}
              >
                <Input
                  id="identity-profile-username"
                  autoComplete="username"
                  maxLength={30}
                  value={usernameValue}
                  onChange={(event) => {
                    setUsername(event.target.value);
                    setUsernameTouched(true);
                    update("username", event.target.value);
                  }}
                  onBlur={() => touch("username")}
                  aria-invalid={Boolean(errors.username)}
                  aria-describedby="identity-profile-username-help identity-profile-username-error"
                />
                <span
                  id="identity-profile-username-help"
                  className="text-xs text-muted-foreground"
                >
                  Letras, números, guion bajo, punto o guion; el primero debe
                  ser letra, número o guion bajo.
                </span>
              </Field>
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  disabled={busy}
                  onClick={closeEditor}
                >
                  Cancelar
                </Button>
                <Button type="submit" disabled={busy}>
                  Actualizar nombre público
                </Button>
              </div>
            </form>
          ) : null}

          {editor === "identity" ? (
            <form
              id="identity-profile-editor"
              aria-label="Editar datos del perfil"
              className="space-y-5 rounded-xl border border-border p-4 sm:p-5"
              onSubmit={(event) => void handleSubmit(event)}
              noValidate
            >
              <p className="text-sm text-muted-foreground">
                Ingresa tus datos tal como figuran en tu cédula. Los campos con
                * son obligatorios.
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                {!snapshot.summary?.complete ? (
                  <Field
                    id="username"
                    label="Nombre de usuario"
                    example="Ejemplo: maria.paz_1"
                    required
                    error={errors.username}
                    valid={fieldState("username")}
                  >
                    <Input
                      id="identity-profile-username"
                      autoComplete="username"
                      maxLength={30}
                      value={usernameValue}
                      onChange={(event) => {
                        setUsername(event.target.value);
                        setUsernameTouched(true);
                        update("username", event.target.value);
                      }}
                      onBlur={() => touch("username")}
                      aria-invalid={Boolean(errors.username)}
                      aria-describedby="identity-profile-username-help identity-profile-username-error"
                    />
                    <span
                      id="identity-profile-username-help"
                      className="text-xs text-muted-foreground"
                    >
                      3–30 caracteres. Letras, números, guion bajo, punto o
                      guion; sin espacios.
                    </span>
                  </Field>
                ) : null}
                <Field
                  id="first_name"
                  label="Nombres legales"
                  example="Tal como aparecen en tu cédula"
                  required
                  error={errors.first_name}
                  valid={fieldState("first_name")}
                >
                  <Input
                    id="identity-profile-first_name"
                    autoComplete="given-name"
                    value={form.first_name}
                    onChange={(event) =>
                      update("first_name", event.target.value)
                    }
                    onBlur={() => touch("first_name")}
                    aria-invalid={Boolean(errors.first_name)}
                    aria-describedby="identity-profile-first_name-error"
                  />
                </Field>
                <Field
                  id="first_last_name"
                  label="Primer apellido"
                  example="Tal como aparece en tu cédula"
                  required
                  error={errors.first_last_name}
                  valid={fieldState("first_last_name")}
                >
                  <Input
                    id="identity-profile-first_last_name"
                    autoComplete="family-name"
                    value={form.first_last_name}
                    onChange={(event) =>
                      update("first_last_name", event.target.value)
                    }
                    onBlur={() => touch("first_last_name")}
                    aria-invalid={Boolean(errors.first_last_name)}
                    aria-describedby="identity-profile-first_last_name-error"
                  />
                </Field>
                <Field
                  id="second_last_name"
                  label="Segundo apellido"
                  example="Tal como aparece en tu cédula"
                  required={!form.no_second_last_name}
                  error={errors.second_last_name}
                  valid={fieldState("second_last_name")}
                >
                  <Input
                    id="identity-profile-second_last_name"
                    autoComplete="additional-name"
                    value={form.second_last_name}
                    disabled={form.no_second_last_name}
                    onChange={(event) =>
                      update("second_last_name", event.target.value)
                    }
                    onBlur={() => touch("second_last_name")}
                    aria-invalid={Boolean(errors.second_last_name)}
                    aria-describedby="identity-profile-second_last_name-error"
                  />
                </Field>
                <label className="flex items-start gap-2 text-sm sm:col-span-2">
                  <input
                    id="identity-profile-no_second_last_name"
                    type="checkbox"
                    className="mt-1 size-4 accent-primary"
                    checked={form.no_second_last_name}
                    aria-invalid={Boolean(errors.second_last_name)}
                    aria-describedby="identity-profile-second_last_name-error"
                    onChange={(event) => {
                      update("no_second_last_name", event.target.checked);
                      if (event.target.checked) {
                        setErrors((current) => {
                          const next = { ...current };
                          delete next.second_last_name;
                          return next;
                        });
                      }
                    }}
                  />
                  <span>No tengo segundo apellido</span>
                </label>
                <Field
                  id="rut"
                  label="RUN"
                  example="Ejemplo: 12.345.678-5"
                  required
                  error={errors.rut}
                  valid={fieldState("rut")}
                >
                  <Input
                    ref={rutInput}
                    id="identity-profile-rut"
                    autoComplete="off"
                    inputMode="text"
                    value={rutLocked ? "" : form.rut}
                    placeholder={
                      rutLocked
                        ? (snapshot.summary?.masked_rut ?? "RUN protegido")
                        : undefined
                    }
                    disabled={rutLocked}
                    onChange={(event) => onRutChange(event.target.value)}
                    onBlur={() => touch("rut")}
                    aria-invalid={Boolean(errors.rut)}
                    aria-describedby="identity-profile-rut-help identity-profile-rut-error"
                  />
                  <span
                    id="identity-profile-rut-help"
                    className="text-xs text-muted-foreground"
                  >
                    {rutLocked
                      ? "El RUN queda asociado a tu cuenta y no se puede reemplazar."
                      : "Escribe o pega el RUN completo con su dígito verificador. Se admite K."}
                  </span>
                </Field>
                <Field
                  id="date_of_birth"
                  label="Fecha de nacimiento"
                  example="Formato esperado: día/mes/año"
                  required
                  error={errors.date_of_birth}
                  valid={fieldState("date_of_birth")}
                >
                  <Input
                    id="identity-profile-date_of_birth"
                    type="date"
                    autoComplete="bday"
                    value={form.date_of_birth}
                    max={new Date().toISOString().slice(0, 10)}
                    onChange={(event) =>
                      update("date_of_birth", event.target.value)
                    }
                    onBlur={() => touch("date_of_birth")}
                    aria-invalid={Boolean(errors.date_of_birth)}
                    aria-describedby="identity-profile-date_of_birth-error"
                  />
                </Field>
                <Field
                  id="phone"
                  label="Teléfono móvil"
                  example="+56 9 1234 5678"
                  required
                  error={errors.phone}
                  valid={fieldState("phone")}
                >
                  <Input
                    id="identity-profile-phone"
                    type="tel"
                    autoComplete="tel"
                    inputMode="tel"
                    placeholder="+56 9 1234 5678"
                    value={form.phone}
                    onChange={(event) => update("phone", event.target.value)}
                    onBlur={() => touch("phone")}
                    aria-invalid={Boolean(errors.phone)}
                    aria-describedby="identity-profile-phone-help identity-profile-phone-error"
                  />
                  <span
                    id="identity-profile-phone-help"
                    className="text-xs text-muted-foreground"
                  >
                    Se guardará en formato internacional E.164.
                  </span>
                </Field>
                <Field
                  id="document_type"
                  label="Tipo de documento"
                  example="Cédula de identidad chilena"
                  required
                  valid
                >
                  <Input
                    id="identity-profile-document_type"
                    value="Cédula de identidad chilena"
                    readOnly
                    aria-describedby="identity-profile-document_type-help"
                  />
                  <span
                    id="identity-profile-document_type-help"
                    className="text-xs text-muted-foreground"
                  >
                    Documento admitido para esta primera etapa.
                  </span>
                </Field>
              </div>

              <label className="flex items-start gap-3 rounded-lg border border-border p-3 text-sm">
                <input
                  id="identity-profile-consent"
                  type="checkbox"
                  className="mt-1 size-4 accent-primary"
                  checked={form.consent}
                  onChange={(event) => update("consent", event.target.checked)}
                  aria-invalid={Boolean(errors.consent)}
                  aria-describedby="identity-profile-consent-error"
                />
                <span>
                  Autorizo el uso de los datos de identidad ingresados para
                  cotejarlos durante la verificación de identidad. Obligatorio.
                </span>
              </label>
              <p
                id="identity-profile-consent-error"
                className={
                  errors.consent ? "text-sm text-destructive" : "sr-only"
                }
              >
                {errors.consent ?? ""}
              </p>

              <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
                <Button
                  type="button"
                  variant="ghost"
                  disabled={busy}
                  onClick={closeEditor}
                >
                  Cancelar
                </Button>
                {!snapshot.summary?.complete ? (
                  <Button
                    type="button"
                    variant="outline"
                    disabled={
                      busy ||
                      snapshot.loading ||
                      !Object.entries(form).some(
                        ([field, value]) =>
                          field !== "document_type" &&
                          (typeof value === "string"
                            ? value.trim().length > 0
                            : value === true),
                      )
                    }
                    onClick={() => void handleSaveDraft()}
                  >
                    Guardar borrador
                  </Button>
                ) : null}
                <Button type="submit" disabled={busy || snapshot.loading}>
                  {busy ? (
                    <Loader2
                      className="mr-2 size-4 animate-spin"
                      aria-hidden="true"
                    />
                  ) : null}
                  Guardar perfil
                </Button>
              </div>
            </form>
          ) : null}
        </CardContent>
      </Card>

      <Card
        id="identity-verification"
        aria-labelledby="identity-verification-title"
        className="scroll-mt-24"
      >
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <ShieldCheck className="size-5" aria-hidden="true" />
            </div>
            <div className="space-y-1">
              <CardTitle id="identity-verification-title">
                Verificación de identidad
              </CardTitle>
              <CardDescription>
                {status === "verified" && snapshot.summary?.verified_until
                  ? "Vigente hasta el " +
                    new Date(
                      snapshot.summary.verified_until,
                    ).toLocaleDateString("es-CL", {
                      timeZone: "America/Santiago",
                    })
                  : status === "in_review"
                    ? "Estamos revisando el resultado de tu verificación."
                    : snapshot.summary?.complete
                      ? "Confirma tu identidad con tu cédula chilena en Didit."
                      : "Completa los datos de tu perfil para iniciar este paso."}
              </CardDescription>
            </div>
          </div>
          <span
            className="w-fit shrink-0 rounded-full border border-border bg-muted px-3 py-1 text-xs font-medium text-foreground"
            data-testid="identity-profile-state"
            role="status"
          >
            {snapshot.loading
              ? "Consultando estado…"
              : snapshot.error
                ? "Estado no disponible"
                : STATE_LABEL[status]}
          </span>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {status !== "verified" && status !== "in_review" ? (
              <Button
                type="button"
                disabled={!canStartVerification || busy}
                onClick={() => void handleVerification()}
              >
                {busy && (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                )}
                {status === "in_progress"
                  ? "Continuar verificación"
                  : ["needs_correction", "expired", "revoked"].includes(status)
                    ? "Volver a verificar"
                    : "Iniciar verificación"}
              </Button>
            ) : null}
            {["in_progress", "in_review"].includes(status) ? (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => void handleRefreshVerification()}
              >
                Actualizar estado
              </Button>
            ) : null}
          </div>
          {status === "needs_correction" ? (
            <p className="text-sm text-muted-foreground">
              Revisa los datos de tu cédula. Si ya configuraste tu identidad en
              otra cuenta, puedes{" "}
              <Link
                href="/recuperar-acceso"
                className="font-medium text-primary underline"
              >
                recuperar el acceso
              </Link>
              .
            </p>
          ) : null}
        </CardContent>
      </Card>

      {snapshot.summary?.complete ? <PinpassAccountSection /> : null}
    </div>
  );
}

function ProfileValue({
  label,
  value,
  children,
}: {
  label: string;
  value?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2.5">
      <div className="min-w-0">
        <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
        <dd className="break-words text-sm font-medium">
          {value || "Por completar"}
        </dd>
      </div>
      {children}
    </div>
  );
}

function Field({
  id,
  label,
  example,
  required,
  error,
  valid,
  children,
}: {
  id: string;
  label: string;
  example: string;
  required: boolean;
  error?: string;
  valid: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={"identity-profile-" + id}>
        {label}{" "}
        {required ? (
          <span className="text-primary" aria-hidden="true">
            *
          </span>
        ) : null}
      </Label>
      {children}
      <span
        id={"identity-profile-" + id + "-error"}
        className={error ? "block text-sm text-destructive" : "sr-only"}
      >
        {error ?? ""}
      </span>
      {!error && example && !valid ? (
        <span className="block text-xs text-muted-foreground">{example}</span>
      ) : null}
    </div>
  );
}
