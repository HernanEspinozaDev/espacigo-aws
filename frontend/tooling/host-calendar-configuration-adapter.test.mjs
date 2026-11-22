import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "host-calendar-"),
);
const source = await readFile(
  path.join(
    root,
    "src/features/host-listings/infrastructure/calendar-configuration-openapi-adapter.ts",
  ),
  "utf8",
);
const domainSource = await readFile(
  path.join(
    root,
    "src/features/host-listings/domain/calendar-configuration.ts",
  ),
  "utf8",
);
const transformedDomain = domainSource.replace(
  'import type { components } from "@/lib/api/generated/openapi";',
  "",
);
await writeFile(
  path.join(scratch, "domain.mjs"),
  ts.transpileModule(transformedDomain, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText,
);
const transformed = source
  .replace('import "client-only";', "")
  .replace(
    'import { apiClient } from "@/lib/api/client";',
    "const apiClient = globalThis.__calendarClient;",
  )
  .replace(
    'import { ApiProtocolError, normalizeApiError } from "@/lib/api/errors";',
    'const ApiProtocolError = class extends Error {}; const normalizeApiError = (response) => Object.assign(new Error("api error"), { status: response.status });',
  )
  .replace(
    'import { serializeExactJson } from "@/lib/api/exact-json";',
    "const serializeExactJson = JSON.stringify;",
  )
  .replace('import type { components } from "@/lib/api/generated/openapi";', "")
  .replace(
    /import \{[^]*?\} from "\.\.\/domain\/calendar-configuration";/,
    'import { strongEtag, validateCalendarConfiguration, validateCalendarUpdate } from "./domain.mjs";',
  );
await writeFile(
  path.join(scratch, "adapter.mjs"),
  ts.transpileModule(transformed, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText,
);
let calls = [],
  next;
globalThis.__calendarClient = {
  GET: async (...args) => {
    calls.push(args);
    return next;
  },
  PUT: async (...args) => {
    calls.push(args);
    return next;
  },
};
const { OpenApiCalendarConfigurationAdapter } = await import(
  pathToFileURL(path.join(scratch, "adapter.mjs"))
);
const dto = {
  space_id: "s-1",
  time_zone: "America/Santiago",
  rule: {
    space_id: "s-1",
    dias_operacion: [1, 3],
    hora_apertura: "09:00",
    hora_cierre: "17:00",
    check_in_hora: "09:00",
    check_out_hora: "17:00",
    tarifa_base_dia: 5001,
    estadia_minima: 1,
    estadia_maxima: 7,
    actualizado_en: "2026-11-16T00:00:00Z",
  },
  overrides: [],
  updated_at: "2026-11-16T00:00:00Z",
};
function response(status = 200, etag = '"rev-2"', data = dto) {
  return {
    response: {
      ok: status >= 200 && status < 300,
      status,
      headers: { get: (name) => (name === "ETag" ? etag : null) },
    },
    data,
  };
}
function reset(result = response()) {
  calls = [];
  next = result;
}
test.after(async () => rm(scratch, { recursive: true, force: true }));
test("GET returns exact validated configuration and strong ETag with signal", async () => {
  reset();
  const controller = new AbortController();
  const result = await new OpenApiCalendarConfigurationAdapter().read(
    "s-1",
    controller.signal,
  );
  assert.deepEqual(result.configuration, dto);
  assert.equal(result.etag, '"rev-2"');
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/spaces/{spaceId}/availability/configuration");
  assert.deepEqual(calls[0][1].params, { path: { spaceId: "s-1" } });
  assert.equal(calls[0][1].signal, controller.signal);
});
test("GET fails closed for invalid DTO, timezone, or missing/weak ETag", async () => {
  for (const [data, etag] of [
    [{ ...dto, extra: true }, '"v"'],
    [{ ...dto, time_zone: "Not/AZone" }, '"v"'],
    [dto, null],
    [dto, 'W/"v"'],
  ]) {
    reset(response(200, etag, data));
    await assert.rejects(() =>
      new OpenApiCalendarConfigurationAdapter().read("s-1"),
    );
  }
});
test("PUT sends exact body, If-Match and signal; returns ACK and ETag without replay", async () => {
  reset(
    response(200, '"rev-3"', {
      space_id: "s-1",
      time_zone: "America/Santiago",
    }),
  );
  const input = {
    time_zone: "America/Santiago",
    dias_operacion: [1, 3],
    hora_apertura: "09:00",
    hora_cierre: "17:00",
    check_in_hora: "09:00",
    check_out_hora: "17:00",
    tarifa_base_dia: 5001,
    estadia_minima: 1,
    estadia_maxima: 7,
    overrides: [],
  };
  const controller = new AbortController();
  const ack = await new OpenApiCalendarConfigurationAdapter().update(
    "s-1",
    input,
    '"rev-2"',
    controller.signal,
  );
  assert.deepEqual(ack, { etag: '"rev-3"' });
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/spaces/{spaceId}/availability/configuration");
  assert.deepEqual(calls[0][1].params, {
    path: { spaceId: "s-1" },
    header: { "If-Match": '"rev-2"' },
  });
  assert.deepEqual(calls[0][1].body, input);
  assert.equal(calls[0][1].signal, controller.signal);
});
test("invalid updates fail before dispatch; HTTP errors propagate without secret payloads or replay", async () => {
  const adapter = new OpenApiCalendarConfigurationAdapter();
  for (const etag of [undefined, 'W/"weak"', "bad"]) {
    reset();
    await assert.rejects(() => adapter.update("s-1", {}, etag));
    assert.equal(calls.length, 0);
  }
  for (const status of [401, 403, 412, 500]) {
    reset(response(status));
    await assert.rejects(
      () => adapter.read("s-1"),
      (error) => error.status === status && !error.message.includes("secret"),
    );
    assert.equal(calls.length, 1);
  }
});

test("calendar contract accepts 24:00 closing and rejects empty updates/days/ETag", async () => {
  const adapter = new OpenApiCalendarConfigurationAdapter();
  const fullDay = { ...dto, rule: { ...dto.rule, hora_cierre: "24:00" } };
  reset(response(200, '"v"', fullDay));
  assert.equal(
    (await adapter.read("s-1")).configuration.rule.hora_cierre,
    "24:00",
  );
  for (const input of [{}, { dias_operacion: [] }]) {
    reset();
    await assert.rejects(() => adapter.update("s-1", input, '"v"'));
    assert.equal(calls.length, 0);
  }
  reset();
  await assert.rejects(() => adapter.update("s-1", { overrides: [] }, '""'));
  assert.equal(calls.length, 0);
});

test("optional undefined keys are omitted before strict payload serialization", async () => {
  reset(
    response(200, '"next"', { space_id: "s-1", time_zone: "America/Santiago" }),
  );
  await new OpenApiCalendarConfigurationAdapter().update(
    "s-1",
    {
      hora_cierre: "20:00",
      tarifa_base_dia: undefined,
      overrides: [
        {
          fecha: "2027-01-02",
          estado: "disponible",
          tarifa_personalizada_clp: undefined,
        },
      ],
    },
    '"v"',
  );
  assert.deepEqual(calls[0][1].body, {
    hora_cierre: "20:00",
    overrides: [{ fecha: "2027-01-02", estado: "disponible" }],
  });
});

test("calendar contract supports disponible_desde and estadia_minima_meses", async () => {
  const adapter = new OpenApiCalendarConfigurationAdapter();
  const monthRuleDto = {
    ...dto,
    rule: {
      ...dto.rule,
      disponible_desde: "2027-03-01",
      estadia_minima_meses: 3,
    },
  };
  reset(response(200, '"etag-month"', monthRuleDto));
  const readRes = await adapter.read("s-1");
  assert.equal(readRes.configuration.rule.disponible_desde, "2027-03-01");
  assert.equal(readRes.configuration.rule.estadia_minima_meses, 3);

  reset(
    response(200, '"etag-next"', {
      space_id: "s-1",
      time_zone: "America/Santiago",
    }),
  );
  await adapter.update(
    "s-1",
    {
      disponible_desde: "2027-04-01",
      estadia_minima_meses: 6,
    },
    '"etag-month"',
  );
  assert.deepEqual(calls[0][1].body, {
    disponible_desde: "2027-04-01",
    estadia_minima_meses: 6,
  });

  // Rejects invalid date format or out-of-range min months
  await assert.rejects(() =>
    adapter.update("s-1", { disponible_desde: "not-a-date" }, '"etag-month"'),
  );
  await assert.rejects(() =>
    adapter.update("s-1", { estadia_minima_meses: 0 }, '"etag-month"'),
  );
  await assert.rejects(() =>
    adapter.update("s-1", { estadia_minima_meses: 25 }, '"etag-month"'),
  );
});
