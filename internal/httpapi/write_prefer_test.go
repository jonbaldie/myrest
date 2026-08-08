package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestParseWritePreferTxValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		header    string
		txEnd     config.TxEnd
		wantTx    string
		wantApply string
		wantErr   bool
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
		{
			name:    "invalid tx under strict",
			header:  "handling=strict, tx=sideways",
			txEnd:   config.TxEndCommitAllowOverride,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prefer, err := parseWritePrefer([]string{tc.header}, tc.txEnd)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected invalid prefer error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWritePrefer: %v", err)
			}
			if prefer.Tx != tc.wantTx {
				t.Fatalf("Tx = %q, want %q", prefer.Tx, tc.wantTx)
			}
			got := strings.Join(prefer.applied, ", ")
			if got != tc.wantApply {
				t.Fatalf("applied = %q, want %q", got, tc.wantApply)
			}
		})
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
