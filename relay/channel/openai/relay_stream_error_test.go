package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func streamErrorTestContext(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 2
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldPing := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = false
	t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream: true, RelayMode: relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"},
	}
	return c, info, w
}

func testStreamResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
}

const roleFrame = "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n"
const contentFrame = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n"
const usageFrame = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1,\"total_tokens\":11}}\n\n"
const errorFrame = "data: {\"error\":{\"type\":\"upstream_error\",\"code\":\"upstream_error\",\"message\":\"provider interrupted\"}}\n\n"

func TestOaiStreamErrorBeforeOutput(t *testing.T) {
	for _, prefix := range []string{"", roleFrame, roleFrame + usageFrame} {
		t.Run(prefix, func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			usage, err := OaiStreamHandler(c, info, testStreamResponse(prefix+errorFrame+"data: [DONE]\n\n"))
			if err == nil || err.StatusCode != 502 || types.IsSkipRetryError(err) || usage != nil {
				t.Fatalf("expected retryable 502 without settlement, usage=%v err=%v", usage, err)
			}
			if strings.Contains(w.Body.String(), "provider interrupted") || strings.Contains(w.Body.String(), "[DONE]") || w.Body.Len() != 0 {
				t.Fatalf("failed attempt leaked to client: %q", w.Body.String())
			}
		})
	}
}

func TestOaiStreamErrorAfterOutputDoesNotReplay(t *testing.T) {
	c, info, w := streamErrorTestContext(t)
	_, err := OaiStreamHandler(c, info, testStreamResponse(roleFrame+contentFrame+errorFrame+"data: [DONE]\n\n"))
	if err == nil || !types.IsSkipRetryError(err) {
		t.Fatalf("partial output must stop retries: %v", err)
	}
	if strings.Count(w.Body.String(), "hello") != 1 || strings.Contains(w.Body.String(), "[DONE]") || strings.Contains(w.Body.String(), "provider interrupted") {
		t.Fatalf("incorrect partial stream: %q", w.Body.String())
	}
}

func TestOaiStreamEmptyAndTruncatedFail(t *testing.T) {
	for _, body := range []string{"", roleFrame, roleFrame + usageFrame + "data: [DONE]\n\n", contentFrame} {
		t.Run(body, func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			_, err := OaiStreamHandler(c, info, testStreamResponse(body))
			if err == nil || strings.Contains(w.Body.String(), "[DONE]") {
				t.Fatalf("empty/truncated stream reported success: err=%v body=%q", err, w.Body.String())
			}
		})
	}
}

func TestOaiStreamValidEmptyAndToolsRemainSuccessful(t *testing.T) {
	for _, frame := range []string{
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_test","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}`,
	} {
		t.Run(frame, func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			usage, err := OaiStreamHandler(c, info, testStreamResponse(roleFrame+"data: "+frame+"\n\n"+usageFrame+"data: [DONE]\n\n"))
			if err != nil || usage == nil || strings.Count(w.Body.String(), "[DONE]") != 1 {
				t.Fatalf("valid stream rejected: usage=%v err=%v body=%q", usage, err, w.Body.String())
			}
		})
	}
}

func TestOaiStreamCanceledClientDoesNotRetry(t *testing.T) {
	c, info, _ := streamErrorTestContext(t)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	_, err := OaiStreamHandler(c, info, testStreamResponse(roleFrame))
	if err == nil || !types.IsSkipRetryError(err) {
		t.Fatalf("canceled client should not retry: %v", err)
	}
}

func TestChatStreamErrorStatusAndShapes(t *testing.T) {
	for _, tc := range []struct {
		data   string
		status int
	}{
		{`{"error":{"message":"invalid input","type":"invalid_request_error"}}`, 400},
		{`{"error":{"message":"limited","type":"rate_limit_error"}}`, 429},
		{`{"error":{"message":"limited","code":"provider_account_unavailable"}}`, 429},
		{`{"type":"error","code":"invalid_prompt","message":"invalid prompt"}`, 400},
		{`{"error":{"message":"invalid","type":"invalid_request_error","status":502}}`, 502},
		{`{"type":"error","message":"interrupted","code":503}`, 503},
		{`{"error":"interrupted"}`, 502},
		{`{"choices":[],"error":null}`, 0},
		{`not json`, 502},
	} {
		err := chatStreamChunkError(tc.data)
		if tc.status == 0 {
			if err != nil {
				t.Fatalf("normal event rejected: %v", err)
			}
		} else if err == nil || err.StatusCode != tc.status {
			t.Fatalf("%s: expected %d got %v", tc.data, tc.status, err)
		}
	}
}

func TestOpenAIHTTP200ErrorEnvelopeIsNotReportedAsSuccess(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"error":{"type":"upstream_error","message":"provider failed"}}`, 502},
		{`{"error":{"type":"upstream_error","code":"provider_account_unavailable","message":"account unavailable"}}`, 429},
		{`{"error":{"type":"invalid_request_error","message":"invalid input"}}`, 400},
		{`{"error":{"type":"upstream_error","status_code":504,"message":"gateway timeout"}}`, 504},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			info.IsStream = false
			resp := testStreamResponse(tc.body)
			resp.Header.Set("Content-Type", "application/json")
			usage, err := OpenaiHandler(c, info, resp)
			if err == nil || err.StatusCode != tc.status || usage != nil || w.Body.Len() != 0 {
				t.Fatalf("HTTP-200 error was delivered/settled as success: status=%d usage=%v err=%v body=%q", tc.status, usage, err, w.Body.String())
			}
		})
	}
}

