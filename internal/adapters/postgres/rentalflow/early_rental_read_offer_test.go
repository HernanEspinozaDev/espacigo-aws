package rentalflowpg

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	offerReadHost   = "10000000-0000-4000-8000-000000000001"
	offerReadRenter = "20000000-0000-4000-8000-000000000002"
	offerReadID     = "30000000-0000-4000-8000-000000000003"
	offerReadSpace  = "40000000-0000-4000-8000-000000000004"
	offerReadPayID  = "50000000-0000-4000-8000-000000000005"
)

// This SQL oracle is independently copied from the persisted reader contract;
// it is deliberately not read from production constants or assembled by them.
const offerReadFinalSQLOracle = `SELECT o.flow_version,o.espacio_id,o.anfitrion_id,o.arrendatario_id,o.titulo_snapshot,o.modalidad,o.precio_unitario_clp,o.moneda,o.unidades,o.subtotal_clp,o.inicio,o.termino,o.zona_horaria,o.contrato_version,o.contrato_texto,o.contrato_digest,o.estado,o.revision,o.creada_en,o.actualizada_en,o.vence_en,
COALESCE((SELECT jsonb_agg(jsonb_build_object('actor',c.actor_id,'class',c.clase,'at',c.confirmada_en,'version',c.contrato_version,'digest',encode(c.contrato_digest,'hex')) ORDER BY c.actor_id,c.clase) FROM public.arriendo_temprano_confirmacion c WHERE c.arriendo_id=o.id),'[]'::jsonb),
(SELECT jsonb_build_object('id',p.pago_id,'sender',p.emisor_id,'receiver',p.receptor_id,'amount',p.monto_clp,'currency',p.moneda,'state',p.estado,'sent',p.enviado_en,'received',p.recibido_en) FROM public.arriendo_temprano_pago p WHERE p.arriendo_id=o.id),
(SELECT count(*) FROM public.arriendo_temprano_evento e WHERE e.arriendo_id=o.id),(SELECT COALESCE(max(e.secuencia),0) FROM public.arriendo_temprano_evento e WHERE e.arriendo_id=o.id)
FROM public.arriendo_temprano o WHERE o.id=$1 AND o.anfitrion_id=$2 AND o.arrendatario_id=$3 AND $4 IN(o.anfitrion_id,o.arrendatario_id)
AND EXISTS(SELECT 1 FROM public.usuario u WHERE u.id=$4 AND u.estado='activo')
AND NOT EXISTS(SELECT 1 FROM public.bloqueo_cuenta_administrativo_local b WHERE b.cuenta_id=$4)`

const offerReadHeaderSQLOracle = `SELECT anfitrion_id, arrendatario_id FROM public.arriendo_temprano WHERE id=$1 AND $2 IN (anfitrion_id,arrendatario_id)`

type offerReadRow struct {
	version                                           string
	space, host, renter                               uuid.UUID
	title, unit, currency, zone, contract, state      string
	price, units, subtotal, contractVersion, revision int64
	start, end, created, updated, expires             time.Time
	digest, confirms, payment                         []byte
	eventCount, maxSequence                           int64
}

type offerReadFakeTx struct {
	pgx.Tx
	row                       offerReadRow
	headerNoRows, finalNoRows bool
	headerErr, finalErr       error
	lockErrAt                 int
	calls                     []string
	lockIDs                   []string
	finalAfter                func()
}

type offerReadFakeRow struct {
	values []any
	err    error
	after  func()
}

