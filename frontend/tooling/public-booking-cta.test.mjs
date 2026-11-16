import assert from "node:assert/strict";
import test from "node:test";
import { createRequire } from "node:module";
import ts from "typescript";

const require = createRequire(import.meta.url);
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const root = new URL("../src/features/public-catalog/", import.meta.url);

function loadTs(path, aliases = {}) {
  const source = require("node:path").resolve(new URL(path, root).pathname);
  const output = ts.transpileModule(
    require("node:fs").readFileSync(source, "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
        jsx: ts.JsxEmit.ReactJSX,
      },
    },
  );
  const loaded = { exports: {} };
  const localRequire = (id) => aliases[id] ?? require(id);
  new Function("require", "module", "exports", output.outputText)(
    localRequire,
    loaded,
    loaded.exports,
  );
  return loaded.exports;
}

const booking = loadTs("domain/marketplace-booking-selection.ts", {
  "@/lib/api/clp": require("../src/lib/api/clp.ts"),
  "./media": require("../src/features/public-catalog/domain/media.ts"),
  "./availability-return": require("../src/features/public-catalog/domain/availability-return.ts"),
});
const presentation = loadTs("domain/presentation.ts", {
  "@/lib/api/clp": require("../src/lib/api/clp.ts"),
});
const { PublicSpaceBookingCta } = loadTs("ui/public-space-booking-cta.tsx", {
  "next/link": {
    default: ({ href, children, ...props }) =>
      React.createElement("a", { href, ...props }, children),
  },
  "../domain/marketplace-booking-selection": booking,
  "../domain/presentation": presentation,
  react: React,
});

const selection = {
  spaceId: "123e4567-e89b-42d3-a456-426614174000",
  date: "2026-12-01",
  duration: 2,
  rateUnit: "hora",
  startAt: "2026-12-01T09:00:00-03:00",
  endAt: "2026-12-01T11:00:00-03:00",
  timeZone: "America/Santiago",
};
const render = (props) =>
  renderToStaticMarkup(React.createElement(PublicSpaceBookingCta, props));

test("selection null renders disabled calendar button without href", () => {
  const html = render({
    selection: null,
    basePriceCLP: 5001,
    rateUnit: "hora",
  });
  assert.match(
    html,
    /<button(?=[^>]*\bdisabled="")[^>]*>Selecciona fechas en el calendario<\/button>/,
  );
  assert.doesNotMatch(html, /<a\b/);
});

test("valid selection renders exact booking href and no invented guarantee amount", () => {
  const html = render({ selection, basePriceCLP: 9000, rateUnit: "hora" });
  assert.match(html, /href="\/reservas\/nueva\?[^\"]+"/);
  assert.doesNotMatch(html, /50\.000|Garantía de arriendo/);
  assert.match(html, /Estimación referencial/);
  assert.match(html, /no ocupa ni confirma una reserva o pago/);
});
