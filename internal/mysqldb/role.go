package mysqldb

import (
	"fmt"
	"regexp"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// activateAllRoles activates every database role the authenticator holds.
// myrest uses it only to read catalog data: MySQL hides catalog rows from a
// login account that holds no privilege on them.
const activateAllRoles = "SET ROLE ALL"

// clearRoles takes every privilege off the connection.
const clearRoles = "SET ROLE NONE"

// roleSwitchStatement is the SET ROLE statement that activates the role.
// MySQL has no placeholder for SET ROLE, so a name that is not one simple
// MySQL name is refused instead of quoted into the statement.
func roleSwitchStatement(role schemacache.Role) (string, error) {
	if !isSimpleName(role) {
		return "", fmt.Errorf("the database role %q is not a simple name", role)
	}
	return "SET ROLE '" + string(role) + "'", nil
}

// simpleName holds the role names myrest can write into a SET ROLE statement:
// one or more letters, digits, or underscores, and nothing else.
var simpleName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// isSimpleName reports whether myrest can write the role name into a
// statement without quoting it.
func isSimpleName(role schemacache.Role) bool {
	return simpleName.MatchString(string(role))
}
