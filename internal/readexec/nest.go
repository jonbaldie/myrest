package readexec

import (
	"context"
	"fmt"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func (e *Executor) nest(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	plan []plannedEmbed,
) ([]rows.Row, error) {
	for _, embed := range plan {
		var err error
		parentRows, err = e.nestOne(ctx, role, parentRows, embed)
		if err != nil {
			return nil, err
		}
	}
	return parentRows, nil
}

func (e *Executor) nestOne(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	if len(parentRows) == 0 {
		return parentRows, nil
	}
	switch embed.relationship.Cardinality {
	case schemacache.ManyToOne:
		return e.nestToOne(ctx, role, parentRows, embed)
	case schemacache.OneToMany:
		return e.nestToMany(ctx, role, parentRows, embed)
	case schemacache.ManyToMany:
		return e.nestManyToMany(ctx, role, parentRows, embed)
	default:
		return nil, fmt.Errorf("unknown embed cardinality %d", embed.relationship.Cardinality)
	}
}

func (e *Executor) nestToOne(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	keys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	related, err := e.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, keys)
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

func (e *Executor) nestToMany(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	keys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	related, err := e.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, keys)
	if err != nil {
		return nil, err
	}
	grouped := groupRows(related, embed.relationship.TargetColumns)
	return attachGroupedEmbeds(parentRows, embed, grouped), nil
}

func (e *Executor) nestManyToMany(
	ctx context.Context,
	role schemacache.Role,
	parentRows []rows.Row,
	embed plannedEmbed,
) ([]rows.Row, error) {
	parentKeys := uniqueKeyTuples(parentRows, embed.relationship.OriginColumns)
	related, parentsByTarget, err := e.loadManyToMany(ctx, role, embed, parentKeys)
	if err != nil {
		return nil, err
	}
	grouped := groupManyToMany(related, parentsByTarget, embed.relationship.TargetColumns)
	return attachGroupedEmbeds(parentRows, embed, grouped), nil
}

func (e *Executor) loadManyToMany(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	parentKeys [][]any,
) ([]rows.Row, map[string][]string, error) {
	joinTable, found := e.cache.Resource(role, embed.relationship.JoinTable)
	if !found {
		return nil, nil, schemacache.RelationshipMissing{
			Origin: embed.relationship.Origin,
			Target: embed.ask.Resource,
		}
	}
	joinFilters, joinGroups, ok := keyCondition(embed.relationship.JoinOriginColumns, parentKeys)
	if !ok {
		return nil, nil, nil
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
	joinRead, err := e.reader.Read(ctx, role, joinTable, joinQuery)
	if err != nil {
		return nil, nil, err
	}
	targetKeys := uniqueKeyTuples(joinRead.Rows, embed.relationship.JoinTargetColumns)
	related, err := e.readByKeys(ctx, role, embed, embed.relationship.TargetColumns, targetKeys)
	if err != nil {
		return nil, nil, err
	}
	parentsByTarget := map[string][]string{}
	for _, link := range joinRead.Rows {
		parentKey := rowKey(link, embed.relationship.JoinOriginColumns)
		targetKey := rowKey(link, embed.relationship.JoinTargetColumns)
		parentsByTarget[targetKey] = append(parentsByTarget[targetKey], parentKey)
	}
	return related, parentsByTarget, nil
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

// readByKeys reads the target rows of one embed that match the key tuples,
// and nests their own embeds.
func (e *Executor) readByKeys(
	ctx context.Context,
	role schemacache.Role,
	embed plannedEmbed,
	keyColumns []string,
	keys [][]any,
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
	query = ensureColumns(embed.target, query, keyColumns)
	read, err := e.reader.Read(ctx, role, embed.target, query)
	if err != nil {
		return nil, err
	}
	nested, err := e.nest(ctx, role, read.Rows, childPlan)
	if err != nil {
		return nil, err
	}
	// Keep key columns for grouping; projectEmbedRow drops them for the client.
	// withJoinColumns may list the same names for a nested embed — do not drop them.
	return dropColumns(nested, exceptNames(injected, keyColumns)), nil
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

// shapeRows filters, orders, and pages set, nests the planned embeds, and
// keeps the selected columns.
func (e *Executor) shapeRows(
	ctx context.Context,
	role schemacache.Role,
	plan Plan,
	set []rows.Row,
	query readquery.Query,
) (readquery.Result, error) {
	shaped, err := readquery.Shape(set, query)
	if err != nil {
		return readquery.Result{}, err
	}
	shaped.Rows, err = e.nest(ctx, role, shaped.Rows, plan.embeds)
	if err != nil {
		return readquery.Result{}, err
	}
	projected, err := readquery.Project(shaped.Rows, query)
	if err != nil {
		return readquery.Result{}, err
	}
	shaped.Rows = projected
	return shaped, nil
}
