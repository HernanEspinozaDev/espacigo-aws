"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  type AccountTrustedContact,
  type AddAccountTrustedContactInput,
  SecurityLocationTrustedError,
  addAccountTrustedContact,
  listAccountTrustedContacts,
  removeAccountTrustedContact,
} from "../security-location-trusted-api";

export interface UseTrustedContactsReturn {
  contacts: AccountTrustedContact[];
  isLoading: boolean;
  isAdding: boolean;
  removingContactId: string | null;
  error: string | null;
  notification: string | null;
  refresh: (signal?: AbortSignal) => Promise<void>;
  addContact: (input: AddAccountTrustedContactInput) => Promise<boolean>;
  removeContact: (contactId: string) => Promise<boolean>;
  clearError: () => void;
  clearNotification: () => void;
}

export function useTrustedContacts(): UseTrustedContactsReturn {
  const { checkSession } = useAuth();

  const [contacts, setContacts] = useState<AccountTrustedContact[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isAdding, setIsAdding] = useState(false);
  const [removingContactId, setRemovingContactId] = useState<string | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  const handleUnauthorized = useCallback(async () => {
    try {
      await checkSession();
    } catch {
      // ignore
    }
  }, [checkSession]);

  const clearError = useCallback(() => setError(null), []);
  const clearNotification = useCallback(() => setNotification(null), []);

  const refresh = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setIsLoading(true);
        setError(null);
        const list = await listAccountTrustedContacts(signal);
        setContacts(list);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          await handleUnauthorized();
        }

        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar la lista de personas de confianza.",
        );
      } finally {
        setIsLoading(false);
      }
    },
    [handleUnauthorized],
  );

  useEffect(() => {
    let active = true;
    const controller = new AbortController();

    listAccountTrustedContacts(controller.signal)
      .then((list) => {
        if (!active) return;
        setContacts(list);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!active) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (err instanceof Error && err.name === "AbortError") return;

        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          handleUnauthorized().catch(() => {});
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo cargar la lista de personas de confianza.",
        );
        setIsLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [handleUnauthorized]);

  const addContact = useCallback(
    async (input: AddAccountTrustedContactInput): Promise<boolean> => {
      try {
        setIsAdding(true);
        setError(null);
        const newContact = await addAccountTrustedContact(input);
        setContacts((prev) => [newContact, ...prev]);
        setNotification("Contacto de confianza agregado exitosamente.");
        return true;
      } catch (err) {
        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo agregar la persona de confianza.",
        );
        return false;
      } finally {
        setIsAdding(false);
      }
    },
    [handleUnauthorized],
  );

  const removeContact = useCallback(
    async (contactId: string): Promise<boolean> => {
      try {
        setRemovingContactId(contactId);
        setError(null);
        await removeAccountTrustedContact(contactId);
        setContacts((prev) => prev.filter((c) => c.id !== contactId));
        setNotification("El contacto de confianza fue retirado.");
        return true;
      } catch (err) {
        if (err instanceof SecurityLocationTrustedError && err.status === 401) {
          await handleUnauthorized();
        }
        setError(
          err instanceof Error
            ? err.message
            : "No se pudo revocar el contacto de confianza.",
        );
        return false;
      } finally {
        setRemovingContactId(null);
      }
    },
    [handleUnauthorized],
  );

  return {
    contacts,
    isLoading,
    isAdding,
    removingContactId,
    error,
    notification,
    refresh,
    addContact,
    removeContact,
    clearError,
    clearNotification,
  };
}
