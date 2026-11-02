package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func xpikiSourceKeyID(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return fmt.Sprintf("xpiki:%x", sum[:8])
}

// Exact provider IDs identify the NewAPI caller. Legacy usage matching is
// explicitly labelled and never overrides rows carrying another provider ID.
func attachXPikiRequestOrigins(ctx context.Context, items []XPikiLogItem) error {
	if model.LOG_DB == nil || len(items) == 0 {
		return nil
	}
	var ids []string
	var channelIDs []int
	var oldest, newest int64
	for _, item := range items {
		if item.RequestID != "" {
			ids = append(ids, item.RequestID)
		}
		for _, ch := range item.SourceChannels {
			channelIDs = append(channelIDs, ch.ID)
		}
		stamp, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			continue
		}
		if oldest == 0 || stamp.Unix() < oldest {
			oldest = stamp.Unix()
		}
		newest = max(newest, stamp.Unix())
	}
	if len(channelIDs) == 0 {
		return nil
	}
	type origin struct {
		ID                int
		Type              int
		ChannelID         int
		RequestID         string
		UpstreamRequestID string
		TokenName         string
		ModelName         string
		PromptTokens      int
		CompletionTokens  int
		CreatedAt         int64
		Other             string
	}
	var exact, legacy []origin
	base := model.LOG_DB.WithContext(ctx).Table("logs").Select("id,type,channel_id,request_id,upstream_request_id,token_name,model_name,prompt_tokens,completion_tokens,created_at,other").Where("type IN (2,5) AND channel_id IN ?", channelIDs)
	if len(ids) > 0 {
		if err := base.Session(&gorm.Session{}).Where("upstream_request_id IN ?", ids).Limit(1001).Scan(&exact).Error; err != nil {
			return err
		}
	}
	if oldest > 0 {
		if err := base.Session(&gorm.Session{}).Where("COALESCE(upstream_request_id,'')='' AND created_at BETWEEN ? AND ?", oldest-1, newest+1).Limit(1001).Scan(&legacy).Error; err != nil {
			return err
		}
	}
	if len(exact) > 1000 || len(legacy) > 1000 {
		return errors.New("origin lookup row budget reached")
	}
	matchedLogIDs := make(map[int]int)
	var localRequestIDs []string
	for i := range items {
		item := &items[i]
		var candidates []origin
		method := "request_id"
		for _, row := range exact {
			if row.UpstreamRequestID == item.RequestID && slices.ContainsFunc(item.SourceChannels, func(ch XPikiSourceChannel) bool { return ch.ID == row.ChannelID }) {
				candidates = append(candidates, row)
			}
		}
		if len(candidates) == 0 {
			method = "usage_time"
			for _, row := range legacy {
				if !slices.ContainsFunc(item.SourceChannels, func(ch XPikiSourceChannel) bool { return ch.ID == row.ChannelID }) {
					continue
				}
				local := XPikiLogItem{ChannelId: row.ChannelID, CreatedAt: time.Unix(row.CreatedAt, 0).Format(time.RFC3339), ModelSlug: row.ModelName, Status: "success", InputTokens: row.PromptTokens, OutputTokens: row.CompletionTokens}
				var usage struct {
					Read     int `json:"cache_tokens"`
					Write    int `json:"cache_write_tokens"`
					Creation int `json:"cache_creation_tokens"`
				}
				if row.Type != model.LogTypeConsume || (row.Other != "" && common.Unmarshal([]byte(row.Other), &usage) != nil) {
					continue
				}
				if usage.Write == 0 {
					usage.Write = usage.Creation
				}
				local.CacheReadTokens, local.CacheWriteTokens = usage.Read, usage.Write
				provider := *item
				provider.ChannelId = row.ChannelID
				if xpikiCostMatches(local, provider) {
					candidates = append(candidates, row)
				}
			}
		}
		if len(candidates) == 0 {
			continue
		}
		first := candidates[0]
		ambiguous := false
		for _, row := range candidates {
			if row.RequestID != first.RequestID || row.ChannelID != first.ChannelID || row.TokenName != first.TokenName {
				ambiguous = true
			}
			if row.ID > first.ID {
				first = row
			}
		}
		if ambiguous {
			item.RequestOrigin = "ambiguous"
			continue
		}
		item.RequestOrigin, item.OriginMatch = "newapi", method
		if method == "usage_time" {
			item.RequestOrigin = "usage_match"
		}
		item.LocalRequestID, item.TokenName = first.RequestID, first.TokenName
		if method == "request_id" && first.RequestID != "" {
			matchedLogIDs[i] = first.ID
			if !slices.Contains(localRequestIDs, first.RequestID) {
				localRequestIDs = append(localRequestIDs, first.RequestID)
			}
		}
		if method == "request_id" || len(item.SourceChannels) == 1 {
			item.ChannelId = first.ChannelID
			for _, ch := range item.SourceChannels {
				if ch.ID == first.ChannelID {
					item.ChannelName = ch.Name
				}
			}
		}
	}
	if len(localRequestIDs) == 0 {
		return nil
	}
	// Provider history describes one XPiki attempt, not the final NewAPI result.
	// Link later successful attempts (including 9router) only through an exact
	// local request ID. Keep provider status/channel/fee unchanged.
	var completed []origin
	if err := model.LOG_DB.WithContext(ctx).Table("logs").Select("id,channel_id,request_id").
		Where("type = ? AND request_id IN ?", model.LogTypeConsume, localRequestIDs).
		Order("id DESC").Limit(1001).Scan(&completed).Error; err != nil {
		return err
	}
	if len(completed) > 1000 {
		return errors.New("fallback outcome lookup row budget reached")
	}
	var finalChannelIDs []int
	for _, row := range completed {
		if !slices.Contains(finalChannelIDs, row.ChannelID) {
			finalChannelIDs = append(finalChannelIDs, row.ChannelID)
		}
	}
	channelNames := make(map[int]string)
	if len(finalChannelIDs) > 0 && model.DB != nil {
		var channels []struct {
			ID   int
			Name string
		}
		if err := model.DB.WithContext(ctx).Table("channels").Select("id,name").Where("id IN ?", finalChannelIDs).Scan(&channels).Error; err != nil {
			return err
		}
		for _, ch := range channels {
			channelNames[ch.ID] = ch.Name
		}
	}
	for i, matchedID := range matchedLogIDs {
		item := &items[i]
		for _, row := range completed {
			if row.RequestID != item.LocalRequestID || row.ID < matchedID {
				continue
			}
			item.LocalFinalStatus = "success"
			if row.ID > matchedID && row.ChannelID != item.ChannelId {
				item.FallbackChannelID = row.ChannelID
				item.FallbackChannelName = channelNames[row.ChannelID]
			}
			break // Rows are newest first; never borrow an older completion.
		}
	}
	return nil
}

