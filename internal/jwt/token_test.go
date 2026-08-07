package jwt_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/jonbaldie/myrest/internal/jwt"
)

const secret = "reallyreallyreallyreallyverysafe"

func sign(t *testing.T, claims gojwt.MapClaims, key []byte) string {
	t.Helper()

	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signed
}

func TestValidTokenGivesTheRoleClaim(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{"role": "myrest_user"}, []byte(secret))

	role, err := verifier.Role(token)
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	if role != "myrest_user" {
		t.Fatalf("role = %q, want myrest_user", role)
	}
}

func TestNestedRoleClaimKey(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{
		Secret:       secret,
		RoleClaimKey: ".myrest.role",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{
		"myrest": map[string]any{"role": "nested_role"},
	}, []byte(secret))

	role, err := verifier.Role(token)
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	if role != "nested_role" {
		t.Fatalf("role = %q, want nested_role", role)
	}
}

func TestBase64Secret(t *testing.T) {
	t.Parallel()

	raw := []byte(secret)
	encoded := base64.StdEncoding.EncodeToString(raw)
	verifier, err := jwt.New(jwt.Options{Secret: encoded, SecretIsBase64: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{"role": "myrest_user"}, raw)

	role, err := verifier.Role(token)
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	if role != "myrest_user" {
		t.Fatalf("role = %q, want myrest_user", role)
	}
}

func TestInvalidSignatureIsInvalidToken(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{"role": "myrest_user"}, []byte("wrong-secret-wrong-secret!!"))

	_, err = verifier.Role(token)
	if !errors.Is(err, jwt.ErrInvalidToken) {
		t.Fatalf("error = %v, want ErrInvalidToken", err)
	}
}

func TestExpiredTokenIsClaimsFailure(t *testing.T) {
	t.Parallel()

	fixed := time.Unix(1_700_000_000, 0)
	verifier, err := jwt.New(jwt.Options{
		Secret: secret,
		Now:    func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{
		"role": "myrest_user",
		"exp":  fixed.Add(-time.Minute).Unix(),
	}, []byte(secret))

	_, err = verifier.Role(token)
	if !errors.Is(err, jwt.ErrClaimsFailed) {
		t.Fatalf("error = %v, want ErrClaimsFailed", err)
	}
}

func TestAudienceMismatchIsClaimsFailure(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret, Aud: "myrest-clients"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{
		"role": "myrest_user",
		"aud":  "other",
	}, []byte(secret))

	_, err = verifier.Role(token)
	if !errors.Is(err, jwt.ErrClaimsFailed) {
		t.Fatalf("error = %v, want ErrClaimsFailed", err)
	}
}

func TestMissingRoleClaimIsNoRole(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{"sub": "alice"}, []byte(secret))

	_, err = verifier.Role(token)
	if !errors.Is(err, jwt.ErrNoRole) {
		t.Fatalf("error = %v, want ErrNoRole", err)
	}
}

func TestCacheBoundsTheNumberOfEntries(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret, CacheMaxEntries: 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i, role := range []string{"a", "b", "c"} {
		token := sign(t, gojwt.MapClaims{"role": role, "n": float64(i)}, []byte(secret))
		if _, err := verifier.Role(token); err != nil {
			t.Fatalf("Role(%s): %v", role, err)
		}
	}
	if got := verifier.CacheLen(); got != 2 {
		t.Fatalf("cache length = %d, want 2", got)
	}
}

func TestZeroCacheMaxTurnsTheCacheOff(t *testing.T) {
	t.Parallel()

	verifier, err := jwt.New(jwt.Options{Secret: secret, CacheMaxEntries: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token := sign(t, gojwt.MapClaims{"role": "myrest_user"}, []byte(secret))
	if _, err := verifier.Role(token); err != nil {
		t.Fatalf("Role: %v", err)
	}
	if got := verifier.CacheLen(); got != 0 {
		t.Fatalf("cache length = %d, want 0", got)
	}
}
