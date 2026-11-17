package rentalflowpg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPostgresEarlyRentalReadOffer(t *testing.T) {
	cases := []string{"InitialBothParticipants", "NonParticipantHidden", "InactiveActorHidden", "BlockedActorHidden", "PersistedSnapshotNotCurrentListing", "CurrentIdentityConfirmation", "InactiveCounterpartVisible"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, dbURL := earlyInsertPostgresFixture(t)
			dsn := dbURL.String()
			grantEarlyOfferReadColumns(t, ctx, dsn)
			ids := earlyInsertIDs()
			seedEarlyInsertActors(t, ctx, dsn, ids)
			now := time.Date(2030, 6, 10, 12, 0, 0, 123456000, time.UTC)
			record := earlyInsertRecord(t, ids, now)
			commitEarlyReadFixtureOffer(t, ctx, dsn, record)

			switch name {
			case "InactiveCounterpartVisible":
				admin := connectEarlyReadAdmin(t, ctx, dsn)
				if _, err := admin.Exec(ctx, `UPDATE public.usuario SET estado='bloqueado' WHERE id=$1`, ids.host); err != nil {
					t.Fatal(err)
				}
				assertEarlyOfferVisible(t, ctx, dsn, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer), record, 1, nil)
			case "NonParticipantHidden":
				actor := uuid.New()
				insertEarlyReadActor(t, ctx, dsn, actor, "outsider")
				assertEarlyOfferHidden(t, ctx, dsn, actor, uuid.MustParse(ids.offer))
			case "InactiveActorHidden":
				admin := connectEarlyReadAdmin(t, ctx, dsn)
				if _, err := admin.Exec(ctx, `UPDATE public.usuario SET estado='bloqueado' WHERE id=$1`, ids.renter); err != nil {
					t.Fatal(err)
				}
				assertEarlyOfferHidden(t, ctx, dsn, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer))
			case "BlockedActorHidden":
				admin := connectEarlyReadAdmin(t, ctx, dsn)
				if _, err := admin.Exec(ctx, `INSERT INTO public.bloqueo_cuenta_administrativo_local(cuenta_id,bloqueada_por,motivo_codigo,bloqueada_en,correlacion_id) VALUES($1,$2,'revision_administrativa',$3,$4)`, ids.renter, ids.host, now, "read-test-"+uuid.NewString()); err != nil {
					t.Fatal(err)
				}
				assertEarlyOfferHidden(t, ctx, dsn, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer))
			case "InitialBothParticipants":
				for _, actor := range []string{ids.host, ids.renter} {
					assertEarlyOfferVisible(t, ctx, dsn, uuid.MustParse(actor), uuid.MustParse(ids.offer), record, 1, nil)
				}
			case "PersistedSnapshotNotCurrentListing":
				admin := connectEarlyReadAdmin(t, ctx, dsn)
				if _, err := admin.Exec(ctx, `UPDATE public.espacio SET titulo='Changed after offer',precio_base_clp=12000,estado='oculta' WHERE id=$1`, ids.space); err != nil {
					t.Fatal(err)
				}
				assertEarlyOfferVisible(t, ctx, dsn, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer), record, 1, nil)
			case "CurrentIdentityConfirmation":
				at := now.Add(time.Minute)
				confirmID := uuid.New()
				conn, tx := createClaimNativeTx(t, ctx, dsn)
				if _, err := tx.Exec(ctx, `UPDATE public.arriendo_temprano SET revision=2,actualizada_en=$2 WHERE id=$1 AND revision=1`, ids.offer, at); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_confirmacion(id,arriendo_id,actor_id,clase,confirmada_en) VALUES($1,$2,$3,'identidad',$4)`, confirmID, ids.offer, ids.renter, at); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(ctx, `INSERT INTO public.arriendo_temprano_evento(id,arriendo_id,secuencia,revision,tipo,actor_id,ocurrido_en,estado_anterior,estado_nuevo,confirmacion_id) VALUES($1,$2,2,2,'identidad_simulada_confirmada',$3,$4,'pendiente_identidad','pendiente_identidad',$5)`, uuid.New(), ids.offer, ids.renter, at, confirmID); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatalf("commit with deferred consistency guards: %v", err)
				}
				if err := conn.Close(ctx); err != nil {
					t.Fatal(err)
				}
				want := map[string]time.Time{ids.renter: at.UTC()}
				assertEarlyOfferVisible(t, ctx, dsn, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer), record, 2, want)
			}
		})
	}
}

func grantEarlyOfferReadColumns(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(context.Background()); err != nil {
			t.Errorf("close ACL setup connection: %v", err)
		}
	}()
	if _, err := c.Exec(ctx, `GRANT SELECT (id,estado) ON TABLE public.usuario TO espacigo_runtime; GRANT SELECT (cuenta_id) ON TABLE public.bloqueo_cuenta_administrativo_local TO espacigo_runtime`); err != nil {
		t.Fatalf("grant only reader-required identity columns: %v", err)
	}
	var userID, userState, blockAccount bool
	if err := c.QueryRow(ctx, `SELECT has_column_privilege('espacigo_runtime','public.usuario','id','SELECT'),has_column_privilege('espacigo_runtime','public.usuario','estado','SELECT'),has_column_privilege('espacigo_runtime','public.bloqueo_cuenta_administrativo_local','cuenta_id','SELECT')`).Scan(&userID, &userState, &blockAccount); err != nil {
		t.Fatal(err)
	}
	if !userID || !userState || !blockAccount {
		t.Fatalf("reader-column grant readback failed: usuario.id=%v usuario.estado=%v bloqueo.cuenta_id=%v", userID, userState, blockAccount)
	}
}

func connectEarlyReadAdmin(t *testing.T, ctx context.Context, dsn string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(context.Background()); err != nil {
			t.Errorf("close fixture admin: %v", err)
		}
	})
	return c
}

func commitEarlyReadFixtureOffer(t *testing.T, ctx context.Context, dsn string, record earlyRentalInitialRecord) {
	t.Helper()
	c, tx := createClaimNativeTx(t, ctx, dsn)
	if err := insertEarlyRentalInitial(ctx, tx, record); err != nil {
		t.Fatalf("seed committed offer: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit seeded offer: %v", err)
	}
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func insertEarlyReadActor(t *testing.T, ctx context.Context, dsn string, id uuid.UUID, label string) {
	t.Helper()
	c := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := c.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'synthetic-test-hash','activo')`, id, id.String()+"-"+label+"@example.test"); err != nil {
		t.Fatal(err)
	}
}

