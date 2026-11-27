"use client";

import "client-only";

import { privateBlobUrls } from "@/lib/api/private-blob-urls";
import type { AuthSnapshot } from "@/features/auth/application/auth-facade";
import type { AccountPort } from "./account.port";
import type { Profile } from "../domain/profile";
import type { ProfilePhotoMetadata } from "../domain/photo";
import type { SyntheticPayoutAccount } from "../domain/payout-account";
import type {
  OwnIdentityExport,
  PrivacyPortabilityBlobResult,
  PrivacyPurposePreference,
  PrivacyTreatmentBlock,
  RightsRequest,
  RightsRequestKind,
  UpdatePrivacyPurposePreferenceInput,
} from "../domain/privacy";

export interface AuthSourceLike {
  getSnapshot(): AuthSnapshot;
  subscribe(listener: () => void): () => void;
  checkSession?(): Promise<unknown>;
}

export interface AccountSnapshot {
  readonly profile: Profile | null;
  readonly photo: ProfilePhotoMetadata | null;
  readonly photoUrl: string | null;
  readonly payoutAccount: SyntheticPayoutAccount | null;
  readonly rightsRequests: readonly RightsRequest[];
  readonly privacyPreferences: readonly PrivacyPurposePreference[];
  readonly privacyRestrictions: readonly PrivacyTreatmentBlock[];
  readonly loading: boolean;
  readonly error: string | null;
}

const initialSnapshot: AccountSnapshot = Object.freeze({
  profile: null,
  photo: null,
  photoUrl: null,
  payoutAccount: null,
  rightsRequests: Object.freeze([]),
  privacyPreferences: Object.freeze([]),
  privacyRestrictions: Object.freeze([]),
  loading: false,
  error: null,
});

const PHOTO_BLOB_RESOURCE_KEY = "account-profile-photo";

function haveEqualArrays(
  a?: readonly string[] | null,
  b?: readonly string[] | null,
): boolean {
  if (a === b) return true;
  const arrA = a ?? [];
  const arrB = b ?? [];
  if (arrA.length !== arrB.length) return false;
  for (let i = 0; i < arrA.length; i++) {
    if (arrA[i] !== arrB[i]) return false;
  }
  return true;
}

function isSameSessionContent(
  prev: AuthSnapshot | null,
  current: AuthSnapshot,
): boolean {
  if (!prev) return false;
  if (prev.status !== current.status) return false;
  if (prev.accountId !== current.accountId) return false;
  if (prev.restrictedMode !== current.restrictedMode) return false;
  if (prev.expiresAt !== current.expiresAt) return false;
  if (!haveEqualArrays(prev.roles, current.roles)) return false;
  if (!haveEqualArrays(prev.capabilities, current.capabilities)) return false;
  return true;
}

function isAuthError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const err = error as {
    status?: unknown;
    statusCode?: unknown;
    code?: unknown;
  };
  return (
    err.status === 401 ||
    err.status === 403 ||
    err.statusCode === 401 ||
    err.statusCode === 403 ||
    err.code === "unauthorized" ||
    err.code === "forbidden" ||
    err.code === "UNAUTHORIZED" ||
    err.code === "FORBIDDEN"
  );
}

function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err !== null && "message" in err) {
    const msg = (err as { message?: unknown }).message;
    if (typeof msg === "string") return msg;
  }
  return fallback;
}

export class AccountFacade {
  #port: AccountPort;
  #authSource?: AuthSourceLike;
  #onPrivatePurge?: () => void;
  #abortController: AbortController | null = null;
  #exportAbortController: AbortController | null = null;
  #updateAbortController: AbortController | null = null;
  #archiveAbortController: AbortController | null = null;
  #portabilityAbortController: AbortController | null = null;
  #photoLoadController: AbortController | null = null;
  #activeControllers = new Set<AbortController>();
  #epoch = 0;
  #snapshot: AccountSnapshot = initialSnapshot;
  #listeners = new Set<() => void>();
  #lastAuthSnapshot: AuthSnapshot | null = null;
  #authUnsubscribe: (() => void) | null = null;

