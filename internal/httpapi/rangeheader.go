package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
)

const (
	headerRange     = "Range"
	headerRangeUnit = "Range-Unit"
	// rangeUnitItems is the only range unit myrest answers, the unit it
	// advertises on read answers.
	rangeUnitItems = "items"
)

// itemRange is one inclusive item window from the Range request header.
type itemRange struct {
	offset uint64
	// limit is nil for an open-ended range such as "5-", which skips rows
	// but does not cap them.
	limit *uint64
}

// applyItemRange puts the Range request header on the query as the same page
// as offset and limit. A limit or offset in the query string wins, the way the
// query string owns the page in the parity contract. It refuses a malformed
// range, a reversed range, and a range unit other than items.
func applyItemRange(request *http.Request, query *readquery.Query) error {
	raw := strings.TrimSpace(request.Header.Get(headerRange))
	if raw == "" {
		return nil
	}
	if err := checkRangeUnit(request.Header.Get(headerRangeUnit)); err != nil {
		return err
	}
	window, err := parseItemRange(raw)
	if err != nil {
		return err
	}
	if query.Limit != nil || query.Offset != 0 {
		return nil
	}
	query.Offset = window.offset
	query.Limit = window.limit
	return nil
}

func checkRangeUnit(unit string) error {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.EqualFold(unit, rangeUnitItems) {
		return nil
	}
	return readquery.ParseFailure{Message: "unsupported range unit '" + unit + "'"}
}

// parseItemRange reads "start-end" and the open-ended "start-".
func parseItemRange(raw string) (itemRange, error) {
	first, last, found := strings.Cut(raw, "-")
	if !found {
		return itemRange{}, rangeFailure(raw)
	}
	start, err := strconv.ParseUint(strings.TrimSpace(first), 10, 64)
	if err != nil {
		return itemRange{}, rangeFailure(raw)
	}
	if strings.TrimSpace(last) == "" {
		return itemRange{offset: start}, nil
	}
	end, err := strconv.ParseUint(strings.TrimSpace(last), 10, 64)
	if err != nil || end < start {
		return itemRange{}, rangeFailure(raw)
	}
	count := end - start + 1
	return itemRange{offset: start, limit: &count}, nil
}

func rangeFailure(raw string) error {
	return readquery.ParseFailure{
		Message: "range '" + raw + "' must be an inclusive item range such as 0-9",
	}
}
