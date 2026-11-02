package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	relaychannel "github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/channel/openai"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/helper"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestStreamErrorsReachFallbackWithoutLeaking(t *testing.T) {
	c, info, param := setupRelayRetryCandidates(t)
	w := httptest.NewRecorder()
	recordedContext, _ := gin.CreateTestContext(w)
	recordedContext.Keys, recordedContext.Request = c.Keys, c.Request
	c, param.Ctx = recordedContext, recordedContext
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 2
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldPing := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = false
	t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
	info.IsStream, info.RelayMode, info.RelayFormat = true, relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI
	// Heartbeats may already have committed HTTP 200; they must not prevent retry.
	helper.SetEventStreamHeaders(c)
	if err := helper.PingData(c); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		param.SetRetry(attempt)
		channel, selectionErr := getChannel(c, info, param)
		if selectionErr != nil {
			t.Fatal(selectionErr)
		}
		param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, channel.Id)
		info.InitChannelMeta(c)
		body := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: {\"error\":{\"type\":\"upstream_error\",\"message\":\"test upstream failed\"}}\n\ndata: [DONE]\n\n"
		if channel.Id == 10 {
			body = "data: {\"choices\":[{\"delta\":{\"content\":\"fallback OK\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n"
		}
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
		_, upstreamErr := openai.OaiStreamHandler(c, info, resp)
		if attempt < 4 {
			if upstreamErr == nil || !shouldRetry(c, upstreamErr, common.RetryTimes-attempt) {
				t.Fatalf("primary %d did not fall back: %v", channel.Id, upstreamErr)
			}
			info.LastError = upstreamErr
		} else if channel.Id != 10 || upstreamErr != nil {
			t.Fatalf("fallback failed: channel=%d err=%v", channel.Id, upstreamErr)
		}
	}
	if strings.Contains(w.Body.String(), "test upstream failed") || strings.Count(w.Body.String(), "fallback OK") != 1 || strings.Count(w.Body.String(), "[DONE]") != 1 {
		t.Fatalf("incorrect client response after retries: %q", w.Body.String())
	}
}

func TestRelayTerminalErrorUsesCorrectFraming(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatOpenAIResponses} {
		for _, committed := range []bool{false, true} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			helper.SetEventStreamHeaders(c)
			if committed {
				_ = helper.PingData(c)
			}
			writeRelayErrorResponse(c, format, types.NewOpenAIError(errors.New("test failure"), types.ErrorCodeBadResponse, 502))
			if strings.Contains(w.Body.String(), "[DONE]") {
				t.Fatal("terminal error fabricated success")
			}
			if committed {
				if w.Code != 200 || !strings.Contains(w.Body.String(), "data: {\"error\"") && format == types.RelayFormatOpenAI {
					t.Fatalf("not an SSE error: code=%d body=%q", w.Code, w.Body.String())
				}
				if format == types.RelayFormatClaude && !strings.Contains(w.Body.String(), "event: error\n") {
					t.Fatal("missing Claude error event")
				}
				if format == types.RelayFormatOpenAIResponses && (!strings.Contains(w.Body.String(), "event: error\n") || !strings.Contains(w.Body.String(), `"type":"error"`) || !strings.Contains(w.Body.String(), `"message":`)) {
					t.Fatalf("invalid Responses error event: %q", w.Body.String())
				}
			} else if w.Code != 502 || c.Writer.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("uncommitted error has wrong HTTP response: %d %s", w.Code, c.Writer.Header().Get("Content-Type"))
			}
		}
	}
}

func TestRelayCanceledRequestDoesNotRetry(t *testing.T) {
	c, _, _ := setupRelayRetryCandidates(t)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	if shouldRetry(c, types.NewOpenAIError(errors.New("interrupted"), types.ErrorCodeBadResponse, 502), 5) {
		t.Fatal("retried after the client left")
	}
}

func TestRelayPartialStreamStopOverridesChannelError(t *testing.T) {
	c, _, _ := setupRelayRetryCandidates(t)
	err := types.NewOpenAIError(errors.New("partial stream interrupted"), types.ErrorCodeChannelResponseTimeExceeded, 502, types.ErrOptionWithSkipRetry())
	if shouldRetry(c, err, 5) {
		t.Fatal("channel error overrode the no-replay guard")
	}
}

