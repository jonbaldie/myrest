package representation

import (
	"encoding/csv"
	"encoding/json"
	"io"

	"github.com/jonbaldie/myrest/internal/rows"
)

// Format writes the body of row data in the representation. It writes no
// status or headers. A singular object writes the one row, or refuses with
// SingularObjectRefusal and writes nothing. CSV writes the header record,
// then one record per row. With no header, CSV uses the columns of the first
// row; with no header and no rows it writes nothing.
func Format(w io.Writer, spec Spec, bodyRows []rows.Row, header []string) error {
	switch spec.Kind {
	case KindSingularObject:
		if err := ValidateCardinality(spec, len(bodyRows)); err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(bodyRows[0])
	case KindCSV:
		return formatCSV(w, bodyRows, header)
	default:
		return json.NewEncoder(w).Encode(bodyRows)
	}
}

func formatCSV(w io.Writer, bodyRows []rows.Row, header []string) error {
	if len(header) == 0 && len(bodyRows) > 0 {
		header = bodyRows[0].Columns
	}
	if len(header) == 0 {
		return nil
	}
	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write(header)
	for _, row := range bodyRows {
		record := make([]string, len(header))
		for i := range header {
			record[i] = csvCell(row.Values, i)
		}
		_ = csvWriter.Write(record)
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

// csvCell is one CSV field. A SQL NULL and a missing value are empty.
func csvCell(values []any, index int) string {
	if index >= len(values) || values[index] == nil {
		return ""
	}
	switch typed := values[index].(type) {
	case []byte:
		return string(typed)
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return csvEncoded(typed)
	}
}

// csvEncoded writes a JSON value as its JSON text. A JSON string loses its
// quotes. A value with no JSON encoding is empty.
func csvEncoded(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	if len(encoded) >= 2 && encoded[0] == '"' {
		var unquoted string
		if json.Unmarshal(encoded, &unquoted) == nil {
			return unquoted
		}
	}
	return string(encoded)
}
