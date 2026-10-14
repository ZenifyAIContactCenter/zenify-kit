package dbread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Query limits and the env var the generated script reads the Mongo URL from.
const (
	DefaultLimit     = 50
	MaxLimit         = 200
	QueryTimeoutMS   = 30000
	MongoURLEnv      = "ZNF_MONGO_URL"
	mongoConnectLine = "const d=connect(process.env." + MongoURLEnv + ").getSiblingDB('3csoft');"
)

// MongoQuery is one structured read. Op is one of collections|doc|count|find|
// aggregate|explain. JSON fields are the raw CLI flag strings (empty = absent).
type MongoQuery struct {
	Op, Coll, Filter, Projection, Sort, Pipeline string
	Limit                                        int
}

// decodeJSON parses s with numbers preserved verbatim (no float64 rounding of
// large ints).
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

// objectFlag validates a JSON-object flag and returns its re-marshalled form,
// or "" when the flag is empty.
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
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

// pipelineFlag validates --pipeline, rejects write stages and appends the
// $limit stage. Returns the marshalled array.
func pipelineFlag(q MongoQuery, limit int) (string, error) {
	p, err := parseJSONArray(q.Pipeline)
	if err != nil {
		return "", fmt.Errorf("--pipeline không phải mảng JSON hợp lệ: %v", err) //znf:allow-lang
	}
	if err := CheckPipeline(p); err != nil {
		return "", err
	}
	p = append(p, map[string]any{"$limit": limit})
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
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
	if filter == "" {
		filter = "{}"
	}
	collB, err := json.Marshal(q.Coll)
	if err != nil {
		return "", err
	}
	coll := "d.getCollection(" + string(collB) + ")"

	var b bytes.Buffer
	b.WriteString(mongoConnectLine)
	switch q.Op {
	case "collections":
		subB, _ := json.Marshal(strings.ToLower(q.Coll))
		fmt.Fprintf(&b, "d.getCollectionNames().sort().filter(n=>n.toLowerCase().includes(%s)).forEach(n=>print(n))", subB)
	case "doc":
		fmt.Fprintf(&b, "print(EJSON.stringify(%s.findOne(), null, 2))", coll)
	case "count":
		if q.Filter == "" {
			fmt.Fprintf(&b, "print(%s.estimatedDocumentCount())", coll)
		} else {
			fmt.Fprintf(&b, "print(%s.countDocuments(%s,{maxTimeMS:%d}))", coll, filter, QueryTimeoutMS)
		}
	case "find", "explain":
		if q.Op == "explain" && q.Pipeline != "" {
			pl, err := pipelineFlag(q, limit)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "print(EJSON.stringify(%s.explain(\"queryPlanner\").aggregate(%s,{maxTimeMS:%d}), null, 2))", coll, pl, QueryTimeoutMS)
			break
		}
		args := filter
		if proj != "" {
			args += "," + proj
		}
		cur := fmt.Sprintf("%s.find(%s)", coll, args)
		if sort != "" {
			cur += ".sort(" + sort + ")"
		}
		cur += fmt.Sprintf(".limit(%d).maxTimeMS(%d)", limit, QueryTimeoutMS)
		if q.Op == "explain" {
			fmt.Fprintf(&b, "print(EJSON.stringify(%s.explain(\"queryPlanner\"), null, 2))", cur)
		} else {
			fmt.Fprintf(&b, "print(EJSON.stringify(%s.toArray(), null, 2))", cur)
		}
	case "aggregate":
		pl, err := pipelineFlag(q, limit)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "print(EJSON.stringify(%s.aggregate(%s,{maxTimeMS:%d}).toArray(), null, 2))", coll, pl, QueryTimeoutMS)
	default:
		return "", fmt.Errorf("op không hợp lệ: %q", q.Op) //znf:allow-lang
	}
	return b.String(), nil
}
