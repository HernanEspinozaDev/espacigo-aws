import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-loc-trusted-test-"),
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

const api = await loadModule(
  "src/features/account/security-location-trusted-api.ts",
  "security-location-trusted-api.mjs",
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

describe("Location & Trusted Contacts & Methods Summary API", () => {
  it("return-target includes security fixed paths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(source, /"\/mi-cuenta\/seguridad\/ubicacion"/);
    assert.match(source, /"\/mi-cuenta\/seguridad\/personas-confianza"/);
    assert.match(source, /"\/mi-cuenta\/seguridad\/metodos"/);
  });

  describe("getAccountLocationPermission", () => {
    it("fetches location permission successfully with required BFF options", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/location");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers.Accept, "application/json");

        return jsonResponse({
          enabled: true,
          consent_updated_at: "2026-11-19T08:00:00Z",
          last_region_label: "Región Metropolitana, Chile",
        });
      };

      const result = await api.getAccountLocationPermission();
      assert.deepEqual(result, {
        enabled: true,
        consent_updated_at: "2026-11-19T08:00:00Z",
        last_region_label: "Región Metropolitana, Chile",
      });
    });

    it("handles 401 unauthenticated error", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          { error: { code: "unauthenticated", message: "No autenticado" } },
          401,
        );

      await assert.rejects(
        async () => api.getAccountLocationPermission(),
        (err) => {
          assert.equal(err.name, "SecurityLocationTrustedError");
          assert.equal(err.status, 401);
          assert.match(err.message, /sesión no es válida/);
          return true;
        },
      );
    });
  });

  describe("updateAccountLocationPermission", () => {
    it("updates location permission successfully with PUT", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/location");
        assert.equal(init.method, "PUT");
        assert.equal(init.credentials, "same-origin");
        assert.equal(init.cache, "no-store");
        assert.equal(init.headers["Content-Type"], "application/json");

        const parsedBody = JSON.parse(init.body);
        assert.deepEqual(parsedBody, {
          enabled: true,
          region_label: "Región de Valparaíso, Chile",
        });

        return jsonResponse({
          enabled: true,
          consent_updated_at: "2026-11-19T08:30:00Z",
          last_region_label: "Región de Valparaíso, Chile",
        });
      };

      const result = await api.updateAccountLocationPermission({
        enabled: true,
        region_label: "Región de Valparaíso, Chile",
      });
      assert.equal(result.enabled, true);
      assert.equal(result.last_region_label, "Región de Valparaíso, Chile");
    });

    it("handles coordinates_not_allowed error friendly", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "coordinates_not_allowed",
              message: "Coordinates not allowed",
            },
          },
          400,
        );

      await assert.rejects(
        async () =>
          api.updateAccountLocationPermission({
            enabled: true,
            region_label: "-33.4489, -70.6693",
          }),
        (err) => {
          assert.equal(err.name, "SecurityLocationTrustedError");
          assert.equal(err.code, "coordinates_not_allowed");
          assert.match(err.message, /coordenadas GPS exactas/);
          return true;
        },
      );
    });
  });

  describe("listAccountTrustedContacts", () => {
    it("lists trusted contacts with masked data", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/trusted-contacts");
        assert.equal(init.method, "GET");
        assert.equal(init.credentials, "same-origin");

        return jsonResponse({
          contacts: [
            {
              id: "550e8400-e29b-41d4-a716-446655440000",
              full_name: "Constanza Silva",
              masked_email: "c••••a@espacigo.cl",
              masked_phone: "+56 •••• 5678",
              relationship: "Hermana",
              created_at: "2026-11-19T08:00:00Z",
            },
          ],
        });
      };

      const list = await api.listAccountTrustedContacts();
      assert.equal(list.length, 1);
      assert.equal(list[0].full_name, "Constanza Silva");
      assert.equal(list[0].masked_email, "c••••a@espacigo.cl");
      assert.equal(list[0].relationship, "Hermana");
    });

    it("returns empty array when contacts list is empty", async () => {
      globalThis.fetch = async () => jsonResponse({ contacts: [] });
      const list = await api.listAccountTrustedContacts();
      assert.deepEqual(list, []);
    });
  });

  describe("addAccountTrustedContact", () => {
    it("adds trusted contact successfully", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/trusted-contacts");
        assert.equal(init.method, "POST");
        assert.equal(init.headers["Content-Type"], "application/json");

        const parsed = JSON.parse(init.body);
        assert.equal(parsed.full_name, "Rodrigo Soto");
        assert.equal(parsed.email, "rodrigo@ejemplo.cl");
        assert.equal(parsed.relationship, "Amigo");
        assert.equal(parsed.phone, "+56912345678");

        return jsonResponse({
          id: "660e8400-e29b-41d4-a716-446655440001",
          full_name: "Rodrigo Soto",
          masked_email: "r••••o@ejemplo.cl",
          masked_phone: "+56 •••• 5678",
          relationship: "Amigo",
          created_at: "2026-11-19T08:35:00Z",
        });
      };

      const contact = await api.addAccountTrustedContact({
        full_name: "Rodrigo Soto",
        email: "rodrigo@ejemplo.cl",
        relationship: "Amigo",
        phone: "+56912345678",
      });
      assert.equal(contact.id, "660e8400-e29b-41d4-a716-446655440001");
      assert.equal(contact.full_name, "Rodrigo Soto");
    });

    it("handles trusted_contact_limit error (409)", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          {
            error: {
              code: "trusted_contact_limit",
              message: "limit reached",
            },
          },
          409,
        );

      await assert.rejects(
        async () =>
          api.addAccountTrustedContact({
            full_name: "Persona Cuatro",
            email: "cuatro@ejemplo.cl",
            relationship: "Colega",
          }),
        (err) => {
          assert.equal(err.name, "SecurityLocationTrustedError");
          assert.equal(err.code, "trusted_contact_limit");
          assert.match(err.message, /máximo de 3 personas de confianza/);
          return true;
        },
      );
    });
  });

  describe("removeAccountTrustedContact", () => {
    it("revokes trusted contact successfully", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(
          url,
          "/api/backend/account/security/trusted-contacts/test-id-123",
        );
        assert.equal(init.method, "DELETE");

        return jsonResponse({
          status: "revoked",
          contact_id: "test-id-123",
        });
      };

      const result = await api.removeAccountTrustedContact("test-id-123");
      assert.deepEqual(result, {
        status: "revoked",
        contact_id: "test-id-123",
      });
    });

    it("handles not_found error on delete", async () => {
      globalThis.fetch = async () =>
        jsonResponse(
          { error: { code: "not_found", message: "Contact not found" } },
          404,
        );

      await assert.rejects(
        async () => api.removeAccountTrustedContact("invalid-id"),
        (err) => {
          assert.equal(err.name, "SecurityLocationTrustedError");
          assert.equal(err.status, 404);
          return true;
        },
      );
    });
  });

  describe("getAccountSecurityMethodsSummary", () => {
    it("fetches and maps all 7 verification methods", async () => {
      globalThis.fetch = async (url, init) => {
        assert.equal(url, "/api/backend/account/security/methods");
        assert.equal(init.method, "GET");

        return jsonResponse({
          protected: true,
          active_count: 3,
          total_count: 7,
          methods: {
            password: { enabled: true },
            email: { enabled: true, masked_email: "u••••o@espacigo.cl" },
            phone: { enabled: true, masked_phone: "+56 •••• 5678" },
            totp: { enabled: false },
            passkeys: { enabled: false, count: 0 },
            qr: { enabled: false },
            facial_kyc: { enabled: false, status: "pending_kyc_module" },
          },
        });
      };

      const result = await api.getAccountSecurityMethodsSummary();
      assert.equal(result.protected, true);
      assert.equal(result.active_count, 3);
      assert.equal(result.total_count, 7);
      assert.equal(result.methods.password.enabled, true);
      assert.equal(result.methods.email.enabled, true);
      assert.equal(result.methods.phone.enabled, true);
      assert.equal(result.methods.phone.masked_phone, "+56 •••• 5678");
      assert.equal(result.methods.facial_kyc.status, "pending_kyc_module");
      assert.equal(result.methods.passkeys.count, 0);
    });
  });
});
