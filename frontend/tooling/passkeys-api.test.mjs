import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-passkeys-test-"),
);
const originalFetch = globalThis.fetch;
const originalNavigatorDesc = Object.getOwnPropertyDescriptor(
  globalThis,
  "navigator",
);

function setMockNavigator(mockObj) {
  Object.defineProperty(globalThis, "navigator", {
    value: mockObj,
    configurable: true,
    writable: true,
  });
}

function restoreNavigator() {
  if (originalNavigatorDesc) {
    Object.defineProperty(globalThis, "navigator", originalNavigatorDesc);
  } else {
    delete globalThis.navigator;
  }
}

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

const passkeysApi = await loadModule(
  "src/features/account/passkeys-api.ts",
  "passkeys-api.mjs",
);

after(async () => rm(scratch, { recursive: true, force: true }));
afterEach(() => {
  globalThis.fetch = originalFetch;
  restoreNavigator();
});

const jsonResponse = (body, status = 200, headers = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });

describe("Passkeys / WebAuthn API Client and Helpers", () => {
  it("return-target includes /mi-cuenta/seguridad/metodos/llaves-de-acceso in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(
      source,
      /"\/mi-cuenta\/seguridad\/metodos\/llaves-de-acceso"/,
    );
  });

  describe("WebAuthn Base64URL and Encoding Helpers", () => {
    it("converts Uint8Array to base64url and back accurately", () => {
      const originalBytes = new Uint8Array([0, 1, 2, 250, 254, 255, 42, 100]);
      const base64url = passkeysApi.uint8ArrayToBase64Url(originalBytes);

      assert.ok(!base64url.includes("+"));
      assert.ok(!base64url.includes("/"));
      assert.ok(!base64url.includes("="));

      const decodedBytes = passkeysApi.base64UrlToUint8Array(base64url);
      assert.deepEqual(Array.from(decodedBytes), Array.from(originalBytes));
    });

    it("handles empty buffers gracefully", () => {
      const empty = new Uint8Array(0);
      const b64 = passkeysApi.uint8ArrayToBase64Url(empty);
      assert.equal(b64, "");
      const back = passkeysApi.base64UrlToUint8Array("");
      assert.equal(back.byteLength, 0);
    });
  });

  describe("listAccountPasskeys", () => {
    it("fetches passkeys with required BFF security headers and options", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/passkeys");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          passkeys: [
            {
              id: "9f38e789-5322-48a0-9759-86ec03792040",
              label: "Laptop Linux",
              transports: ["internal"],
              created_at: "2026-11-19T06:00:00Z",
              last_used_at: "2026-11-19T07:00:00Z",
            },
          ],
        });
      };

      const result = await passkeysApi.listAccountPasskeys();
      assert.equal(result.length, 1);
      assert.equal(result[0].id, "9f38e789-5322-48a0-9759-86ec03792040");
      assert.equal(result[0].label, "Laptop Linux");
      assert.deepEqual(result[0].transports, ["internal"]);
      assert.equal(result[0].created_at, "2026-11-19T06:00:00Z");
      assert.equal(result[0].last_used_at, "2026-11-19T07:00:00Z");
    });

    it("returns empty array when response has no passkeys list", async () => {
      globalThis.fetch = async () => jsonResponse({});
      const result = await passkeysApi.listAccountPasskeys();
      assert.deepEqual(result, []);
    });

    it("maps 401 unauthenticated error to friendly user message", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "unauthenticated" } }, 401);

      await assert.rejects(
        async () => passkeysApi.listAccountPasskeys(),
        (err) => {
          assert.equal(err.name, "PasskeysApiError");
          assert.equal(err.code, "unauthenticated");
          assert.equal(err.status, 401);
          assert.match(err.message, /sesión no es válida o ha expirado/);
          return true;
        },
      );
    });
  });

  describe("beginAccountPasskeyRegistration", () => {
    it("requests registration options with required BFF options", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/passkeys/register/options",
        );
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          challenge_id: "c4636fcb-b935-4ae2-a279-b1dca09886eb",
          challenge: "Y3JlYXRlQ2hhbGxlbmdlQmFzZTY0VXJs",
          rp: { id: "127.0.0.1", name: "EspaciGo" },
          user: {
            id: "account-uuid-1",
            name: "u••••o@ejemplo.cl",
            displayName: "Usuario Demo",
          },
          pubKeyCredParams: [{ type: "public-key", alg: -7 }],
          timeout: 60000,
          excludeCredentials: [],
        });
      };

      const result = await passkeysApi.beginAccountPasskeyRegistration();
      assert.equal(
        result.challenge_id,
        "c4636fcb-b935-4ae2-a279-b1dca09886eb",
      );
      assert.equal(result.rp.id, "127.0.0.1");
      assert.equal(result.user.name, "u••••o@ejemplo.cl");
    });
  });

  describe("completeAccountPasskeyRegistration", () => {
    it("submits verification payload and returns created passkey", async () => {
      const inputPayload = {
        challenge_id: "c4636fcb-b935-4ae2-a279-b1dca09886eb",
        label: "Mi MacBook",
        credential_id: "cred-abc-123",
        public_key: "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...",
        client_data_json: "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIn0",
        transports: ["internal"],
      };

      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/passkeys/register/verify",
        );
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");
        assert.equal(init.headers.Accept, "application/json");
        assert.equal(init.body, JSON.stringify(inputPayload));

        return jsonResponse({
          id: "e6f49492-c9a1-432d-b5d1-9258ce50d601",
          label: "Mi MacBook",
          transports: ["internal"],
          created_at: "2026-11-19T07:15:00Z",
          last_used_at: null,
        });
      };

      const result =
        await passkeysApi.completeAccountPasskeyRegistration(inputPayload);
      assert.equal(result.id, "e6f49492-c9a1-432d-b5d1-9258ce50d601");
      assert.equal(result.label, "Mi MacBook");
      assert.deepEqual(result.transports, ["internal"]);
    });

    it("maps 409 credential_conflict error to friendly user message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "credential_conflict",
              message: "Esta credencial ya existe.",
            },
          },
          409,
        );

      await assert.rejects(
        async () =>
          passkeysApi.completeAccountPasskeyRegistration({
            challenge_id: "c4636fcb-b935-4ae2-a279-b1dca09886eb",
            credential_id: "cred-abc-123",
            public_key: "MFkw...",
            client_data_json: "eyJ0eX...",
          }),
        (err) => {
          assert.equal(err.name, "PasskeysApiError");
          assert.equal(err.code, "credential_conflict");
          assert.equal(err.status, 409);
          assert.match(err.message, /ya se encuentra registrada/);
          return true;
        },
      );
    });
  });

  describe("deleteAccountPasskey", () => {
    it("deletes passkey by id with encoded path", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/passkeys/9f38e789-5322-48a0-9759-86ec03792040",
        );
        assert.equal(init.method, "DELETE");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          status: "deleted",
          passkey_id: "9f38e789-5322-48a0-9759-86ec03792040",
        });
      };

      const result = await passkeysApi.deleteAccountPasskey(
        "9f38e789-5322-48a0-9759-86ec03792040",
      );
      assert.equal(result.status, "deleted");
      assert.equal(result.passkey_id, "9f38e789-5322-48a0-9759-86ec03792040");
    });

    it("maps 404 passkey_not_found error", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "passkey_not_found",
              message: "No encontrada.",
            },
          },
          404,
        );

      await assert.rejects(
        async () => passkeysApi.deleteAccountPasskey("non-existent-id"),
        (err) => {
          assert.equal(err.name, "PasskeysApiError");
          assert.equal(err.code, "passkey_not_found");
          assert.equal(err.status, 404);
          assert.match(err.message, /no fue encontrada/);
          return true;
        },
      );
    });
  });

  describe("createWebAuthnCredential WebAuthn Invocation", () => {
    it("handles NotAllowedError (user cancelled) cleanly", async () => {
      setMockNavigator({
        credentials: {
          create: async () => {
            const err = new Error("The operation either timed out or was not allowed.");
            err.name = "NotAllowedError";
            throw err;
          },
        },
      });

      const options = {
        challenge_id: "c4636fcb-b935-4ae2-a279-b1dca09886eb",
        challenge: "Y3JlYXRlQ2hhbGxlbmdlQmFzZTY0VXJs",
        rp: { id: "127.0.0.1", name: "EspaciGo" },
        user: {
          id: "account-uuid-1",
          name: "u••••o@ejemplo.cl",
          displayName: "Usuario Demo",
        },
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
        timeout: 60000,
        excludeCredentials: [],
      };

      await assert.rejects(
        async () => passkeysApi.createWebAuthnCredential(options, "Mi Dispositivo"),
        (err) => {
          assert.equal(err.name, "PasskeysApiError");
          assert.equal(err.code, "user_cancelled");
          assert.match(err.message, /cancelado por el usuario/);
          return true;
        },
      );
    });

    it("successfully transforms WebAuthn response into verify input", async () => {
      const clientDataJSONBytes = new TextEncoder().encode(
        JSON.stringify({
          type: "webauthn.create",
          challenge: "Y3JlYXRlQ2hhbGxlbmdlQmFzZTY0VXJs",
          origin: "http://127.0.0.1:3000",
        }),
      );
      const fakePublicKeyBytes = new Uint8Array([1, 2, 3, 4, 5]);

      setMockNavigator({
        credentials: {
          create: async (opts) => {
            assert.ok(opts.publicKey);
            assert.equal(opts.publicKey.rp.id, "127.0.0.1");
            return {
              id: "credential-id-base64url",
              rawId: new Uint8Array([10, 20, 30]).buffer,
              response: {
                clientDataJSON: clientDataJSONBytes.buffer,
                getPublicKey: () => fakePublicKeyBytes.buffer,
                getTransports: () => ["internal", "hybrid"],
                getPublicKeyAlgorithm: () => -7,
              },
            };
          },
        },
      });

      const options = {
        challenge_id: "c4636fcb-b935-4ae2-a279-b1dca09886eb",
        challenge: "Y3JlYXRlQ2hhbGxlbmdlQmFzZTY0VXJs",
        rp: { id: "127.0.0.1", name: "EspaciGo" },
        user: {
          id: "account-uuid-1",
          name: "u••••o@ejemplo.cl",
          displayName: "Usuario Demo",
        },
        pubKeyCredParams: [{ type: "public-key", alg: -7 }],
        timeout: 60000,
        excludeCredentials: [],
      };

      const result = await passkeysApi.createWebAuthnCredential(
        options,
        "Laptop Linux",
      );
      assert.equal(result.challenge_id, "c4636fcb-b935-4ae2-a279-b1dca09886eb");
      assert.equal(result.label, "Laptop Linux");
      assert.equal(result.credential_id, "credential-id-base64url");
      assert.equal(
        result.public_key,
        passkeysApi.uint8ArrayToBase64Url(fakePublicKeyBytes),
      );
      assert.deepEqual(result.transports, ["internal", "hybrid"]);
      assert.equal(result.algorithm, -7);
    });
  });
});
