package config

import (
	"fmt"
	"strconv"
	"strings"
)

// knob is one entry on the normative surface: its kebab-case PostgREST name
// and how a raw text value becomes a Settings field.
type knob struct {
	name  string
	apply func(Settings, string) (Settings, error)
}

// variable is the MYREST_* environment variable name of the knob.
func (k knob) variable() string {
	return "MYREST_" + strings.ToUpper(strings.ReplaceAll(k.name, "-", "_"))
}

// knobDefinitions lists every knob on the normative surface, in surface order.
func knobDefinitions() []knob {
	definitions := minimumRunSetKnobs()
	definitions = append(definitions, jwtKnobs()...)
	definitions = append(definitions, behaviourGateKnobs()...)
	return append(definitions, openAPIKnobs()...)
}

// definitionsByName indexes the normative surface by kebab-case knob name.
func definitionsByName() map[string]knob {
	definitions := make(map[string]knob)
	for _, definition := range knobDefinitions() {
		definitions[definition.name] = definition
	}
	return definitions
}

// minimumRunSetKnobs are the knobs the serve gate asks for.
func minimumRunSetKnobs() []knob {
	return []knob{
		{name: "db-uri", apply: setter(parseText, func(s Settings, value string) Settings {
			s.DB.URI = value
			return s
		})},
		{name: "db-schemas", apply: setter(parseList, func(s Settings, value []string) Settings {
			s.DB.Schemas = value
			return s
		})},
		{name: "jwt-secret", apply: setter(parseText, func(s Settings, value string) Settings {
			s.JWT.Secret = value
			return s
		})},
		{name: "db-anon-role", apply: setter(parseText, func(s Settings, value string) Settings {
			s.DB.AnonRole = value
			return s
		})},
	}
}

// jwtKnobs carry the JWT detail of the parity target.
func jwtKnobs() []knob {
	return []knob{
		{name: "jwt-secret-is-base64", apply: setter(parseBoolean, func(s Settings, value bool) Settings {
			s.JWT.SecretIsBase64 = value
			return s
		})},
		{name: "jwt-aud", apply: setter(parseText, func(s Settings, value string) Settings {
			s.JWT.Aud = value
			return s
		})},
		{name: "jwt-role-claim-key", apply: setter(parseText, func(s Settings, value string) Settings {
			s.JWT.RoleClaimKey = value
			return s
		})},
		{name: "jwt-cache-max-entries", apply: setter(parseCount, func(s Settings, value int) Settings {
			s.JWT.CacheMaxEntries = value
			return s
		})},
	}
}

// behaviourGateKnobs are the client-visible gates on the normative map.
func behaviourGateKnobs() []knob {
	return []knob{
		{name: "db-aggregates-enabled", apply: setter(parseBoolean, func(s Settings, value bool) Settings {
			s.DB.AggregatesEnabled = value
			return s
		})},
		{name: "db-max-rows", apply: setter(parseRowLimit, func(s Settings, value RowLimit) Settings {
			s.DB.MaxRows = value
			return s
		})},
		{name: "db-pre-request", apply: setter(parseText, func(s Settings, value string) Settings {
			s.DB.PreRequest = value
			return s
		})},
		{name: "db-tx-end", apply: setter(parseTxEnd, func(s Settings, value TxEnd) Settings {
			s.DB.TxEnd = value
			return s
		})},
		{name: "server-cors-allowed-origins", apply: setter(parseList, func(s Settings, value []string) Settings {
			s.Server.CORSAllowedOrigins = value
			return s
		})},
	}
}

// openAPIKnobs name the OpenAPI surface; later tickets give it behaviour.
func openAPIKnobs() []knob {
	return []knob{
		{name: "openapi-mode", apply: setter(parseOpenAPIMode, func(s Settings, value OpenAPIMode) Settings {
			s.OpenAPI.Mode = value
			return s
		})},
		{name: "openapi-security-active", apply: setter(parseBoolean, func(s Settings, value bool) Settings {
			s.OpenAPI.SecurityActive = value
			return s
		})},
		{name: "openapi-server-proxy-uri", apply: setter(parseText, func(s Settings, value string) Settings {
			s.OpenAPI.ServerProxyURI = value
			return s
		})},
		{name: "db-root-spec", apply: setter(parseText, func(s Settings, value string) Settings {
			s.DB.RootSpec = value
			return s
		})},
	}
}

// setter joins a value parser to a Settings field, and gives back the apply
// function of a knob.
func setter[T any](
	parse func(string) (T, error),
	set func(Settings, T) Settings,
) func(Settings, string) (Settings, error) {
	return func(settings Settings, raw string) (Settings, error) {
		value, err := parse(raw)
		if err != nil {
			return settings, err
		}
		return set(settings, value), nil
	}
}

// parseText keeps a knob value as it is.
func parseText(raw string) (string, error) {
	return raw, nil
}

// parseList reads a comma-separated knob value as a list, without empty items.
func parseList(raw string) ([]string, error) {
	var items []string
	for _, item := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items, nil
}

// parseBoolean reads a knob value as true or false.
func parseBoolean(raw string) (bool, error) {
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s is not true or false", raw)
	}
	return value, nil
}

// parseCount reads a knob value as a count of zero or more.
func parseCount(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s is not a count of zero or more", raw)
	}
	return value, nil
}

// parseRowLimit reads a knob value as a hard row cap.
func parseRowLimit(raw string) (RowLimit, error) {
	rows, err := parseCount(raw)
	if err != nil {
		return RowLimit{}, err
	}
	return RowLimit{Rows: rows, Capped: true}, nil
}

// parseTxEnd reads a knob value as a transaction end value.
func parseTxEnd(raw string) (TxEnd, error) {
	return parseChoice(raw, "transaction end", []TxEnd{
		TxEndCommit,
		TxEndCommitAllowOverride,
		TxEndRollback,
		TxEndRollbackAllowOverride,
	})
}

// parseOpenAPIMode reads a knob value as an OpenAPI mode.
func parseOpenAPIMode(raw string) (OpenAPIMode, error) {
	return parseChoice(raw, "OpenAPI mode", []OpenAPIMode{
		OpenAPIModeFollowPrivileges,
		OpenAPIModeIgnorePrivileges,
		OpenAPIModeDisabled,
	})
}

// parseChoice reads a knob value that must be one of a closed set of values.
func parseChoice[T ~string](raw string, kind string, allowed []T) (T, error) {
	for _, value := range allowed {
		if T(raw) == value {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is not a %s value", raw, kind)
}
