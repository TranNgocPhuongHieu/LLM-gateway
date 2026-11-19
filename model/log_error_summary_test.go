package model

import (
	"encoding/json"
	"testing"
)

func TestClassifyErrorLogContent(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		// insufficient_balance: user wallet shortfall
		{"status_code=403, 用户额度不足, 剩余额度: $0.000000", "insufficient_balance"},
		{"status_code=403, 预扣费额度失败, 用户剩余额度: $0.1, 需要预扣费额度: $0.2", "insufficient_balance"},
		{"status_code=403, user quota is not enough, user quota: $0.10, need quota: $0.20", "insufficient_balance"},
		{"insufficient_user_quota", "insufficient_balance"},
		// Upstream provider quota errors must not tell the customer to top up.
		{"status_code=429, insufficient_quota: You exceeded your current quota", "rate_limited"},
		{"insufficient_quota: provider account out of credit", "server_error"},
		// key_quota_exhausted: token-level quota
		{"status_code=403, token quota is not enough, token remain quota: $0.00, need quota: $0.01", "key_quota_exhausted"},
		{"token quota exhausted", "key_quota_exhausted"},
		{"pre_consume_token_quota_failed", "key_quota_exhausted"},
		// rate_limited
		{"status_code=429, rate limit reached for gpt-4o", "rate_limited"},
		{"status_code=429, rate_limit_exceeded", "rate_limited"},
		{"您已达到请求数限制：1分钟内最多请求10次", "rate_limited"},
		// model_unavailable
		{"No available channel for model gpt-x under group default (distributor)", "model_unavailable"},
		{"Failed to get available channel for model gpt-x under group auto(default) (distributor): db timeout", "model_unavailable"},
		{"status_code=404, model_not_found: The model `gpt-x` does not exist", "model_unavailable"},
		{"status_code=404, model not found", "model_unavailable"},
		{"This token has no access to model gpt-x", "model_unavailable"},
		{"This channel has been disabled", "model_unavailable"},
		// content_blocked
		{"sensitive_words_detected", "content_blocked"},
		{"status_code=400, Content violates usage guidelines. Failed check: SAFETY_CHECK_TYPE_CSAM", "content_blocked"},
		{"violation_fee.grok.csam", "content_blocked"},
		{"status_code=400, prompt_blocked", "content_blocked"},
		// auth_failed
		{"Unauthorized, invalid access token", "auth_failed"},
		{"status_code=401, invalid_api_key", "auth_failed"},
		{"您的 IP 不在令牌允许访问的列表中", "auth_failed"},
		{"无权访问 vip 分组", "auth_failed"},
		{"User has been banned", "auth_failed"},
		// invalid_request
		{"Invalid request: unexpected end of JSON input", "invalid_request"},
		{"status_code=413, request body too large", "invalid_request"},
		{"Model name not specified, model name cannot be empty", "invalid_request"},
		{"status_code=400, bad_request_body", "invalid_request"},
		{"Invalid playground request: missing model", "invalid_request"},
		// server_error fallback, including internal rate-limit check failures
		{"status_code=500, upstream returned 500", "server_error"},
		{"status_code=503, dial tcp 10.0.0.5:443: connection refused", "server_error"},
		{"rate_limit_check_failed", "server_error"},
		{"", "server_error"},
	}
	for _, tc := range cases {
		if got := classifyErrorLogContent(tc.content); got != tc.want {
			t.Errorf("classifyErrorLogContent(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}

func TestGetUserErrorSummaryFiltersAndClassifies(t *testing.T) {
	db := setupSelfUsageTestDB(t)
	retry := func(final bool) string {
		encoded, err := json.Marshal(map[string]any{"request_final": final})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	rows := []Log{
		{UserId: 1, Type: LogTypeError, CreatedAt: 1000, ModelName: "gpt-old", Content: "status_code=429, rate limit reached", Other: retry(true)},
		{UserId: 1, Type: LogTypeError, CreatedAt: 1500, ModelName: "gpt-4o", Content: "status_code=403, token quota is not enough", Other: retry(true)},
		{UserId: 1, Type: LogTypeError, CreatedAt: 1800, ModelName: "claude", Content: "status_code=429, rate limit reached", Other: retry(true)},
		// Intermediate retry attempt: must be excluded.
		{UserId: 1, Type: LogTypeError, CreatedAt: 1900, ModelName: "retry-only", Content: "status_code=503, dial tcp failed", Other: retry(false)},
		// Successful consume log: not an error.
		{UserId: 1, Type: LogTypeConsume, CreatedAt: 1900, ModelName: "gpt-4o", Content: "ok"},
		// Another user's error: must be excluded.
		{UserId: 2, Type: LogTypeError, CreatedAt: 1900, ModelName: "gpt-4o", Content: "Unauthorized, invalid access token", Other: retry(true)},
		// Outside the range.
		{UserId: 1, Type: LogTypeError, CreatedAt: 9000, ModelName: "gpt-4o", Content: "status_code=500, boom", Other: retry(true)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	summary, err := GetUserErrorSummary(1, 500, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 3 || summary.Truncated {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if len(summary.Categories) != 2 {
		t.Fatalf("unexpected categories: %+v", summary.Categories)
	}
	first := summary.Categories[0]
	if first.Category != "rate_limited" || first.Count != 2 || first.LastAt != 1800 || first.ModelName != "claude" {
		t.Fatalf("rate_limited category wrong (most recent model expected): %+v", first)
	}
	second := summary.Categories[1]
	if second.Category != "key_quota_exhausted" || second.Count != 1 || second.LastAt != 1500 || second.ModelName != "gpt-4o" {
		t.Fatalf("key_quota_exhausted category wrong: %+v", second)
	}
}

func TestGetUserErrorSummaryTruncatesAtCap(t *testing.T) {
	db := setupSelfUsageTestDB(t)
	oldLimit := errorSummaryRowLimit
	errorSummaryRowLimit = 2
	t.Cleanup(func() { errorSummaryRowLimit = oldLimit })

	rows := []Log{
		{UserId: 1, Type: LogTypeError, CreatedAt: 1000, ModelName: "oldest", Content: "status_code=500, boom"},
		{UserId: 1, Type: LogTypeError, CreatedAt: 2000, ModelName: "middle", Content: "status_code=429, rate limit"},
		{UserId: 1, Type: LogTypeError, CreatedAt: 3000, ModelName: "newest", Content: "status_code=429, rate limit"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	summary, err := GetUserErrorSummary(1, 1, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Truncated || summary.Total != 2 {
		t.Fatalf("expected truncated summary of 2 rows: %+v", summary)
	}
	if len(summary.Categories) != 1 || summary.Categories[0].Category != "rate_limited" ||
		summary.Categories[0].Count != 2 || summary.Categories[0].ModelName != "newest" {
		t.Fatalf("truncation must keep the newest rows: %+v", summary.Categories)
	}
}
