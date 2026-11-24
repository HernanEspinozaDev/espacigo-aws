# Mapas y ubicación de espacios

`LocationMap` carga MapLibre GL JS en el navegador. El mismo componente sirve el formulario privado, la ficha pública y la vista separada del catálogo. El anfitrión puede colocar el pin con un toque y ajustarlo arrastrándolo.

## Cartografía del navegador

Configura antes de compilar Next:

- `NEXT_PUBLIC_AMAZON_LOCATION_REGION`: región AWS asignada al proyecto y compatible con Amazon Location Maps V2.
- `NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY`: clave web de Amazon Location restringida a la cartografía y a los dominios autorizados. El navegador puede ver esta clave. Usa una distinta por entorno.
- `NEXT_PUBLIC_MAP_STYLE_URL`: descriptor de estilo MapLibre alternativo para pruebas o cartografía autorizada. Tiene precedencia sobre Amazon Location y debe incluir la atribución de sus fuentes.

Para el piloto de Contratos se crearon en `us-east-2` dos claves distintas con expiración el 21 de febrero de 2027. `espacigo-map-web-test` permite solo las operaciones Maps y solo desde localhost y `prueba-mercadopago.testingpage.store`. `espacigo-places-pilot-server` permite `geo-places:Geocode` y `geo-places:ReverseGeocode`. Sus valores están en `.local/secrets/amazon-location/`, ignorado por Git y con permisos `0600`. No los copies a documentación, historial de terminal ni variables `NEXT_PUBLIC_` salvo la clave de mapas, que está limitada por dominio.

El archivo privado `pilot.env` carga ambas claves y la región para los procesos locales. `scripts/local-dev.sh` y `scripts/reiniciar-local-fondo.sh` lo cargan automáticamente si existe. No contiene ni sustituye las credenciales del emulador. En Next, solo la clave de Maps se publica en el bundle del navegador y sigue limitada por el referer.

Maps V2 no necesita crear un recurso `AWS::Location::Map`; usa el proveedor `geo-maps` por defecto. El comando `aws location create-key` es útil para bootstrap porque devuelve el valor de la clave que debe guardarse localmente. CloudFormation sí admite `AWS::Location::APIKey`, aunque no expone el valor de clave como atributo de salida; se recupera con `aws location describe-key`. Para el backend de producción, la plantilla `infra/cloudformation/app-and-cicd.yaml` prepara `geo-places:Geocode` en el rol de ejecución y habilita la variable de función. Esa modificación de infraestructura queda pendiente de promoción y despliegue aprobados; las claves del piloto local no se reutilizan en producción.

## Búsqueda y validación en la API

- `AWS_PLACES_GEOCODING_ENABLED=true` habilita las consultas de dirección desde Go. En producción, el SDK usa la cadena de credenciales de AWS del proceso en la región asignada mediante `AWS_REGION`. Para un piloto local, configura `AWS_PLACES_PILOT_REGION` con la región asignada y una clave de servidor `AWS_PLACES_PILOT_API_KEY` restringida a las operaciones de Places que use el piloto. Esa clave es distinta de la de cartografía y queda fuera de Git y de las variables públicas. El cliente del piloto opera de forma independiente del emulador.
- Go resuelve los códigos de región y comuna desde PostgreSQL. Envía la dirección a Amazon Location Places con filtro de Chile e `IntendedUse: Storage`, porque el anfitrión puede guardar un candidato. Las consultas del formulario tienen un límite por cuenta en cada instancia.
- `ESPACIGO_COMMUNE_BOUNDARIES_FILE` apunta a un archivo local. En Contratos se preparó `.local/geo/dpa_2023_communes.bin.gz` con los 345 códigos CUT del catálogo. Go lo carga al arrancar. También admite GeoJSON WGS84 de polígonos o multipolígonos con `properties.comuna_code` como texto de cinco dígitos.
- Con el archivo cargado, la selección de comuna centra el mapa y Go valida el pin antes de confirmar la ubicación pública. Si el punto está fuera de la comuna o esta carece de geometría, se puede corregir o guardar como borrador. Sin archivo, se conserva el comportamiento anterior de confirmación.

En el alta y la edición, el anfitrión elige primero el punto en el mapa. La API obtiene la región y comuna a partir de los límites oficiales y pide a Places una dirección estimada con `IntendedUse: Storage`. El anfitrión puede corregir esa dirección y conservar el punto. Mover el pin o editar la dirección requiere revisar la confirmación pública antes de guardar. Si solo se identifica la comuna, el anfitrión escribe la dirección. Si tampoco se identifica la comuna, puede seleccionarla manualmente, con la validación habitual al confirmar. Si falla el mapa, se pueden indicar las coordenadas manualmente.

Para sugerir la dirección, la clave de Places del piloto y el rol de producción requieren `geo-places:ReverseGeocode`. Los límites comunales locales permiten completar región y comuna aunque Places no devuelva una dirección. La migración V135 sigue siendo necesaria para guardar los metadatos del pin, que pueden estar vacíos en espacios anteriores.

