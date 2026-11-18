package rentalflowpg

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/earlyrental"
	"github.com/google/uuid"
)

func TestPostgresEarlyRentalListTransactions(t *testing.T) {
	ctx, dbURL := earlyInsertPostgresFixture(t)
	dsn := dbURL.String()
	grantEarlyOfferReadColumns(t, ctx, dsn)
	acl := connectEarlyReadAdmin(t, ctx, dsn)
	if _, err := acl.Exec(ctx, `GRANT UPDATE (activo,desactivada_en) ON TABLE public.ocupacion TO espacigo_runtime`); err != nil {
		t.Fatal(err)
	}
	var occupancyUpdate bool
	if err := acl.QueryRow(ctx, `SELECT has_column_privilege('espacigo_runtime','public.ocupacion','activo','UPDATE') AND has_column_privilege('espacigo_runtime','public.ocupacion','desactivada_en','UPDATE')`).Scan(&occupancyUpdate); err != nil || !occupancyUpdate {
		t.Fatalf("runtime occupancy-release grants: %v", err)
	}
	ids := earlyInsertIDs()
	seedEarlyInsertActors(t, ctx, dsn, ids)
	outsider := uuid.New()
	blockedRenter := uuid.MustParse(ids.renter)
	insertEarlyReadActor(t, ctx, dsn, outsider, "outsider")
	createdAt := time.Date(2030, 8, 9, 10, 11, 12, 123456000, time.UTC)

	// Persist three offers sharing one participant pair and timestamp: one
	// canceled after payment was sent, one sent, and one with no payment.
	type fixture struct {
		record earlyRentalInitialRecord
		ids    earlyInsertFixtureIDs
		state  string
	}
	fixtures := make([]fixture, 0, 3)
	for i, state := range []string{"anulado", "enviado", "sin_pago"} {
		caseIDs := ids
		if i > 0 {
			caseIDs.offer, caseIDs.space, caseIDs.occupancy = uuid.NewString(), uuid.NewString(), uuid.NewString()
			seedEarlyReadSpace(t, ctx, dsn, caseIDs)
		}
		record := earlyInsertRecord(t, caseIDs, createdAt)
		commitEarlyReadFixtureOffer(t, ctx, dsn, record)
		fixtures = append(fixtures, fixture{record: record, ids: caseIDs, state: state})
	}
	fixtures[0].record.Offer = advanceEarlyTransactionFixture(t, ctx, dsn, fixtures[0].ids, fixtures[0].record.Offer, true, createdAt)
	fixtures[1].record.Offer = advanceEarlyTransactionFixture(t, ctx, dsn, fixtures[1].ids, fixtures[1].record.Offer, false, createdAt)

	before := earlyOfferReadCounts(t, ctx, dsn)
	var paymentCount int64
	if err := acl.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_pago`).Scan(&paymentCount); err != nil {
		t.Fatal(err)
	}

	wantPaymentIDs := []uuid.UUID{uuid.MustParse(fixtures[0].record.Offer.PaymentID), uuid.MustParse(fixtures[1].record.Offer.PaymentID)}
	sort.Slice(wantPaymentIDs, func(i, j int) bool { return wantPaymentIDs[i].String() > wantPaymentIDs[j].String() })

	t.Run("BothParticipants", func(t *testing.T) {
		for _, actor := range []uuid.UUID{uuid.MustParse(ids.renter), uuid.MustParse(ids.host)} {
			result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: actor, Limit: 10})
			if len(result.Rows) != 2 || result.Next != nil {
				t.Fatalf("actor %s got %d transactions, next=%v", actor, len(result.Rows), result.Next)
			}
			assertTransactionPaymentIDs(t, result.Rows, wantPaymentIDs)
			for _, row := range result.Rows {
				if row.Offer.PaymentSentAt == nil || row.Offer.PaymentID == "" {
					t.Fatalf("transaction lacks persisted sent evidence: %+v", row.Offer)
				}
				foundSnapshot := false
				for _, fixture := range fixtures {
					if row.Offer.ID == fixture.record.Offer.ID {
						foundSnapshot = row.Offer.Snapshot == fixture.record.Offer.Snapshot
						break
					}
				}
				if !foundSnapshot {
					t.Fatalf("transaction %s did not retain its persisted offer snapshot", row.Offer.ID)
				}
			}
		}
	})

	t.Run("OutsiderHidden", func(t *testing.T) {
		result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: outsider, Limit: 10})
		if len(result.Rows) != 0 || result.Next != nil {
			t.Fatalf("outsider received transactions: %+v", result)
		}
	})

	t.Run("InactiveActorHidden", func(t *testing.T) {
		if _, err := acl.Exec(ctx, `UPDATE public.usuario SET estado='bloqueado' WHERE id=$1`, blockedRenter); err != nil {
			t.Fatal(err)
		}
		result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: blockedRenter, Limit: 10})
		if len(result.Rows) != 0 || result.Next != nil {
			t.Fatalf("inactive participant received transactions: %+v", result)
		}
		if _, err := acl.Exec(ctx, `UPDATE public.usuario SET estado='activo' WHERE id=$1`, blockedRenter); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("BlockedParticipantHidden", func(t *testing.T) {
		if _, err := acl.Exec(ctx, `INSERT INTO public.bloqueo_cuenta_administrativo_local(cuenta_id,bloqueada_por,motivo_codigo,bloqueada_en,correlacion_id) VALUES($1,$2,'revision_administrativa',$3,'synthetic-transaction-list-block')`, blockedRenter, uuid.MustParse(ids.host), createdAt); err != nil {
			t.Fatal(err)
		}
		result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: blockedRenter, Limit: 10})
		if len(result.Rows) != 0 || result.Next != nil {
			t.Fatalf("blocked participant received transactions: %+v", result)
		}
		if _, err := acl.Exec(ctx, `DELETE FROM public.bloqueo_cuenta_administrativo_local WHERE cuenta_id=$1`, blockedRenter); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("RoleFiltering", func(t *testing.T) {
		for _, tc := range []struct {
			actor uuid.UUID
			role  string
			want  int
		}{{uuid.MustParse(ids.renter), "renter", 2}, {uuid.MustParse(ids.host), "host", 2}, {uuid.MustParse(ids.renter), "host", 0}} {
			result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: tc.actor, Role: tc.role, Limit: 10})
			if len(result.Rows) != tc.want {
				t.Fatalf("role %s rows=%d, want %d", tc.role, len(result.Rows), tc.want)
			}
		}
	})

	t.Run("StateFiltering", func(t *testing.T) {
		for _, tc := range []struct {
			state string
			want  []uuid.UUID
		}{{"anulado", []uuid.UUID{uuid.MustParse(fixtures[0].record.Offer.PaymentID)}}, {"enviado", []uuid.UUID{uuid.MustParse(fixtures[1].record.Offer.PaymentID)}}, {"recibido", nil}, {"vencido", nil}} {
			result := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: uuid.MustParse(ids.renter), State: tc.state, Limit: 10})
			if len(result.Rows) != len(tc.want) {
				t.Fatalf("state %s rows=%d, want %d", tc.state, len(result.Rows), len(tc.want))
			}
			assertTransactionPaymentIDs(t, result.Rows, tc.want)
		}
	})

	t.Run("StableKeysetPages", func(t *testing.T) {
		first := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: uuid.MustParse(ids.renter), Limit: 1})
		if len(first.Rows) != 1 || first.Next == nil || first.Next.PaymentID != wantPaymentIDs[0] || !first.Next.CreatedAt.Equal(createdAt) {
			t.Fatalf("first page/cursor mismatch: %+v", first)
		}
		assertTransactionPaymentIDs(t, first.Rows, wantPaymentIDs[:1])
		second := listEarlyTransactionsInReadTx(t, ctx, dsn, EarlyRentalTransactionListInput{ActorID: uuid.MustParse(ids.renter), Limit: 1, After: first.Next})
		if len(second.Rows) != 1 || second.Next != nil {
			t.Fatalf("second page mismatch: %+v", second)
		}
		assertTransactionPaymentIDs(t, second.Rows, wantPaymentIDs[1:])
	})

	t.Run("ReadOnlyNoWrites", func(t *testing.T) {
		if after := earlyOfferReadCounts(t, ctx, dsn); after != before {
			t.Fatalf("transaction list changed aggregate counts: %v -> %v", before, after)
		}
		var afterPayments int64
		if err := acl.QueryRow(ctx, `SELECT count(*) FROM public.arriendo_temprano_pago`).Scan(&afterPayments); err != nil || afterPayments != paymentCount {
			t.Fatalf("transaction list changed payment count: %d -> %d (%v)", paymentCount, afterPayments, err)
		}
	})
}

func advanceEarlyTransactionFixture(t *testing.T, ctx context.Context, dsn string, ids earlyInsertFixtureIDs, initial earlyrental.Offer, cancel bool, now time.Time) earlyrental.Offer {
	t.Helper()
	want := initial
	steps := []struct {
		actor string
		cmd   earlyrental.Command
	}{
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandConfirmIdentity}},
		{ids.host, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: want.ContractVersion, ContractDigest: want.ContractDigest}},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandAcceptContract, ContractVersion: want.ContractVersion, ContractDigest: want.ContractDigest}},
		{ids.renter, earlyrental.Command{Kind: earlyrental.CommandSendPayment, PaymentID: uuid.NewString()}},
	}
	if cancel {
		steps = append(steps, struct {
			actor string
			cmd   earlyrental.Command
		}{ids.host, earlyrental.Command{Kind: earlyrental.CommandCancel, ReasonCode: "cambio_de_planes"}})
	}
	for i, step := range steps {
		step.cmd.ExpectedRevision = want.Revision
		at := now.Add(time.Duration(i+1) * time.Minute)
		next, event, err := earlyrental.Apply(want, step.actor, step.cmd, at)
		if err != nil {
			t.Fatalf("derive transaction lifecycle %s: %v", step.cmd.Kind, err)
		}
		applyCanceledSentPaymentStep(t, ctx, dsn, ids, want, next, event, at)
		want = next
	}
	return want
}

func listEarlyTransactionsInReadTx(t *testing.T, ctx context.Context, dsn string, in EarlyRentalTransactionListInput) EarlyRentalTransactionListResult {
	t.Helper()
	_, tx := createEarlyOfferReadTx(t, ctx, dsn)
	got, err := listEarlyRentalTransactions(ctx, tx, in)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertTransactionPaymentIDs(t *testing.T, rows []EarlyRentalTransactionListRow, want []uuid.UUID) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("rows=%d want %d", len(rows), len(want))
	}
	for i, id := range want {
		if rows[i].Offer.PaymentID != id.String() {
			t.Fatalf("row %d payment_id=%s want %s", i, rows[i].Offer.PaymentID, id)
		}
	}
}
