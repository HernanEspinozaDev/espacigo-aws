BEGIN;
DO $test$
DECLARE
    owner_id uuid := gen_random_uuid();
    rep_id uuid := gen_random_uuid();
    renter_id uuid := gen_random_uuid();
    org_id uuid := gen_random_uuid();
    fixture_space uuid := gen_random_uuid();
    second_space uuid := gen_random_uuid();
    company_deadline timestamptz;
    identity_revision uuid := gen_random_uuid();
    quote_id uuid := gen_random_uuid();
    template_id uuid := gen_random_uuid();
    proposal_id uuid := gen_random_uuid();
    agreement_id uuid := gen_random_uuid();
    session_id uuid := gen_random_uuid();
    starts_at timestamptz := now() + interval '1 day';
    ends_at timestamptz := starts_at + interval '1 hour';
    document_hash bytea := sha256('fixture');
    clauses text := '[{"key":"uso","title":"Uso","body":"Fixture"}]';
    terms jsonb;
    error_message text;
BEGIN
    INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado)
    VALUES (owner_id,'owner-company@example.test','owner-company@example.test','fixture','activo'),
           (rep_id,'representative@example.test','representative@example.test','fixture','activo'),
           (renter_id,'renter-company@example.test','renter-company@example.test','fixture','activo');
    INSERT INTO public.organizacion(id,usuario_id,created_by_usuario_id,legal_name)
    VALUES (org_id,owner_id,owner_id,'Empresa sintética SpA');
    INSERT INTO public.organizacion_miembro(organizacion_id,usuario_id,rol,estado,joined_at)
    VALUES (org_id,owner_id,'representante_legal','activo',now()), (org_id,rep_id,'representante_legal','activo',now());
    INSERT INTO public.account_identity_profile_revision(id,account_id,key_version,encrypted_data,masked_rut,masked_phone,saved_at)
    VALUES (identity_revision,rep_id,'fixture',decode(repeat('ab',40),'hex'),'•••56-0','•••1234',now());
    INSERT INTO public.account_identity_profile(account_id,username,current_revision)
    VALUES (rep_id,'verified_rep_fixture',identity_revision);
    INSERT INTO public.account_identity_verification(id,account_id,profile_revision,state,started_at,updated_at,verified_at,verified_until)
    VALUES (gen_random_uuid(),rep_id,identity_revision,'verified',now(),now(),now(),now()+interval '1 day');
    INSERT INTO public.sesion(id,usuario_id,token_hash,expira_en)
    VALUES (session_id,rep_id,repeat('a',64),now()+interval '1 hour');
    INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,zona_horaria,es_b2b,organizacion_id,modo_anfitrion)
    VALUES (fixture_space,owner_id,'oficina','Empresa','Fixture',30,2,'Regla','hora',10000,'Calle fixture','America/Santiago',true,org_id,'empresa');
    INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,zona_horaria,es_b2b,organizacion_id,modo_anfitrion)
    VALUES (second_space,owner_id,'oficina','Segundo espacio','Fixture',30,2,'Regla','hora',10000,'Otra calle','America/Santiago',true,org_id,'empresa');
    UPDATE public.espacio SET estado='activa' WHERE id=fixture_space;
    SELECT grace_due_at INTO company_deadline FROM public.organizacion WHERE id=org_id;
    UPDATE public.espacio SET estado='activa' WHERE id=second_space;
    IF company_deadline IS NULL OR (SELECT grace_due_at FROM public.organizacion WHERE id=org_id) IS DISTINCT FROM company_deadline THEN
        RAISE EXCEPTION 'A second company member reset the first publication deadline';
    END IF;
    INSERT INTO public.espacio_contrato_borrador(espacio_id,revision,secciones)
    VALUES (fixture_space,1,clauses::jsonb);
    INSERT INTO public.espacio_contrato_version(id,espacio_id,version,revision_origen,texto_canonico,secciones,sha256,publicada_por)
    VALUES (template_id,fixture_space,1,1,clauses,clauses::jsonb,sha256(convert_to(clauses,'UTF8')),owner_id);
    INSERT INTO public.cotizacion_arriendo_comercial(id,espacio_id,propietario_id,arrendatario_id,titulo_espacio,tarifa_version,modalidad,precio_unitario_clp,moneda,unidades,subtotal_clp,inicio,termino,zona_horaria,creada_en,vence_en)
    VALUES (quote_id,fixture_space,owner_id,renter_id,'Empresa',1,'hora',10000,'CLP',1,10000,starts_at,ends_at,'America/Santiago',now(),now()+interval '6 minutes');
    terms := jsonb_build_object('space_id',fixture_space,'quote_id',quote_id,'template_id',template_id,
        'subtotal_clp',10000,'currency','CLP','time_zone','America/Santiago','start_at',starts_at,'end_at',ends_at,
        'space_title','Empresa','template_version',1,'sections',clauses::jsonb,
        'location',jsonb_build_object('address','Calle fixture','latitude',-33.4,'longitude',-70.6,'revision',1,'region_code','13','commune_code','13101'),
        'rate_unit','hora','rate_version',1,'unit_price_clp',10000,'units',1,
        'cancellation_policy',jsonb_build_object('version','flexible_v1','text','Fixture'),
        'company_representation', jsonb_build_object('organization_id',org_id,'legal_name','Empresa sintética SpA',
            'rut_masked','•••56-0','rut_verified',true,'representative_user_id',rep_id,'representative_name','Representante',
            'representative_rut_masked','•••78-5','representative_rut_verified',true));
    INSERT INTO public.contrato_propuesta_comercial(id,cotizacion_id,espacio_id,propietario_id,arrendatario_id,version,plantilla_version_id,inicio,termino,zona_horaria,subtotal_clp,moneda,terminos,terminos_canonicos,terminos_sha256,documento_sha256,identidades_cifradas,clave_version,es_b2b,organizacion_id,empresa_razon_social_snapshot,empresa_rut_masked_snapshot,empresa_rut_verificada,representante_legal_id,representante_nombre_snapshot,representante_rut_masked_snapshot,representante_rut_verificada)
    VALUES (proposal_id,quote_id,fixture_space,owner_id,renter_id,1,template_id,starts_at,ends_at,'America/Santiago',10000,'CLP',terms,terms::text,sha256(convert_to(terms::text,'UTF8')),document_hash,decode(repeat('ab',40),'hex'),'fixture',true,org_id,'Empresa sintética SpA','•••56-0',true,rep_id,'Representante','•••78-5',true);
    INSERT INTO public.contrato_solicitud_comercial(id,propuesta_id,cotizacion_id,espacio_id,propietario_id,arrendatario_id,propuesta_version,documento_sha256,idempotency_key,fingerprint,estado,vence_en,creada_en,actualizada_en,es_b2b,organizacion_id,empresa_razon_social_snapshot,empresa_rut_masked_snapshot,empresa_rut_verificada,representante_legal_id,representante_nombre_snapshot,representante_rut_masked_snapshot,representante_rut_verificada)
    VALUES (agreement_id,proposal_id,quote_id,fixture_space,owner_id,renter_id,1,document_hash,'fixture-agreement',document_hash,'pending_landlord',starts_at,now(),now(),true,org_id,'Empresa sintética SpA','•••56-0',true,rep_id,'Representante','•••78-5',true);
    BEGIN
        INSERT INTO public.contrato_firma_comercial(id,solicitud_id,actor_id,sesion_id,rol,propuesta_version,documento_sha256,consentimiento,consentimiento_version,firmada_en)
        VALUES (gen_random_uuid(),agreement_id,owner_id,session_id,'arrendador',1,document_hash,true,'commercial-agreement-v1',now());
        RAISE EXCEPTION 'Company signature accepted a different member';
    EXCEPTION WHEN check_violation THEN
        GET STACKED DIAGNOSTICS error_message = MESSAGE_TEXT;
        IF error_message <> 'commercial signature binding mismatch' THEN RAISE; END IF;
    END;
    INSERT INTO public.contrato_firma_comercial(id,solicitud_id,actor_id,sesion_id,rol,propuesta_version,documento_sha256,consentimiento,consentimiento_version,firmada_en)
    VALUES (gen_random_uuid(),agreement_id,rep_id,session_id,'arrendador',1,document_hash,true,'commercial-agreement-v1',now());
    IF NOT EXISTS (SELECT 1 FROM public.contrato_firma_comercial f WHERE f.solicitud_id=agreement_id AND f.actor_id=rep_id) THEN
        RAISE EXCEPTION 'Missing valid company representative signature';
    END IF;
END;
$test$;
ROLLBACK;
