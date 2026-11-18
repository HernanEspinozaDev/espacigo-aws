"use strict";

const el = (id) => document.getElementById(id);
const api = "/api/v1";
const rentals = api + "/local/early-rentals";
const errors = {
  unauthenticated: "Inicia sesión con una cuenta de prueba válida.",
  forbidden: "Tu cuenta no puede realizar esta acción.",
  origin_denied: "Abre la demo desde la dirección local configurada.",
  not_found: "No se encontró el arriendo para esta cuenta.",
  expired: "La retención venció. Elige otra hora para comenzar.",
  conflict: "La hora ya no está disponible o faltan confirmaciones.",
  stale_revision: "El arriendo cambió. Actualiza el estado y vuelve a intentar.",
  invalid_request: "Revisa los datos que ingresaste."
};

let token = "";
let offer = null;
let etag = "";
let busy = false;
let slots = [];
let slotSpace = "";
let creationKey = "";

function message(value) {
  el("status").textContent = value;
}

async function request(path, { method = "GET", body, headers = {} } = {}) {
  if (token) headers.Authorization = "Bearer " + token;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method, headers, body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store"
  });
  const json = response.status === 204 ? null : await response.json().catch(() => null);
  if (!response.ok) {
    const code = json?.error?.code;
    throw new Error(errors[code] || (response.status === 401
      ? errors.unauthenticated : "No se pudo completar la operación."));
  }
  return { data: json?.data ?? json, etag: response.headers.get("ETag") };
}

async function run(work) {
  if (busy) return;
  busy = true;
  message("");
  controls();
  try {
    await work();
  } catch (error) {
    message(error.message);
  } finally {
    busy = false;
    controls();
  }
}

function money(amount) {
  return new Intl.NumberFormat("es-CL", { style: "currency", currency: "CLP" }).format(amount);
}

function dateTime(value, zone) {
  return new Intl.DateTimeFormat("es-CL", {
    dateStyle: "medium", timeStyle: "short", timeZone: zone
  }).format(new Date(value));
}

function controls() {
  const actions = new Set(offer?.allowed_actions ?? []);
  el("identity").disabled = busy || !actions.has("confirmar_identidad_simulada");
  const canSign = !busy && actions.has("aceptar_contrato");
  el("approval").disabled = !canSign;
  el("signature").disabled = !canSign;
  el("sign").disabled = !canSign || !el("approval").checked || !el("signature").checked;
  el("pay").disabled = busy || !actions.has("marcar_pago_enviado");
  el("pay").hidden = offer?.actor_role !== "arrendatario" || !!offer.invoice;
  el("refresh").disabled = busy;
}

function counterpartStatus(pair) {
  return "Arrendatario: " + (pair.arrendatario ? "listo" : "pendiente") +
    " · Arrendador: " + (pair.arrendador ? "listo" : "pendiente");
}

function show(result) {
  offer = result.data;
  etag = result.etag;
  const role = offer.actor_role;
  const signed = Boolean(offer.contract.acceptances[role]);
  el("rental-id").value = offer.id;
  const link = new URL(window.location.href);
  link.searchParams.set("arriendo", offer.id);
  history.replaceState(null, "", link);
  el("share").href = link.href;
  el("space-title").textContent = offer.space_title;
  el("summary").textContent = role === "arrendatario" ? "Arrendatario" : "Arrendador";
  el("summary").textContent += " · " + dateTime(offer.start_at, offer.time_zone) + " · " + money(offer.subtotal_clp);
  el("terms").textContent = offer.contract.terms_text;
  el("identity").checked = Boolean(offer.identity_confirmations[role]);
  el("approval").checked = signed;
  el("signature").checked = signed;
  el("identities").textContent = counterpartStatus(offer.identity_confirmations);
  el("signatures").textContent = counterpartStatus(offer.contract.acceptances);
  el("payment-status").textContent = offer.invoice ? "Pago aprobado · Arriendo confirmado"
    : Date.now() >= Date.parse(offer.expires_at) ? "La retención venció."
    : offer.state === "pendiente_envio_pago" ? "Listo para pagar."
    : "Pendiente de las confirmaciones de ambos participantes.";
  el("invoice").hidden = !offer.invoice;
  if (offer.invoice) {
    el("invoice-number").textContent = offer.invoice.number;
    el("invoice-total").textContent = money(offer.invoice.amount_clp);
    el("invoice-date").textContent = dateTime(offer.invoice.approved_at, offer.time_zone);
    el("invoice-payment").textContent = offer.invoice.payment_id;
  }
  el("detail").hidden = false;
  el("creation").hidden = true;
  controls();
}

