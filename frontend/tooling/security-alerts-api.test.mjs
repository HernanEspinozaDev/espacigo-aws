import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-alerts-test-"),
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

const alertsApi = await loadModule(
  "src/features/account/security-alerts-api.ts",
  "security-alerts-api.mjs",
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

describe("Security Alerts API Client and Domain", () => {
  it("return-target includes /mi-cuenta/seguridad/alertas in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(source, /"\/mi-cuenta\/seguridad\/alertas"/);
  });

  describe("getSecurityAlerts", () => {
    it("fetches security alerts and preferences successfully with required BFF options", async () => {
      const mockPreferences = {
        app_notifications_enabled: true,
        email_alerts_enabled: true,
        whatsapp_alerts_enabled: false,
        masked_email: "u••••1@example.com",
        masked_extra_email: "s••••y@example.com",
        extra_email_configured: true,
        masked_phone: "+56 •••• 1234",
      };

      const mockAlerts = [
        {
          id: "alert-1",
          event_code: "alert_channel_updated",
          title: "Canales de alerta actualizados",
          device_label: "Chrome en Linux",
          ip_masked: "190.162.•••.•••",
          created_at: "2026-11-19T08:00:00Z",
        },
        {
          id: "alert-2",
          event_code: "password_changed",
          title: "Contraseña de la cuenta actualizada",
          device_label: "Firefox en MacOS",
          ip_masked: "200.54.•••.•••",
          created_at: "2026-11-18T10:00:00Z",
        },
      ];

      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/alerts");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          preferences: mockPreferences,
          alerts: mockAlerts,
        });
      };

      const result = await alertsApi.getSecurityAlerts();
      assert.deepEqual(result.preferences, mockPreferences);
      assert.equal(result.alerts.length, 2);
      assert.equal(result.alerts[0].id, "alert-1");
      assert.equal(result.alerts[0].event_code, "alert_channel_updated");
      assert.equal(result.alerts[1].id, "alert-2");
    });

    it("returns safe defaults if response has missing fields", async () => {
      globalThis.fetch = async () => jsonResponse({});
      const result = await alertsApi.getSecurityAlerts();
      assert.equal(result.preferences.app_notifications_enabled, true);
      assert.equal(result.preferences.email_alerts_enabled, true);
      assert.equal(result.preferences.whatsapp_alerts_enabled, false);
      assert.deepEqual(result.alerts, []);
    });

    it("maps 401 unauthenticated error to user friendly message", async () => {
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
          await alertsApi.getSecurityAlerts();
        },
        (err) => {
          assert(err instanceof alertsApi.SecurityAlertsApiError);
          assert.equal(err.status, 401);
          assert.equal(err.code, "unauthenticated");
          assert.match(err.message, /Tu sesión no es válida o ha expirado/);
          return true;
        },
      );
    });
  });

  describe("updateSecurityAlertChannels", () => {
    it("dispatches PUT request with payload and returns updated preferences", async () => {
      const updatedPrefs = {
        app_notifications_enabled: true,
        email_alerts_enabled: false,
        whatsapp_alerts_enabled: true,
        masked_email: "u••••1@example.com",
        masked_extra_email: "s••••y@example.com",
        extra_email_configured: true,
        masked_phone: "+56 •••• 1234",
      };

      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/alerts/channels");
        assert.equal(init.method, "PUT");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");
        assert.equal(init.headers.Accept, "application/json");

        const body = JSON.parse(init.body);
        assert.equal(body.app_notifications_enabled, true);
        assert.equal(body.email_alerts_enabled, false);
        assert.equal(body.whatsapp_alerts_enabled, true);
        assert.equal(body.extra_email, "seguridad@empresa.com");

        return jsonResponse(updatedPrefs);
      };

      const result = await alertsApi.updateSecurityAlertChannels({
        app_notifications_enabled: true,
        email_alerts_enabled: false,
        whatsapp_alerts_enabled: true,
        extra_email: "seguridad@empresa.com",
      });

      assert.deepEqual(result, updatedPrefs);
    });

    it("maps 400 invalid_email error to descriptive error message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "invalid_email",
              message: "Formato de correo electrónico secundario inválido.",
              request_id: "req-invalid-email",
            },
          },
          400,
        );

      await assert.rejects(
        async () => {
          await alertsApi.updateSecurityAlertChannels({
            app_notifications_enabled: true,
            email_alerts_enabled: true,
            whatsapp_alerts_enabled: false,
            extra_email: "not-an-email",
          });
        },
        (err) => {
          assert(err instanceof alertsApi.SecurityAlertsApiError);
          assert.equal(err.status, 400);
          assert.equal(err.code, "invalid_email");
          assert.match(
            err.message,
            /Formato de correo electrónico secundario inválido/,
          );
          return true;
        },
      );
    });

    it("maps 400 email_same_as_primary error to descriptive error message", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "email_same_as_primary",
              message:
                "El correo secundario no puede ser igual al correo principal.",
              request_id: "req-same-email",
            },
          },
          400,
        );

      await assert.rejects(
        async () => {
          await alertsApi.updateSecurityAlertChannels({
            app_notifications_enabled: true,
            email_alerts_enabled: true,
            whatsapp_alerts_enabled: false,
            extra_email: "primary@example.com",
          });
        },
        (err) => {
          assert(err instanceof alertsApi.SecurityAlertsApiError);
          assert.equal(err.status, 400);
          assert.equal(err.code, "email_same_as_primary");
          assert.match(
            err.message,
            /El correo secundario no puede ser igual al correo principal/,
          );
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
              message: "Error interno",
              request_id: "req-500",
            },
          },
          500,
        );

      await assert.rejects(
        async () => {
          await alertsApi.updateSecurityAlertChannels({
            app_notifications_enabled: true,
            email_alerts_enabled: true,
            whatsapp_alerts_enabled: false,
          });
        },
        (err) => {
          assert(err instanceof alertsApi.SecurityAlertsApiError);
          assert.equal(err.status, 500);
          assert.match(err.message, /Servicio temporalmente no disponible/);
          return true;
        },
      );
    });
  });
});
