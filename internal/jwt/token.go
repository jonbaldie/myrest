// Package jwt verifies PostgREST-shaped Bearer JWTs and reads the database
// role claim. Signature checks, time checks, audience checks, and the token
// cache live here so the HTTP layer only decides which role to activate.
package jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// The clock skew of the parity target for exp, nbf, and iat.
const skew = 30 * time.Second

// Error kinds that map to PostgREST JWT codes at the HTTP boundary.
var (
	// ErrInvalidToken is a JWT that cannot be decoded or verified (PGRST301).
	ErrInvalidToken = errors.New("jwt: invalid token")
	// ErrClaimsFailed is a JWT whose claims fail validation (PGRST303).
	ErrClaimsFailed = errors.New("jwt: claims validation failed")
	// ErrNoRole means the JWT holds no role claim and no anonymous role is
	// available to fall back to (PGRST302).
	ErrNoRole = errors.New("jwt: no role claim")
)

// Options are the JWT knobs a verifier needs.
type Options struct {
	Secret          string
	SecretIsBase64  bool
	Aud             string
	RoleClaimKey    string
	CacheMaxEntries int
	// Now overrides the clock for tests. Production leaves it nil.
	Now func() time.Time
}

// Verifier checks Bearer JWTs and returns the database role of the claim.
type Verifier struct {
	key          []byte
	aud          string
	roleClaimKey string
	now          func() time.Time
	cache        *tokenCache
}

// New builds a verifier from the JWT knobs. An empty secret means myrest
// cannot verify tokens; callers must not ask it to.
func New(options Options) (*Verifier, error) {
	key := []byte(options.Secret)
	if options.SecretIsBase64 {
		decoded, err := base64.StdEncoding.DecodeString(options.Secret)
		if err != nil {
			return nil, fmt.Errorf("jwt-secret-is-base64: %w", err)
		}
		key = decoded
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	roleKey := options.RoleClaimKey
	if roleKey == "" {
		roleKey = ".role"
	}
	return &Verifier{
		key:          key,
		aud:          options.Aud,
		roleClaimKey: roleKey,
		now:          now,
		cache:        newTokenCache(options.CacheMaxEntries),
	}, nil
}

// Role is the database role named by a valid JWT, or ErrNoRole when the JWT
// is valid but names no role.
func (v *Verifier) Role(token string) (schemacache.Role, error) {
	claims, err := v.parse(token)
	if err != nil {
		return "", err
	}
	role, ok := roleClaim(claims, v.roleClaimKey)
	if !ok || role == "" {
		return "", ErrNoRole
	}
	return schemacache.Role(role), nil
}

func (v *Verifier) parse(token string) (map[string]any, error) {
	if err := checkTokenShape(token); err != nil {
		return nil, err
	}
	if claims, hit := v.cache.get(token); hit {
		return claims, v.checkClaims(claims)
	}

	claims, err := v.verifySignature(token)
	if err != nil {
		return nil, err
	}
	v.cache.put(token, claims)
	return claims, v.checkClaims(claims)
}

func checkTokenShape(token string) error {
	if token == "" {
		return fmt.Errorf("%w: empty token", ErrInvalidToken)
	}
	if parts := strings.Count(token, ".") + 1; parts != 3 {
		return fmt.Errorf("%w: expected 3 parts, got %d", ErrInvalidToken, parts)
	}
	return nil
}

func (v *Verifier) verifySignature(token string) (map[string]any, error) {
	parsed, err := gojwt.Parse(token, func(t *gojwt.Token) (any, error) {
		if t.Method == nil || t.Method.Alg() != gojwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: unsupported algorithm", ErrInvalidToken)
		}
		return v.key, nil
	}, gojwt.WithoutClaimsValidation())
	if err != nil || !parsed.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	mapClaims, ok := parsed.Claims.(gojwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: claims are not a JSON object", ErrClaimsFailed)
	}
	return map[string]any(mapClaims), nil
}