  constructor(
    port: AccountPort,
    authSource?: AuthSourceLike,
    onPrivatePurge?: () => void,
  ) {
    this.#port = port;
    this.#authSource = authSource;
    this.#onPrivatePurge = onPrivatePurge;

    if (authSource) {
      this.#lastAuthSnapshot = authSource.getSnapshot();
      this.#authUnsubscribe = authSource.subscribe(() => {
        this.#handleAuthChange();
      });
    }
  }

  #isCurrent(epoch: number, signal?: AbortSignal): boolean {
    return this.#epoch === epoch && !signal?.aborted;
  }

  #handleAuthChange(): void {
    if (!this.#authSource) return;
    const current = this.#authSource.getSnapshot();
    const prev = this.#lastAuthSnapshot;
    this.#lastAuthSnapshot = current;

    if (current.status !== "authenticated" || !current.accountId) {
      if (
        prev &&
        prev.status === current.status &&
        prev.accountId === current.accountId
      ) {
        return;
      }
      this.clear();
      return;
    }

    if (!prev || prev.status !== "authenticated") {
      this.clear();
      return;
    }

    if (!isSameSessionContent(prev, current)) {
      this.clear();
    }
  }

  getSnapshot = (): AccountSnapshot => {
    return this.#snapshot;
  };

  getServerSnapshot = (): AccountSnapshot => {
    return initialSnapshot;
  };

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  cancel(): void {
    if (this.#abortController) {
      this.#abortController.abort();
      this.#abortController = null;
    }
    if (this.#exportAbortController) {
      this.#exportAbortController.abort();
      this.#exportAbortController = null;
    }
    if (this.#updateAbortController) {
      this.#updateAbortController.abort();
      this.#updateAbortController = null;
    }
    if (this.#archiveAbortController) {
      this.#archiveAbortController.abort();
      this.#archiveAbortController = null;
    }
    if (this.#portabilityAbortController) {
      this.#portabilityAbortController.abort();
      this.#portabilityAbortController = null;
    }
    if (this.#photoLoadController) {
      this.#photoLoadController.abort();
      this.#photoLoadController = null;
    }
    for (const controller of this.#activeControllers) {
      controller.abort();
    }
    this.#activeControllers.clear();
  }

  #setSnapshot(partial: Partial<AccountSnapshot>): void {
    this.#snapshot = Object.freeze({
      ...this.#snapshot,
      ...partial,
    });
    for (const listener of this.#listeners) {
      listener();
    }
  }

  async loadOverview(): Promise<AccountSnapshot> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return this.#snapshot;
    }

    this.cancel();
    const controller = new AbortController();
    this.#abortController = controller;
    const signal = controller.signal;
    const epoch = this.#epoch;

    this.#setSnapshot({ loading: true, error: null });

    try {
      if (
        !this.#isCurrent(epoch, signal) ||
        this.#abortController !== controller
      ) {
        return this.#snapshot;
      }

      if (
        this.#authSource &&
        this.#authSource.getSnapshot().status !== "authenticated"
      ) {
        return this.#snapshot;
      }

      const [
        profile,
        photo,
        payoutAccount,
        rightsRequests,
        privacyPreferences,
        privacyRestrictions,
      ] = await Promise.all([
        this.#port.getProfile(signal),
        this.#port.getProfilePhoto(signal),
        this.#port.getPayoutAccount(signal),
        this.#port.listRightsRequests(signal),
        this.#port.listPrivacyPreferences(signal).catch((err) => {
          if (signal.aborted || isAuthError(err)) throw err;
          return [] as PrivacyPurposePreference[];
        }),
        this.#port.listPrivacyRestrictions(signal).catch((err) => {
          if (signal.aborted || isAuthError(err)) throw err;
          return [] as PrivacyTreatmentBlock[];
        }),
      ]);

      if (
        !this.#isCurrent(epoch, signal) ||
        this.#abortController !== controller
      ) {
        return this.#snapshot;
      }

      let photoUrl: string | null = null;
      if (photo) {
        const photoBlob = await this.#port.getProfilePhotoContent(signal);
        if (
          !this.#isCurrent(epoch, signal) ||
          this.#abortController !== controller
        ) {
          return this.#snapshot;
        }
        if (photoBlob) {
          const registered = privateBlobUrls.create(
            PHOTO_BLOB_RESOURCE_KEY,
            photoBlob,
          );
          photoUrl = registered.url;
        }
      } else {
        if (
          !this.#isCurrent(epoch, signal) ||
          this.#abortController !== controller
        ) {
          return this.#snapshot;
        }
        privateBlobUrls.revoke(PHOTO_BLOB_RESOURCE_KEY);
      }

      if (
        !this.#isCurrent(epoch, signal) ||
        this.#abortController !== controller
      ) {
        return this.#snapshot;
      }

      this.#setSnapshot({
        profile,
        photo,
        photoUrl,
        payoutAccount,
        rightsRequests: Object.freeze(rightsRequests),
        privacyPreferences: Object.freeze(privacyPreferences),
        privacyRestrictions: Object.freeze(privacyRestrictions),
        loading: false,
        error: null,
      });
      return this.#snapshot;
    } catch (err) {
      if (this.#epoch !== epoch) {
        return this.#snapshot;
      }
      if (signal.aborted) throw err;
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error al cargar perfil");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      if (this.#abortController === controller) {
        this.#abortController = null;
      }
    }
  }

  async loadProfilePhoto(signal?: AbortSignal): Promise<void> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return;
    }
    if (this.#snapshot.photoUrl || this.#photoLoadController) return;

    const controller = new AbortController();
    this.#photoLoadController = controller;
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;
    try {
      const photo = await this.#port.getProfilePhoto(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#photoLoadController !== controller
      )
        return;
      if (!photo) {
        privateBlobUrls.revoke(PHOTO_BLOB_RESOURCE_KEY);
        this.#setSnapshot({ photo: null, photoUrl: null });
        return;
      }
      const blob = await this.#port.getProfilePhotoContent(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#photoLoadController !== controller
      )
        return;
      if (!blob) {
        this.#setSnapshot({ photo: null, photoUrl: null });
        return;
      }
      const photoUrl = privateBlobUrls.create(
        PHOTO_BLOB_RESOURCE_KEY,
        blob,
      ).url;
      this.#setSnapshot({ photo, photoUrl });
    } catch (err) {
      if (!this.#isCurrent(epoch, activeSignal)) return;
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
    } finally {
      this.#activeControllers.delete(controller);
      if (this.#photoLoadController === controller) {
        this.#photoLoadController = null;
      }
    }
  }

  async updateProfile(input: {
    displayName: string;
    phone?: string;
  }): Promise<Profile> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    if (this.#updateAbortController) {
      this.#updateAbortController.abort();
      this.#updateAbortController = null;
    }
    const controller = new AbortController();
    this.#updateAbortController = controller;
    const signal = controller.signal;
    const epoch = this.#epoch;

    this.#setSnapshot({ loading: true, error: null });

    try {
      const updated = await this.#port.updateProfile(input, signal);

      if (
        !this.#isCurrent(epoch, signal) ||
        this.#updateAbortController !== controller
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      this.#setSnapshot({
        profile: updated,
        loading: false,
        error: null,
      });
      return updated;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, signal) ||
        this.#updateAbortController !== controller
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error al actualizar perfil");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      if (this.#updateAbortController === controller) {
        this.#updateAbortController = null;
      }
    }
  }

  generateSyntheticPhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    return this.#saveProfilePhoto(
      (activeSignal) =>
        this.#port.generateSyntheticPhoto(idempotencyKey, activeSignal),
      signal,
    );
  }

  uploadProfilePhoto(
    idempotencyKey: string,
    image: File,
    signal?: AbortSignal,
  ): Promise<void> {
    return this.#saveProfilePhoto(
      (activeSignal) =>
        this.#port.uploadProfilePhoto(idempotencyKey, image, activeSignal),
      signal,
    );
  }

  async #saveProfilePhoto(
    save: (signal: AbortSignal) => Promise<void>,
    signal?: AbortSignal,
  ): Promise<void> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      await save(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      const photo = await this.#port.getProfilePhoto(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      if (!photo) {
        throw new Error("No se pudo confirmar la foto guardada.");
      }

      const blob = await this.#port.getProfilePhotoContent(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (!blob) {
        throw new Error("No se pudo cargar la foto guardada.");
      }
      const photoUrl = privateBlobUrls.create(
        PHOTO_BLOB_RESOURCE_KEY,
        blob,
      ).url;

      this.#setSnapshot({
        photo,
        photoUrl,
        loading: false,
        error: null,
      });
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error con foto de perfil");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async removeProfilePhoto(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      await this.#port.removeProfilePhoto(idempotencyKey, activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      privateBlobUrls.revoke(PHOTO_BLOB_RESOURCE_KEY);
      this.#setSnapshot({
        photo: null,
        photoUrl: null,
        loading: false,
        error: null,
      });
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error al eliminar foto");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async createOrReplacePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<SyntheticPayoutAccount> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      const account = await this.#port.createOrReplacePayoutAccount(
        idempotencyKey,
        activeSignal,
      );
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      this.#setSnapshot({
        payoutAccount: account,
        loading: false,
        error: null,
      });
      return account;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(
        err,
        "Error al configurar cuenta de cobro",
      );
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async revokePayoutAccount(
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<void> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      await this.#port.revokePayoutAccount(idempotencyKey, activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      this.#setSnapshot({
        payoutAccount: null,
        loading: false,
        error: null,
      });
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error al revocar cuenta de cobro");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async requestDataRight(
    kind: RightsRequestKind,
    detail = "",
    signal?: AbortSignal,
  ): Promise<RightsRequest> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      const req = await this.#port.requestDataRight(kind, detail, activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      const list = await this.#port.listRightsRequests(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      this.#setSnapshot({
        rightsRequests: Object.freeze(list),
        loading: false,
        error: null,
      });
      return req;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(err, "Error al solicitar derecho");
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async exportIdentityData(
    signal?: AbortSignal,
  ): Promise<OwnIdentityExport | null> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return null;
    }

    const epoch = this.#epoch;
    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      return null;
    }

    try {
      const data = await this.#port.exportIdentityData(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return null;
      }
      return data;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return null;
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async exportIdentityBlob(signal?: AbortSignal): Promise<Blob | null> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return null;
    }
    const epoch = this.#epoch;
    if (this.#epoch !== epoch) {
      return null;
    }
    const controller = new AbortController();
    this.#exportAbortController = controller;
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      return null;
    }

    try {
      const blob = await this.#port.exportIdentityBlob(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#exportAbortController !== controller
      ) {
        return null;
      }
      return blob;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#exportAbortController !== controller
      ) {
        return null;
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      if (this.#exportAbortController === controller) {
        this.#exportAbortController = null;
      }
    }
  }

  async downloadIdentityArchive(signal?: AbortSignal): Promise<Blob | null> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return null;
    }
    const epoch = this.#epoch;
    if (this.#epoch !== epoch) {
      return null;
    }
    const controller = new AbortController();
    this.#archiveAbortController = controller;
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      return null;
    }

    try {
      const blob = await this.#port.downloadIdentityArchive(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#archiveAbortController !== controller
      ) {
        return null;
      }
      return blob;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#archiveAbortController !== controller
      ) {
        return null;
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      if (this.#archiveAbortController === controller) {
        this.#archiveAbortController = null;
      }
    }
  }

  async listPrivacyPreferences(
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference[]> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return [];
    }

    const epoch = this.#epoch;
    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      return [];
    }

    try {
      const items = await this.#port.listPrivacyPreferences(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return [];
      }
      this.#setSnapshot({
        privacyPreferences: Object.freeze(items),
      });
      return items;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return [];
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async updatePrivacyPreference(
    input: UpdatePrivacyPurposePreferenceInput,
    signal?: AbortSignal,
  ): Promise<PrivacyPurposePreference> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      throw new DOMException("The operation was aborted", "AbortError");
    }

    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;
    const epoch = this.#epoch;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      throw new DOMException("The operation was aborted", "AbortError");
    }

    this.#setSnapshot({ loading: true, error: null });

    try {
      const updated = await this.#port.updatePrivacyPreference(
        input,
        activeSignal,
      );
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        throw new DOMException("The operation was aborted", "AbortError");
      }

      const remaining = this.#snapshot.privacyPreferences.filter(
        (item) => item.purpose !== updated.purpose,
      );
      this.#setSnapshot({
        privacyPreferences: Object.freeze([...remaining, updated]),
        loading: false,
        error: null,
      });
      return updated;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        if (err instanceof Error && err.name === "AbortError") {
          throw err;
        }
        throw new DOMException("The operation was aborted", "AbortError");
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      const message = getErrorMessage(
        err,
        "Error al actualizar preferencia de finalidad",
      );
      this.#setSnapshot({ loading: false, error: message });
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async listPrivacyRestrictions(
    signal?: AbortSignal,
  ): Promise<PrivacyTreatmentBlock[]> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return [];
    }

    const epoch = this.#epoch;
    const controller = new AbortController();
    this.#activeControllers.add(controller);
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      this.#activeControllers.delete(controller);
      return [];
    }

    try {
      const items = await this.#port.listPrivacyRestrictions(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return [];
      }
      this.#setSnapshot({
        privacyRestrictions: Object.freeze(items),
      });
      return items;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        !this.#activeControllers.has(controller)
      ) {
        return [];
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      this.#activeControllers.delete(controller);
    }
  }

  async exportPortabilityBlob(
    signal?: AbortSignal,
  ): Promise<PrivacyPortabilityBlobResult | null> {
    if (
      this.#authSource &&
      this.#authSource.getSnapshot().status !== "authenticated"
    ) {
      return null;
    }
    const epoch = this.#epoch;
    if (this.#epoch !== epoch) {
      return null;
    }
    const controller = new AbortController();
    this.#portabilityAbortController = controller;
    const activeSignal = signal
      ? AbortSignal.any([controller.signal, signal])
      : controller.signal;

    if (activeSignal.aborted) {
      return null;
    }

    try {
      const result = await this.#port.exportPortabilityBlob(activeSignal);
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#portabilityAbortController !== controller
      ) {
        return null;
      }
      return result;
    } catch (err) {
      if (
        !this.#isCurrent(epoch, activeSignal) ||
        this.#portabilityAbortController !== controller
      ) {
        return null;
      }
      if (isAuthError(err)) {
        this.clear();
        try {
          void this.#authSource?.checkSession?.().catch(() => undefined);
        } catch {
          /* best effort */
        }
      }
      throw err;
    } finally {
      if (this.#portabilityAbortController === controller) {
        this.#portabilityAbortController = null;
      }
    }
  }

  clear(): void {
    this.#epoch += 1;
    this.cancel();
    privateBlobUrls.revoke(PHOTO_BLOB_RESOURCE_KEY);
    this.#onPrivatePurge?.();
    this.#setSnapshot(initialSnapshot);
  }
}
