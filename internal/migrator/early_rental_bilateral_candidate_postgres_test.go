package migrator

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The evidence suite calls this after installing its auxiliary candidate. It
// exercises schema DML, not an implemented HTTP/service rental lifecycle.
func assertEvidenceBilateralLifecycle(t *testing.T, ctx context.Context, admin, reader *pgx.Conn) {
	t.Helper()
	t.Run("schema-bilateral-lifecycle", func(t *testing.T) {
		f := newEarlyRentalFixture(t, ctx, admin)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		for _, step := range []evidenceLifecycleStep{
			{"identidad_simulada_confirmada", f.owner, "identidad", "pendiente_identidad", "pendiente_contrato"},
			{"identidad_simulada_confirmada", f.renter, "identidad", "pendiente_contrato", ""},
			{"contrato_aceptado", f.owner, "contrato", "pendiente_contrato", "pendiente_envio_pago"},
			{"contrato_aceptado", f.renter, "contrato", "pendiente_envio_pago", ""},
			{"pago_marcado_enviado", f.renter, "", "pendiente_recepcion_pago", ""},
			{"pago_marcado_recibido", f.owner, "", "confirmado", ""},
		} {
			if step.prematureState != "" {
				before := earlyRentalCounts(t, ctx, reader)
				beforeEvents := evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID)
				beforeConfirmations := evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID)
				var beforeRevision int64
				if err := reader.QueryRow(ctx, `SELECT revision FROM public.arriendo_temprano WHERE id=$1`, f.offerID).Scan(&beforeRevision); err != nil {
					t.Fatal(err)
				}
				invalid := step
				invalid.nextState = step.prematureState
				err := applyEvidenceLifecycleStep(ctx, admin, f, invalid)
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
					t.Fatalf("premature bilateral phase accepted: %v", err)
				}
				var afterRevision int64
				if err := reader.QueryRow(ctx, `SELECT revision FROM public.arriendo_temprano WHERE id=$1`, f.offerID).Scan(&afterRevision); err != nil || afterRevision != beforeRevision || earlyRentalCounts(t, ctx, reader) != before || evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID) != beforeEvents || evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID) != beforeConfirmations {
					t.Fatalf("premature phase changed committed facts: %v", err)
				}
			}
			if err := applyEvidenceLifecycleStep(ctx, admin, f, step); err != nil {
				t.Fatalf("valid schema step %s: %v", step.kind, err)
			}
		}
		var valid bool
		if err := reader.QueryRow(ctx, `SELECT a.estado='confirmado' AND a.revision=(SELECT count(*) FROM public.arriendo_temprano_evento WHERE arriendo_id=a.id) AND o.tipo='reserva_temprana' AND o.activo AND o.expira_en IS NULL AND p.estado='recibido' AND p.recibido_en>=p.enviado_en AND (SELECT count(*) FROM public.arriendo_temprano_confirmacion WHERE arriendo_id=a.id)=4 FROM public.arriendo_temprano a JOIN public.ocupacion o ON o.id=a.ocupacion_id JOIN public.arriendo_temprano_pago p ON p.arriendo_id=a.id WHERE a.id=$1`, f.offerID).Scan(&valid); err != nil || !valid {
			t.Fatalf("schema lifecycle independent readback: %t %v", valid, err)
		}
	})
}

type evidenceLifecycleStep struct {
	kind, actor, class, nextState, prematureState string
}

func applyEvidenceLifecycleStep(ctx context.Context, db *pgx.Conn, f *earlyRentalFixture, step evidenceLifecycleStep) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var revision int64
	var prior string
	if err := tx.QueryRow(ctx, `SELECT revision+1,estado FROM public.arriendo_temprano WHERE id=$1 FOR UPDATE`, f.offerID).Scan(&revision, &prior); err != nil {
		return err
	}
	var confirmation, payment any
	if step.class != "" {
		var id string
		if step.class == "identidad" {
			err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase) VALUES($1,$2,'identidad') RETURNING id`, f.offerID, step.actor).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest) SELECT id,$2,'contrato',contrato_version,contrato_digest FROM public.arriendo_temprano WHERE id=$1 RETURNING id`, f.offerID, step.actor).Scan(&id)
		}
		if err != nil {
			return err
		}
		confirmation = id
	}
	if step.kind == "pago_marcado_enviado" {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado) SELECT id,arrendatario_id,anfitrion_id,subtotal_clp,moneda,'pendiente' FROM public.arriendo_temprano WHERE id=$1 RETURNING pago_id`, f.offerID).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano_pago SET estado='enviado',enviado_en=clock_timestamp() WHERE arriendo_id=$1`, f.offerID); err != nil {
			return err
		}
		payment = id
	}
	if step.kind == "pago_marcado_recibido" {
		var id string
		if err := tx.QueryRow(ctx, `UPDATE public.arriendo_temprano_pago SET estado='recibido',recibido_en=clock_timestamp() WHERE arriendo_id=$1 RETURNING pago_id`, f.offerID).Scan(&id); err != nil {
			return err
		}
		payment = id
		if _, err := tx.Exec(ctx, `UPDATE public.ocupacion SET tipo='reserva_temprana',expira_en=NULL WHERE id=$1`, f.occupancyID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$2,revision=$3,actualizada_en=clock_timestamp() WHERE id=$1`, f.offerID, step.nextState, revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,confirmacion_id,pago_id) VALUES($1,$2,$2,$3,$4,$5,$6,$7,$8)`, f.offerID, revision, step.kind, step.actor, prior, step.nextState, confirmation, payment); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
