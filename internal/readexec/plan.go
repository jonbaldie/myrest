package readexec

import (
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Plan is an embed tree resolved against the schema cache.
type Plan struct {
	embeds []plannedEmbed
}

// plannedEmbed is one embed resolved against the schema cache.
type plannedEmbed struct {
	ask          readquery.Embed
	relationship schemacache.Relationship
	target       schemacache.Table
	children     []plannedEmbed
}

// SpreadAggregateRefused is the refuse for aggregates inside a to-many or
// many-to-many spread embed.
type SpreadAggregateRefused struct{}

func (SpreadAggregateRefused) Error() string {
	return "aggregates inside a to-many spread embed are refused"
}

// SpreadNotSupported is a spread embed shape myrest does not implement.
type SpreadNotSupported struct{}

func (SpreadNotSupported) Error() string {
	return "spread embeds are not available"
}

// Plan resolves the embeds asked from origin, and every nested embed, against
// the declared relationships of the schema cache. It reads no rows, so a
// caller can refuse a bad select before it writes.
func (e *Executor) Plan(
	role schemacache.Role,
	origin schemacache.TableID,
	asks []readquery.Embed,
) (Plan, error) {
	embeds, err := e.planEmbeds(role, origin, asks)
	if err != nil {
		return Plan{}, err
	}
	return Plan{embeds: embeds}, nil
}

// OriginColumns names the origin columns the top-level embeds join on.
func (p Plan) OriginColumns() []string {
	var names []string
	for _, embed := range p.embeds {
		names = append(names, embed.relationship.OriginColumns...)
	}
	return names
}

func (e *Executor) planEmbeds(
	role schemacache.Role,
	origin schemacache.TableID,
	asks []readquery.Embed,
) ([]plannedEmbed, error) {
	planned := make([]plannedEmbed, 0, len(asks))
	for _, ask := range asks {
		rel, err := e.cache.ResolveEmbed(role, origin, ask.Resource, ask.Hint)
		if err != nil {
			return nil, err
		}
		if err := checkSpread(ask, rel); err != nil {
			return nil, err
		}
		target, ok := e.cache.Resource(role, rel.Target)
		if !ok {
			return nil, schemacache.RelationshipMissing{Origin: origin, Target: ask.Resource}
		}
		children, err := e.planEmbeds(role, target.ID, ask.Embeds)
		if err != nil {
			return nil, err
		}
		planned = append(planned, plannedEmbed{
			ask: ask, relationship: rel, target: target, children: children,
		})
	}
	return planned, nil
}

func checkSpread(ask readquery.Embed, rel schemacache.Relationship) error {
	if !ask.Spread {
		return nil
	}
	if readquery.EmbedHasAggregates(ask) &&
		(rel.Cardinality == schemacache.OneToMany || rel.Cardinality == schemacache.ManyToMany) {
		return SpreadAggregateRefused{}
	}
	return SpreadNotSupported{}
}

func embedAsks(plan []plannedEmbed) []readquery.Embed {
	asks := make([]readquery.Embed, len(plan))
	for i, embed := range plan {
		asks[i] = embed.ask
	}
	return asks
}