const responsesCreated = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"status\":\"in_progress\"}}\n\n"
const responsesDelta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
const responsesCompleted = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n"

func TestResponsesFailuresBeforeOutputAreRetryable(t *testing.T) {
	for _, event := range []string{
		`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"provider failed"}}}`,
		`{"type":"response.failed","response":{"status":"failed"}}`,
		`{"type":"error","code":"server_error","message":"provider failed"}`,
		`{"type":"response.cancelled","response":{"status":"cancelled"}}`,
		`{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"upstream_interrupted"}}}`,
		`not json`,
		`{}`,
	} {
		t.Run(event, func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			info.RelayMode = relayconstant.RelayModeResponses
			ready := 0
			info.FirstResponseReady = func() { ready++ }
			usage, err := OaiResponsesStreamHandler(c, info, testStreamResponse(responsesCreated+"data: "+event+"\n\n"))
			if err == nil || err.StatusCode != 502 || types.IsSkipRetryError(err) || usage != nil || w.Body.Len() != 0 || ready != 0 {
				t.Fatalf("failed Responses attempt leaked or settled: usage=%v err=%v body=%q", usage, err, w.Body.String())
			}
		})
	}
}

func TestResponsesTruncatedAndPartialStreamsDoNotReportSuccess(t *testing.T) {
	for _, body := range []string{"", responsesCreated, responsesCreated + responsesDelta, responsesDelta + errorFrame} {
		t.Run(body, func(t *testing.T) {
			c, info, w := streamErrorTestContext(t)
			info.RelayMode = relayconstant.RelayModeResponses
			usage, err := OaiResponsesStreamHandler(c, info, testStreamResponse(body))
			partial := strings.Contains(body, responsesDelta)
			if err == nil || usage != nil || types.IsSkipRetryError(err) != partial || strings.Contains(w.Body.String(), "response.completed") {
				t.Fatalf("truncated stream treated as success/replayed: usage=%v err=%v body=%q", usage, err, w.Body.String())
			}
		})
	}
}

func TestResponsesCompletedAndTokenLimitRemainValid(t *testing.T) {
	for _, terminal := range []string{responsesCompleted, "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n"} {
		c, info, w := streamErrorTestContext(t)
		info.RelayMode = relayconstant.RelayModeResponses
		ready := 0
		info.FirstResponseReady = func() { ready++ }
		usage, err := OaiResponsesStreamHandler(c, info, testStreamResponse(responsesCreated+responsesDelta+terminal))
		if err != nil || usage == nil || usage.PromptTokens != 10 || usage.CompletionTokens != 1 || ready == 0 || strings.Count(w.Body.String(), "hello") != 1 {
			t.Fatalf("valid Responses stream rejected: usage=%v err=%v ready=%d body=%q", usage, err, ready, w.Body.String())
		}
	}
}

