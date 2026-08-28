package main

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/jwt"
	"github.com/jonbaldie/myrest/internal/readquery"
)

type verdict int

const (
	verdictOK verdict = iota
	verdictDiscard
	verdictFail
)

type checkResult struct {
	Verdict  verdict
	Property string
	Detail   string
	Cover    []string
}

type finding struct {
	Property string `json:"property"`
	Detail   string `json:"detail"`
	Input    string `json:"input"`
	Shrunk   string `json:"shrunk,omitempty"`
}

var codePattern = regexp.MustCompile(`^(PGRST|MYREST)\d+$`)

var codeStatus = map[string][]int{
	"PGRST100":  {http.StatusBadRequest},
	"PGRST102":  {http.StatusBadRequest},
	"PGRST105":  {http.StatusBadRequest},
	"PGRST106":  {http.StatusNotAcceptable},
	"PGRST107":  {http.StatusUnsupportedMediaType},
	"PGRST116":  {http.StatusNotAcceptable},
	"PGRST122":  {http.StatusBadRequest},
	"PGRST123":  {http.StatusBadRequest},
	"PGRST124":  {http.StatusBadRequest},
	"PGRST127":  {http.StatusBadRequest},
	"PGRST200":  {http.StatusBadRequest},
	"PGRST201":  {http.StatusBadRequest},
	"PGRST202":  {http.StatusNotFound},
	"PGRST204":  {http.StatusBadRequest},
	"PGRST205":  {http.StatusNotFound},
	"PGRST300":  {http.StatusInternalServerError},
	"PGRST301":  {http.StatusUnauthorized},
	"PGRST302":  {http.StatusUnauthorized},
	"PGRST303":  {http.StatusUnauthorized},
	"MYREST001": {http.StatusBadRequest},
	"MYREST002": {
		http.StatusBadRequest, http.StatusForbidden, http.StatusConflict,
		http.StatusInternalServerError, http.StatusServiceUnavailable,
	},
	"MYREST003": {http.StatusNotFound},
}

func evaluate(campaign *campaign, in input) (result checkResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = checkResult{
				Verdict:  verdictFail,
				Property: "no-panic",
				Detail:   fmt.Sprintf("panic: %v", recovered),
				Cover:    []string{"panic"},
			}
		}
	}()
	switch in.Kind {
	case kindParse:
		return evalParse(in)
	case kindJWT:
		return evalJWT(campaign.verifier, in)
	default:
		return evalHTTP(campaign, in)
	}
}

func evalParse(in input) checkResult {
	values := url.Values{}
	values.Set("select", in.Select)
	values.Set("order", in.Order)
	values.Set("name", "eq."+in.Filter)
	if in.Group != "" {
		values.Set("or", "("+in.Group+")")
	}
	query, err := readquery.Parse(values, nil)
	cover := []string{"parse"}
	if err != nil {
		cover = append(cover, "parse-err")
		if extraClosingParen(in.Group) || extraClosingParen(in.Filter) {
			cover = append(cover, "parse-unbalanced")
		}
		return checkResult{Verdict: verdictOK, Cover: cover}
	}
	cover = append(cover, "parse-ok")
	if extraClosingParen(in.Group) && !strings.ContainsAny(in.Group, "()") {
		// Group is wrapped by the property as "("+group+")". An extra ")"
		// after a value with no inner parens must fail.
		if !strings.ContainsAny(in.Group, "()") && strings.HasSuffix(in.Group, ")") {
			return checkResult{
				Verdict:  verdictFail,
				Property: "parse-rejects-extra-paren",
				Detail:   fmt.Sprintf("accepted extra closing paren in group %q", in.Group),
				Cover:    cover,
			}
		}
	}
	if in.Filter != "" && len(query.Filters) == 1 && query.Filters[0].Op == readquery.OpEq && query.Filters[0].Value != in.Filter {
		return checkResult{
			Verdict:  verdictFail,
			Property: "parse-keeps-eq-value",
			Detail:   fmt.Sprintf("eq value %q became %q", in.Filter, query.Filters[0].Value),
			Cover:    cover,
		}
	}
	return checkResult{Verdict: verdictOK, Cover: cover}
}

func extraClosingParen(value string) bool {
	return strings.Count(value, ")") > strings.Count(value, "(")
}

