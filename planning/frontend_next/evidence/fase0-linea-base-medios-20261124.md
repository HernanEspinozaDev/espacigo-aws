# Fase 0 · Línea base y P0 de medios (24 de noviembre de 2026)

## 1. Diagnóstico P0 de medios (`0.2`)

Comprobación ejecutada contra `http://127.0.0.1:8080/api/v1/public/spaces?page_size=10`, `http://127.0.0.1:3000/api/backend/public/spaces/...`, PostgreSQL `espacigo_aws_local` y `aws-emulator` (`s3://espacigo-media-local`).

- **Inventario público retornado por Go**: 5 espacios:
  1. `7aec3e2d-8ac8-441c-a906-18020289b0d0` · `"QA Tickets horarios e0a83204877f"` (`oficina`) · `cover_image_id: null`, `images: []` (sin foto).
  2. `f0e100b6-5732-4a64-aa68-6c2aeea7c5b1` · `"sala para kinesiología"` (`sala_multiproposito`) · `cover_image_id: 31ffc0ff-34d6-4652-ab1a-5059a3a86a67`.
  3. `9417a1ee-4e87-4a4a-9bab-3b2a11b036d9` · `"casa cartagena"` (`inmuebles`) · `cover_image_id: 8e1b42a1-8244-43fe-a8f7-cfe7bab3e547`.
  4. `47551091-2de6-4816-a7da-bc71d2bbcc44` · `"Departamento Vista Mar La Serena"` (`inmuebles`) · `cover_image_id: 6f154e45-bcfb-4800-b1ce-9b67f76da698`.
  5. `d3e5e765-61c1-4f47-b03d-be40c064fa23` · `"Loft Panorámico Providencia"` (`inmuebles`) · `cover_image_id: 6ac15f68-125d-4875-80f6-3a7c9c40ad31`.
- **Estado en PostgreSQL (`space_media` + `media_variant`)**: Los 4 espacios con portada tienen filas `ready` en `media_variant` (`display` y `thumbnail`) apuntando a `provider = 's3'`, `bucket_name = 'espacigo-media-local'`.
- **Causa raíz de los 404**: El bucket `espacigo-media-local` en `aws-emulator` fue recreado el `2026-11-24 05:15:37` y está vacío (`0` objetos). Tanto `/api/v1/public/spaces/{id}/images/{img}/variants/thumbnail` como el fallback original `/api/v1/public/spaces/{id}/images/{img}` devuelven `HTTP 404` (`code: not_found`) desde Go y desde el BFF Next.js, haciendo que `PublicMediaImage` caiga al placeholder `/espacigo-mark.svg`.
- **Acción / asignación**: No es un bug de `PublicMediaImage` ni del BFF; requiere poblar/resincronizar las imágenes de los espacios semilla en `s3://espacigo-media-local` (módulo de bootstrap/entorno local).

## 2. Datos QA y duplicación en portada (`0.3` / `0.4`)

- **Publicación QA en catálogo**: `7aec3e2d-8ac8-441c-a906-18020289b0d0` (`"QA Tickets horarios e0a83204877f"`) está publicada en la base local sin etiqueta de exclusión.
- **Duplicación en portada**: `(marketplace)/page.tsx` consulta `publicCatalog.spaces({ page_size: 6 })` y `publicCatalog.spaces({ category_code: "inmuebles", page_size: 3 })` sin deduplicar por `space_id`, provocando que los 3 inmuebles aparezcan dos veces (8 tarjetas para 5 espacios distintos). Esto se corrige directamente en Fase 2.3.
