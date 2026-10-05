package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/prefer"
)

func TestNewWritePreferTxValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		header    string
		txEnd     config.TxEnd
		wantTx    string
		wantApply string
	}{
		{
			name:      "rollback under allow-override",
			header:    "tx=rollback, return=representation",
			txEnd:     config.TxEndCommitAllowOverride,
			wantTx:    "rollback",
			wantApply: "return=representation, tx=rollback",
		},
		{
			name:      "commit under rollback-allow-override",
			header:    "tx=commit",
			txEnd:     config.TxEndRollbackAllowOverride,
			wantTx:    "commit",
			wantApply: "tx=commit",
		},
		{
			name:   "rollback ignored when override off",
			header: "tx=rollback",
			txEnd:  config.TxEndCommit,
			wantTx: "rollback",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			written := newWritePrefer(prefer.Parse([]string{tc.header}), tc.txEnd, writeKindPatch)
			if written.Tx != tc.wantTx {
				t.Fatalf("Tx = %q, want %q", written.Tx, tc.wantTx)
			}
			got := strings.Join(written.applied, ", ")
			if got != tc.wantApply {
				t.Fatalf("applied = %q, want %q", got, tc.wantApply)
			}
		})
	}
}

// Preference-Applied names only the preferences the write kind applied.
func TestNewWritePreferApplied(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		kind   writeKind
		want   string
	}{
		{name: "missing default on insert", header: "missing=default", kind: writeKindInsert, want: "missing=default"},
		{name: "missing default on patch", header: "missing=default", kind: writeKindPatch},
		{name: "missing default on delete", header: "missing=default", kind: writeKindDelete},
		{name: "missing default on put", header: "missing=default", kind: writeKindPut},
		{name: "invalid return is not applied", header: "return=bogus", kind: writeKindPatch},
		{name: "default return is not applied", header: "", kind: writeKindPatch},
		{
			name:   "strict max-affected on patch",
			header: "handling=strict, max-affected=2",
			kind:   writeKindPatch,
			want:   "handling=strict, max-affected=2",
		},
		{
			name:   "strict max-affected on insert",
			header: "handling=strict, max-affected=2",
			kind:   writeKindInsert,
			want:   "handling=strict",
		},
		{name: "lenient max-affected", header: "max-affected=2", kind: writeKindDelete},
		{name: "count is not applied", header: "count=exact", kind: writeKindPatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			written := newWritePrefer(prefer.Parse([]string{tc.header}), config.TxEndCommit, tc.kind)
			if got := strings.Join(written.applied, ", "); got != tc.want {
				t.Fatalf("applied = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewWritePreferDefaultsReturnToMinimal(t *testing.T) {
	t.Parallel()

	written := newWritePrefer(prefer.Parse(nil), config.TxEndCommit, writeKindInsert)
	if written.Return != returnMinimal {
		t.Fatalf("Return = %q, want %q", written.Return, returnMinimal)
	}
}

func TestSetTxPreferenceApplied(t *testing.T) {
	t.Parallel()

	writer := httptest.NewRecorder()
	setTxPreferenceApplied(writer, "rollback", config.TxEndCommit)
	if got := writer.Header().Get("Preference-Applied"); got != "" {
		t.Fatalf("Preference-Applied = %q, want empty when override is off", got)
	}

	writer = httptest.NewRecorder()
	setTxPreferenceApplied(writer, "rollback", config.TxEndCommitAllowOverride)
	if got := writer.Header().Get("Preference-Applied"); got != "tx=rollback" {
		t.Fatalf("Preference-Applied = %q, want tx=rollback", got)
	}
}

func TestSetPreferenceAppliedJoinsTokens(t *testing.T) {
	t.Parallel()

	writer := httptest.NewRecorder()
	setPreferenceApplied(writer, writePrefer{
		applied: []string{"return=representation", "tx=rollback"},
	})
	if got := writer.Header().Get("Preference-Applied"); got != "return=representation, tx=rollback" {
		t.Fatalf("Preference-Applied = %q", got)
	}

	empty := httptest.NewRecorder()
	setPreferenceApplied(empty, writePrefer{})
	if got := empty.Header().Get("Preference-Applied"); got != "" {
		t.Fatalf("empty applied set header %q", got)
	}
}