func evalJWT(verifier *jwt.Verifier, in input) checkResult {
	_, err := verifier.Role(in.Token)
	cover := []string{"jwt"}
	if err == nil {
		return checkResult{Verdict: verdictOK, Cover: append(cover, "jwt-ok")}
	}
	if in.Token == "" {
		return checkResult{Verdict: verdictOK, Cover: append(cover, "jwt-empty")}
	}
	if strings.Count(in.Token, ".") != 2 {
		var decode jwt.DecodeFailure
		if !asDecode(err) {
			return checkResult{
				Verdict:  verdictFail,
				Property: "jwt-shape",
				Detail:   fmt.Sprintf("bad token shape gave %T %v", err, err),
				Cover:    cover,
			}
		}
		_ = decode
		return checkResult{Verdict: verdictOK, Cover: append(cover, "jwt-shape")}
	}
	return checkResult{Verdict: verdictOK, Cover: append(cover, "jwt-err")}
}

func asDecode(err error) bool {
	var decode jwt.DecodeFailure
	return errorAs(err, &decode)
}

func errorAs(err error, target *jwt.DecodeFailure) bool {
	return err != nil && (strings.Contains(err.Error(), "jwt: invalid token") ||
		strings.Contains(err.Error(), "Expected 3 parts") ||
		strings.Contains(err.Error(), "Empty JWT"))
}