func assertEarlyOfferHidden(t *testing.T, ctx context.Context, dsn string, actor, offer uuid.UUID) {
	t.Helper()
	before := earlyOfferReadCounts(t, ctx, dsn)
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := readEarlyRentalOffer(ctx, tx, actor, offer)
	if !errors.Is(err, rentalflow.ErrNotFound) || !reflect.DeepEqual(got, earlyRentalStoredOffer{}) {
		t.Fatalf("hidden read returned nonzero result: %+v, %v", got, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
		t.Fatalf("reader changed table counts: %v -> %v", before, after)
	}
}

func assertEarlyOfferVisible(t *testing.T, ctx context.Context, dsn string, actor, offer uuid.UUID, record earlyRentalInitialRecord, revision uint64, identity map[string]time.Time) {
	t.Helper()
	before := earlyOfferReadCounts(t, ctx, dsn)
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := readEarlyRentalOffer(ctx, tx, actor, offer)
	if err != nil {
		t.Fatalf("read offer: %v", err)
	}
	want := record.Offer
	want.Revision = revision
	want.IdentityConfirmed = identity
	if want.IdentityConfirmed == nil {
		want.IdentityConfirmed = map[string]time.Time{}
	}
	if want.ContractAccepted == nil {
		want.ContractAccepted = map[string]time.Time{}
	}
	wantUpdated := record.UpdatedAt.UTC()
	if revision == 2 {
		wantUpdated = wantUpdated.Add(time.Minute)
	}
	if !earlyOfferValuesEqual(got.Offer, want) || !got.CreatedAt.Equal(record.CreatedAt.UTC()) || !got.UpdatedAt.Equal(wantUpdated) || got.LastEventSequence != revision {
		t.Fatalf("read result differs from persisted contract snapshot: got=%+v created=%s updated=%s seq=%d", got.Offer, got.CreatedAt, got.UpdatedAt, got.LastEventSequence)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
		t.Fatalf("reader changed table counts: %v -> %v", before, after)
	}
}

func createEarlyOfferReadTx(t *testing.T, ctx context.Context, dsn string) (*pgx.Conn, pgx.Tx) {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(context.Background()); err != nil {
			t.Errorf("close reader connection: %v", err)
		}
	})
	tx, err := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback read transaction: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	return c, tx
}

type earlyOfferCounts struct{ offers, occupancies, events, confirmations, claims int64 }

func earlyOfferReadCounts(t *testing.T, ctx context.Context, dsn string) earlyOfferCounts {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(context.Background()); err != nil {
			t.Errorf("close independent count connection: %v", err)
		}
	}()
	var n earlyOfferCounts
	err = c.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.arriendo_temprano),(SELECT count(*) FROM public.ocupacion),(SELECT count(*) FROM public.arriendo_temprano_evento),(SELECT count(*) FROM public.arriendo_temprano_confirmacion),(SELECT count(*) FROM public.arriendo_temprano_idempotencia)`).Scan(&n.offers, &n.occupancies, &n.events, &n.confirmations, &n.claims)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func earlyOfferValuesEqual(got, want earlyrental.Offer) bool {
	// Compare all offer fields, retaining nil/empty map equivalence only for the
	// confirmation maps as defined by the domain's initial-offer contract.
	return reflect.DeepEqual(got, want)
}