func (r offerReadFakeRow) Scan(dest ...any) error {
	if r.after != nil {
		defer r.after()
	}
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("fake scan destination count mismatch")
	}
	for i, d := range dest {
		dv := reflect.ValueOf(d)
		if dv.Kind() != reflect.Ptr || dv.IsNil() {
			return errors.New("fake scan destination is not pointer")
		}
		sv := reflect.ValueOf(r.values[i])
		if !sv.IsValid() {
			dv.Elem().SetZero()
			continue
		}
		if !sv.Type().AssignableTo(dv.Elem().Type()) {
			return errors.New("fake scan type mismatch: " + sv.Type().String() + " -> " + dv.Elem().Type().String())
		}
		dv.Elem().Set(sv)
	}
	return nil
}
func (f *offerReadFakeTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.calls = append(f.calls, "query:"+query)
	switch query {
	case offerReadHeaderSQLOracle:
		if len(args) != 2 || args[0] != uuid.MustParse(offerReadID) || (args[1] != uuid.MustParse(offerReadRenter) && args[1] != uuid.MustParse(offerReadHost)) {
			return offerReadFakeRow{err: errors.New("header args mismatch")}
		}
		if f.headerErr != nil {
			return offerReadFakeRow{err: f.headerErr}
		}
		if f.headerNoRows {
			return offerReadFakeRow{err: pgx.ErrNoRows}
		}
		return offerReadFakeRow{values: []any{uuid.MustParse(offerReadHost), uuid.MustParse(offerReadRenter)}}
	case offerReadFinalSQLOracle:
		if len(args) != 4 || args[0] != uuid.MustParse(offerReadID) || args[1] != uuid.MustParse(offerReadHost) || args[2] != uuid.MustParse(offerReadRenter) || (args[3] != uuid.MustParse(offerReadRenter) && args[3] != uuid.MustParse(offerReadHost)) {
			return offerReadFakeRow{err: errors.New("final args mismatch")}
		}
		if f.finalErr != nil {
			return offerReadFakeRow{err: f.finalErr}
		}
		if f.finalNoRows {
			return offerReadFakeRow{err: pgx.ErrNoRows}
		}
		r := f.row
		return offerReadFakeRow{values: []any{r.version, r.space, r.host, r.renter, r.title, r.unit, r.price, r.currency, r.units, r.subtotal, r.start, r.end, r.zone, r.contractVersion, r.contract, r.digest, r.state, r.revision, r.created, r.updated, r.expires, r.confirms, r.payment, r.eventCount, r.maxSequence}, after: f.finalAfter}
	default:
		return offerReadFakeRow{err: errors.New("unexpected SQL query")}
	}
}
func (f *offerReadFakeTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, "exec:"+query)
	if query != `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))` || len(args) != 1 {
		return pgconn.CommandTag{}, errors.New("unexpected account-lock SQL")
	}
	id, ok := args[0].(string)
	if !ok {
		return pgconn.CommandTag{}, errors.New("account lock key not string")
	}
	f.lockIDs = append(f.lockIDs, id)
	if f.lockErrAt > 0 && len(f.lockIDs) == f.lockErrAt {
		return pgconn.CommandTag{}, f.finalErr
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (*offerReadFakeTx) Begin(context.Context) (pgx.Tx, error) {
	panic("reader must not begin a transaction")
}
func (*offerReadFakeTx) Commit(context.Context) error   { panic("reader must not commit") }
func (*offerReadFakeTx) Rollback(context.Context) error { panic("reader must not roll back") }

func TestReadEarlyRentalOfferPartialIdentity(t *testing.T) {
	row, want := offerReadFixture(earlyrental.StatePendingIdentity, 2, offerReadConfirms("identidad"), nil)
	if err := earlyrental.ValidateOffer(want); err != nil {
		t.Fatal("invalid independent control", err)
	}
	got, err := readEarlyRentalOffer(context.Background(), &offerReadFakeTx{row: row}, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID))
	if err != nil || !reflect.DeepEqual(got, earlyRentalStoredOffer{Offer: want, CreatedAt: row.created.UTC(), UpdatedAt: row.updated.UTC(), LastEventSequence: 2}) {
		t.Fatalf("partial identity read mismatch: %#v %v", got, err)
	}
}

