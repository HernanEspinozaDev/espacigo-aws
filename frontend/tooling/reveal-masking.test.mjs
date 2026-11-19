import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const cacheDir = path.join(root, "node_modules", ".cache");
await mkdir(cacheDir, { recursive: true });
const scratch = await mkdtemp(path.join(cacheDir, "masking-test-"));

async function compile(sourcePath, outputName) {
  let source = await readFile(path.join(root, sourcePath), "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const destination = path.join(scratch, outputName);
  await writeFile(destination, output);
  return destination;
}

const modulePath = await compile(
  "src/components/reveal-email.tsx",
  "reveal-email.mjs",
);

test.after(async () => {
  await rm(scratch, { recursive: true, force: true });
});

const { maskEmail, maskRUT } = await import(pathToFileURL(modulePath).href);

test("maskEmail masks valid emails preserving first and last characters", () => {
  assert.equal(maskEmail("ana@ejemplo.cl"), "a••••l");
  assert.equal(maskEmail("user@domain.com"), "u••••m");
  assert.equal(maskEmail("test@test.org"), "t••••g");
  assert.equal(maskEmail("invalid"), "••••");
  assert.equal(maskEmail(""), "••••");
  assert.equal(maskEmail("a@"), "••••");
});

test("maskRUT masks valid RUTs preserving last 4 characters including DV", () => {
  assert.equal(maskRUT("12.345.678-5"), "••••6785");
  assert.equal(maskRUT("12345678-5"), "••••6785");
  assert.equal(maskRUT("11.111.111-1"), "••••1111");
  assert.equal(maskRUT("22.222.222-2"), "••••2222");
  assert.equal(maskRUT("12.345.678-k"), "••••678K");
  assert.equal(maskRUT("12.345.678-K"), "••••678K");
  assert.equal(maskRUT("1234"), "••••1234");
  assert.equal(maskRUT("123"), "••••");
  assert.equal(maskRUT(""), "••••");
  assert.equal(maskRUT("invalid"), "••••");
  assert.equal(maskRUT("••••6785"), "••••6785");
  assert.equal(maskRUT("••••"), "••••");
});
