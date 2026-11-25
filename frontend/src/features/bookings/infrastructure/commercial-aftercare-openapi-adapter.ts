import "client-only";

import { apiClient } from "@/lib/api/client";
import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";
import type { components } from "@/lib/api/generated/openapi";

export type CommercialSupportCase =
  components["schemas"]["CommercialSupportCase"];

function dataOf<T>(result: {
  data?: { data: T };
  error?: unknown;
  response: Response;
}): T {
  if (result.data !== undefined) return result.data.data;
  throw normalizeApiError(result.response, result.error);
}

function sequence(value: number | string): number {
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 1) throw new ApiProtocolError();
  return parsed;
}

function message<T extends { sequence: string | number }>(item: T) {
  return { ...item, sequence: sequence(item.sequence) };
}

export const commercialAftercare = {
  async summary(reservationID: string) {
    return dataOf(
      await apiClient.GET(
        "/commercial-reservations/{reservationID}/post-sale-summary",
        {
          params: { path: { reservationID } },
        },
      ),
    );
  },
  async messages(reservationID: string, before?: number) {
    const page = dataOf(
      await apiClient.GET("/commercial-reservations/{reservationID}/messages", {
        params: { path: { reservationID }, query: { before, limit: 30 } },
      }),
    );
    return {
      ...page,
      items: page.items.map(message),
      older_cursor:
        page.older_cursor == null ? null : sequence(page.older_cursor),
    };
  },
  async send(reservationID: string, body: string, key: string) {
    return message(
      dataOf(
        await apiClient.POST(
          "/commercial-reservations/{reservationID}/messages",
          {
            params: {
              path: { reservationID },
              header: { "Idempotency-Key": key },
            },
            body: { body },
          },
        ),
      ),
    );
  },
  async markRead(reservationID: string, through_sequence: number) {
    return dataOf(
      await apiClient.POST(
        "/commercial-reservations/{reservationID}/messages/read",
        {
          params: { path: { reservationID } },
          body: { through_sequence },
        },
      ),
    );
  },
  async reviews(reservationID: string) {
    return dataOf(
      await apiClient.GET("/commercial-reservations/{reservationID}/reviews", {
        params: { path: { reservationID } },
      }),
    );
  },
  async review(
    reservationID: string,
    rating: number,
    comment: string,
    key: string,
  ) {
    return dataOf(
      await apiClient.POST("/commercial-reservations/{reservationID}/reviews", {
        params: { path: { reservationID }, header: { "Idempotency-Key": key } },
        body: { rating, comment },
      }),
    );
  },
  async cases(reservationID?: string) {
    return dataOf(
      reservationID
        ? await apiClient.GET(
            "/commercial-reservations/{reservationID}/support-cases",
            {
              params: { path: { reservationID } },
            },
          )
        : await apiClient.GET("/admin/global/reservation-support-cases"),
    );
  },
  async openCase(
    reservationID: string,
    category: string,
    description: string,
    key: string,
  ) {
    return dataOf(
      await apiClient.POST(
        "/commercial-reservations/{reservationID}/support-cases",
        {
          params: {
            path: { reservationID },
            header: { "Idempotency-Key": key },
          },
          body: { category, description },
        },
      ),
    );
  },
  async caseAction(
    caseID: string,
    action: string,
    description: string,
    key: string,
    reservationID?: string,
  ) {
    const header = { "Idempotency-Key": key };
    const body = { action, description };
    return dataOf(
      reservationID
        ? await apiClient.POST(
            "/commercial-reservations/{reservationID}/support-cases/{caseID}/actions",
            {
              params: { path: { reservationID, caseID }, header },
              body,
            },
          )
        : await apiClient.POST(
            "/admin/global/reservation-support-cases/{caseID}/actions",
            {
              params: { path: { caseID }, header },
              body,
            },
          ),
    );
  },
  async attach(
    reservationID: string,
    caseID: string,
    file: File,
    signal: AbortSignal,
  ) {
    const content = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onerror = () => reject(new Error("No se pudo leer el archivo."));
      reader.onload = () => {
        if (typeof reader.result === "string")
          resolve(reader.result.split(",", 2)[1] ?? "");
        else reject(new Error("No se pudo leer el archivo."));
      };
      reader.readAsDataURL(file);
    });
    signal.throwIfAborted();
    return dataOf(
      await apiClient.POST(
        "/commercial-reservations/{reservationID}/support-cases/{caseID}/evidence",
        {
          params: { path: { reservationID, caseID } },
          body: { content },
          signal,
        },
      ),
    );
  },
  async evidence(
    caseID: string,
    evidenceID: string,
    reservationID: string | undefined,
    signal: AbortSignal,
  ) {
    const result = reservationID
      ? await apiClient.GET(
          "/commercial-reservations/{reservationID}/support-cases/{caseID}/evidence/{evidenceID}",
          {
            params: { path: { reservationID, caseID, evidenceID } },
            parseAs: "blob",
            signal,
          },
        )
      : await apiClient.GET(
          "/admin/global/reservation-support-cases/{caseID}/evidence/{evidenceID}",
          {
            params: { path: { caseID, evidenceID } },
            parseAs: "blob",
            signal,
          },
        );
    if (!result.data) throw normalizeApiError(result.response, result.error);
    return result.data;
  },
};