func TestReadEarlyRentalOfferCanceledAfterFinalRead(t *testing.T) {
	row, _ := offerReadFixture(earlyrental.StatePendingIdentity, 1, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &offerReadFakeTx{row: row, finalAfter: cancel}
	got, err := readEarlyRentalOffer(ctx, f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("late cancellation: %v", err)
	}
	assertOfferReadZero(t, got)
}

func TestReadEarlyRentalOfferRejectsCorruptStoredFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*offerReadRow)
	}{
		{"flow", func(r *offerReadRow) { r.version = "other" }},
		{"revision", func(r *offerReadRow) { r.revision = 0 }},
		{"event count", func(r *offerReadRow) { r.eventCount = 2 }},
		{"event sequence", func(r *offerReadRow) { r.maxSequence = 2 }},
		{"short digest", func(r *offerReadRow) { r.digest = r.digest[:31] }},
		{"digest mismatch", func(r *offerReadRow) { r.digest[0] ^= 1 }},
		{"created zero", func(r *offerReadRow) { r.created = time.Time{} }},
		{"updated before creation", func(r *offerReadRow) { r.updated = r.created.Add(-time.Second) }},
		{"expiry", func(r *offerReadRow) { r.expires = r.expires.Add(time.Second) }},
		{"confirmation JSON", func(r *offerReadRow) { r.confirms = []byte("{") }},
		{"payment JSON", func(r *offerReadRow) { r.payment = []byte("{") }},
		{"duplicate confirmation", func(r *offerReadRow) {
			cs := offerReadConfirms("identidad")
			cs = append(cs, cs[0])
			r.confirms, _ = json.Marshal(cs)
		}},
		{"foreign confirmation actor", func(r *offerReadRow) {
			cs := offerReadConfirms("identidad")
			cs[0]["actor"] = offerReadPayID
			r.confirms, _ = json.Marshal(cs)
		}},
		{"nil confirmation time", func(r *offerReadRow) {
			cs := offerReadConfirms("identidad")
			cs[0]["at"] = nil
			r.confirms, _ = json.Marshal(cs)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, want := offerReadFixture(earlyrental.StatePendingIdentity, 1, nil, nil)
			if err := earlyrental.ValidateOffer(want); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&row)
			got, err := readEarlyRentalOffer(context.Background(), &offerReadFakeTx{row: row}, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID))
			if !errors.Is(err, earlyrental.ErrInvalid) {
				t.Fatalf("corrupt facts: %v", err)
			}
			assertOfferReadZero(t, got)
		})
	}
}