async function openRental(id) {
  show(await request(rentals + "/" + encodeURIComponent(id)));
}

async function command(path, body) {
  const url = rentals + "/" + offer.id + "/" + path;
  const headers = { "If-Match": etag };
  if (path !== "pay") headers["Idempotency-Key"] = offer.id + ":" + path;
  show(await request(url, { method: "POST", body, headers }));
}

function clearOffer() {
  offer = null;
  etag = "";
  el("detail").hidden = true;
  el("creation").hidden = false;
  controls();
}

function invalidateSlots() {
  slots = [];
  slotSpace = "";
  creationKey = "";
  el("create").hidden = true;
}

function submit(id, work) {
  el(id).addEventListener("submit", (event) => {
    event.preventDefault();
    void run(work);
  });
}

submit("login", async () => {
  const password = el("password").value;
  el("password").value = "";
  const result = await request(api + "/auth/login", {
    method: "POST", body: { email: el("email").value, password }
  });
  token = result.data.access_token;
  el("login").hidden = true;
  el("workspace").hidden = false;
  if (el("rental-id").value) await openRental(el("rental-id").value.trim());
});

el("logout").addEventListener("click", () => void run(async () => {
  try {
    await request(api + "/auth/logout", { method: "POST" });
  } finally {
    token = "";
    clearOffer();
    invalidateSlots();
    el("login").hidden = false;
    el("workspace").hidden = true;
  }
}));

submit("open", () => openRental(el("rental-id").value.trim()));
el("new").addEventListener("click", () => {
  if (busy) return;
  clearOffer();
  invalidateSlots();
  el("rental-id").value = "";
  history.replaceState(null, "", window.location.pathname);
  el("space-id").focus();
});
el("refresh").addEventListener("click", () => void run(() => openRental(offer.id)));

for (const id of ["space-id", "date"]) {
  el(id).addEventListener("change", invalidateSlots);
}

submit("availability", async () => {
  invalidateSlots();
  const space = el("space-id").value.trim();
  const date = el("date").value;
  const result = await request(api + "/spaces/" + encodeURIComponent(space) + "/rental-availability?date=" + date + "&duration=1");
  if (result.data.rate_unit !== "hora") throw new Error("Selecciona un espacio con tarifa por hora.");
  slots = result.data.items ?? [];
  slotSpace = space;
  if (slots.length === 0) throw new Error("No hay horas disponibles para esa fecha.");
  el("slot").replaceChildren(...slots.map((slot, index) => new Option(dateTime(slot.start_at, result.data.time_zone), index)));
  creationKey = crypto.randomUUID();
  el("create").hidden = false;
});

el("slot").addEventListener("change", () => { creationKey = crypto.randomUUID(); });

submit("create", async () => {
  const slot = slots[Number(el("slot").value)];
  if (!slot || !slotSpace) return;
  const result = await request(rentals, {
    method: "POST",
    body: { space_id: slotSpace, ...slot },
    headers: { "Idempotency-Key": creationKey }
  });
  show(result);
  invalidateSlots();
});

el("identity").addEventListener("change", () => {
  el("identity").checked = false;
  void run(() => command("identity-confirmations", {}));
});

el("approval").addEventListener("change", controls);
el("signature").addEventListener("change", controls);
el("sign").addEventListener("click", () => void run(() => command("contract-acceptances", {
  contract_version: offer.contract.version, contract_digest: offer.contract.digest
})));
el("pay").addEventListener("click", () => void run(() => command("pay", {})));

el("rental-id").value = new URLSearchParams(location.search).get("arriendo") ?? "";
const today = new Intl.DateTimeFormat("sv-SE", { timeZone: "America/Santiago" }).format(new Date());
el("date").value = today;
el("date").min = today;

void request(api + "/public/spaces?rate_unit=hora").then(({ data }) => {
  el("spaces").replaceChildren(...data.items.map((space) => new Option(space.title, space.space_id)));
}).catch(() => { /* The selector also accepts a published space UUID. */ });
