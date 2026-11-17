package migrator

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Exercise E1 evidence only after the shared V99 core suite has passed for
// both fixture histories. V100 exists solely in this test's temporary inventory.
func TestRunnerEarlyRentalEvidenceCandidatePostgres(t *testing.T) {
	withEarlyRentalCandidate(t, func(t *testing.T, ctx context.Context, admin, reader *pgx.Conn, databaseURL string, inventory []Migration) {
		sql, err := os.ReadFile(filepath.Join("testdata", "early_rental_evidence_candidate.sql"))
		if err != nil {
			t.Fatal(err)
		}
		dir := prefixMigrationDirForCombined(t, inventory, 99)
		writeMigration(t, dir, "V000100__early_rental_evidence_candidate.sql", string(sql))
		next, err := DiscoverMigrations(dir)
		if err != nil {
			t.Fatal(err)
		}
		target := SingleOwnerMigrationTarget{Host: admin.Config().Host, Port: admin.Config().Port}
		if err := admin.QueryRow(ctx, `SELECT current_database(),d.oid,(SELECT system_identifier::text FROM pg_catalog.pg_control_system()),current_setting('server_version_num')::int,session_user::text,(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.ocupacion'::regclass),(SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.schema_migrations'::regclass) FROM pg_database d WHERE datname=current_database()`).Scan(&target.Database, &target.DatabaseOID, &target.SystemIdentifier, &target.ServerVersionNum, &target.SessionUser, &target.OccupancyOwner, &target.LedgerOwner); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `GRANT CREATE ON SCHEMA public TO espacigo_schema_owner; GRANT REFERENCES(id) ON public.usuario TO espacigo_schema_owner`); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := admin.Exec(context.Background(), `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner; REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner`); err != nil {
				t.Errorf("revoke temporary CREATE: %v", err)
			}
		}()
		contract := SingleOwnerMigrationContract{InventorySHA256: inventoryFingerprint(next), Version: 100, Name: next[99].Name, Checksum: next[99].Checksum, Target: target}
		got, err := RunSingleOwnerMigration(ctx, admin, dir, contract)
		if err != nil || !got.Committed || !got.Verified {
			t.Fatalf("install evidence candidate: %+v %v", got, err)
		}
		if _, err := admin.Exec(ctx, `REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner; REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner`); err != nil {
			t.Fatal(err)
		}
		var acl bool
		if err := reader.QueryRow(ctx, `SELECT NOT has_schema_privilege('espacigo_schema_owner','public','CREATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','UPDATE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','DELETE') AND has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','SELECT') AND has_table_privilege('espacigo_runtime','public.arriendo_temprano_confirmacion','INSERT') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_evento','DELETE') AND NOT has_table_privilege('espacigo_runtime','public.arriendo_temprano_evento','UPDATE') AND has_table_privilege('espacigo_runtime','public.arriendo_temprano_pago','UPDATE')`).Scan(&acl); err != nil || !acl {
			t.Fatalf("runtime ACL: %t %v", acl, err)
		}
		f := newEarlyRentalFixture(t, ctx, admin)
		if err := seedEvidenceOffer(ctx, admin, f); err != nil {
			t.Fatal(err)
		}
		good := func(t *testing.T, q string, args ...any) {
			t.Helper()
			if _, err := admin.Exec(ctx, q, args...); err != nil {
				t.Fatal(err)
			}
		}
		bad := func(t *testing.T, code, q string, args ...any) {
			t.Helper()
			before := [4]int{evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_pago", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_idempotencia", f.offerID)}
			_, err := admin.Exec(ctx, q, args...)
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || pe.Code != code {
				t.Fatalf("expected SQLSTATE %s, got %v", code, err)
			}
			after := [4]int{evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_pago", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID), evidenceCount(t, ctx, reader, "arriendo_temprano_idempotencia", f.offerID)}
			if after != before {
				t.Fatalf("rejected evidence write changed counts: %v -> %v", before, after)
			}
		}
		digest := sha256.Sum256([]byte("synthetic early rental contract"))
		t.Run("confirmation-participant-and-exact-contract", func(t *testing.T) {
			good(t, `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase) VALUES($1,$2,'identidad')`, f.offerID, f.owner)
			before := evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID)
			bad(t, "23505", `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase) VALUES($1,$2,'identidad')`, f.offerID, f.owner)
			bad(t, "23514", `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest) VALUES($1,$2,'contrato',2,$3)`, f.offerID, f.renter, digest[:])
			bad(t, "23514", `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase) VALUES($1,$2,'identidad')`, f.offerID, uuid.NewString())
			bad(t, "23514", `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest) VALUES($1,$2,'contrato',1,$3)`, f.offerID, f.renter, make([]byte, 32))
			if n := evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID); n != before {
				t.Fatalf("unexpected confirmation count %d", n)
			}
			good(t, `INSERT INTO arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest) VALUES($1,$2,'contrato',1,$3)`, f.offerID, f.renter, digest[:])
			if n := evidenceCount(t, ctx, reader, "arriendo_temprano_confirmacion", f.offerID); n != before+1 {
				t.Fatalf("contract acceptance was not persisted: %d", n)
			}
		})
		t.Run("payment-snapshot-parties-and-order", func(t *testing.T) {
			bad(t, "23514", `INSERT INTO arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado,recibido_en,enviado_en) VALUES($1,$2,$3,10000,'CLP','recibido',now(),now())`, f.offerID, f.renter, f.owner)
			bad(t, "23514", `INSERT INTO arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado) VALUES($1,$2,$3,9999,'CLP','pendiente')`, f.offerID, f.renter, f.owner)
			bad(t, "23514", `INSERT INTO arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado) VALUES($1,$2,$3,10000,'CLP','pendiente')`, f.offerID, f.owner, f.renter)
			good(t, `INSERT INTO arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado) VALUES($1,$2,$3,10000,'CLP','pendiente')`, f.offerID, f.renter, f.owner)
			bad(t, "23505", `INSERT INTO arriendo_temprano_pago(arriendo_id,emisor_id,receptor_id,monto_clp,moneda,estado) VALUES($1,$2,$3,10000,'CLP','pendiente')`, f.offerID, f.renter, f.owner)
			bad(t, "55000", `UPDATE arriendo_temprano_pago SET estado='recibido',recibido_en=now() WHERE arriendo_id=$1`, f.offerID)
			good(t, `UPDATE arriendo_temprano_pago SET estado='enviado',enviado_en=clock_timestamp() WHERE arriendo_id=$1`, f.offerID)
			bad(t, "23514", `UPDATE arriendo_temprano_pago SET estado='recibido',recibido_en=enviado_en-interval '1 second' WHERE arriendo_id=$1`, f.offerID)
			good(t, `UPDATE arriendo_temprano_pago SET estado='recibido',recibido_en=enviado_en+interval '1 second' WHERE arriendo_id=$1`, f.offerID)
			var received bool
			if err := reader.QueryRow(ctx, `SELECT estado='recibido' AND recibido_en>=enviado_en FROM public.arriendo_temprano_pago WHERE arriendo_id=$1`, f.offerID).Scan(&received); err != nil || !received {
				t.Fatalf("payment committed readback: %t %v", received, err)
			}
		})
		t.Run("events-actor-sequence-and-append-only", func(t *testing.T) {
			var confirmationID string
			if err := reader.QueryRow(ctx, `SELECT id FROM public.arriendo_temprano_confirmacion WHERE arriendo_id=$1 AND actor_id=$2 AND clase='identidad'`, f.offerID, f.owner).Scan(&confirmationID); err != nil {
				t.Fatal(err)
			}
			tx, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET revision=2,actualizada_en=clock_timestamp() WHERE id=$1`, f.offerID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,confirmacion_id) VALUES($1,2,2,'identidad_simulada_confirmada',$2,'pendiente_identidad','pendiente_identidad',$3)`, f.offerID, f.owner, confirmationID); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			bad(t, "23514", `INSERT INTO arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo) VALUES($1,2,2,'contrato_aceptado',$2,'pendiente_contrato','pendiente_envio_pago')`, f.offerID, uuid.NewString())
			bad(t, "23505", `INSERT INTO arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_nuevo) VALUES($1,1,1,'oferta_creada',$2,'pendiente_identidad')`, f.offerID, f.renter)
			var id string
			if err := reader.QueryRow(ctx, `SELECT id FROM arriendo_temprano_evento WHERE arriendo_id=$1 AND secuencia=1`, f.offerID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			bad(t, "55000", `UPDATE arriendo_temprano_evento SET revision=9 WHERE id=$1`, id)
			bad(t, "55000", `DELETE FROM arriendo_temprano_evento WHERE id=$1`, id)
		})
		t.Run("idempotency-unique-key-and-fingerprint", func(t *testing.T) {
			var eventID string
			if err := reader.QueryRow(ctx, `SELECT id FROM arriendo_temprano_evento WHERE arriendo_id=$1 AND secuencia=1`, f.offerID).Scan(&eventID); err != nil {
				t.Fatal(err)
			}
			fp := make([]byte, 32)
			good(t, `INSERT INTO arriendo_temprano_idempotencia(arriendo_id,actor_id,comando,clave,huella,evento_id) VALUES($1,$2,'crear_oferta','unicode-ñ-123', $3,$4)`, f.offerID, f.renter, fp, eventID)
			bad(t, "23505", `INSERT INTO arriendo_temprano_idempotencia(arriendo_id,actor_id,comando,clave,huella,evento_id) VALUES($1,$2,'crear_oferta','unicode-ñ-123', $3,$4)`, f.offerID, f.renter, fp, eventID)
			bad(t, "23514", `INSERT INTO arriendo_temprano_idempotencia(arriendo_id,actor_id,comando,clave,huella,evento_id) VALUES($1,$2,'crear_oferta','invalid-key',decode('00','hex'),$3)`, f.offerID, f.renter, eventID)
			bad(t, "23514", `INSERT INTO arriendo_temprano_idempotencia(arriendo_id,actor_id,comando,clave,huella,evento_id) VALUES($1,$2,'aceptar_contrato','wrong-command', $3,$4)`, f.offerID, f.renter, fp, eventID)
		})
		assertEvidenceEventInvariants(t, ctx, admin, reader)
		assertEvidenceBilateralLifecycle(t, ctx, admin, reader)
	})
}

