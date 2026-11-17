package migrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAuthorityACLReaderCapturesScopedSchema(t *testing.T) {
	q := &authorityACLTestTx{data: [][]any{{"", "null", "owner", "PUBLIC", "USAGE", false}}}
	got, err := CaptureScopedAuthorityACLs(context.Background(), q, []AuthorityObjectRef{{Class: "schema", Identity: "public"}})
	want := AuthorityACLObservation{
		Objects:   []AuthorityObject{{Class: "schema", Identity: "public", Owner: "owner"}},
		ACLs:      []AuthorityACL{{Class: "schema", ObjectIdentity: "public", Grantor: "owner", Grantee: "PUBLIC", Privilege: "USAGE"}},
		ACLStates: []AuthorityACLState{{Class: "schema", ObjectIdentity: "public", State: "null"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped ACL observation missing or changed: %+v %v", got, err)
	}
	if q.aclCalls != 1 || q.closed != 1 {
		t.Fatalf("ACL rows were not consumed and closed: calls=%d closed=%d", q.aclCalls, q.closed)
	}
}

func TestAuthorityACLReaderRejectsIncompleteCatalog(t *testing.T) {
	for name, data := range map[string][][]any{
		"missing metadata":      nil,
		"unknown raw state":     {{"", "unknown", "", "", "", false}},
		"conflicting raw state": {{"", "null", "owner", "PUBLIC", "USAGE", false}, {"", "empty", "", "", "", false}},
		"partial tuple":         {{"", "explicit", "owner", "", "", false}},
		"duplicate tuple":       {{"", "explicit", "owner", "PUBLIC", "USAGE", false}, {"", "explicit", "owner", "PUBLIC", "USAGE", true}},
	} {
		t.Run(name, func(t *testing.T) {
			q := &authorityACLTestTx{data: data}
			got, err := CaptureScopedAuthorityACLs(context.Background(), q, []AuthorityObjectRef{{Class: "schema", Identity: "public"}})
			if err == nil || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
				t.Fatalf("incomplete ACL catalog accepted: %+v %v", got, err)
			}
			if q.closed != 1 {
				t.Fatalf("ACL rows not closed: %d", q.closed)
			}
		})
	}
}

func TestAuthorityACLReaderStopsAfterCanceledObjectRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &authorityACLTestTx{data: [][]any{{"", "null", "owner", "PUBLIC", "USAGE", false}}, cancel: cancel}
	got, err := CaptureScopedAuthorityACLs(ctx, q, []AuthorityObjectRef{{Class: "schema", Identity: "public"}, {Class: "schema", Identity: "other"}})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, AuthorityACLObservation{}) || q.aclCalls != 1 {
		t.Fatalf("cancellation did not stop the next catalog read: %+v %v calls=%d", got, err, q.aclCalls)
	}
}

func TestAuthorityACLReaderWithholdsDependencyFailures(t *testing.T) {
	for _, failure := range []string{"query", "scan", "iterate", "cancel-final", "second-object"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			q := &authorityACLTestTx{data: [][]any{{"", "null", "owner", "PUBLIC", "USAGE", false}}, failure: failure}
			objects := []AuthorityObjectRef{{Class: "schema", Identity: "public"}}
			if failure == "cancel-final" {
				q.cancel = cancel
			}
			if failure == "second-object" {
				objects = append(objects, AuthorityObjectRef{Class: "schema", Identity: "other"})
			}
			got, err := CaptureScopedAuthorityACLs(ctx, q, objects)
			if err == nil || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
				t.Fatalf("dependency failure leaked ACLs: %+v %v", got, err)
			}
			if failure == "cancel-final" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if failure != "query" && q.closed != 1 {
				t.Fatalf("rows not closed: %d", q.closed)
			}
		})
	}
}

func TestAuthorityACLReaderRejectsInvalidScopeBeforeCatalog(t *testing.T) {
	ref := AuthorityObjectRef{Class: "schema", Identity: "public"}
	for name, objects := range map[string][]AuthorityObjectRef{
		"empty":         nil,
		"duplicate":     {ref, ref},
		"unknown-class": {{Class: "relation; DROP TABLE t", Identity: "public.t"}},
		"nul":           {{Class: "schema", Identity: "public\x00"}},
		"invalid-utf8":  {{Class: "schema", Identity: "public\xff"}},
		"blank":         {{Class: "schema", Identity: " "}},
	} {
		t.Run(name, func(t *testing.T) {
			q := &authorityACLTestTx{}
			got, err := CaptureScopedAuthorityACLs(context.Background(), q, objects)
			if err == nil || q.calls != 0 || !reflect.DeepEqual(got, AuthorityACLObservation{}) {
				t.Fatalf("invalid scope reached catalog: %+v %v calls=%d", got, err, q.calls)
			}
		})
	}
}

func TestAuthorityACLReaderCapturesExplicitEmptyWithoutMutatingScope(t *testing.T) {
	objects := []AuthorityObjectRef{{Class: "schema", Identity: "public"}}
	before := append([]AuthorityObjectRef(nil), objects...)
	q := &authorityACLTestTx{data: [][]any{{"", "empty", "", "", "", false}}}
	got, err := CaptureScopedAuthorityACLs(context.Background(), q, objects)
	if err != nil || got.ACLs == nil || len(got.ACLs) != 0 || !reflect.DeepEqual(got.ACLStates, []AuthorityACLState{{Class: "schema", ObjectIdentity: "public", State: "empty"}}) {
		t.Fatalf("explicit empty lost: %+v %v", got, err)
	}
	if !reflect.DeepEqual(objects, before) {
		t.Fatal("input scope mutated")
	}
}

type authorityACLTestTx struct {
	pgx.Tx
	data                    [][]any
	failure                 string
	calls, aclCalls, closed int
	cancel                  context.CancelFunc
}

func (q *authorityACLTestTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.calls++
	if strings.Contains(sql, "current_setting") {
		return authorityIdentityValueRow{values: []any{true, "repeatable read", 180000, "pg_catalog"}}
	}
	if len(args) != 1 {
		return authorityIdentityValueRow{err: errors.New("unexpected object args")}
	}
	return authorityIdentityValueRow{values: []any{args[0], "owner"}}
}
func (q *authorityACLTestTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.calls++
	if strings.Contains(sql, "pg_catalog.pg_roles r WHERE") {
		return &authorityIdentityCatalogRows{}, nil
	}
	if !strings.Contains(sql, "aclexplode") || len(args) != 1 || args[0] != "public" {
		return nil, errors.New("unexpected ACL query/scope")
	}
	q.aclCalls++
	if q.failure == "query" {
		return nil, errors.New("synthetic ACL query error")
	}
	r := &authorityACLTestRows{authorityIdentityCatalogRows: authorityIdentityCatalogRows{data: q.data}, q: q}
	if q.failure == "iterate" {
		r.err = errors.New("synthetic ACL iteration error")
	}
	return r, nil
}

type authorityACLTestRows struct {
	authorityIdentityCatalogRows
	q *authorityACLTestTx
}

func (r *authorityACLTestRows) Scan(dest ...any) error {
	if r.q.failure == "scan" {
		return errors.New("synthetic ACL scan error")
	}
	return r.authorityIdentityCatalogRows.Scan(dest...)
}
func (r *authorityACLTestRows) Close() {
	r.q.closed++
	if r.q.cancel != nil {
		r.q.cancel()
	}
}
