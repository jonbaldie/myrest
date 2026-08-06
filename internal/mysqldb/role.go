package mysqldb

import (
	"fmt"
	"regexp"
)

// allRoles activates every database role the authenticator holds. myrest uses
// it only to read catalog data: MySQL hides catalog rows from a login account
// that holds no privilege on them.
const allRoles = "ALL"

// simpleName is the shape of a database role name myrest can activate. MySQL
// has no placeholder for SET ROLE, so a name outside this shape is refused
// instead of quoted into the statement.
var simpleName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// roleSwitchStatement is the SET ROLE statement that activates the role.
func roleSwitchStatement(role string) (string, error) {
	if role == allRoles {
		return "SET ROLE ALL", nil
	}
	if !simpleName.MatchString(role) {
		return "", fmt.Errorf("the database role %q is not a simple name", role)
	}
	return "SET ROLE '" + role + "'", nil
}
