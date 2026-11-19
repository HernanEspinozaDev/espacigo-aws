import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-devices-test-"),
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

const devicesApi = await loadModule(
  "src/features/account/linked-devices-api.ts",
  "linked-devices-api.mjs",
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

describe("Linked Devices API Client and Domain", () => {
  it("return-target includes /mi-cuenta/seguridad/dispositivos in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(source, /"\/mi-cuenta\/seguridad\/dispositivos"/);
  });

  describe("listLinkedDevices", () => {
    it("fetches linked devices successfully with required BFF options", async () => {
      const mockDevices = [
        {
          id: "session-1",
          device_label: "Chrome en Linux",
          ip_masked: "190.162.•••.•••",
          created_at: "2026-11-19T08:00:00Z",
          last_activity_at: "2026-11-19T08:15:00Z",
          expires_at: "2026-11-20T08:00:00Z",
          is_current: true,
        },
        {
          id: "session-2",
          device_label: "Safari en iPhone",
          ip_masked: "200.54.•••.•••",
          created_at: "2026-11-18T12:00:00Z",
          last_activity_at: "2026-11-19T05:00:00Z",
          expires_at: "2026-11-20T12:00:00Z",
          is_current: false,
        },
      ];

      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/devices");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({ devices: mockDevices });
      };

      const result = await devicesApi.listLinkedDevices();
      assert.equal(result.length, 2);
      assert.equal(result[0].id, "session-1");
      assert.equal(result[0].is_current, true);
      assert.equal(result[1].id, "session-2");
      assert.equal(result[1].is_current, false);
    });

    it("returns empty array if response devices is null or missing", async () => {
      globalThis.fetch = async () => jsonResponse({});
      const result = await devicesApi.listLinkedDevices();
      assert.deepEqual(result, []);
    });

    it("maps 401 error to user friendly session expiration message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "unauthenticated",
              message: "No autenticado",
              request_id: "req-123",
            },
          },
          401,
        );

      await assert.rejects(
        async () => {
          await devicesApi.listLinkedDevices();
        },
        (err) => {
          assert(err instanceof devicesApi.LinkedDevicesApiError);
          assert.equal(err.status, 401);
          assert.equal(err.code, "unauthenticated");
          assert.match(err.message, /Tu sesión no es válida o ha expirado/);
          return true;
        },
      );
    });
  });

  describe("revokeLinkedDevice", () => {
    it("validates session ID presence before making request", async () => {
      await assert.rejects(
        async () => {
          await devicesApi.revokeLinkedDevice("");
        },
        (err) => {
          assert(err instanceof devicesApi.LinkedDevicesApiError);
          assert.equal(err.status, 400);
          assert.equal(err.code, "invalid_device");
          return true;
        },
      );
    });

    it("dispatches DELETE request with encoded session ID", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/devices/session-target-id",
        );
        assert.equal(init.method, "DELETE");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          status: "revoked",
          session_id: "session-target-id",
        });
      };

      const result = await devicesApi.revokeLinkedDevice("session-target-id");
      assert.equal(result.status, "revoked");
      assert.equal(result.session_id, "session-target-id");
    });

    it("maps 404 device_not_found error accurately", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "device_not_found",
              message: "Sesión o dispositivo no encontrado.",
              request_id: "req-404",
            },
          },
          404,
        );

      await assert.rejects(
        async () => {
          await devicesApi.revokeLinkedDevice("unknown-session");
        },
        (err) => {
          assert(err instanceof devicesApi.LinkedDevicesApiError);
          assert.equal(err.status, 404);
          assert.equal(err.code, "device_not_found");
          assert.match(
            err.message,
            /El dispositivo o sesión que intentas revocar no fue encontrado/,
          );
          return true;
        },
      );
    });
  });

  describe("revokeOtherLinkedDevices", () => {
    it("dispatches POST to revoke-others and returns count", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/devices/revoke-others",
        );
        assert.equal(init.method, "POST");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          status: "revoked_others",
          revoked_count: 3,
        });
      };

      const result = await devicesApi.revokeOtherLinkedDevices();
      assert.equal(result.status, "revoked_others");
      assert.equal(result.revoked_count, 3);
    });

    it("handles 500 server error gracefully", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "internal_error",
              message: "Error interno al gestionar dispositivos.",
              request_id: "req-500",
            },
          },
          500,
        );

      await assert.rejects(
        async () => {
          await devicesApi.revokeOtherLinkedDevices();
        },
        (err) => {
          assert(err instanceof devicesApi.LinkedDevicesApiError);
          assert.equal(err.status, 500);
          assert.match(
            err.message,
            /Servicio temporalmente no disponible/,
          );
          return true;
        },
      );
    });
  });
});
