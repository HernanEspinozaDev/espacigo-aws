-- Lower minimum space/rate base price from > 5000 CLP to >= 1000 CLP.
DO $migration$
DECLARE
    tbl text;
    col text;
    con_name text;
BEGIN
    IF session_user <> current_user AND current_user <> 'espacigo_schema_owner' THEN
        RAISE EXCEPTION 'candidate requires session/effective role equality before owner switching';
    END IF;
    IF NOT pg_catalog.pg_has_role(session_user, 'espacigo_schema_owner', 'SET') THEN
        RAISE EXCEPTION 'session user cannot SET ROLE espacigo_schema_owner';
    END IF;

    SET LOCAL ROLE espacigo_schema_owner;

    FOR tbl, col IN VALUES
        ('espacio', 'precio_base_clp'),
        ('tarifa_espacio', 'precio_base_clp'),
        ('simulacion_precio_privada', 'precio_unitario_clp'),
        ('cotizacion_reserva_ensayo', 'precio_unitario_clp'),
        ('arriendo_temprano', 'precio_unitario_clp')
    LOOP
        IF to_regclass(format('public.%I', tbl)) IS NULL THEN
            CONTINUE;
        END IF;
        FOR con_name IN
            SELECT c.conname
            FROM pg_catalog.pg_constraint c
            JOIN pg_catalog.pg_class r ON r.oid = c.conrelid
            JOIN pg_catalog.pg_namespace n ON n.oid = r.relnamespace
            WHERE n.nspname = 'public'
              AND r.relname = tbl
              AND c.contype = 'c'
              AND (
                  pg_catalog.pg_get_constraintdef(c.oid) LIKE '%5000%'
                  OR pg_catalog.pg_get_constraintdef(c.oid) LIKE '%5001%'
              )
        LOOP
            EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', tbl, con_name);
        END LOOP;
        EXECUTE format(
            'ALTER TABLE public.%I ADD CONSTRAINT %I CHECK (%I >= 1000)',
            tbl,
            tbl || '_' || col || '_min_1000_ck',
            col
        );
    END LOOP;
END $migration$;