func (v *Verifier) checkClaims(claims map[string]any) error {
	now := v.now().Unix()
	skewSeconds := int64(skew.Seconds())

	if err := checkTimeClaim(claims, "exp", func(value int64) bool {
		return now-skewSeconds > value
	}, "JWT expired"); err != nil {
		return err
	}
	if err := checkTimeClaim(claims, "nbf", func(value int64) bool {
		return now+skewSeconds < value
	}, "JWT not yet valid"); err != nil {
		return err
	}
	if err := checkTimeClaim(claims, "iat", func(value int64) bool {
		return now+skewSeconds < value
	}, "JWT issued at future"); err != nil {
		return err
	}
	if v.aud == "" {
		return nil
	}
	return checkAudience(claims, v.aud)
}

func checkTimeClaim(claims map[string]any, name string, invalid func(int64) bool, message string) error {
	raw, held := claims[name]
	if !held || raw == nil {
		return nil
	}
	value, ok := asInt64(raw)
	if !ok {
		return fmt.Errorf("%w: the JWT %q claim must be a number", ErrClaimsFailed, name)
	}
	if invalid(value) {
		return fmt.Errorf("%w: %s", ErrClaimsFailed, message)
	}
	return nil
}

func checkAudience(claims map[string]any, want string) error {
	raw, held := claims["aud"]
	if !held || raw == nil {
		return nil
	}
	matched, typed := audienceMatches(raw, want)
	if !typed {
		return fmt.Errorf("%w: the JWT aud claim must be a string or an array of strings", ErrClaimsFailed)
	}
	if !matched {
		return fmt.Errorf("%w: JWT not in audience", ErrClaimsFailed)
	}
	return nil
}

func audienceMatches(raw any, want string) (matched, typed bool) {
	switch value := raw.(type) {
	case string:
		return value == want, true
	case []any:
		if len(value) == 0 {
			return true, true
		}
		for _, one := range value {
			text, ok := one.(string)
			if ok && text == want {
				return true, true
			}
		}
		return false, true
	default:
		return false, false
	}
}

func asInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		return int64(value), true
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case int64:
		return value, true
	case int:
		return int64(value), true
	default:
		return 0, false
	}
}

// roleClaim walks a JSPath key path such as ".role" or ".myrest.role".
func roleClaim(claims map[string]any, path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, "."), ".")
	current, ok := walkClaimPath(claims, parts)
	if !ok {
		return "", false
	}
	return claimAsString(current)
}

func walkClaimPath(claims map[string]any, parts []string) (any, bool) {
	var current any = claims
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func claimAsString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case nil:
		return "", false
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

// tokenCache bounds verified signature results the way jwt-cache-max-entries
// does. A zero max turns the cache off. Entries stay after claim failures so
// an expired token stays fast on the signature path.
type tokenCache struct {
	mu      sync.Mutex
	max     int
	order   []string
	entries map[string]map[string]any
}

func newTokenCache(max int) *tokenCache {
	if max <= 0 {
		return &tokenCache{max: 0}
	}
	return &tokenCache{
		max:     max,
		entries: make(map[string]map[string]any, max),
	}
}

func (c *tokenCache) get(token string) (map[string]any, bool) {
	if c == nil || c.max == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	claims, ok := c.entries[token]
	return claims, ok
}

func (c *tokenCache) put(token string, claims map[string]any) {
	if c == nil || c.max == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, held := c.entries[token]; held {
		return
	}
	c.evictToMakeRoom()
	c.entries[token] = claims
	c.order = append(c.order, token)
}

func (c *tokenCache) evictToMakeRoom() {
	for c.full() {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *tokenCache) full() bool {
	return len(c.entries) >= c.max && len(c.order) > 0
}

// Len reports how many tokens the cache holds. Tests use it to prove the bound.
func (c *tokenCache) Len() int {
	if c == nil || c.max == 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// CacheLen is the number of cached tokens for the verifier.
func (v *Verifier) CacheLen() int {
	return v.cache.Len()
}
