package mysqldb

import (
	"github.com/jonbaldie/myrest/internal/mysqlname"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// activateAllRoles activates every database role the authenticator holds.
// myrest uses it only to read catalog data: MySQL hides catalog rows from a
// login account that holds no privilege on them.
const activateAllRoles = "SET ROLE ALL"

// clearRoles takes every privilege off the connection.
const clearRoles = "SET ROLE NONE"

// roleSwitchStatement is the SET ROLE statement that activates the role.
// MySQL has no placeholder for SET ROLE, so myrest writes the name into the
// statement itself and must be sure of every character of it.
func roleSwitchStatement(role schemacache.Role) (string, error) {
	quoted, err := quoteRole(role)
	if err != nil {
		return "", err
	}
	return "SET ROLE " + quoted, nil
}

// quoteRole gives the MySQL role identifier of a db-anon-role value. MySQL
// names a role `name@host`, and a name alone means the host `%`, so both
// `web-anon` and `web-anon@localhost` are role names an operator can write.
func quoteRole(role schemacache.Role) (string, error) {
	parsed, err := mysqlname.ParseRole(string(role))
	if err != nil {
		return "", err
	}
	if !parsed.HasHost {
		return "'" + parsed.Name + "'", nil
	}
	return "'" + parsed.Name + "'@'" + parsed.Host + "'", nil
}
