package mysqldb

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestValueBindingContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		dataType      string
		value         any
		want          any
		writeError    string
		argumentError string
		marshalError  bool
	}{
		{name: "object", dataType: "json", value: map[string]any{"k": float64(1)}, want: `{"k":1}`},
		{name: "array", dataType: "JSON", value: []any{float64(1), "two", nil}, want: `[1,"two",null]`},
		{name: "mixed case object", dataType: "JsOn", value: map[string]any{}, want: `{}`},
		{name: "mixed case array", dataType: "jSoN", value: []any{}, want: `[]`},
		{name: "nil object", dataType: "json", value: map[string]any(nil), want: `null`},
		{name: "nil array", dataType: "json", value: []any(nil), want: `null`},
		{
			name: "object on non JSON", dataType: "varchar", value: map[string]any{"k": float64(1)},
			writeError:    "Cannot write a JSON object into column doc: the column does not hold JSON",
			argumentError: "Cannot pass a JSON object to parameter doc: the parameter does not hold JSON",
		},
		{
			name: "array on non JSON", dataType: "int", value: []any{float64(1)},
			writeError:    "Cannot write a JSON array into column doc: the column does not hold JSON",
			argumentError: "Cannot pass a JSON array to parameter doc: the parameter does not hold JSON",
		},
		{
			name: "unknown target", value: map[string]any{},
			writeError:    "Cannot write a JSON object into column doc: the column does not hold JSON",
			argumentError: "Cannot pass a JSON object to parameter doc: the parameter does not hold JSON",
		},
		{name: "null", dataType: "json", value: nil, want: nil},
		{name: "non JSON null", dataType: "varchar", value: nil, want: nil},
		{name: "string", dataType: "varchar", value: "hello", want: "hello"},
		{name: "JSON document string", dataType: "json", value: `{"a":1}`, want: `{"a":1}`},
		{name: "boolean", dataType: "boolean", value: true, want: true},
		{name: "float", dataType: "double", value: float64(1.25), want: float64(1.25)},
		{name: "integer", dataType: "int", value: int64(42), want: int64(42)},
		{name: "exact integer", dataType: "bigint", value: json.Number("9007199254740993"), want: json.Number("9007199254740993")},
		{name: "exact decimal", dataType: "decimal", value: json.Number("123.4500"), want: json.Number("123.4500")},
		{
			name: "nested exact numbers", dataType: "json",
			value: map[string]any{"n": []any{json.Number("9007199254740993"), json.Number("123.4500")}},
			want:  `{"n":[9007199254740993,123.4500]}`,
		},
		{name: "unencodable object", dataType: "json", value: map[string]any{"bad": func() {}}, marshalError: true},
		{name: "unencodable array", dataType: "json", value: []any{func() {}}, marshalError: true},
		{
			name: "refuse before marshaling", dataType: "varchar", value: map[string]any{"bad": func() {}},
			writeError:    "Cannot write a JSON object into column doc: the column does not hold JSON",
			argumentError: "Cannot pass a JSON object to parameter doc: the parameter does not hold JSON",
		},
	}
	adapters := []struct {
		name string
		bind func(string, any) (any, error)
	}{
		{"write", func(dataType string, value any) (any, error) {
			return writeValue(schemacache.Column{DataType: dataType}, "doc", value)
		}},
		{"argument", func(dataType string, value any) (any, error) {
			return boundArgument(schemacache.ParameterFact{Name: "doc", DataType: dataType}, map[string]any{"doc": value})
		}},
	}
	for _, adapter := range adapters {
		t.Run(adapter.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					got, err := adapter.bind(tt.dataType, tt.value)
					wantError := tt.writeError
					if adapter.name == "argument" {
						wantError = tt.argumentError
					}
					if wantError != "" {
						var refusal readquery.UnsupportedFeature
						if !errors.As(err, &refusal) || refusal.Message != wantError || got != nil {
							t.Fatalf("bind = (%#v, %v), want (nil, UnsupportedFeature{%q})", got, err, wantError)
						}
						return
					}
					if tt.marshalError {
						var marshalError *json.UnsupportedTypeError
						if !errors.As(err, &marshalError) || got != nil {
							t.Fatalf("bind = (%#v, %v), want (nil, *json.UnsupportedTypeError)", got, err)
						}
						return
					}
					if err != nil || !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("bind = (%#v, %v), want (%#v, nil)", got, err, tt.want)
					}
				})
			}
		})
	}
}

func TestBoundArgumentKeepsMissingArgument(t *testing.T) {
	t.Parallel()

	got, err := boundArgument(schemacache.ParameterFact{Name: "doc", DataType: "json"}, map[string]any{})
	var missing MissingArgument
	if !errors.As(err, &missing) || missing.Name != "doc" || got != nil {
		t.Fatalf("bind = (%#v, %v), want (nil, MissingArgument{Name: doc})", got, err)
	}
}
