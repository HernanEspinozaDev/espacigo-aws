package migrator

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
	"time"
)

func assertOperationalTerminalReasonRequired(t *testing.T, ctx context.Context, admin, reader *pgx.Conn) {
	t.Helper()
	t.Run("terminal-reason-required", func(t *testing.T) {
		f := newEarlyRentalFixture(t, ctx, admin)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		before := earlyRentalCounts(t, ctx, reader)
		err := applyOperationalTerminal(ctx, admin, f, f.renter, "oferta_cancelada", "cancelado", nil)
		requireTerminalConstraint(t, err, "arriendo_temprano_evento_motivo_ck")
		if after := earlyRentalCounts(t, ctx, reader); after != before {
			t.Fatalf("rejected write changed counts %v -> %v", before, after)
		}
		var state string
		var revision int
		var active bool
		if err := reader.QueryRow(ctx, `SELECT a.estado,a.revision,o.activo FROM public.arriendo_temprano a JOIN public.ocupacion o ON o.id=a.ocupacion_id WHERE a.id=$1`, f.offerID).Scan(&state, &revision, &active); err != nil || state != "pendiente_identidad" || revision != 1 || !active {
			t.Fatalf("failed terminal write partially persisted: %s %d %t %v", state, revision, active, err)
		}
	})
	t.Run("terminal-reasons-preserved", func(t *testing.T) {
		for _, role := range []string{"host", "renter"} {
			for _, kind := range []string{"oferta_rechazada", "oferta_cancelada"} {
				t.Run(role+"-"+kind, func(t *testing.T) {
					f := newEarlyRentalFixture(t, ctx, admin)
					if err := seedEvidenceOffer(ctx, admin, f); err != nil {
						t.Fatal(err)
					}
					actor := f.owner
					if role == "renter" {
						actor = f.renter
					}
					state := "rechazado"
					if kind == "oferta_cancelada" {
						state = "cancelado"
					}
					if err := applyOperationalTerminal(ctx, admin, f, actor, kind, state, "cambio_de_planes"); err != nil {
						t.Fatal(err)
					}
					var reason, stored string
					var active bool
					if err := reader.QueryRow(ctx, `SELECT e.motivo_codigo,a.estado,o.activo FROM public.arriendo_temprano_evento e JOIN public.arriendo_temprano a ON a.id=e.arriendo_id JOIN public.ocupacion o ON o.id=a.ocupacion_id WHERE a.id=$1 AND e.secuencia=2`, f.offerID).Scan(&reason, &stored, &active); err != nil || reason != "cambio_de_planes" || stored != state || active {
						t.Fatalf("terminal readback: %q %q %t %v", reason, stored, active, err)
					}
				})
			}
		}
		f := newEarlyRentalFixture(t, ctx, admin)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		requireTerminalConstraint(t, applyOperationalTerminal(ctx, admin, f, f.renter, "oferta_cancelada", "cancelado", "untrusted_free_text"), "arriendo_temprano_evento_motivo_ck")
	})
	t.Run("expiration-respects-deadline", func(t *testing.T) {
		f := newEarlyRentalFixture(t, ctx, admin)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		requireTerminalConstraint(t, applyOperationalTerminal(ctx, admin, f, nil, "oferta_vencida", "vencido", nil), "arriendo_temprano_evento_vencimiento_ck")
		f = newEarlyRentalFixture(t, ctx, admin)
		f.created = f.created.Add(-time.Hour)
		f.deadline = f.created.Add(30 * time.Minute)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		if err := applyOperationalTerminal(ctx, admin, f, nil, "oferta_vencida", "vencido", nil); err != nil {
			t.Fatalf("valid expired offer rejected: %v", err)
		}
		var state string
		var active bool
		if err := reader.QueryRow(ctx, `SELECT a.estado,o.activo FROM public.arriendo_temprano a JOIN public.ocupacion o ON o.id=a.ocupacion_id WHERE a.id=$1`, f.offerID).Scan(&state, &active); err != nil || state != "vencido" || active {
			t.Fatalf("expiry readback: %s %t %v", state, active, err)
		}
	})
}

func requireTerminalConstraint(t *testing.T, err error, constraint string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23514" || pe.ConstraintName != constraint {
		t.Fatalf("expected constraint %s, got %v", constraint, err)
	}
}

func applyOperationalTerminal(ctx context.Context, db *pgx.Conn, f *earlyRentalFixture, actor any, kind, state string, reason any) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET estado=$2,revision=2,actualizada_en=clock_timestamp() WHERE id=$1`, f.offerID, state); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.ocupacion SET activo=false,desactivada_en=clock_timestamp() WHERE id=$1`, f.occupancyID); err != nil {
		return err
	}
	if reason == nil && kind == "oferta_cancelada" {
		// Same existing API exercised for RED before the column/constraint existed.
		if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo) VALUES($1,2,2,$2,$3,'pendiente_identidad',$4)`, f.offerID, kind, actor, state); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,motivo_codigo) VALUES($1,2,2,$2,$3,'pendiente_identidad',$4,$5)`, f.offerID, kind, actor, state, reason); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
