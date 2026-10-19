// Package mysqlname checks names shared by configuration and the MySQL adapter.
// SQL quoting remains the responsibility of the adapter.
package mysqlname

import (
	"errors"
	"fmt"
	"strings"
)

// SplitPreRequest splits the database.routine form of db-pre-request.
// Back quotes are valid here: the adapter escapes them as identifiers.
func SplitPreRequest(name string) (database, routine string, err error) {
	parts := strings.Split(name, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("db-pre-request must be database.routine")
	}
	return parts[0], parts[1], nil
}

// Role holds the checked parts of a MySQL account name.
type Role struct {
	Name    string
	Host    string
	HasHost bool
}

// ParseRole checks a name or name@host using MySQL account quoting rules.
// The host follows the last @; a missing host uses MySQL's default host.
func ParseRole(value string) (Role, error) {
	role := Role{Name: value}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		role.Name, role.Host, role.HasHost = value[:at], value[at+1:], true
	}
	if err := checkQuotable("the database role name", role.Name); err != nil {
		return Role{}, err
	}
	if role.HasHost {
		if err := checkQuotable("the host of the database role", role.Host); err != nil {
			return Role{}, err
		}
	}
	return role, nil
}

// checkQuotable refuses quotes, escapes, and control characters without
// including configured values in diagnostics.
func checkQuotable(part, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", part)
	}
	for _, character := range value {
		if strings.ContainsRune("'\"`\\", character) || character < ' ' || character == 0x7f {
			return fmt.Errorf("%s holds a character which myrest cannot quote", part)
		}
	}
	return nil
}
