package config_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestDecideTxEnd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		txEnd      config.TxEnd
		preferTx   string
		wantCommit bool
		wantApply  bool
	}{
		{name: "commit default", txEnd: config.TxEndCommit, wantCommit: true},
		{name: "commit ignores prefer rollback", txEnd: config.TxEndCommit, preferTx: "rollback", wantCommit: true},
		{name: "rollback default", txEnd: config.TxEndRollback, wantCommit: false},
		{name: "rollback ignores prefer commit", txEnd: config.TxEndRollback, preferTx: "commit", wantCommit: false},
		{
			name: "commit-allow-override default",
			txEnd: config.TxEndCommitAllowOverride, wantCommit: true,
		},
		{
			name: "commit-allow-override prefer rollback",
			txEnd: config.TxEndCommitAllowOverride, preferTx: "rollback",
			wantCommit: false, wantApply: true,
		},
		{
			name: "commit-allow-override prefer commit",
			txEnd: config.TxEndCommitAllowOverride, preferTx: "commit",
			wantCommit: true, wantApply: true,
		},
		{
			name: "rollback-allow-override default",
			txEnd: config.TxEndRollbackAllowOverride, wantCommit: false,
		},
		{
			name: "rollback-allow-override prefer commit",
			txEnd: config.TxEndRollbackAllowOverride, preferTx: "commit",
			wantCommit: true, wantApply: true,
		},
		{
			name: "rollback-allow-override prefer rollback",
			txEnd: config.TxEndRollbackAllowOverride, preferTx: "rollback",
			wantCommit: false, wantApply: true,
		},
		{name: "empty knob defaults to commit", wantCommit: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			commit, applied := config.DecideTxEnd(tc.txEnd, tc.preferTx)
			if commit != tc.wantCommit || applied != tc.wantApply {
				t.Fatalf(
					"DecideTxEnd(%q, %q) = (%v, %v), want (%v, %v)",
					tc.txEnd, tc.preferTx, commit, applied, tc.wantCommit, tc.wantApply,
				)
			}
		})
	}
}

func TestAllowsTxPrefer(t *testing.T) {
	t.Parallel()

	if config.AllowsTxPrefer(config.TxEndCommit) {
		t.Fatal("commit must not allow Prefer: tx=")
	}
	if !config.AllowsTxPrefer(config.TxEndCommitAllowOverride) {
		t.Fatal("commit-allow-override must allow Prefer: tx=")
	}
	if !config.AllowsTxPrefer(config.TxEndRollbackAllowOverride) {
		t.Fatal("rollback-allow-override must allow Prefer: tx=")
	}
}
