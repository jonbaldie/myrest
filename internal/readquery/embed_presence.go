package readquery

// refuseEmbedPresence keeps null checks on selected embeds out of the column
// filter path. Parent filtering by embed presence is not supported.
func refuseEmbedPresence(query Query) error {
	group := Group{Filters: query.Filters, Groups: query.Groups}
	if err := refuseEmbedPresenceGroup(query.Embeds, group); err != nil {
		return err
	}
	for _, embed := range query.Embeds {
		nested := Query{Embeds: embed.Embeds, Filters: embed.Filters, Groups: embed.Groups}
		if err := refuseEmbedPresence(nested); err != nil {
			return err
		}
	}
	return nil
}

func isNullCheck(filter Filter) bool {
	return filter.Op == OpIs && filter.Value == "null" && filter.Path == nil && !filter.ValueQuoted
}

func refuseEmbedPresenceGroup(embeds []Embed, group Group) error {
	for _, filter := range group.Filters {
		if !isNullCheck(filter) {
			continue
		}
		for _, embed := range embeds {
			if filter.Column == embed.Key() {
				return ParseFailure{Gap: true, Message: "PostgREST embed presence filters are not supported"}
			}
		}
	}
	for _, nested := range group.Groups {
		if err := refuseEmbedPresenceGroup(embeds, nested); err != nil {
			return err
		}
	}
	return nil
}
