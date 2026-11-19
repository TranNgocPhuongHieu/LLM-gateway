package channel

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/helper"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestDelayedUpstreamHeadersDoNotCommitSSE(t *testing.T) {
	const upstreamHeaderDelay = 126 * time.Second
	seenCancel := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTTP/1 detects a disconnected client in its background read only
		// after the request body reaches EOF.
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read mock request body: %v", err)
			return
		}
		select {
		case <-time.After(upstreamHeaderDelay):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
			close(seenCancel)
		}
	}))
	defer server.Close()

	// The mock is configured beyond the 125s boundary; the request context
	// cancels after two seconds so the suite does not sleep for two minutes.
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT", "1")
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT_HOSTS", "127.0.0.1")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"stream":true}`)).WithContext(requestCtx)
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI},
	}
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, server.URL, strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := firstResponseTimeout(upstreamReq, info); got != time.Second {
		t.Fatalf("test setup did not enable first-response watchdog: got %s", got)
	}

	resp, err := DoRequest(c, upstreamReq, info)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "first answer") {
		t.Fatalf("delayed headers returned wrong error: %v", err)
	}
	select {
	case <-seenCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("mock provider did not observe watchdog cancellation")
	}
	// The 1s watchdog fires before the first 15s heartbeat, preserving retry.
	if recorder.Flushed || recorder.Body.Len() != 0 || recorder.Result().Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("delayed upstream committed an SSE response before headers: flushed=%t body=%q content-type=%q", recorder.Flushed, recorder.Body.String(), recorder.Result().Header.Get("Content-Type"))
	}
}

func TestEarlyHeartbeatDelayedHeadersHandoff(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 5
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldPing := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = true
	t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT", "0")
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Errorf("read mock request body: %v", err)
					return
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"mock\":true}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			done := make(chan error, 1)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: format,
					RelayMode:   relayconstant.RelayModeChatCompletions,
					ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI}}
				if format == types.RelayFormatClaude {
					info.ApiType = constant.APITypeAnthropic
				}
				req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
				if err != nil {
					done <- err
					return
				}
				resp, err := DoRequest(c, req, info)
				if err != nil {
					done <- err
					return
				}
				helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
					if err := helper.StringData(c, data); err != nil {
						sr.Stop(err)
					}
				})
				helper.Done(c)
				done <- nil
			}))
			defer gateway.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL, strings.NewReader("{}"))
			started := time.Now()
			resp, err := gateway.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			reader := bufio.NewReader(resp.Body)
			line, err := reader.ReadString('\n')
			if err != nil || line != ": PING\n" || time.Since(started) < 14*time.Second || time.Since(started) > 22*time.Second {
				t.Fatalf("missing timely flushed heartbeat: line=%q err=%v elapsed=%s", line, err, time.Since(started))
			}
			close(release)
			body, err := io.ReadAll(reader)
			if err != nil || !strings.Contains(string(body), `data: {"mock":true}`) || strings.Count(string(body), "[DONE]") != 1 || strings.Contains(string(body), ": PING") {
				t.Fatalf("invalid heartbeat handoff: body=%q err=%v", body, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEarlyHeartbeatClientDisconnectBeforeHeaders(t *testing.T) {
	seenCancel := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read mock request body: %v", err)
			return
		}
		<-r.Context().Done()
		close(seenCancel)
	}))
	defer upstream.Close()
	defer upstream.CloseClientConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic}}
	// Deliberately no context on req: DoRequest must attach the client context.
	req, _ := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	resp, err := DoRequest(c, req, info)
	if resp != nil || err == nil {
		t.Fatalf("disconnect did not interrupt header wait: resp=%v err=%v", resp, err)
	}
	select {
	case <-seenCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("mock provider did not observe cancellation")
	}
}

