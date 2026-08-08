package readquery

// HasAggregates is true when the query or a nested embed asks for an aggregate.
func HasAggregates(query Query) bool {
	for _, column := range query.Columns {
		if column.Agg != "" {
			return true
		}
	}
	for _, embed := range query.Embeds {
		if EmbedHasAggregates(embed) {
			return true
		}
	}
	return false
}

// EmbedHasAggregates is true when the embed or a nested embed asks for an aggregate.
func EmbedHasAggregates(embed Embed) bool {
	for _, column := range embed.Columns {
		if column.Agg != "" {
			return true
		}
	}
	for _, nested := range embed.Embeds {
		if EmbedHasAggregates(nested) {
			return true
		}
	}
	return false
}