func evalHTTP(campaign *campaign, in input) checkResult {
	if in.Method == "" || in.Path == "" || !strings.HasPrefix(in.Path, "/") {
		return checkResult{Verdict: verdictDiscard, Cover: []string{"http-discard-path"}}
	}
	wire, err := doHTTP(campaign.url, in)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return checkResult{Verdict: verdictDiscard, Cover: []string{"http-discard-req"}}
		}
		return checkResult{
			Verdict:  verdictFail,
			Property: "http-reachable",
			Detail:   err.Error(),
			Cover:    []string{"http-err"},
		}
	}
	cover := httpCover(in, wire)
	if fail := checkEnvelope(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkCodeStatus(wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkNoLeak(wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkContentRange(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkUnboundedWrite(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkUnknownTable(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkBadProfile(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkAuthStatus(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	if fail := checkHeadEmpty(in, wire); fail != nil {
		fail.Cover = cover
		return *fail
	}
	return checkResult{Verdict: verdictOK, Cover: cover}
}

func httpCover(in input, wire wireResponse) []string {
	code := envelopeCode(wire.Body)
	return []string{
		"http",
		"m:" + in.Method,
		"p:" + pathClass(in.Path),
		"s:" + strconv.Itoa(wire.Status),
		"c:" + code,
		"ct:" + mediaClass(wire.Header.Get("Content-Type")),
	}
}

func effectivePath(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return raw
}

func pathClass(raw string) string {
	requestPath := effectivePath(raw)
	switch {
	case requestPath == "/":
		return "root"
	case strings.HasPrefix(requestPath, "/rpc/"):
		return "rpc"
	case requestPath == "/items", requestPath == "/orders", requestPath == "/secrets":
		return strings.TrimPrefix(requestPath, "/")
	default:
		return "other"
	}
}

func mediaClass(value string) string {
	switch {
	case value == "", strings.HasPrefix(value, "application/json"):
		return "json"
	case strings.HasPrefix(value, "text/csv"):
		return "csv"
	case strings.Contains(value, "openapi"):
		return "openapi"
	default:
		return "other"
	}
}

func envelopeCode(body []byte) string {
	fields, ok := envelopeOf(body)
	if !ok {
		return ""
	}
	code, _ := jsonString(fields["code"])
	return code
}

func handledRoute(in input) bool {
	switch pathClass(in.Path) {
	case "items", "orders", "secrets":
		switch in.Method {
		case httpGET, httpHEAD, httpPOST, httpPATCH, httpPUT, httpDELETE, httpOPTIONS:
			return true
		}
	case "rpc":
		if in.Path == "/rpc" || in.Path == "/rpc/" {
			return false
		}
		switch in.Method {
		case httpGET, httpHEAD, httpPOST, httpOPTIONS:
			return true
		}
	}
	return false
}

func checkEnvelope(in input, wire wireResponse) *checkResult {
	if wire.Status < 400 {
		return nil
	}
	// net/http drops the body of a HEAD answer.
	if in.Method == httpHEAD {
		return nil
	}
	if ct := wire.Header.Get("Content-Type"); ct != "application/json" {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "error-content-type",
			Detail:   fmt.Sprintf("status %d Content-Type %q", wire.Status, ct),
		}
	}
	fields, ok := envelopeOf(wire.Body)
	if !ok {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "error-envelope-json",
			Detail:   fmt.Sprintf("status %d body %q", wire.Status, wire.Body),
		}
	}
	for _, field := range []string{"code", "message", "details", "hint"} {
		if _, held := fields[field]; !held {
			return &checkResult{
				Verdict:  verdictFail,
				Property: "error-envelope-fields",
				Detail:   fmt.Sprintf("missing %s in %s", field, wire.Body),
			}
		}
	}
	code, ok := jsonString(fields["code"])
	if !ok || !codePattern.MatchString(code) {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "error-code-family",
			Detail:   fmt.Sprintf("code %q", fields["code"]),
		}
	}
	message, ok := jsonString(fields["message"])
	if !ok || message == "" {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "error-message",
			Detail:   fmt.Sprintf("message %s", fields["message"]),
		}
	}
	return nil
}

func checkCodeStatus(wire wireResponse) *checkResult {
	if wire.Status < 400 {
		return nil
	}
	code := envelopeCode(wire.Body)
	allowed, held := codeStatus[code]
	if !held {
		return nil
	}
	for _, status := range allowed {
		if wire.Status == status {
			return nil
		}
	}
	return &checkResult{
		Verdict:  verdictFail,
		Property: "code-status",
		Detail:   fmt.Sprintf("code %s status %d want %v", code, wire.Status, allowed),
	}
}

func checkNoLeak(wire wireResponse) *checkResult {
	body := string(wire.Body)
	for _, secret := range []string{jwtSecret, "authenticator:secret", "campaign-secret"} {
		if strings.Contains(body, secret) {
			return &checkResult{
				Verdict:  verdictFail,
				Property: "no-secret-leak",
				Detail:   "error body holds a secret",
			}
		}
	}
	return nil
}

func checkContentRange(in input, wire wireResponse) *checkResult {
	if in.Method != httpGET || wire.Status >= 400 {
		return nil
	}
	if pathClass(in.Path) != "items" && pathClass(in.Path) != "orders" && pathClass(in.Path) != "secrets" {
		return nil
	}
	value := wire.Header.Get("Content-Range")
	if value == "" {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "content-range-present",
			Detail:   "successful table read has no Content-Range",
		}
	}
	if value == "*/*" || strings.HasPrefix(value, "*/") {
		return nil
	}
	rangePart, _, ok := strings.Cut(value, "/")
	if !ok {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "content-range-shape",
			Detail:   value,
		}
	}
	startText, endText, ok := strings.Cut(rangePart, "-")
	if !ok {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "content-range-shape",
			Detail:   value,
		}
	}
	start, err1 := strconv.ParseUint(startText, 10, 64)
	end, err2 := strconv.ParseUint(endText, 10, 64)
	if err1 != nil || err2 != nil || start > end {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "content-range-order",
			Detail:   value,
		}
	}
	return nil
}

func checkUnboundedWrite(in input, wire wireResponse) *checkResult {
	if in.Method != httpPATCH && in.Method != httpDELETE {
		return nil
	}
	if in.Path != "/items" {
		return nil
	}
	if hasAuthFailure(wire) || hasProfileFailure(wire) {
		return nil
	}
	if preferHas(in, "all-rows") {
		return nil
	}
	if preferHas(in, "timezone") || preferHas(in, "row-security") || preferHas(in, "jwt-claims") {
		return nil
	}
	values, err := url.ParseQuery(in.RawQuery)
	if err != nil {
		return nil
	}
	if hasWriteFilter(values) {
		return nil
	}
	if wire.Status == http.StatusBadRequest && envelopeCode(wire.Body) == "PGRST122" {
		return nil
	}
	if envelopeCode(wire.Body) != "PGRST100" || wire.Status != http.StatusBadRequest {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "unbounded-write",
			Detail:   fmt.Sprintf("status %d code %s", wire.Status, envelopeCode(wire.Body)),
		}
	}
	return nil
}

func hasWriteFilter(values url.Values) bool {
	for key := range values {
		if key == "select" || key == "order" || key == "limit" || key == "offset" || key == "columns" {
			continue
		}
		return true
	}
	return false
}

