import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import ts from "typescript";

const dir = await mkdtemp(path.join(os.tmpdir(), "contract-template-"));
test.after(() => rm(dir, { recursive: true, force: true }));
const compile = async (sourceFile, output, replacements = []) => {
  let source = await readFile(new URL(sourceFile, import.meta.url), "utf8");
  for (const [from, to] of replacements) source = source.replace(from, to);
  await writeFile(
    path.join(dir, output),
    ts.transpileModule(source, {
      compilerOptions: {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ESNext,
      },
    }).outputText,
  );
  return import(pathToFileURL(path.join(dir, output)).href);
};
const domain = await compile(
  "../src/features/host-listings/domain/contract-template.ts",
  "domain.mjs",
);
const errors = await compile("../src/lib/api/errors.ts", "errors.mjs");
const { ContractTemplateService } = await compile(
  "../src/features/host-listings/application/contract-template.service.ts",
  "service.mjs",
  [["../domain/contract-template", "./domain.mjs"]],
);
const { OpenApiContractTemplateAdapter } = await compile(
  "../src/features/host-listings/infrastructure/contract-template-openapi-adapter.ts",
  "adapter.mjs",
  [
    ['import "client-only";', ""],
    ['import { apiClient } from "@/lib/api/client";', "const apiClient = {};"],
    ['from "@/lib/api/errors"', 'from "./errors.mjs"'],
    [
      'import { serializeExactJson } from "@/lib/api/exact-json";',
      "const serializeExactJson = JSON.stringify;",
    ],
    ['from "../domain/contract-template"', 'from "./domain.mjs"'],
  ],
);
const space = "10000000-0000-4000-8000-000000000001";
const sections = [
  { key: "uso", title: "Uso permitido", body: "Respetar los horarios." },
];
const draft = {
  space_id: space,
  revision: 1,
  sections,
  published_version: 0,
  updated_at: "2030-01-01T10:00:00Z",
};

test("contract clauses reject executable content, duplicate sections and unsafe counters", () => {
  assert.equal(domain.validContractSections(sections), true);
  assert.equal(domain.validContractSections([...sections, ...sections]), false);
  assert.equal(
    domain.validContractSections([{ ...sections[0], body: "<script>" }]),
    false,
  );
  assert.equal(
    domain.validContractSections([{ ...sections[0], body: "reglas\u202e" }]),
    false,
  );
  assert.equal(
    domain.validTemplateDraft(
      { ...draft, revision: Number.MAX_SAFE_INTEGER + 1 },
      space,
    ),
    false,
  );
  assert.equal(domain.validTemplateDraft(draft, "another-space"), false);
});

test("template adapter sends exact clauses with server revision and validates response ETag", async () => {
  let captured;
  const adapter = new OpenApiContractTemplateAdapter({
    PUT: async (route, options) => {
      captured = { route, options };
      return {
        response: new Response(null, {
          headers: { ETag: '"contract-template-1"' },
        }),
        data: { data: draft },
      };
    },
  });
  const result = await adapter.save(space, sections, '"contract-template-0"');
  assert.equal(result.draft.revision, 1);
  assert.equal(captured.route, "/spaces/{spaceId}/contract-template/draft");
  assert.equal(
    captured.options.params.header["If-Match"],
    '"contract-template-0"',
  );
  assert.deepEqual(captured.options.body, { sections });
  const corrupt = new OpenApiContractTemplateAdapter({
    GET: async () => ({
      response: new Response(null, {
        headers: { ETag: '"contract-template-2"' },
      }),
      data: { data: draft },
    }),
  });
  await assert.rejects(corrupt.read(space), errors.ApiProtocolError);
});

test("template application rejects invalid editor content before any mutation", () => {
  let calls = 0;
  const service = new ContractTemplateService({
    save() {
      calls++;
    },
  });
  assert.throws(
    () => service.save(space, [], '"contract-template-1"'),
    TypeError,
  );
  assert.equal(calls, 0);
});

test("copy and publish use dedicated owner operations with revision; failed writes stay errors", async () => {
  const captures = [];
  const adapter = new OpenApiContractTemplateAdapter({
    POST: async (route, options) => {
      captures.push({ route, options });
      if (route.endsWith("copy"))
        return {
          response: new Response(null, {
            headers: { ETag: '"contract-template-1"' },
          }),
          data: { data: draft },
        };
      return {
        response: new Response(null, { status: 412 }),
        error: { error: { code: "precondition_failed" } },
      };
    },
  });
  await adapter.copy(space, '"contract-template-0"', space, 2);
  assert.deepEqual(captures[0].options.body, {
    source_space_id: space,
    source_version: 2,
  });
  await assert.rejects(
    adapter.publish(space, '"contract-template-1"'),
    (error) =>
      error instanceof errors.ApiError && error.code === "precondition_failed",
  );
  assert.equal(
    captures[1].options.params.header["If-Match"],
    '"contract-template-1"',
  );
});
