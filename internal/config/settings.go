// Package config resolves the myrest configuration surface: the normative
// knobs, their values from a config file and from MYREST_* environment
// variables, and the gate that keeps a half-configured process off the wire.
package config

// TxEnd tells myrest how to end a database transaction (knob db-tx-end).
type TxEnd string

// The transaction end values of the parity target.
const (
	TxEndCommit                TxEnd = "commit"
	TxEndCommitAllowOverride   TxEnd = "commit-allow-override"
	TxEndRollback              TxEnd = "rollback"
	TxEndRollbackAllowOverride TxEnd = "rollback-allow-override"
)

// OpenAPIMode tells myrest how much of the OpenAPI output to show
// (knob openapi-mode).
type OpenAPIMode string

// The OpenAPI mode values of the parity target.
const (
	OpenAPIModeFollowPrivileges OpenAPIMode = "follow-privileges"
	OpenAPIModeIgnorePrivileges OpenAPIMode = "ignore-privileges"
	OpenAPIModeDisabled         OpenAPIMode = "disabled"
)

// RowLimit is the hard row cap of a read (knob db-max-rows). The zero value
// puts no cap on a read.
type RowLimit struct {
	Rows   int
	Capped bool
}

// DatabaseSettings holds the `db-*` knobs.
type DatabaseSettings struct {
	// URI is the MySQL authenticator URI (knob db-uri).
	URI string
	// Schemas lists the MySQL databases to expose (knob db-schemas).
	Schemas []string
	// AnonRole is the anonymous database role (knob db-anon-role).
	AnonRole string
	// AggregatesEnabled opens the aggregate gate (knob db-aggregates-enabled).
	AggregatesEnabled bool
	// MaxRows caps the rows a read returns (knob db-max-rows).
	MaxRows RowLimit
	// PreRequest names the routine to call after the role switch
	// (knob db-pre-request).
	PreRequest string
	// TxEnd ends the request transaction (knob db-tx-end).
	TxEnd TxEnd
	// RootSpec names the routine that replaces the OpenAPI output
	// (knob db-root-spec).
	RootSpec string
}

// JWTSettings holds the `jwt-*` knobs.
type JWTSettings struct {
	// Secret verifies the Bearer JWT (knob jwt-secret).
	Secret string
	// SecretIsBase64 reads the secret as base64 (knob jwt-secret-is-base64).
	SecretIsBase64 bool
	// Aud is the audience the JWT aud claim must hold (knob jwt-aud).
	Aud string
	// RoleClaimKey is the key path of the role claim (knob jwt-role-claim-key).
	RoleClaimKey string
	// CacheMaxEntries caps the JWT cache; 0 turns the cache off
	// (knob jwt-cache-max-entries).
	CacheMaxEntries int
}

// OpenAPISettings holds the `openapi-*` knobs.
type OpenAPISettings struct {
	// Mode selects how much OpenAPI output to show (knob openapi-mode).
	Mode OpenAPIMode
	// SecurityActive adds security options to the OpenAPI output
	// (knob openapi-security-active).
	SecurityActive bool
	// ServerProxyURI replaces the base URL of the OpenAPI output
	// (knob openapi-server-proxy-uri).
	ServerProxyURI string
}

// ServerSettings holds the `server-*` knobs.
type ServerSettings struct {
	// CORSAllowedOrigins lists the allowed CORS origins; an empty list allows
	// every origin (knob server-cors-allowed-origins).
	CORSAllowedOrigins []string
}

// Settings holds the resolved value of every knob on the normative surface.
type Settings struct {
	DB      DatabaseSettings
	JWT     JWTSettings
	OpenAPI OpenAPISettings
	Server  ServerSettings
}

// DefaultDatabase is the MySQL database a request reads when it names none.
// It is the first database of db-schemas, as the parity target reads the
// first schema of its schema list. Settings that did not pass the serve gate
// name no database at all.
func (s Settings) DefaultDatabase() string {
	if len(s.DB.Schemas) == 0 {
		return ""
	}
	return s.DB.Schemas[0]
}

// Defaults holds the value of every knob that nobody has set, as the parity
// target documents them.
func Defaults() Settings {
	return Settings{
		DB:      DatabaseSettings{TxEnd: TxEndCommit},
		JWT:     JWTSettings{RoleClaimKey: ".role", CacheMaxEntries: 1000},
		OpenAPI: OpenAPISettings{Mode: OpenAPIModeFollowPrivileges},
	}
}