func TestEarlyHeartbeatScope(t *testing.T) {
	for _, tc := range []struct {
		format types.RelayFormat
		mode   int
		api    int
		stream bool
		want   bool
	}{
		{types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, constant.APITypeOpenAI, true, true},
		{types.RelayFormatClaude, relayconstant.RelayModeUnknown, constant.APITypeAnthropic, true, true},
		{types.RelayFormatOpenAIResponses, relayconstant.RelayModeResponses, constant.APITypeAnthropic, true, true},
		{types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, constant.APITypeOpenAI, false, false},
		{types.RelayFormatOpenAI, relayconstant.RelayModeModerations, constant.APITypeOpenAI, true, false},
		{types.RelayFormatOpenAIResponsesCompaction, relayconstant.RelayModeResponsesCompact, constant.APITypeOpenAI, true, false},
		{types.RelayFormatGemini, relayconstant.RelayModeGemini, constant.APITypeOpenAI, true, false},
		{types.RelayFormatOpenAIAudio, relayconstant.RelayModeAudioSpeech, constant.APITypeOpenAI, true, false},
		{types.RelayFormatOpenAIImage, relayconstant.RelayModeImagesGenerations, constant.APITypeOpenAI, true, false},
		{types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, constant.APITypeGemini, true, false},
	} {
		info := &relaycommon.RelayInfo{IsStream: tc.stream, RelayFormat: tc.format, RelayMode: tc.mode,
			ChannelMeta: &relaycommon.ChannelMeta{ApiType: tc.api}}
		if got := earlyHeartbeatStream(info); got != tc.want {
			t.Errorf("format=%s mode=%d api=%d stream=%t: got %t want %t", tc.format, tc.mode, tc.api, tc.stream, got, tc.want)
		}
	}
}

func TestFirstResponseTimeoutExactHostAndMode(t *testing.T) {
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT", "15")
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT_HOSTS", "api.xpiki.com")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI}}
	for _, tc := range []struct {
		url  string
		want time.Duration
	}{
		{"https://api.xpiki.com/v1/chat/completions", 15 * time.Second},
		{"https://API.XPIKI.COM:443/v1/chat/completions", 15 * time.Second},
		{"https://api.xpiki.com.attacker.invalid/v1/chat/completions", 0},
		{"https://9router.trnhieu.dev/v1/chat/completions", 0},
	} {
		req, _ := http.NewRequest(http.MethodPost, tc.url, nil)
		if got := firstResponseTimeout(req, info); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.url, got, tc.want)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, "https://api.xpiki.com/v1/responses", nil)
	info.RelayMode = relayconstant.RelayModeResponses
	if firstResponseTimeout(req, info) != 15*time.Second {
		t.Fatal("Responses handler must use the scoped first-output watchdog")
	}
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.ApiType = constant.APITypeAnthropic
	if firstResponseTimeout(req, info) != 0 {
		t.Fatal("watchdog applied to a native Claude handler")
	}
	info.RelayMode = relayconstant.RelayModeUnknown
	info.RelayFormat = types.RelayFormatClaude
	info.ApiType = constant.APITypeOpenAI
	claudeReq, _ := http.NewRequest(http.MethodPost, "https://api.xpiki.com/v1/chat/completions", nil)
	if firstResponseTimeout(claudeReq, info) != 15*time.Second {
		t.Fatal("watchdog not applied to Claude format mapped to OpenAI channel")
	}
}

func TestFirstResponseWatchdogExpiresWithoutOutput(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.invalid", nil)
	info := &relaycommon.RelayInfo{}
	req, cleanup := withFirstResponseTimeout(req, info, time.Millisecond)
	defer cleanup()
	select {
	case <-req.Context().Done():
		if !errors.Is(context.Cause(req.Context()), relaycommon.ErrFirstResponseTimeout) {
			t.Fatal("lost timeout cause")
		}
	case <-time.After(time.Second):
		t.Fatal("first response watchdog did not cancel the attempt")
	}
}

