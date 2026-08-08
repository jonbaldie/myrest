package config

// Prefer transaction-end values of the parity target (Prefer: tx=...).
const (
	PreferTxCommit   = "commit"
	PreferTxRollback = "rollback"
)

// DecideTxEnd chooses commit or rollback for one request transaction.
// applied is true only when a Prefer: tx= value changed the default end.
func DecideTxEnd(txEnd TxEnd, preferTx string) (commit bool, applied bool) {
	if txEnd == "" {
		txEnd = TxEndCommit
	}
	switch txEnd {
	case TxEndRollback:
		return false, false
	case TxEndCommitAllowOverride:
		return decideAllowOverride(true, preferTx)
	case TxEndRollbackAllowOverride:
		return decideAllowOverride(false, preferTx)
	default:
		// TxEndCommit and any unknown value: always commit; Prefer: tx= is off.
		return true, false
	}
}

func decideAllowOverride(defaultCommit bool, preferTx string) (commit bool, applied bool) {
	switch preferTx {
	case PreferTxCommit:
		return true, true
	case PreferTxRollback:
		return false, true
	default:
		return defaultCommit, false
	}
}
