package migrator

import (
	"context"
	"errors"
	"fmt"
)

// These structural gates complement, not replace, the exact source/ledger,
// function identity/owner, raw ACL and effective privilege comparisons.
func guardE1SchemaCatalog(ctx context.Context, q queryRower) error {
	var valid bool
	err := q.QueryRow(ctx, `SELECT
 (SELECT count(*)=5 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r' AND c.relname=ANY(ARRAY['arriendo_temprano','arriendo_temprano_confirmacion','arriendo_temprano_pago','arriendo_temprano_evento','arriendo_temprano_idempotencia']))
 AND (SELECT count(*)=7 FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname LIKE 'arriendo_temprano%' AND p.prokind='f' AND p.pronargs=0 AND p.prorettype='pg_catalog.trigger'::pg_catalog.regtype AND NOT p.prosecdef)
 AND EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.ocupacion'::pg_catalog.regclass AND conname='ocupacion_no_solapa_excl' AND contype='x')
 AND (SELECT count(*)=2 FROM pg_catalog.pg_constraint WHERE ((conrelid='public.ocupacion'::pg_catalog.regclass AND conname='ocupacion_temprana_agregado_fk') OR (conrelid='public.arriendo_temprano'::pg_catalog.regclass AND conname='arriendo_temprano_ocupacion_fk')) AND contype='f' AND condeferrable AND condeferred AND pg_catalog.cardinality(conkey)=4)
 AND (SELECT count(*)=2 FROM pg_catalog.pg_constraint WHERE conrelid='public.arriendo_temprano_evento'::pg_catalog.regclass AND conname IN ('arriendo_temprano_evento_motivo_ck','arriendo_temprano_evento_secuencia_check') AND contype='c')
 AND (SELECT count(*)=9 FROM (VALUES
 ('arriendo_temprano','arriendo_temprano_immutable_update_trg','arriendo_temprano_immutable_update'),
 ('arriendo_temprano','arriendo_temprano_occupancy_consistency_trg','arriendo_temprano_occupancy_consistency'),
 ('ocupacion','ocupacion_arriendo_temprano_consistency_trg','arriendo_temprano_occupancy_consistency'),
 ('arriendo_temprano_confirmacion','arriendo_temprano_confirmacion_guard','arriendo_temprano_confirmacion_guard'),
 ('arriendo_temprano_pago','arriendo_temprano_pago_guard','arriendo_temprano_pago_guard'),
 ('arriendo_temprano_evento','arriendo_temprano_evento_guard','arriendo_temprano_evento_guard'),
 ('arriendo_temprano_evento','arriendo_temprano_evento_consistency','arriendo_temprano_evento_consistency'),
 ('arriendo_temprano','arriendo_temprano_evento_revision_consistency','arriendo_temprano_evento_consistency'),
 ('arriendo_temprano_idempotencia','arriendo_temprano_idempotencia_guard','arriendo_temprano_idempotencia_guard')
 ) e(rel,trg,fn) JOIN pg_catalog.pg_namespace n ON n.nspname='public' JOIN pg_catalog.pg_class c ON c.relnamespace=n.oid AND c.relname=e.rel JOIN pg_catalog.pg_trigger t ON t.tgrelid=c.oid AND t.tgname=e.trg AND NOT t.tgisinternal AND t.tgenabled='O' JOIN pg_catalog.pg_proc p ON p.oid=t.tgfoid AND p.pronamespace=n.oid AND p.proname=e.fn)
 `).Scan(&valid)
	if err != nil {
		return fmt.Errorf("E1 structural catalog readback: %w", err)
	}
	if !valid {
		return errors.New("E1 structural catalog differs from the reviewed schema; do not replay")
	}
	return nil
}
