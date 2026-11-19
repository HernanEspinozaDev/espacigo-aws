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
let observedPrefetch;
const { PublicSpaceBookingCta } = loadTs("ui/public-space-booking-cta.tsx", {
  "next/link": {
    default: ({ href, prefetch, children, ...props }) => {
      observedPrefetch = prefetch;
      return React.createElement("a", { href, ...props }, children);
    },
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

test("sin selección muestra botón calendario e invocación enlazada por React", () => {
  const html = render({
    selection: null,
    basePriceCLP: 5001,
    rateUnit: "hora",
  });
  assert.match(html, /aria-label="Solicitud de arriendo"/);
  assert.match(
    html,
    /<button[^>]*>Selecciona fechas en el calendario<\/button>/,
  );
  assert.doesNotMatch(html, /<a\b/);
});

test("selección válida presenta fechas, duración, subtotal exacto y href validado", () => {
  const html = render({
    selection,
    basePriceCLP: "9007199254740000",
    rateUnit: "hora",
  });
  assert.match(html, /Solicitud de arriendo/);
  assert.match(html, /2026/);
  assert.match(html, /2 horas/);
  assert.match(html, /18\.014\.398\.509\.480\.000/);
  assert.match(html, /href="\/reservas\/nueva\?/);
  assert.equal(observedPrefetch, false);
  assert.match(html, /El siguiente paso calcula una cotización oficial/);
  assert.match(html, /El pago se confirma al volver desde Mercado Pago/);
});

test("importe inválido o unsafe no activa enlace ni inventa un total", () => {
  for (const basePriceCLP of [
    "no-precio",
    Number.MAX_SAFE_INTEGER + 1,
    "9223372036854775808",
  ]) {
    const html = render({ selection, basePriceCLP, rateUnit: "hora" });
    assert.doesNotMatch(html, /<a\b/);
    assert.match(html, /<button\b/);
    assert.doesNotMatch(html, /Subtotal estimado:.*(?:0 CLP|NaN)/);
  }
});

test("selección rechazada por helpers reales queda sin enlace", () => {
  const html = render({
    selection: { ...selection, spaceId: "bad" },
    basePriceCLP: 5001,
    rateUnit: "hora",
    mobile: true,
  });
  assert.match(html, /Solicitud de arriendo móvil/);
  assert.doesNotMatch(html, /<a\b/);
});
