package verification

// NormativeScenarios is the Verification scenario index.
// Capability chapters own the scenario bodies; this index holds id, area, and label.
func NormativeScenarios() ScenarioIndex {
	index := make(ScenarioIndex, 0, 80)
	index = append(index, authScenarios()...)
	index = append(index, cacheAndConfigScenarios()...)
	index = append(index, readScenarios()...)
	index = append(index, embedAndWriteScenarios()...)
	index = append(index, rpcScenarios()...)
	index = append(index, representationScenarios()...)
	index = append(index, errorDiscoveryTxScenarios()...)
	index = append(index, smokeScenarios()...)
	return index
}

func authScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "auth-001", Area: "auth", Label: FullMatch, Outcome: Success},
		{ID: "auth-002", Area: "auth", Label: FullMatch, Outcome: Success},
		{ID: "auth-003", Area: "auth", Label: FullMatch, Outcome: Refuse},
		{ID: "auth-004", Area: "auth", Label: PartialMatch, Outcome: Success},
		{ID: "auth-005", Area: "auth", Label: NotSupported, Outcome: Refuse},
		{ID: "auth-006", Area: "auth", Label: NotSupported, Outcome: Refuse},
		{ID: "auth-007", Area: "auth", Label: NotSupported, Outcome: Refuse},
		{ID: "auth-008", Area: "auth", Label: PartialMatch, Outcome: Refuse},
	}
}

func cacheAndConfigScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "cache-001", Area: "schema-cache", Label: FullMatch, Outcome: Success},
		{ID: "cache-002", Area: "schema-cache", Label: FullMatch, Outcome: Refuse},
		{ID: "cache-003", Area: "schema-cache", Label: FullMatch, Outcome: Success},
		{ID: "cache-004", Area: "schema-cache", Label: NotSupported, Outcome: Refuse},
		{ID: "cache-005", Area: "schema-cache", Label: NotSupported, Outcome: Refuse},
		{ID: "cfg-001", Area: "config", Label: FullMatch, Outcome: Refuse},
		{ID: "cfg-002", Area: "config", Label: FullMatch, Outcome: Success},
		{ID: "cfg-003", Area: "config", Label: NotSupported, Outcome: Refuse},
	}
}

func readScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "read-001", Area: "read", Label: FullMatch, Outcome: Success},
		{ID: "read-002", Area: "read", Label: FullMatch, Outcome: Success},
		{ID: "read-003", Area: "read", Label: PartialMatch, Outcome: Success},
		{ID: "read-004", Area: "read", Label: PartialMatch, Outcome: Refuse},
		{ID: "read-005", Area: "read", Label: PartialMatch, Outcome: Success},
		{ID: "read-006", Area: "read", Label: PartialMatch, Outcome: Refuse},
		{ID: "read-007", Area: "read", Label: NotSupported, Outcome: Refuse},
		{ID: "read-008", Area: "read", Label: NotSupported, Outcome: Refuse},
		{ID: "read-009", Area: "read", Label: NotSupported, Outcome: Refuse},
		{ID: "read-010", Area: "read", Label: FullMatch, Outcome: Success},
		{ID: "read-011", Area: "read", Label: FullMatch, Outcome: Refuse},
		{ID: "read-012", Area: "read", Label: FullMatch, Outcome: Success},
		{ID: "read-013", Area: "read", Label: NotSupported, Outcome: Refuse},
	}
}

func embedAndWriteScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "embed-001", Area: "embed", Label: FullMatch, Outcome: Success},
		{ID: "embed-002", Area: "embed", Label: FullMatch, Outcome: Success},
		{ID: "embed-003", Area: "embed", Label: NotSupported, Outcome: Refuse},
		{ID: "embed-004", Area: "embed", Label: NotSupported, Outcome: Refuse},
		{ID: "embed-005", Area: "embed", Label: FullMatch, Outcome: Success},
		{ID: "write-001", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-002", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-003", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-004", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-005", Area: "write", Label: FullMatch, Outcome: Refuse},
		{ID: "write-006", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-007", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-008", Area: "write", Label: PartialMatch, Outcome: Success},
		{ID: "write-009", Area: "write", Label: PartialMatch, Outcome: Refuse},
		{ID: "write-010", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-011", Area: "write", Label: FullMatch, Outcome: Success},
		{ID: "write-012", Area: "write", Label: NotSupported, Outcome: Refuse},
		{ID: "write-013", Area: "write", Label: PartialMatch, Outcome: Success},
		{ID: "write-014", Area: "write", Label: PartialMatch, Outcome: Refuse},
	}
}

func rpcScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "rpc-001", Area: "rpc", Label: FullMatch, Outcome: Success},
		{ID: "rpc-002", Area: "rpc", Label: FullMatch, Outcome: Success},
		{ID: "rpc-003", Area: "rpc", Label: PartialMatch, Outcome: Success},
		{ID: "rpc-004", Area: "rpc", Label: PartialMatch, Outcome: Refuse},
		{ID: "rpc-005", Area: "rpc", Label: FullMatch, Outcome: Success},
		{ID: "rpc-006", Area: "rpc", Label: NotSupported, Outcome: Refuse},
		{ID: "rpc-007", Area: "rpc", Label: NotSupported, Outcome: Refuse},
		{ID: "rpc-008", Area: "rpc", Label: NotSupported, Outcome: Refuse},
		{ID: "rpc-009", Area: "rpc", Label: NotSupported, Outcome: Refuse},
		{ID: "rpc-010", Area: "rpc", Label: NotSupported, Outcome: Refuse},
	}
}

func representationScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "repr-001", Area: "representation", Label: FullMatch, Outcome: Success},
		{ID: "repr-002", Area: "representation", Label: FullMatch, Outcome: Success},
		{ID: "repr-003", Area: "representation", Label: NotSupported, Outcome: Refuse},
		{ID: "repr-004", Area: "representation", Label: FullMatch, Outcome: Success},
		{ID: "repr-005", Area: "representation", Label: FullMatch, Outcome: Success},
		{ID: "repr-006", Area: "representation", Label: FullMatch, Outcome: Success},
		{ID: "repr-007", Area: "representation", Label: NotSupported, Outcome: Refuse},
		{ID: "repr-008", Area: "representation", Label: FullMatch, Outcome: Refuse},
		{ID: "repr-009", Area: "representation", Label: NotSupported, Outcome: Refuse},
		{ID: "repr-010", Area: "representation", Label: NotSupported, Outcome: Refuse},
		{ID: "prefer-001", Area: "representation", Label: NotSupported, Outcome: Refuse},
	}
}

func errorDiscoveryTxScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "err-001", Area: "errors", Label: FullMatch, Outcome: Refuse},
		{ID: "err-002", Area: "errors", Label: FullMatch, Outcome: Refuse},
		{ID: "err-003", Area: "errors", Label: FullMatch, Outcome: Refuse},
		{ID: "err-004", Area: "errors", Label: PartialMatch, Outcome: Success},
		{ID: "err-005", Area: "errors", Label: PartialMatch, Outcome: Refuse},
		{ID: "discovery-001", Area: "discovery", Label: PartialMatch, Outcome: Success},
		{ID: "discovery-002", Area: "discovery", Label: PartialMatch, Outcome: Success},
		{ID: "discovery-003", Area: "discovery", Label: PartialMatch, Outcome: Success},
		{ID: "discovery-004", Area: "discovery", Label: NotSupported, Outcome: Refuse},
		{ID: "discovery-005", Area: "discovery", Label: PartialMatch, Outcome: Refuse},
		{ID: "discovery-006", Area: "discovery", Label: PartialMatch, Outcome: Refuse},
		{ID: "discovery-007", Area: "discovery", Label: PartialMatch, Outcome: Refuse},
		{ID: "discovery-008", Area: "discovery", Label: FullMatch, Outcome: Refuse},
		{ID: "discovery-009", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-010", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-011", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-012", Area: "discovery", Label: FullMatch, Outcome: Refuse},
		{ID: "discovery-013", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-014", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-015", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-016", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-017", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-018", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "discovery-019", Area: "discovery", Label: FullMatch, Outcome: Success},
		{ID: "cors-001", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "cors-002", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "cors-003", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "cors-004", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "cors-005", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "cors-006", Area: "cors-proxy", Label: FullMatch, Outcome: Success},
		{ID: "tx-001", Area: "transactions", Label: FullMatch, Outcome: Success},
		{ID: "tx-002", Area: "transactions", Label: NotSupported, Outcome: Refuse},
		{ID: "tx-003", Area: "transactions", Label: NotSupported, Outcome: Refuse},
	}
}

func smokeScenarios() ScenarioIndex {
	return ScenarioIndex{
		{ID: "smoke-001", Area: "verification", Label: FullMatch, Outcome: Success},
		{ID: "smoke-002", Area: "verification", Label: FullMatch, Outcome: Success},
		{ID: "smoke-003", Area: "verification", Label: PartialMatch, Outcome: Success},
		{ID: "smoke-004", Area: "verification", Label: FullMatch, Outcome: Success},
		{ID: "smoke-005", Area: "verification", Label: FullMatch, Outcome: Success},
		{ID: "smoke-006", Area: "verification", Label: NotSupported, Outcome: Refuse},
	}
}
