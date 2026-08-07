package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

const (
	// codeNoRelationship is the parity-target code for a missing embed path.
	codeNoRelationship = "PGRST200"
	// codeAmbiguousRelationship is the parity-target code for more than one path.
	codeAmbiguousRelationship = "PGRST201"
)

// plannedEmbed is one embed resolved against the schema cache.
type plannedEmbed struct {
	ask          readquery.Embed
	relationship schemacache.Relationship
	target       schemacache.Table
	children     []plannedEmbed
}

func (s *Service) planEmbeds(
	role schemacache.Role,
	origin schemacache.TableID,
	asks []readquery.Embed,
) ([]plannedEmbed, error) {
	planned := make([]plannedEmbed, 0, len(asks))
	for _, ask := range asks {
		rel, err := s.cache.ResolveEmbed(role, origin, ask.Resource, ask.Hint)
		if err != nil {
			return nil, err
		}
		target, ok := s.cache.Resource(role, rel.Target)
		if !ok {
			return nil, schemacache.RelationshipMissing{Origin: origin, Target: ask.Resource}
		}
		children, err := s.planEmbeds(role, target.ID, ask.Embeds)
		if err != nil {
			return nil, err
		}
		planned = append(planned, plannedEmbed{
			ask: ask, relationship: rel, target: target, children: children,
		})
	}
	return planned, nil
}

func writeEmbedPlanFailure(writer http.ResponseWriter, err error) bool {
	switch failure := err.(type) {
	case schemacache.RelationshipMissing:
		writeFailure(writer, http.StatusBadRequest, codeNoRelationship, failure.Error())
		return true
	case schemacache.ComputedRelationship:
		writeUnsupportedFeature(writer, failure.Error())
		return true
	case schemacache.RelationshipAmbiguous:
		writeFailureExtra(
			writer,
			http.StatusMultipleChoices,
			codeAmbiguousRelationship,
			failure.Error(),
			ambiguousDetails(failure),
			ambiguousHint(failure),
		)
		return true
	default:
		return false
	}
}

func ambiguousDetails(failure schemacache.RelationshipAmbiguous) []map[string]string {
	details := make([]map[string]string, 0, len(failure.Options))
	for _, option := range failure.Options {
		details = append(details, map[string]string{
			"cardinality":  cardinalityName(option.Cardinality),
			"embedding":    failure.Origin.Name + " with " + failure.Target,
			"relationship": option.Name,
		})
	}
	return details
}

func ambiguousHint(failure schemacache.RelationshipAmbiguous) string {
	options := make([]string, 0, len(failure.Options))
	for _, option := range failure.Options {
		options = append(options, failure.Target+"!"+option.Name)
	}
	return "Try changing '" + failure.Target + "' to one of the following: '" +
		strings.Join(options, "', '") + "'. Find the desired relationship in the 'details' key."
}

func cardinalityName(cardinality schemacache.Cardinality) string {
	switch cardinality {
	case schemacache.ManyToOne:
		return "many-to-one"
	case schemacache.OneToMany:
		return "one-to-many"
	case schemacache.ManyToMany:
		return "many-to-many"
	case schemacache.OneToOne:
		return "one-to-one"
	default:
		return "unknown"
	}
}

// withJoinColumns adds the parent columns an embed plan needs, and returns the
// names that were injected so the response can drop them again.
func withJoinColumns(table schemacache.Table, query readquery.Query, plan []plannedEmbed) (readquery.Query, []string) {
	if len(plan) == 0 || (len(query.Columns) == 0 && query.SelectAll) {
		return query, nil
	}
	needed := originColumnsNeeded(plan)
	have := selectedNames(query.Columns)
	var injected []string
	for _, column := range table.Columns {
		if !needed[column.Name] || have[column.Name] {
			continue
		}
		query.Columns = append(query.Columns, readquery.Column{Name: column.Name})
		injected = append(injected, column.Name)
		have[column.Name] = true
	}
	return query, injected
}

func originColumnsNeeded(plan []plannedEmbed) map[string]bool {
	needed := map[string]bool{}
	for _, embed := range plan {
		for _, column := range embed.relationship.OriginColumns {
			needed[column] = true
		}
	}
	return needed
}

func selectedNames(columns []readquery.Column) map[string]bool {
	have := map[string]bool{}
	for _, column := range columns {
		have[column.Name] = true
	}
	return have
}

func dropInjectedColumns(read []rows.Row, injected []string) []rows.Row {
	if len(injected) == 0 {
		return read
	}
	drop := map[string]bool{}
	for _, name := range injected {
		drop[name] = true
	}
	cleaned := make([]rows.Row, len(read))
	for i, row := range read {
		var columns []string
		var values []any
		for j, column := range row.Columns {
			if drop[column] {
				continue
			}
			columns = append(columns, column)
			if j < len(row.Values) {
				values = append(values, row.Values[j])
			} else {
				values = append(values, nil)
			}
		}
		cleaned[i] = rows.Row{Columns: columns, Values: values}
	}
	return cleaned
}

