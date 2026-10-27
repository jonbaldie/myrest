package mysqldb

import (
	"encoding/json"
	"fmt"
)

type nonJSONTarget struct {
	kind string
}

func (e nonJSONTarget) Error() string {
	return fmt.Sprintf("cannot bind JSON %s to a non-JSON target", e.kind)
}

func bindValue(dataType string, value any) (any, error) {
	var kind string
	switch value.(type) {
	case map[string]any:
		kind = "object"
	case []any:
		kind = "array"
	default:
		return value, nil
	}
	if !isJSONDataType(dataType) {
		return nil, nonJSONTarget{kind: kind}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}