## Límites comunales

Fuente para validación del servidor: División Política Administrativa de Chile 2023, Grupo de Trabajo DPA de IDE Chile, con SUBDERE, IGM, DIFROL e INE. La ficha de Datos Abiertos consigna licencia Creative Commons Attribution. Usa la cartografía base del proveedor configurado para la interfaz pública. [Descarga oficial de DPA 2023](https://www.geoportal.cl/geoportal/catalog/download/912598ad-ac92-35f6-8045-098f214bd9c2) · [Ficha y licencia](https://datos.gob.cl/dataset/categoria-geoespacial-limites-y-fronteras/resource/0cc095c1-80f0-4d31-9aad-4dcffce3d957)

Desde la raíz de Contratos, convierte el ZIP oficial y suministra el resultado al proceso de la API:

```bash
python3 scripts/build-commune-boundaries.py /ruta/al/archivo-dpa-2023.zip .local/geo/dpa_2023_communes.bin.gz
export ESPACIGO_COMMUNE_BOUNDARIES_FILE="$PWD/.local/geo/dpa_2023_communes.bin.gz"
```

El formato binario comprimido conserva las coordenadas del archivo fuente. En una prueba aislada, cargó 345 comunas en aproximadamente 1,6 segundos con 140 MiB de RSS máximo. Verifica el consumo junto con la API y Next al habilitarlo en otro entorno.

## Prueba del catálogo en localhost

El escenario usa un descriptor vectorial mínimo y datos de prueba. La validación de calles y atribución del proveedor requiere un descriptor real.

```bash
NEXT_PUBLIC_MAP_STYLE_URL=/__catalog-map-style.json pnpm build
pnpm test:e2e:catalog-map
```

Explorar comparte la ubicación, el radio y los filtros entre la cuadrícula y el mapa. Los puntos de referencia comunales se calcularon a partir de los polígonos DPA 2023. Sirven como origen aproximado para una búsqueda por radio. Si una comuna carece de geometría, se puede seleccionar el origen en el mapa. El mapa agrupa marcadores próximos y muestra hasta 480 espacios con ubicación confirmada por búsqueda.

El corte de Explorar sobre `fa5b16f` pasó las pruebas dirigidas de Go, 12 pruebas del adaptador y servicio del catálogo, y 9 escenarios de Chromium con cartografía y datos de prueba. La suite incluye escritorio, tablet, móvil, GPS permitido y rechazado, selección de comuna y pin, y agrupamiento.

## Comprobaciones en Contratos

### Fase 0. Ubicación del anfitrión

El mapa gobierna latitud, longitud, región y comuna. Elegir un resultado, usar GPS, tocar el mapa o arrastrar el pin inicia la misma consulta. La dirección queda editable. Si la resolución oficial no identifica comuna o no está disponible, el formulario bloquea el avance o el guardado.

La creación guarda la ubicación y el borrador en la misma transacción. Cambiar la ubicación requiere `PUT /spaces/{id}/location` con el ETag vigente. Go exige geometría comunal y catálogo activo tanto en alta como en edición. El script de API local usa `.local/geo/dpa_2023_communes.bin.gz` cuando está disponible y no se indica otro archivo. Antes de una futura promoción se debe preparar el archivo de límites y su configuración en el entorno destino.

Ejecuta `./scripts/test-phase0-location-postgres.sh` para repetir la prueba de depósito aislado. Usa una copia del esquema y catálogos de referencia locales, crea sus propios datos sintéticos y verifica el alta, el cambio de región, el conflicto por ETag y la publicación en ficha y Explorar.

Corte de trabajo sobre `fa5b16f`, con los cambios del mapa en el worktree local. Pasaron la compilación Next, cinco escenarios del catálogo en Chromium y el escenario del asistente que recupera un pin rechazado como borrador. También pasaron las pruebas Go de geocodificación, límites comunales, ubicación y catálogo. La migración V135 se ejecutó en PostgreSQL 18 aislado sobre una ubicación anterior y se comprobó la limpieza de metadatos tras una edición antigua.

El escenario de Chromium usa cartografía de prueba y respuestas controladas. La entrega de Amazon Maps y Places con las claves del proyecto se comprueba durante el piloto real.

En el corte que pone el mapa primero, sobre `fa5b16f`, pasaron la compilación local, TypeScript, el lint de los archivos modificados, la verificación del contrato OpenAPI, las pruebas Go dirigidas y los escenarios del asistente en Chromium de escritorio y con emulación de Pixel 7. Los escenarios de navegador usan un estilo mínimo y respuestas de la API controladas. El servidor local quedó activo con los límites comunales cargados. El 23 de noviembre de 2026 se añadió `geo-places:ReverseGeocode` a la clave del piloto con AWS CLI. Se verificó una respuesta HTTP 200 con una dirección para un punto público de Santiago.
