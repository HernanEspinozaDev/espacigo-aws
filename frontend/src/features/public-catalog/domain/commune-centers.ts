import centers from "./commune-centers.json";

// Interior reference points derived from the DPA 2023 polygons of IDE Chile,
// SUBDERE, IGM, DIFROL and INE (Creative Commons Attribution).
// https://datos.gob.cl/dataset/categoria-geoespacial-limites-y-fronteras/resource/0cc095c1-80f0-4d31-9aad-4dcffce3d957
const references: Readonly<Partial<Record<string, readonly number[]>>> =
  centers;

export function getCatalogCommuneCenter(code: string) {
  if (!/^\d{5}$/.test(code)) return undefined;
  const point = references[code];
  return point ? { latitude: point[0]!, longitude: point[1]! } : undefined;
}
