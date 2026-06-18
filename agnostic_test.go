package demesne

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EID-278 / v3 WS4 — deployment-agnostic completion. The engine must govern a
// schema that follows NONE of Foir's `id` / `<level>_id` naming conventions. The
// worked example (examples/agnostic.demesne) uses non-`id` primary keys
// (asset_pk / space_pk), non-`<level>_id` tenancy FKs (tenant_ref / space_ref),
// and non-`<level>_id` claim keys (tnt / spc). These tests assert the threaded
// sites — containment, the level-entity self column, owner / grant / mode /
// kernel predicates, the point-check, and schema binding — all compile against
// those names, and that sibling isolation still holds.
//
// The complementary byte-identity half ("Foir, all id/<level>_id, emits exactly
// as before") is the UNCHANGED existing oracle suite: every other *_test.go
// passes with zero edits because each new knob defaults to the convention.

func loadAgnostic(t *testing.T) *Spec {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("examples", "agnostic.demesne"))
	if err != nil {
		t.Fatalf("read agnostic example: %v", err)
	}
	s, err := Parse(string(src))
	if err != nil {
		t.Fatalf("parse agnostic example: %v", err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("validate agnostic example: %v", err)
	}
	return s
}

// TestAgnostic_ScopeClaimDecoupling — the containment predicate pins the level's
// declared SCOPE COLUMN to its declared CLAIM KEY, independently named: a sub-row
// object pins `tenant_ref = claim('tnt')` and `space_ref = claim('spc')`; the
// level-entity object pins its own PRIMARY KEY `space_pk = claim('spc')` for its
// own node (NOT `id`, NOT `space_id`). The claims contract is the claim keys, not
// the column names.
func TestAgnostic_ScopeClaimDecoupling(t *testing.T) {
	s := loadAgnostic(t)
	res, err := s.EmitRLS()
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	asset := findPolicy(res, "assets_select")
	if asset == nil {
		t.Fatalf("no assets_select (unsupported: %v)", res.Unsupported)
	}
	for _, frag := range []string{
		// scope column (FK) = claim key — decoupled at BOTH non-virtual levels.
		"tenant_ref = " + s.claim("tnt"),
		"space_ref = " + s.claim("spc"),
		// owner axis over a non-conventional column + its subject's claim.
		"holder_ref = " + s.claim("member_ref"),
		// the ambient membership-operator branch reads the leaf level's CLAIM KEY.
		s.claim("spc") + " IS NULL",
	} {
		if !strings.Contains(asset.Using, frag) {
			t.Errorf("assets_select missing %q in:\n%s", frag, asset.Using)
		}
	}
	// No conventional name must leak — neither the column as a claim nor `<level>_id`.
	for _, leaked := range []string{
		s.claim("tenant_id"), s.claim("space_id"), s.claim("tenant_ref"),
	} {
		if strings.Contains(asset.Using, leaked) {
			t.Errorf("assets_select leaked a conventional claim %q:\n%s", leaked, asset.Using)
		}
	}

	// The level-entity object pins its own PRIMARY KEY for its own node, read
	// against the level's claim key — the level-entity self column threaded the PK.
	space := findPolicy(res, "spaces_select")
	if space == nil {
		t.Fatal("no spaces_select policy")
	}
	for _, frag := range []string{
		"space_pk = " + s.claim("spc"),   // self node = PK = claim
		"tenant_ref = " + s.claim("tnt"), // ancestor containment
		"tenant_ref, space_pk)",          // role definer scope cols include the PK
	} {
		if !strings.Contains(space.Using, frag) {
			t.Errorf("spaces_select missing %q in:\n%s", frag, space.Using)
		}
	}

	// The claims contract is the declared CLAIM KEYS (tnt/spc), never the column
	// names or the `<level>_id` convention.
	contract, err := s.ClaimsContract()
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, c := range contract {
		set[c] = true
	}
	for _, want := range []string{"tnt", "spc", "sub", "member_ref"} {
		if !set[want] {
			t.Errorf("claims contract missing %q: %v", want, contract)
		}
	}
	for _, bad := range []string{"tenant_id", "space_id", "tenant_ref", "space_ref", "platform_id"} {
		if set[bad] {
			t.Errorf("claims contract leaked a conventional/column key %q: %v", bad, contract)
		}
	}
}