func TestFirstResponseTimeoutStillSelects9routerWithConnectedClient(t *testing.T) {
	c, info, param := setupRelayRetryCandidates(t)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 2
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	t.Setenv("RELAY_MAX_HOST_ATTEMPTS", "1")
	if err := model.DB.Model(&model.Channel{}).Where("id IN ?", []int{8, 9, 11, 12}).Update("base_url", "https://api.xpiki.com").Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", 10).Update("base_url", "http://9router:20128").Error; err != nil {
		t.Fatal(err)
	}
	model.InitChannelCache()
	primary, err := getChannel(c, info, param)
	if err != nil || primary.Id == 10 {
		t.Fatal("initial channel was not XPiki")
	}
	param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, primary.Id)
	info.InitChannelMeta(c)
	info.IsStream, info.RelayMode, info.RelayFormat = true, relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(relaycommon.ErrFirstResponseTimeout)
	info.FirstResponseContext = ctx
	settings := operation_setting.GetGeneralSetting()
	oldPing := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = false
	t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))}
	_, timeoutErr := openai.OaiStreamHandler(c, info, resp)
	if timeoutErr == nil || timeoutErr.StatusCode != 503 || !shouldRetry(c, timeoutErr, 5) || c.Request.Context().Err() != nil {
		t.Fatalf("upstream timeout blocked fallback: %v", timeoutErr)
	}
	info.LastError = timeoutErr
	param.SetRetry(1)
	fallback, err := getChannel(c, info, param)
	if err != nil || fallback.Id != 10 {
		t.Fatalf("timeout did not select 9router: channel=%v err=%v", fallback, err)
	}
	info.InitChannelMeta(c)
	resp.Body = io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"fallback OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	if _, err := openai.OaiStreamHandler(c, info, resp); err != nil {
		t.Fatalf("9router inherited the expired XPiki watchdog: %v", err)
	}
}

