package claude

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const claudeMockFrames = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_mock\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"mock\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n" +
	"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
	"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
	"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
	"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"

const claudeMockStop = "data: {\"type\":\"message_stop\"}\n\n"

type claudeMockReadError struct{}

func (claudeMockReadError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestClaudeStreamTerminalValidation(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldPing := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = false
	t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		for _, scenario := range []string{"complete", "truncated", "empty", "error", "scanner_error", "timeout", "disconnect"} {
			t.Run(string(format)+"/"+scenario, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
				info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: format,
					ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic, UpstreamModelName: "mock"}}
				var body io.ReadCloser = io.NopCloser(strings.NewReader(claudeMockFrames + claudeMockStop))
				switch scenario {
				case "truncated":
					body = io.NopCloser(strings.NewReader(claudeMockFrames))
				case "empty":
					body = io.NopCloser(strings.NewReader(""))
				case "error":
					body = io.NopCloser(strings.NewReader(claudeMockFrames + "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"mock failure\"}}\n\n"))
				case "scanner_error":
					body = io.NopCloser(io.MultiReader(strings.NewReader(claudeMockFrames), claudeMockReadError{}))
				case "timeout", "disconnect":
					reader, writer := io.Pipe()
					defer writer.Close()
					body = reader
					if scenario == "disconnect" {
						timer := time.AfterFunc(20*time.Millisecond, cancel)
						defer timer.Stop()
					}
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
				var usage *dto.Usage
				var streamErr *types.NewAPIError
				if format == types.RelayFormatOpenAIResponses {
					usage, streamErr = ClaudeResponsesStreamHandler(c, resp, info)
				} else {
					usage, streamErr = ClaudeStreamHandler(c, resp, info)
				}
				if scenario == "complete" {
					if streamErr != nil || usage == nil || usage.PromptTokens != 10 || usage.CompletionTokens != 2 {
						t.Fatalf("valid stream changed usage: usage=%+v err=%v", usage, streamErr)
					}
					return
				}
				if streamErr == nil || usage != nil {
					t.Fatalf("failed stream allowed successful settlement: usage=%+v err=%v", usage, streamErr)
				}
				if strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), `"type":"response.completed"`) {
					t.Fatalf("failed stream fabricated success: %s", w.Body.String())
				}
				if scenario == "disconnect" && !types.IsSkipRetryError(streamErr) {
					t.Fatal("disconnect must prohibit replay")
				}
			})
		}
	}
}
