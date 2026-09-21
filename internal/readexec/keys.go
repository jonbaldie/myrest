package readexec

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
)

func uniqueKeyTuples(read []rows.Row, columns []string) [][]any {
	seen := map[string]bool{}
	var keys [][]any
	for _, row := range read {
		key := rowKey(row, columns)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, rowValues(row, columns))
	}
	return keys
}

func rowValues(row rows.Row, columns []string) []any {
	values := make([]any, len(columns))
	for i, column := range columns {
		values[i] = columnValue(row, column)
	}
	return values
}

func columnValue(row rows.Row, column string) any {
	for i, name := range row.Columns {
		if name == column {
			if i < len(row.Values) {
				return row.Values[i]
			}
			return nil
		}
	}
	return nil
}

func rowKey(row rows.Row, columns []string) string {
	if len(columns) == 0 {
		return ""
	}
	parts := make([]string, len(columns))
	for i, column := range columns {
		value := columnValue(row, column)
		if value == nil {
			return ""
		}
		parts[i] = stringifyValue(value)
	}
	if len(parts) == 1 {
		if parts[0] == "" {
			return "\x00"
		}
		if strings.HasPrefix(parts[0], "\x00") {
			return "\x00" + parts[0]
		}
		return parts[0]
	}
	var key strings.Builder
	for _, part := range parts {
		key.WriteString(strconv.Itoa(len(part)))
		key.WriteByte(':')
		key.WriteString(part)
	}
	return key.String()
}

// keyCondition limits a read to the given key tuples. A single key column uses
// one IN list; a composite key uses an OR of AND groups, one group per tuple,
// so every key column of the foreign key takes part in the match. It reports
// false when no key columns are named, because no condition can hold the read
// down to the related rows.
func keyCondition(columns []string, keys [][]any) ([]readquery.Filter, []readquery.Group, bool) {
	switch {
	case len(columns) == 0 || len(keys) == 0:
		return nil, nil, false
	case len(columns) == 1:
		return []readquery.Filter{{
			Column: columns[0],
			Op:     readquery.OpIn,
			Values: stringifyKeys(keys),
		}}, nil, true
	}
	tuples := make([]readquery.Group, 0, len(keys))
	for _, key := range keys {
		tuples = append(tuples, readquery.Group{Filters: tupleFilters(columns, key)})
	}
	return nil, []readquery.Group{{Or: true, Groups: tuples}}, true
}

func tupleFilters(columns []string, key []any) []readquery.Filter {
	filters := make([]readquery.Filter, 0, len(columns))
	for i, column := range columns {
		filters = append(filters, readquery.Filter{
			Column: column,
			Op:     readquery.OpEq,
			Value:  stringifyValue(key[i]),
		})
	}
	return filters
}

func stringifyKeys(keys [][]any) []string {
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = stringifyValue(key[0])
	}
	return values
}

func stringifyValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

func indexRows(read []rows.Row, columns []string) map[string]rows.Row {
	indexed := make(map[string]rows.Row, len(read))
	for _, row := range read {
		indexed[rowKey(row, columns)] = row
	}
	return indexed
}

func groupRows(read []rows.Row, columns []string) map[string][]rows.Row {
	grouped := map[string][]rows.Row{}
	for _, row := range read {
		key := rowKey(row, columns)
		grouped[key] = append(grouped[key], row)
	}
	return grouped
}
