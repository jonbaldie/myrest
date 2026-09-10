package readquery

// RangeFailure is a Range request window the query cannot satisfy.
type RangeFailure struct {
	// LowerGTUpper is true when the bounds of the Range header itself cross.
	// It is false when the window and the limit/offset of the query do not
	// meet.
	LowerGTUpper bool
}

func (e RangeFailure) Error() string { return "Requested range not satisfiable" }

// ApplyRange applies the Range request window first..last (both ends
// inclusive, last nil when open) to the query as limit/offset. The window
// meets any limit/offset the client also sent, the way the parity target
// intersects them. limit=0 keeps its meaning and bypasses the window. An
// error returns when the window cannot meet the bounds of the query.
func ApplyRange(query *Query, first uint64, last *uint64) error {
	if query.Limit != nil && *query.Limit == 0 {
		return nil
	}
	if last != nil && first > *last {
		return RangeFailure{LowerGTUpper: true}
	}
	end, bounded := limitEnd(query)
	if last != nil && (!bounded || *last < end) {
		end, bounded = *last, true
	}
	if first < query.Offset {
		first = query.Offset
	}
	if bounded && first > end {
		return RangeFailure{}
	}
	query.Offset = first
	if bounded {
		limit := end - first + 1
		query.Limit = &limit
	}
	return nil
}

// limitEnd is the last row of the query window, or false when no limit caps
// it or the cap overflows the row-count space.
func limitEnd(query *Query) (uint64, bool) {
	if query.Limit == nil || *query.Limit == 0 {
		return 0, false
	}
	if *query.Limit-1 > ^uint64(0)-query.Offset {
		return 0, false
	}
	return query.Offset + *query.Limit - 1, true
}
