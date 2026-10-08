package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// plannedEmbed is one embed resolved against the schema cache. It holds every
// schema fact its execution reads, so execution never reads the cache again.
type plannedEmbed struct {
	ask          readquery.Embed
	relationship schemacache.Relationship
	target       schemacache.Table
	// joinTable is the many-to-many join table, when the role can read it.
	joinTable    schemacache.Table
	joinReadable bool
	children     []plannedEmbed
}

// planEmbeds resolves embeds from one schema-cache snapshot: the snapshot the
// request was admitted from.
func planEmbeds(
	snapshot schemacache.Snapshot,
	role schemacache.Role,
	origin schemacache.TableID,
	asks []readquery.Embed,
) ([]plannedEmbed, error) {
	planned := make([]plannedEmbed, 0, len(asks))
	for _, ask := range asks {
		rel, err := schemacache.ResolveEmbedIn(snapshot, role, origin, ask.Resource, ask.Hint)
		if err != nil {
			return nil, err
		}
		if err := checkSpreadAggregate(ask, rel); err != nil {
			return nil, err
		}
		target, ok := schemacache.TableWithPrivilegeFrom(snapshot, role, rel.Target, "SELECT")
		if !ok {
			return nil, schemacache.RelationshipMissing{Origin: origin, Target: ask.Resource}
		}
		children, err := planEmbeds(snapshot, role, target.ID, ask.Embeds)
		if err != nil {
			return nil, err
		}
		embed := plannedEmbed{
			ask: ask, relationship: rel, target: target, children: children,
		}
		if rel.Cardinality == schemacache.ManyToMany {
			embed.joinTable, embed.joinReadable = schemacache.TableWithPrivilegeFrom(
				snapshot, role, rel.JoinTable, "SELECT",
			)
		}
		planned = append(planned, embed)
	}
	return planned, nil
}

// spreadAggregateRefused is the parity-target refuse for aggregates inside a
// to-many or many-to-many spread embed.
type spreadAggregateRefused struct{}

func (spreadAggregateRefused) Error() string { return msgSpreadAggregate }

// spreadNotSupported is a spread embed shape this ticket does not implement.
type spreadNotSupported struct{}

func (spreadNotSupported) Error() string { return msgSpreadNotSupported }

func checkSpreadAggregate(ask readquery.Embed, rel schemacache.Relationship) error {
	if !ask.Spread {
		return nil
	}
	if readquery.EmbedHasAggregates(ask) &&
		(rel.Cardinality == schemacache.OneToMany || rel.Cardinality == schemacache.ManyToMany) {
		return spreadAggregateRefused{}
	}
	return spreadNotSupported{}
}

func writeEmbedPlanFailure(writer http.ResponseWriter, err error) bool {
	var missing schemacache.RelationshipMissing
	if errors.As(err, &missing) {
		writeFailure(writer, http.StatusBadRequest, codeNoRelationship, missing.Error())
		return true
	}
	var computed schemacache.ComputedRelationship
	if errors.As(err, &computed) {
		writeUnsupportedFeature(writer, computed.Error())
		return true
	}
	var ambiguous schemacache.RelationshipAmbiguous
	if errors.As(err, &ambiguous) {
		writeFailureExtra(
			writer,
			http.StatusMultipleChoices,
			codeAmbiguousRelationship,
			ambiguous.Error(),
			ambiguousDetails(ambiguous),
			ambiguousHint(ambiguous),
		)
		return true
	}
	var spreadAgg spreadAggregateRefused
	if errors.As(err, &spreadAgg) {
		writeFailureExtra(
			writer,
			http.StatusBadRequest,
			codeSpreadAggregate,
			msgSpreadAggregate,
			detailsSpreadAggregate,
			nil,
		)
		return true
	}
	var spread spreadNotSupported
	if errors.As(err, &spread) {
		writeUnsupportedFeature(writer, spread.Error())
		return true
	}
	return false
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
		options = append(options, failure.Target+"!"+optionHint(failure.Origin, option))
	}
	return "Try changing '" + failure.Target + "' to one of the following: '" +
		strings.Join(options, "', '") + "'. Find the desired relationship in the 'details' key."
}

// optionHint gives the hint that selects one option. On a self-relationship
// the two directions share the constraint name, so the one-to-many option
// suggests the key column that selects it instead.
func optionHint(origin schemacache.TableID, option schemacache.Relationship) string {
	if option.Cardinality == schemacache.OneToMany && option.Target == origin &&
		len(option.TargetColumns) == 1 {
		return option.TargetColumns[0]
	}
	return option.Name
}

