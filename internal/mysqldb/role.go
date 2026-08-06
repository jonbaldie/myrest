package mysqldb

import (
	"fmt"
	"strings"

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
	name, host := string(role), ""
	hasHost := false
	// The host comes after the last @, because a host holds none itself.
	if at := strings.LastIndex(name, "@"); at >= 0 {
		name, host, hasHost = name[:at], name[at+1:], true
	}

	if err := checkQuotable("the database role name", name); err != nil {
		return "", err
	}
	if !hasHost {
		return "'" + name + "'", nil
	}
	if err := checkQuotable("the host of the database role", host); err != nil {
		return "", err
	}
	return "'" + name + "'@'" + host + "'", nil
}

// unquotable are the characters myrest cannot put inside a MySQL quoted name:
// the three quote characters and the escape character.
const unquotable = "'\"`\\"

// checkQuotable says whether myrest can write the part into a quoted name. A
// part that holds a quote, an escape, or a control character is refused, so
// that no db-anon-role value can end the quote and change the statement.
func checkQuotable(part, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", part)
	}
	for _, character := range value {
		if strings.ContainsRune(unquotable, character) || character < ' ' || character == 0x7f {
			return fmt.Errorf("%s %q holds the character %q, which myrest cannot quote", part, value, character)
		}
	}
	return nil
}
