import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-qr-test-"),
);
const originalFetch = globalThis.fetch;

async function loadModule(relPath, outName) {
  const source = (await readFile(path.join(root, relPath), "utf8")).replace(
    'import "client-only";',
    "",
  );
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  });
  const file = path.join(scratch, outName);
  await writeFile(file, outputText);
  return import(pathToFileURL(file).href);
}

const qrApi = await loadModule(
  "src/features/account/qr-verification-api.ts",
  "qr-verification-api.mjs",
);

after(async () => rm(scratch, { recursive: true, force: true }));
afterEach(() => {
  globalThis.fetch = originalFetch;
});

const jsonResponse = (body, status = 200, headers = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });

describe("QR Verification API Client and Domain", () => {
  it("return-target includes /mi-cuenta/seguridad/metodos/codigo-qr in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(source, /"\/mi-cuenta\/seguridad\/metodos\/codigo-qr"/);
  });

  describe("getAccountQRStatus", () => {
    it("fetches QR status successfully with required BFF options", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/qr");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          enabled: true,
          enabled_at: "2026-11-19T07:30:00Z",
          trusted_device_label: "iPhone 15 Pro",
        });
      };

      const result = await qrApi.getAccountQRStatus();
      assert.deepEqual(result, {
        enabled: true,
        enabled_at: "2026-11-19T07:30:00Z",
        trusted_device_label: "iPhone 15 Pro",
      });
    });

    it("returns safe defaults when optional fields are null or omitted", async () => {
      globalThis.fetch = async () =>
        jsonResponse({
          enabled: false,
        });

      const result = await qrApi.getAccountQRStatus();
      assert.equal(result.enabled, false);
      assert.equal(result.enabled_at, null);
      assert.equal(result.trusted_device_label, null);
    });

    it("maps 401 unauthenticated error to user-friendly message", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "unauthenticated" } }, 401);

      await assert.rejects(
        async () => qrApi.getAccountQRStatus(),
        (err) => {
          assert.equal(err.name, "QRApiError");
          assert.equal(err.status, 401);
          assert.equal(err.code, "unauthenticated");
          assert.match(err.message, /sesión no es válida/i);
          return true;
        },
      );
    });
  });

  describe("createAccountQRTicket", () => {
    it("generates a new QR ticket with 120s TTL payload", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/qr/ticket");
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          ticket_id: "c8c4da40-844c-47ea-8d4e-b52b3636f890",
          qr_payload: "v1.qr.secret-random-token-base64",
          expires_at: "2026-11-19T07:32:00Z",
        });
      };

      const result = await qrApi.createAccountQRTicket();
      assert.equal(result.ticket_id, "c8c4da40-844c-47ea-8d4e-b52b3636f890");
      assert.equal(result.qr_payload, "v1.qr.secret-random-token-base64");
      assert.equal(result.expires_at, "2026-11-19T07:32:00Z");
    });
  });

  describe("getAccountQRTicketStatus", () => {
    it("fetches ticket status by ticket ID", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/qr/ticket/c8c4da40-844c-47ea-8d4e-b52b3636f890",
        );
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");

        return jsonResponse({
          ticket_id: "c8c4da40-844c-47ea-8d4e-b52b3636f890",
          status: "pending",
          expires_at: "2026-11-19T07:32:00Z",
          resolved_at: null,
        });
      };

      const result = await qrApi.getAccountQRTicketStatus(
        "c8c4da40-844c-47ea-8d4e-b52b3636f890",
      );
      assert.equal(result.status, "pending");
      assert.equal(result.ticket_id, "c8c4da40-844c-47ea-8d4e-b52b3636f890");
      assert.equal(result.resolved_at, null);
    });

    it("normalizes unknown status to pending fallback", async () => {
      globalThis.fetch = async () =>
        jsonResponse({
          ticket_id: "abc",
          status: "something_else",
          expires_at: "2026-11-19T07:32:00Z",
        });

      const result = await qrApi.getAccountQRTicketStatus("abc");
      assert.equal(result.status, "pending");
    });

    it("maps 404 ticket not found to clean user message", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "ticket_not_found" } }, 404);

      await assert.rejects(
        async () => qrApi.getAccountQRTicketStatus("invalid-id"),
        (err) => {
          assert.equal(err.name, "QRApiError");
          assert.equal(err.status, 404);
          assert.match(err.message, /no fue encontrado o no existe/i);
          return true;
        },
      );
    });
  });

  describe("approveAccountQRTicket", () => {
    it("approves ticket with token and device label", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/qr/approve");
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");

        const parsedBody = JSON.parse(init.body);
        assert.equal(parsedBody.token, "my-token-123");
        assert.equal(parsedBody.device_label, "Samsung Galaxy S24");

        return jsonResponse({
          status: "approved",
          resolved_at: "2026-11-19T07:31:00Z",
        });
      };

      const result = await qrApi.approveAccountQRTicket({
        token: "my-token-123",
        device_label: "Samsung Galaxy S24",
      });

      assert.equal(result.status, "approved");
      assert.equal(result.resolved_at, "2026-11-19T07:31:00Z");
    });

    it("maps ticket_expired error (422) properly", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: { code: "ticket_expired", message: "El ticket ha expirado" },
          },
          422,
        );

      await assert.rejects(
        async () => qrApi.approveAccountQRTicket({ token: "expired-token" }),
        (err) => {
          assert.equal(err.name, "QRApiError");
          assert.equal(err.status, 422);
          assert.match(err.message, /código QR ha expirado/i);
          return true;
        },
      );
    });

    it("maps ticket_already_resolved error (409) properly", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "ticket_already_resolved" } }, 409);

      await assert.rejects(
        async () => qrApi.approveAccountQRTicket({ token: "used-token" }),
        (err) => {
          assert.equal(err.name, "QRApiError");
          assert.equal(err.status, 409);
          assert.match(err.message, /ya fue utilizado o resuelto/i);
          return true;
        },
      );
    });
  });

  describe("disableAccountQR", () => {
    it("disables QR verification with DELETE method", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/qr");
        assert.equal(init.method, "DELETE");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");

        return jsonResponse({ enabled: false });
      };

      const result = await qrApi.disableAccountQR();
      assert.equal(result.enabled, false);
    });

    it("maps qr_not_enabled error (400) properly", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "qr_not_enabled" } }, 400);

      await assert.rejects(
        async () => qrApi.disableAccountQR(),
        (err) => {
          assert.equal(err.name, "QRApiError");
          assert.equal(err.status, 400);
          assert.match(err.message, /no se encuentra activado/i);
          return true;
        },
      );
    });
  });
});