func (s *Service) nestEmbeds(
	ctx context.Context,
	role schemacache.Role,
	parent schemacache.Table,
	parentRows []rows.Row,
	plan []plannedEmbed,
) ([]rows.Row, error) {
	for _, embed := range plan {
		var err error
		parentRows, err = s.nestOneEmbed(ctx, role, parent, parentRows, embed)
		if err != nil {
			return nil, err
		}
	}
	return parentRows, nil
}

func (s *Service) nestOneEmbed(
	ctx context.Context,
	role schemacache.Role,
	parent schemacache.Table,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	if len(parentRows) == 0 {
		return parentRows, nil
	}
	switch embed.relationship.Cardinality {
	case schemacache.ManyToOne, schemacache.OneToOne:
		return s.nestToOne(ctx, role, parentRows, embed)
	case schemacache.OneToMany:
		return s.nestToMany(ctx, role, parentRows, embed)
	case schemacache.ManyToMany:
		return s.nestManyToMany(ctx, role, parentRows, embed)
	default:
		return nil, fmt.Errorf("unknown embed cardinality %d", embed.relationship.Cardinality)
	}
}

func (s *Service) nestToOne(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	keys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	related, err := s.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, keys)
	if err != nil {
		return nil, err
	}
	byKey := indexRows(related, embed.relationship.TargetColumns)
	keyName := embed.ask.Key()
	for i, row := range parentRows {
		key := rowKey(row, embed.relationship.OriginColumns)
		if child, ok := byKey[key]; ok {
			parentRows[i] = appendColumn(row, keyName, projectEmbedRow(child, embed.ask))
		} else {
			parentRows[i] = appendColumn(row, keyName, nil)
		}
	}
	return parentRows, nil
}

func (s *Service) nestToMany(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	keys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	related, err := s.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, keys)
	if err != nil {
		return nil, err
	}
	grouped := groupRows(related, embed.relationship.TargetColumns)
	keyName := embed.ask.Key()
	for i, row := range parentRows {
		key := rowKey(row, embed.relationship.OriginColumns)
		children := pageRows(grouped[key], embed.ask)
		projected := make([]rows.Row, len(children))
		for j, child := range children {
			projected[j] = projectEmbedRow(child, embed.ask)
		}
		parentRows[i] = appendColumn(row, keyName, projected)
	}
	return parentRows, nil
}

func (s *Service) nestManyToMany(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	parentKeys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	joinTable, ok := s.cache.Resource(role, embed.relationship.JoinTable)
	if !ok {
		return nil, schemacache.RelationshipMissing{
			Origin: embed.relationship.Origin,
			Target: embed.ask.Resource,
		}
	}
	if len(embed.relationship.JoinOriginColumns) != 1 {
		return nil, fmt.Errorf("composite many-to-many join keys are not available yet")
	}
	joinQuery := readquery.Query{
		SelectAll: true,
		Columns: columnList(append(
			append([]string{}, embed.relationship.JoinOriginColumns...),
			embed.relationship.JoinTargetColumns...,
		)),
		Filters: []readquery.Filter{{
			Column: embed.relationship.JoinOriginColumns[0],
			Op:     readquery.OpIn,
			Values: stringifyKeys(parentKeys),
		}},
	}
	joinRead, err := s.reader.Read(ctx, role, joinTable, joinQuery)
	if err != nil {
		return nil, err
	}
	targetKeys := uniqueKeyTuples(joinRead.Rows, embed.relationship.JoinTargetColumns)
	related, err := s.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, targetKeys)
	if err != nil {
		return nil, err
	}
	relatedByKey := indexRows(related, embed.relationship.TargetColumns)
	grouped := map[string][]rows.Row{}
	for _, link := range joinRead.Rows {
		parentKey := rowKey(link, embed.relationship.JoinOriginColumns)
		targetKey := rowKey(link, embed.relationship.JoinTargetColumns)
		if child, ok := relatedByKey[targetKey]; ok {
			grouped[parentKey] = append(grouped[parentKey], child)
		}
	}
	keyName := embed.ask.Key()
	for i, row := range parentRows {
		key := rowKey(row, embed.relationship.OriginColumns)
		children := pageRows(grouped[key], embed.ask)
		projected := make([]rows.Row, len(children))
		for j, child := range children {
			projected[j] = projectEmbedRow(child, embed.ask)
		}
		parentRows[i] = appendColumn(row, keyName, projected)
	}
	return parentRows, nil
}