// TestAgnostic_NonIDPrimaryKey — the row-identity sites all reference the declared
// PK (asset_pk / space_pk), never a hard-coded `id`: the point-check, the kernel
// reachability gate, the grant-edge predicate, and the runtime access surface.
func TestAgnostic_NonIDPrimaryKey(t *testing.T) {
	s := loadAgnostic(t)

	// Point-check is over the declared PK.
	pc, err := s.PointCheckSQL("asset")
	if err != nil {
		t.Fatal(err)
	}
	if pc != "SELECT EXISTS (SELECT 1 FROM assets WHERE asset_pk = $1)" {
		t.Errorf("PointCheckSQL not over the declared PK: %s", pc)
	}

	// The grant-edge fragment references <table>.<pk> as the record identity.
	res, err := s.EmitRLS()
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	asset := findPolicy(res, "assets_select")
	if asset == nil || !strings.Contains(asset.Using, "assets.asset_pk, 'read')") {
		t.Errorf("grant fragment must reference assets.asset_pk:\n%v", asset)
	}

	// The kernel (realtime) gate matches the row by the declared PK.
	defs, err := s.EmitDefiners()
	if err != nil {
		t.Fatalf("emit definers: %v", err)
	}
	var kernel *GenFn
	for i := range defs {
		if defs[i].Name == "member_can_access_asset" {
			kernel = &defs[i]
		}
	}
	if kernel == nil {
		t.Fatal("no member_can_access_asset kernel definer")
	}
	if !strings.Contains(kernel.Body, "r.asset_pk = p_asset_id") {
		t.Errorf("kernel gate must match on the declared PK:\n%s", kernel.Body)
	}
	if strings.Contains(kernel.Body, "r.id = ") {
		t.Errorf("kernel gate still assumes an `id` PK:\n%s", kernel.Body)
	}

	// The runtime access surface (read/set visibility) is keyed on the declared PK.
	surf, err := s.ResourceAccessSurface("asset")
	if err != nil {
		t.Fatalf("resource access surface: %v", err)
	}
	if got := surf.ModeSQL(); !strings.HasSuffix(got, "WHERE asset_pk = $1") {
		t.Errorf("ModeSQL not over the declared PK: %s", got)
	}
	if got := surf.SetVisibilitySQL(); !strings.HasSuffix(got, "WHERE asset_pk = $2") {
		t.Errorf("SetVisibilitySQL not over the declared PK: %s", got)
	}
	// The scope columns the surface writes are the declared FK names (root→leaf).
	if !eqStrs(surf.ScopeCols, []string{"tenant_ref", "space_ref"}) {
		t.Errorf("access surface scope cols = %v, want [tenant_ref space_ref]", surf.ScopeCols)
	}
}

