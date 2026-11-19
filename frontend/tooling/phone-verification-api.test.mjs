import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { after, afterEach, describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-phone-test-"),
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

const phoneApi = await loadModule(
  "src/features/account/phone-verification-api.ts",
  "phone-verification-api.mjs",
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

describe("Phone verification client and return-target", () => {
  it("return-target permits /mi-cuenta/seguridad/metodos/telefono in fixedPaths", async () => {
    const source = await readFile(
      path.join(root, "src/features/auth/domain/return-target.ts"),
      "utf8",
    );
    assert.match(source, /"\/mi-cuenta\/seguridad\/metodos\/telefono"/);
  });

  it("getAccountPhoneStatus returns status when successful", async () => {
    globalThis.fetch = async (url, init) => {
      assert.equal(url, "/api/backend/account/security/phone");
      assert.equal(init.method, "GET");
      assert.equal(init.credentials, "same-origin");
      assert.equal(init.cache, "no-store");
      return jsonResponse({
        configured: true,
        verified: true,
        masked_phone: "+56 •••• 5678",
        verified_at: "2026-11-19T04:00:00Z",
        pending_verification: false,
        resend_available_at: null,
      });
    };

    const status = await phoneApi.getAccountPhoneStatus();
    assert.equal(status.configured, true);
    assert.equal(status.verified, true);
    assert.equal(status.masked_phone, "+56 •••• 5678");
  });

  it("sendAccountPhoneCode dispatches phone and maps safe cooldown error with Retry-After", async () => {
    globalThis.fetch = async (url, init) => {
      assert.equal(url, "/api/backend/account/security/phone/send-code");
      assert.equal(init.method, "POST");
      const body = JSON.parse(init.body);
      assert.equal(body.phone, "+56 9 1234 5678");

      return jsonResponse(
        {
          error: {
            code: "cooldown_active",
            message: "Debes esperar 60 segundos antes de solicitar un nuevo código.",
          },
        },
        429,
        { "retry-after": "60" },
      );
    };

    await assert.rejects(
      async () => {
        await phoneApi.sendAccountPhoneCode("+56 9 1234 5678");
      },
      (err) => {
        assert(err instanceof phoneApi.PhoneVerificationApiError);
        assert.equal(err.code, "cooldown_active");
        assert.equal(err.status, 429);
        assert.equal(err.retryAfterSeconds, 60);
        assert(err.message.includes("60 segundos"));
        return true;
      },
    );
  });

  it("sendAccountPhoneCode maps invalid_phone error to safe user message", async () => {
    globalThis.fetch = async () => {
      return jsonResponse(
        {
          error: {
            code: "invalid_phone",
            message: "invalid phone internal details",
          },
        },
        400,
      );
    };

    await assert.rejects(
      async () => {
        await phoneApi.sendAccountPhoneCode("123");
      },
      (err) => {
        assert(err instanceof phoneApi.PhoneVerificationApiError);
        assert.equal(err.code, "invalid_phone");
        assert(err.message.includes("chileno inválido"));
        return true;
      },
    );
  });

  it("verifyAccountPhoneCode returns verification result on 200", async () => {
    globalThis.fetch = async (url, init) => {
      assert.equal(url, "/api/backend/account/security/phone/verify");
      assert.equal(init.method, "POST");
      const body = JSON.parse(init.body);
      assert.equal(body.code, "123456");

      return jsonResponse({
        status: "verified",
        verified: true,
        masked_phone: "+56 •••• 5678",
        verified_at: "2026-11-19T04:10:00Z",
      });
    };

    const result = await phoneApi.verifyAccountPhoneCode("123456");
    assert.equal(result.status, "verified");
    assert.equal(result.verified, true);
    assert.equal(result.masked_phone, "+56 •••• 5678");
  });

  it("verifyAccountPhoneCode maps invalid_code to safe message", async () => {
    globalThis.fetch = async () => {
      return jsonResponse(
        {
          error: {
            code: "invalid_code",
            message: "internal code mismatch",
          },
        },
        422,
      );
    };

    await assert.rejects(
      async () => {
        await phoneApi.verifyAccountPhoneCode("000000");
      },
      (err) => {
        assert(err instanceof phoneApi.PhoneVerificationApiError);
        assert.equal(err.code, "invalid_code");
        assert(err.message.includes("incorrecto"));
        return true;
      },
    );
  });
});
