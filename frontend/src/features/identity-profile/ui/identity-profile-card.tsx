"use client";

import Link from "next/link";

import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Eye,
  EyeOff,
  Loader2,
  Pencil,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiError } from "@/lib/api/errors";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  chileanMobileSubscriber,
  formatChileanRutAtCursor,
  normalizeChileanE164,
  normalizeChileanRut,
  normalizeUsername,
  validateIdentityProfile,
  type IdentityProfileErrors,
  type IdentityProfileInput,
  type ProfileConfirmationMethod,
} from "../domain/identity-profile";
import { useIdentityProfile } from "./use-identity-profile";
import { IdentityVerifiedBadge } from "./identity-verified-badge";
import { verificationReturnHref } from "@/features/host-listings/domain/publish-guide";
import {
  getPinpassStatus,
  PinpassApiError,
  revealPinpassValue,
  unlockPinpass,
} from "@/features/account/pinpass-api";
import { useSecurityMethodsSummary } from "@/features/account/ui/use-security-methods-summary";
import { ProfileConfirmationDialog } from "./profile-confirmation-dialog";
import type { components } from "@/lib/api/generated/openapi";

const privateFields = [
  { field: "email", label: "Correo electrónico", masked: "masked_email" },
  { field: "run", label: "RUN", masked: "masked_rut" },
  { field: "phone", label: "Teléfono móvil", masked: "masked_phone" },
  { field: "legal_name", label: "Nombre legal completo", masked: null },
  { field: "date_of_birth", label: "Fecha de nacimiento", masked: null },
] as const;
type PrivateField = (typeof privateFields)[number]["field"];

const documentTypes = { cl_id: "Cédula de identidad chilena" };

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

function remainingAccess(expiresAt: string): number {
  return Date.parse(expiresAt) - Date.now();
}

function errorCode(error: unknown): string | null {
  return error instanceof ApiError ? error.code : null;
}

