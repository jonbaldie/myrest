package readquery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/rows"
)

// HasRowSetFeatures reports whether the query needs a tabular RPC result.
// Argument keys stay outside this check; only read shaping counts.
func HasRowSetFeatures(query Query) bool {
	if query.Limit != nil || query.Offset > 0 || query.ExactCount {
		return true
	}
	if len(query.Filters)+len(query.Groups)+len(query.Order)+len(query.Embeds) > 0 {
		return true
	}
	return !query.SelectAll && len(query.Columns) > 0
}

// Shape applies filter, order, and page of query to a row set in memory.
// Column select and embed stay outside this step so join columns remain for
// nesting, then Project keeps the client columns.
func Shape(set []rows.Row, query Query) (Result, error) {
	filtered, err := filterRowSet(set, query.Filters, query.Groups)
	if err != nil {
		return Result{}, err
	}

	var total *int64
	if query.ExactCount {
		count := int64(len(filtered))
		total = &count
	}
	if err := sortRows(filtered, query.Order); err != nil {
		return Result{}, err
	}
	return Result{
		Rows:  pageRowSet(filtered, query.Offset, query.EffectiveLimit()),
		Total: total,
	}, nil
}

// Project keeps the selected columns and embed keys of each row.
func Project(set []rows.Row, query Query) ([]rows.Row, error) {
	if query.SelectAll || len(query.Columns) == 0 {
		return set, nil
	}
	keepEmbed := embedKeys(query.Embeds)
	out := make([]rows.Row, len(set))
	for i, row := range set {
		projected, err := projectOne(row, query.Columns, keepEmbed)
		if err != nil {
			return nil, err
		}
		out[i] = projected
	}
	return out, nil
}

func embedKeys(embeds []Embed) map[string]bool {
	keys := map[string]bool{}
	for _, embed := range embeds {
		keys[embed.Key()] = true
	}
	return keys
}

func projectOne(row rows.Row, columns []Column, keepEmbed map[string]bool) (rows.Row, error) {
	names := make([]string, 0, len(columns)+len(keepEmbed))
	values := make([]any, 0, len(columns)+len(keepEmbed))
	for _, column := range columns {
		if column.Path != nil {
			return rows.Row{}, UnsupportedFeature{Message: "JSON path select on RPC row sets is not available yet"}
		}
		value, ok := lookupColumn(row, column.Name)
		if !ok {
			return rows.Row{}, ColumnNotFound{Name: column.Name}
		}
		name := column.Name
		if column.Alias != "" {
			name = column.Alias
		}
		names = append(names, name)
		values = append(values, value)
	}
	for _, column := range row.Columns {
		if !keepEmbed[column] {
			continue
		}
		value, _ := lookupColumn(row, column)
		names = append(names, column)
		values = append(values, value)
	}
	return rows.Row{Columns: names, Values: values}, nil
}

