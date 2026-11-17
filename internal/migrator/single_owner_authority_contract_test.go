package migrator

import (
	"reflect"
	"strings"
	"testing"
)

func TestSingleOwnerAuthorityContractAcceptsExactExplicitSnapshot(t *testing.T) {
	want := validSingleOwnerAuthorityFixture()
	got := validSingleOwnerAuthorityFixture()
	got.Objects[0], got.Objects[1] = got.Objects[1], got.Objects[0]
	got.Roles[0], got.Roles[1] = got.Roles[1], got.Roles[0]
	if err := CompareSingleOwnerAuthorityContract(want, got); err != nil {
		t.Fatalf("exact set comparison: %v", err)
	}
}

func TestSingleOwnerAuthorityContractRejectsIndividualAuthorityDrift(t *testing.T) {
	mutations := map[string]func(*SingleOwnerAuthoritySnapshot){
		"role flag":    func(s *SingleOwnerAuthoritySnapshot) { s.Roles[0].Superuser = !s.Roles[0].Superuser },
		"membership":   func(s *SingleOwnerAuthoritySnapshot) { s.Memberships[0].SetOption = !s.Memberships[0].SetOption },
		"grant":        func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].Privilege = "UPDATE" },
		"grant option": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].GrantOption = !s.ACLs[0].GrantOption },
		"owner":        func(s *SingleOwnerAuthoritySnapshot) { s.Objects[0].Owner = "other" },
		"default ACL":  func(s *SingleOwnerAuthoritySnapshot) { s.DefaultACLs[0].Privilege = "UPDATE" },
		"effective probe": func(s *SingleOwnerAuthoritySnapshot) {
			s.EffectivePrivileges[0].Granted = !s.EffectivePrivileges[0].Granted
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			mutate(got)
			if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
				t.Fatal("authority drift accepted")
			}
		})
	}
}

func TestSingleOwnerAuthorityContractRejectsMissingExtraAndDuplicateTuples(t *testing.T) {
	cases := map[string]func(*SingleOwnerAuthoritySnapshot){
		"missing": func(s *SingleOwnerAuthoritySnapshot) { s.Objects = s.Objects[:1] },
		"extra": func(s *SingleOwnerAuthoritySnapshot) {
			s.Objects = append(s.Objects, s.Objects[0])
			s.Objects[2].Identity = "public.unexpected"
		},
		"duplicate": func(s *SingleOwnerAuthoritySnapshot) { s.Objects = append(s.Objects, s.Objects[0]) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			mutate(got)
			if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
				t.Fatal("set mismatch accepted")
			}
		})
	}
}

func TestSingleOwnerAuthorityContractRejectsAbsentOrAmbiguousContract(t *testing.T) {
	cases := map[string]func(*SingleOwnerAuthoritySnapshot){
		"nil":                          func(s *SingleOwnerAuthoritySnapshot) { s.Roles = nil },
		"missing collection":           func(s *SingleOwnerAuthoritySnapshot) { s.ACLs = nil },
		"empty required":               func(s *SingleOwnerAuthoritySnapshot) { s.Objects = []AuthorityObject{} },
		"duplicate role identity":      func(s *SingleOwnerAuthoritySnapshot) { s.Roles = append(s.Roles, s.Roles[0]) },
		"duplicate object identity":    func(s *SingleOwnerAuthoritySnapshot) { s.Objects = append(s.Objects, s.Objects[0]) },
		"ambiguous function signature": func(s *SingleOwnerAuthoritySnapshot) { s.Objects[0].Identity = "public.f()" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			mutate(got)
			err := CompareSingleOwnerAuthorityContract(want, got)
			if err == nil || strings.TrimSpace(err.Error()) == "" {
				t.Fatal("invalid contract accepted")
			}
		})
	}
}

func TestSingleOwnerAuthorityContractRepeatedTupleIdentity(t *testing.T) {
	duplicates := map[string]func(*SingleOwnerAuthoritySnapshot){
		"membership": func(s *SingleOwnerAuthoritySnapshot) {
			r := s.Memberships[0]
			r.SetOption = false
			s.Memberships = append(s.Memberships, r)
		},
		"ACL": func(s *SingleOwnerAuthoritySnapshot) {
			r := s.ACLs[0]
			r.GrantOption = true
			s.ACLs = append(s.ACLs, r)
		},
		"default ACL": func(s *SingleOwnerAuthoritySnapshot) {
			r := s.DefaultACLs[0]
			r.GrantOption = true
			s.DefaultACLs = append(s.DefaultACLs, r)
		},
		"default ACL changed grantor": func(s *SingleOwnerAuthoritySnapshot) {
			r := s.DefaultACLs[0]
			r.Grantor = "another_grantor"
			s.DefaultACLs = append(s.DefaultACLs, r)
		},
		"effective probe": func(s *SingleOwnerAuthoritySnapshot) {
			r := s.EffectivePrivileges[0]
			r.Granted = false
			s.EffectivePrivileges = append(s.EffectivePrivileges, r)
		},
	}
	for label, duplicate := range duplicates {
		t.Run(label+"/both sides", func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			duplicate(want)
			duplicate(got)
			if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
				t.Fatal("matching duplicate identities accepted")
			}
		})
		for _, side := range []string{"expected", "actual"} {
			t.Run(label+"/"+side, func(t *testing.T) {
				want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
				target := got
				if side == "expected" {
					target = want
				}
				duplicate(target)
				if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
					t.Fatal("duplicate identity accepted")
				}
			})
		}
	}
}

