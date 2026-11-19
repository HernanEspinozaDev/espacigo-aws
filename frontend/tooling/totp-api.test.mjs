import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-totp-test-"),
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

const totpApi = await loadModule(
  "src/features/account/totp-api.ts",
  "totp-api.mjs",
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

describe("Google Authenticator (TOTP) API Client and Domain", () => {
  it("return-target includes /mi-cuenta/seguridad/metodos/google-authenticator in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(
      source,
      /"\/mi-cuenta\/seguridad\/metodos\/google-authenticator"/,
    );
  });

  describe("getAccountTOTPStatus", () => {
    it("fetches TOTP status successfully with required BFF options", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/totp");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          enabled: true,
          pending_setup: false,
          enabled_at: "2026-11-19T06:00:00Z",
          remaining_backup_codes: 8,
        });
      };

      const result = await totpApi.getAccountTOTPStatus();
      assert.deepEqual(result, {
        enabled: true,
        pending_setup: false,
        enabled_at: "2026-11-19T06:00:00Z",
        remaining_backup_codes: 8,
      });
    });

    it("returns safe defaults when optional fields are null or omitted", async () => {
      globalThis.fetch = async () =>
        jsonResponse({
          enabled: false,
        });

      const result = await totpApi.getAccountTOTPStatus();
      assert.equal(result.enabled, false);
      assert.equal(result.pending_setup, false);
      assert.equal(result.enabled_at, null);
      assert.equal(result.remaining_backup_codes, 0);
    });

    it("maps 401 unauthenticated error to user-friendly message", async () => {
      globalThis.fetch = async () =>
        jsonResponse({ error: { code: "unauthenticated" } }, 401);

      await assert.rejects(
        async () => totpApi.getAccountTOTPStatus(),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "unauthenticated");
          assert.equal(err.status, 401);
          assert.match(err.message, /sesión no es válida o ha expirado/);
          return true;
        },
      );
    });
  });

  describe("setupAccountTOTP", () => {
    it("dispatches POST setup request and returns otpauth_uri and manual_key", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/totp/setup");
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          otpauth_uri:
            "otpauth://totp/EspaciGo:user@example.cl?secret=JBSWY3DPEHPK3PXP&issuer=EspaciGo",
          manual_key: "JBSWY3DPEHPK3PXP",
        });
      };

      const result = await totpApi.setupAccountTOTP();
      assert.equal(result.manual_key, "JBSWY3DPEHPK3PXP");
      assert.match(result.otpauth_uri, /^otpauth:\/\/totp\//);
    });

    it("maps 400 totp_already_enabled error to descriptive message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "totp_already_enabled",
              message: "El autenticador TOTP ya está activado.",
            },
          },
          400,
        );

      await assert.rejects(
        async () => totpApi.setupAccountTOTP(),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "totp_already_enabled");
          assert.equal(err.status, 400);
          assert.match(err.message, /ya se encuentra activado/);
          return true;
        },
      );
    });
  });

  describe("enableAccountTOTP", () => {
    it("dispatches POST enable request with code and returns backup codes", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/totp/enable");
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");
        assert.equal(init.headers.Accept, "application/json");

        const body = JSON.parse(init.body);
        assert.equal(body.code, "123456");

        return jsonResponse({
          enabled: true,
          enabled_at: "2026-11-19T06:30:00Z",
          backup_codes: [
            "A1B2C3D4",
            "E5F6G7H8",
            "I9J0K1L2",
            "M3N4O5P6",
            "Q7R8S9T0",
            "U1V2W3X4",
            "Y5Z6A7B8",
            "C9D0E1F2",
          ],
        });
      };

      const result = await totpApi.enableAccountTOTP({ code: "123456" });
      assert.equal(result.enabled, true);
      assert.equal(result.enabled_at, "2026-11-19T06:30:00Z");
      assert.equal(result.backup_codes.length, 8);
    });

    it("maps 422 invalid_code error to descriptive message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "invalid_code",
              message: "Código de autenticación incorrecto o expirado.",
            },
          },
          422,
        );

      await assert.rejects(
        async () => totpApi.enableAccountTOTP({ code: "000000" }),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "invalid_code");
          assert.equal(err.status, 422);
          assert.match(err.message, /incorrecto o expirado/);
          return true;
        },
      );
    });

    it("maps 422 code_replayed error to descriptive message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "code_replayed",
              message: "Código ya utilizado.",
            },
          },
          422,
        );

      await assert.rejects(
        async () => totpApi.enableAccountTOTP({ code: "123456" }),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "code_replayed");
          assert.equal(err.status, 422);
          assert.match(err.message, /ya fue utilizado/);
          return true;
        },
      );
    });

    it("maps 400 totp_not_pending error to descriptive message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "totp_not_pending",
              message: "No hay una configuración pendiente.",
            },
          },
          400,
        );

      await assert.rejects(
        async () => totpApi.enableAccountTOTP({ code: "123456" }),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "totp_not_pending");
          assert.equal(err.status, 400);
          assert.match(err.message, /No hay una configuración/);
          return true;
        },
      );
    });
  });

  describe("disableAccountTOTP", () => {
    it("dispatches POST disable request with code and returns enabled: false", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/totp/disable");
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");
        assert.equal(init.headers.Accept, "application/json");

        const body = JSON.parse(init.body);
        assert.equal(body.code, "A1B2C3D4");

        return jsonResponse({
          enabled: false,
        });
      };

      const result = await totpApi.disableAccountTOTP({ code: "A1B2C3D4" });
      assert.equal(result.enabled, false);
    });

    it("maps 400 totp_not_enabled error to descriptive message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "totp_not_enabled",
              message: "El autenticador TOTP no se encuentra activado.",
            },
          },
          400,
        );

      await assert.rejects(
        async () => totpApi.disableAccountTOTP({ code: "123456" }),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.code, "totp_not_enabled");
          assert.equal(err.status, 400);
          assert.match(err.message, /no se encuentra activado/);
          return true;
        },
      );
    });

    it("handles 500 server error gracefully", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "internal_error",
              message: "database error",
            },
          },
          500,
        );

      await assert.rejects(
        async () => totpApi.disableAccountTOTP({ code: "123456" }),
        (err) => {
          assert.equal(err.name, "TOTPApiError");
          assert.equal(err.status, 500);
          assert.match(err.message, /Servicio temporalmente no disponible/);
          return true;
        },
      );
    });
  });
});