func cardinalityName(cardinality schemacache.Cardinality) string {
	switch cardinality {
	case schemacache.ManyToOne:
		return "many-to-one"
	case schemacache.OneToMany:
		return "one-to-many"
	case schemacache.ManyToMany:
		return "many-to-many"
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
		if column.Agg != "" || column.Name == "" {
			continue
		}
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
	case schemacache.ManyToOne:
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
	return attachGroupedEmbeds(parentRows, embed, grouped), nil
}

func (s *Service) nestManyToMany(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	parentKeys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	links, err := s.loadManyToManyLinks(ctx, role, embed, parentKeys)
	if err != nil {
		return nil, err
	}
	if hasAggregateColumn(embed.ask.Columns) {
		grouped, err := s.readManyToManyAggregates(ctx, role, parentRows, embed, links)
		if err != nil {
			return nil, err
		}
		return attachGroupedEmbeds(parentRows, embed, grouped), nil
	}

	targetKeys := uniqueKeyTuples(links, embed.relationship.JoinTargetColumns)
	related, err := s.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, targetKeys)
	if err != nil {
		return nil, err
	}
	parentsByTarget := map[string][]string{}
	for _, link := range links {
		parentKey := rowKey(link, embed.relationship.JoinOriginColumns)
		targetKey := rowKey(link, embed.relationship.JoinTargetColumns)
		parentsByTarget[targetKey] = append(parentsByTarget[targetKey], parentKey)
	}
	grouped := groupManyToMany(related, parentsByTarget, embed.relationship.TargetColumns)
	return attachGroupedEmbeds(parentRows, embed, grouped), nil
}

func (s *Service) loadManyToManyLinks(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	parentKeys [][]any,
) ([]rows.Row, error) {
	if !embed.joinReadable {
		return nil, schemacache.RelationshipMissing{
			Origin: embed.relationship.Origin,
			Target: embed.ask.Resource,
		}
	}
	joinFilters, joinGroups, ok := keyCondition(embed.relationship.JoinOriginColumns, parentKeys)
	if !ok {
		return nil, nil
	}
	joinQuery := readquery.Query{
		SelectAll: true,
		Columns: columnList(append(
			append([]string{}, embed.relationship.JoinOriginColumns...),
			embed.relationship.JoinTargetColumns...,
		)),
		Filters: joinFilters,
		Groups:  joinGroups,
	}
	joinRead, err := s.reader.Read(ctx, role, embed.joinTable, joinQuery)
	if err != nil {
		return nil, err
	}
	return joinRead.Rows, nil
}

func hasAggregateColumn(columns []readquery.Column) bool {
	for _, column := range columns {
		if column.Agg != "" {
			return true
		}
	}
	return false
}

// readManyToManyAggregates reads each parent's targets in one scope. A single
// batch groups by target keys, not by the parent that owns each target.
func (s *Service) readManyToManyAggregates(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
	links []rows.Row,
) (map[string][]rows.Row, error) {
	keysByParent := manyToManyTargetKeysByParent(links, embed.relationship)
	grouped := map[string][]rows.Row{}
	for _, parent := range parentRows {
		parentKey := rowKey(parent, embed.relationship.OriginColumns)
		aggregates, err := s.readByKeysForAggregate(
			ctx,
			role,
			embed,
			embed.relationship.TargetColumns,
			keysByParent[parentKey],
		)
		if err != nil {
			return nil, err
		}
		grouped[parentKey] = aggregates
	}
	return grouped, nil
}

func manyToManyTargetKeysByParent(
	links []rows.Row,
	relationship schemacache.Relationship,
) map[string][][]any {
	keys := map[string][][]any{}
	seen := map[string]map[string]bool{}
	for _, link := range links {
		parentKey := rowKey(link, relationship.JoinOriginColumns)
		targetKey := rowKey(link, relationship.JoinTargetColumns)
		if parentKey == "" || targetKey == "" {
			continue
		}
		if seen[parentKey] == nil {
			seen[parentKey] = map[string]bool{}
		}
		if seen[parentKey][targetKey] {
			continue
		}
		seen[parentKey][targetKey] = true
		keys[parentKey] = append(keys[parentKey], rowValues(link, relationship.JoinTargetColumns))
	}
	return keys
}

func groupManyToMany(
	related []rows.Row,
	parentsByTarget map[string][]string,
	targetColumns []string,
) map[string][]rows.Row {
	grouped := map[string][]rows.Row{}
	for _, child := range related {
		targetKey := rowKey(child, targetColumns)
		for _, parentKey := range parentsByTarget[targetKey] {
			grouped[parentKey] = append(grouped[parentKey], child)
		}
	}
	return grouped
}

