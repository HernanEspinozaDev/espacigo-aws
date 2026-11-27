const storageKey = "espacigo.cookie-preferences.v1";
const changeEvent = "espacigo:cookie-preferences";
const visitedSpacesKey = "espacigo.explore.visited.v1";
const policyVersion = 1;
const preferenceLifetime = 365 * 24 * 60 * 60 * 1000;

let unsavedChoice: string | undefined;

export function subscribeToCookiePreferences(onChange: () => void) {
  if (getCookiePreferences() !== "visited-spaces") clearVisitedSpaces();
  function onStorage(event: StorageEvent) {
    if (event.key !== null && event.key !== storageKey) return;
    unsavedChoice = undefined;
    if (getCookiePreferences() !== "visited-spaces") clearVisitedSpaces();
    onChange();
  }
  window.addEventListener(changeEvent, onChange);
  window.addEventListener("storage", onStorage);
  return () => {
    window.removeEventListener(changeEvent, onChange);
    window.removeEventListener("storage", onStorage);
  };
}

export function getCookiePreferences() {
  try {
    const raw = unsavedChoice ?? localStorage.getItem(storageKey);
    if (!raw) return null;
    const data = JSON.parse(raw);
    if (data?.version !== policyVersion || typeof data.chosenAt !== "string") {
      return null;
    }
    const age = Date.now() - Date.parse(data.chosenAt);
    if (!Number.isFinite(age) || age < 0 || age >= preferenceLifetime)
      return null;
    return data.visitedSpaces === true ? "visited-spaces" : "necessary";
  } catch {
    return null;
  }
}

export function saveCookiePreferences(visitedSpaces: boolean) {
  if (!visitedSpaces || getCookiePreferences() !== "visited-spaces") {
    clearVisitedSpaces();
  }
  const data = JSON.stringify({
    version: policyVersion,
    chosenAt: new Date().toISOString(),
    visitedSpaces,
  });
  try {
    localStorage.setItem(storageKey, data);
    unsavedChoice = undefined;
  } catch {
    unsavedChoice = data;
  }
  window.dispatchEvent(new Event(changeEvent));
}

function clearVisitedSpaces() {
  try {
    sessionStorage.removeItem(visitedSpacesKey);
  } catch {
    // Preference checks still stop reads and writes in restricted browsers.
  }
}

export { visitedSpacesKey };
