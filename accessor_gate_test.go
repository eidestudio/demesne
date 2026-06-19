package demesne

import (
	"strings"
	"testing"
)

func selectPerm(expr []*Term, tree *PermNode) *Perm {
	return &Perm{Verb: "read", Maps: "select", Expr: expr, Tree: tree}
}

// Owner + grant in a flat OR is exactly what the enumerator covers → no refusal
// (and the Foir-shaped case, so the gate must not trip it).
func TestAccessorCoverage_OwnerGrantUnion_Covered(t *testing.T) {
	obj := &Object{
		Name: "record", Table: "records",
		Relations: []*Relation{
			{Name: "owner", Types: []string{"customer"}, Repr: ViaColumn{Column: "owner_id"}},
			{Name: "grantee", Types: []string{"customer"}, Repr: ViaGrant{Table: "resource_acl"}},
		},
		Perms: []*Perm{selectPerm(
			[]*Term{{Ident: "owner"}, {Ident: "grantee"}},
			&PermNode{Op: "or", Kids: []*PermNode{
				{Op: "leaf", Term: &Term{Ident: "owner"}},
				{Op: "leaf", Term: &Term{Ident: "grantee"}},
			}},
		)},
	}
	if ok, reason := accessorCoverage(obj); !ok {
		t.Errorf("owner+grant flat-OR must be covered, got refusal: %s", reason)
	}
}

// A SELECT term over a relation with no reverse branch (here ViaGroup) must fail
// closed — emitting would silently under-report who can access the row.
func TestAccessorCoverage_UncoveredRepr_FailsClosed(t *testing.T) {
	obj := &Object{
		Name: "doc", Table: "docs",
		Relations: []*Relation{{Name: "team", Repr: ViaGroup{Closure: "team_closure"}}},
		Perms:     []*Perm{selectPerm([]*Term{{Ident: "team"}}, &PermNode{Op: "leaf", Term: &Term{Ident: "team"}})},
	}
	ok, reason := accessorCoverage(obj)
	if ok {
		t.Fatal("a SELECT via ViaGroup must fail closed (no accessor branch yet)")
	}
	if !strings.Contains(reason, "team") {
		t.Errorf("reason should name the offending relation, got %q", reason)
	}
}

// Intersection / exclusion in the SELECT tree must fail closed — the union-only
// enumerator cannot represent INTERSECT / EXCEPT.
func TestAccessorCoverage_IntersectionExclusion_FailsClosed(t *testing.T) {
	obj := &Object{
		Name: "doc", Table: "docs",
		Relations: []*Relation{{Name: "owner", Repr: ViaColumn{Column: "owner_id"}}},
		Perms: []*Perm{selectPerm(
			[]*Term{{Ident: "owner"}},
			&PermNode{Op: "and", Kids: []*PermNode{
				{Op: "leaf", Term: &Term{Ident: "owner"}},
				{Op: "not", Kids: []*PermNode{{Op: "leaf", Term: &Term{Ident: "owner"}}}},
			}},
		)},
	}
	if ok, reason := accessorCoverage(obj); ok {
		t.Errorf("an `and`/`and not` SELECT tree must fail closed, got covered (reason=%q)", reason)
	}
}

func TestAccessorCoverage_NoSelectPerm_Covered(t *testing.T) {
	if ok, _ := accessorCoverage(&Object{Name: "x", Table: "xs"}); !ok {
		t.Error("no SELECT perm → nothing to enumerate → covered")
	}
}

// Non-relation leaves (a builtin) must not trip the gate.
func TestAccessorCoverage_BuiltinLeaf_Covered(t *testing.T) {
	obj := &Object{
		Name: "record", Table: "records",
		Relations: []*Relation{{Name: "owner", Repr: ViaColumn{Column: "owner_id"}}},
		Perms: []*Perm{selectPerm(
			[]*Term{{Ident: "owner"}, {Builtin: "app_scope", ExcludeRel: "owner"}},
			&PermNode{Op: "or", Kids: []*PermNode{
				{Op: "leaf", Term: &Term{Ident: "owner"}},
				{Op: "leaf", Term: &Term{Builtin: "app_scope", ExcludeRel: "owner"}},
			}},
		)},
	}
	if ok, reason := accessorCoverage(obj); !ok {
		t.Errorf("a builtin leaf alongside owner must not trip the gate, got refusal: %s", reason)
	}
}
