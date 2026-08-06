package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestLoadGivesTheParityTargetDefaultsWhenNoKnobIsSet(t *testing.T) {
	t.Parallel()

	settings, err := config.Load("", config.Environment{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if settings.DB.AggregatesEnabled {
		t.Error("DBAggregatesEnabled = true, want off by default")
	}
	if settings.JWT.RoleClaimKey != ".role" {
		t.Errorf("JWTRoleClaimKey = %q, want .role", settings.JWT.RoleClaimKey)
	}
	if settings.JWT.CacheMaxEntries != 1000 {
		t.Errorf("JWTCacheMaxEntries = %d, want 1000", settings.JWT.CacheMaxEntries)
	}
	if settings.DB.TxEnd != config.TxEndCommit {
		t.Errorf("DBTxEnd = %q, want %q", settings.DB.TxEnd, config.TxEndCommit)
	}
	if settings.OpenAPI.Mode != config.OpenAPIModeFollowPrivileges {
		t.Errorf("OpenAPIMode = %q, want %q", settings.OpenAPI.Mode, config.OpenAPIModeFollowPrivileges)
	}
	if settings.DB.MaxRows.Capped {
		t.Errorf("DBMaxRows = %+v, want no cap by default", settings.DB.MaxRows)
	}
}

func TestLoadReadsEveryKeptKnobFromTheConfigFile(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(fullSurfaceFile, config.Environment{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !reflect.DeepEqual(settings, fullSurfaceSettings()) {
		t.Fatalf("settings = %+v, want %+v", settings, fullSurfaceSettings())
	}
}

func TestLoadReadsEveryKeptKnobFromTheEnvironment(t *testing.T) {
	t.Parallel()

	settings, err := config.Load("", fullSurfaceEnvironment())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !reflect.DeepEqual(settings, fullSurfaceSettings()) {
		t.Fatalf("settings = %+v, want %+v", settings, fullSurfaceSettings())
	}
}

func TestLoadRefusesAValueThatDoesNotMatchTheKnobType(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		file string
		want string
	}{
		{name: "boolean", file: "db-aggregates-enabled = maybe", want: "db-aggregates-enabled"},
		{name: "integer", file: `jwt-cache-max-entries = "many"`, want: "jwt-cache-max-entries"},
		{name: "negative integer", file: "db-max-rows = -1", want: "db-max-rows"},
		{name: "transaction end", file: `db-tx-end = "sideways"`, want: "db-tx-end"},
		{name: "openapi mode", file: `openapi-mode = "hidden"`, want: "openapi-mode"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(testCase.file, config.Environment{})
			if err == nil {
				t.Fatalf("Load accepted %q", testCase.file)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error %q does not name %q", err, testCase.want)
			}
		})
	}
}

func TestLoadRefusesAnEnvironmentValueThatDoesNotMatchTheKnobType(t *testing.T) {
	t.Parallel()

	_, err := config.Load("", config.Environment{"MYREST_DB_MAX_ROWS": "plenty"})
	if err == nil {
		t.Fatal("Load accepted MYREST_DB_MAX_ROWS=plenty")
	}
	if !strings.Contains(err.Error(), "MYREST_DB_MAX_ROWS") {
		t.Fatalf("error %q does not name MYREST_DB_MAX_ROWS", err)
	}
}

const fullSurfaceFile = `# myrest.conf
db-uri = "mysql://authenticator:secret@127.0.0.1:3306/"
db-schemas = "shop, warehouse"
jwt-secret = "reallyreallyreallyreallyverysafe"
db-anon-role = "myrest_anon"
jwt-secret-is-base64 = true
jwt-aud = "myrest-clients"
jwt-role-claim-key = ".myrest.role"
jwt-cache-max-entries = 0
db-aggregates-enabled = true
db-max-rows = 500
db-pre-request = "myrest_hooks.before_request"
db-tx-end = "commit-allow-override"
server-cors-allowed-origins = "https://shop.example, https://admin.example"
openapi-mode = "ignore-privileges"
openapi-security-active = true
openapi-server-proxy-uri = "https://api.example"
db-root-spec = "myrest_hooks.root_spec"
`

func fullSurfaceEnvironment() config.Environment {
	return config.Environment{
		"MYREST_DB_URI":                      "mysql://authenticator:secret@127.0.0.1:3306/",
		"MYREST_DB_SCHEMAS":                  "shop, warehouse",
		"MYREST_JWT_SECRET":                  "reallyreallyreallyreallyverysafe",
		"MYREST_DB_ANON_ROLE":                "myrest_anon",
		"MYREST_JWT_SECRET_IS_BASE64":        "true",
		"MYREST_JWT_AUD":                     "myrest-clients",
		"MYREST_JWT_ROLE_CLAIM_KEY":          ".myrest.role",
		"MYREST_JWT_CACHE_MAX_ENTRIES":       "0",
		"MYREST_DB_AGGREGATES_ENABLED":       "true",
		"MYREST_DB_MAX_ROWS":                 "500",
		"MYREST_DB_PRE_REQUEST":              "myrest_hooks.before_request",
		"MYREST_DB_TX_END":                   "commit-allow-override",
		"MYREST_SERVER_CORS_ALLOWED_ORIGINS": "https://shop.example, https://admin.example",
		"MYREST_OPENAPI_MODE":                "ignore-privileges",
		"MYREST_OPENAPI_SECURITY_ACTIVE":     "true",
		"MYREST_OPENAPI_SERVER_PROXY_URI":    "https://api.example",
		"MYREST_DB_ROOT_SPEC":                "myrest_hooks.root_spec",
	}
}

func fullSurfaceSettings() config.Settings {
	return config.Settings{
		DB: config.DatabaseSettings{
			URI:               "mysql://authenticator:secret@127.0.0.1:3306/",
			Schemas:           []string{"shop", "warehouse"},
			AnonRole:          "myrest_anon",
			AggregatesEnabled: true,
			MaxRows:           config.RowLimit{Rows: 500, Capped: true},
			PreRequest:        "myrest_hooks.before_request",
			TxEnd:             config.TxEndCommitAllowOverride,
			RootSpec:          "myrest_hooks.root_spec",
		},
		JWT: config.JWTSettings{
			Secret:          "reallyreallyreallyreallyverysafe",
			SecretIsBase64:  true,
			Aud:             "myrest-clients",
			RoleClaimKey:    ".myrest.role",
			CacheMaxEntries: 0,
		},
		OpenAPI: config.OpenAPISettings{
			Mode:           config.OpenAPIModeIgnorePrivileges,
			SecurityActive: true,
			ServerProxyURI: "https://api.example",
		},
		Server: config.ServerSettings{
			CORSAllowedOrigins: []string{"https://shop.example", "https://admin.example"},
		},
	}
}