interface IdentityProfileCardProps {
  readonly editOnMount?: boolean;
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
  editOnMount = false,
  returnTo,
}: IdentityProfileCardProps) {
  const {
    snapshot,
    save,
    saveUsername,
    saveDraft,
    readProtectedProfile,
    reload,
  } = useIdentityProfile();
  const [form, setForm] = useState<IdentityProfileInput>(EMPTY);
  const [username, setUsername] = useState("");
  const [usernameTouched, setUsernameTouched] = useState(false);
  const [confirmation, setConfirmation] = useState<
    "draft" | "edit" | "username" | PrivateField | null
  >(null);
  const [confirmationError, setConfirmationError] = useState<string | null>(
    null,
  );
  const [pinConfigured, setPinConfigured] = useState(false);
  const { summary: securityMethods } = useSecurityMethodsSummary();
  const [visibleValues, setVisibleValues] = useState<
    Partial<Record<PrivateField, string>>
  >({});
  const privateRequest = useRef<AbortController | null>(null);
  const visibilityTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [editor, setEditor] = useState<"identity" | "username" | null>(null);
  const [errors, setErrors] = useState<IdentityProfileErrors>({});
  const [touched, setTouched] = useState<Set<string>>(new Set());
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const firstInvalid = useRef<HTMLElement | null>(null);
  const rutInput = useRef<HTMLInputElement>(null);
  const openedInitialEditor = useRef(false);

  const status = snapshot.summary?.state ?? "incomplete";
  const verified =
    status === "verified" && !snapshot.loading && !snapshot.error;
  const rutLocked =
    snapshot.summary?.rut_locked ??
    ["verified", "expired", "revoked"].includes(status);
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
    if (!editOnMount || !snapshot.summary || openedInitialEditor.current)
      return;
    const { complete, draft_saved } = snapshot.summary;
    const frame = window.requestAnimationFrame(() => {
      openedInitialEditor.current = true;
      const url = new URL(window.location.href);
      url.searchParams.delete("editar");
      window.history.replaceState(
        window.history.state,
        "",
        url.pathname + url.search + url.hash,
      );
      if (complete) setConfirmation("edit");
      else if (draft_saved) setConfirmation("draft");
      else setEditor("identity");
    });
    return () => window.cancelAnimationFrame(frame);
  }, [editOnMount, snapshot.summary]);

  useEffect(() => {
    if (!snapshot.accountId) return;
    const controller = new AbortController();
    void getPinpassStatus(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setPinConfigured(result.configured);
      })
      .catch(() => {});
    function hideValues() {
      privateRequest.current?.abort();
      setVisibleValues({});
      if (visibilityTimer.current) clearTimeout(visibilityTimer.current);
    }
    function hideOnBackground() {
      if (document.visibilityState !== "visible") hideValues();
    }
    document.addEventListener("visibilitychange", hideOnBackground);
    return () => {
      document.removeEventListener("visibilitychange", hideOnBackground);
      controller.abort();
      hideValues();
      setPinConfigured(false);
    };
  }, [snapshot.accountId]);

  function openConfirmation(next: NonNullable<typeof confirmation>) {
    privateRequest.current?.abort();
    setSummaryError(null);
    setConfirmationError(null);
    setConfirmation(next);
  }

  async function revealField(field: PrivateField, pin?: string) {
    const controller = new AbortController();
    privateRequest.current?.abort();
    privateRequest.current = controller;
    try {
      if (pin) await unlockPinpass(pin);
      if (controller.signal.aborted) return;
      const result = await revealPinpassValue(field, controller.signal);
      if (controller.signal.aborted) return;
      const remaining = remainingAccess(result.access_expires_at);
      if (!Number.isFinite(remaining) || remaining <= 0) {
        setVisibleValues({});
        openConfirmation(field);
        return;
      }
      if (visibilityTimer.current) clearTimeout(visibilityTimer.current);
      visibilityTimer.current = setTimeout(
        () => setVisibleValues({}),
        remaining,
      );
      setVisibleValues((current) => ({ ...current, [field]: result.value }));
      setConfirmation(null);
    } catch (error) {
      if (controller.signal.aborted) return;
      if (
        !pin &&
        error instanceof PinpassApiError &&
        (error.code === "pinpass_access_required" ||
          error.code === "pinpass_not_configured")
      ) {
        openConfirmation(field);
      } else if (pin) {
        setConfirmationError(
          error instanceof Error
            ? error.message
            : "No pudimos mostrar este dato.",
        );
      } else {
        setSummaryError("No pudimos mostrar este dato. Inténtalo nuevamente.");
      }
    }
  }

  function togglePrivateField(field: PrivateField) {
    if (visibleValues[field] !== undefined) {
      setVisibleValues((current) => ({ ...current, [field]: undefined }));
    } else if (!pinConfigured) {
      openConfirmation(field);
    } else {
      void revealField(field);
    }
  }

  function closeEditor() {
    setEditor(null);
    setForm(EMPTY);
    setUsername("");
    setUsernameTouched(false);

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
      setMessage("Borrador guardado. Puedes continuar tu perfil más tarde.");
    } catch {
      setSummaryError("No pudimos guardar el borrador. Inténtalo nuevamente.");
    } finally {
      setBusy(false);
    }
  }

  async function confirmIdentity(
    method: ProfileConfirmationMethod,
    value: string,
  ) {
    if (!confirmation) return;
    setBusy(true);
    setConfirmationError(null);
    try {
      if (
        confirmation !== "draft" &&
        confirmation !== "edit" &&
        confirmation !== "username"
      ) {
        await revealField(confirmation, value);
        return;
      }
      const intent = confirmation === "draft" ? "draft" : "edit";
      const data = await readProtectedProfile({ [method]: value }, intent);
      closeEditor();
      setForm({
        ...EMPTY,
        username: data.username ?? "",
        first_name: data.first_name ?? "",
        first_last_name: data.first_last_name ?? "",
        second_last_name: data.second_last_name ?? "",
        no_second_last_name: data.no_second_last_name ?? false,
        rut: formatChileanRutAtCursor(data.rut ?? "", 0).value,
        date_of_birth: data.date_of_birth ?? "",
        phone: data.phone?.startsWith("+569")
          ? `+569${chileanMobileSubscriber(data.phone)}`
          : (normalizeChileanE164(data.phone ?? "") ?? ""),
        consent: data.consent ?? false,
      });
      setUsername(data.username ?? "");
      setUsernameTouched(true);
      setEditor(confirmation === "username" ? "username" : "identity");
      setConfirmation(null);
    } catch (error) {
      const code = errorCode(error);
      setConfirmationError(
        code === "temporarily_locked"
          ? "Los intentos están temporalmente bloqueados. Vuelve a intentarlo más tarde."
          : code === "reauthentication_required" ||
              code === "reauthentication_failed"
            ? "No pudimos confirmar tu identidad. Revisa el dato o elige otro método."
            : "No pudimos cargar tus datos. Inténtalo nuevamente.",
      );
    } finally {
      setBusy(false);
    }
  }

  const fieldState = (field: keyof IdentityProfileInput) =>
    touched.has(field) &&
    !errors[field] &&
    Boolean(field === "username" ? usernameValue : form[field]);

  return (
    <div aria-labelledby="identity-profile-title" className="space-y-6">
      <Card id="profile-data" className="scroll-mt-24">
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
                  if (snapshot.summary?.complete) openConfirmation("edit");
                  else if (snapshot.summary?.draft_saved)
                    openConfirmation("draft");
                  else setEditor("identity");
                }
              }}
            >
              <Pencil className="size-4" aria-hidden="true" />
              {editor === "identity"
                ? "Cerrar edición"
                : snapshot.summary.complete
                  ? rutLocked
                    ? "Editar teléfono"
                    : "Editar datos"
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
                      openConfirmation("username");
                    }}
                  >
                    Editar
                  </Button>
                ) : null}
              </ProfileValue>
              {privateFields.map(({ field, label, masked }) => {
                const revealed = visibleValues[field];
                const concealed = masked
                  ? snapshot.summary?.[masked]
                  : snapshot.summary?.complete
                    ? "••••••••"
                    : undefined;
                return (
                  <ProfileValue
                    key={field}
                    label={label}
                    value={revealed ?? concealed}
                  >
                    {concealed && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        disabled={busy}
                        aria-label={`${revealed === undefined ? "Mostrar" : "Ocultar"} ${label}`}
                        onClick={() => togglePrivateField(field)}
                      >
                        {revealed === undefined ? (
                          <Eye aria-hidden="true" />
                        ) : (
                          <EyeOff aria-hidden="true" />
                        )}
                      </Button>
                    )}
                  </ProfileValue>
                );
              })}
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
                {rutLocked
                  ? "Puedes actualizar tu teléfono móvil. Tus datos legales ya están verificados."
                  : "Ingresa tus datos tal como figuran en tu cédula. Los campos con * son obligatorios."}
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
                {!rutLocked && (
                  <>
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
                  </>
                )}
                <Field
                  id="phone"
                  label="Teléfono móvil"
                  example="+56 9 1234 5678"
                  required
                  error={errors.phone}
                  valid={fieldState("phone")}
                >
                  <div className="flex items-center gap-2">
                    <span
                      className="shrink-0 rounded-md border bg-muted px-3 py-2 text-sm font-medium"
                      aria-hidden="true"
                    >
                      +56 9
                    </span>
                    <Input
                      id="identity-profile-phone"
                      type="tel"
                      autoComplete="tel-national"
                      inputMode="numeric"
                      placeholder="1234 5678"
                      value={form.phone.slice(4)}
                      onChange={(event) => {
                        const subscriber = chileanMobileSubscriber(
                          event.target.value,
                        );
                        update("phone", subscriber ? `+569${subscriber}` : "");
                      }}
                      onBlur={() => touch("phone")}
                      aria-invalid={Boolean(errors.phone)}
                      aria-describedby="identity-profile-phone-help identity-profile-phone-error"
                    />
                  </div>
                  <span
                    id="identity-profile-phone-help"
                    className="text-xs text-muted-foreground"
                  >
                    Ingresa los ocho dígitos que siguen a +56 9.
                  </span>
                </Field>
                {!rutLocked && (
                  <Field
                    id="document_type"
                    label="Tipo de documento"
                    example=""
                    required
                    valid
                  >
                    <Select
                      value={form.document_type}
                      items={documentTypes}
                      required
                    >
                      <SelectTrigger
                        id="identity-profile-document_type"
                        className="w-full"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="cl_id">
                          {documentTypes.cl_id}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                )}
              </div>

              {!rutLocked && (
                <label className="flex items-start gap-3 rounded-lg border border-border p-3 text-sm">
                  <input
                    id="identity-profile-consent"
                    type="checkbox"
                    className="mt-1 size-4 accent-primary"
                    checked={form.consent}
                    onChange={(event) =>
                      update("consent", event.target.checked)
                    }
                    aria-invalid={Boolean(errors.consent)}
                    aria-describedby="identity-profile-consent-error"
                  />
                  <span>
                    Autorizo el uso de los datos de identidad ingresados para
                    cotejarlos durante la verificación de identidad.
                    Obligatorio.
                  </span>
                </label>
              )}
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

      <Card id="identity-verification" className="scroll-mt-24">
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle>Verificación de identidad</CardTitle>
            {verified && <IdentityVerifiedBadge />}
          </div>
          <CardDescription>
            {verified
              ? "Consulta la vigencia de tu verificación."
              : "Consulta el estado de tu identidad o inicia la verificación con tu cédula chilena."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline">
            <Link href={verificationReturnHref(returnTo)}>
              Ir a verificaciones
            </Link>
          </Button>
        </CardContent>
      </Card>

      {confirmation && (
        <ProfileConfirmationDialog
          key={confirmation}
          purpose={
            confirmation === "draft"
              ? "draft"
              : confirmation === "edit" || confirmation === "username"
                ? "edit"
                : "reveal"
          }
          pinConfigured={pinConfigured}
          otpConfigured={securityMethods?.methods.totp.enabled === true}
          busy={busy}
          error={confirmationError}
          onClose={() => setConfirmation(null)}
          onConfirm={(method, value) => void confirmIdentity(method, value)}
        />
      )}
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
