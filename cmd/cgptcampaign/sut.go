package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/jwt"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

const jwtSecret = "campaign-secret"

type memoryReader struct {
	items  []rows.Row
	orders []rows.Row
}

func (r *memoryReader) Read(
	_ context.Context,
	_ schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	if err := rejectUnknownColumns(table, query); err != nil {
		return readquery.Result{}, err
	}
	source := r.items
	if table.ID.Name == "orders" {
		source = r.orders
	}
	if table.ID.Name == "secrets" {
		source = []rows.Row{{Columns: []string{"payload"}, Values: []any{"hidden"}}}
	}
	if query.Offset >= uint64(len(source)) {
		return readquery.Result{}, nil
	}
	start := int(query.Offset)
	end := len(source)
	if limit := query.EffectiveLimit(); limit != nil {
		if *limit > uint64(end-start) {
			return readquery.Result{Rows: append([]rows.Row(nil), source[start:end]...)}, nil
		}
		end = start + int(*limit)
	}
	return readquery.Result{Rows: append([]rows.Row(nil), source[start:end]...)}, nil
}

func rejectUnknownColumns(table schemacache.Table, query readquery.Query) error {
	known := map[string]bool{}
	for _, column := range table.Columns {
		known[column.Name] = true
	}
	for _, column := range query.Columns {
		if column.Name != "" && !known[column.Name] {
			return readquery.ColumnNotFound{Name: column.Name}
		}
	}
	for _, filter := range query.Filters {
		if filter.Column != "" && !known[filter.Column] {
			return readquery.ColumnNotFound{Name: filter.Column}
		}
	}
	for _, order := range query.Order {
		if order.Column != "" && !known[order.Column] {
			return readquery.ColumnNotFound{Name: order.Column}
		}
	}
	return nil
}

type memoryWriter struct{}

func (memoryWriter) Insert(
	_ context.Context,
	_ schemacache.Role,
	_ schemacache.Table,
	body []map[string]any,
	options writequery.Options,
) (writequery.Result, error) {
	if options.MaxAffected != nil && int64(len(body)) > *options.MaxAffected {
		return writequery.Result{}, writequery.MaxAffectedExceeded{
			Affected: int64(len(body)),
			Max:      *options.MaxAffected,
		}
	}
	return writequery.Result{Affected: int64(len(body))}, nil
}

func (memoryWriter) Update(
	_ context.Context,
	_ schemacache.Role,
	_ schemacache.Table,
	_ map[string]any,
	_ readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	if options.MaxAffected != nil && *options.MaxAffected == 0 {
		return writequery.Result{}, writequery.MaxAffectedExceeded{Affected: 1, Max: 0}
	}
	return writequery.Result{Affected: 1}, nil
}

func (memoryWriter) Delete(
	_ context.Context,
	_ schemacache.Role,
	_ schemacache.Table,
	_ readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	if options.MaxAffected != nil && *options.MaxAffected == 0 {
		return writequery.Result{}, writequery.MaxAffectedExceeded{Affected: 1, Max: 0}
	}
	return writequery.Result{Affected: 1}, nil
}

func (memoryWriter) Upsert(
	_ context.Context,
	_ schemacache.Role,
	_ schemacache.Table,
	_ map[string]any,
	_ []string,
	_ httpapi.UpsertResolution,
	_ writequery.Options,
) (bool, error) {
	return true, nil
}

type memoryCaller struct{}

func (memoryCaller) Call(
	_ context.Context,
	_ schemacache.Role,
	routine schemacache.RoutineFact,
	_ map[string]any,
	_ httpapi.CallOptions,
) (any, error) {
	if routine.ID.Name == "list_items" {
		return []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		}, nil
	}
	if routine.ID.Name == "add_them" {
		return int64(3), nil
	}
	return nil, nil
}

func campaignCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	secrets := schemacache.TableID{Database: "shop", Name: "secrets"}
	addThem := schemacache.RoutineID{Database: "shop", Name: "add_them"}
	listItems := schemacache.RoutineID{Database: "shop", Name: "list_items"}
	writeMarker := schemacache.RoutineID{Database: "shop", Name: "write_marker"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders, secrets},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id", DataType: "bigint"},
			{Table: items, Name: "name", DataType: "varchar", Collation: "utf8mb4_0900_ai_ci"},
			{Table: items, Name: "meta", DataType: "json"},
			{Table: orders, Name: "id", DataType: "bigint"},
			{Table: orders, Name: "item_id", DataType: "bigint"},
			{Table: secrets, Name: "payload", DataType: "varchar"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name:              "orders_item",
			Table:             orders,
			Columns:           []string{"item_id"},
			ReferencedTable:   items,
			ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: orders},
			{Role: "myrest_user", Table: items},
			{Role: "myrest_user", Table: orders},
			{Role: "myrest_user", Table: secrets},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: items, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: items, Privilege: "UPDATE"},
			{Role: "myrest_anon", Table: items, Privilege: "DELETE"},
			{Role: "myrest_user", Table: items, Privilege: "INSERT"},
			{Role: "myrest_user", Table: items, Privilege: "UPDATE"},
			{Role: "myrest_user", Table: items, Privilege: "DELETE"},
		},
		Routines: []schemacache.RoutineFact{
			{
				ID:            addThem,
				Kind:          "FUNCTION",
				ReturnType:    "bigint",
				SQLDataAccess: "NO SQL",
				Parameters: []schemacache.ParameterFact{
					{Ordinal: 0, DataType: "bigint"},
					{Name: "a", Mode: "IN", Ordinal: 1, DataType: "bigint"},
					{Name: "b", Mode: "IN", Ordinal: 2, DataType: "bigint"},
				},
			},
			{
				ID:            listItems,
				Kind:          "PROCEDURE",
				SQLDataAccess: "READS SQL DATA",
			},
			{
				ID:            writeMarker,
				Kind:          "PROCEDURE",
				SQLDataAccess: "MODIFIES SQL DATA",
			},
		},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: "myrest_anon", Routine: addThem, Privilege: "EXECUTE"},
			{Role: "myrest_anon", Routine: listItems, Privilege: "EXECUTE"},
			{Role: "myrest_user", Routine: addThem, Privilege: "EXECUTE"},
			{Role: "myrest_user", Routine: listItems, Privilege: "EXECUTE"},
			{Role: "myrest_user", Routine: writeMarker, Privilege: "EXECUTE"},
		},
	})
}

func campaignSettings() config.Settings {
	resolved := config.Defaults()
	resolved.DB.URI = "mysql://authenticator:secret@127.0.0.1:3306/"
	resolved.DB.Schemas = []string{"shop"}
	resolved.DB.AnonRole = "myrest_anon"
	resolved.DB.AggregatesEnabled = true
	resolved.JWT.Secret = jwtSecret
	return resolved
}

func startService() (*httpapi.Service, error) {
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: campaignSettings(),
		Cache:    campaignCache(),
		Reader: &memoryReader{
			items: []rows.Row{
				{Columns: []string{"id", "name", "meta"}, Values: []any{int64(1), "alpha", `{"k":"v"}`}},
				{Columns: []string{"id", "name", "meta"}, Values: []any{int64(2), "beta", `{"k":"w"}`}},
			},
			orders: []rows.Row{
				{Columns: []string{"id", "item_id"}, Values: []any{int64(10), int64(1)}},
			},
		},
		Writer: memoryWriter{},
		Caller: memoryCaller{},
		Log:    log.New(io.Discard, "", 0),
	})
	if err != nil {
		return nil, err
	}
	go func() { _ = service.Serve() }()
	return service, nil
}

func newVerifier() (*jwt.Verifier, error) {
	return jwt.New(jwt.Options{Secret: jwtSecret, RoleClaimKey: ".role"})
}

func signedRoleToken(role string, extra map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := map[string]any{"role": role}
	for key, value := range extra {
		claims[key] = value
	}
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(payload)
	unsigned := header + "." + body
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	_, _ = mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig
}

type wireResponse struct {
	Status  int
	Header  http.Header
	Body    []byte
	Elapsed time.Duration
	Panic   string
}

func doHTTP(baseURL string, in input) (wireResponse, error) {
	started := time.Now()
	var reader io.Reader
	if in.Body != "" {
		reader = strings.NewReader(in.Body)
	}
	request, err := http.NewRequest(in.Method, baseURL+in.Path, reader)
	if err != nil {
		return wireResponse{}, err
	}
	if in.RawQuery != "" {
		request.URL.RawQuery = in.RawQuery
	}
	for name, values := range in.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return wireResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return wireResponse{}, err
	}
	return wireResponse{
		Status:  response.StatusCode,
		Header:  response.Header.Clone(),
		Body:    bytes.TrimSuffix(body, []byte("\n")),
		Elapsed: time.Since(started),
	}, nil
}

func envelopeOf(body []byte) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

func jsonString(raw json.RawMessage) (string, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false
	}
	return text, true
}

func formatInput(in input) string {
	return fmt.Sprintf("%s %s?%s headers=%v body=%q token=%q select=%q",
		in.Kind, in.Method+" "+in.Path, in.RawQuery, in.Headers, in.Body, in.Token, in.Select)
}
