# Mapas y búsqueda de direcciones

`LocationMap` carga Leaflet en el navegador. La selección manual y los campos de coordenadas siguen disponibles cuando falla la cartografía.

Configura antes de compilar Next:

- `NEXT_PUBLIC_MAP_TILE_URL`: plantilla XYZ del proveedor, con `{z}`, `{x}` y `{y}`.
- `NEXT_PUBLIC_MAP_ATTRIBUTION`: atribución exigida por ese proveedor. El componente requiere ambas variables para descargar tiles.
- `NEXT_PUBLIC_GEOCODING_URL`: endpoint explícito que acepta `?q=...` y devuelve GeoJSON `FeatureCollection`, geometrías `Point` con coordenadas `[longitud, latitud]` y `properties.label` opcional. La búsqueda ocurre solo al pulsar el botón; no usa autocompletado ni selecciona Nominatim público por defecto.

Las variables son públicas: no pongas secretos. Verifica autorización, condiciones, atribución, límites y CORS del proveedor elegido. Sin geocodificador, la interfaz explica el fallo y permite fijar el punto manualmente. Un candidato no confirma por sí mismo dirección, comuna o pin: el anfitrión revisa y confirma explícitamente. Cambiar dirección, códigos o coordenadas retira el consentimiento anterior.

Las coordenadas usan WGS84, latitud entre -90 y 90 y longitud entre -180 y 180. El catálogo consume únicamente la ubicación confirmada; los datos históricos sin pin permanecen en cuadrícula.
