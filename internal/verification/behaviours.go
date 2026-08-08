package verification

// FullMatchBehaviours lists full-match labelled behaviours and their scenarios.
// Partial match and not supported behaviours live in chapter Gap list rows.
func FullMatchBehaviours() []Behaviour {
	return []Behaviour{
		{Area: "auth", Item: "Bearer JWT ordinary read", Label: FullMatch, Scenarios: []string{"auth-001"}},
		{Area: "auth", Item: "Anonymous database role ordinary read", Label: FullMatch, Scenarios: []string{"auth-002"}},
		{Area: "auth", Item: "Invalid or expired JWT error", Label: FullMatch, Scenarios: []string{"auth-003"}},

		{Area: "schema-cache", Item: "Privileged object exposed as resource", Label: FullMatch, Scenarios: []string{"cache-001"}},
		{Area: "schema-cache", Item: "Unprivileged object hidden", Label: FullMatch, Scenarios: []string{"cache-002"}},
		{Area: "schema-cache", Item: "Explicit schema cache reload", Label: FullMatch, Scenarios: []string{"cache-003"}},

		{Area: "config", Item: "Serve gate for incomplete minimum run set", Label: FullMatch, Scenarios: []string{"cfg-001"}},
		{Area: "config", Item: "MYREST_* env equivalent to config file", Label: FullMatch, Scenarios: []string{"cfg-002"}},

		{Area: "read", Item: "Ordinary read select/filter/order/page", Label: FullMatch, Scenarios: []string{"read-001"}},
		{Area: "read", Item: "Prefer count=exact", Label: FullMatch, Scenarios: []string{"read-002"}},
		{Area: "read", Item: "Aggregates when enabled", Label: FullMatch, Scenarios: []string{"read-010"}},
		{Area: "read", Item: "Aggregates refused when disabled", Label: FullMatch, Scenarios: []string{"read-011"}},
		{Area: "read", Item: "Aggregate plus embed allowed combo", Label: FullMatch, Scenarios: []string{"read-012"}},

		{Area: "embed", Item: "Nested select over declared foreign key", Label: FullMatch, Scenarios: []string{"embed-001"}},
		{Area: "embed", Item: "Nested filter/order/limit", Label: FullMatch, Scenarios: []string{"embed-002"}},

		{Area: "write", Item: "POST insert including bulk", Label: FullMatch, Scenarios: []string{"write-001"}},
		{Area: "write", Item: "PATCH by filter", Label: FullMatch, Scenarios: []string{"write-002"}},
		{Area: "write", Item: "DELETE by filter", Label: FullMatch, Scenarios: []string{"write-003"}},
		{Area: "write", Item: "PUT upsert by primary key", Label: FullMatch, Scenarios: []string{"write-004"}},
		{Area: "write", Item: "Unbounded write safety gate", Label: FullMatch, Scenarios: []string{"write-005"}},
		{Area: "write", Item: "Writable view write", Label: FullMatch, Scenarios: []string{"write-006"}},
		{Area: "write", Item: "Prefer return=minimal and headers-only", Label: FullMatch, Scenarios: []string{"write-007"}},
		{Area: "write", Item: "Prefer missing/max-affected/handling", Label: FullMatch, Scenarios: []string{"write-010"}},
		{Area: "write", Item: "Embed after write with representation", Label: FullMatch, Scenarios: []string{"write-011"}},

		{Area: "rpc", Item: "POST /rpc function with named JSON args", Label: FullMatch, Scenarios: []string{"rpc-001"}},
		{Area: "rpc", Item: "POST /rpc procedure stable response", Label: FullMatch, Scenarios: []string{"rpc-002"}},
		{Area: "rpc", Item: "Row-set RPC filter/order/page/embed", Label: FullMatch, Scenarios: []string{"rpc-005"}},

		{Area: "representation", Item: "JSON primary representation", Label: FullMatch, Scenarios: []string{"repr-001"}},
		{Area: "representation", Item: "Accept-Profile and Content-Profile", Label: FullMatch, Scenarios: []string{"repr-002"}},
		{Area: "representation", Item: "JSON and array Accept aliases", Label: FullMatch, Scenarios: []string{"repr-004"}},
		{Area: "representation", Item: "Singular object Accept", Label: FullMatch, Scenarios: []string{"repr-005"}},
		{Area: "representation", Item: "CSV Accept", Label: FullMatch, Scenarios: []string{"repr-006"}},

		{Area: "errors", Item: "Error envelope shape", Label: FullMatch, Scenarios: []string{"err-001"}},
		{Area: "errors", Item: "PGRST codes when the case fits", Label: FullMatch, Scenarios: []string{"err-002"}},
		{Area: "errors", Item: "myrest gap codes for MySQL gaps", Label: FullMatch, Scenarios: []string{"err-003"}},

		{Area: "transactions", Item: "Write and RPC request transactions under db-tx-end", Label: FullMatch, Scenarios: []string{"tx-001"}},

		{Area: "verification", Item: "Anonymous read smoke", Label: FullMatch, Scenarios: []string{"smoke-001"}},
		{Area: "verification", Item: "JWT read smoke", Label: FullMatch, Scenarios: []string{"smoke-002"}},
		{Area: "verification", Item: "POST RPC smoke", Label: FullMatch, Scenarios: []string{"smoke-004"}},
		{Area: "verification", Item: "Embed read smoke", Label: FullMatch, Scenarios: []string{"smoke-005"}},
	}
}
