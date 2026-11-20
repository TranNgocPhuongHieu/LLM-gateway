package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
)

// errorSummaryRowLimit caps how many of the caller's newest error rows are
// classified. One extra row is fetched to detect truncation. Package var so
// tests can use a smaller cap.
var errorSummaryRowLimit = 5000

const (
	errorCategoryInsufficientBalance = "insufficient_balance"
	errorCategoryKeyQuotaExhausted   = "key_quota_exhausted"
	errorCategoryRateLimited         = "rate_limited"
	errorCategoryModelUnavailable    = "model_unavailable"
	errorCategoryContentBlocked      = "content_blocked"
	errorCategoryAuthFailed          = "auth_failed"
	errorCategoryInvalidRequest      = "invalid_request"
	errorCategoryServerError         = "server_error"
)

type errorCategoryRule struct {
	category string
	keywords []string
}

// errorCategoryRules are evaluated in order; the first rule with a keyword
// contained in the lowercased log content wins. Keywords come from the error
// strings produced by service/quota.go, service/billing_session.go,
// middleware/*.go, middleware/distributor.go, service/violation_fee.go,
// controller/relay.go and relaykit/types/error.go.
var errorCategoryRules = []errorCategoryRule{
	{errorCategoryInsufficientBalance, []string{
		"insufficient_user_quota",
		"user quota is not enough", // service/quota.go
		// Upstream "insufficient_quota" means a provider account is out of
		// credit, not the customer's wallet, so it is deliberately not here.
		"quota not enough",
		"insufficient balance", // billing_session.go: insufficientBalanceError
		// Rows written before the messages were translated to English.
		"用户额度不足",
		"预扣费额度失败",
	}},
	{errorCategoryKeyQuotaExhausted, []string{
		"token quota is not enough", // service/quota.go (token remain quota)
		"token quota exhausted",
		"pre_consume_token_quota_failed", // billing_session.go: token-level reservation
	}},
	{errorCategoryRateLimited, []string{
		"status_code=429",
		"rate_limit_exceeded",
		"upstream_rate_limited",
		"rate limit",
		"请求数限制", // middleware/model-rate-limit.go
	}},
	{errorCategoryModelUnavailable, []string{
		"model_not_found",
		"model not found",
		"invalid model",
		"for the models you can use",      // distributor.model_unavailable
		"no available channel",            // distributor.no_available_channel
		"failed to get available channel", // distributor.get_channel_failed
		"no access to model",              // distributor.token_model_forbidden
		"no access to any models",         // distributor.token_no_model_access
		"channel has been disabled",       // distributor.channel_disabled
		"origin_task_channel_disabled",
	}},
	{errorCategoryContentBlocked, []string{
		"prompt_blocked",
		"sensitive_words_detected", // controller/relay.go
		"violation_fee",            // service/violation_fee.go
		"content violates usage guidelines",
		"failed check: safety_check_type",
		"content_policy_violation", // upstream code mapped to 400
	}},
	{errorCategoryAuthFailed, []string{
		"status_code=401",
		"status_code=403",
		"unauthorized",
		"invalid access token", // auth.access_token_invalid
		"invalid token",
		"token not provided",
		"invalid_api_key",
		"authentication_error",
		"user has been banned", // auth.user_banned
		"no permission to access this group",
		"无权访问",                   // middleware/auth.go group access
		"不在令牌允许访问的列表中",           // middleware/auth.go IP allowlist
		"无法解析客户端 ip 地址",          // middleware/auth.go (lowercased)
		"普通用户不支持指定渠道",            // middleware/auth.go
		"已被弃用",                   // middleware/auth.go deprecated group
		"token packages support", // middleware/auth.go access_denied
	}},
	{errorCategoryInvalidRequest, []string{
		"status_code=400",
		"status_code=413",
		"status_code=422",
		"invalid_request",
		"invalid request", // distributor.invalid_request, relay type checks
		"invalid playground request",
		"invalid channel id",
		"bad_request_body",
		"read_request_body_failed",
		"request entity too large",
		"request body too large",
		"model name not specified",
		"invalid_api_type",
	}},
}

