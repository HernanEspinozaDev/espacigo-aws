import "client-only";
import { authFacade } from "@/features/auth/auth-runtime";
import type { MercadoPagoConnection } from "../domain/mercado-pago-connection";
import type { MercadoPagoPort } from "./mercado-pago.port";

export interface MercadoPagoSnapshot {
  connection: MercadoPagoConnection | null;
  loading: boolean;
  starting: boolean;
  error: "unavailable" | "not_configured" | "request_failed" | null;
}

const EMPTY: MercadoPagoSnapshot = Object.freeze({
  connection: null,
  loading: false,
  starting: false,
  error: null,
});

export class MercadoPagoFacade {
  #snapshot: MercadoPagoSnapshot = EMPTY;
  #listeners = new Set<() => void>();
  #generation = 0;

  constructor(private readonly port: MercadoPagoPort) {}

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };
  getSnapshot = () => this.#snapshot;
  getServerSnapshot = () => EMPTY;

  #publish(snapshot: MercadoPagoSnapshot) {
    this.#snapshot = Object.freeze(snapshot);
    for (const listener of this.#listeners) listener();
  }

  clear() {
    this.#generation++;
    this.#publish(EMPTY);
  }

  async load() {
    const generation = ++this.#generation;
    this.#publish({ ...this.#snapshot, loading: true, error: null });
    try {
      const connection = await authFacade.runAuthenticatedOperation(
        (token, signal) => this.port.getConnection(token, signal),
      );
      if (generation === this.#generation) {
        this.#publish({
          connection,
          loading: false,
          starting: false,
          error: null,
        });
      }
    } catch {
      if (generation === this.#generation) {
        this.#publish({
          ...this.#snapshot,
          loading: false,
          error: "request_failed",
        });
      }
    }
  }

  async startAuthorization() {
    if (!this.#snapshot.connection?.authorizationAvailable) {
      this.#publish({ ...this.#snapshot, error: "not_configured" });
      return;
    }
    const generation = this.#generation;
    this.#publish({ ...this.#snapshot, starting: true, error: null });
    try {
      const url = await authFacade.runAuthenticatedOperation((token, signal) =>
        this.port.startAuthorization(token, signal),
      );
      if (generation === this.#generation) window.location.assign(url);
    } catch {
      if (generation === this.#generation) {
        this.#publish({
          ...this.#snapshot,
          starting: false,
          error: "request_failed",
        });
      }
      throw new Error("Unable to start Mercado Pago authorization");
    }
  }
}
