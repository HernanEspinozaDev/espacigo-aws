import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ts from "typescript";

const require = createRequire(import.meta.url);
const sourcePath = new URL(
  "../src/features/public-catalog/ui/public-space-details.tsx",
  import.meta.url,
);
const presentationPath = new URL(
  "../src/features/public-catalog/domain/presentation.ts",
  import.meta.url,
);
const clpPath = new URL("../src/lib/api/clp.ts", import.meta.url);
const source = await readFile(sourcePath, "utf8").catch(() => "");
const presentationSource = await readFile(presentationPath, "utf8");
const clpSource = await readFile(clpPath, "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    jsx: ts.JsxEmit.ReactJSX,
    target: ts.ScriptTarget.ES2022,
  },
});
const componentModule = { exports: {} };
const compile = (text) =>
  ts.transpileModule(text, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
const clpModule = { exports: {} };
new Function("require", "module", "exports", compile(clpSource))(
  () => ({}),
  clpModule,
  clpModule.exports,
);
const presentationModule = { exports: {} };
new Function("require", "module", "exports", compile(presentationSource))(
  (id) => (id === "@/lib/api/clp" ? clpModule.exports : {}),
  presentationModule,
  presentationModule.exports,
);
const hostAvatar = ({ name, imageUrl }) =>
  React.createElement("span", {
    "data-avatar-name": name,
    "data-avatar-url": imageUrl,
  });
const localRequire = (id) =>
  id === "react"
    ? React
    : id === "lucide-react"
      ? require(id)
      : id === "../domain/types"
        ? {}
        : id === "../domain/presentation"
          ? presentationModule.exports
          : id === "./public-host-avatar"
            ? { PublicHostAvatar: hostAvatar }
            : require(id);
new Function("require", "module", "exports", output.outputText)(
  localRequire,
  componentModule,
  componentModule.exports,
);
const Component = componentModule.exports.PublicSpaceDetails;

const space = (overrides = {}) => ({
  space_id: "id",
  category_code: "salon",
  category_name: "Salón",
  title: "Sala",
  description: "",
  area_m2: 0,
  capacity_max: 6,
  host: null,
  attributes: [],
  cancellation_policy: null,
  guarantee: null,
  rate_unit: "hora",
  base_price_clp: 5001,
  currency: "CLP",
  estimated_total_clp: null,
  usage_rules: "",
  features: [],
  images: [],
  ...overrides,
});

test("detalles públicos presentan valores tipados, anfitrión verificado y privacidad sin inventar reglas", () => {
  assert.equal(
    typeof Component,
    "function",
    "PublicSpaceDetails debe exportarse",
  );
  const html = renderToStaticMarkup(
    React.createElement(Component, {
      space: space({
        area_m2: 24,
        capacity_max: 0,
        host: {
          name: "Ana Pérez ana@example.com",
          avatar_url: "/api/backend/public/spaces/id/host/avatar",
          joined_at: "2021-04-02T12:00:00Z",
          spaces_count: 2,
          identity_verified: true,
        },
        attributes: [
          {
            code: "cero",
            label: "Valor cero",
            type: "integer",
            order: 0,
            value: 0,
          },
          {
            code: "wifi",
            label: "WiFi",
            type: "boolean",
            order: 2,
            value: false,
          },
          {
            code: "capacidad",
            label: "Capacidad",
            type: "integer",
            order: 1,
            unit: "personas",
            value: 8,
          },
          {
            code: "piso",
            label: "Tipo de piso",
            type: "enum",
            order: 3,
            value: "madera_natural",
          },
        ],
        usage_rules:
          "Sin zapatos\\nContacto ana@example.com teléfono +56 9 1234 5678 en Av. Siempre Viva 123",
      }),
    }),
  );
  for (const text of [
    "Especificaciones y reglas",
    "24 m²",
    "8<span",
    "personas</span>",
    "No",
    "madera natural",
    "Ana Pérez",
    'data-avatar-url="/api/backend/public/spaces/id/host/avatar"',
    "[contacto omitido]",
    "0</dd>",
    "Reseñas y recomendaciones",
    "Aún no hay reseñas públicas.",
    "no acredita titularidad",
    "[contacto omitido]",
  ]) {
    assert.ok(html.includes(text), `falta: ${text}`);
  }
  assert.ok(
    html.indexOf("Capacidad") < html.indexOf("WiFi"),
    "atributos ordenados por order",
  );
  assert.ok(!html.includes("50.000"));
  assert.ok(!html.includes("★★★★★"));
  const anonymous = renderToStaticMarkup(
    React.createElement(Component, { space: space() }),
  );
  assert.ok(anonymous.includes("Anfitrión"));
  assert.ok(!anonymous.includes("2021"));
  assert.ok(!anonymous.includes("espacios publicados"));
  assert.ok(anonymous.includes("Reglas del anuncio no informadas"));
  assert.ok(anonymous.includes("Política de cancelación no informada"));
  assert.ok(anonymous.includes("Garantía no informada"));
});
