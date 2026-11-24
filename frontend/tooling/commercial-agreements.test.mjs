import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import ts from "typescript";
const dir = await mkdtemp(path.join(os.tmpdir(), "commercial-agreements-"));
test.after(() => rm(dir, { recursive: true, force: true }));
async function compile(file, output, replacements = []) {
  let source = await readFile(new URL(file, import.meta.url), "utf8");
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
}
const { ApiError, ApiProtocolError } = await compile(
  "../src/lib/api/errors.ts",
  "errors.mjs",
);
await compile(
  "../src/features/host-listings/domain/contract-template.ts",
  "templates.mjs",
);
const domain = await compile(
  "../src/features/commercial-contracts/domain/agreements.ts",
  "domain.mjs",
  [
    ['from "@/lib/api/errors"', 'from "./errors.mjs"'],
    [
      'from "@/features/host-listings/domain/contract-template"',
      'from "./templates.mjs"',
    ],
  ],
);
const { OpenApiAgreementsAdapter } = await compile(
  "../src/features/commercial-contracts/infrastructure/agreements-openapi-adapter.ts",
  "adapter.mjs",
  [
    ['import "client-only";', ""],
    ['import { apiClient } from "@/lib/api/client";', "const apiClient={};"],
    ['from "@/lib/api/errors"', 'from "./errors.mjs"'],
    [
      'import { serializeExactJson } from "@/lib/api/exact-json";',
      "const serializeExactJson=JSON.stringify;",
    ],
    ['from "../domain/agreements"', 'from "./domain.mjs"'],
  ],
);
const { AgreementsService } = await compile(
  "../src/features/commercial-contracts/application/agreements.service.ts",
  "service.mjs",
);
const ids = Array.from(
  { length: 5 },
  (_, i) => `10000000-0000-4000-8000-00000000000${i + 1}`,
);
const sha = "a".repeat(64),
  at = "2030-01-01T12:00:00Z";
const proposal = {
  id: ids[0],
  version: 1,
  sha256: sha,
  quote_id: ids[1],
  space_id: ids[2],
  template_id: ids[3],
  template_version: 1,
  space_title: "Sala sintética",
  location: {
    address: "Dirección de prueba 123",
    latitude: -33,
    longitude: -70,
    revision: 1,
  },
  sections: [{ key: "uso", title: "Uso", body: "Respetar horarios." }],
  rate_unit: "hora",
  rate_version: 1,
  unit_price_clp: 12000,
  units: 1,
  subtotal_clp: 12000,
  currency: "CLP",
  start_at: at,
  end_at: "2030-01-01T13:00:00Z",
  time_zone: "America/Santiago",
  cancellation_policy: {
    version: "synthetic-v1",
    text: "Condiciones de prueba.",
  },
  parties: [
    { role: "arrendador", self: false },
    {
      role: "arrendatario",
      self: true,
      legal_name: "Nombre propio",
      rut: "••••2222",
    },
  ],
  contact_notice: "Contactos después de confirmar el pago.",
  generated_at: at,
};
const signature = (role) => ({
  role,
  signed_at: at,
  snapshot_sha256: sha,
  proposal_version: 1,
  consent_version: domain.agreementConsentVersion,
});
const agreement = {
  id: ids[4],
  proposal_id: ids[0],
  quote_id: ids[1],
  space_id: ids[2],
  proposal_version: 1,
  snapshot_sha256: sha,
  state: "pending_landlord",
  expires_at: at,
  payment_expires_at: null,
  revision: 1,
  created_at: at,
  updated_at: at,
  signatures: [signature("arrendatario")],
};

