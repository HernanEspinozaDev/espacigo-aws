"use client";

import { Bell, CheckCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { apiClient } from "@/lib/api/client";
import type { components } from "@/lib/api/generated/openapi";
import { useSessionIdentity } from "@/features/auth/ui/use-session-identity";
import { cn } from "@/lib/utils";

type UserNotification = components["schemas"]["UserNotification"];

function formatNotificationTime(iso: string) {
  const timestamp = Date.parse(iso);
  if (!Number.isFinite(timestamp)) return "";
  const diffMinutes = Math.floor((Date.now() - timestamp) / 60_000);
  if (diffMinutes < 1) return "Ahora";
  if (diffMinutes < 60) return `Hace ${diffMinutes} min`;
  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours < 24) return `Hace ${diffHours} h`;
  return new Intl.DateTimeFormat("es-CL", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(timestamp));
}

export function NotificationBell() {
  const { status, accountId } = useSessionIdentity();
  if (status !== "authenticated" || !accountId) return null;
  return <NotificationBellInner key={accountId} />;
}

function NotificationBellInner() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [items, setItems] = useState<UserNotification[]>([]);
  const [loading, setLoading] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);

  const loadNotifications = useCallback(async (signal?: AbortSignal) => {
    try {
      const result = await apiClient.GET("/notifications", { signal });
      if (!result.response.ok || !result.data) return;
      setUnreadCount(result.data.unread_count);
      setItems(result.data.items);
    } catch {
      // Keep previous state on transient network/abort errors.
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    queueMicrotask(() => {
      if (!controller.signal.aborted) {
        void loadNotifications(controller.signal);
      }
    });

    const intervalId = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void loadNotifications(controller.signal);
      }
    }, 12_000);

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        void loadNotifications(controller.signal);
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      controller.abort();
      window.clearInterval(intervalId);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [loadNotifications]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  async function handleSelect(item: UserNotification) {
    setOpen(false);
    if (!item.read_at) {
      const nowIso = new Date().toISOString();
      setItems((prev) =>
        prev.map((entry) =>
          entry.id === item.id ? { ...entry, read_at: nowIso } : entry,
        ),
      );
      setUnreadCount((prev) => Math.max(0, prev - 1));
      try {
        await apiClient.POST("/notifications/{notificationID}/read", {
          params: { path: { notificationID: item.id } },
        });
      } catch {
        // Navigation still proceeds.
      }
    }
    router.push(item.href);
  }

  async function handleReadAll() {
    setUnreadCount(0);
    const nowIso = new Date().toISOString();
    setItems((prev) =>
      prev.map((entry) => ({ ...entry, read_at: entry.read_at ?? nowIso })),
    );
    try {
      await apiClient.POST("/notifications/read-all");
    } catch {
      void loadNotifications();
    }
  }

  return (
    <div ref={containerRef} className="relative inline-flex items-center">
      <button
        type="button"
        aria-label={
          unreadCount > 0
            ? `Notificaciones (${unreadCount} sin leer)`
            : "Notificaciones"
        }
        aria-expanded={open}
        onClick={() => {
          const next = !open;
          setOpen(next);
          if (next) void loadNotifications();
        }}
        className="relative inline-flex size-9 items-center justify-center rounded-full text-gray-700 transition-colors hover:bg-secondary hover:text-gray-900"
      >
        <Bell className="size-5" aria-hidden="true" />
        {unreadCount > 0 && (
          <span className="absolute -right-0.5 -top-0.5 inline-flex min-w-4 h-4 items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-bold leading-none text-white">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <section
          aria-label="Notificaciones"
          className="absolute right-0 top-full z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] rounded-xl border border-border bg-white shadow-lg"
        >
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <h2 className="text-sm font-semibold text-gray-900">
              Notificaciones
            </h2>
            {unreadCount > 0 && (
              <button
                type="button"
                onClick={() => void handleReadAll()}
                className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
              >
                <CheckCheck className="size-3.5" aria-hidden="true" />
                Marcar todo leído
              </button>
            )}
          </div>

          <div className="max-h-96 overflow-y-auto divide-y divide-border/60">
            {loading && items.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-foreground">
                Cargando notificaciones…
              </p>
            ) : items.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-foreground">
                No tienes notificaciones por ahora.
              </p>
            ) : (
              items.map((item) => {
                const unread = !item.read_at;
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => void handleSelect(item)}
                    className={cn(
                      "flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/50",
                      unread && "bg-primary/5",
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        "mt-1.5 size-2 shrink-0 rounded-full",
                        unread ? "bg-primary" : "bg-transparent",
                      )}
                    />
                    <div className="min-w-0 flex-1 space-y-0.5">
                      <p
                        className={cn(
                          "text-sm leading-snug text-gray-900",
                          unread ? "font-semibold" : "font-medium",
                        )}
                      >
                        {item.title}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {item.message}
                      </p>
                      <p className="text-[11px] text-muted-foreground/80">
                        {formatNotificationTime(item.created_at)}
                      </p>
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </section>
      )}
    </div>
  );
}