func TestXPikiInvalidResponseBodyActuallyRetriesTo9router(t *testing.T) {
	c, info, param := setupRelayRetryCandidates(t)
	t.Setenv("RELAY_MAX_HOST_ATTEMPTS", "1")
	if err := model.DB.Model(&model.Channel{}).Where("id IN ?", []int{8, 9, 11, 12}).Update("base_url", "https://api.xpiki.com").Error; err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", 10).Update("base_url", "http://9router:20128").Error; err != nil {
		t.Fatal(err)
	}
	model.InitChannelCache()
	primary, err := getChannel(c, info, param)
	if err != nil || primary.Id == 10 {
		t.Fatal("initial attempt did not select XPiki")
	}
	param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, primary.Id)
	info.InitChannelMeta(c)
	info.RelayMode, info.RelayFormat = relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader("{}{}"))}
	_, bodyErr := openai.OpenaiHandler(c, info, resp)
	if bodyErr == nil || bodyErr.GetErrorCode() != types.ErrorCodeBadResponseBody || !shouldRetry(c, bodyErr, 5) {
		t.Fatalf("malformed XPiki HTTP-200 body stopped real fallback: %v", bodyErr)
	}
	info.LastError = bodyErr
	param.SetRetry(1)
	fallback, err := getChannel(c, info, param)
	if err != nil || fallback.Id != 10 {
		t.Fatalf("malformed body did not select 9router: channel=%v err=%v", fallback, err)
	}
	info.InitChannelMeta(c)
	resp.Body = io.NopCloser(strings.NewReader(`{"id":"fallback","object":"chat.completion","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"fallback OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	if _, err := openai.OpenaiHandler(c, info, resp); err != nil {
		t.Fatalf("9router valid response failed: %v", err)
	}
}

type fallbackTestTransport func(*http.Request) (*http.Response, error)

func (f fallbackTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// Real HTTP requests to two local upstreams, using the production request,
// response parsing, retry eligibility and channel-selection functions. No paid
// provider calls or production database/configuration changes.
func TestXPikiFailuresReach9routerOverHTTP(t *testing.T) {
	for _, scenario := range []string{"malformed_json", "http200_error", "408", "504", "524", "stream_error", "responses_error", "responses_truncated", "responses_200_error"} {
		t.Run(scenario, func(t *testing.T) {
			c, info, param := setupRelayRetryCandidates(t)
			t.Setenv("RELAY_MAX_HOST_ATTEMPTS", "1")
			t.Setenv("RELAY_FIRST_RESPONSE_TIMEOUT", "0")
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 2
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			settings := operation_setting.GetGeneralSetting()
			oldPing := settings.PingIntervalEnabled
			settings.PingIntervalEnabled = false
			t.Cleanup(func() { settings.PingIntervalEnabled = oldPing })
			for _, update := range []struct {
				ids  []int
				base string
			}{
				{[]int{8, 9, 11, 12}, "https://api.xpiki.com"}, {[]int{10}, "http://9router:20128"},
			} {
				if err := model.DB.Model(&model.Channel{}).Where("id IN ?", update.ids).Update("base_url", update.base).Error; err != nil {
					t.Fatal(err)
				}
			}
			model.InitChannelCache()
			payload := `{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"local fallback test"}]}`
			responses := strings.HasPrefix(scenario, "responses_")
			path := "/v1/chat/completions"
			if responses {
				path = "/v1/responses"
				payload = `{"model":"gpt-6.1-sol","input":"local fallback test"}`
				c.Request.URL.Path, param.RequestPath = path, path
			}
			var primaryCalls, fallbackCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != payload {
					t.Error("retry did not replay the original request")
				}
				if r.Host == "api.xpiki.com" {
					primaryCalls.Add(1)
					switch scenario {
					case "malformed_json":
						_, _ = io.WriteString(w, "{}{}")
					case "http200_error":
						_, _ = io.WriteString(w, `{"error":{"type":"upstream_error","message":"XPiki test failure"}}`)
					case "stream_error":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: {\"error\":{\"type\":\"upstream_error\",\"message\":\"XPiki test failure\"}}\n\n")
					case "responses_error", "responses_truncated":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"status\":\"in_progress\"}}\n\n")
						if scenario == "responses_error" {
							_, _ = io.WriteString(w, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"XPiki test failure\"}}}\n\n")
						}
					case "responses_200_error":
						_, _ = io.WriteString(w, `{"status":"failed","error":{"code":"server_error","message":"XPiki test failure"}}`)
					default:
						code := map[string]int{"408": 408, "504": 504, "524": 524}[scenario]
						w.WriteHeader(code)
						_, _ = io.WriteString(w, `{"error":{"type":"upstream_error","message":"XPiki test failure"}}`)
					}
					return
				}
				if r.Host != "9router" {
					t.Error("unexpected retry host")
				}
				fallbackCalls.Add(1)
				if responses {
					body := `{"id":"fallback-response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2}}`
					if scenario != "responses_200_error" {
						w.Header().Set("Content-Type", "text/event-stream")
						body = "data: {\"type\":\"response.completed\",\"response\":" + body + "}\n\n"
					}
					_, _ = io.WriteString(w, body)
				} else if scenario == "stream_error" {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fallback OK\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
				} else {
					_, _ = io.WriteString(w, `{"model":"gpt-6.1-sol","choices":[{"message":{"role":"assistant","content":"fallback OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
				}
			}))
			defer server.Close()
			info.IsStream = scenario == "stream_error" || scenario == "responses_error" || scenario == "responses_truncated"
			info.RelayMode, info.RelayFormat = relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI
			if responses {
				info.RelayMode, info.RelayFormat = relayconstant.RelayModeResponses, types.RelayFormatOpenAIResponses
			}
			client, err := service.GetHttpClientWithProxySettings("", info.ChannelSetting)
			if err != nil {
				t.Fatal(err)
			}
			oldTransport := client.Transport
			client.Transport = fallbackTestTransport(func(req *http.Request) (*http.Response, error) {
				clone := req.Clone(req.Context())
				target, err := url.Parse(server.URL)
				if err != nil {
					return nil, err
				}
				clone.Host = req.URL.Hostname()
				clone.URL = target
				clone.URL.Path = req.URL.Path
				return http.DefaultTransport.RoundTrip(clone)
			})
			t.Cleanup(func() { client.Transport = oldTransport })
			for attempt := 0; attempt < 2; attempt++ {
				param.SetRetry(attempt)
				selected, selectionErr := getChannel(c, info, param)
				if selectionErr != nil || selected == nil {
					t.Fatalf("channel selection failed: %v", selectionErr)
				}
				if (attempt == 0 && selected.Id == 10) || (attempt == 1 && selected.Id != 10) {
					t.Fatal("not an XPiki-to-9router retry")
				}
				param.AttemptedChannelIDs = append(param.AttemptedChannelIDs, selected.Id)
				info.InitChannelMeta(c)
				c.Request.Body = io.NopCloser(strings.NewReader(payload))
				base := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
				req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, base+path, strings.NewReader(payload))
				if err != nil {
					t.Fatal(err)
				}
				resp, err := relaychannel.DoRequest(c, req, info)
				if err != nil {
					t.Fatal(err)
				}
				var upstreamErr *types.NewAPIError
				if resp.StatusCode != 200 {
					upstreamErr = service.RelayErrorHandler(c.Request.Context(), resp, false)
				} else if responses && info.IsStream {
					_, upstreamErr = openai.OaiResponsesStreamHandler(c, info, resp)
				} else if responses {
					_, upstreamErr = openai.OaiResponsesHandler(c, info, resp)
				} else if info.IsStream {
					_, upstreamErr = openai.OaiStreamHandler(c, info, resp)
				} else {
					_, upstreamErr = openai.OpenaiHandler(c, info, resp)
				}
				if attempt == 0 {
					if info.SendResponseCount != 0 {
						t.Fatal("failed attempt leaked output before fallback")
					}
					if upstreamErr == nil || !shouldRetry(c, upstreamErr, 5) {
						t.Fatalf("real upstream failure did not retry: %v", upstreamErr)
					}
					info.LastError = upstreamErr
				} else if upstreamErr != nil {
					t.Fatalf("fallback HTTP response failed: %v", upstreamErr)
				}
			}
			if primaryCalls.Load() != 1 || fallbackCalls.Load() != 1 {
				t.Fatalf("network calls: XPiki=%d 9router=%d", primaryCalls.Load(), fallbackCalls.Load())
			}
		})
	}
}