func filterRowSet(set []rows.Row, filters []Filter, groups []Group) ([]rows.Row, error) {
	filtered := make([]rows.Row, 0, len(set))
	for _, row := range set {
		ok, err := rowMatches(row, filters, groups)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func rowMatches(row rows.Row, filters []Filter, groups []Group) (bool, error) {
	for _, filter := range filters {
		ok, err := matchFilter(row, filter)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	for _, group := range groups {
		ok, err := matchGroup(row, group)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func matchGroup(row rows.Row, group Group) (bool, error) {
	if group.Or {
		return matchOrGroup(row, group)
	}
	ok, err := rowMatches(row, group.Filters, group.Groups)
	if err != nil {
		return false, err
	}
	return negate(ok, group.Negated), nil
}

func matchOrGroup(row rows.Row, group Group) (bool, error) {
	for _, filter := range group.Filters {
		ok, err := matchFilter(row, filter)
		if err != nil {
			return false, err
		}
		if ok {
			return negate(true, group.Negated), nil
		}
	}
	for _, nested := range group.Groups {
		ok, err := matchGroup(row, nested)
		if err != nil {
			return false, err
		}
		if ok {
			return negate(true, group.Negated), nil
		}
	}
	return negate(false, group.Negated), nil
}

func negate(value, negated bool) bool {
	if negated {
		return !value
	}
	return value
}

func matchFilter(row rows.Row, filter Filter) (bool, error) {
	if filter.Path != nil {
		return false, UnsupportedFeature{Message: "JSON path filters on RPC row sets are not available yet"}
	}
	value, ok := lookupColumn(row, filter.Column)
	if !ok {
		return false, ColumnNotFound{Name: filter.Column}
	}
	matched, err := compareValue(value, filter)
	if err != nil {
		return false, err
	}
	if filter.Negated {
		return !matched, nil
	}
	return matched, nil
}

func compareValue(value any, filter Filter) (bool, error) {
	switch filter.Op {
	case OpEq:
		return valuesEqual(value, filter.Value), nil
	case OpNeq:
		return !valuesEqual(value, filter.Value), nil
	case OpGt, OpGte, OpLt, OpLte:
		return compareOrderedOp(value, filter.Value, filter.Op), nil
	case OpLike, OpILike:
		return compareLike(value, filter.Value, filter.Op == OpILike), nil
	case OpIn:
		return compareIn(value, filter.Values), nil
	case OpIs:
		return matchIs(value, filter.Value)
	case OpIsDistinct:
		return compareIsDistinct(value, filter.Value), nil
	default:
		return false, fmt.Errorf("filter operator %q is not supported on a row set", filter.Op)
	}
}

func compareOrderedOp(value any, raw string, op Operator) bool {
	cmp, ok := compareOrdered(value, raw)
	if !ok {
		return false
	}
	switch op {
	case OpGt:
		return cmp > 0
	case OpGte:
		return cmp >= 0
	case OpLt:
		return cmp < 0
	default:
		return cmp <= 0
	}
}

func compareLike(value any, pattern string, caseInsensitive bool) bool {
	text, ok := valueAsString(value)
	if !ok {
		return false
	}
	return matchLike(text, pattern, caseInsensitive)
}

func compareIn(value any, candidates []string) bool {
	for _, candidate := range candidates {
		if valuesEqual(value, candidate) {
			return true
		}
	}
	return false
}

func compareIsDistinct(value any, raw string) bool {
	if raw == "null" {
		return value != nil
	}
	return !valuesEqual(value, raw)
}

func matchIs(value any, want string) (bool, error) {
	switch strings.ToLower(want) {
	case "null", "unknown":
		return value == nil, nil
	case "not_null":
		return value != nil, nil
	case "true":
		return boolValue(value), nil
	case "false":
		return value != nil && !boolValue(value), nil
	default:
		return false, ParseFailure{Message: "is operator value must be null, not_null, true, false, or unknown"}
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	default:
		return false
	}
}

func matchLike(text, pattern string, caseInsensitive bool) bool {
	pattern = strings.ReplaceAll(pattern, "*", "%")
	parts := strings.Split(pattern, "%")
	if caseInsensitive {
		text = strings.ToLower(text)
		lowered := make([]string, len(parts))
		for i, part := range parts {
			lowered[i] = strings.ToLower(part)
		}
		parts = lowered
	}
	return matchLikeParts(text, parts)
}

func matchLikeParts(text string, parts []string) bool {
	if len(parts) == 1 {
		return text == parts[0]
	}
	if !strings.HasPrefix(text, parts[0]) {
		return false
	}
	cursor := len(parts[0])
	last := len(parts) - 1
	for i := 1; i < last; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		index := strings.Index(text[cursor:], part)
		if index < 0 {
			return false
		}
		cursor += index + len(part)
	}
	return strings.HasSuffix(text[cursor:], parts[last])
}

func valuesEqual(value any, raw string) bool {
	if value == nil {
		return raw == "null"
	}
	if text, ok := valueAsString(value); ok && text == raw {
		return true
	}
	if left, ok := asFloat(value); ok {
		right, err := strconv.ParseFloat(raw, 64)
		return err == nil && left == right
	}
	left, ok := value.(bool)
	if !ok {
		return false
	}
	right, err := strconv.ParseBool(raw)
	return err == nil && left == right
}

func compareOrdered(value any, raw string) (int, bool) {
	if left, ok := asFloat(value); ok {
		right, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, false
		}
		return cmpFloat(left, right), true
	}
	left, ok := valueAsString(value)
	if !ok {
		return 0, false
	}
	return strings.Compare(left, raw), true
}

func cmpFloat(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return asFloatSmallInt(value)
	}
}

func asFloatSmallInt(value any) (float64, bool) {
	switch typed := value.(type) {
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func valueAsString(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		if number, ok := asFloat(value); ok {
			return formatNumber(number), true
		}
		return fmt.Sprint(value), true
	}
}

func formatNumber(number float64) string {
	if number == float64(int64(number)) {
		return strconv.FormatInt(int64(number), 10)
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

func sortRows(set []rows.Row, order []Order) error {
	if len(order) == 0 {
		return nil
	}
	var sortErr error
	sort.SliceStable(set, func(i, j int) bool {
		if sortErr != nil {
			return false
		}
		less, err := rowLess(set[i], set[j], order)
		if err != nil {
			sortErr = err
			return false
		}
		return less
	})
	return sortErr
}

func rowLess(left, right rows.Row, order []Order) (bool, error) {
	for _, key := range order {
		if key.Path != nil {
			return false, UnsupportedFeature{Message: "JSON path order on RPC row sets is not available yet"}
		}
		lv, okLeft := lookupColumn(left, key.Column)
		rv, okRight := lookupColumn(right, key.Column)
		if !okLeft || !okRight {
			return false, ColumnNotFound{Name: key.Column}
		}
		cmp := compareAny(lv, rv)
		if cmp == 0 {
			continue
		}
		if key.Desc {
			return cmp > 0, nil
		}
		return cmp < 0, nil
	}
	return false, nil
}

func compareAny(left, right any) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if lf, ok := asFloat(left); ok {
		if rf, ok := asFloat(right); ok {
			return cmpFloat(lf, rf)
		}
	}
	ls, _ := valueAsString(left)
	rs, _ := valueAsString(right)
	return strings.Compare(ls, rs)
}

func pageRowSet(set []rows.Row, offset uint64, limit *uint64) []rows.Row {
	if offset > 0 {
		if offset >= uint64(len(set)) {
			return []rows.Row{}
		}
		set = set[offset:]
	}
	if limit != nil && uint64(len(set)) > *limit {
		set = set[:*limit]
	}
	return set
}

func lookupColumn(row rows.Row, name string) (any, bool) {
	for i, column := range row.Columns {
		if column != name {
			continue
		}
		if i < len(row.Values) {
			return row.Values[i], true
		}
		return nil, true
	}
	return nil, false
}
