package channel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

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