func offerReadFixture(state earlyrental.State, revision int64, confirms []map[string]any, payment map[string]any) (offerReadRow, earlyrental.Offer) {
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("fixture", -5*60*60))
	start := created.Add(48 * time.Hour)
	end := start.Add(2 * time.Hour)
	contract := "Contrato fijo de prueba independiente."
	digest := sha256.Sum256([]byte(contract))
	confirmJSON, _ := json.Marshal(confirms)
	var paymentJSON []byte
	if payment != nil {
		paymentJSON, _ = json.Marshal(payment)
	}
	r := offerReadRow{version: "manual_early_v1", space: uuid.MustParse(offerReadSpace), host: uuid.MustParse(offerReadHost), renter: uuid.MustParse(offerReadRenter), title: "Cabaña fija", unit: "hora", price: 12500, currency: "CLP", units: 2, subtotal: 25000, start: start, end: end, zone: "America/Santiago", contractVersion: 2, contract: contract, digest: append([]byte(nil), digest[:]...), state: string(state), revision: revision, created: created, updated: created.Add(10 * time.Minute), expires: created.Add(30 * time.Minute), confirms: confirmJSON, payment: paymentJSON, eventCount: revision, maxSequence: revision}
	// Expected aggregate is constructed directly from fixture literals, not from row values.
	want := earlyrental.Offer{ID: offerReadID, HostID: offerReadHost, RenterID: offerReadRenter, Snapshot: earlyrental.OfferSnapshot{SpaceID: uuid.MustParse(offerReadSpace), SpaceTitle: "Cabaña fija", RateUnit: "hora", UnitPriceCLP: 12500, Currency: "CLP", Units: 2, SubtotalCLP: 25000, StartAt: start.UTC(), EndAt: end.UTC(), TimeZone: "America/Santiago"}, State: state, Revision: uint64(revision), ContractVersion: 2, ContractText: contract, ContractDigest: "", IdentityConfirmed: map[string]time.Time{}, ContractAccepted: map[string]time.Time{}, ExpiresAt: created.Add(30 * time.Minute).UTC()}
	want.ContractDigest = fmtHex(digest[:])
	for _, c := range confirms {
		at := c["at"].(time.Time).UTC()
		actor := c["actor"].(string)
		if c["class"] == "identidad" {
			want.IdentityConfirmed[actor] = at
		} else {
			want.ContractAccepted[actor] = at
		}
	}
	if payment != nil {
		if s, ok := payment["sent"].(time.Time); ok {
			v := s.UTC()
			want.PaymentSentAt = &v
		}
		if s, ok := payment["received"].(time.Time); ok {
			v := s.UTC()
			want.PaymentReceivedAt = &v
		}
		if payment["state"] == "enviado" || payment["state"] == "recibido" {
			want.PaymentID = payment["id"].(string)
		}
	}
	return r, want
}
func fmtHex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&15]
	}
	return string(out)
}
func offerReadConfirms(classes ...string) []map[string]any {
	base := time.Date(2026, 3, 4, 10, 7, 0, 0, time.UTC)
	out := []map[string]any{}
	for i, class := range classes {
		actor := offerReadRenter
		if i%2 == 1 {
			actor = offerReadHost
		}
		c := map[string]any{"actor": actor, "class": class, "at": base.Add(time.Duration(i) * time.Minute)}
		if class == "contrato" {
			c["version"] = 2
			c["digest"] = func() string {
				d := sha256.Sum256([]byte("Contrato fijo de prueba independiente."))
				return fmtHex(d[:])
			}()
		}
		out = append(out, c)
	}
	return out
}
func assertOfferReadZero(t *testing.T, got earlyRentalStoredOffer) {
	t.Helper()
	if !reflect.DeepEqual(got, earlyRentalStoredOffer{}) {
		t.Fatalf("expected zero result, got %#v", got)
	}
}
func assertOfferReadLockOrder(t *testing.T, f *offerReadFakeTx) {
	t.Helper()
	want := []string{offerReadHost, offerReadRenter}
	sort.Strings(want)
	if !reflect.DeepEqual(f.lockIDs, want) {
		t.Fatalf("lock IDs=%v want sorted %v", f.lockIDs, want)
	}
}

func TestReadEarlyRentalOfferRehydratesInitialOffer(t *testing.T) {
	row, want := offerReadFixture(earlyrental.StatePendingIdentity, 1, nil, nil)
	f := &offerReadFakeTx{row: row}
	if err := earlyrental.ValidateOffer(want); err != nil {
		t.Fatalf("independent expected offer invalid: %v", err)
	}
	got, err := readEarlyRentalOffer(context.Background(), f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID))
	if err != nil {
		t.Fatal(err)
	}
	wantStored := earlyRentalStoredOffer{Offer: want, CreatedAt: row.created.UTC(), UpdatedAt: row.updated.UTC(), LastEventSequence: 1}
	if !reflect.DeepEqual(got, wantStored) {
		t.Fatalf("rehydrated offer mismatch\n got: %#v\nwant: %#v", got, wantStored)
	}
	if len(f.calls) != 4 || f.calls[0] != "query:"+offerReadHeaderSQLOracle || f.calls[3] != "query:"+offerReadFinalSQLOracle {
		t.Fatalf("unexpected query sequence: %#v", f.calls)
	}
	assertOfferReadLockOrder(t, f)
}

