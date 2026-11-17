package migrator

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

// AuthorityRole contains the complete set of sensitive role attributes.
type AuthorityRole struct {
	Name                                                                    string
	Superuser, CreateDB, CreateRole, Replication, BypassRLS, Login, Inherit bool
}

// AuthorityObject is keyed by catalog class and full identity; functions include signatures.
type AuthorityObject struct{ Class, Identity, Owner string }

// AuthorityMembership records granted role, receiving member, and all edge options.
type AuthorityMembership struct {
	Role, Member                          string
	AdminOption, InheritOption, SetOption bool
}

// AuthorityACL records a direct ACL tuple, with PUBLIC spelled exactly "PUBLIC".
type AuthorityACL struct {
	Class, ObjectIdentity, Column, Grantor, Grantee, Privilege string
	GrantOption                                                bool
}

// AuthorityDefaultACL records owner/schema/object-class-scoped default ACLs.
type AuthorityDefaultACL struct {
	Owner, Grantor, Schema, Class, Grantee, Privilege string
	GrantOption                                       bool
}

// AuthorityPrivilegeProbe is an expected effective privilege result.
type AuthorityPrivilegeProbe struct {
	Role, ObjectClass, ObjectIdentity, Column, Privilege string
	Granted                                              bool
}

// SingleOwnerAuthoritySnapshot requires explicit (possibly empty) slices for every inventory.
// It models data only: no catalog coverage, reader, preflight, or operational authority is implied.
type SingleOwnerAuthoritySnapshot struct {
	Roles               []AuthorityRole
	Objects             []AuthorityObject
	Memberships         []AuthorityMembership
	ACLs                []AuthorityACL
	DefaultACLs         []AuthorityDefaultACL
	EffectivePrivileges []AuthorityPrivilegeProbe
}

// CompareSingleOwnerAuthorityContract compares exact tuple multisets, without mutating either input.
// expected must be an independently reviewed trusted contract; it is never learned from actual.
func CompareSingleOwnerAuthorityContract(expected, actual *SingleOwnerAuthoritySnapshot) error {
	if err := validateAuthoritySnapshot("expected", expected); err != nil {
		return err
	}
	if err := validateAuthoritySnapshot("actual", actual); err != nil {
		return err
	}
	if err := compareAuthoritySet("roles", expected.Roles, actual.Roles); err != nil {
		return err
	}
	if err := compareAuthoritySet("objects", expected.Objects, actual.Objects); err != nil {
		return err
	}
	if err := compareAuthoritySet("memberships", expected.Memberships, actual.Memberships); err != nil {
		return err
	}
	if err := compareAuthoritySet("direct ACLs", expected.ACLs, actual.ACLs); err != nil {
		return err
	}
	if err := compareAuthoritySet("default ACLs", expected.DefaultACLs, actual.DefaultACLs); err != nil {
		return err
	}
	return compareAuthoritySet("effective privilege probes", expected.EffectivePrivileges, actual.EffectivePrivileges)
}

