package httpapi

import (
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestAmbiguousMessages(t *testing.T) {
	t.Parallel()

	failure := schemacache.RelationshipAmbiguous{
		Origin: schemacache.TableID{Name: "deliveries"},
		Target: "addresses",
		Options: []schemacache.Relationship{
			{Name: "deliveries_from", Cardinality: schemacache.ManyToOne},
			{Name: "deliveries_to", Cardinality: schemacache.ManyToOne},
		},
	}
	hint := ambiguousHint(failure)
	if hint == "" || cardinalityName(schemacache.ManyToMany) != "many-to-many" {
		t.Fatalf("hint = %q", hint)
	}
	details := ambiguousDetails(failure)
	if len(details) != 2 {
		t.Fatalf("details = %#v", details)
	}
	// The hint of a self one-to-many option names its key column, so the
	// suggestion resolves instead of repeating the shared constraint name.
	employees := schemacache.TableID{Name: "employees"}
	self := schemacache.RelationshipAmbiguous{
		Origin: employees,
		Target: "employees",
		Options: []schemacache.Relationship{
			{Name: "employees_manager", Cardinality: schemacache.ManyToOne, Origin: employees, Target: employees},
			{Name: "employees_manager", Cardinality: schemacache.OneToMany, Origin: employees, Target: employees, TargetColumns: []string{"manager_id"}},
		},
	}
	selfHint := ambiguousHint(self)
	if !strings.Contains(selfHint, "'employees!employees_manager'") ||
		!strings.Contains(selfHint, "'employees!manager_id'") {
		t.Fatalf("self hint = %q", selfHint)
	}
}

func TestCardinalityNameUnknown(t *testing.T) {
	t.Parallel()

	if cardinalityName(schemacache.Cardinality(99)) != "unknown" {
		t.Fatal("unknown cardinality")
	}
}