// classifyErrorLogContent maps a stored error-log content string to a category.
// Only the category name is returned; the content itself is never exposed.
func classifyErrorLogContent(content string) string {
	lower := strings.ToLower(content)
	for _, rule := range errorCategoryRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				return rule.category
			}
		}
	}
	return errorCategoryServerError
}

type ErrorSummaryCategory struct {
	Category  string `json:"category"`
	Count     int    `json:"count"`
	LastAt    int64  `json:"last_at"`
	ModelName string `json:"model_name"`
}

type ErrorSummary struct {
	Total      int                    `json:"total"`
	Truncated  bool                   `json:"truncated"`
	Categories []ErrorSummaryCategory `json:"categories"`
}

// GetUserErrorSummary classifies one user's terminal error logs in
// [startTimestamp, endTimestamp], newest first, up to errorSummaryRowLimit rows.
// Intermediate retry attempts are excluded by hideIntermediateRelayErrors.
func GetUserErrorSummary(userId int, startTimestamp int64, endTimestamp int64) (*ErrorSummary, error) {
	return getErrorSummary(&userId, startTimestamp, endTimestamp)
}

// GetErrorSummary is the system-wide variant of GetUserErrorSummary. userId 0
// means all users, minus admin and root accounts when excludeInternal is set;
// any other value filters to that user exactly as the self-service endpoint
// does and ignores excludeInternal.
func GetErrorSummary(userId int, startTimestamp int64, endTimestamp int64, excludeInternal bool) (*ErrorSummary, error) {
	if userId != 0 {
		return getErrorSummary(&userId, startTimestamp, endTimestamp)
	}
	var excludeIDs []int
	if excludeInternal {
		ids, err := internalUserIDs()
		if err != nil {
			return nil, err
		}
		excludeIDs = ids
	}
	return getErrorSummary(nil, startTimestamp, endTimestamp, excludeIDs...)
}

// getErrorSummary keeps the newest errorSummaryRowLimit terminal error rows,
// restricted to userId when it is non-nil and without excludeIDs' rows.
func getErrorSummary(userId *int, startTimestamp int64, endTimestamp int64, excludeIDs ...int) (*ErrorSummary, error) {
	order := "created_at desc, id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	tx := LOG_DB.Model(&Log{}).
		Select("content, model_name, created_at").
		Where("type = ? and created_at >= ? and created_at <= ?", LogTypeError, startTimestamp, endTimestamp)
	if userId != nil {
		tx = tx.Where("user_id = ?", *userId)
	}
	tx = hideIntermediateRelayErrors(excludeUserIDs(tx, excludeIDs), "")

	var rows []*Log
	if err := tx.Order(order).Limit(errorSummaryRowLimit + 1).Find(&rows).Error; err != nil {
		common.SysError("failed to query error summary logs: " + err.Error())
		return nil, errors.New("failed to query error logs")
	}
	truncated := len(rows) > errorSummaryRowLimit
	if truncated {
		rows = rows[:errorSummaryRowLimit]
	}

	summary := &ErrorSummary{
		Total:      len(rows),
		Truncated:  truncated,
		Categories: make([]ErrorSummaryCategory, 0),
	}
	indexByCategory := make(map[string]int)
	for _, row := range rows {
		category := classifyErrorLogContent(row.Content)
		idx, ok := indexByCategory[category]
		if !ok {
			// Rows are newest first, so the first row seen per category holds
			// its most recent model_name and last_at.
			idx = len(summary.Categories)
			indexByCategory[category] = idx
			summary.Categories = append(summary.Categories, ErrorSummaryCategory{
				Category:  category,
				LastAt:    row.CreatedAt,
				ModelName: row.ModelName,
			})
		}
		summary.Categories[idx].Count++
	}

	sort.SliceStable(summary.Categories, func(i, j int) bool {
		a, b := summary.Categories[i], summary.Categories[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.LastAt != b.LastAt {
			return a.LastAt > b.LastAt
		}
		return a.Category < b.Category
	})
	return summary, nil
}
