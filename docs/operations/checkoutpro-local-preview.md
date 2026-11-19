# Checkout Pro: base local de revisión

El runner de migraciones tiene un modo opt-in que conserva una base sintética
para pruebas manuales:

```bash
TMPDIR=/tmp GOCACHE=/tmp/espacigo-go-cache \
  bash scripts/test-migration-runner-postgres.sh aws-bootstrap-preview
```

El modo crea su propio contenedor PostgreSQL sin red, puertos publicados ni
volúmenes Docker; `PGDATA` vive en tmpfs y solo comparte un socket Unix y un
marcador efímero con el host. Conserva el contenedor únicamente cuando el test
termina correctamente y `pg_isready` confirma que sigue sano. El modo normal
del runner continúa limpiando su contenedor.

La fixture instala y verifica el ledger V1–V100 para pruebas. V99 se ejercita
con la autoridad E1 sintética del test; **esto no es un bootstrap operacional
E1 ni una vía para omitir sus preflights**. La base retenida solo sirve para
pruebas locales. No contiene usuarios, tokens OAuth ni pagos reales.

El acceso a la ruta comercial está sujeto a `COMMERCIAL_RENTAL_FLOW_ENABLED=1`
y a `commercialRentalFlowEnabled`: requiere `LOCAL_AUTH_PROTOTYPE=1`, API y
PostgreSQL en loopback, perfil `standard`, reloj de producción, sin
`APP_PROFILE` y con `LOCAL_BOOKING_TRIAL` apagado. Checkout Pro agrega
`MERCADOPAGO_CHECKOUT_ENABLED=1`, OAuth configurado en modo de prueba y el
archivo privado `MERCADOPAGO_WEBHOOK_SECRET_FILE`. Este montaje deliberadamente
no habilita pagos dentro de la composición de producción. Una entrega futura
requiere integrar quotes y checkout con la identidad de producción, autenticar
participantes en esas rutas, configurar la URL HTTPS/webhook y secretos del
entorno destino, revisar el diff y obtener autorización explícita antes de
habilitar cobros reales.

La salida del runner entrega el ID/nombre del contenedor, ruta del socket, base
y DSN sin contraseña para la app y para el migrador. La ruta del socket es
temporal y debe usarse literalmente; no se publica un puerto TCP. El ledger se
debe consultar con `cmd/dbmigrate -plan-only` antes de escribir. La fixture de
esta rama necesitó preparar permisos después del ledger V100:

- Ejecutar `db/bootstrap/migration-owner.sql` intacto contra la base exacta
  `espacigo_prod`; el script comprueba el checksum V100 y habilita `CREATE`
  únicamente al schema owner.
- Transferir puntualmente `version_terminos`, `media_variant` y
  `media_variant_intent` al schema owner, conservando los grants explícitos de
  `espacigo_runtime`. En esta fixture estaban vacías y pertenecían a `postgres`.
- Conceder `REFERENCES(id)` al schema owner sobre `usuario` y `espacio`, que
  siguen siendo tablas históricas. OAuth y checkout necesitan esas referencias
  para sus claves foráneas; no requieren otros privilegios sobre esas tablas.

Estas son preparaciones del fixture aislado, no migraciones de aplicación. En
cualquier futuro destino, comprueba los permisos FK y ownership mediante el
readiness aprobado para ese entorno; nunca ejecutes esta preparación manual
sobre una base compartida o productiva.

Las nuevas migraciones se aplican con `cmd/dbmigrate` y la identidad normal:

- `DATABASE_CONNECTION_MODE=direct`
- `DATABASE_URL` con usuario `espacigo_migrator_login` y el socket indicado
- `DATABASE_SET_ROLE=espacigo_schema_owner`

No edites el ledger ni ejecutes SQL de la aplicación como superusuario para
simular una migración. Prepara la fixture solo después de verificar el
contenedor, la base, el ledger, los dueños y los grants; deja evidencia de la
preparación como estado sintético de pruebas.

Para retirar únicamente la fixture retenida, usa el nombre exacto que imprimió
el runner y verifica su desaparición. No elimines contenedores compartidos de
PostgreSQL, AWS Emulator, Mailpit ni la demo de arriendo.

El AWS Emulator local es un emulador de APIs AWS; no proporciona un motor RDS
PostgreSQL. Esta preview usa el contenedor PostgreSQL aislado del runner.

## Preview integrado actual

En la preview de esta rama, Next escucha en `127.0.0.1:3100`, Go en
`127.0.0.1:8180` y la demo simulada de arriendo permanece separada en
`127.0.0.1:8181`. El BFF de Next debe usar
`API_BACKEND_BASE_URL=http://127.0.0.1:8180/api/v1`; su origen público y el
`FRONTEND_PUBLIC_ORIGIN` de Go deben coincidir. La API usa el socket temporal
emitido por `aws-bootstrap-preview` y `MERCADOPAGO_CHECKOUT_ENABLED=0`. No se
configuran credenciales de Mercado Pago ni se hacen llamadas al proveedor.

La fixture de V1–V106 no crea una cuenta local ni publica un espacio. El test
integrado `TestCheckoutPreferenceSignedWebhookAndNoticesPostgres` inserta actores,
conexión ficticia, cotización y reserva aleatorios, ejercita Preference/webhook
con gateway y mailer falsos, y los elimina al terminar. La lectura de la
fixture después de la prueba debe confirmar cero filas con
`@checkout-preview.invalid` y cero espacios `Checkout webhook fixture`.
No existe hoy un comando de seed persistente para navegar checkout comercial.

Sin una sesión local, la UI muestra la puerta de autenticación y el BFF
`GET /api/backend/host/mercadopago` responde `401 unauthenticated`; es el
comportamiento esperado y no indica que falte la ruta OAuth. Para una sesión
manual hacen falta una cuenta anfitriona de pruebas autenticada, un espacio
publicado y una cotización válida en esta misma base. El usuario debe preparar
esos datos mediante el flujo local autorizado o una fixture dedicada revisada;
no reutilices usuarios de otra base.

Para iniciar la autorización real de prueba también hacen falta, aportados
manualmente, el cliente OAuth de prueba, su URL de retorno HTTPS coincidente con
Next/BFF, y el archivo privado de firma webhook. Solo después de comprobar esos
valores se puede habilitar `MERCADOPAGO_CHECKOUT_ENABLED=1`; sin ellos, mantén el
checkout apagado. El test integrado es la comprobación actual de la ruta
comercial; no simula una cuenta real ni sustituye el smoke manual del proveedor.
