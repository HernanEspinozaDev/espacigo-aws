"use client";

import Link from "next/link";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { AlertCircle, CheckCircle2, Loader2, ShieldCheck } from "lucide-react";
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
  const [editingIdentity, setEditingIdentity] = useState(false);
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
  const canStartVerification = snapshot.summary?.complete && status === "ready";
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
        phone: normalizeChileanE164(input.phone) ?? input.phone,
      });
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
      const saved = await saveUsername(normalizeUsername(usernameToSave));
      setUsername(saved.username ?? normalizeUsername(usernameToSave));
      setUsernameTouched(true);
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
    <Card aria-labelledby="identity-profile-title">
      <CardHeader>
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-5 text-primary" aria-hidden="true" />
          <CardTitle id="identity-profile-title">Completa tus datos</CardTitle>
        </div>
        <CardDescription>
          Guarda tu nombre público y los datos de tu cédula chilena. Los datos
          legales se mantienen protegidos y aquí solo se muestra su versión
          enmascarada.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/30 p-3">
          <span className="text-sm font-medium">Estado de identidad</span>
          <span
            className="rounded-full bg-background px-3 py-1 text-sm"
            data-testid="identity-profile-state"
            role="status"
          >
            {snapshot.loading
              ? "Consultando estado…"
              : snapshot.error
                ? "Estado no disponible"
                : STATE_LABEL[status]}
          </span>
          {snapshot.summary?.masked_email ? (
            <span className="text-sm text-muted-foreground">
              Correo: {snapshot.summary.masked_email}
            </span>
          ) : null}
          {snapshot.summary?.masked_rut ? (
            <span className="text-sm text-muted-foreground">
              RUN: {snapshot.summary.masked_rut}
            </span>
          ) : null}
          {snapshot.summary?.masked_phone ? (
            <span className="text-sm text-muted-foreground">
              Teléfono: {snapshot.summary.masked_phone}
            </span>
          ) : null}
          {snapshot.summary?.verified_until ? (
            <span className="text-sm text-muted-foreground">
              Vigente hasta{" "}
              {new Date(snapshot.summary.verified_until).toLocaleDateString(
                "es-CL",
              )}
            </span>
          ) : null}
          {["in_progress", "in_review", "needs_correction"].includes(status) ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void handleRefreshVerification()}
            >
              Actualizar estado
            </Button>
          ) : null}
          {snapshot.error ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void reload()}
            >
              Reintentar
            </Button>
          ) : null}
        </div>

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

        {snapshot.summary?.draft_saved &&
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

        {snapshot.summary?.complete ? (
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
                Letras, números, guion bajo, punto o guion; el primero debe ser
                letra, número o guion bajo.
              </span>
            </Field>
            <div className="flex justify-end">
              <Button type="submit" variant="outline" disabled={busy}>
                Actualizar nombre público
              </Button>
            </div>
          </form>
        ) : null}

        {snapshot.summary?.complete && !editingIdentity ? (
          <Button
            type="button"
            variant="ghost"
            onClick={() => setEditingIdentity(true)}
          >
            Actualizar datos legales
          </Button>
        ) : null}

        {!snapshot.summary?.complete || editingIdentity ? (
          <form
            className="space-y-5"
            onSubmit={(event) => void handleSubmit(event)}
            noValidate
          >
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
                    3–30 caracteres. Letras, números, guion bajo, punto o guion;
                    sin espacios.
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
                  onChange={(event) => update("first_name", event.target.value)}
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
                  maxLength={12}
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
                    : "Escribe o pega el RUN completo; el dígito verificador lo ingresas tú. Se admite K."}
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

            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">
                Los campos marcados Obligatorio se necesitan para finalizar el
                perfil.
              </p>
              <div className="flex flex-wrap gap-2">
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
            </div>
          </form>
        ) : null}

        <section
          id="identity-verification"
          aria-labelledby="identity-verification-title"
          className="border-t border-border pt-5"
        >
          <h3 id="identity-verification-title" className="font-semibold">
            Verifica tu identidad
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            La verificación se abre en el proveedor y su resultado se confirma
            en EspaciGo.
          </p>
          <Button
            type="button"
            className="mt-3"
            variant="outline"
            disabled={!canStartVerification || busy}
            onClick={() => void handleVerification()}
          >
            {busy ? (
              <Loader2
                className="mr-2 size-4 animate-spin"
                aria-hidden="true"
              />
            ) : null}
            Iniciar verificación
          </Button>
          {!canStartVerification ? (
            <p className="mt-2 text-xs text-muted-foreground">
              Guarda primero todos los datos obligatorios para habilitar esta
              acción.
            </p>
          ) : null}
        </section>
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
        {snapshot.summary?.complete ? <PinpassAccountSection /> : null}
      </CardContent>
    </Card>
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
        <span className="font-normal text-muted-foreground">
          ({example}) · {required ? "Obligatorio" : "Opcional"}
        </span>
      </Label>
      {children}
      <span
        id={"identity-profile-" + id + "-error"}
        className={error ? "block text-sm text-destructive" : "sr-only"}
      >
        {error ?? ""}
      </span>
      {!error && valid ? (
        <span className="flex items-center gap-1 text-xs text-emerald-700">
          <CheckCircle2 className="size-3.5" aria-hidden="true" />
          Dato válido
        </span>
      ) : !error ? (
        <span className="text-xs text-muted-foreground">Pendiente</span>
      ) : null}
    </div>
  );
}
