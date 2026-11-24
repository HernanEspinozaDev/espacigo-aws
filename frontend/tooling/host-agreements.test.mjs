import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";
const source = await readFile(
  new URL(
    "../src/features/host-agreements/application/host-agreements-service.ts",
    import.meta.url,
  ),
  "utf8",
);
const loaded = { exports: {} };
new Function(
  "module",
  "exports",
  ts.transpile(source, { module: ts.ModuleKind.CommonJS }),
)(loaded, loaded.exports);
const { HostAgreementsService } = loaded.exports;
const agreement = {
  id: "request",
  proposal_id: "proposal",
  quote_id: "quote",
  space_id: "space",
  proposal_version: 2,
  snapshot_sha256: "hash",
  state: "pending_landlord",
};
const proposal = {
  id: "proposal",
  quote_id: "quote",
  space_id: "space",
  version: 2,
  sha256: "hash",
};
const read = { agreement, etag: '"revision-1"' };
test("host review binds proposal hash, version, quote and space to the selected request", async () => {
  for (const overrides of [
    { sha256: "other" },
    { version: 3 },
    { id: "other" },
    { quote_id: "other" },
    { space_id: "other" },
  ]) {
    const service = new HostAgreementsService({
      get: async () => read,
      proposal: async () => ({ ...proposal, ...overrides }),
    });
    await assert.rejects(() => service.review("request"), /no coincide/);
  }
  const service = new HostAgreementsService({
    get: async () => read,
    proposal: async () => proposal,
  });
  assert.deepEqual(await service.review("request"), { read, proposal });
});
test("host acceptance requires explicit consent and forwards the reviewed ETag once", async () => {
  let calls = 0;
  const service = new HostAgreementsService({
    accept: async (value) => {
      calls++;
      assert.equal(value, read);
      return {
        agreement: { ...agreement, state: "ready_for_payment" },
        etag: '"revision-2"',
      };
    },
  });
  assert.throws(() => service.accept({ read, proposal }, false), /Confirma/);
  assert.equal(calls, 0);
  const result = await service.accept({ read, proposal }, true);
  assert.equal(calls, 1);
  assert.equal(result.etag, '"revision-2"');
});
test("terminal or mismatched proposals never cause signing or rejection writes", () => {
  let writes = 0;
  const service = new HostAgreementsService({
    accept: async () => {
      writes++;
    },
    reject: async () => {
      writes++;
    },
  });
  for (const state of [
    "ready_for_payment",
    "rejected",
    "withdrawn",
    "expired",
  ]) {
    const terminal = {
      read: { ...read, agreement: { ...agreement, state } },
      proposal,
    };
    assert.throws(() => service.accept(terminal, true));
    assert.throws(() => service.reject(terminal));
  }
  assert.throws(() =>
    service.accept(
      { read, proposal: { ...proposal, sha256: "changed" } },
      true,
    ),
  );
  assert.equal(writes, 0);
});