func fetchXPikiLogsForCredential(
	ctx context.Context,
	cred XPikiQuotaCredential,
	endpoint string,
	page, pageSize int,
	status, reqType string,
) ([]XPikiLogItem, bool, error) {
	apiKey := strings.TrimSpace(cred.APIKey)
	if apiKey == "" {
		return nil, false, errors.New("empty API key")
	}

	client, err := GetHttpClientWithProxy("")
	if err != nil || client == nil {
		return nil, false, errors.New("could not configure HTTP client")
	}
	clientCopy := *client
	clientCopy.Timeout = 15 * time.Second
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	reqURL := fmt.Sprintf("%s?page=%d&page_size=%d", endpoint, page, pageSize)
	if status != "" && status != "all" {
		reqURL += "&status=" + url.QueryEscape(status)
	}
	if reqType != "" && reqType != "all" {
		reqURL += "&type=" + url.QueryEscape(reqType)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := clientCopy.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, false, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d", response.StatusCode)
	}

	var envelope struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
		Data    struct {
			HasMore bool `json:"has_more"`
			Items   []struct {
				CreatedAt        string          `json:"created_at"`
				Type             string          `json:"type"`
				ModelSlug        string          `json:"model_slug"`
				Status           string          `json:"status"`
				ErrorCode        string          `json:"error_code"`
				InputTokens      int             `json:"input_tokens"`
				OutputTokens     int             `json:"output_tokens"`
				CacheReadTokens  int             `json:"cache_read_tokens"`
				CacheWriteTokens int             `json:"cache_write_tokens"`
				CreditCost       json.RawMessage `json:"credit_cost"`
				DurationMs       int             `json:"duration_ms"`
				RequestID        string          `json:"request_id"`
				IP               string          `json:"ip"`
				ImageCount       int             `json:"image_count"`
				VideoCount       int             `json:"video_count"`
				VideoSeconds     float64         `json:"video_seconds"`
			} `json:"items"`
		} `json:"data"`
	}

	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, false, err
	}
	if envelope.Code == nil || *envelope.Code != 0 {
		return nil, false, fmt.Errorf("upstream error: %s", envelope.Message)
	}

	maskedKey := MaskXPikiKey(apiKey)
	items := make([]XPikiLogItem, 0, len(envelope.Data.Items))
	for _, raw := range envelope.Data.Items {
		costDec, costErr := parseXPikiDecimal(raw.CreditCost)
		costFloat, _ := costDec.Float64()
		costVND := FormatXPikiCostVNDForModel(costDec, raw.ModelSlug)
		if costErr != nil || costDec.IsNegative() {
			costVND = ""
		}

		item := XPikiLogItem{
			CreatedAt:         sanitizeXPikiText(raw.CreatedAt, apiKey, 50),
			Type:              sanitizeXPikiText(raw.Type, apiKey, 30),
			ModelSlug:         sanitizeXPikiText(raw.ModelSlug, apiKey, 100),
			Status:            sanitizeXPikiText(raw.Status, apiKey, 30),
			ErrorCode:         sanitizeXPikiText(raw.ErrorCode, apiKey, 100),
			InputTokens:       raw.InputTokens,
			OutputTokens:      raw.OutputTokens,
			CacheReadTokens:   raw.CacheReadTokens,
			CacheWriteTokens:  raw.CacheWriteTokens,
			CreditCost:        costFloat,
			CostVND:           costVND,
			DurationMs:        raw.DurationMs,
			RequestID:         sanitizeXPikiText(raw.RequestID, apiKey, 120),
			IP:                sanitizeXPikiText(raw.IP, apiKey, 50),
			KeyLabel:          cred.Label,
			KeyMasked:         maskedKey,
			ChannelId:         cred.ChannelId,
			ChannelName:       cred.ChannelName,
			Destination:       "api.xpiki.com",
			mediaUsage:        raw.ImageCount != 0 || raw.VideoCount != 0 || raw.VideoSeconds != 0,
			UpstreamRequestID: sanitizeXPikiText(raw.RequestID, apiKey, 120),
		}
		if costErr == nil && !costDec.IsNegative() {
			tokens, reference := xpikiChargedReferenceTokens(costDec, raw.ModelSlug)
			if reference != "" {
				item.CostReferenceModel = reference
				item.ChargedReferenceTokens = formatXPikiNumber(tokens.Round(4))
			}
			item.UpstreamQuota = costDec.String()
			item.QuotaAstra = formatXPikiNumber(costDec.Mul(decimal.NewFromInt(xpikiAstraTokenMultiple)))
			item.QuotaSource = "xpiki"
		} else {
			item.QuotaSource = "unavailable"
		}
		items = append(items, item)
	}

	return items, envelope.Data.HasMore, nil
}