func seedEvidenceOffer(ctx context.Context, db *pgx.Conn, f *earlyRentalFixture) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
		return err
	}
	if err = f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, "retencion_temprana", nil, f.deadline, true); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_nuevo) VALUES($1,1,1,'oferta_creada',$2,'pendiente_identidad')`, f.offerID, f.renter); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func evidenceCount(t *testing.T, ctx context.Context, db *pgx.Conn, table, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM public."+table+" WHERE arriendo_id=$1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func assertEvidenceEventInvariants(t *testing.T, ctx context.Context, admin, reader *pgx.Conn) {
	t.Helper()
	for _, name := range []string{"gap", "missing-confirmation", "wrong-confirmation-actor", "wrong-confirmation-kind", "missing-payment", "prior-state", "event-without-revision", "revision-without-event", "wrong-next-state"} {
		t.Run("event-integrity-"+name, func(t *testing.T) {
			f := newEarlyRentalFixture(t, ctx, admin)
			if err := seedEvidenceOffer(ctx, admin, f); err != nil {
				t.Fatal(err)
			}
			var identityID, contractID string
			if err := admin.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase) VALUES($1,$2,'identidad') RETURNING id`, f.offerID, f.owner).Scan(&identityID); err != nil {
				t.Fatal(err)
			}
			if err := admin.QueryRow(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(arriendo_id,actor_id,clase,contrato_version,contrato_digest) SELECT id,anfitrion_id,'contrato',contrato_version,contrato_digest FROM public.arriendo_temprano WHERE id=$1 RETURNING id`, f.offerID).Scan(&contractID); err != nil {
				t.Fatal(err)
			}
			before := earlyRentalCounts(t, ctx, reader)
			beforeEvents := evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID)
			tx, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			sequence, kind, actor := int64(2), "identidad_simulada_confirmada", f.owner
			prior, next, confirmation := "pendiente_identidad", "pendiente_identidad", any(identityID)
			switch name {
			case "gap":
				sequence = 3
			case "missing-confirmation":
				confirmation = nil
			case "wrong-confirmation-actor":
				actor = f.renter
			case "wrong-confirmation-kind":
				confirmation = contractID
			case "missing-payment":
				kind, actor = "pago_marcado_enviado", f.renter
			case "prior-state":
				prior = "pendiente_contrato"
			case "wrong-next-state":
				next = "confirmado"
			}
			if name == "revision-without-event" {
				_, err = tx.Exec(ctx, `UPDATE public.arriendo_temprano SET revision=revision+1,actualizada_en=clock_timestamp() WHERE id=$1`, f.offerID)
			} else {
				if name == "missing-payment" {
					confirmation = nil
				}
				_, err = tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(arriendo_id,secuencia,revision,tipo,actor_id,estado_anterior,estado_nuevo,confirmacion_id) VALUES($1,$2,$2,$3,$4,$5,$6,$7)`, f.offerID, sequence, kind, actor, prior, next, confirmation)
			}
			if err == nil {
				err = tx.Commit(ctx)
			} else if rb := tx.Rollback(ctx); rb != nil {
				t.Fatal(rb)
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("want integrity SQLSTATE23514, got %v", err)
			}
			if earlyRentalCounts(t, ctx, reader) != before || evidenceCount(t, ctx, reader, "arriendo_temprano_evento", f.offerID) != beforeEvents {
				t.Fatal("failed event transaction changed committed counts")
			}
			var revision int64
			var state string
			if err := reader.QueryRow(ctx, `SELECT revision,estado FROM public.arriendo_temprano WHERE id=$1`, f.offerID).Scan(&revision, &state); err != nil || revision != 1 || state != "pendiente_identidad" {
				t.Fatalf("aggregate changed after event rollback: %d/%s %v", revision, state, err)
			}
		})
	}
	t.Run("event-integrity-missing-initial-event", func(t *testing.T) {
		f := newEarlyRentalFixture(t, ctx, admin)
		before := earlyRentalCounts(t, ctx, reader)
		tx, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if err := f.insertOffer(ctx, tx, f.offerID, f.occupancyID, f.spaceID, f.start); err != nil {
			t.Fatal(err)
		}
		if err := f.insertOccupancy(ctx, tx, f.occupancyID, f.offerID, f.spaceID, f.start, "retencion_temprana", nil, f.deadline, true); err != nil {
			t.Fatal(err)
		}
		assertPgError(t, tx.Commit(ctx), "23514", "arriendo_temprano_evento_consistencia_ck")
		if earlyRentalCounts(t, ctx, reader) != before {
			t.Fatal("offer without initial event committed")
		}
	})
}
