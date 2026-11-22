import "client-only";

import { apiClient } from "@/lib/api/client";
import {
  ApiError,
  ApiProtocolError,
  normalizeApiError,
} from "@/lib/api/errors";
import { serializeExactJson } from "@/lib/api/exact-json";
import type { components } from "@/lib/api/generated/openapi";
import type { BookingsPort } from "../application/bookings.port";
import type {
  BookingDetail,
  BookingQuote,
  BookingReservation,
  BookingState,
  BusinessClock,
  CancellationPreview,
  DamageClaim,
  Guarantee,
  RentalOperation,
  ReservationMessage,
  ReservationReview,
  ReviewReport,
  Settlement,
  SettlementReceipt,
  SimulatedPaymentOutcome,
  SyntheticContract,
  TrialDispute,
  TrialDisputeTransition,
} from "../domain/types";

type CommercialReservationSchema =
  components["schemas"]["CommercialReservation"];

type CommercialReservationWithBuyerFees = CommercialReservationSchema & {
  buyer_commission_clp?: number | string;
  buyer_vat_clp?: number | string;
  buyer_fee_clp?: number | string;
  total_charged_clp?: number | string;
};

function optionalNumber(
  value: number | string | undefined,
): number | undefined {
  if (value === undefined) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function mapCommercialToBookingReservation(
  item: CommercialReservationSchema,
): BookingReservation {
  const extended = item as CommercialReservationWithBuyerFees;
  return {
    id: item.id,
    quote_id: item.quote_id,
    space_id: item.space_id,
    space_title: item.space_title,
    host_id: item.participant_role === "anfitrion" ? "me" : "",
    renter_id: item.participant_role === "arrendatario" ? "me" : "",
    participant_role: item.participant_role,
    checkout_status: item.checkout_status,
    checkout_method: item.checkout_method,
    provider_payment_id: item.provider_payment_id,
    commission_clp: optionalNumber(item.commission_clp),
    vat_clp: optionalNumber(item.vat_clp),
    marketplace_fee_clp: optionalNumber(item.marketplace_fee_clp),
    net_host_amount_clp: optionalNumber(item.net_host_amount_clp),
    buyer_commission_clp: optionalNumber(extended.buyer_commission_clp),
    buyer_vat_clp: optionalNumber(extended.buyer_vat_clp),
    buyer_fee_clp: optionalNumber(extended.buyer_fee_clp),
    total_charged_clp: optionalNumber(extended.total_charged_clp),
    is_commercial: true,
    state: item.status as BookingState,
    rate_unit: item.rate_unit,
    unit_price_clp: Number(item.unit_price_clp),
    currency: "CLP",
    units: Number(item.units),
    subtotal_clp: Number(item.subtotal_clp),
    start_at: item.start_at,
    end_at: item.end_at,
    time_zone: item.time_zone,
    ...(Array.isArray(item.intervals) && item.intervals.length > 0
      ? {
          intervals: item.intervals.map((iv) => ({
            ...(iv.id ? { id: iv.id } : {}),
            start_at: iv.start_at,
            end_at: iv.end_at,
            units: Number(iv.units),
            ...(iv.usage_status ? { usage_status: iv.usage_status } : {}),
            ...(iv.check_in_at ? { check_in_at: iv.check_in_at } : {}),
            ...(iv.check_out_at ? { check_out_at: iv.check_out_at } : {}),
            ...(iv.pass_code ? { pass_code: iv.pass_code } : {}),
            ...(iv.qr_payload ? { qr_payload: iv.qr_payload } : {}),
          })),
        }
      : {}),
    conditions: "",
    cancellation_policy_version: "v1",
    pay_expires_at: item.payment_expires_at,
    created_at: item.created_at,
    updated_at: item.updated_at,
    unread_count: 0,
  };
}

function mapCommercialToBookingDetail(
  item: CommercialReservationSchema,
): BookingDetail {
  const reservation = mapCommercialToBookingReservation(item);
  const methodLabel =
    item.checkout_method === "payments_api"
      ? "Checkout API (Tarjeta)"
      : "Checkout Pro";
  const paymentSuffix = item.provider_payment_id
    ? ` · Operación #${item.provider_payment_id}`
    : "";
  return {
    ...reservation,
    history: [
      {
        sequence: 1,
        to: item.status,
        reason: `${methodLabel}: ${item.checkout_status}${paymentSuffix}`,
        at: item.updated_at,
      },
    ],
  };
}

export class OpenApiBookingsAdapter implements BookingsPort {
  async quote(
    input: { space_id: string; start_at: string; end_at: string },
    signal?: AbortSignal,
  ): Promise<BookingQuote> {
    const result = await apiClient.POST("/local/booking-trial/quotes", {
      body: input,
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as BookingQuote;
  }

  async getFixture(signal?: AbortSignal): Promise<unknown> {
    const result = await apiClient.GET("/local/booking-trial/fixture", {
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return result.data;
  }

  async listReservations(signal?: AbortSignal): Promise<BookingReservation[]> {
    try {
      const commercial = await apiClient.GET("/commercial-reservations", {
        signal,
      });
      if (commercial.response.ok && commercial.data?.data) {
        const items = commercial.data.data.items ?? [];
        return items.map(mapCommercialToBookingReservation);
      }
      if (commercial.response.status !== 404) {
        throw normalizeApiError(commercial.response, commercial.error);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status !== 404) {
        throw err;
      }
      if (
        err &&
        typeof err === "object" &&
        "status" in err &&
        err.status !== 404
      ) {
        throw err;
      }
    }

    const result = await apiClient.GET("/local/booking-trial/reservations", {
      signal,
    });
    if (result.response.status === 404) {
      return [];
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data?.items as unknown as BookingReservation[]) ?? [];
  }

  async getReservation(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<BookingDetail | null> {
    try {
      const commercial = await apiClient.GET(
        "/commercial-reservations/{reservationID}",
        {
          params: { path: { reservationID: reservationId } },
          signal,
        },
      );
      if (commercial.response.ok && commercial.data?.data) {
        return mapCommercialToBookingDetail(commercial.data.data);
      }
      if (commercial.response.status !== 404) {
        throw normalizeApiError(commercial.response, commercial.error);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status !== 404) {
        throw err;
      }
    }

    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data as unknown as BookingDetail) ?? null;
  }

  async createReservation(
    idempotencyKey: string,
    quoteId: string,
    signal?: AbortSignal,
  ): Promise<BookingReservation> {
    const result = await apiClient.POST("/local/booking-trial/reservations", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: { quote_id: quoteId },
      bodySerializer: serializeExactJson,
      signal,
    });
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as BookingReservation;
  }

  async decideReservation(
    reservationId: string,
    decision: "aprobar" | "rechazar",
    reason?: string,
    signal?: AbortSignal,
  ): Promise<BookingReservation> {
    const body: { decision: "aprobar" | "rechazar"; reason?: string } = {
      decision,
    };
    if (reason) {
      body.reason = reason;
    }
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/decision",
      {
        params: { path: { reservationId } },
        body,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as BookingReservation;
  }

  async previewCancellation(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<CancellationPreview> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}/cancellation-preview",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    const d = result.data.data;
    return {
      reservation_id: d.reservation_id,
      policy_version: d.policy_version,
      eligible: d.eligible,
      refund_ratio: d.eligible ? 1 : 0,
      refund_amount_clp: d.amount_clp,
      penalty_amount_clp: 0,
      reason: d.reason_code ?? d.refund_label,
    };
  }

  async cancelReservation(
    idempotencyKey: string,
    reservationId: string,
    reason?: string,
    signal?: AbortSignal,
  ): Promise<BookingReservation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/cancel",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { reason },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data?.reservation) throw new ApiProtocolError();
    return result.data.data.reservation as unknown as BookingReservation;
  }

  async simulatePayment(
    idempotencyKey: string,
    reservationId: string,
    outcome: SimulatedPaymentOutcome,
    signal?: AbortSignal,
  ): Promise<BookingReservation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/payment",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { outcome },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as BookingReservation;
  }

  async simulateRefund(
    idempotencyKey: string,
    reservationId: string,
    outcome: "exito" | "fallo" | "sin_respuesta",
    signal?: AbortSignal,
  ): Promise<BookingReservation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/refund",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { outcome },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const updated = await this.getReservation(reservationId, signal);
    if (updated) return updated;
    throw new ApiProtocolError();
  }

  async createOrGetContract(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<SyntheticContract | null> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/contract",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) return null;
    return result.data.data as unknown as SyntheticContract;
  }

  async getContract(
    contractId: string,
    signal?: AbortSignal,
  ): Promise<SyntheticContract | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/contracts/{contractId}",
      {
        params: { path: { contractId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data as unknown as SyntheticContract) ?? null;
  }

  async signContract(
    contractId: string,
    signal?: AbortSignal,
  ): Promise<SyntheticContract> {
    const result = await apiClient.POST(
      "/local/booking-trial/contracts/{contractId}/sign",
      {
        params: { path: { contractId } },
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as SyntheticContract;
  }

  async rejectContract(
    contractId: string,
    signal?: AbortSignal,
  ): Promise<SyntheticContract> {
    const result = await apiClient.POST(
      "/local/booking-trial/contracts/{contractId}/reject",
      {
        params: { path: { contractId } },
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as SyntheticContract;
  }

  async downloadContractPdf(
    contractId: string,
    signal?: AbortSignal,
  ): Promise<Blob | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/contracts/{contractId}/document",
      {
        params: { path: { contractId } },
        parseAs: "blob",
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async recordCheckIn(
    idempotencyKey: string,
    reservationId: string,
    input: {
      occurred_at?: string;
      time_zone?: string;
      comments?: string;
      fixture_evidence_code?:
        "sin_evidencia" | "synthetic-png-v1" | "synthetic-damage-png-v1";
    },
    signal?: AbortSignal,
  ): Promise<RentalOperation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/check-in",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as RentalOperation;
  }

  async recordCheckOut(
    idempotencyKey: string,
    reservationId: string,
    input: {
      occurred_at?: string;
      time_zone?: string;
      comments?: string;
      fixture_evidence_code?:
        "sin_evidencia" | "synthetic-png-v1" | "synthetic-damage-png-v1";
    },
    signal?: AbortSignal,
  ): Promise<RentalOperation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/check-out",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as RentalOperation;
  }

  async recordReception(
    idempotencyKey: string,
    reservationId: string,
    input: {
      result?:
        "registrada" | "recepcion_conforme" | "recepcion_con_observaciones";
      observations?: string;
    },
    signal?: AbortSignal,
  ): Promise<RentalOperation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/reception",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as RentalOperation;
  }

  async listOperations(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<RentalOperation[]> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationID}/operations",
      {
        params: { path: { reservationID: reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return [];
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data?.items as unknown as RentalOperation[]) ?? [];
  }

  async downloadEvidence(
    reservationId: string,
    evidenceId: string,
    signal?: AbortSignal,
  ): Promise<Blob | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationID}/evidence/{evidenceID}",
      {
        params: {
          path: { reservationID: reservationId, evidenceID: evidenceId },
        },
        parseAs: "blob",
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data as Blob) ?? (await result.response.blob());
  }

  async listMessages(
    reservationId: string,
    query?: { before?: number | string; limit?: number },
    signal?: AbortSignal,
  ): Promise<ReservationMessage[]> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}/messages",
      {
        params: {
          path: { reservationId },
          query,
        },
        signal,
      },
    );
    if (result.response.status === 404) return [];
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data?.items as unknown as ReservationMessage[]) ?? [];
  }

  async sendMessage(
    idempotencyKey: string,
    reservationId: string,
    body: string,
    signal?: AbortSignal,
  ): Promise<ReservationMessage> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/messages",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { body },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as ReservationMessage;
  }

  async advanceReadCursor(
    reservationId: string,
    throughSequence: number | string,
    signal?: AbortSignal,
  ): Promise<{ sequence: number }> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/messages/read",
      {
        params: { path: { reservationId } },
        body: { through_sequence: throughSequence },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return { sequence: Number(result.data.data.read_through_sequence) };
  }

  async listReviews(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<ReservationReview[]> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationID}/reviews",
      {
        params: { path: { reservationID: reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return [];
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data?.items as unknown as ReservationReview[]) ?? [];
  }

  async createReview(
    idempotencyKey: string,
    reservationId: string,
    input: {
      rating: number;
      comment?: string;
      target_type?: "espacio" | "arrendatario";
    },
    signal?: AbortSignal,
  ): Promise<ReservationReview> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/reviews",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as ReservationReview;
  }

  async reportReview(
    idempotencyKey: string,
    reservationId: string,
    reviewId: string,
    input: {
      reason_code:
        "insultos_acoso" | "datos_personales" | "spam" | "ajeno_experiencia";
      comment?: string;
    },
    signal?: AbortSignal,
  ): Promise<ReviewReport> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/reviews/{reviewID}/report",
      {
        params: {
          path: { reservationID: reservationId, reviewID: reviewId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: input,
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as ReviewReport;
  }

  async listDisputes(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<TrialDispute[]> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservation_id}/disputes",
      {
        params: { path: { reservation_id: reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return [];
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.items as unknown as TrialDispute[]) ?? [];
  }

  async openDispute(
    idempotencyKey: string,
    reservationId: string,
    _reasonCode: string,
    signal?: AbortSignal,
  ): Promise<TrialDispute> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservation_id}/disputes",
      {
        params: {
          path: { reservation_id: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { reason_code: "ensayo_privacidad" },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data) throw new ApiProtocolError();
    return result.data as unknown as TrialDispute;
  }

  async getDisputeHistory(
    disputeId: string,
    signal?: AbortSignal,
  ): Promise<TrialDisputeTransition[]> {
    const result = await apiClient.GET(
      "/local/booking-trial/disputes/{dispute_id}/history",
      {
        params: { path: { dispute_id: disputeId } },
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.items as unknown as TrialDisputeTransition[]) ?? [];
  }

  async getDamageClaim(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<DamageClaim | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationID}/damage-claim",
      {
        params: { path: { reservationID: reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data as unknown as DamageClaim) ?? null;
  }

  async openDamageClaim(
    idempotencyKey: string,
    reservationId: string,
    description: string,
    signal?: AbortSignal,
  ): Promise<DamageClaim> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/damage-claim",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { description },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as DamageClaim;
  }

  async submitDamageClaimDefense(
    idempotencyKey: string,
    reservationId: string,
    description: string,
    signal?: AbortSignal,
  ): Promise<DamageClaim> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/damage-claim/defense",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { description },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    const updated = await this.getDamageClaim(reservationId, signal);
    if (updated) return updated;
    throw new ApiProtocolError();
  }

  async createDamageClaimDefenseEvidence(
    idempotencyKey: string,
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<RentalOperation> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationID}/damage-claim/defense/evidence",
      {
        params: {
          path: { reservationID: reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: {},
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as RentalOperation;
  }

  async getGuarantee(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<Guarantee | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}/guarantee",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data as unknown as Guarantee) ?? null;
  }

  async executeGuaranteeOperation(
    idempotencyKey: string,
    reservationId: string,
    input: {
      kind: "autorizacion" | "captura" | "liberacion";
      amount_clp: number;
      outcome?: SimulatedPaymentOutcome;
    },
    signal?: AbortSignal,
  ): Promise<Guarantee> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/guarantee",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: {
          kind: input.kind,
          amount_clp: input.amount_clp,
          outcome: input.outcome ?? "exito",
        },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as Guarantee;
  }

  async getSettlement(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<Settlement | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}/settlement",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return (result.data?.data as unknown as Settlement) ?? null;
  }

  async requestSettlement(
    idempotencyKey: string,
    reservationId: string,
    outcome?: SimulatedPaymentOutcome,
    signal?: AbortSignal,
  ): Promise<Settlement> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/settlement",
      {
        params: {
          path: { reservationId },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body: { outcome: outcome ?? "exito" },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as Settlement;
  }

  async reconcileSettlement(
    reservationId: string,
    outcome: "exito" | "rechazo",
    signal?: AbortSignal,
  ): Promise<Settlement> {
    const result = await apiClient.POST(
      "/local/booking-trial/reservations/{reservationId}/settlement-reconcile",
      {
        params: { path: { reservationId } },
        body: { outcome },
        bodySerializer: serializeExactJson,
        signal,
      },
    );
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) throw new ApiProtocolError();
    return result.data.data as unknown as Settlement;
  }

  async getSettlementReceipt(
    reservationId: string,
    signal?: AbortSignal,
  ): Promise<SettlementReceipt | null> {
    const result = await apiClient.GET(
      "/local/booking-trial/reservations/{reservationId}/receipt",
      {
        params: { path: { reservationId } },
        signal,
      },
    );
    if (result.response.status === 404) return null;
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    if (!result.data?.data) return null;
    const r = result.data.data;
    return {
      receipt_id: r.receipt_id,
      reservation_id: r.reservation_id,
      policy_version: r.policy_version,
      currency: r.currency,
      gross_rent_clp: r.gross_rent_clp,
      confirmed_refunds_clp: r.confirmed_refunds_clp,
      commission_clp: r.commission_clp,
      transfer_clp: r.transfer_clp,
      confirmed_at: r.confirmed_at,
      banner: r.banner,
    };
  }

  async getBusinessClock(signal?: AbortSignal): Promise<BusinessClock> {
    const result = await apiClient.GET("/local/booking-trial/business-clock", {
      signal,
    });
    if (result.response.status === 404) {
      return {
        current_time: new Date().toISOString(),
        time_zone: "America/Santiago",
      };
    }
    if (!result.response.ok) {
      throw normalizeApiError(result.response, result.error);
    }
    return {
      current_time: result.data?.data?.now ?? new Date().toISOString(),
      time_zone: "America/Santiago",
    };
  }
}