func TestResponsesHTTP200ErrorsAreNotSuccess(t *testing.T) {
	for _, body := range []string{`{"error":{"code":"server_error","message":"provider failed"}}`, `{"status":"failed"}`} {
		c, info, w := streamErrorTestContext(t)
		usage, err := OaiResponsesHandler(c, info, testStreamResponse(body))
		if err == nil || err.StatusCode != 502 || usage != nil || w.Body.Len() != 0 {
			t.Fatalf("HTTP-200 Responses error reported success: usage=%v err=%v body=%q", usage, err, w.Body.String())
		}
	}
}

func TestResponsesCancellationAndWatchdogSafety(t *testing.T) {
	for _, clientCanceled := range []bool{false, true} {
		c, info, w := streamErrorTestContext(t)
		info.RelayMode = relayconstant.RelayModeResponses
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(relaycommon.ErrFirstResponseTimeout)
		info.FirstResponseContext = ctx
		if clientCanceled {
			clientCtx, cancelClient := context.WithCancel(c.Request.Context())
			cancelClient()
			c.Request = c.Request.WithContext(clientCtx)
		}
		usage, err := OaiResponsesStreamHandler(c, info, testStreamResponse(responsesCreated))
		if err == nil || usage != nil || types.IsSkipRetryError(err) != clientCanceled || w.Body.Len() != 0 {
			t.Fatalf("wrong cancellation behavior: clientGone=%v usage=%v err=%v body=%q", clientCanceled, usage, err, w.Body.String())
		}
		if !clientCanceled && err.StatusCode != 503 {
			t.Fatalf("upstream watchdog must be retryable: %v", err)
		}
	}
}

func TestOaiStreamIncompleteChoicesFails(t *testing.T) {
	c, info, _ := streamErrorTestContext(t)
	// Choice 0 finishes with stop, Choice 1 is still generating and doesn't finish before [DONE]
	body := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"A\"}},{\"index\":1,\"delta\":{\"content\":\"B\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	_, err := OaiStreamHandler(c, info, testStreamResponse(body))
	if err == nil {
		t.Fatalf("expected error for incomplete choices, got nil")
	}
	if info.StreamStatus.TerminalEvidence != "incomplete_choices:1/2" {
		t.Fatalf("expected terminal evidence incomplete_choices:1/2, got %q", info.StreamStatus.TerminalEvidence)
	}
}

func TestOaiStreamAllChoicesCompleteSucceeds(t *testing.T) {
	c, info, _ := streamErrorTestContext(t)
	body := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"A\"}},{\"index\":1,\"delta\":{\"content\":\"B\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[{\"index\":1,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	_, err := OaiStreamHandler(c, info, testStreamResponse(body))
	if err != nil {
		t.Fatalf("expected success for all choices completed, got %v", err)
	}
	if info.StreamStatus.TerminalEvidence != "finish_reason:stop" {
		t.Fatalf("expected terminal evidence finish_reason:stop, got %q", info.StreamStatus.TerminalEvidence)
	}
}

func TestOaiStreamRequestNChoicesMissingUpstreamChoiceFails(t *testing.T) {
	c, info, _ := streamErrorTestContext(t)
	n := 2
	info.Request = &dto.GeneralOpenAIRequest{
		N: &n,
	}
	// Upstream only sends choice 0 completed with stop, never sends choice 1, then sends [DONE] and EOF
	body := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"A\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	_, err := OaiStreamHandler(c, info, testStreamResponse(body))
	if err == nil {
		t.Fatalf("expected error for missing choice 1 when n=2, got nil")
	}
	if info.StreamStatus.TerminalEvidence != "incomplete_choices:1/2" {
		t.Fatalf("expected terminal evidence incomplete_choices:1/2, got %q", info.StreamStatus.TerminalEvidence)
	}
}

func TestOaiStreamRequestNChoicesAllChoicesCompleteSucceeds(t *testing.T) {
	c, info, _ := streamErrorTestContext(t)
	n := 2
	info.Request = &dto.GeneralOpenAIRequest{
		N: &n,
	}
	body := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"A\"}},{\"index\":1,\"delta\":{\"content\":\"B\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[{\"index\":1,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	_, err := OaiStreamHandler(c, info, testStreamResponse(body))
	if err != nil {
		t.Fatalf("expected success when all n=2 choices complete, got %v", err)
	}
	if info.StreamStatus.TerminalEvidence != "finish_reason:stop" {
		t.Fatalf("expected terminal evidence finish_reason:stop, got %q", info.StreamStatus.TerminalEvidence)
	}
}


