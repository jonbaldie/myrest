package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jonbaldie/myrest/internal/readexec"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

const (
	// codeNoRelationship is the parity-target code for a missing embed path.
	codeNoRelationship = "PGRST200"
	// codeAmbiguousRelationship is the parity-target code for more than one path.
	codeAmbiguousRelationship = "PGRST201"
)

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
	if errors.As(err, &readexec.SpreadAggregateRefused{}) {
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
	if errors.As(err, &readexec.SpreadNotSupported{}) {
		writeUnsupportedFeature(writer, msgSpreadNotSupported)
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