test("proposal validation rejects counterpart PII, unmasked own identifiers and contact clauses", () => {
  assert.equal(domain.parseProposal(proposal, { quoteId: ids[1] }), proposal);
  for (const changed of [
    {
      ...proposal,
      parties: [
        { ...proposal.parties[0], legal_name: "Counterpart" },
        proposal.parties[1],
      ],
    },
    {
      ...proposal,
      parties: [
        proposal.parties[0],
        { ...proposal.parties[1], rut: "22222222-2" },
      ],
    },
    {
      ...proposal,
      sections: [
        { key: "uso", title: "Contacto", body: "Llama al +56 9 1234 5678" },
      ],
    },
    { ...proposal, sha256: "bad" },
  ])
    assert.throws(() => domain.parseProposal(changed));
});
test("payment presentation requires both matching signatures and an unexpired server payment deadline", () => {
  assert.equal(domain.canPayAgreement(agreement, 0), false);
  assert.throws(() =>
    domain.parseAgreement({
      ...agreement,
      state: "ready_for_payment",
      payment_expires_at: at,
    }),
  );
  const ready = {
    ...agreement,
    state: "ready_for_payment",
    payment_expires_at: at,
    signatures: [signature("arrendatario"), signature("arrendador")],
  };
  assert.equal(
    domain.canPayAgreement(domain.parseAgreement(ready), Date.parse(at) - 1),
    true,
  );
  assert.equal(domain.canPayAgreement(ready, Date.parse(at)), false);
  assert.throws(() =>
    domain.parseAgreement({
      ...ready,
      signatures: [
        signature("arrendatario"),
        { ...signature("arrendador"), snapshot_sha256: "b".repeat(64) },
      ],
    }),
  );
});
test("send uses a fixed versioned consent and the caller's retry key with an exact snapshot payload", async () => {
  const captures = [];
  const adapter = new OpenApiAgreementsAdapter({
    POST: async (route, options) => {
      captures.push({ route, options });
      return {
        response: new Response(null, {
          headers: { ETag: domain.agreementEtag(agreement) },
        }),
        data: agreement,
      };
    },
  });
  await adapter.send(proposal, "same-intent");
  await adapter.send(proposal, "same-intent");
  assert.equal(captures[0].route, "/commercial-agreements");
  assert.deepEqual(captures[0].options.body, {
    proposal_id: proposal.id,
    snapshot_sha256: sha,
    consent: true,
    consent_version: "commercial-agreement-v1",
  });
  assert.equal(
    captures[1].options.params.header["Idempotency-Key"],
    "same-intent",
  );
});
test("capability errors remain errors and resume rejects a changed snapshot", async () => {
  for (const result of [
    { response: new Response(null, { status: 503 }), error: {} },
    { response: new Response(null), data: { data: {} } },
  ]) {
    const adapter = new OpenApiAgreementsAdapter({ GET: async () => result });
    await assert.rejects(adapter.capabilities());
  }
  let scoped = 0;
  const service = new AgreementsService(
    {
      get: async () => ({ agreement, etag: domain.agreementEtag(agreement) }),
      readProposal: async () => ({ ...proposal, sha256: "b".repeat(64) }),
    },
    async (operation) => {
      scoped++;
      return operation(new AbortController().signal);
    },
  );
  await assert.rejects(service.resume(agreement.id), /snapshot mismatch/);
  assert.equal(scoped, 1);
});

function makeApiError(status, code) {
  return new ApiError({
    status,
    code,
    requestId: null,
    retryAfter: null,
    fields: [],
  });
}

