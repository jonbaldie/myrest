package config_test

import (
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestJWTOptionsRetainResolvedKnobs(t *testing.T) {
	t.Parallel()
	settings := config.JWTSettings{Secret: "secret", SecretIsBase64: true, Aud: "audience", RoleClaimKey: ".claims.role", CacheMaxEntries: 17}
	options := settings.Options()
	if options.Secret != settings.Secret || options.SecretIsBase64 != settings.SecretIsBase64 || options.Aud != settings.Aud || options.RoleClaimKey != settings.RoleClaimKey || options.CacheMaxEntries != settings.CacheMaxEntries {
		t.Fatal("JWT options do not retain all resolved knobs")
	}
}

func TestForProcessChecksLocalShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		knob, value, mode string
		valid             bool
	}{
		{"DB_PRE_REQUEST", "", "", true},
		{"DB_PRE_REQUEST", "db.routine", "", true},
		{"DB_PRE_REQUEST", " db.od`d ", "", true},
		{"DB_PRE_REQUEST", "nodot", "", false},
		{"DB_PRE_REQUEST", "db.routine.extra", "", false},
		{"DB_PRE_REQUEST", ".routine", "", false},
		{"DB_PRE_REQUEST", "db.", "", false},
		{"DB_ANON_ROLE", "web anon", "", true},
		{"DB_ANON_ROLE", "web-anon@%", "", true},
		{"DB_ANON_ROLE", "name@part@localhost", "", true},
		{"DB_ANON_ROLE", "bad'role", "", false},
		{"DB_ANON_ROLE", "bad\"role", "", false},
		{"DB_ANON_ROLE", "bad`role", "", false},
		{"DB_ANON_ROLE", `bad\role`, "", false},
		{"DB_ANON_ROLE", "bad\nrole", "", false},
		{"DB_ANON_ROLE", "bad\x7frole", "", false},
		{"DB_ANON_ROLE", "@localhost", "", false},
		{"DB_ANON_ROLE", "role@", "", false},
		{"DB_ANON_ROLE", "role@'", "", false},
		{"JWT_SECRET", "%%%not-base64", "true", false},
		{"JWT_SECRET", "%%%raw-secret", "false", true},
		{"JWT_SECRET", "c2VjcmV0", "true", true},
		{"JWT_SECRET", "", "true", true},
	}
	for _, tc := range cases {
		t.Run(tc.knob+"/"+tc.value, func(t *testing.T) {
			env := config.Environment{"MYREST_DB_URI": "mysql://authenticator:private-password@127.0.0.1:1/", "MYREST_DB_SCHEMAS": "shop", "MYREST_DB_ANON_ROLE": "anon"}
			env["MYREST_"+tc.knob] = tc.value
			if tc.mode != "" {
				env["MYREST_JWT_SECRET_IS_BASE64"] = tc.mode
			}
			_, err := config.ForProcess(nil, env)
			if tc.valid {
				if err != nil {
					t.Fatalf("valid shape: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted invalid shape")
			}
			knob := strings.ToLower(strings.ReplaceAll(tc.knob, "_", "-"))
			if !strings.Contains(err.Error(), knob) {
				t.Fatalf("error %q does not name %s", err, knob)
			}
			if strings.Contains(err.Error(), "private-password") || (tc.knob == "JWT_SECRET" && strings.Contains(err.Error(), tc.value)) {
				t.Fatalf("error discloses a configured value: %v", err)
			}
		})
	}
}

func TestForProcessReportsAllLocalFaultsInStableOrder(t *testing.T) {
	t.Parallel()
	_, err := config.ForProcess(nil, config.Environment{
		"MYREST_DB_PRE_REQUEST": "nodot", "MYREST_DB_ANON_ROLE": "bad'role",
		"MYREST_JWT_SECRET": "%%%not-base64", "MYREST_JWT_SECRET_IS_BASE64": "true",
	})
	if err == nil {
		t.Fatal("accepted invalid settings")
	}
	message := err.Error()
	previous := -1
	for _, knob := range []string{"db-uri", "db-schemas", "db-pre-request", "db-anon-role", "jwt-secret"} {
		at := strings.Index(message, knob)
		if at <= previous {
			t.Fatalf("missing or unordered %s: %s", knob, message)
		}
		previous = at
	}
}

func TestForProcessChecksShapesAfterEnvironmentPrecedence(t *testing.T) {
	t.Parallel()
	path := writeConfigFile(t, `db-uri = "mysql://authenticator@127.0.0.1:1/"
db-schemas = "shop"
db-anon-role = "bad'role"
db-pre-request = "nodot"
jwt-secret = "%%%not-base64"
jwt-secret-is-base64 = true
`)
	env := config.Environment{"MYREST_DB_ANON_ROLE": "anon", "MYREST_DB_PRE_REQUEST": "db.hook", "MYREST_JWT_SECRET": "c2VjcmV0"}
	if _, err := config.ForProcess([]string{path}, env); err != nil {
		t.Fatalf("valid overrides: %v", err)
	}
	env["MYREST_DB_PRE_REQUEST"] = ""
	if _, err := config.ForProcess([]string{path}, env); err == nil || !strings.Contains(err.Error(), "db-pre-request") {
		t.Fatalf("empty environment must retain invalid file value: %v", err)
	}
}
