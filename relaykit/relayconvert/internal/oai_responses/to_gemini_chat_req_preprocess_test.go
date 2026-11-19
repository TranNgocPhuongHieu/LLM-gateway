package oairesponses

import (
	"bytes"
	"testing"
)

func TestFilterGeminiResponsesToolsPreservesUnchangedInput(t *testing.T) {
	raw := []byte(`[
  {"type":"function","name":"lookup"}
]`)
	got, err := filterGeminiResponsesTools(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("unchanged tools were rewritten: got %q want %q", got, raw)
	}
}

func TestFilterGeminiResponsesToolsRemovesUnsupportedItems(t *testing.T) {
	got, err := filterGeminiResponsesTools([]byte(`[{"type":"function","name":"lookup"},{"type":"computer"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"name":"lookup","type":"function"}]` && string(got) != `[{"type":"function","name":"lookup"}]` {
		t.Fatalf("filtered tools = %s", got)
	}
}

func TestFilterGeminiResponsesInputKeepsUnrelatedFunctionOutput(t *testing.T) {
	raw := []byte(`[{"type":"function_call_output","call_id":"keep","output":"ok"}]`)
	got, err := filterGeminiResponsesInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("unrelated function output was rewritten: got %q want %q", got, raw)
	}
}

func TestFilterGeminiResponsesInputRemovesCustomToolPair(t *testing.T) {
	got, err := filterGeminiResponsesInput([]byte(`[{"type":"custom_tool_call","call_id":"drop"},{"type":"function_call_output","call_id":"drop","output":"x"},{"type":"function_call_output","call_id":"keep","output":"ok"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"call_id":"keep","output":"ok","type":"function_call_output"}]` && string(got) != `[{"type":"function_call_output","call_id":"keep","output":"ok"}]` {
		t.Fatalf("filtered input = %s", got)
	}
}