func TestSingleOwnerAuthorityContractDoesNotMutateInputs(t *testing.T) {
	want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
	beforeWant, beforeGot := *validSingleOwnerAuthorityFixture(), *validSingleOwnerAuthorityFixture()
	if err := CompareSingleOwnerAuthorityContract(want, got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*want, beforeWant) || !reflect.DeepEqual(*got, beforeGot) {
		t.Fatal("comparison mutated an input")
	}
}

func TestSingleOwnerAuthorityContractRejectsEachSensitiveRoleFlag(t *testing.T) {
	cases := map[string]func(*AuthorityRole){"superuser": func(r *AuthorityRole) { r.Superuser = !r.Superuser }, "createdb": func(r *AuthorityRole) { r.CreateDB = !r.CreateDB }, "createrole": func(r *AuthorityRole) { r.CreateRole = !r.CreateRole }, "replication": func(r *AuthorityRole) { r.Replication = !r.Replication }, "bypassrls": func(r *AuthorityRole) { r.BypassRLS = !r.BypassRLS }, "login": func(r *AuthorityRole) { r.Login = !r.Login }, "inherit": func(r *AuthorityRole) { r.Inherit = !r.Inherit }}
	for name, flip := range cases {
		t.Run(name, func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			flip(&got.Roles[0])
			if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
				t.Fatal("role flag drift accepted")
			}
		})
	}
}
func TestSingleOwnerAuthorityContractRejectsEachMembershipOption(t *testing.T) {
	cases := map[string]func(*AuthorityMembership){"admin": func(m *AuthorityMembership) { m.AdminOption = !m.AdminOption }, "inherit": func(m *AuthorityMembership) { m.InheritOption = !m.InheritOption }, "set": func(m *AuthorityMembership) { m.SetOption = !m.SetOption }}
	for name, flip := range cases {
		t.Run(name, func(t *testing.T) {
			want, got := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			flip(&got.Memberships[0])
			if err := CompareSingleOwnerAuthorityContract(want, got); err == nil {
				t.Fatal("membership drift accepted")
			}
		})
	}
}
func TestSingleOwnerAuthorityContractRejectsTupleDriftAndEveryCollectionMissingExtra(t *testing.T) {
	mutators := map[string]func(*SingleOwnerAuthoritySnapshot){
		"object owner": func(s *SingleOwnerAuthoritySnapshot) { s.Objects[0].Owner = "different" }, "function signature": func(s *SingleOwnerAuthoritySnapshot) { s.Objects[1].Identity = "public.f(uuid)" },
		"ACL class": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].Class = "sequence" }, "ACL column": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].Column = "id" }, "ACL grantor": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].Grantor = "other" }, "ACL PUBLIC grantee": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs[0].Grantee = "reader" },
		"default grantor": func(s *SingleOwnerAuthoritySnapshot) { s.DefaultACLs[0].Grantor = "other" }, "probe role": func(s *SingleOwnerAuthoritySnapshot) { s.EffectivePrivileges[0].Role = "other" },
	}
	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			w, g := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			mutate(g)
			if err := CompareSingleOwnerAuthorityContract(w, g); err == nil {
				t.Fatal("tuple drift accepted")
			}
		})
	}
	for name, add := range map[string]func(*SingleOwnerAuthoritySnapshot){"roles": func(s *SingleOwnerAuthoritySnapshot) { s.Roles = append(s.Roles, AuthorityRole{Name: "extra"}) }, "objects": func(s *SingleOwnerAuthoritySnapshot) {
		s.Objects = append(s.Objects, AuthorityObject{Class: "type", Identity: "public.extra", Owner: "owner"})
	}, "memberships": func(s *SingleOwnerAuthoritySnapshot) {
		s.Memberships = append(s.Memberships, AuthorityMembership{Role: "extra", Member: "member"})
	}, "ACLs": func(s *SingleOwnerAuthoritySnapshot) {
		s.ACLs = append(s.ACLs, AuthorityACL{Class: "relation", ObjectIdentity: "extra", Grantor: "owner", Grantee: "PUBLIC", Privilege: "SELECT"})
	}, "defaults": func(s *SingleOwnerAuthoritySnapshot) {
		s.DefaultACLs = append(s.DefaultACLs, AuthorityDefaultACL{Owner: "owner", Grantor: "owner", Class: "relation", Grantee: "PUBLIC", Privilege: "SELECT"})
	}, "probes": func(s *SingleOwnerAuthoritySnapshot) {
		s.EffectivePrivileges = append(s.EffectivePrivileges, AuthorityPrivilegeProbe{Role: "extra", ObjectClass: "relation", ObjectIdentity: "x", Privilege: "SELECT"})
	}} {
		t.Run(name+" extra", func(t *testing.T) {
			w, g := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			add(g)
			if err := CompareSingleOwnerAuthorityContract(w, g); err == nil {
				t.Fatal("extra tuple accepted")
			}
		})
	}
	for name, remove := range map[string]func(*SingleOwnerAuthoritySnapshot){"roles": func(s *SingleOwnerAuthoritySnapshot) { s.Roles = s.Roles[:1] }, "objects": func(s *SingleOwnerAuthoritySnapshot) { s.Objects = s.Objects[:1] }, "memberships": func(s *SingleOwnerAuthoritySnapshot) { s.Memberships = nilSliceMembership() }, "ACLs": func(s *SingleOwnerAuthoritySnapshot) { s.ACLs = []AuthorityACL{} }, "defaults": func(s *SingleOwnerAuthoritySnapshot) { s.DefaultACLs = []AuthorityDefaultACL{} }, "probes": func(s *SingleOwnerAuthoritySnapshot) { s.EffectivePrivileges = s.EffectivePrivileges[:0] }} {
		t.Run(name+" missing", func(t *testing.T) {
			w, g := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
			remove(g)
			if err := CompareSingleOwnerAuthorityContract(w, g); err == nil {
				t.Fatal("missing tuple accepted")
			}
		})
	}
}
func nilSliceMembership() []AuthorityMembership { return []AuthorityMembership{} }
func TestSingleOwnerAuthorityContractAllowsGlobalDefaultACLAndRequiresGrantor(t *testing.T) {
	w, g := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
	w.DefaultACLs[0].Schema = ""
	g.DefaultACLs[0].Schema = ""
	if err := CompareSingleOwnerAuthorityContract(w, g); err != nil {
		t.Fatalf("global default ACL rejected: %v", err)
	}
	g.DefaultACLs[0].Grantor = ""
	if err := CompareSingleOwnerAuthorityContract(w, g); err == nil {
		t.Fatal("default ACL without grantor accepted")
	}
}
func TestSingleOwnerAuthorityContractRejectsNULAndInvalidUTF8OnEitherSide(t *testing.T) {
	for name, bad := range map[string]string{"NUL": "x\x00y", "invalid UTF-8": string([]byte{0xff})} {
		for _, side := range []string{"expected", "actual"} {
			t.Run(name+"/"+side, func(t *testing.T) {
				w, g := validSingleOwnerAuthorityFixture(), validSingleOwnerAuthorityFixture()
				target := g
				if side == "expected" {
					target = w
				}
				target.ACLs[0].Grantee = bad
				if err := CompareSingleOwnerAuthorityContract(w, g); err == nil {
					t.Fatal("invalid string accepted")
				}
			})
		}
	}
}

