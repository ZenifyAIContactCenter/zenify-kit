package dbperf

import "testing"

func cfgLists() Config {
	c := Defaults()
	c.TenantScoped = []string{"tickets", "chat_rooms"}
	c.Global = []string{"configs"}
	return c
}

func TestRawLiteralMissingTenantIsBlocking(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({_id:x})"), cfgLists())
	if !hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatalf("want missing-tenant-filter BLOCKING: %+v", r.Findings)
	}
}

func TestTenantKeyPresentNoFinding(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({tenant_id:t,_id:x})"), cfgLists())
	if hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatal("tenant_id present must not flag")
	}
}

func TestUnclassifiedCollectionIsBlocking(t *testing.T) {
	r := ScanStatic(mk("db.collection('abc_new').find({_id:x})"), cfgLists())
	if !hasSignal(r, "unclassified-collection", Blocking) {
		t.Fatalf("want unclassified BLOCKING: %+v", r.Findings)
	}
}

func TestGlobalCollectionNoTenantFinding(t *testing.T) {
	r := ScanStatic(mk("db.collection('configs').find({k:1})"), cfgLists())
	if hasSignal(r, "missing-tenant-filter", Blocking) || hasSignal(r, "unclassified-collection", Blocking) {
		t.Fatal("global collection must not raise tenant/unclassified")
	}
}

func TestTenantDynamicFilterIsAdvisory(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find(buildFilter(req))"), cfgLists())
	if !hasSignal(r, "missing-tenant-filter", Advisory) {
		t.Fatalf("dynamic filter on tenant coll → ADVISORY: %+v", r.Findings)
	}
}

// Nested sub-object before the tenant key must NOT falsely block: a single
// regex bounded by [^}]* truncated the filter at the first nested `}`.
func TestNestedObjectBeforeTenantKeyNotBlocking(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({created_at:{$gte:a,$lte:b}, tenant_id:t})"), cfgLists())
	if hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatalf("tenant_id present after a nested sub-object must not block: %+v", r.Findings)
	}
}

func TestOrClauseBeforeTenantKeyNotBlocking(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({$or:[{a:1},{b:2}], tenant_id:t})"), cfgLists())
	if hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatalf("tenant_id present after $or must not block: %+v", r.Findings)
	}
}

func TestNestedFilterTrulyMissingTenantBlocks(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({created_at:{$gte:a,$lte:b}})"), cfgLists())
	if !hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatalf("nested filter with no tenant key must still block: %+v", r.Findings)
	}
}

// A value merely containing the tenant-key substring must not suppress a real
// missing-filter finding (tenantKeyRe requires the trailing colon of a key).
func TestTenantSubstringInValueStillBlocks(t *testing.T) {
	r := ScanStatic(mk("db.collection('tickets').find({name:\"tenant_id_field\"})"), cfgLists())
	if !hasSignal(r, "missing-tenant-filter", Blocking) {
		t.Fatalf("tenant_id as a value substring must not suppress the finding: %+v", r.Findings)
	}
}
