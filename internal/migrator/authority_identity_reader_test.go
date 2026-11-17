package migrator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type authorityIdentityTestTx struct{ pgx.Tx }
type authorityIdentityTestRow struct{}

func (authorityIdentityTestRow) Scan(dest ...any) error {
	*(dest[0].(*bool)) = false
	*(dest[1].(*string)) = "read committed"
	*(dest[2].(*int)) = 180000
	*(dest[3].(*string)) = "pg_catalog"
	return nil
}
func (authorityIdentityTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return authorityIdentityTestRow{}
}

func TestAuthorityIdentityReaderRejectsInvalidScope(t *testing.T) {
	tests := []struct {
		name  string
		scope AuthorityIdentityScope
	}{
		{"empty", AuthorityIdentityScope{}},
		{"duplicate role", AuthorityIdentityScope{Roles: []string{"app", "app"}}},
		{"blank role", AuthorityIdentityScope{Roles: []string{" "}}},
		{"unknown class", AuthorityIdentityScope{Objects: []AuthorityObjectRef{{Class: "table", Identity: "public.t"}}}},
		{"blank object", AuthorityIdentityScope{Objects: []AuthorityObjectRef{{Class: "schema", Identity: ""}}}},
		{"duplicate object", AuthorityIdentityScope{Objects: []AuthorityObjectRef{{Class: "schema", Identity: "public"}, {Class: "schema", Identity: "public"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CaptureSingleOwnerAuthorityIdentity(context.Background(), authorityIdentityTestTx{}, tt.scope)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if got.Roles != nil || got.Objects != nil || got.Memberships != nil {
				t.Fatalf("expected zero snapshot on error, got %#v", got)
			}
		})
	}
}

func TestAuthorityIdentityReaderRejectsUnsafeTransactionGuard(t *testing.T) {
	_, err := CaptureSingleOwnerAuthorityIdentity(context.Background(), authorityIdentityTestTx{}, AuthorityIdentityScope{Objects: []AuthorityObjectRef{{Class: "function", Identity: "public.f(integer)"}}})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected exact transaction guard rejection: %v", err)
	}
}

func TestAuthorityIdentityReaderReadsCatalogAndDropsPartialErrors(t *testing.T) {
	for _, failure := range []string{"", "roles query", "roles iterate", "object", "members query", "members iterate"} {
		t.Run(failure, func(t *testing.T) {
			q := &authorityIdentityCatalogTx{failure: failure}
			got, err := CaptureSingleOwnerAuthorityIdentity(context.Background(), q, authorityIdentityCatalogScope())
			if failure != "" {
				if err == nil || !reflect.DeepEqual(got, AuthorityIdentitySnapshot{}) {
					t.Fatalf("partial catalog leaked after %s: %+v %v", failure, got, err)
				}
				return
			}
			want := AuthorityIdentitySnapshot{Roles: []AuthorityRole{{Name: "app", Inherit: true}}, Objects: []AuthorityObject{{Class: "schema", Identity: "public", Owner: "owner"}}, Memberships: []AuthorityMembership{{Role: "group", Member: "app", SetOption: true}}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("valid catalog projection: %+v %v", got, err)
			}
		})
	}
}

func TestAuthorityIdentityReaderRejectsCanceledBeforeCatalog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &authorityIdentityCatalogTx{}
	got, err := CaptureSingleOwnerAuthorityIdentity(ctx, q, authorityIdentityCatalogScope())
	if err == nil || q.calls != 0 || !reflect.DeepEqual(got, AuthorityIdentitySnapshot{}) {
		t.Fatalf("canceled context reached catalog: calls=%d got=%+v err=%v", q.calls, got, err)
	}
}

func TestAuthorityIdentityReaderWithholdsCancellationAfterFinalRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &authorityIdentityCatalogTx{cancel: cancel}
	got, err := CaptureSingleOwnerAuthorityIdentity(ctx, q, authorityIdentityCatalogScope())
	if err == nil || !reflect.DeepEqual(got, AuthorityIdentitySnapshot{}) {
		t.Fatalf("final-read cancellation leaked snapshot: %+v %v", got, err)
	}
}

func authorityIdentityCatalogScope() AuthorityIdentityScope {
	return AuthorityIdentityScope{Roles: []string{"app"}, Objects: []AuthorityObjectRef{{Class: "schema", Identity: "public"}}}
}

type authorityIdentityCatalogTx struct {
	pgx.Tx
	failure string
	calls   int
	cancel  context.CancelFunc
}

func (q *authorityIdentityCatalogTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.calls++
	if strings.Contains(sql, "current_setting") {
		return authorityIdentityValueRow{values: []any{true, "repeatable read", 180000, "pg_catalog"}}
	}
	if len(args) != 1 || args[0] != "public" {
		return authorityIdentityValueRow{err: errors.New("unexpected object scope")}
	}
	if q.failure == "object" {
		return authorityIdentityValueRow{err: errors.New("synthetic object read error")}
	}
	return authorityIdentityValueRow{values: []any{"public", "owner"}}
}
func (q *authorityIdentityCatalogTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.calls++
	if len(args) != 1 || !reflect.DeepEqual(args[0], []string{"app"}) {
		return nil, errors.New("unexpected role scope")
	}
	if strings.Contains(sql, "pg_auth_members") {
		if q.failure == "members query" {
			return nil, errors.New("synthetic membership query error")
		}
		r := &authorityIdentityCatalogRows{data: [][]any{{"group", "app", false, false, true}}, close: q.cancel}
		if q.failure == "members iterate" {
			r.err = errors.New("synthetic membership iteration error")
		}
		return r, nil
	}
	if q.failure == "roles query" {
		return nil, errors.New("synthetic roles query error")
	}
	r := &authorityIdentityCatalogRows{data: [][]any{{"app", false, false, false, false, false, false, true}}}
	if q.failure == "roles iterate" {
		r.err = errors.New("synthetic role iteration error")
	}
	return r, nil
}

type authorityIdentityValueRow struct {
	values []any
	err    error
}

func (r authorityIdentityValueRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("unexpected scan shape: %d", len(dest))
	}
	for i, v := range r.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}

type authorityIdentityCatalogRows struct {
	pgx.Rows
	data  [][]any
	index int
	err   error
	close context.CancelFunc
}

func (r *authorityIdentityCatalogRows) Next() bool { r.index++; return r.index <= len(r.data) }
func (r *authorityIdentityCatalogRows) Scan(dest ...any) error {
	return (authorityIdentityValueRow{values: r.data[r.index-1]}).Scan(dest...)
}
func (r *authorityIdentityCatalogRows) Err() error { return r.err }
func (r *authorityIdentityCatalogRows) Close() {
	if r.close != nil {
		r.close()
		r.close = nil
	}
}
