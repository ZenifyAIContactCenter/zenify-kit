package dbread

import (
	"strings"
	"testing"
)

const connectLine = `const d=connect(process.env.ZNF_MONGO_URL).getSiblingDB('3csoft');`

func TestCheckPipeline(t *testing.T) {
	cases := []struct {
		name, js string
		bad      bool
	}{
		{"match ok", `[{"$match":{}}]`, false},
		{"out", `[{"$out":"x"}]`, true},
		{"merge", `[{"$merge":{"into":"x"}}]`, true},
		{"facet", `[{"$facet":{"a":[{"$merge":"x"}]}}]`, true},
		{"lookup", `[{"$lookup":{"from":"y","pipeline":[{"$out":"z"}]}}]`, true},
		{"unionWith", `[{"$unionWith":{"coll":"y","pipeline":[{"$merge":"z"}]}}]`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := parseJSONArray(c.js)
			if err != nil {
				t.Fatal(err)
			}
			err = CheckPipeline(p)
			if !c.bad {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || (!strings.Contains(err.Error(), "$out") && !strings.Contains(err.Error(), "$merge")) {
				t.Fatalf("want $out/$merge error, got %v", err)
			}
		})
	}
}

func TestBuildMongoScript_StartsWithConnect(t *testing.T) {
	for _, op := range []string{"collections", "doc", "count", "find", "aggregate", "explain"} {
		q := MongoQuery{Op: op, Coll: "users", Pipeline: `[{"$match":{}}]`}
		s, err := BuildMongoScript(q)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if !strings.HasPrefix(s, connectLine) {
			t.Errorf("%s: script does not start with connect line: %q", op, s)
		}
		if strings.Contains(s, "db.") {
			t.Errorf("%s: script touches db.: %q", op, s)
		}
	}
}

func TestBuildMongoScript_CollNameJSONEncoded(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "find", Coll: "x');db.dropDatabase();('"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `d.getCollection("x');db.dropDatabase();('")`) {
		t.Errorf("collection name not JSON-quoted: %s", s)
	}
	if n := strings.Count(s, "dropDatabase"); n != 1 {
		t.Errorf("dropDatabase count = %d, want 1", n)
	}
}

func TestBuildMongoScript_InvalidJSON(t *testing.T) {
	_, err := BuildMongoScript(MongoQuery{Op: "find", Coll: "u", Filter: "{a:1"})
	if err == nil || !strings.Contains(err.Error(), "--filter") {
		t.Fatalf("want --filter error, got %v", err)
	}
}

func TestBuildMongoScript_Limits(t *testing.T) {
	has := func(q MongoQuery, want string) {
		t.Helper()
		s, err := BuildMongoScript(q)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, want) {
			t.Errorf("want %q in %s", want, s)
		}
	}
	has(MongoQuery{Op: "find", Coll: "u"}, ".limit(50)")
	has(MongoQuery{Op: "find", Coll: "u", Limit: 200}, ".limit(200)")
	has(MongoQuery{Op: "find", Coll: "u"}, ".maxTimeMS(30000)")
	if _, err := BuildMongoScript(MongoQuery{Op: "find", Coll: "u", Limit: 201}); err == nil || !strings.Contains(err.Error(), "200") {
		t.Errorf("want error mentioning 200, got %v", err)
	}
	has(MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `[{"$match":{"a":1}}]`}, `[{"$match":{"a":1}},{"$limit":50}]`)
	has(MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `[{"$match":{}}]`}, "maxTimeMS:30000")
}

func TestBuildMongoScript_Count(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "u"})
	if err != nil || !strings.Contains(s, "estimatedDocumentCount()") {
		t.Fatalf("no filter: %v %s", err, s)
	}
	s, err = BuildMongoScript(MongoQuery{Op: "count", Coll: "u", Filter: `{"a":1}`})
	if err != nil || !strings.Contains(s, `countDocuments({"a":1},{maxTimeMS:30000})`) {
		t.Fatalf("filter: %v %s", err, s)
	}
}

func TestBuildMongoScript_ExplainPipelineChecked(t *testing.T) {
	_, err := BuildMongoScript(MongoQuery{Op: "explain", Coll: "u", Pipeline: `[{"$out":"x"}]`})
	if err == nil {
		t.Fatal("want error for $out in explain pipeline")
	}
}

func TestBuildMongoScript_PrintsEJSON(t *testing.T) {
	for _, op := range []string{"find", "aggregate", "doc"} {
		s, err := BuildMongoScript(MongoQuery{Op: op, Coll: "u", Pipeline: `[{"$match":{}}]`})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, "EJSON.stringify") {
			t.Errorf("%s: no EJSON.stringify: %s", op, s)
		}
	}
}
