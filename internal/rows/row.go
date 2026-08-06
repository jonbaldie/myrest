// Package rows holds the row shape myrest gives back to a client: the columns
// of a resource in catalog order, with their values.
package rows

import (
	"bytes"
	"encoding/json"
)

// Row is one row of a resource. Columns and Values hold the same number of
// items, in the column order of the resource.
type Row struct {
	Columns []string
	Values  []any
}

// MarshalJSON writes a JSON object that keeps the column order of the
// resource. A Go map cannot do that: it writes its keys in name order.
func (r Row) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for i, column := range r.Columns {
		if i > 0 {
			buffer.WriteByte(',')
		}
		name, err := json.Marshal(column)
		if err != nil {
			return nil, err
		}
		buffer.Write(name)
		buffer.WriteByte(':')

		var value any
		if i < len(r.Values) {
			value = r.Values[i]
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		buffer.Write(encoded)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}