func (s *Service) readByKeys(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	keyColumns []string,
	keys [][]any,
) ([]rows.Row, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if len(keyColumns) != 1 {
		return nil, fmt.Errorf("composite embed keys are not available yet")
	}
	// Limit and offset apply per parent row after grouping, not to the batch.
	query := readquery.Query{
		Columns:   embed.ask.Columns,
		SelectAll: len(embed.ask.Columns) == 0,
		Filters: append([]readquery.Filter{{
			Column: keyColumns[0],
			Op:     readquery.OpIn,
			Values: stringifyKeys(keys),
		}}, embed.ask.Filters...),
		Groups: embed.ask.Groups,
		Order:  embed.ask.Order,
		Embeds: embedAsks(embed.children),
	}
	childPlan := embed.children
	query, injected := withJoinColumns(embed.target, query, childPlan)
	query, _ = ensureColumns(embed.target, query, keyColumns)
	read, err := s.reader.Read(ctx, role, embed.target, query)
	if err != nil {
		return nil, err
	}
	nested, err := s.nestEmbeds(ctx, role, embed.target, read.Rows, childPlan)
	if err != nil {
		return nil, err
	}
	// Keep key columns for grouping; projectEmbedRow drops them for the client.
	return dropInjectedColumns(nested, injected), nil
}

// ensureColumns adds named columns to the select list when the client did not
// ask for every column.
func ensureColumns(table schemacache.Table, query readquery.Query, names []string) (readquery.Query, []string) {
	if len(query.Columns) == 0 && query.SelectAll {
		return query, nil
	}
	have := map[string]bool{}
	for _, column := range query.Columns {
		have[column.Name] = true
	}
	var injected []string
	for _, name := range names {
		if have[name] {
			continue
		}
		if _, ok := columnOf(table, name); !ok {
			continue
		}
		query.Columns = append(query.Columns, readquery.Column{Name: name})
		injected = append(injected, name)
		have[name] = true
	}
	return query, injected
}

func columnOf(table schemacache.Table, name string) (schemacache.Column, bool) {
	for _, column := range table.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return schemacache.Column{}, false
}

// projectEmbedRow keeps the columns the client asked for, plus nested embeds.
func projectEmbedRow(row rows.Row, ask readquery.Embed) rows.Row {
	if len(ask.Columns) == 0 {
		return row
	}
	keep := map[string]bool{}
	for _, column := range ask.Columns {
		name := column.Name
		if column.Alias != "" {
			name = column.Alias
		}
		keep[name] = true
	}
	for _, nested := range ask.Embeds {
		keep[nested.Key()] = true
	}
	var columns []string
	var values []any
	for i, column := range row.Columns {
		if !keep[column] {
			continue
		}
		columns = append(columns, column)
		if i < len(row.Values) {
			values = append(values, row.Values[i])
		} else {
			values = append(values, nil)
		}
	}
	return rows.Row{Columns: columns, Values: values}
}

func embedAsks(plan []plannedEmbed) []readquery.Embed {
	asks := make([]readquery.Embed, len(plan))
	for i, embed := range plan {
		asks[i] = embed.ask
	}
	return asks
}

func pageRows(children []rows.Row, ask readquery.Embed) []rows.Row {
	if children == nil {
		children = []rows.Row{}
	}
	children = sortRows(children, ask.Order)
	if ask.Offset > 0 {
		if ask.Offset >= uint64(len(children)) {
			return []rows.Row{}
		}
		children = children[ask.Offset:]
	}
	if ask.Limit != nil && uint64(len(children)) > *ask.Limit {
		children = children[:*ask.Limit]
	}
	return children
}

func sortRows(children []rows.Row, order []readquery.Order) []rows.Row {
	if len(order) == 0 || len(children) < 2 {
		return children
	}
	sorted := append([]rows.Row{}, children...)
	for i := len(order) - 1; i >= 0; i-- {
		key := order[i]
		sort.SliceStable(sorted, func(a, b int) bool {
			left := stringifyValue(columnValue(sorted[a], key.Column))
			right := stringifyValue(columnValue(sorted[b], key.Column))
			if key.Desc {
				return left > right
			}
			return left < right
		})
	}
	return sorted
}

func columnList(names []string) []readquery.Column {
	columns := make([]readquery.Column, 0, len(names))
	for _, name := range names {
		columns = append(columns, readquery.Column{Name: name})
	}
	return columns
}

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
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = stringifyValue(columnValue(row, column))
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
	}
	return strings.Join(parts, "\x1f")
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
		return strconv.FormatInt(int64(typed), 10)
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

func appendColumn(row rows.Row, name string, value any) rows.Row {
	columns := append(append([]string{}, row.Columns...), name)
	values := append(append([]any{}, row.Values...), value)
	return rows.Row{Columns: columns, Values: values}
}