// TestAgnostic_ForwardIsolation — sibling isolation holds under the
// non-conventional naming. The asset SELECT containment is a sargable AND-chain
// of `<scope col> = claim(<claim key>)` over BOTH non-virtual levels, so a row
// whose tenant_ref or space_ref differs from the caller's tnt/spc claims is
// excluded (a caller in tenant A can never see tenant B's assets). We assert it
// structurally on the emitted predicate, then prove the invariant generatively
// over random custom-named topologies.
func TestAgnostic_ForwardIsolation(t *testing.T) {
	s := loadAgnostic(t)
	res, err := s.EmitRLS()
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	asset := findPolicy(res, "assets_select")
	if asset == nil {
		t.Fatal("no assets_select")
	}
	// The containment block is the AND-chain pinning every ancestor scope column.
	containment := "tenant_ref = " + s.claim("tnt") + " AND space_ref = " + s.claim("spc")
	if !strings.Contains(asset.Using, containment) {
		t.Errorf("assets_select containment not the expected sargable AND-chain %q in:\n%s", containment, asset.Using)
	}
	// Isolation is fail-closed: the ONLY containment-independent branch is the
	// virtual-root operator (the membership god-flag), gated by `<leaf claim> IS
	// NULL`. No other top-level disjunct may be unconditional.
	if !strings.Contains(asset.Using, "auth.is_ops("+s.claim("sub")+") AND "+s.claim("spc")+" IS NULL") {
		t.Errorf("operator branch must be scope-gated (no ambient cross-tenant reach):\n%s", asset.Using)
	}

	// Generative: the isolation invariant survives ARBITRARY scope-column / claim-key
	// naming (the EID-278 generalization of the V7 property).
	rng := rand.New(rand.NewSource(0x2778))
	for iter := 0; iter < 500; iter++ {
		spec := genCustomNamedSpec(rng)
		for _, sub := range spec.Subjects {
			cols, virtual, err := spec.PinnedColumns(sub)
			if err != nil {
				t.Fatalf("iter %d PinnedColumns(%s): %v", iter, sub.Name, err)
			}
			// Empty pins iff the anchor is virtual (the sanctioned operator).
			if (len(cols) == 0) != virtual {
				t.Fatalf("iter %d subject %s: empty-pin %v must equal virtual-anchor %v", iter, sub.Name, cols == nil, virtual)
			}
			if virtual {
				continue
			}
			// The deepest pinned dimension is the anchor's DECLARED claim key — so two
			// subjects at sibling anchors pin the same dimension to different values,
			// isolated by construction, whatever the level was named.
			want := spec.Topology.LevelByName(sub.Anchor).claimKey()
			if cols[len(cols)-1] != want {
				t.Fatalf("iter %d subject %s deepest pin %q, want anchor claim key %q", iter, sub.Name, cols[len(cols)-1], want)
			}
		}
		// And the emitted containment pins the custom SCOPE COLUMN to the custom
		// CLAIM KEY at every level (the SQL-level isolation, custom-named).
		res, err := spec.EmitRLS()
		if err != nil {
			t.Fatalf("iter %d emit: %v", iter, err)
		}
		pol := findPolicy(res, "rows_select")
		if pol == nil {
			t.Fatalf("iter %d: no rows_select (unsupported %v)", iter, res.Unsupported)
		}
		for _, l := range spec.Topology.Levels {
			if l.Virtual {
				continue
			}
			want := l.scopeColumn() + " = " + spec.claim(l.claimKey())
			if !strings.Contains(pol.Using, want) {
				t.Fatalf("iter %d: rows_select missing custom-named containment %q in:\n%s", iter, want, pol.Using)
			}
		}
	}
}

// genCustomNamedSpec builds a random linear topology whose every non-virtual level
// declares DELIBERATELY non-conventional scope-column and claim-key names (col
// l<i>_fk, claim l<i>_k), plus one `reach self`/`descendants` subject per level
// and a single scoped object — so the emitter exercises the decoupled names.
func genCustomNamedSpec(rng *rand.Rand) *Spec {
	depth := 2 + rng.Intn(3) // 2..4 levels
	virtualRoot := rng.Intn(2) == 0
	top := &Topology{}
	names := make([]string, depth)
	for i := 0; i < depth; i++ {
		names[i] = fmt.Sprintf("l%d", i)
		lv := &Level{Name: names[i]}
		if i == 0 {
			lv.Virtual = virtualRoot
		} else {
			lv.Parents = []string{names[i-1]}
		}
		if !lv.Virtual {
			lv.ScopeCol = names[i] + "_fk" // non-`<level>_id` column
			lv.ClaimKey = names[i] + "_k"  // non-`<level>_id` claim
		}
		top.Levels = append(top.Levels, lv)
	}
	s := &Spec{Topology: top}
	for i := 0; i < depth; i++ {
		for _, reach := range []string{"self", "descendants"} {
			s.Subjects = append(s.Subjects, &Subject{
				Name: fmt.Sprintf("s%d_%s", i, reach), Anchor: names[i], Reach: reach, Identifies: "sub",
			})
		}
	}
	// A leaf-scoped object with a single @scoped read so containment is the whole
	// predicate (no other grant terms to obscure the isolation check).
	var scoped []string
	for _, l := range top.Levels {
		if !l.Virtual {
			scoped = append(scoped, l.Name)
		}
	}
	leaf := names[depth-1]
	s.Objects = []*Object{{
		Name: "row", Table: "rows", PK: "row_pk", Scoped: scoped,
		Perms: []*Perm{{
			Verb: "view", Maps: "select", Layers: []string{"rls"},
			Tree: &PermNode{Op: "leaf", Term: &Term{Builtin: "scoped"}},
			Expr: []*Term{{Builtin: "scoped"}},
		}},
	}}
	_ = leaf
	return s
}