func validateAuthoritySnapshot(label string, s *SingleOwnerAuthoritySnapshot) error {
	if s == nil {
		return fmt.Errorf("%s authority snapshot is nil", label)
	}
	if s.Roles == nil || s.Objects == nil || s.Memberships == nil || s.ACLs == nil || s.DefaultACLs == nil || s.EffectivePrivileges == nil {
		return fmt.Errorf("%s authority snapshot omits an inventory collection", label)
	}
	if len(s.Roles) == 0 || len(s.Objects) == 0 || len(s.EffectivePrivileges) == 0 {
		return fmt.Errorf("%s authority contract requires roles, owned objects and effective privilege probes", label)
	}
	if err := rejectInvalidAuthorityStrings(reflect.ValueOf(s), label); err != nil {
		return err
	}
	roleIDs := map[string]bool{}
	for _, r := range s.Roles {
		if strings.TrimSpace(r.Name) == "" || roleIDs[r.Name] {
			return fmt.Errorf("%s authority roles contain an empty or duplicate identity", label)
		}
		roleIDs[r.Name] = true
	}
	objectIDs := map[objectIdentity]bool{}
	for _, o := range s.Objects {
		if strings.TrimSpace(o.Class) == "" || strings.TrimSpace(o.Identity) == "" || strings.TrimSpace(o.Owner) == "" {
			return fmt.Errorf("%s authority object has incomplete identity or owner", label)
		}
		if o.Class == "function" && (!strings.Contains(o.Identity, "(") || !strings.HasSuffix(o.Identity, ")")) {
			return fmt.Errorf("%s function identity must include its complete signature", label)
		}
		id := objectIdentity{o.Class, o.Identity}
		if objectIDs[id] {
			return fmt.Errorf("%s authority objects contain duplicate identity", label)
		}
		objectIDs[id] = true
	}
	membershipIDs := map[membershipIdentity]bool{}
	for _, m := range s.Memberships {
		if m.Role == "" || m.Member == "" || m.Role == m.Member {
			return fmt.Errorf("%s membership identity is incomplete", label)
		}
		id := membershipIdentity{m.Role, m.Member}
		if membershipIDs[id] {
			return fmt.Errorf("%s memberships contain duplicate role/member identity", label)
		}
		membershipIDs[id] = true
	}
	aclIDs := map[aclIdentity]bool{}
	for _, a := range s.ACLs {
		if a.Class == "" || a.ObjectIdentity == "" || a.Grantor == "" || a.Grantee == "" || a.Privilege == "" {
			return fmt.Errorf("%s direct ACL tuple is incomplete", label)
		}
		id := aclIdentity{a.Class, a.ObjectIdentity, a.Column, a.Grantor, a.Grantee, a.Privilege}
		if aclIDs[id] {
			return fmt.Errorf("%s direct ACLs contain duplicate identity", label)
		}
		aclIDs[id] = true
	}
	defaultIDs := map[defaultACLIdentity]bool{}
	for _, a := range s.DefaultACLs {
		if a.Owner == "" || a.Grantor == "" || a.Class == "" || a.Grantee == "" || a.Privilege == "" {
			return fmt.Errorf("%s default ACL tuple is incomplete", label)
		}
		id := defaultACLIdentity{a.Owner, a.Schema, a.Class, a.Grantee, a.Privilege}
		if defaultIDs[id] {
			return fmt.Errorf("%s default ACLs contain duplicate identity", label)
		}
		defaultIDs[id] = true
	}
	probeIDs := map[probeIdentity]bool{}
	for _, p := range s.EffectivePrivileges {
		if p.Role == "" || p.ObjectClass == "" || p.ObjectIdentity == "" || p.Privilege == "" {
			return fmt.Errorf("%s effective privilege probe is incomplete", label)
		}
		id := probeIdentity{p.Role, p.ObjectClass, p.ObjectIdentity, p.Column, p.Privilege}
		if probeIDs[id] {
			return fmt.Errorf("%s effective probes contain duplicate identity", label)
		}
		probeIDs[id] = true
	}
	return nil
}

type objectIdentity struct{ class, identity string }
type membershipIdentity struct{ role, member string }
type aclIdentity struct{ class, object, column, grantor, grantee, privilege string }
type defaultACLIdentity struct{ owner, schema, class, grantee, privilege string }
type probeIdentity struct{ role, class, object, column, privilege string }

func rejectInvalidAuthorityStrings(v reflect.Value, label string) error {
	if v.Kind() == reflect.String {
		s := v.String()
		if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
			return fmt.Errorf("%s authority snapshot contains invalid UTF-8 or NUL", label)
		}
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return rejectInvalidAuthorityStrings(v.Elem(), label)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := rejectInvalidAuthorityStrings(v.Field(i), label); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := rejectInvalidAuthorityStrings(v.Index(i), label); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareAuthoritySet[T comparable](label string, expected, actual []T) error {
	counts := make(map[T]int, len(expected))
	for _, row := range expected {
		counts[row]++
	}
	seen := make(map[T]int, len(actual))
	for _, row := range actual {
		seen[row]++
	}
	if len(counts) != len(seen) {
		return fmt.Errorf("%s differ: expected %d tuples, got %d", label, len(counts), len(seen))
	}
	for tuple, n := range counts {
		if seen[tuple] != n {
			return fmt.Errorf("%s differ: missing or altered tuple", label)
		}
	}
	return nil
}
