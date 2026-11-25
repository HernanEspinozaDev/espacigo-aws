import type {
  IdentityProfileInput,
  IdentityProfileSummary,
} from "../domain/identity-profile";
import type { components } from "@/lib/api/generated/openapi";
import {
  getIdentityProfile,
  refreshIdentityVerification,
  restoreIdentityProfileDraft,
  saveIdentityProfile,
  saveIdentityProfileDraft,
  saveIdentityUsername,
  startIdentityVerification,
} from "../infrastructure/identity-profile-api";

export interface IdentityProfileSnapshot {
  readonly accountId: string | null;
  readonly loading: boolean;
  readonly summary: IdentityProfileSummary | null;
  readonly error: boolean;
}

const EMPTY: IdentityProfileSnapshot = Object.freeze({
  accountId: null,
  loading: false,
  summary: null,
  error: false,
});

export class IdentityProfileStore {
  #snapshot: IdentityProfileSnapshot = EMPTY;
  #controller: AbortController | null = null;
  #listeners = new Set<() => void>();
  #epoch = 0;

  getSnapshot = (): IdentityProfileSnapshot => this.#snapshot;
  getServerSnapshot = (): IdentityProfileSnapshot => EMPTY;
  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  #publish(snapshot: IdentityProfileSnapshot): void {
    this.#snapshot = Object.freeze(snapshot);
    for (const listener of this.#listeners) listener();
  }

  async setAccount(accountId: string | null): Promise<void> {
    if (this.#snapshot.accountId === accountId) return;
    this.#epoch += 1;
    this.#controller?.abort();
    this.#controller = null;
    if (!accountId) {
      this.#publish(EMPTY);
      return;
    }
    const epoch = this.#epoch;
    const controller = new AbortController();
    this.#controller = controller;
    this.#publish({ accountId, loading: true, summary: null, error: false });
    try {
      const summary = await getIdentityProfile(controller.signal);
      if (epoch !== this.#epoch || controller.signal.aborted) return;
      this.#publish({ accountId, loading: false, summary, error: false });
    } catch {
      if (epoch !== this.#epoch || controller.signal.aborted) return;
      this.#publish({ accountId, loading: false, summary: null, error: true });
    } finally {
      if (this.#controller === controller) this.#controller = null;
    }
  }

  async reload(): Promise<void> {
    const accountId = this.#snapshot.accountId;
    if (!accountId) return;
    this.#epoch += 1;
    const epoch = this.#epoch;
    this.#controller?.abort();
    const controller = new AbortController();
    this.#controller = controller;
    this.#publish({ ...this.#snapshot, loading: true, error: false });
    try {
      const summary = await getIdentityProfile(controller.signal);
      if (epoch === this.#epoch && !controller.signal.aborted)
        this.#publish({ accountId, loading: false, summary, error: false });
    } catch {
      if (epoch === this.#epoch && !controller.signal.aborted)
        this.#publish({ ...this.#snapshot, loading: false, error: true });
    } finally {
      if (this.#controller === controller) this.#controller = null;
    }
  }

  async save(input: IdentityProfileInput): Promise<IdentityProfileSummary> {
    const accountId = this.#snapshot.accountId;
    if (!accountId) throw new Error("La sesión ya no está disponible.");
    const summary = await saveIdentityProfile(input);
    if (this.#snapshot.accountId === accountId)
      this.#publish({ accountId, loading: false, summary, error: false });
    return summary;
  }

  async saveUsername(username: string): Promise<IdentityProfileSummary> {
    const accountId = this.#snapshot.accountId;
    if (!accountId) throw new Error("La sesión ya no está disponible.");
    const summary = await saveIdentityUsername(username);
    if (this.#snapshot.accountId === accountId)
      this.#publish({ accountId, loading: false, summary, error: false });
    return summary;
  }

  async saveDraft(
    draft: components["schemas"]["IdentityProfileDraft"],
  ): Promise<void> {
    if (!this.#snapshot.accountId)
      throw new Error("La sesión ya no está disponible.");
    await saveIdentityProfileDraft(draft);
    await this.reload();
  }

  restoreDraft(
    currentPassword: string,
  ): Promise<components["schemas"]["IdentityProfileDraft"]> {
    if (!this.#snapshot.accountId)
      throw new Error("La sesión ya no está disponible.");
    return restoreIdentityProfileDraft(currentPassword);
  }

  startVerification(returnTo?: string | null): Promise<{ url: string }> {
    if (!this.#snapshot.accountId)
      throw new Error("La sesión ya no está disponible.");
    return startIdentityVerification(returnTo);
  }

  async refreshVerification(): Promise<IdentityProfileSummary> {
    const accountId = this.#snapshot.accountId;
    if (!accountId) throw new Error("La sesión ya no está disponible.");
    const summary = await refreshIdentityVerification();
    if (this.#snapshot.accountId === accountId)
      this.#publish({ accountId, loading: false, summary, error: false });
    return summary;
  }
}
