// Package mysqldb speaks to MySQL for myrest: it opens pooled connections as
// the authenticator, activates a database role for one request, reads catalog
// data for the schema cache, and reads the rows of a resource.
package mysqldb

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// defaultPort is the MySQL port a db-uri without a port means.
const defaultPort = "3306"

// dataSourceName turns a db-uri (the MySQL authenticator URI) into the data
// source name the MySQL driver reads.
func dataSourceName(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("db-uri: %w", err)
	}
	if parsed.Scheme != "mysql" {
		return "", errors.New("db-uri: the scheme must be mysql")
	}
	if parsed.User.Username() == "" {
		return "", errors.New("db-uri: the authenticator user is missing")
	}
	if parsed.Host == "" {
		return "", errors.New("db-uri: the host is missing")
	}

	password, _ := parsed.User.Password()
	query := parsed.Query()
	query.Set("parseTime", "true")

	return fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?%s",
		parsed.User.Username(),
		password,
		hostWithPort(parsed.Host),
		strings.TrimPrefix(parsed.Path, "/"),
		query.Encode(),
	), nil
}

// hostWithPort adds the MySQL port to a host that carries none.
func hostWithPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, defaultPort)
}
