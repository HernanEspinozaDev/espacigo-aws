package migrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAuthorityDefaultACLReaderCapturesScopedRows(t *testing.T) {
	q := &defaultACLTestTx{data: [][]any{{"owner", "public", "relation", "owner", "PUBLIC", "SELECT", true, "explicit"}, {"owner", "", "function", "", "", "", false, "empty"}}}
	got, err := CaptureScopedAuthorityDefaultACLs(context.Background(), q, []string{"owner"})
	want := AuthorityDefaultACLObservation{ACLs: []AuthorityDefaultACL{{Owner: "owner", Grantor: "owner", Schema: "public", Class: "relation", Grantee: "PUBLIC", Privilege: "SELECT", GrantOption: true}}, States: []AuthorityDefaultACLState{{Owner: "owner", Schema: "public", Class: "relation", State: "explicit"}, {Owner: "owner", Schema: "", Class: "function", State: "empty"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if q.closed != 1 {
		t.Fatalf("rows close=%d", q.closed)
	}
}
func TestAuthorityDefaultACLReaderRejectsBadOwners(t *testing.T) {
	for _, owners := range [][]string{nil, {""}, {"a", "a"}, {"bad\x00name"}, {"bad\xff"}} {
		q := &defaultACLTestTx{}
		got, err := CaptureScopedAuthorityDefaultACLs(context.Background(), q, owners)
		if err == nil || !reflect.DeepEqual(got, AuthorityDefaultACLObservation{}) || q.calls != 0 {
			t.Fatalf("owners %q got=%+v err=%v calls=%d", owners, got, err, q.calls)
		}
	}
}
func TestAuthorityDefaultACLReaderWithholdsReadErrors(t *testing.T) {
	for _, failure := range []string{"query", "scan", "iterate", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			q := &defaultACLTestTx{failure: failure}
			if failure == "canceled" {
				q.cancel = cancel
			}
			got, err := CaptureScopedAuthorityDefaultACLs(ctx, q, []string{"owner"})
			if err == nil || !reflect.DeepEqual(got, AuthorityDefaultACLObservation{}) {
				t.Fatalf("partial observation: %+v %v", got, err)
			}
			wantClosed := 1
			if failure == "query" {
				wantClosed = 0
			}
			if q.closed != wantClosed {
				t.Fatalf("rows close=%d", q.closed)
			}
		})
	}
}

type defaultACLTestTx struct {
	pgx.Tx
	calls, closed int
	data          [][]any
	failure       string
	cancel        context.CancelFunc
}

func (q *defaultACLTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return authorityIdentityValueRow{values: []any{true, "repeatable read", 180000, "pg_catalog"}}
}
func (q *defaultACLTestTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.calls++
	if strings.Contains(sql, "pg_roles r WHERE") {
		return &authorityIdentityCatalogRows{data: [][]any{{"owner", false, false, false, false, false, false, true}}}, nil
	}
	if strings.Contains(sql, "pg_auth_members") {
		return &authorityIdentityCatalogRows{}, nil
	}
	if !strings.Contains(sql, "pg_default_acl") || len(args) != 1 || !reflect.DeepEqual(args[0], []string{"owner"}) {
		return nil, errors.New("unexpected defaults query")
	}
	if q.failure == "query" {
		return nil, errors.New("query failed")
	}
	data := q.data
	if data == nil {
		data = [][]any{{"owner", "", "function", "owner", "reader", "EXECUTE", false, "explicit"}}
	}
	r := &defaultACLTestRows{authorityIdentityCatalogRows: authorityIdentityCatalogRows{data: data}, q: q}
	if q.failure == "iterate" {
		r.err = errors.New("iteration failed")
	}
	return r, nil
}

type defaultACLTestRows struct {
	authorityIdentityCatalogRows
	q *defaultACLTestTx
}

func (r *defaultACLTestRows) Scan(dest ...any) error {
	if r.q.failure == "scan" {
		return errors.New("scan failed")
	}
	return r.authorityIdentityCatalogRows.Scan(dest...)
}
func (r *defaultACLTestRows) Close() {
	r.q.closed++
	if r.q.cancel != nil {
		r.q.cancel()
	}
}
