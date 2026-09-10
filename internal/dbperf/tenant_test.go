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
