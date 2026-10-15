package dbread

import (
	"encoding/json"
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
	has(MongoQuery{Op: "find", Coll: "u"}, ".limit(51)")
	has(MongoQuery{Op: "find", Coll: "u", Limit: 200}, ".limit(201)")
	has(MongoQuery{Op: "find", Coll: "u"}, ".maxTimeMS(30000)")
	if _, err := BuildMongoScript(MongoQuery{Op: "find", Coll: "u", Limit: 201}); err == nil || !strings.Contains(err.Error(), "200") {
		t.Errorf("want error mentioning 200, got %v", err)
	}
	has(MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `[{"$match":{"a":1}}]`}, `[...EJSON.parse("[{\"$match\":{\"a\":1}}]"),{"$limit":51}]`)
	has(MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `[{"$match":{}}]`}, "maxTimeMS:30000")
}

func TestBuildMongoScript_Count(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "u"})
	if err != nil || !strings.Contains(s, "estimatedDocumentCount()") {
		t.Fatalf("no filter: %v %s", err, s)
	}
	s, err = BuildMongoScript(MongoQuery{Op: "count", Coll: "u", Filter: `{"a":1}`})
	if err != nil || !strings.Contains(s, `countDocuments(EJSON.parse("{\"a\":1}"),{maxTimeMS:30000})`) {
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

func TestBuildMongoScript_PreservesKeyOrderAndEJSON(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "find", Coll: "u", Sort: `{"b":1,"a":-1}`, Filter: `{"_id":{"$oid":"507f1f77bcf86cd799439011"}}`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `.sort(EJSON.parse("{\"b\":1,\"a\":-1}"))`) {
		t.Errorf("sort key order lost: %s", s)
	}
	if !strings.Contains(s, `EJSON.parse("{\"_id\":{\"$oid\":\"507f1f77bcf86cd799439011\"}}")`) {
		t.Errorf("extended JSON filter not passed through EJSON.parse: %s", s)
	}
	p, err := BuildMongoScript(MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `[{"$group":{"_id":"$z","n":{"$sum":1}}}]`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, `{\"_id\":\"$z\",\"n\":{\"$sum\":1}}`) {
		t.Errorf("pipeline key order lost: %s", p)
	}
}

func TestBuildMongoScript_Rejections(t *testing.T) {
	cases := []struct {
		name string
		q    MongoQuery
	}{
		{"trailing data", MongoQuery{Op: "find", Coll: "u", Filter: `{"a":1});db.x()`}},
		{"non-object filter", MongoQuery{Op: "find", Coll: "u", Filter: `[]`}},
		{"negative limit", MongoQuery{Op: "find", Coll: "u", Limit: -1}},
		{"unknown op", MongoQuery{Op: "drop", Coll: "u"}},
		{"non-array pipeline", MongoQuery{Op: "aggregate", Coll: "u", Pipeline: `{}`}},
	}
	for _, c := range cases {
		if _, err := BuildMongoScript(c.q); err == nil {
			t.Errorf("%s: want error", c.name)
		}
	}
}

func TestBuildMongoScript_ExplainFind(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "explain", Coll: "u", Filter: `{"a":1}`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `.find(EJSON.parse("{\"a\":1}")).maxTimeMS(30000).explain("executionStats")`) {
		t.Errorf("unexpected explain script: %s", s)
	}
	if strings.Contains(s, ".limit(") {
		t.Errorf("explain must run uncapped: %s", s)
	}
}

func TestBuildMongoScript_ExplainPipelineUncapped(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "explain", Coll: "u", Pipeline: `[{"$match":{"a":1}}]`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "$limit") {
		t.Errorf("explain pipeline must not get a trailing $limit: %s", s)
	}
	if !strings.Contains(s, `aggregate(EJSON.parse("[{\"$match\":{\"a\":1}}]"),{maxTimeMS:30000})`) {
		t.Errorf("unexpected explain pipeline script: %s", s)
	}
}

// find/aggregate fetch limit+1 so a cut result says so on stderr.
func TestBuildMongoScript_TruncationNotice(t *testing.T) {
	notice := `process.stderr.write("… đã cắt ở 50 dòng (--limit, tối đa 200)\n")` //znf:allow-lang
	for _, op := range []string{"find", "aggregate"} {
		s, err := BuildMongoScript(MongoQuery{Op: op, Coll: "u", Pipeline: `[{"$match":{}}]`})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, "if(r.length>50){r.pop();") || !strings.Contains(s, notice) {
			t.Errorf("%s: no limit+1 truncation notice: %s", op, s)
		}
		// mongosh --eval prints the last statement's value; write() returns true.
		if !strings.HasSuffix(s, "if(t)void "+notice) {
			t.Errorf("%s: notice write must be void so stdout gets no 'true': %s", op, s)
		}
		if strings.Index(s, "print(EJSON.stringify(r") > strings.Index(s, "process.stderr.write") {
			t.Errorf("%s: notice must follow the data: %s", op, s)
		}
	}
}

func TestBuildMongoScript_CollectionsOnlyRealCollections(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "collections", Coll: "Chat"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `d.getCollectionInfos({type:"collection"}).map(c=>c.name).sort().filter(n=>n.toLowerCase().includes("chat"))`) {
		t.Errorf("collections must list real collections only: %s", s)
	}
}

func TestBuildMongoScript_CountBlankFilterIsEstimated(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "u", Filter: "  "})
	if err != nil || !strings.Contains(s, "estimatedDocumentCount()") {
		t.Fatalf("blank filter: %v %s", err, s)
	}
}

func TestBuildMongoScript_DBDefault3csoft(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "getSiblingDB('3csoft')") {
		t.Errorf("script = %q, want default 3csoft db", s)
	}
}

func TestBuildMongoScript_DBOverride(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "users", DB: "audit_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `getSiblingDB("audit_logs")`) || strings.Contains(s, "3csoft") {
		t.Errorf("script = %q, want audit_logs db only", s)
	}
}

func TestBuildMongoScript_DBRejectsQuote(t *testing.T) {
	s, err := BuildMongoScript(MongoQuery{Op: "count", Coll: "users", DB: "x');"})
	if err != nil {
		t.Fatal(err)
	}
	const pre, post = "getSiblingDB(", ");"
	i := strings.Index(s, pre)
	if i < 0 || strings.Count(s, pre) != 1 {
		t.Fatalf("script = %q, want exactly one getSiblingDB call", s)
	}
	rest := s[i+len(pre):]
	// The argument must be exactly one JS (JSON) string literal that decodes
	// back to the raw name, followed directly by the closing paren.
	dec := json.NewDecoder(strings.NewReader(rest))
	var got string
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("argument is not one string literal: %v (script %q)", err, s)
	}
	if got != "x');" {
		t.Errorf("literal decodes to %q, want %q", got, "x');")
	}
	if tail := rest[dec.InputOffset():]; !strings.HasPrefix(tail, post) {
		t.Errorf("after the literal = %q, want %q", tail, post)
	}
}
