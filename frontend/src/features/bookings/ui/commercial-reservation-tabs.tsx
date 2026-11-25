"use client";

import { useEffect, useId, useState } from "react";
import { MessageSquare, Star, ShieldAlert } from "lucide-react";
import { BookingConversationPanel } from "./booking-conversation-panel";
import { BookingReviewsPanel } from "./booking-reviews-panel";
import { BookingDisputesPanel } from "./booking-disputes-panel";
import { useBookings } from "./use-bookings";
import { useAuth } from "@/features/auth/ui/use-auth";
import { commercialAftercare } from "../infrastructure/commercial-aftercare-openapi-adapter";

type ReservationTab = "messages" | "reviews" | "cases";

interface CommercialReservationTabsProps {
  reservationId: string;
  unreadCount?: number;
}

const tabs: {
  id: ReservationTab;
  label: string;
  Icon: typeof MessageSquare;
}[] = [
  { id: "messages", label: "Mensajería privada", Icon: MessageSquare },
  { id: "reviews", label: "Reseñas", Icon: Star },
  { id: "cases", label: "Disputas y reclamos", Icon: ShieldAlert },
];

/** Tabs keep future commercial post-sale panels lazy until their tab is selected. */
export function CommercialReservationTabs({
  reservationId,
  unreadCount = 0,
}: CommercialReservationTabsProps) {
  const [activeTab, setActiveTab] = useState<ReservationTab>("messages");
  const id = useId();
  const { currentReservation, messages, reviews, disputes } = useBookings();
  const { snapshot } = useAuth();
  const [summary, setSummary] = useState<{
    key: string;
    counts: Awaited<ReturnType<typeof commercialAftercare.summary>>;
  }>();
  const key = `${snapshot.accountId}:${reservationId}`;
  const reservationReady = currentReservation?.id === reservationId;
  const commercial =
    currentReservation?.id === reservationId &&
    currentReservation.is_commercial === true;
  useEffect(() => {
    if (!commercial || snapshot.status !== "authenticated") return;
    let current = true;
    let loading = false;
    async function refresh() {
      if (loading || document.visibilityState !== "visible") return;
      loading = true;
      try {
        const counts = await commercialAftercare.summary(reservationId);
        if (current) setSummary({ key, counts });
      } catch {
        // The tab contents expose their own network status.
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
  }, [commercial, key, reservationId, snapshot.status, activeTab]);
  const counts = summary?.key === key ? summary.counts : undefined;
  const effectiveUnreadCount =
    unreadCount ||
    (reservationReady ? (currentReservation?.unread_count ?? 0) : 0);
  const badgeCounts: Record<ReservationTab, number> = {
    messages: commercial
      ? (counts?.unread_messages ?? 0)
      : Math.max(effectiveUnreadCount, messages.length),
    reviews: commercial ? (counts?.reviews ?? 0) : reviews.length,
    cases: commercial ? (counts?.open_cases ?? 0) : disputes.length,
  };

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const currentIndex = tabs.findIndex((tab) => tab.id === activeTab);
    let nextIndex: number | null = null;
    if (event.key === "ArrowRight")
      nextIndex = (currentIndex + 1) % tabs.length;
    if (event.key === "ArrowLeft")
      nextIndex = (currentIndex + tabs.length - 1) % tabs.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = tabs.length - 1;
    if (nextIndex !== null) {
      event.preventDefault();
      const nextTab = tabs[nextIndex].id;
      setActiveTab(nextTab);
      document.getElementById(`${id}-tab-${nextTab}`)?.focus();
    }
  }

  return (
    <section className="rounded-2xl border border-border bg-card shadow-sm">
      <div
        role="tablist"
        aria-label="Información y soporte de la reserva"
        onKeyDown={handleKeyDown}
        className="flex flex-wrap gap-1 border-b border-border p-2"
      >
        {tabs.map(({ id: tab, label, Icon }) => (
          <button
            key={tab}
            id={`${id}-tab-${tab}`}
            type="button"
            role="tab"
            aria-selected={activeTab === tab}
            aria-controls={`${id}-panel-${tab}`}
            tabIndex={activeTab === tab ? 0 : -1}
            onClick={() => setActiveTab(tab)}
            className={`inline-flex min-h-10 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
              activeTab === tab
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
          >
            <Icon aria-hidden="true" className="h-4 w-4" />
            {label}
            {badgeCounts[tab] > 0 && (
              <span
                className="rounded-full bg-background/20 px-2 py-0.5 text-xs"
                aria-label={`${badgeCounts[tab]} ${tab === "messages" ? "mensajes" : tab === "reviews" ? "reseñas" : "casos"}`}
              >
                {badgeCounts[tab]}
              </span>
            )}
          </button>
        ))}
      </div>
      {tabs.map(({ id: tab }) =>
        activeTab === tab ? (
          <div
            key={tab}
            id={`${id}-panel-${tab}`}
            role="tabpanel"
            aria-labelledby={`${id}-tab-${tab}`}
            tabIndex={0}
            className="p-3 sm:p-5"
          >
            {!reservationReady ? (
              <p
                role="status"
                className="py-6 text-center text-sm text-muted-foreground"
              >
                Cargando los datos de la reserva…
              </p>
            ) : tab === "messages" ? (
              <BookingConversationPanel
                reservationId={reservationId}
                commercial={commercial}
                participantRole={
                  currentReservation?.id === reservationId
                    ? currentReservation.participant_role
                    : undefined
                }
              />
            ) : tab === "reviews" ? (
              <BookingReviewsPanel
                reservationId={reservationId}
                commercial={commercial}
                participantRole={
                  currentReservation?.id === reservationId
                    ? currentReservation.participant_role
                    : undefined
                }
              />
            ) : (
              <BookingDisputesPanel
                reservationId={reservationId}
                commercial={commercial}
                participantRole={
                  currentReservation?.id === reservationId
                    ? currentReservation.participant_role
                    : undefined
                }
              />
            )}
          </div>
        ) : null,
      )}
    </section>
  );
}
