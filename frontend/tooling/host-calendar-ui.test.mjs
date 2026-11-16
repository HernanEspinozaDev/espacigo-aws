import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const root = new URL("../src/features/host-listings/ui/", import.meta.url);
const component = await readFile(
  new URL("host-availability-calendar.tsx", root),
  "utf8",
);
const hook = await readFile(
  new URL("use-host-calendar-configuration.ts", root),
  "utf8",
);

test("host calendar delegates private configuration to controller and avoids raw availability PUT", () => {
  assert.match(component, /useHostCalendarConfiguration\(props\.spaceId\)/);
  assert.match(component, /controller\.save\(payload\)/);
  assert.doesNotMatch(component, /fetch\s*\(/);
  assert.match(component, /Crear el espacio primero para guardar/);
  assert.match(
    component,
    /check_in_hora:\s*rateUnit === "dia" \? openTime : rule\.check_in_hora/,
  );
  assert.match(
    component,
    /check_out_hora:\s*rateUnit === "dia" \? closeTime : rule\.check_out_hora/,
  );
  assert.match(
    component,
    /hora_apertura:\s*rateUnit === "dia" \? rule\.hora_apertura : openTime/,
  );
  assert.match(
    component,
    /hora_cierre:\s*rateUnit === "dia" \? rule\.hora_cierre : closeTime/,
  );
  assert.match(hook, /controller\.refresh\(\)/);
  assert.match(hook, /getServerSnapshot/);
});