// TestAgnostic_BindsToSchema — the spec binds to a database whose columns carry
// the non-conventional names, and a MISSING primary-key column is reported (the
// PK is now part of the bind surface, not an `id` assumption).
func TestAgnostic_BindsToSchema(t *testing.T) {
	s := loadAgnostic(t)
	sc := agnosticSchema()
	if err := s.ValidateAgainst(sc); err != nil {
		t.Fatalf("agnostic spec should bind to its matching schema, got: %v", err)
	}
	// Drop the asset table's declared PK → the bind fails on it (proving the PK is
	// checked under its declared name, not `id`).
	delete(sc.tables["assets"], "asset_pk")
	if err := s.ValidateAgainst(sc); err == nil || !strings.Contains(err.Error(), `no column "asset_pk"`) {
		t.Errorf("missing declared PK should be reported, got: %v", err)
	}
}

// TestAgnostic_GrammarBindsKnobs — the new `pk` / `col` / `claim` clauses parse
// into the right AST fields, and an absent clause defaults to the convention.
func TestAgnostic_GrammarBindsKnobs(t *testing.T) {
	s := loadAgnostic(t)

	// Level overrides bind; the virtual root carries none (defaults).
	tenant := s.Topology.LevelByName("tenant")
	if tenant.ScopeCol != "tenant_ref" || tenant.ClaimKey != "tnt" {
		t.Errorf("tenant level knobs = (%q,%q), want (tenant_ref,tnt)", tenant.ScopeCol, tenant.ClaimKey)
	}
	space := s.Topology.LevelByName("space")
	if space.ScopeCol != "space_ref" || space.ClaimKey != "spc" {
		t.Errorf("space level knobs = (%q,%q), want (space_ref,spc)", space.ScopeCol, space.ClaimKey)
	}
	// Object PKs bind.
	if o := findObject(s, "asset"); o == nil || o.PK != "asset_pk" {
		t.Errorf("asset.PK = %q, want asset_pk", o.PK)
	}
	if o := findObject(s, "space"); o == nil || o.PK != "space_pk" {
		t.Errorf("space.PK = %q, want space_pk", o.PK)
	}

	// Defaults: a spec that declares NO knob falls back to the conventions, so the
	// helper methods reproduce Foir's names exactly (the byte-identity guarantee).
	d := mustSpec(t, `
		topology { level tenant level project parent tenant }
		subject admin { anchor tenant reach descendants identifies sub roles none }
		object doc { table docs scoped tenant > project
		  relation t: tenant via tenant_id
		  permission view = @scoped @rls maps select }`)
	if got := d.Topology.LevelByName("project").scopeColumn(); got != "project_id" {
		t.Errorf("default scopeColumn = %q, want project_id", got)
	}
	if got := d.Topology.LevelByName("project").claimKey(); got != "project_id" {
		t.Errorf("default claimKey = %q, want project_id", got)
	}
	if got := findObject(d, "doc").pk(); got != "id" {
		t.Errorf("default object pk = %q, want id", got)
	}
}

// agnosticSchema returns a Schema satisfying examples/agnostic.demesne exactly.
func agnosticSchema() *Schema {
	sc := NewSchema()
	for _, c := range []string{"asset_pk", "tenant_ref", "space_ref", "holder_ref", "share"} {
		sc.AddColumn("assets", c, "text", c == "share")
	}
	for _, c := range []string{"space_pk", "tenant_ref"} {
		sc.AddColumn("spaces", c, "text", false)
	}
	for _, c := range []string{"member_kind", "member_ref", "role_ref", "killed_at", "tenant_ref", "space_ref"} {
		sc.AddColumn("crew_grants", c, "text", c == "killed_at")
	}
	sc.AddColumn("crew_roles", "row_id", "text", false)
	sc.AddColumn("crew_roles", "slug", "text", false)
	for _, c := range []string{"asset_ref", "grantee_kind", "grantee_ref", "perm"} {
		sc.AddColumn("asset_acl", c, "text", false)
	}
	sc.AddColumn("ops_users", "user_ref", "text", false)
	sc.AddColumn("ops_users", "is_ops", "boolean", false)
	sc.AddColumn("ops_users", "state", "text", false)
	return sc
}
