BEGIN;
INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado)
VALUES('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','a@example.test','a@example.test','fixture','activo'),
('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','b@example.test','b@example.test','fixture','activo');
INSERT INTO public.rol_usuario(usuario_id,rol) VALUES('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','arrendatario');
INSERT INTO public.espacio(id,propietario_id,categoria_codigo,titulo,descripcion,superficie_m2,capacidad_maxima,reglas_uso,modalidad_tarifa,precio_base_clp,direccion,zona_horaria)
VALUES('10000000-0000-4000-8000-000000000001','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','oficina','Test A','A',30,2,'Uso','hora',12000,'Calle A','America/Santiago'),
('10000000-0000-4000-8000-000000000002','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','oficina','Test B','B',30,2,'Uso','hora',12000,'Calle B','America/Santiago'),
('10000000-0000-4000-8000-000000000003','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','oficina','Test C','C',30,2,'Uso','hora',12000,'Calle C','America/Santiago');
SET ROLE espacigo_runtime;
PREPARE policy_status(uuid,uuid,text,text) AS WITH current AS (SELECT id FROM public.version_terminos WHERE id=$2 AND codigo=$3 AND tipo='terminos'
	 AND hash_sha256=$4 AND publicada_en<=now())
	 SELECT EXISTS(SELECT 1 FROM public.aceptacion_terminos a JOIN current c ON c.id=a.version_id WHERE a.usuario_id=$1),
	        EXISTS(SELECT 1 FROM current);
PREPARE policy_accept(uuid,uuid,text,text) AS WITH current AS (SELECT id FROM public.version_terminos WHERE id=$2 AND codigo=$3 AND tipo='terminos'
	 AND hash_sha256=$4 AND publicada_en<=now()), recorded AS (
		  INSERT INTO public.aceptacion_terminos(id,usuario_id,version_id,canal)
		  SELECT gen_random_uuid(),$1,id,'web' FROM current
		  ON CONFLICT (usuario_id,version_id) DO NOTHING
		) SELECT EXISTS(SELECT 1 FROM current);
EXECUTE policy_status('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','00000000-0000-4000-8000-000000000103','evaluacion-terminos-20261126-v2','d952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57');
EXECUTE policy_accept('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','00000000-0000-4000-8000-000000000103','evaluacion-terminos-20261126-v2','d952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57');
EXECUTE policy_accept('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','00000000-0000-4000-8000-000000000103','evaluacion-terminos-20261126-v2','d952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57');
EXECUTE policy_status('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','00000000-0000-4000-8000-000000000103','evaluacion-terminos-20261126-v2','d952a43b260febe5470eb9af348dddf7f1b2e2c744d18e408c814bf82cc76b57');
INSERT INTO public.espacio_publicacion_historial_local(espacio_id,actor_id,estado_anterior,estado_nuevo,ocurrida_en,correlacion_id)
SELECT id,propietario_id,'borrador','activa',now(),'sql-fixture' FROM public.espacio WHERE id::text LIKE '10000000-%';
UPDATE public.espacio SET estado='activa' WHERE id::text LIKE '10000000-%';
SELECT public.kyb_record_publication_document_request(id,propietario_id) FROM public.espacio WHERE id::text LIKE '10000000-%';
SELECT public.kyb_record_publication_document_request(id,propietario_id) FROM public.espacio WHERE id::text LIKE '10000000-%';
DO $$
DECLARE
    a uuid := '10000000-0000-4000-8000-000000000001';
    b uuid := '10000000-0000-4000-8000-000000000002';
    c uuid := '10000000-0000-4000-8000-000000000003';
    u uuid := 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
    reviewer uuid := 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
    due timestamptz;
BEGIN
    IF (SELECT count(*) FROM public.aceptacion_terminos WHERE usuario_id=u AND version_id='00000000-0000-4000-8000-000000000103')<>1 THEN RAISE EXCEPTION 'Policy acceptance must be idempotent'; END IF;
    SELECT grace_due_at INTO due FROM public.anfitrion_cumplimiento_personal WHERE usuario_id=u;
    IF due IS NULL OR (SELECT count(*) FROM public.espacio_solicitud_documental WHERE espacio_id IN (a,b) AND vence_en=due AND titular_usuario_id=u AND actor_usuario_id=u AND documentos_titular=ARRAY['sii_start_activities'])<>2 THEN
        RAISE EXCEPTION 'Document requests did not retain shared deadline and correct owner';
    END IF;
    IF has_table_privilege(current_user,'public.espacio_solicitud_documental','UPDATE') THEN RAISE EXCEPTION 'Audit writable'; END IF;
    IF (SELECT can_book FROM public.kyb_space_eligibility(a,due)) OR (SELECT can_publish FROM public.kyb_space_eligibility(b,due)) THEN RAISE EXCEPTION 'Overdue holder retained access'; END IF;
    IF NOT EXISTS(SELECT 1 FROM public.usuario WHERE id=u AND estado='activo')
        OR NOT EXISTS(SELECT 1 FROM public.rol_usuario WHERE usuario_id=u AND rol='arrendatario')
        OR NOT (SELECT can_book FROM public.kyb_space_eligibility(c,now())) THEN RAISE EXCEPTION 'Tenant access lost'; END IF;
    UPDATE public.anfitrion_cumplimiento_personal SET compliance_status='compliant',sii_status='APPROVED' WHERE usuario_id=u;
    INSERT INTO public.space_kyb_verification(espacio_id,host_mode,owner_usuario_id,document_storage_ref,mime_type,size_bytes,content_sha256,status,submitted_by,reviewer_id,reviewed_at)
    VALUES(a,'personal',u,gen_random_uuid(),'application/pdf',100,decode(repeat('ab',32),'hex'),'APPROVED',u,reviewer,now());
    IF NOT (SELECT can_book FROM public.kyb_space_eligibility(a,due)) OR (SELECT can_book FROM public.kyb_space_eligibility(b,due)) THEN
        RAISE EXCEPTION 'Property approval not independent'; END IF;
    INSERT INTO public.space_kyb_verification(espacio_id,host_mode,owner_usuario_id,document_storage_ref,mime_type,size_bytes,content_sha256,status,submitted_by,submitted_at)
    VALUES(b,'personal',u,gen_random_uuid(),'application/pdf',100,decode(repeat('ab',32),'hex'),'PENDING',u,due+interval '1 second');
    IF (SELECT can_book FROM public.kyb_space_eligibility(b,due+interval '2 seconds')) THEN RAISE EXCEPTION 'Late upload reopened grace'; END IF;
END;
$$;
ROLLBACK;
