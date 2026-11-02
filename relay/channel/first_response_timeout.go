package channel

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	common2 "github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	relayconstant "github.com/TranNgocPhuongHieu/LLM-gateway/relay/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

func isReasoningModelName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	if strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") || strings.HasPrefix(name, "o4") {
		return true
	}
	keywords := []string{"reason", "think", "r1", "qwq", "sol", "luna"}
	for _, kw := range keywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	suffixes := []string{"-max", "-xhigh", "-high", "-medium", "-low", "-minimal"}
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

func isReasoningRequest(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if effort := strings.TrimSpace(strings.ToLower(info.ReasoningEffort)); effort != "" && effort != "none" && effort != "off" {
		return true
	}
	if info.ReasoningConversion != nil {
		return true
	}
	if info.ThinkingContentInfo.HasSentThinkingContent || info.ThinkingContentInfo.IsFirstThinkingContent {
		return true
	}
	if info.ChannelMeta != nil && info.ChannelSetting.ThinkingToContent {
		return true
	}
	if isReasoningModelName(info.GetOriginModelName()) ||
		isReasoningModelName(info.GetUpstreamModelName()) ||
		isReasoningModelName(info.GetBillingModelName()) {
		return true
	}
	return false
}

func promptPrefillTimeout(promptTokens int) time.Duration {
	if promptTokens <= 0 {
		return 0
	}
	// At ~2,000 tokens/second prefill throughput under load, prompt prefill
	// can take tens of seconds before producing the first token.
	// We add prefill allowance (e.g. ~35s for 70k tokens, ~57s for 114k tokens), capped at 120s.
	seconds := promptTokens / 2000
	if seconds > 120 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

func firstResponseTimeout(req *http.Request, info *relaycommon.RelayInfo) time.Duration {
	if req == nil || req.URL == nil || info == nil || info.ChannelMeta == nil || info.ApiType != constant.APITypeOpenAI {
		return 0
	}
	isChatOrMessages := info.RelayMode == relayconstant.RelayModeChatCompletions ||
		info.RelayMode == relayconstant.RelayModeCompletions ||
		info.RelayMode == relayconstant.RelayModeResponses ||
		info.RelayFormat == types.RelayFormatClaude
	if !isChatOrMessages {
		return 0
	}
	baseSeconds := common2.GetEnvOrDefault("RELAY_FIRST_RESPONSE_TIMEOUT", 0)
	if baseSeconds <= 0 {
		return 0
	}
	host := req.URL.Hostname()
	hostAllowed := false
	for _, allowed := range strings.Split(common2.GetEnvOrDefaultString("RELAY_FIRST_RESPONSE_TIMEOUT_HOSTS", ""), ",") {
		if strings.EqualFold(host, strings.TrimSpace(allowed)) {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return 0
	}

	timeout := time.Duration(baseSeconds) * time.Second

	// Reasoning models require significant time to perform internal chain-of-thought
	// or thinking before emitting the first token chunk.
	if isReasoningRequest(info) {
		reasoningSeconds := common2.GetEnvOrDefault("RELAY_REASONING_FIRST_RESPONSE_TIMEOUT", 0)
		if reasoningSeconds <= 0 {
			// By default, grant at least 180s (3 minutes) or 3x base timeout for reasoning models.
			reasoningSeconds = max(baseSeconds*3, 180)
		}
		if reasoningTimeout := time.Duration(reasoningSeconds) * time.Second; reasoningTimeout > timeout {
			timeout = reasoningTimeout
		}
	}

	// Large prompts require GPU KV-cache prefill before generating the first token.
	timeout += promptPrefillTimeout(info.GetEstimatePromptTokens())

	maxTimeout := common2.GetEnvOrDefault("STREAMING_TIMEOUT", 600)
	if maxTimeout <= 0 {
		maxTimeout = 600
	}
	if timeout > time.Duration(maxTimeout)*time.Second {
		timeout = time.Duration(maxTimeout) * time.Second
	}
	return timeout
}

func withFirstResponseTimeout(req *http.Request, info *relaycommon.RelayInfo, timeout time.Duration) (*http.Request, func()) {
	ctx, cancel := context.WithCancelCause(req.Context())
	var first sync.Once
	timer := time.AfterFunc(timeout, func() {
		first.Do(func() { cancel(relaycommon.ErrFirstResponseTimeout) })
	})
	info.FirstResponseContext = ctx
	info.FirstResponseReady = func() { first.Do(func() { timer.Stop() }) }
	cleanup := func() {
		timer.Stop()
		cancel(nil)
	}
	return req.WithContext(ctx), cleanup
}

// Keep the watchdog alive across response headers, but release it with the body.
type firstResponseBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *firstResponseBody) Close() error {
	b.cleanup()
	return b.ReadCloser.Close()
}
