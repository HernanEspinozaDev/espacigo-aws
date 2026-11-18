"use client";

import { useRef, useState, type ChangeEvent } from "react";
import Image from "next/image";
import {
  AlertCircle,
  Camera,
  CheckCircle2,
  Upload,
  Loader2,
  Trash2,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Button } from "@/components/workspace/ui/button";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isValidPhotoSize, MAX_PHOTO_SIZE_BYTES } from "../domain/photo";
import { useAccount } from "./use-account";

const photoTypes = ["image/jpeg", "image/png", "image/webp"];

export function ProfilePhotoCard() {
  const { snapshot: authSnapshot } = useAuth();
  const { snapshot, uploadProfilePhoto, removeProfilePhoto } = useAccount();
  const fileInput = useRef<HTMLInputElement>(null);
  const [loadingAction, setLoadingAction] = useState<
    "upload" | "delete" | null
  >(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const canEdit =
    authSnapshot.status === "authenticated" &&
    !loadingAction &&
    !snapshot.loading;

  const handleUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const image = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!image || !canEdit) return;

    setError(null);
    setMessage(null);
    if (image.type && !photoTypes.includes(image.type)) {
      setError("Elige una imagen JPG, PNG o WebP.");
      return;
    }
    if (!isValidPhotoSize(image.size)) {
      setError("Elige una imagen de hasta 10 MB que no esté vacía.");
      return;
    }

    setLoadingAction("upload");
    try {
      await uploadProfilePhoto(crypto.randomUUID(), image);
      setMessage("Foto de perfil actualizada.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error al subir la foto.");
    } finally {
      setLoadingAction(null);
    }
  };

  const handleDelete = async () => {
    if (!canEdit) return;
    setError(null);
    setMessage(null);
    setLoadingAction("delete");
    try {
      const idempotencyKey = crypto.randomUUID();
      await removeProfilePhoto(idempotencyKey);
      setMessage("Foto de perfil eliminada.");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Error al eliminar la foto",
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const photoUrl = snapshot.photoUrl;
  const hasPhoto = Boolean(snapshot.photo);

  return (
    <Card aria-labelledby="photo-title">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Camera className="size-5 text-primary" aria-hidden="true" />
          <CardTitle id="photo-title">Foto de perfil</CardTitle>
        </div>
        <CardDescription>
          JPG, PNG o WebP. Hasta {MAX_PHOTO_SIZE_BYTES / (1024 * 1024)} MB.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <div
            role="alert"
            className="flex items-center gap-2 rounded-md bg-destructive/15 p-3 text-sm text-destructive border border-destructive/20"
          >
            <AlertCircle className="size-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {message && (
          <div
            role="status"
            className="flex items-center gap-2 rounded-md bg-emerald-50 p-3 text-sm text-emerald-800 border border-emerald-200 dark:bg-emerald-950/30 dark:border-emerald-800 dark:text-emerald-300"
          >
            <CheckCircle2 className="size-4 shrink-0" />
            <span>{message}</span>
          </div>
        )}

        <div className="flex flex-col sm:flex-row items-center gap-6 pt-2">
          <div className="relative size-28 overflow-hidden rounded-full border-2 border-dashed border-border bg-muted flex items-center justify-center shrink-0">
            {hasPhoto && photoUrl ? (
              <Image
                src={photoUrl}
                alt="Foto de perfil del usuario"
                fill
                sizes="112px"
                unoptimized
                className="object-cover"
              />
            ) : (
              <Camera
                className="size-8 text-muted-foreground"
                aria-hidden="true"
              />
            )}
          </div>

          <div className="flex flex-wrap gap-3">
            <input
              ref={fileInput}
              type="file"
              accept={photoTypes.join(",")}
              aria-label="Archivo de foto de perfil"
              onChange={handleUpload}
              disabled={!canEdit}
              hidden
            />
            <Button
              type="button"
              onClick={() => fileInput.current?.click()}
              disabled={!canEdit}
            >
              {loadingAction === "upload" ? (
                <Loader2
                  className="mr-2 size-4 animate-spin"
                  aria-hidden="true"
                />
              ) : (
                <Upload className="mr-2 size-4" aria-hidden="true" />
              )}
              {loadingAction === "upload"
                ? "Subiendo foto…"
                : hasPhoto
                  ? "Cambiar foto"
                  : "Subir foto"}
            </Button>

            {hasPhoto && (
              <Button
                type="button"
                variant="destructive"
                onClick={handleDelete}
                disabled={!canEdit}
              >
                {loadingAction === "delete" ? (
                  <Loader2
                    className="mr-2 size-4 animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <Trash2 className="mr-2 size-4" aria-hidden="true" />
                )}
                Retirar foto
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
