package relayconvert

import (
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

func TestResponseStreamStateDiagnosticsCursor(t *testing.T) {
	state := &ResponseStreamState{}
	first := types.ConversionDiagnostic{Code: "first", Path: "input"}
	second := types.ConversionDiagnostic{Code: "second", Path: "output"}

	state.rememberDiagnostics([]types.ConversionDiagnostic{first, first})
	got := state.TakeNewDiagnostics()
	if len(got) != 1 || got[0] != first {
		t.Fatalf("first diagnostics batch = %#v", got)
	}
	if got := state.TakeNewDiagnostics(); len(got) != 0 {
		t.Fatalf("repeated diagnostics read = %#v", got)
	}

	state.rememberDiagnostics([]types.ConversionDiagnostic{first, second})
	got = state.TakeNewDiagnostics()
	if len(got) != 1 || got[0] != second {
		t.Fatalf("second diagnostics batch = %#v", got)
	}

	all := state.Diagnostics()
	if len(all) != 2 || all[0] != first || all[1] != second {
		t.Fatalf("cumulative diagnostics = %#v", all)
	}
}
