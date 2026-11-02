package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

var (
	channelRefRegex = regexp.MustCompile(`(?i)(?:channel\s*#?\s*\d+(?:\s*['"][^'"]*['"])?[:\s-]*|channel:\s*\w+[:\s-]*)`)
	requestIdRegex  = regexp.MustCompile(`(?i)(?:request\s*id:\s*|request_id=)([a-zA-Z0-9_-]+)`)
)

func isClientActionableError(content string) bool {
	lower := strings.ToLower(content)

	// Explicit client-side error keywords
	clientKeywords := []string{
		"insufficient_user_quota",
		"insufficient_quota",
		"pre_consume_token_quota_failed",
		"user quota is not enough",
		"quota not enough",
		"token quota exhausted",
		"invalid_request",
		"bad_request_body",
		"read_request_body_failed",
		"invalid_api_type",
		"model_not_found",
		"model not found",
		"not found",
		"does not exist",
		"invalid model",
		"prompt_blocked",
		"sensitive_words_detected",
		"violation_fee.grok.csam",
		"status_code=400",
		"status_code=401",
		"status_code=403",
		"status_code=404",
		"status_code=413",
		"status_code=422",
		"status_code=429",
		"rate_limit_exceeded",
		"rate limit",
		"request entity too large",
		"invalid access token",
		"unauthorized",
	}

	for _, kw := range clientKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func containsInternalDiagnostics(content string) bool {
	lower := strings.ToLower(content)
	internalKeywords := []string{
		"channel #",
		"channel:",
		"channel_",
		"upstream",
		"provider",
		"9router",
		"status_code=500",
		"status_code=502",
		"status_code=503",
		"status_code=504",
		"bad response status code: 5",
		"dial tcp",
		"connection refused",
		"context deadline exceeded",
		"i/o timeout",
		"tls handshake",
		"read_response_body_failed",
		"do_request_failed",
		"get_channel_failed",
		"no_available_key",
		"all_channels_failed",
	}

	for _, kw := range internalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// ProjectUserSafeErrorContent projects a log's content field for user-facing views (self/token logs).
// Client-actionable errors are preserved so users know how to fix them.
// Internal/channel/provider diagnostics are masked into a safe message with request ID.
func ProjectUserSafeErrorContent(content string, requestId string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	// Extract request ID from content if not explicitly provided
	if requestId == "" {
		if match := requestIdRegex.FindStringSubmatch(content); len(match) > 1 {
			requestId = match[1]
		}
	}

	if isClientActionableError(content) {
		// Clean any incidental internal channel references
		cleaned := channelRefRegex.ReplaceAllString(content, "")
		cleaned = kitutil.MaskSensitiveInfo(cleaned)
		cleaned = strings.TrimSpace(cleaned)
		// Check if remaining message still has internal server diagnostics (like 5xx, dial tcp)
		if !containsInternalDiagnostics(cleaned) {
			if requestId != "" && !strings.Contains(cleaned, requestId) {
				return fmt.Sprintf("%s (request id: %s)", cleaned, requestId)
			}
			return cleaned
		}
	}

	// If it's an internal / provider error or contains internal diagnostics
	if requestId != "" {
		return fmt.Sprintf("The server encountered an error processing your request. (request id: %s)", requestId)
	}
	return "The server encountered an error processing your request."
}

// ProjectUserSafeStreamError projects stream_status error strings for user-facing views.
func ProjectUserSafeStreamError(errStr string) string {
	errStr = strings.TrimSpace(errStr)
	if errStr == "" {
		return ""
	}

	lower := strings.ToLower(errStr)
	if strings.Contains(lower, "cancel") || strings.Contains(lower, "client_gone") || strings.Contains(lower, "context canceled") {
		return "cancelled"
	}
	if strings.Contains(lower, "incomplete") {
		return "stream incomplete"
	}
	if containsInternalDiagnostics(errStr) {
		return "stream interrupted"
	}
	cleaned := channelRefRegex.ReplaceAllString(errStr, "")
	return kitutil.MaskSensitiveInfo(cleaned)
}
