import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";

const sourcePath = new URL(
  "../src/features/host-listings/application/calendar-configuration-controller.ts",
  import.meta.url,
);
async function loadController() {
  const ts = await import("typescript");
  const source = await readFile(sourcePath, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
  const mod = { exports: {} };
  new Function("require", "module", "exports", compiled)(
    createRequire(sourcePath),
    mod,
    mod.exports,
  );
  return mod.exports.CalendarConfigurationController;
}
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((a, b) => {
    resolve = a;
    reject = b;
  });
  return { promise, resolve, reject };
};
const read = (etag) => ({
  etag,
  configuration: {
    space_id: "s",
    time_zone: "UTC",
    rule: {},
    overrides: [],
    updated_at: "",
  },
});
const tick = () => new Promise((resolve) => setImmediate(resolve));

test("save sends exact input and read ETag, then confirms with an independent readback", async () => {
  const Controller = await loadController();
  const calls = [];
  const first = deferred(),
    ack = deferred(),
    back = deferred();
  let n = 0;
  const port = {
    read: (...a) => {
      calls.push(["read", ...a]);
      return ++n === 1 ? first.promise : back.promise;
    },
    update: (...a) => {
      calls.push(["update", ...a]);
      return ack.promise;
    },
  };
  const c = new Controller(port, "s", () => "A");
  const loading = c.refresh();
  first.resolve(read('"old"'));
  await loading;
  const input = { time_zone: "UTC" };
  const saving = c.save(input);
  assert.equal(c.getSnapshot().phase, "saving");
  ack.resolve({ etag: '"new"' });
  await tick();
  assert.equal(calls[1][0], "update");
  assert.equal(calls[1][2], input);
  assert.equal(calls[1][3], '"old"');
  assert.equal(calls[2][0], "read");
  back.resolve(read('"new"'));
  await saving;
  assert.equal(c.getSnapshot().phase, "ready");
  assert.equal(c.getSnapshot().read.etag, '"new"');
});

test("mismatched readback needs manual refresh and uncertain PUT is never replayed", async () => {
  const Controller = await loadController();
  let reads = 0,
    updates = 0;
  const c = new Controller(
    {
      read: async () => {
        reads++;
        return read(reads === 1 ? '"a"' : '"c"');
      },
      update: async () => {
        updates++;
        return { etag: '"b"' };
      },
    },
    "s",
    () => "A",
  );
  await c.refresh();
  await c.save({});
  assert.equal(c.getSnapshot().phase, "needs_refresh");
  assert.equal(c.getSnapshot().read, null);
  assert.equal(updates, 1);
  await c.refresh();
  assert.equal(c.getSnapshot().phase, "ready");
  assert.equal(reads, 3);
  const d = new Controller(
    {
      read: async () => read('"a"'),
      update: async () => {
        updates++;
        throw Error("uncertain");
      },
    },
    "s",
    () => "A",
  );
  await d.refresh();
  await d.save({});
  assert.equal(d.getSnapshot().phase, "needs_refresh");
  assert.equal(updates, 2);
});

test("authorization errors purge state; stale session and disposed responses are ignored", async () => {
  const Controller = await loadController();
  let identity = "A";
  const a = deferred();
  const c = new Controller(
    { read: () => a.promise, update: async () => ({ etag: '"b"' }) },
    "s",
    () => identity,
  );
  const p = c.refresh();
  identity = "B";
  assert.equal(c.getSnapshot().read, null);
  identity = "A";
  a.resolve(read('"a"'));
  await p;
  assert.equal(c.getSnapshot().read, null);
  const b = deferred();
  const d = new Controller(
    { read: () => b.promise, update: async () => ({ etag: '"b"' }) },
    "s",
    () => "A",
  );
  const q = d.refresh();
  d.dispose();
  b.resolve(read('"a"'));
  await q;
  assert.equal(d.getSnapshot().read, null);
  const e = new Controller(
    {
      read: async () => {
        throw { status: 401 };
      },
      update: async () => ({ etag: '"b"' }),
    },
    "s",
    () => "A",
  );
  await e.refresh();
  assert.equal(e.getSnapshot().phase, "error");
  assert.equal(e.getSnapshot().read, null);
  assert.ok(e.getSnapshot().message);
});

test("snapshot reads never notify React subscribers and logged-out refresh cannot dispatch", async () => {
  const Controller = await loadController();
  let identity = "A",
    calls = 0,
    notifications = 0;
  const c = new Controller(
    {
      read: async () => {
        calls++;
        return read('"v"');
      },
      update: async () => ({ etag: '"next"' }),
    },
    "s",
    () => identity,
  );
  await c.refresh();
  c.subscribe(() => notifications++);
  identity = null;
  assert.equal(c.getSnapshot().read, null);
  assert.equal(
    notifications,
    0,
    "getSnapshot must not trigger subscribers during render",
  );
  await c.refresh();
  assert.equal(calls, 1, "no private read without a session");
});

test("obsolete success/finally cannot release a newer refresh", async () => {
  const Controller = await loadController();
  const old = deferred(),
    current = deferred();
  let calls = 0;
  const c = new Controller(
    {
      read: () => (++calls === 1 ? old.promise : current.promise),
      update: async () => ({ etag: '"v"' }),
    },
    "s",
    () => "A",
  );
  const first = c.refresh();
  const second = c.refresh();
  old.resolve(read('"obsolete"'));
  await first;
  assert.equal(c.getSnapshot().phase, "loading");
  assert.equal(c.getSnapshot().read, null);
  current.resolve(read('"current"'));
  await second;
  assert.equal(c.getSnapshot().read.etag, '"current"');
});
