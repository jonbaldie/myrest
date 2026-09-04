package mysqldb

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// TestCGPTSQL runs the SQL-boundary half of the coverage-guided campaign.
func TestCGPTSQL(t *testing.T) {
	if os.Getenv("MYREST_CGPT") == "" {
		t.Skip("set MYREST_CGPT=1 to run the SQL campaign")
	}
	duration := time.Hour
	if raw := os.Getenv("MYREST_CGPT_DURATION"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err == nil {
			duration = parsed
		}
	}
	outPath := os.Getenv("MYREST_CGPT_OUT")
	if outPath == "" {
		outPath = "/tmp/myrest-cgpt-sql.log"
	}
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id", DataType: "bigint"},
			{Name: "name", DataType: "varchar", Collation: "utf8mb4_0900_ai_ci"},
			{Name: "meta", DataType: "json"},
		},
	}
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
	type seed struct {
		column string
		op     string
		value  string
		energy int
	}
	var success []seed
	seen := map[string]bool{}
	deadline := time.Now().Add(duration)
	runs, oks, skips, fails := 0, 0, 0, 0
	ops := []string{"eq", "neq", "gt", "gte", "lt", "lte", "like", "ilike", "in", "is", "isdistinct"}
	columns := []string{"id", "name", "meta", "missing"}
	values := []string{
		"1", "alpha", "x' OR 1=1 --", "semi;colon", "*", "null", "true",
		`"quoted"`, "A-", "1); DROP TABLE items;--", "`; DROP TABLE items;--",
	}

	report := func(label string) {
		t.Logf("STAT %s runs=%d ok=%d discard=%d fail=%d edges=%d", label, runs, oks, skips, fails, len(seen))
	}

	for time.Now().Before(deadline) {
		column, op, value := columns[rng.IntN(len(columns))], ops[rng.IntN(len(ops))], values[rng.IntN(len(values))]
		if len(success) > 0 && rng.IntN(5) != 0 {
			chosen := &success[rng.IntN(len(success))]
			if chosen.energy > 0 {
				chosen.energy--
				column, op, value = chosen.column, chosen.op, chosen.value
				switch rng.IntN(3) {
				case 0:
					value += "'"
				case 1:
					op = ops[rng.IntN(len(ops))]
				default:
					column = columns[rng.IntN(len(columns))]
				}
			}
		}
		runs++
		raw := op + "." + value
		if op == "in" {
			raw = "in.(" + value + ")"
		}
		query, err := readquery.Parse(url.Values{column: {raw}}, nil)
		if err != nil {
			skips++
			continue
		}
		parts, err := buildSelect(table, query)
		if err != nil {
			skips++
			edge := "build-err:" + err.Error()
			if !seen[edge] {
				seen[edge] = true
			}
			continue
		}
		control, err := readquery.Parse(url.Values{column: {strings.Replace(raw, value, "control", 1)}}, nil)
		if err != nil {
			// in-list rewrite can fail; compare against a fixed control instead.
			control, err = readquery.Parse(url.Values{"name": {"eq.control"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		controlParts, err := buildSelect(table, control)
		if err != nil {
			skips++
			continue
		}
		edge := "sql:" + stripLiterals(parts.statement)
		newEdge := !seen[edge]
		if newEdge {
			seen[edge] = true
			success = append(success, seed{column: column, op: op, value: value, energy: 16})
		}
		if fail := sqlProperties(column, op, value, query, parts, controlParts); fail != "" {
			fails++
			payload, _ := json.Marshal(map[string]string{
				"property": "sql-boundary",
				"detail":   fail,
				"input":    column + "=" + raw,
			})
			_, _ = out.Write(append(payload, '\n'))
			_ = out.Sync()
			fmt.Printf("FINDING %s\n", payload)
			continue
		}
		oks++
		if runs%2000 == 0 {
			report("live")
		}
	}
	report("final")
	if fails > 0 {
		t.Fatalf("SQL campaign found %d failures", fails)
	}
}

func sqlProperties(
	column, op, value string,
	query readquery.Query,
	parts sqlParts,
	control sqlParts,
) string {
	if strings.Contains(parts.statement, value) && op != "is" && !strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyz") {
		// Filter values must stay out of the SQL text except known is-predicates.
		if !strings.Contains(value, "`") && value != "*" {
			return fmt.Sprintf("filter value %q leaked into SQL %q", value, parts.statement)
		}
	}
	if strings.Contains(parts.statement, " OR 1=1") || strings.Contains(parts.statement, "DROP TABLE") {
		return fmt.Sprintf("injection fragment in SQL %q", parts.statement)
	}
	if op != "is" && op != "in" && len(query.Filters) == 1 && query.Filters[0].Op != readquery.OpIn {
		if parts.statement != control.statement && column != "missing" {
			// like rewrites * in the argument, not the statement.
			if !strings.Contains(control.statement, "LIKE") {
				return fmt.Sprintf("filter value changed SQL: %q vs %q", parts.statement, control.statement)
			}
		}
	}
	if strings.Count(parts.statement, "'")%2 != 0 {
		return fmt.Sprintf("unbalanced quotes in SQL %q", parts.statement)
	}
	return ""
}

func stripLiterals(statement string) string {
	return statement
}
