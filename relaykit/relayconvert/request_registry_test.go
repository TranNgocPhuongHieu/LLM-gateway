package relayconvert

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

func TestConvertOpenAIResponsesToGeminiPreparesOnce(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model: "gemini-test",
		Input: json.RawMessage(`[{"type":"input_text","text":"hello"}]`),
		Tools: json.RawMessage(`[{"type":"function","name":"lookup"}]`),
	}
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, request)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Value == nil {
		t.Fatal("conversion returned no value")
	}
	if len(result.Steps) != 1 || result.Steps[0].To != types.RelayFormatGemini {
		t.Fatalf("conversion steps = %#v", result.Steps)
	}
}
