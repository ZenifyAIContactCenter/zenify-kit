package dbread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Query limits and the env var the generated script reads the Mongo URL from.
const (
	DefaultLimit   = 50
	MaxLimit       = 200
	QueryTimeoutMS = 30000
	MongoURLEnv    = "ZNF_MONGO_URL"
	defaultMongoDB = "3csoft"
)

// mongoConnectLine connects and picks the database. Empty db keeps the
// historical single-quoted default; any other name is JSON-encoded so it
// cannot escape the literal.
func mongoConnectLine(db string) string {
	lit := "'" + defaultMongoDB + "'"
	if db != "" {
		b, _ := json.Marshal(db)
		lit = string(b)
	}
	return "const d=connect(process.env." + MongoURLEnv + ").getSiblingDB(" + lit + ");"
}

// MongoQuery is one structured read. Op is one of collections|doc|count|find|
// aggregate|explain|indexes|stats. JSON fields are the raw CLI flag strings (empty = absent).
type MongoQuery struct {
	Op, Coll, Filter, Projection, Sort, Pipeline string
	DB                                           string // empty = "3csoft"
	Limit                                        int
	// PlanOnly runs explain at "queryPlanner" verbosity: the plan without
	// executing the query, so nothing the query reads can surface.
	PlanOnly bool
}

// decodeJSON parses s only to validate it (single value, no trailing data).
func decodeJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data")
	}
	return v, nil
}

func parseJSONArray(s string) ([]any, error) {
	v, err := decodeJSON(s)
	if err != nil {
		return nil, err
	}
	a, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a JSON array")
	}
	return a, nil
}

// ejsonExpr turns validated JSON text into a JS expression. The compacted
// original text (key order preserved) becomes a JS string literal handed to
// EJSON.parse, so Extended JSON ($oid, $date) works and nothing is ever
// interpolated as raw JS.
func ejsonExpr(raw string) (string, error) {
	var c bytes.Buffer
	if err := json.Compact(&c, []byte(raw)); err != nil {
		return "", err
	}
	lit, err := json.Marshal(c.String())
	if err != nil {
		return "", err
	}
	return "EJSON.parse(" + string(lit) + ")", nil
}

// objectFlag validates a JSON-object flag and returns its JS expression, or ""
// when the flag is empty.
func objectFlag(flag, s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	v, err := decodeJSON(s)
	if err != nil {
		return "", fmt.Errorf("%s không phải JSON hợp lệ: %v", flag, err) //znf:allow-lang
	}
	if _, ok := v.(map[string]any); !ok {
		return "", fmt.Errorf("%s phải là một JSON object", flag) //znf:allow-lang
	}
	return ejsonExpr(s)
}

// CheckPipeline rejects any $out or $merge stage key at any depth (maps and
// slices, so $facet / $lookup.pipeline / $unionWith.pipeline are covered).
func CheckPipeline(pipeline []any) error {
	return checkNode(pipeline)
}

