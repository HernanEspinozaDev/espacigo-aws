package rentalflowpg

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestPostgresEarlyRentalCreateReplayLookup exercises the actual V99 schema,
// runtime role, advisory-key lock, and replay lookup in caller-owned txs.
func TestPostgresEarlyRentalCreateReplayLookup(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	base := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dbURL.String(), base)
	otherActor := uuid.NewString()
	seedReplayActor(t, ctx, dbURL.String(), otherActor)
	now := time.Date(2030, 5, 6, 12, 0, 0, 0, time.UTC)
	key := "replay-key-α-東京"
	fingerprint := replayFingerprint(t, base.space, now)

	t.Run("MissingNoWrites", func(t *testing.T) {
		assertLookup(t, ctx, dbURL, base.renter, key, fingerprint, uuid.Nil, false, nil)
	})

	t.Run("MatchingKeyReturnsOriginal", func(t *testing.T) {
		claimReplay(t, ctx, dbURL, base, now, key, fingerprint)
		assertLookup(t, ctx, dbURL, base.renter, key, fingerprint, uuid.MustParse(base.offer), true, nil)
	})

	t.Run("ChangedFingerprintConflict", func(t *testing.T) {
		key := key + "-conflict"
		ids := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, ids, now.Add(24*time.Hour), key, fingerprint)
		changed := fingerprint
		changed[0] ^= 0xff
		assertLookup(t, ctx, dbURL, base.renter, key, changed, uuid.Nil, false, errEarlyRentalCreateKeyConflict)
	})

	t.Run("AmbiguousSameFingerprint", func(t *testing.T) {
		key := key + "-ambiguous-same"
		ids := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, ids, now.Add(48*time.Hour), key, fingerprint)
		second := freshReplayIDs(ids)
		claimReplay(t, ctx, dbURL, second, now.Add(72*time.Hour), key, fingerprint)
		assertLookup(t, ctx, dbURL, base.renter, key, fingerprint, uuid.Nil, false, errEarlyRentalCreateReplayAmbiguous)
	})

	t.Run("AmbiguousDifferentFingerprint", func(t *testing.T) {
		key := key + "-ambiguous-different"
		ids := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, ids, now.Add(96*time.Hour), key, fingerprint)
		second := freshReplayIDs(ids)
		claimReplay(t, ctx, dbURL, second, now.Add(120*time.Hour), key, changedReplayFingerprint(fingerprint))
		// Ambiguity has precedence; even a first-row fingerprint mismatch must
		// not disclose a conflict while another row makes replay ambiguous.
		queryFingerprint := fingerprint
		queryFingerprint[1] ^= 0xff // differs from both stored fingerprints
		assertLookup(t, ctx, dbURL, base.renter, key, queryFingerprint, uuid.Nil, false, errEarlyRentalCreateReplayAmbiguous)
	})

	t.Run("ActorAndOpaqueKeyIsolation", func(t *testing.T) {
		key := key + "-isolation"
		ids := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, ids, now.Add(144*time.Hour), key, fingerprint)
		for _, variant := range []string{strings.Replace(key, "replay", "REPLAY", 1), key + " ", strings.Replace(key, "東京", "とうきょう", 1), "café-replay-key", "café-replay-key"} {
			assertLookup(t, ctx, dbURL, base.renter, variant, fingerprint, uuid.Nil, false, nil)
		}
		assertLookup(t, ctx, dbURL, otherActor, key, fingerprint, uuid.Nil, false, nil)
		// Exact code-point sequences remain distinct even where Unicode
		// canonical equivalence would otherwise normalize them.
		composed, decomposed := "café-replay-key", "café-replay-key"
		composedIDs := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, composedIDs, now.Add(168*time.Hour), composed, fingerprint)
		assertLookup(t, ctx, dbURL, base.renter, composed, fingerprint, uuid.MustParse(composedIDs.offer), true, nil)
		assertLookup(t, ctx, dbURL, base.renter, decomposed, fingerprint, uuid.Nil, false, nil)
		decomposedIDs := freshReplayIDs(base)
		claimReplay(t, ctx, dbURL, decomposedIDs, now.Add(192*time.Hour), decomposed, fingerprint)
		assertLookup(t, ctx, dbURL, base.renter, decomposed, fingerprint, uuid.MustParse(decomposedIDs.offer), true, nil)
	})
}