func GetLocalRequestHealth(ctx context.Context, end time.Time) (*RequestHealth, error) {
	const limit = 10000
	r := &RequestHealth{StartAt: end.Add(-15 * time.Minute).Unix(), EndAt: end.Unix(),
		Outcomes: make(map[string]int), Failures: make(map[string]int), Alerts: []string{}}
	var rows []model.Log
	if err := model.LOG_DB.WithContext(ctx).Select("id, request_id, type, is_stream, content, other").
		Where("created_at >= ? AND created_at < ? AND type IN (2, 5)", r.StartAt, r.EndAt).
		Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		r.Capped = true
	}
	r.Attempts = len(rows)
	groups := make(map[string][]model.Log)
	for _, row := range rows {
		key := "req:" + row.RequestId
		if strings.TrimSpace(row.RequestId) == "" {
			key = fmt.Sprintf("log:%d", row.Id)
		}
		groups[key] = append(groups[key], row)
	}
	for _, attempts := range groups {
		best, failed := attempts[0], 0
		for _, row := range attempts {
			if row.Type == model.LogTypeError {
				failed++
			}
			if row.Type == model.LogTypeConsume && (best.Type != model.LogTypeConsume || row.Id > best.Id) {
				best = row
			}
		}
		outcome := localRequestOutcome(best.Type, best.IsStream, best.Content, best.Other, failed)
		if strings.TrimSpace(best.RequestId) == "" {
			outcome = "unknown"
		}
		r.Outcomes[outcome]++
		if outcome != "failed_final" && outcome != "stream_failed" {
			continue
		}
		var info struct {
			Code         int    `json:"status_code"`
			Origin       string `json:"failure_origin"`
			UpstreamCode int    `json:"upstream_status_code"`
		}
		_ = json.Unmarshal([]byte(best.Other), &info)
		kind := "unclassified"
		switch {
		case info.Origin == "upstream" && info.UpstreamCode == 429:
			kind = "upstream_429"
		case info.Origin == "upstream" && info.UpstreamCode >= 500 && info.UpstreamCode < 600:
			kind = "upstream_5xx"
		case info.Code == 408 || info.Code == 504 || info.Code == 524:
			kind = "timeout"
		case info.Origin == "gateway":
			kind = "gateway"
		case info.Code == 429:
			kind = "http_429"
		case info.Code >= 500 && info.Code < 600:
			kind = "http_5xx"
		}
		r.Failures[kind]++
	}
	r.Requests = len(groups)
	known := r.Outcomes["success"] + r.Outcomes["retry_recovered"] + r.Outcomes["failed_final"] + r.Outcomes["stream_failed"] + r.Outcomes["stream_incomplete"]
	// A conservative local warning, not a claimed regression against an unknown
	// baseline. Capped/unknown cohorts must never produce a final error-rate alert.
	if !r.Capped && r.Outcomes["unknown"] == 0 && known >= 100 &&
		(r.Outcomes["failed_final"]+r.Outcomes["stream_failed"])*100 >= known*10 {
		r.Alerts = append(r.Alerts, "final_failure_rate_at_least_10_percent")
	}
	return r, nil
}
