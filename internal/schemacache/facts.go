package schemacache

import "strings"

// CommentFact is a comment MySQL holds on a table or a view.
type CommentFact struct {
	Relation TableID
	Comment  string
}

// KeyFact is a primary or unique key of a table or view, with columns in key
// order.
type KeyFact struct {
	Table   TableID
	Name    string
	Kind    string
	Columns []string
}

// ForeignKeyFact is a foreign key the catalog declares, with columns in key
// order and the update and delete rules MySQL reports.
type ForeignKeyFact struct {
	Name              string
	Table             TableID
	Columns           []string
	ReferencedTable   TableID
	ReferencedColumns []string
	UpdateRule        string
	DeleteRule        string
}

// RoutineID names one routine of one MySQL database.
type RoutineID struct {
	Database string
	Name     string
}

// ParameterFact is one parameter of a routine, in ordinal order. Ordinal 0 is
// the return type of a function.
type ParameterFact struct {
	Name     string
	Mode     string
	Ordinal  int
	DataType string
}

// RoutineFact is one function or procedure of a configured MySQL database.
type RoutineFact struct {
	ID            RoutineID
	Kind          string
	Comment       string
	ReturnType    string
	SQLDataAccess string
	Parameters    []ParameterFact
}

// ReadSafe reports whether myrest may call this routine with GET /rpc.
// The decision uses only MySQL ROUTINES.SQL_DATA_ACCESS:
// NO SQL, CONTAINS SQL, and READS SQL DATA are read-safe; MODIFIES SQL DATA
// and any other or empty value are not.
func (r RoutineFact) ReadSafe() bool {
	switch strings.ToUpper(strings.TrimSpace(r.SQLDataAccess)) {
	case "NO SQL", "CONTAINS SQL", "READS SQL DATA":
		return true
	default:
		return false
	}
}

// TablePrivilegeFact says that a database role holds one table privilege the
// exposure rule needs (SELECT, INSERT, UPDATE, or DELETE).
type TablePrivilegeFact struct {
	Role      Role
	Table     TableID
	Privilege string
}

// RoutinePrivilegeFact says that a database role holds EXECUTE on a routine.
type RoutinePrivilegeFact struct {
	Role      Role
	Routine   RoutineID
	Privilege string
}