func checkNode(n any) error {
	switch v := n.(type) {
	case map[string]any:
		for k, c := range v {
			if k == "$out" || k == "$merge" {
				return fmt.Errorf("pipeline chứa %s — ghi dữ liệu bị từ chối", k) //znf:allow-lang
			}
			if err := checkNode(c); err != nil {
				return err
			}
		}
	case []any:
		for _, c := range v {
			if err := checkNode(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// pipelineFlag validates --pipeline, rejects write stages and returns a JS
// array expression, ending with a {$limit: limit} stage when limit > 0.
func pipelineFlag(q MongoQuery, limit int) (string, error) {
	p, err := parseJSONArray(q.Pipeline)
	if err != nil {
		return "", fmt.Errorf("--pipeline không phải mảng JSON hợp lệ: %v", err) //znf:allow-lang
	}
	if err := CheckPipeline(p); err != nil {
		return "", err
	}
	e, err := ejsonExpr(q.Pipeline)
	if err != nil {
		return "", err
	}
	if limit <= 0 {
		return e, nil
	}
	return fmt.Sprintf(`[...%s,{"$limit":%d}]`, e, limit), nil
}

// printCapped prints the array expression arr (fetched with limit+1), drops
// the extra row and, when there was one, writes the truncation notice to
// stderr after the data.
func printCapped(b *bytes.Buffer, arr string, limit int) {
	notice, _ := json.Marshal(fmt.Sprintf(truncNotice, limit, MaxLimit))
	fmt.Fprintf(b, "const r=%s;let t=false;if(r.length>%d){r.pop();t=true}print(EJSON.stringify(r, null, 2));if(t)void process.stderr.write(%s)", arr, limit, notice)
}

// BuildMongoScript returns the script for `mongosh --nodb --quiet --eval`.
// All user JSON is parsed then re-marshalled; the collection name is
// JSON-encoded. The script connects first: touching `db` before connect()
// throws in mongosh 2.5.
func BuildMongoScript(q MongoQuery) (string, error) {
	limit := q.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 0 || limit > MaxLimit {
		return "", fmt.Errorf("--limit phải trong khoảng 1..%d", MaxLimit) //znf:allow-lang
	}
	filter, err := objectFlag("--filter", q.Filter)
	if err != nil {
		return "", err
	}
	proj, err := objectFlag("--projection", q.Projection)
	if err != nil {
		return "", err
	}
	sort, err := objectFlag("--sort", q.Sort)
	if err != nil {
		return "", err
	}
	filterOrEmpty := filter
	if filterOrEmpty == "" {
		filterOrEmpty = "{}"
	}
	collB, err := json.Marshal(q.Coll)
	if err != nil {
		return "", err
	}
	coll := "d.getCollection(" + string(collB) + ")"

	verbosity := "executionStats"
	if q.PlanOnly {
		verbosity = "queryPlanner"
	}

	var b bytes.Buffer
	b.WriteString(mongoConnectLine(q.DB))
	switch q.Op {
	case "collections":
		subB, _ := json.Marshal(strings.ToLower(q.Coll))
		fmt.Fprintf(&b, `d.getCollectionInfos({type:"collection"}).map(c=>c.name).sort().filter(n=>n.toLowerCase().includes(%s)).forEach(n=>print(n))`, subB)
	case "doc":
		fmt.Fprintf(&b, "print(EJSON.stringify(%s.findOne(), null, 2))", coll)
	case "indexes":
		fmt.Fprintf(&b, "print(EJSON.stringify(%s.getIndexes(), null, 2))", coll)
	case "stats":
		// Sizes and counts only: stats() also carries per-index and storage
		// engine detail nobody asked for.
		fmt.Fprintf(&b, "const s=%s.stats();print(EJSON.stringify({ns:s.ns,count:s.count,size:s.size,avgObjSize:s.avgObjSize,storageSize:s.storageSize,nindexes:s.nindexes,totalIndexSize:s.totalIndexSize}, null, 2))", coll)
	case "count":
		if filter == "" {
			fmt.Fprintf(&b, "print(%s.estimatedDocumentCount())", coll)
		} else {
			fmt.Fprintf(&b, "print(%s.countDocuments(%s,{maxTimeMS:%d}))", coll, filterOrEmpty, QueryTimeoutMS)
		}
	case "find", "explain":
		// explain runs uncapped (FR-3.1): a limit would hide the real scan.
		if q.Op == "explain" && q.Pipeline != "" {
			pl, err := pipelineFlag(q, 0)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "print(EJSON.stringify(%s.explain(%q).aggregate(%s,{maxTimeMS:%d}), null, 2))", coll, verbosity, pl, QueryTimeoutMS)
			break
		}
		args := filterOrEmpty
		if proj != "" {
			args += "," + proj
		}
		cur := fmt.Sprintf("%s.find(%s)", coll, args)
		if sort != "" {
			cur += ".sort(" + sort + ")"
		}
		if q.Op == "explain" {
			cur += fmt.Sprintf(".maxTimeMS(%d)", QueryTimeoutMS)
			fmt.Fprintf(&b, "print(EJSON.stringify(%s.explain(%q), null, 2))", cur, verbosity)
		} else {
			cur += fmt.Sprintf(".limit(%d).maxTimeMS(%d)", limit+1, QueryTimeoutMS)
			printCapped(&b, cur+".toArray()", limit)
		}
	case "aggregate":
		pl, err := pipelineFlag(q, limit+1)
		if err != nil {
			return "", err
		}
		printCapped(&b, fmt.Sprintf("%s.aggregate(%s,{maxTimeMS:%d}).toArray()", coll, pl, QueryTimeoutMS), limit)
	default:
		return "", fmt.Errorf("op không hợp lệ: %q", q.Op) //znf:allow-lang
	}
	return b.String(), nil
}