func validSingleOwnerAuthorityFixture() *SingleOwnerAuthoritySnapshot {
	return &SingleOwnerAuthoritySnapshot{
		Roles:               []AuthorityRole{{Name: "espacigo_migrator_login", Login: true, Inherit: false}, {Name: "espacigo_schema_owner", CreateRole: false}},
		Objects:             []AuthorityObject{{Class: "relation", Identity: "public.ocupacion", Owner: "espacigo_schema_owner"}, {Class: "function", Identity: "public.f(uuid, text)", Owner: "espacigo_schema_owner"}},
		Memberships:         []AuthorityMembership{{Role: "espacigo_schema_owner", Member: "espacigo_migrator_login", AdminOption: false, InheritOption: false, SetOption: true}},
		ACLs:                []AuthorityACL{{Class: "relation", ObjectIdentity: "public.ocupacion", Grantor: "espacigo_schema_owner", Grantee: "PUBLIC", Privilege: "SELECT"}},
		DefaultACLs:         []AuthorityDefaultACL{{Owner: "espacigo_schema_owner", Grantor: "espacigo_schema_owner", Schema: "public", Class: "relation", Grantee: "PUBLIC", Privilege: "SELECT"}},
		EffectivePrivileges: []AuthorityPrivilegeProbe{{Role: "espacigo_migrator_login", ObjectClass: "relation", ObjectIdentity: "public.ocupacion", Privilege: "SELECT", Granted: true}},
	}
}