func replayFingerprint(t *testing.T, space string, now time.Time) [32]byte {
	t.Helper()
	start := now.Add(2 * time.Hour)
	fp, err := earlyRentalCreateFingerprint(uuid.MustParse(space), start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

func changedReplayFingerprint(fp [32]byte) [32]byte { fp[0] ^= 0x80; return fp }

func freshReplayIDs(base earlyInsertFixtureIDs) earlyInsertFixtureIDs {
	ids := earlyInsertIDs()
	ids.host, ids.renter, ids.space = base.host, base.renter, base.space
	return ids
}

func seedReplayActor(t *testing.T, ctx context.Context, dsn, id string) {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := c.Close(cleanup); err != nil {
			t.Errorf("close actor seed connection: %v", err)
		}
	}()
	_, err = c.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,correo_normalizado,hash_clave,estado) VALUES($1,$2,$2,'synthetic-test-hash','activo')`, id, id+"-isolated@example.test")
	if err != nil {
		t.Fatal(err)
	}
}

func claimReplay(t *testing.T, parent context.Context, dbURL *url.URL, ids earlyInsertFixtureIDs, now time.Time, key string, fingerprint [32]byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	// Each offer gets a disjoint, valid interval while retaining the same
	// authorized actors and space; no user/space guard is bypassed.
	record := earlyInsertRecord(t, ids, now)
	conn, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := conn.Close(cleanup); e != nil {
			t.Errorf("close claim connection: %v", e)
		}
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = tx.Rollback(cleanup)
		}
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	if err = lockEarlyRentalCreateKey(ctx, tx, uuid.MustParse(ids.renter), key); err != nil {
		t.Fatal(err)
	}
	if err = insertEarlyRentalInitial(ctx, tx, record); err != nil {
		t.Fatalf("insert valid offer/hold/event: %v", err)
	}
	var eventID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM public.arriendo_temprano_evento WHERE arriendo_id=$1 AND secuencia=1`, ids.offer).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	claimedEvent, err := insertEarlyRentalCreateClaim(ctx, tx, uuid.MustParse(ids.renter), uuid.MustParse(ids.offer), key, fingerprint)
	if err != nil || claimedEvent != eventID {
		t.Fatalf("insert guarded replay claim: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit valid claim under runtime role: %v", err)
	}
	finished = true
	reader, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := reader.Close(cleanup); err != nil {
			t.Errorf("close claim reader: %v", err)
		}
	}()
	var storedActor, storedOffer, storedEvent uuid.UUID
	var storedKey, command string
	var digest []byte
	var created time.Time
	if err := reader.QueryRow(ctx, `SELECT actor_id,arriendo_id,comando,clave,huella,evento_id,creado_en FROM public.arriendo_temprano_idempotencia WHERE arriendo_id=$1`, ids.offer).Scan(&storedActor, &storedOffer, &command, &storedKey, &digest, &storedEvent, &created); err != nil {
		t.Fatal(err)
	}
	if storedActor != uuid.MustParse(ids.renter) || storedOffer != uuid.MustParse(ids.offer) || command != "crear_oferta" || storedKey != key || !bytes.Equal(digest, fingerprint[:]) || storedEvent != eventID || !created.Equal(record.CreatedAt) {
		t.Fatal("committed create claim metadata mismatch")
	}
}

func assertLookup(t *testing.T, parent context.Context, dbURL *url.URL, actor, key string, fingerprint [32]byte, wantID uuid.UUID, wantFound bool, wantErr error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	before := replayTableCounts(t, ctx, dbURL.String())
	conn, err := pgx.Connect(ctx, dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if e := conn.Close(cleanup); e != nil {
			t.Errorf("close lookup connection: %v", e)
		}
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = tx.Rollback(cleanup)
		}
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	actorID := uuid.MustParse(actor)
	if err = lockEarlyRentalCreateKey(ctx, tx, actorID, key); err != nil {
		t.Fatalf("lock before lookup: %v", err)
	}
	id, found, gotErr := lookupEarlyRentalCreateReplay(ctx, tx, actorID, key, fingerprint)
	if wantErr == nil {
		if gotErr != nil {
			t.Fatalf("lookup error=%v, want nil", gotErr)
		}
	} else if gotErr != wantErr {
		t.Fatalf("lookup error=%v, want exact %v", gotErr, wantErr)
	}
	if wantErr != nil && (id != uuid.Nil || found) {
		t.Fatalf("error result leaked id/found: %s/%v", id, found)
	}
	if wantErr == nil && (id != wantID || found != wantFound) {
		t.Fatalf("lookup=(%s,%v), want (%s,%v)", id, found, wantID, wantFound)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatalf("read-only lookup rollback: %v", err)
	}
	finished = true
	after := replayTableCounts(t, ctx, dbURL.String())
	if before != after {
		t.Fatalf("lookup changed fixture table counts: before=%v after=%v", before, after)
	}
}

type replayCounts struct{ offers, occupancies, events, claims int64 }

func replayTableCounts(t *testing.T, ctx context.Context, dsn string) replayCounts {
	t.Helper()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := c.Close(cleanup); err != nil {
			t.Errorf("close count connection: %v", err)
		}
	}()
	var n replayCounts
	err = c.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.arriendo_temprano),(SELECT count(*) FROM public.ocupacion),(SELECT count(*) FROM public.arriendo_temprano_evento),(SELECT count(*) FROM public.arriendo_temprano_idempotencia)`).Scan(&n.offers, &n.occupancies, &n.events, &n.claims)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