func attachGroupedEmbeds(
	parentRows []rows.Row,
	embed plannedEmbed,
	grouped map[string][]rows.Row,
) []rows.Row {
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
	return parentRows
}

func (s *Service) readByKeys(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	keyColumns []string,
	keys [][]any,
) ([]rows.Row, error) {
	return s.readByKeysAndEmbeds(ctx, role, embed, keyColumns, keys, true)
}

// readByKeysForAggregate keeps the target keys out of the selected columns.
// The caller already knows the parent scope, so target keys would split groups.
func (s *Service) readByKeysForAggregate(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	keyColumns []string,
	keys [][]any,
) ([]rows.Row, error) {
	return s.readByKeysAndEmbeds(ctx, role, embed, keyColumns, keys, false)
}

func (s *Service) readByKeysAndEmbeds(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	keyColumns []string,
	keys [][]any,
	injectKeyColumns bool,
) ([]rows.Row, error) {
	keyFilters, keyGroups, ok := keyCondition(keyColumns, keys)
	if !ok {
		return nil, nil
	}
	// Limit and offset apply per parent row after grouping, not to the batch.
	query := readquery.Query{
		Columns:   embed.ask.Columns,
		SelectAll: len(embed.ask.Columns) == 0,
		Filters:   append(keyFilters, embed.ask.Filters...),
		Groups:    append(keyGroups, embed.ask.Groups...),
		Order:     embed.ask.Order,
		Embeds:    embedAsks(embed.children),
	}
	childPlan := embed.children
	query, injected := withJoinColumns(embed.target, query, childPlan)
	presenceColumn := ""
	if injectKeyColumns {
		query, _ = ensureColumns(embed.target, query, keyColumns)
	} else {
		query, presenceColumn = addAggregatePresence(query)
	}
	read, err := s.reader.Read(ctx, role, embed.target, query)
	if err != nil {
		return nil, err
	}
	if presenceColumn != "" {
		read.Rows = rowsWithAggregatePresence(read.Rows, presenceColumn)
		read.Rows = dropInjectedColumns(read.Rows, []string{presenceColumn})
	}
	nested, err := s.nestEmbeds(ctx, role, embed.target, read.Rows, childPlan)
	if err != nil {
		return nil, err
	}
	// Key columns let the caller map each ordinary child row to its parent.
	// An aggregate read already has its parent scope, so do not add those keys.
	if injectKeyColumns {
		return dropInjectedColumns(nested, exceptNames(injected, keyColumns)), nil
	}
	return dropInjectedColumns(nested, injected), nil
}

func addAggregatePresence(query readquery.Query) (readquery.Query, string) {
	name := "_myrest_aggregate_presence"
	for {
		used := false
		for _, column := range query.Columns {
			if column.ResultName() == name {
				used = true
				break
			}
		}
		if !used {
			break
		}
		name += "_"
	}
	query.Columns = append(query.Columns, readquery.Column{Alias: name, Agg: readquery.AggCount})
	return query, name
}

func rowsWithAggregatePresence(read []rows.Row, column string) []rows.Row {
	present := make([]rows.Row, 0, len(read))
	for _, row := range read {
		value := columnValue(row, column)
		var raw string
		switch typed := value.(type) {
		case []byte:
			raw = string(typed)
		case string:
			raw = typed
		default:
			raw = fmt.Sprint(value)
		}
		if raw == "0" {
			continue
		}
		present = append(present, row)
	}
	return present
}

func exceptNames(names, keep []string) []string {
	blocked := map[string]bool{}
	for _, name := range keep {
		blocked[name] = true
	}
	var out []string
	for _, name := range names {
		if !blocked[name] {
			out = append(out, name)
		}
	}
	return out
}

// ensureColumns adds named columns to the select list when the client did not
// ask for every column.
func ensureColumns(table schemacache.Table, query readquery.Query, names []string) (readquery.Query, []string) {
	if len(query.Columns) == 0 && query.SelectAll {
		return query, nil
	}
	have := selectedNames(query.Columns)
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
		keep[column.ResultName()] = true
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
	// Related rows keep the SQL order from readByKeys; grouping preserves it.
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

func appendColumn(row rows.Row, name string, value any) rows.Row {
	columns := append(append([]string{}, row.Columns...), name)
	values := append(append([]any{}, row.Values...), value)
	return rows.Row{Columns: columns, Values: values}
}
