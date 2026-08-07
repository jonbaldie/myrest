package schemacache

import "strings"

// Cardinality is the shape of a relationship for embed responses.
type Cardinality int

const (
	// ManyToOne nests one related object (or null).
	ManyToOne Cardinality = iota
	// OneToMany nests an array of related objects.
	OneToMany
	// ManyToMany nests an array through a declared join table.
	ManyToMany
)

// Relationship is one declared-FK path between two resources.
type Relationship struct {
	Cardinality       Cardinality
	Name              string
	Origin            TableID
	Target            TableID
	OriginColumns     []string
	TargetColumns     []string
	JoinTable         TableID
	JoinOriginColumns []string
	JoinTargetColumns []string
}

// RelationshipAmbiguous means more than one declared path applies.
type RelationshipAmbiguous struct {
	Origin   TableID
	Target   string
	Options  []Relationship
}

func (e RelationshipAmbiguous) Error() string {
	return "Could not embed because more than one relationship was found for '" +
		e.Origin.Name + "' and '" + e.Target + "'"
}

// RelationshipMissing means no declared foreign-key path applies.
type RelationshipMissing struct {
	Origin TableID
	Target string
}

func (e RelationshipMissing) Error() string {
	return "Could not find a relationship between '" + e.Origin.Name + "' and '" + e.Target +
		"' in the schema cache"
}

// ComputedRelationship means the client asked to embed through a routine.
type ComputedRelationship struct {
	Name string
}

func (e ComputedRelationship) Error() string {
	return "Computed relationships are not available with MySQL"
}

// ResolveEmbed finds the declared relationship from origin to the named target.
// hint is a foreign-key name or column name that picks one path when several
// apply.
func (c *Cache) ResolveEmbed(role Role, origin TableID, targetName, hint string) (Relationship, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targetID := TableID{Database: origin.Database, Name: targetName}
	if _, held := c.tables[targetID]; !held {
		if _, isRoutine := c.routinesByID[RoutineID{Database: origin.Database, Name: targetName}]; isRoutine {
			return Relationship{}, ComputedRelationship{Name: targetName}
		}
		return Relationship{}, RelationshipMissing{Origin: origin, Target: targetName}
	}
	if !c.selects[bareName(role)][targetID] {
		return Relationship{}, RelationshipMissing{Origin: origin, Target: targetName}
	}

	candidates := c.relationshipsUnlocked(origin, targetID)
	if hint != "" {
		candidates = filterByHint(candidates, hint)
	}
	switch len(candidates) {
	case 0:
		return Relationship{}, RelationshipMissing{Origin: origin, Target: targetName}
	case 1:
		return candidates[0], nil
	default:
		return Relationship{}, RelationshipAmbiguous{
			Origin:  origin,
			Target:  targetName,
			Options: candidates,
		}
	}
}

func (c *Cache) relationshipsUnlocked(origin, target TableID) []Relationship {
	var found []Relationship
	for _, fk := range c.foreignKeys {
		if fk.Table == origin && fk.ReferencedTable == target {
			found = append(found, relationshipFromFK(fk, originHoldsFK(origin, target, fk)))
		}
		if fk.Table == target && fk.ReferencedTable == origin {
			found = append(found, relationshipFromFK(fk, originHoldsFK(origin, target, fk)))
		}
	}
	found = append(found, c.manyToManyUnlocked(origin, target)...)
	return found
}

func originHoldsFK(origin, _ TableID, fk ForeignKeyFact) Cardinality {
	if fk.Table == origin {
		return ManyToOne
	}
	return OneToMany
}

func relationshipFromFK(fk ForeignKeyFact, cardinality Cardinality) Relationship {
	rel := Relationship{
		Cardinality: cardinality,
		Name:        fk.Name,
	}
	switch cardinality {
	case ManyToOne:
		rel.Origin = fk.Table
		rel.Target = fk.ReferencedTable
		rel.OriginColumns = append([]string(nil), fk.Columns...)
		rel.TargetColumns = append([]string(nil), fk.ReferencedColumns...)
	default: // OneToMany: origin is the referenced table
		rel.Origin = fk.ReferencedTable
		rel.Target = fk.Table
		rel.OriginColumns = append([]string(nil), fk.ReferencedColumns...)
		rel.TargetColumns = append([]string(nil), fk.Columns...)
	}
	return rel
}

func (c *Cache) manyToManyUnlocked(origin, target TableID) []Relationship {
	var found []Relationship
	for joinID := range c.tables {
		if joinID == origin || joinID == target {
			continue
		}
		toOrigin, toTarget, ok := c.joinTableFKs(joinID, origin, target)
		if !ok {
			continue
		}
		if !c.pkCoversFKs(joinID, toOrigin, toTarget) {
			continue
		}
		found = append(found, Relationship{
			Cardinality:       ManyToMany,
			Name:              joinID.Name,
			Origin:            origin,
			Target:            target,
			OriginColumns:     primaryKeyColumns(c.keys[origin]),
			TargetColumns:     primaryKeyColumns(c.keys[target]),
			JoinTable:         joinID,
			JoinOriginColumns: append([]string(nil), toOrigin.Columns...),
			JoinTargetColumns: append([]string(nil), toTarget.Columns...),
		})
	}
	return found
}

func (c *Cache) joinTableFKs(join, origin, target TableID) (toOrigin, toTarget ForeignKeyFact, ok bool) {
	var originFK, targetFK ForeignKeyFact
	var haveOrigin, haveTarget bool
	for _, fk := range c.foreignKeys {
		if fk.Table != join {
			continue
		}
		if fk.ReferencedTable == origin {
			originFK = fk
			haveOrigin = true
		}
		if fk.ReferencedTable == target {
			targetFK = fk
			haveTarget = true
		}
	}
	return originFK, targetFK, haveOrigin && haveTarget
}

func (c *Cache) pkCoversFKs(join TableID, a, b ForeignKeyFact) bool {
	pk := primaryKeyColumns(c.keys[join])
	if len(pk) == 0 {
		return false
	}
	return columnsSubset(a.Columns, pk) && columnsSubset(b.Columns, pk)
}

func primaryKeyColumns(keys []KeyFact) []string {
	for _, key := range keys {
		if strings.EqualFold(key.Kind, "PRIMARY") {
			return append([]string(nil), key.Columns...)
		}
	}
	return nil
}

func columnsSubset(have, all []string) bool {
	set := make(map[string]bool, len(all))
	for _, name := range all {
		set[name] = true
	}
	for _, name := range have {
		if !set[name] {
			return false
		}
	}
	return len(have) > 0
}

func filterByHint(candidates []Relationship, hint string) []Relationship {
	var matched []Relationship
	for _, rel := range candidates {
		if rel.Name == hint || columnsContain(rel.OriginColumns, hint) || columnsContain(rel.TargetColumns, hint) ||
			columnsContain(rel.JoinOriginColumns, hint) || columnsContain(rel.JoinTargetColumns, hint) {
			matched = append(matched, rel)
		}
	}
	return matched
}

func columnsContain(columns []string, name string) bool {
	for _, column := range columns {
		if column == name {
			return true
		}
	}
	return false
}