func checkUnknownTable(in input, wire wireResponse) *checkResult {
	if pathClass(in.Path) != "other" {
		return nil
	}
	// net/http's ServeMux redirects dot and repeated-slash spellings of the
	// root path. The client follows that redirect, so these are root requests,
	// not unknown-table requests.
	if path.Clean(in.Path) == "/" || strings.HasPrefix(in.Path, "/rpc/") {
		return nil
	}
	if hasAuthFailure(wire) || hasProfileFailure(wire) {
		return nil
	}
	if in.Method == httpOPTIONS && wire.Status == http.StatusOK {
		return nil
	}
	if wire.Status == http.StatusNotFound && (envelopeCode(wire.Body) == "PGRST205" || envelopeCode(wire.Body) == "MYREST003") {
		return nil
	}
	if wire.Status >= 400 {
		return nil
	}
	return &checkResult{
		Verdict:  verdictFail,
		Property: "unknown-table",
		Detail:   fmt.Sprintf("%s %s status %d code %s", in.Method, in.Path, wire.Status, envelopeCode(wire.Body)),
	}
}

func checkBadProfile(in input, wire wireResponse) *checkResult {
	if in.Method != httpGET || (pathClass(in.Path) != "items" && pathClass(in.Path) != "orders" && pathClass(in.Path) != "secrets") {
		return nil
	}
	profile := headerValue(in, "Accept-Profile")
	if in.Method == httpPOST || in.Method == httpPATCH || in.Method == httpPUT || in.Method == httpDELETE {
		profile = headerValue(in, "Content-Profile")
	}
	if profile == "" || profile == "shop" {
		return nil
	}
	if hasAuthFailure(wire) {
		return nil
	}
	if pathClass(in.Path) == "root" || pathClass(in.Path) == "other" {
		return nil
	}
	if envelopeCode(wire.Body) != "PGRST106" || wire.Status != http.StatusNotAcceptable {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "bad-profile",
			Detail:   fmt.Sprintf("profile %q status %d code %s", profile, wire.Status, envelopeCode(wire.Body)),
		}
	}
	return nil
}

func checkAuthStatus(in input, wire wireResponse) *checkResult {
	if !handledRoute(in) || in.Method != httpGET || pathClass(in.Path) == "rpc" {
		return nil
	}
	auth := headerValue(in, "Authorization")
	if auth == "" {
		return nil
	}
	scheme, token, found := strings.Cut(auth, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return nil
	}
	if validLookingJWT(token) {
		return nil
	}
	if preferHas(in, "timezone") || preferHas(in, "row-security") || preferHas(in, "jwt-claims") {
		return nil
	}
	code := envelopeCode(wire.Body)
	if in.Method == httpHEAD && wire.Status == http.StatusUnauthorized {
		return nil
	}
	if wire.Status == http.StatusUnauthorized && code == "PGRST301" {
		return nil
	}
	return &checkResult{
		Verdict:  verdictFail,
		Property: "jwt-http-status",
		Detail:   fmt.Sprintf("token %q status %d code %s", token, wire.Status, code),
	}
}

func validLookingJWT(token string) bool {
	return strings.Count(token, ".") == 2 && len(token) > 20
}

func checkHeadEmpty(in input, wire wireResponse) *checkResult {
	if in.Method != httpHEAD || wire.Status >= 400 {
		return nil
	}
	if len(wire.Body) != 0 {
		return &checkResult{
			Verdict:  verdictFail,
			Property: "head-empty",
			Detail:   fmt.Sprintf("HEAD body %q", wire.Body),
		}
	}
	return nil
}

func hasAuthFailure(wire wireResponse) bool {
	code := envelopeCode(wire.Body)
	return code == "PGRST301" || code == "PGRST302" || code == "PGRST303" || code == "MYREST001"
}

func hasProfileFailure(wire wireResponse) bool {
	return envelopeCode(wire.Body) == "PGRST106"
}

func preferHas(in input, name string) bool {
	for _, header := range in.Headers["Prefer"] {
		for _, part := range strings.Split(header, ",") {
			key, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			if strings.EqualFold(key, name) {
				return true
			}
		}
	}
	return false
}

func headerValue(in input, name string) string {
	values := in.Headers[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
