import type {
  CalendarConfigurationRead,
  CalendarInventoryUpdate,
} from "../domain/calendar-configuration";

export interface CalendarConfigurationPort {
  read(
    spaceId: string,
    signal: AbortSignal,
  ): Promise<CalendarConfigurationRead>;
  update(
    spaceId: string,
    input: CalendarInventoryUpdate,
    etag: string,
    signal: AbortSignal,
  ): Promise<{ etag: string }>;
}

export interface CalendarConfigurationState {
  phase: "idle" | "loading" | "ready" | "saving" | "needs_refresh" | "error";
  read: CalendarConfigurationRead | null;
  message: string | null;
}

const EMPTY: CalendarConfigurationState = Object.freeze({
  phase: "idle",
  read: null,
  message: null,
});
const AUTH_MESSAGE = "Tu sesión no permite realizar esta operación.";
const ERROR_MESSAGE = "No se pudo cargar la configuración del calendario.";
const UNCERTAIN_MESSAGE =
  "No se pudo confirmar el cambio. Actualiza para verificarlo.";

function statusOf(error: unknown): number | undefined {
  if (typeof error !== "object" || error === null) return undefined;
  const candidate = error as { status?: unknown; statusCode?: unknown };
  if (typeof candidate.status === "number") return candidate.status;
  if (typeof candidate.statusCode === "number") return candidate.statusCode;
  const response = (error as { response?: { status?: unknown } }).response;
  return typeof response?.status === "number" ? response.status : undefined;
}

export class CalendarConfigurationController {
  private state: CalendarConfigurationState = EMPTY;
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private operation: AbortController | null = null;
  private capturedIdentity: string | null = null;

  constructor(
    private readonly port: CalendarConfigurationPort,
    private readonly spaceId: string,
    private readonly getSessionIdentity: () => string | null,
  ) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): CalendarConfigurationState => {
    if (this.getSessionIdentity() !== this.capturedIdentity) {
      if (this.state.read !== null || this.operation !== null) {
        this.generation++;
        this.operation?.abort();
        this.operation = null;
        this.capturedIdentity = this.getSessionIdentity();
        this.state = EMPTY;
      }
      return EMPTY;
    }
    return this.state;
  };

  getServerSnapshot = (): CalendarConfigurationState => EMPTY;

  private publish(state: CalendarConfigurationState): void {
    this.state = state;
    for (const listener of this.listeners) listener();
  }

  private start(phase: CalendarConfigurationState["phase"]): {
    generation: number;
    identity: string | null;
    controller: AbortController;
  } {
    this.operation?.abort();
    const controller = new AbortController();
    this.operation = controller;
    const generation = ++this.generation;
    const identity = this.getSessionIdentity();
    this.capturedIdentity = identity;
    this.publish({ phase, read: null, message: null });
    return { generation, identity, controller };
  }

  private current(generation: number, identity: string | null): boolean {
    return (
      !this.disposed &&
      this.generation === generation &&
      this.getSessionIdentity() === identity
    );
  }

  private finish(generation: number, controller: AbortController): void {
    if (this.generation === generation && this.operation === controller) {
      this.operation = null;
    }
  }

  async refresh(): Promise<void> {
    if (this.disposed) return;
    if (this.getSessionIdentity() === null) {
      this.generation++;
      this.operation?.abort();
      this.operation = null;
      this.capturedIdentity = null;
      this.publish(EMPTY);
      return;
    }
    const op = this.start("loading");
    try {
      const result = await this.port.read(this.spaceId, op.controller.signal);
      if (!this.current(op.generation, op.identity)) return;
      this.capturedIdentity = op.identity;
      this.publish({ phase: "ready", read: result, message: null });
    } catch (error) {
      if (!this.current(op.generation, op.identity)) return;
      this.capturedIdentity = op.identity;
      this.publish({
        phase: "error",
        read: null,
        message:
          statusOf(error) === 401 || statusOf(error) === 403
            ? AUTH_MESSAGE
            : ERROR_MESSAGE,
      });
    } finally {
      this.finish(op.generation, op.controller);
    }
  }

  async save(input: CalendarInventoryUpdate): Promise<void> {
    const identity = this.getSessionIdentity();
    const snapshot = this.getSnapshot();
    if (
      this.disposed ||
      identity === null ||
      identity !== this.capturedIdentity ||
      snapshot.phase !== "ready" ||
      snapshot.read === null
    )
      return;
    const etag = snapshot.read.etag;
    const op = this.start("saving");
    try {
      const ack = await this.port.update(
        this.spaceId,
        input,
        etag,
        op.controller.signal,
      );
      if (!this.current(op.generation, op.identity)) return;
      const readback = await this.port.read(this.spaceId, op.controller.signal);
      if (!this.current(op.generation, op.identity)) return;
      if (readback.etag !== ack.etag) {
        this.publish({
          phase: "needs_refresh",
          read: null,
          message: UNCERTAIN_MESSAGE,
        });
      } else {
        this.capturedIdentity = op.identity;
        this.publish({
          phase: "ready",
          read: readback,
          message: "Configuración guardada y verificada.",
        });
      }
    } catch (error) {
      if (!this.current(op.generation, op.identity)) return;
      this.capturedIdentity = op.identity;
      this.publish({
        phase: "needs_refresh",
        read: null,
        message:
          statusOf(error) === 401 || statusOf(error) === 403
            ? AUTH_MESSAGE
            : UNCERTAIN_MESSAGE,
      });
    } finally {
      this.finish(op.generation, op.controller);
    }
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.generation++;
    this.operation?.abort();
    this.operation = null;
    this.capturedIdentity = null;
    this.publish(EMPTY);
    this.listeners.clear();
  }
}
