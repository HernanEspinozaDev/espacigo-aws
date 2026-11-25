"use client";

import { useEffect, useRef, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { Button } from "@/components/workspace/ui/button";
import { commercialAftercare } from "../infrastructure/commercial-aftercare-openapi-adapter";
import {
  sanitizeMessageBody,
  validateReviewComment,
} from "../domain/booking-rules";
import { useAftercareRequest } from "./use-aftercare-request";

export type ParticipantPanelProps = {
  reservationId: string;
  participantRole?: "anfitrion" | "arrendatario";
};

type MessagePage = Awaited<ReturnType<typeof commercialAftercare.messages>>;
type Reviews = Awaited<ReturnType<typeof commercialAftercare.reviews>>;
const inputStyle = "w-full rounded-md border bg-background p-2";

export function CommercialConversationPanel(props: ParticipantPanelProps) {
  const { snapshot } = useAuth();
  if (snapshot.status !== "authenticated") return null;
  return (
    <Conversation
      key={`${snapshot.accountId}:${props.reservationId}`}
      {...props}
      accountId={snapshot.accountId ?? ""}
    />
  );
}

function Conversation({
  reservationId,
  participantRole,
  accountId,
}: ParticipantPanelProps & { accountId: string }) {
  const [page, setPage] = useState<MessagePage>();
  const [previous, setPrevious] = useState<MessagePage["items"]>([]);
  const [older, setOlder] = useState<number | null | undefined>();
  const [body, setBody] = useState("");
  const [loadError, setLoadError] = useState(false);
  const [validation, setValidation] = useState("");
  const readerCursor = useRef(0);
  const mutation = useAftercareRequest();
  const items = [
    ...new Map(
      [...previous, ...(page?.items ?? [])].map((item) => [
        item.sequence,
        item,
      ]),
    ).values(),
  ].sort((a, b) => a.sequence - b.sequence);

  useEffect(() => {
    let current = true;
    let loading = false;
    async function refresh() {
      if (loading || document.visibilityState !== "visible") return;
      loading = true;
      try {
        const next = await commercialAftercare.messages(reservationId);
        if (!current) return;
        setPage((existing) =>
          (existing?.items.at(-1)?.sequence ?? 0) >
          (next.items.at(-1)?.sequence ?? 0)
            ? existing
            : next,
        );
        setLoadError(false);
        const sequence = next.items.at(-1)?.sequence ?? 0;
        if (sequence > readerCursor.current) {
          await commercialAftercare.markRead(reservationId, sequence);
          readerCursor.current = sequence;
        }
      } catch {
        if (current) setLoadError(true);
      } finally {
        loading = false;
      }
    }
    void refresh();
    const timer = window.setInterval(refresh, 10_000);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      current = false;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [reservationId]);

  const olderCursor = older === undefined ? page?.older_cursor : older;
  const counterparty =
    participantRole === "anfitrion"
      ? "Arrendatario"
      : participantRole === "arrendatario"
        ? "Anfitrión"
        : "Contraparte";
  return (
    <section
      aria-label="Mensajería privada"
      className="space-y-4 rounded-lg border p-4"
    >
      {loadError && <p role="alert">No se pudo actualizar la conversación.</p>}
      {olderCursor != null && (
        <Button
          variant="outline"
          disabled={mutation.busy}
          onClick={() => {
            void mutation.run(`older:${olderCursor}`, async () => {
              const next = await commercialAftercare.messages(
                reservationId,
                olderCursor,
              );
              setPrevious((current) => [
                ...next.items,
                ...current,
                ...(page?.items ?? []),
              ]);
              setOlder(next.older_cursor);
            });
          }}
        >
          Ver mensajes anteriores
        </Button>
      )}
      <ol className="max-h-96 space-y-3 overflow-y-auto">
        {items.map((item) => (
          <li key={item.id} className="rounded-md border p-3">
            <p className="text-sm font-semibold">
              {item.author_id === accountId ? "Tú" : counterparty}
            </p>
            <p className="whitespace-pre-wrap break-words">{item.body}</p>
          </li>
        ))}
      </ol>
      {page && items.length === 0 && <p>Todavía no hay mensajes.</p>}
      <form
        className="space-y-2"
        onSubmit={(event) => {
          event.preventDefault();
          const result = sanitizeMessageBody(body);
          if (!result.valid || !result.sanitized) {
            setValidation(result.error ?? "Escribe un mensaje válido.");
            return;
          }
          setValidation("");
          const text = result.sanitized;
          void mutation.run(`send:${text}`, async (key) => {
            const sent = await commercialAftercare.send(
              reservationId,
              text,
              key,
            );
            setPage((current) =>
              current?.items.some((item) => item.id === sent.id)
                ? current
                : {
                    items: [...(current?.items ?? []), sent],
                    older_cursor: current?.older_cursor ?? null,
                  },
            );
            setBody("");
          });
        }}
      >
        <label htmlFor="commercial-message">Mensaje para la contraparte</label>
        <textarea
          id="commercial-message"
          value={body}
          onChange={(event) => setBody(event.target.value)}
          rows={3}
          className={inputStyle}
          required
        />
        <Button type="submit" disabled={mutation.busy}>
          Enviar mensaje
        </Button>
      </form>
      {(validation || mutation.error) && (
        <p role="alert">{validation || mutation.error}</p>
      )}
    </section>
  );
}

export function CommercialReviewsPanel(props: ParticipantPanelProps) {
  const { snapshot } = useAuth();
  if (snapshot.status !== "authenticated") return null;
  return (
    <ReviewsPanel
      key={`${snapshot.accountId}:${props.reservationId}`}
      {...props}
    />
  );
}

function ReviewsPanel({
  reservationId,
  participantRole,
}: ParticipantPanelProps) {
  const [reviews, setReviews] = useState<Reviews>();
  const [loadError, setLoadError] = useState(false);
  const [rating, setRating] = useState(5);
  const [comment, setComment] = useState("");
  const [validation, setValidation] = useState("");
  const mutation = useAftercareRequest();
  useEffect(() => {
    let current = true;
    commercialAftercare
      .reviews(reservationId)
      .then((value) => {
        if (current) setReviews(value);
      })
      .catch(() => {
        if (current) setLoadError(true);
      });
    return () => {
      current = false;
    };
  }, [reservationId]);

  const label =
    participantRole === "anfitrion"
      ? "Calificar al arrendatario"
      : "Calificar el espacio";
  return (
    <section
      aria-label="Reseñas de la reserva"
      className="space-y-4 rounded-lg border p-4"
    >
      {loadError && <p role="alert">No se pudieron cargar las reseñas.</p>}
      {reviews && !reviews.can_review && (
        <p>
          {reviews.review_unavailable_reason === "reservation_not_ended"
            ? `Podrás dejar tu reseña cuando finalice el horario de tu reserva (${new Date(reviews.reservation_ends_at).toLocaleString("es-CL", { timeZone: "America/Santiago" })}).`
            : reviews.review_unavailable_reason === "already_reviewed"
              ? "Ya publicaste una reseña para esta reserva."
              : "La reserva no admite una reseña en este momento."}
        </p>
      )}
      {reviews?.items.map((item) => (
        <article key={item.id} className="rounded-md border p-3">
          <p className="font-semibold">
            {item.target_type === "arrendatario"
              ? "Reseña del arrendatario"
              : "Reseña del espacio"}{" "}
            · {item.rating}/5
          </p>
          <p className="whitespace-pre-wrap break-words">{item.comment}</p>
        </article>
      ))}
      {reviews?.can_review && (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            const result = validateReviewComment(comment);
            if (!result.valid) {
              setValidation(result.error ?? "Comentario inválido.");
              return;
            }
            setValidation("");
            const text = result.sanitized ?? "";
            void mutation.run(`${rating}:${text}`, async (key) => {
              const item = await commercialAftercare.review(
                reservationId,
                rating,
                text,
                key,
              );
              setReviews(
                (current) =>
                  current && {
                    ...current,
                    can_review: false,
                    review_unavailable_reason: "already_reviewed",
                    items: [
                      ...current.items.filter(
                        (review) => review.id !== item.id,
                      ),
                      item,
                    ],
                  },
              );
              setComment("");
            });
          }}
        >
          <label htmlFor="commercial-rating">{label}</label>
          <select
            id="commercial-rating"
            value={rating}
            className={inputStyle}
            onChange={(event) => setRating(Number(event.target.value))}
          >
            {[5, 4, 3, 2, 1].map((value) => (
              <option key={value} value={value}>
                {value} de 5
              </option>
            ))}
          </select>
          <label htmlFor="commercial-review-comment">Comentario</label>
          <textarea
            id="commercial-review-comment"
            value={comment}
            className={inputStyle}
            rows={3}
            onChange={(event) => setComment(event.target.value)}
          />
          <Button type="submit" disabled={mutation.busy}>
            Publicar reseña
          </Button>
        </form>
      )}
      {(validation || mutation.error) && (
        <p role="alert">{validation || mutation.error}</p>
      )}
    </section>
  );
}