func TestReadEarlyRentalOfferRehydratesPartialIdentityAndTerminalPayment(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        earlyrental.State
		paymentState string
	}{
		{"canceled sent payment", earlyrental.StateCancelled, "anulado"},
		{"expired sent payment", earlyrental.StateExpired, "vencido"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sent := time.Date(2026, 3, 4, 10, 11, 0, 0, time.UTC)
			payment := map[string]any{"id": offerReadPayID, "sender": offerReadRenter, "receiver": offerReadHost, "amount": int64(25000), "currency": "CLP", "state": tc.paymentState, "sent": sent}
			row, want := offerReadFixture(tc.state, 7, offerReadConfirms("identidad", "identidad", "contrato", "contrato"), payment)
			want.PaymentID = offerReadPayID // Sending establishes identity, even after a terminal transition.
			if tc.state == earlyrental.StateExpired {
				row.updated = row.expires.Add(time.Second)
			}
			if err := earlyrental.ValidateOffer(want); err != nil {
				t.Fatal("invalid independent terminal control", err)
			}
			f := &offerReadFakeTx{row: row}
			got, err := readEarlyRentalOffer(context.Background(), f, uuid.MustParse(offerReadHost), uuid.MustParse(offerReadID))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, earlyRentalStoredOffer{Offer: want, CreatedAt: row.created.UTC(), UpdatedAt: row.updated.UTC(), LastEventSequence: 7}) {
				t.Fatalf("terminal read mismatch: %#v", got)
			}
			assertOfferReadLockOrder(t, f)
		})
	}
}

func TestReadEarlyRentalOfferRejectsInvalidInputsBeforeDatabase(t *testing.T) {
	id := uuid.MustParse(offerReadID)
	cases := []struct {
		name         string
		ctx          context.Context
		actor, offer uuid.UUID
		want         error
	}{
		{"nil context", nil, uuid.MustParse(offerReadRenter), id, earlyrental.ErrInvalid},
		{"nil actor", context.Background(), uuid.Nil, id, earlyrental.ErrInvalid},
		{"nil offer", context.Background(), uuid.MustParse(offerReadRenter), uuid.Nil, earlyrental.ErrInvalid},
		{"cancelled context", func() context.Context { c, x := context.WithCancel(context.Background()); x(); return c }(), uuid.MustParse(offerReadRenter), id, context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &offerReadFakeTx{}
			got, err := readEarlyRentalOffer(tc.ctx, f, tc.actor, tc.offer)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			assertOfferReadZero(t, got)
			if len(f.calls) != 0 {
				t.Fatalf("invalid input issued calls: %#v", f.calls)
			}
		})
	}
}
func TestReadEarlyRentalOfferNotFoundAndErrorsReturnZero(t *testing.T) {
	sentinel := errors.New("sentinel query failure")
	cases := []struct {
		name  string
		f     *offerReadFakeTx
		want  error
		locks int
	}{
		{"header absent", &offerReadFakeTx{headerNoRows: true}, rentalflow.ErrNotFound, 0},
		{"header scan failure", &offerReadFakeTx{headerErr: sentinel}, sentinel, 0},
		{"first account lock failure", &offerReadFakeTx{lockErrAt: 1, finalErr: sentinel}, sentinel, 1},
		{"second account lock failure", &offerReadFakeTx{lockErrAt: 2, finalErr: sentinel}, sentinel, 2},
		{"final absent", &offerReadFakeTx{finalNoRows: true}, rentalflow.ErrNotFound, 2},
		{"final scan failure", &offerReadFakeTx{finalErr: sentinel}, sentinel, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readEarlyRentalOffer(context.Background(), tc.f, uuid.MustParse(offerReadRenter), uuid.MustParse(offerReadID))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			assertOfferReadZero(t, got)
			if len(tc.f.lockIDs) != tc.locks {
				t.Fatalf("locks=%d want=%d", len(tc.f.lockIDs), tc.locks)
			}
			finalExpected := tc.f.lockErrAt == 0 && (tc.f.finalNoRows || tc.f.finalErr != nil)
			if finalExpected && (len(tc.f.calls) == 0 || tc.f.calls[len(tc.f.calls)-1] != "query:"+offerReadFinalSQLOracle) {
				t.Fatalf("missing final query: %#v", tc.f.calls)
			}
			if tc.f.lockErrAt > 0 && len(tc.f.calls) != 1+tc.f.lockErrAt {
				t.Fatalf("queries continued after failed lock: %#v", tc.f.calls)
			}
		})
	}
}

func TestReadEarlyRentalOfferMinTime(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := a.Add(time.Second)
	if got := minTime(a, b); !got.Equal(a) {
		t.Fatalf("minTime=%v", got)
	}
	if got := minTime(b, a); !got.Equal(a) {
		t.Fatalf("minTime=%v", got)
	}
}
