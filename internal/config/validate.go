package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/jwt"
	"github.com/jonbaldie/myrest/internal/mysqlname"
)

// Validate checks the minimum run set and local knob shapes together, in a
// stable order. It opens no external resources and does not report values.
func (s Settings) Validate() error {
	faults := []error{s.ServeGate()}
	if name := strings.TrimSpace(s.DB.PreRequest); name != "" {
		_, _, err := mysqlname.SplitPreRequest(name)
		faults = append(faults, err)
	}
	if s.DB.AnonRole != "" {
		_, err := mysqlname.ParseRole(s.DB.AnonRole)
		if err != nil {
			faults = append(faults, fmt.Errorf("db-anon-role: %w", err))
		}
	}
	if s.JWT.Secret != "" {
		_, err := jwt.New(s.JWT.Options())
		if err != nil {
			faults = append(faults, err)
		}
	}
	return errors.Join(faults...)
}

// Options maps resolved JWT knobs to the verifier's options. Validation and
// verifier construction use the same secret decoding rule in jwt.New.
func (s JWTSettings) Options() jwt.Options {
	return jwt.Options{
		Secret: s.Secret, SecretIsBase64: s.SecretIsBase64, Aud: s.Aud,
		RoleClaimKey: s.RoleClaimKey, CacheMaxEntries: s.CacheMaxEntries,
	}
}
