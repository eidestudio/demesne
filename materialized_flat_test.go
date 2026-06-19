package demesne

import (
	"strings"
	"testing"
)

const matGroupSpec = `
topology { level tenant level project parent tenant }
vocabulary cust { permission self:read }
subject customer { anchor project reach self identifies customer_id roles configurable cust binds owner }
object doc {
  table  docs
  scoped tenant > project
  relation grantee: customer via grant dacl(resource_id, principal_kind, principal_id, access)
  relation team:    customer via group tc(grp, mem) edge te(mem, grp) on team_id materialized
  permission view = grantee:read + team @rls maps select
}`

func TestEmitMaterializedFlats_GroupRelation(t *testing.T) {
	s, err := Parse(matGroupSpec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("validate: %v", err)
	}
	flats := s.EmitMaterializedFlats()
	if len(flats) != 1 {
		t.Fatalf("want 1 materialized flat, got %d", len(flats))
	}
	f := flats[0]
	if f.Flat != "docs_team_flat" {
		t.Errorf("flat name = %q, want docs_team_flat", f.Flat)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS auth.docs_team_flat (resource_id text NOT NULL, principal_kind text NOT NULL, principal_id text NOT NULL)",
		"CREATE INDEX IF NOT EXISTS docs_team_flat_res_idx ON auth.docs_team_flat (resource_id)",
		"CREATE INDEX IF NOT EXISTS docs_team_flat_prin_idx ON auth.docs_team_flat (principal_id)",
	} {
		if !strings.Contains(f.TableSQL(), want) {
			t.Errorf("TableSQL missing %q:\n%s", want, f.TableSQL())
		}
	}
	for _, want := range []string{
		"DELETE FROM auth.docs_team_flat",
		"SELECT o.id, 'customer', c.mem",
		"FROM public.docs o JOIN public.tc c ON c.grp = o.team_id",
	} {
		if !strings.Contains(f.FunctionSQL(), want) {
			t.Errorf("FunctionSQL missing %q:\n%s", want, f.FunctionSQL())
		}
	}
	for _, want := range []string{
		"AFTER INSERT OR UPDATE OR DELETE ON public.docs",
		"AFTER INSERT OR UPDATE OR DELETE ON public.tc",
		"EXECUTE FUNCTION auth.docs_team_flat_rebuild()",
	} {
		if !strings.Contains(f.TriggerSQL(), want) {
			t.Errorf("TriggerSQL missing %q:\n%s", want, f.TriggerSQL())
		}
	}
}

// A non-materialized group relation emits NO flat — the modifier is opt-in, so any
// existing spec is byte-identical (no flat tables/triggers appear).
func TestEmitMaterializedFlats_NoneWhenNotMaterialized(t *testing.T) {
	spec := strings.Replace(matGroupSpec, "on team_id materialized", "on team_id", 1)
	s, err := Parse(spec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if flats := s.EmitMaterializedFlats(); len(flats) != 0 {
		t.Errorf("non-materialized group must emit no flat, got %d", len(flats))
	}
}
