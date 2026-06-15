package demesne

import (
	"strings"
	"testing"
)

// findAccessor returns the CreateSQL of the auth.<table>_accessors definer, or
// fails if the spec didn't emit one.
func findAccessor(t *testing.T, spec string, table string) string {
	t.Helper()
	s, err := Parse(spec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("validate: %v", err)
	}
	defs, err := s.EmitDefiners()
	if err != nil {
		t.Fatalf("emit definers: %v", err)
	}
	for _, d := range defs {
		if d.Name == table+"_accessors" {
			return d.CreateSQL()
		}
	}
	t.Fatalf("no %s_accessors definer emitted; definers: %v", table, defNames(defs))
	return ""
}

func defNames(defs []GenFn) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

// The accessor enumerator (Expand) is the read-side dual of the SELECT predicate:
// it lists every NAMED accessor the descriptor admits — owner column(s), the
// grant rows, and the role plane — built from the SAME descriptor, so it agrees
// with <table>_select by construction.
func TestAccessorEnumerator(t *testing.T) {
	sql := findAccessor(t, adminOwnerSpec, "records")

	// Set-returning SECURITY DEFINER over the four accessor columns.
	for _, want := range []string{
		"CREATE OR REPLACE FUNCTION auth.records_accessors(p_id text)",
		"RETURNS TABLE(source text, principal_kind text, principal_id text, access text)",
		"SECURITY DEFINER",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("accessor missing %q:\n%s", want, sql)
		}
	}

	// OWNER — the customer-plane owner column.
	if !strings.Contains(sql, "SELECT 'owner'::text AS source, 'customer'::text AS principal_kind, customer_id AS principal_id, 'write'::text AS access\n    FROM records WHERE id = p_id AND customer_id IS NOT NULL") {
		t.Errorf("missing customer-owner branch:\n%s", sql)
	}
	// OWNER — the admin-plane owner column.
	if !strings.Contains(sql, "SELECT 'owner'::text, 'admin'::text, admin_owner_id, 'write'::text\n    FROM records WHERE id = p_id AND admin_owner_id IS NOT NULL") {
		t.Errorf("missing admin-owner branch:\n%s", sql)
	}
	// GRANT — the explicit resource_acl rows, discriminated.
	if !strings.Contains(sql, "SELECT 'grant'::text, principal_kind, principal_id, access\n    FROM resource_acl WHERE resource_id = p_id AND resource_type = 'record'") {
		t.Errorf("missing grant branch:\n%s", sql)
	}
	// ROLE — admins with a role reaching the row's scope, gated by the admin-owner
	// exclusion (mirrors @app_scope: admin-owned rows are operator-private).
	if !strings.Contains(sql, "SELECT 'role'::text, 'admin'::text, ra.principal_id, 'read'::text") {
		t.Errorf("missing role branch:\n%s", sql)
	}
	if !strings.Contains(sql, "ra.tenant_id = r.tenant_id AND (ra.project_id IS NULL OR ra.project_id = r.project_id)") {
		t.Errorf("role branch scope-reach (ancestor-or-equal) wrong:\n%s", sql)
	}
	if !strings.Contains(sql, "WHERE r.id = p_id AND r.admin_owner_id IS NULL") {
		t.Errorf("role branch not gated by admin-owner exclusion:\n%s", sql)
	}
}

// Without an admin-owner axis the accessor has no admin-owner OWNER branch and
// the role branch carries no admin-owner gate (purely additive — a bare
// descriptor still gets a correct enumerator).
func TestAccessorEnumerator_BareDescriptor(t *testing.T) {
	const bareSpec = `
topology {
  level platform virtual
  level tenant   parent platform
  level project  parent tenant
}
vocabulary admin { permission c:r  preset pa @ project = c:r }
vocabulary cust  { permission self:read }
rolestore admin {
  assignments role_assignments
  kind        principal_kind = "admin"
  subject     principal_id
  scope       tenant_id project_id
  rolejoin    role_id roles id key
  revoked     revoked_at
}
subject admin    { anchor tenant  reach descendants identifies sub roles configurable admin binds admin }
subject customer { anchor project reach self identifies customer_id roles configurable cust binds owner }
subject service  { anchor project reach self identifies sub roles none }
object record {
  table  records
  scoped tenant > project
  descriptor {
    owner       customer | service via customer_id
    mode        via access_mode
    modes       private + read "public" + list "customer"
    grants      via edge resource_acl(resource_id, principal_kind, principal_id, access) where resource_type = "record"
  }
  permission view = @app_scope + @descriptor @rls maps select
}
`
	sql := findAccessor(t, bareSpec, "records")
	if strings.Contains(sql, "admin_owner_id") {
		t.Errorf("bare descriptor accessor should not reference admin_owner_id:\n%s", sql)
	}
	// Role branch still present, but gated only by the row id (no admin-owner exclusion).
	if !strings.Contains(sql, "WHERE r.id = p_id\n") {
		t.Errorf("bare role branch should be gated by r.id alone:\n%s", sql)
	}
	if !strings.Contains(sql, "JOIN role_assignments ra") {
		t.Errorf("role branch should read the role store:\n%s", sql)
	}
}