test("agreementErrorMessage surfaces specific OTP, conflict, validation, and protocol errors", () => {
  const cases = [
    [
      makeApiError(409, "otp_invalid"),
      "El código de firma es inválido o superó el máximo de intentos. Solicita uno nuevo.",
    ],
    [
      makeApiError(409, "otp_expired"),
      "El código de firma expiró (5 minutos). Solicita un código nuevo.",
    ],
    [
      makeApiError(422, "otp_required"),
      "Ingresa el código de 6 dígitos enviado a tu correo antes de confirmar.",
    ],
    [
      makeApiError(409, "agreement_conflict"),
      "La cotización venció (6 minutos) o cambió el estado de la solicitud. Actualiza la cotización para continuar.",
    ],
    [
      makeApiError(409, "idempotency_conflict"),
      "Ya se procesó un intento previo con este envío. Actualiza el estado o solicita un nuevo código.",
    ],
    [
      makeApiError(422, "validation_error"),
      "Los datos de firma no son válidos. Revisa el código e inténtalo nuevamente.",
    ],
    [
      makeApiError(401, "unauthenticated"),
      "Inicia sesión con la cuenta que participa en esta solicitud.",
    ],
    [
      makeApiError(404, "not_found"),
      "La cotización o la solicitud ya no están disponibles para esta cuenta.",
    ],
    [
      makeApiError(503, "service_unavailable"),
      "No pudimos confirmar la operación. Actualiza el estado antes de continuar o reintenta el envío.",
    ],
    [
      new ApiProtocolError(),
      "No pudimos confirmar la respuesta del servidor. Actualiza la página o intenta nuevamente.",
    ],
    [
      new TypeError("Failed to fetch"),
      "No pudimos confirmar la operación. Actualiza la página o reintenta el envío.",
    ],
  ];
  for (const [err, expected] of cases) {
    assert.equal(domain.agreementErrorMessage(err), expected);
  }
});

test("signing idempotency key rotates across OTP challenges and definitive 4xx errors while preserving transient 5xx retries", () => {
  let seq = 0;
  const nextKey = () => `key-${++seq}`;

  const first = domain.nextSigningIdempotencyKey(null, null, "ch-1", nextKey);
  assert.deepEqual(first, { idempotencyKey: "key-1", challengeId: "ch-1" });

  // Transient 5xx / network error preserves key for retry with same challenge
  assert.equal(
    domain.shouldClearSigningIdempotencyKey(
      makeApiError(503, "service_unavailable"),
    ),
    false,
  );
  assert.equal(
    domain.shouldClearSigningIdempotencyKey(new TypeError("network")),
    false,
  );
  const sameChallengeRetry = domain.nextSigningIdempotencyKey(
    first.idempotencyKey,
    first.challengeId,
    "ch-1",
    nextKey,
  );
  assert.deepEqual(sameChallengeRetry, {
    idempotencyKey: "key-1",
    challengeId: "ch-1",
  });

  // New OTP challenge rotates Idempotency-Key even if previous key wasn't cleared
  const rotatedChallenge = domain.nextSigningIdempotencyKey(
    first.idempotencyKey,
    first.challengeId,
    "ch-2",
    nextKey,
  );
  assert.deepEqual(rotatedChallenge, {
    idempotencyKey: "key-2",
    challengeId: "ch-2",
  });

  // Definitive 4xx errors (400, 409, 422) clear the key so next retry gets a fresh one
  for (const [status, code] of [
    [400, "invalid_request"],
    [409, "otp_invalid"],
    [409, "idempotency_conflict"],
    [422, "validation_error"],
  ]) {
    assert.equal(
      domain.shouldClearSigningIdempotencyKey(makeApiError(status, code)),
      true,
    );
  }

  const afterCleared = domain.nextSigningIdempotencyKey(
    null,
    null,
    "ch-2",
    nextKey,
  );
  assert.deepEqual(afterCleared, {
    idempotencyKey: "key-3",
    challengeId: "ch-2",
  });
});

test("isQuoteRefreshErrorCode identifies quote-refreshable conflict and expiry codes", () => {
  for (const code of [
    "agreement_conflict",
    "request_expired",
    "proposal_changed",
    "contract_terms_changed",
  ]) {
    assert.equal(domain.isQuoteRefreshErrorCode(code), true);
  }
  for (const code of ["otp_invalid", "otp_expired", "validation_error", null]) {
    assert.equal(domain.isQuoteRefreshErrorCode(code), false);
  }
});