func TestFirstResponseWatchdogStopsOnOutputAndCloses(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.invalid", nil)
	info := &relaycommon.RelayInfo{}
	req, cleanup := withFirstResponseTimeout(req, info, 50*time.Millisecond)
	defer cleanup()
	info.FirstResponseReady()
	info.FirstResponseReady()
	select {
	case <-req.Context().Done():
		t.Fatal("watchdog canceled a stream after its first answer")
	case <-time.After(100 * time.Millisecond):
	}
	body := &firstResponseBody{ReadCloser: io.NopCloser(strings.NewReader("test")), cleanup: cleanup}
	if err := body.Close(); err != nil || req.Context().Err() == nil {
		t.Fatal("body close did not release the attempt context")
	}
}

func TestFirstResponseTimeoutReasoningAndPrefill(t *testing.T) {
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT", "15")
	t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT_HOSTS", "api.xpiki.com")

	req, _ := http.NewRequest(http.MethodPost, "https://api.xpiki.com/v1/chat/completions", nil)

	// Case 1: Standard non-reasoning model with 0 tokens => base timeout 15s
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeChatCompletions,
		ChannelMeta:     &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI},
		OriginModelName: "gpt-4o",
	}
	if got := firstResponseTimeout(req, info); got != 15*time.Second {
		t.Fatalf("standard gpt-4o: got %s want 15s", got)
	}

	// Case 2: Reasoning model (gpt-6.1-sol) with 0 tokens => default reasoning timeout 180s (max(15*3, 180))
	info.OriginModelName = "tranhieu13102003/gpt-6.1-sol"
	if got := firstResponseTimeout(req, info); got != 180*time.Second {
		t.Fatalf("gpt-6.1-sol: got %s want 180s", got)
	}

	// Case 3: Reasoning model with 71,391 prompt tokens (as in production log) => 180s + 35s = 215s
	info.SetEstimatePromptTokens(71391)
	if got := firstResponseTimeout(req, info); got != 215*time.Second {
		t.Fatalf("gpt-6.1-sol with 71391 tokens: got %s want 215s", got)
	}

	// Case 4: Reasoning model with 113,936 prompt tokens (as in production log) => 180s + 56s = 236s
	info.SetEstimatePromptTokens(113936)
	if got := firstResponseTimeout(req, info); got != 236*time.Second {
		t.Fatalf("gpt-6.1-sol with 113936 tokens: got %s want 236s", got)
	}

	// Case 5: Standard non-reasoning model with large prompt (100,000 tokens) => 15s + 50s = 65s
	info.OriginModelName = "gpt-4o"
	info.SetEstimatePromptTokens(100000)
	if got := firstResponseTimeout(req, info); got != 65*time.Second {
		t.Fatalf("gpt-4o with 100k tokens: got %s want 65s", got)
	}

	// Case 6: Explicit RELAY_REASONING_FIRST_RESPONSE_TIMEOUT env var
	t.Setenv("RELAY_REASONING_FIRST_RESPONSE_TIMEOUT", "240")
	info.OriginModelName = "gpt-5.6-luna"
	info.SetEstimatePromptTokens(0)
	if got := firstResponseTimeout(req, info); got != 240*time.Second {
		t.Fatalf("gpt-5.6-luna with custom env 240s: got %s want 240s", got)
	}

	// Case 7: Reasoning models via other signals: o1-mini, deepseek-r1, qwq
	for _, m := range []string{"o1-mini", "deepseek-r1", "qwq-32b", "claude-3-7-sonnet-thinking"} {
		info.OriginModelName = m
		if got := firstResponseTimeout(req, info); got != 240*time.Second {
			t.Fatalf("model %s: got %s want 240s", m, got)
		}
	}

	// Case 8: Reasoning via ReasoningEffort
	info.OriginModelName = "custom-generic-model"
	info.ReasoningEffort = "high"
	if got := firstResponseTimeout(req, info); got != 240*time.Second {
		t.Fatalf("ReasoningEffort high: got %s want 240s", got)
	}
}
