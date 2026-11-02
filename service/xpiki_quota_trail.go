package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func fetchLocalRequestLogsReport(
	ctx context.Context,
	credentials []XPikiQuotaCredential,
	filter XPikiLogsFilter,
) (XPikiLogsReport, error) {
	report := XPikiLogsReport{Source: "newapi", Items: make([]XPikiLogItem, 0), KeyOptions: make([]XPikiKeyOption, 0)}
	groupMode := strings.ToLower(strings.TrimSpace(filter.GroupMode))
	if groupMode == "" {
		groupMode = "grouped"
	}
	if groupMode != "grouped" && groupMode != "all" && groupMode != "raw" {
		return report, fmt.Errorf("invalid group_mode: use grouped or all")
	}
	grouped := groupMode == "grouped"
	channelNameMap := make(map[int]string)
	channelBaseURLMap := make(map[int]string)
	for _, cred := range credentials {
		if cred.ChannelId > 0 {
			channelNameMap[cred.ChannelId] = cred.ChannelName
			channelBaseURLMap[cred.ChannelId] = cred.BaseURL
		}
	}
	allChannels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return report, fmt.Errorf("load log channels: %w", err)
	}
	for _, ch := range allChannels {
		channelNameMap[ch.Id] = ch.Name
		channelBaseURLMap[ch.Id] = ch.GetBaseURL()
	}
	channelName := func(id int) string {
		if id == 0 {
			return "Chưa chọn kênh"
		}
		if name := channelNameMap[id]; name != "" {
			return name
		}
		return fmt.Sprintf("OpenAI-%d", id)
	}
	destination := func(id int) string {
		if id == 0 {
			return ""
		}
		return extractDestination(channelBaseURLMap[id])
	}
	report.KeyOptions = append(report.KeyOptions, XPikiKeyOption{Value: "all", Label: "All Keys / Channels"})
	if err := model.LOG_DB.WithContext(ctx).Table("logs").Where("logs.type IN (2, 5) AND logs.model_name != ''").
		Distinct("logs.model_name").Order("logs.model_name").Pluck("logs.model_name", &report.ModelOptions).Error; err != nil {
		return report, fmt.Errorf("load log models: %w", err)
	}
	if err := model.LOG_DB.WithContext(ctx).Table("logs").Where("logs.type IN (2, 5) AND logs.username != ''").
		Distinct("logs.username").Order("logs.username").Pluck("logs.username", &report.UserOptions).Error; err != nil {
		return report, fmt.Errorf("load log users: %w", err)
	}
	// Select identity and name only, never API key material. Include deleted keys
	// so historical requests remain identifiable after a key is retired.
	var tokens []struct {
		Id        int
		Name      string
		DeletedAt gorm.DeletedAt
	}
	if err := model.DB.WithContext(ctx).Unscoped().Model(&model.Token{}).Select("id, name, deleted_at").Order("id").Scan(&tokens).Error; err != nil {
		return report, fmt.Errorf("load tokens: %w", err)
	}
	tokenNameByID := make(map[int]string, len(tokens))
	for _, token := range tokens {
		tokenNameByID[token.Id] = token.Name
		if token.DeletedAt.Valid {
			continue
		}
		report.KeyOptions = append(report.KeyOptions, XPikiKeyOption{Value: fmt.Sprintf("token_id:%d", token.Id), Label: fmt.Sprintf("Khóa #%d: %s", token.Id, token.Name)})
	}
	var channelIDs []int
	for _, ch := range allChannels {
		channelIDs = append(channelIDs, ch.Id)
	}
	slices.Sort(channelIDs)
	for _, id := range slices.Compact(channelIDs) {
		label := "Kênh: " + channelName(id)
		report.KeyOptions = append(report.KeyOptions, XPikiKeyOption{
			Value: fmt.Sprintf("channel:%d", id), Label: label,
			ChannelId: id, ChannelName: channelName(id),
		})
	}

	// Scope both modes to request attempts, never recharge or administrative logs.
	innerWhere := []string{"type IN (2, 5)"}
	var innerArgs []any
	dateBounds := []struct{ value, operator string }{
		{strings.TrimSpace(filter.StartDate), ">="},
		{strings.TrimSpace(filter.EndDate), "<"},
	}
	if date := strings.TrimSpace(filter.Date); date != "" {
		dateBounds[0].value, dateBounds[1].value = date, date
	}
	for _, bound := range dateBounds {
		if bound.value == "" {
			continue
		}
		stamp, err := time.ParseInLocation("2006-01-02", bound.value, time.Local)
		if err != nil {
			return report, fmt.Errorf("invalid log date: use YYYY-MM-DD")
		}
		if bound.operator == "<" {
			stamp = stamp.AddDate(0, 0, 1)
		}
		innerWhere = append(innerWhere, "created_at "+bound.operator+" ?")
		innerArgs = append(innerArgs, stamp.Unix())
	}
	// Prefix blank-ID row keys to avoid collisions with real request IDs.
	// Current relay logs persist only the terminal row. Older logs may persist
	// every attempt; support both without manufacturing usage/cost entries.
	recordedAttemptsSQL := `CASE WHEN json_valid(%s.other) THEN
 CASE WHEN json_extract(%s.other, '$.admin_info.use_channel[#-1]') = CAST(%s.channel_id AS TEXT)
 AND NOT EXISTS (SELECT 1 FROM json_each(%s.other, '$.admin_info.use_channel') j
 WHERE j.type != 'text' OR j.value = '' OR j.value GLOB '*[^0-9]*' OR CAST(j.value AS INTEGER) <= 0)
 THEN COALESCE(json_array_length(%s.other, '$.admin_info.use_channel'), 0) ELSE 0 END ELSE 0 END`
	recordedAttemptsSQL = fmt.Sprintf(recordedAttemptsSQL, "logs", "logs", "logs", "logs", "logs")
	if model.LOG_DB.Dialector.Name() == "postgres" {
		// Extract only a well-formed numeric channel array so legacy malformed
		// metadata cannot make the JSON cast fail for the entire log page.
		trailSQL := func(alias string) string {
			return fmt.Sprintf(`substring(%s.other FROM '%s')::json`, alias, xpikiChannelTrailPattern)
		}
		recordedAttemptsSQL = fmt.Sprintf(`CASE WHEN (%s ->> -1) = CAST(%s.channel_id AS TEXT) THEN COALESCE(json_array_length(%s), 0) ELSE 0 END`, trailSQL("logs"), "logs", trailSQL("logs"))
	} else if model.LOG_DB.Dialector.Name() == "mysql" {
		recordedAttemptsSQL = `CASE WHEN JSON_VALID(logs.other) THEN COALESCE(JSON_LENGTH(JSON_EXTRACT(logs.other, '$.admin_info.use_channel')), 0) ELSE 0 END`
	}
	// Keep the expensive JSON/regex expression out of scoped. It is only
	// needed for retry_recovered and is evaluated against the joined winner
	// after grouping, rather than for every log row.
	recordedAttemptsExpr := recordedAttemptsSQL
	if strings.ToLower(strings.TrimSpace(filter.Status)) != "retry_recovered" {
		recordedAttemptsExpr = "0"
	}
	cte := fmt.Sprintf(`
WITH scoped AS (
	SELECT logs.id, logs.created_at, logs.type, logs.content, logs.token_name, logs.token_id,
    logs.model_name, logs.quota, logs.prompt_tokens, logs.completion_tokens, logs.use_time,
    logs.other, logs.upstream_request_id, logs.channel_id, logs.request_id, logs.ip,
	    logs.username, logs.is_stream, 0 AS recorded_attempts,
    CASE WHEN COALESCE(logs.request_id, '') != ''
    THEN 'req:' || logs.request_id ELSE 'log:' || CAST(logs.id AS text) END AS req_key
  FROM logs WHERE %s
), grouped AS (
  SELECT req_key, MAX(created_at) AS created_at,
    COALESCE(MAX(CASE WHEN type = 2 THEN id END), MAX(id)) AS best_id,
    COUNT(*) AS total_attempts,
    COUNT(CASE WHEN type = 5 THEN 1 END) AS failed_attempts,
    MAX(CASE WHEN type = 2 THEN 1 ELSE 0 END) AS has_success,
    MIN(CASE WHEN type = 5 THEN id END) AS first_error_id
  FROM scoped GROUP BY req_key
	)`, strings.Join(innerWhere, " AND "))

	// In grouped mode, match filters against every attempt, not only the winner.
	var outerWhere []string
	var outerArgs []any
	attemptMatch := func(condition string) string {
		if grouped {
			return "EXISTS (SELECT 1 FROM scoped sub WHERE sub.req_key = g.req_key AND " + condition + ")"
		}
		return strings.ReplaceAll(strings.ReplaceAll(condition, "sub.", "l."), "g.", "l.")
	}
	keyFilter := strings.TrimSpace(filter.KeyFilter)
	if keyFilter != "" && keyFilter != "all" {
		if strings.HasPrefix(keyFilter, "token_id:") {
			id, err := strconv.Atoi(strings.TrimPrefix(keyFilter, "token_id:"))
			if err != nil || id <= 0 {
				return report, fmt.Errorf("invalid token ID filter")
			}
			outerWhere = append(outerWhere, attemptMatch("sub.token_id = ?"))
			outerArgs = append(outerArgs, id)
		} else if strings.HasPrefix(keyFilter, "token:") {
			outerWhere = append(outerWhere, attemptMatch("sub.token_name = ?"))
			outerArgs = append(outerArgs, strings.TrimPrefix(keyFilter, "token:"))
		} else if strings.HasPrefix(keyFilter, "channel:") {
			id, err := strconv.Atoi(strings.TrimPrefix(keyFilter, "channel:"))
			if err != nil || id < 0 {
				return report, fmt.Errorf("invalid channel filter")
			}
			outerWhere = append(outerWhere, attemptMatch("sub.channel_id = ?"))
			outerArgs = append(outerArgs, id)
		} else if id, err := strconv.Atoi(keyFilter); err == nil {
			outerWhere = append(outerWhere, attemptMatch("sub.channel_id = ?"))
			outerArgs = append(outerArgs, id)
		} else {
			outerWhere = append(outerWhere, attemptMatch("sub.token_name = ?"))
			outerArgs = append(outerArgs, keyFilter)
		}
	}
	if channel := strings.TrimSpace(filter.ChannelFilter); channel != "" && channel != "all" {
		id, err := strconv.Atoi(strings.TrimPrefix(channel, "channel:"))
		if err != nil || id < 0 {
			return report, fmt.Errorf("invalid channel filter")
		}
		outerWhere = append(outerWhere, attemptMatch("sub.channel_id = ?"))
		outerArgs = append(outerArgs, id)
	}
	if name := strings.TrimSpace(filter.Model); name != "" {
		outerWhere = append(outerWhere, attemptMatch("sub.model_name IN ?"))
		outerArgs = append(outerArgs, splitLogFilterValues(name))
	}
	if users := strings.TrimSpace(filter.User); users != "" {
		outerWhere = append(outerWhere, attemptMatch("sub.username IN ?"))
		outerArgs = append(outerArgs, splitLogFilterValues(users))
	}
	successCondition := "g.has_success = 1"
	failureCondition := "g.has_success = 0"
	if !grouped {
		successCondition, failureCondition = "l.type = 2", "l.type = 5"
	}
	canceledCondition := "(LOWER(COALESCE(l.content, '')) LIKE '%cancel%' OR LOWER(COALESCE(l.content, '')) LIKE '%abort%')"
	retryRecoveredCondition := fmt.Sprintf("g.has_success = 1 AND (g.failed_attempts > 0 OR (%s) > 1)", strings.ReplaceAll(recordedAttemptsExpr, "logs.", "l."))
	switch strings.ToLower(strings.TrimSpace(filter.Status)) {
	case "success":
		outerWhere = append(outerWhere, successCondition)
	case "retry_recovered":
		if grouped {
			outerWhere = append(outerWhere, retryRecoveredCondition)
		} else {
			outerWhere = append(outerWhere, "l.type = 2 AND "+retryRecoveredCondition)
		}
	case "failed":
		outerWhere = append(outerWhere, failureCondition+" AND NOT "+canceledCondition)
	case "canceled":
		outerWhere = append(outerWhere, failureCondition+" AND "+canceledCondition)
	}
	if search := strings.ToLower(strings.TrimSpace(filter.Search)); search != "" {
		var matches []string
		for _, column := range []string{"request_id", "model_name", "token_name", "ip", "content", "username", "upstream_request_id"} {
			matches = append(matches, "LOWER(COALESCE(sub."+column+", '')) LIKE ?")
			outerArgs = append(outerArgs, "%"+search+"%")
		}
		var matchedIDs []int
		for _, id := range channelIDs {
			if strings.Contains(strings.ToLower(channelName(id)), search) ||
				strings.Contains(strings.ToLower(channelBaseURLMap[id]), search) {
				matchedIDs = append(matchedIDs, id)
			}
		}
		if len(matchedIDs) > 0 {
			matches = append(matches, "sub.channel_id IN ?")
			outerArgs = append(outerArgs, matchedIDs)
		}
		outerWhere = append(outerWhere, attemptMatch("("+strings.Join(matches, " OR ")+")"))
	}
	whereClause := ""
	if len(outerWhere) > 0 {
		whereClause = "WHERE " + strings.Join(outerWhere, " AND ")
	}
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}
	joinCondition, createdColumn := "l.id = g.best_id", "g.created_at"
	if !grouped {
		joinCondition, createdColumn = "l.req_key = g.req_key", "l.created_at"
	}
	fromSQL := fmt.Sprintf(`
FROM grouped g JOIN scoped l ON %s
LEFT JOIN scoped initial ON initial.id = g.first_error_id
%s`, joinCondition, whereClause)
	pageCTE := cte
	fastPage := grouped && strings.TrimSpace(filter.Search) == "" && strings.TrimSpace(filter.KeyFilter) == "" && strings.TrimSpace(filter.ChannelFilter) == "" && strings.TrimSpace(filter.Model) == "" && strings.TrimSpace(filter.User) == "" && strings.TrimSpace(filter.Status) != "retry_recovered"
	if fastPage {
		window := max((page-1)*pageSize*10+pageSize*10, 500)
		scopeWhere := strings.Join(innerWhere, " AND ")
		pageCTE = strings.Replace(pageCTE, "FROM logs WHERE "+scopeWhere, fmt.Sprintf("FROM logs WHERE %s ORDER BY created_at DESC, id DESC LIMIT %d", scopeWhere, window), 1)
	}
	pageQuerySQL := pageCTE + fmt.Sprintf(`
	SELECT g.req_key, %s AS report_created_at,
  g.total_attempts, g.failed_attempts,
  initial.channel_id AS initial_channel_id, initial.content AS initial_error,
  l.id, l.type, l.content, l.prompt_tokens, l.completion_tokens, l.token_name, l.token_id,
	  l.model_name, l.username, l.quota, l.other, l.upstream_request_id, l.channel_id, l.use_time,
  l.request_id, l.ip, l.is_stream
%s ORDER BY %s DESC, l.id DESC LIMIT ? OFFSET ?`, createdColumn, fromSQL, createdColumn)
	type logRow struct {
		FullTotal         int
		ReqKey            string
		ReportCreatedAt   int64
		TotalAttempts     int
		FailedAttempts    int
		InitialChannelID  int
		InitialError      string
		ID                int
		Type              int
		Content           string
		PromptTokens      int
		CompletionTokens  int
		TokenName         string
		TokenID           int
		ModelName         string
		Username          string
		Quota             int
		Other             string
		UpstreamRequestID string
		ChannelID         int
		UseTime           int
		RequestID         string
		IP                string
		IsStream          bool
	}
	args := append(append([]any{}, innerArgs...), outerArgs...)
	var rows []logRow
	if err := model.LOG_DB.WithContext(ctx).Raw(pageQuerySQL, append(append([]any{}, args...), pageSize, (page-1)*pageSize)...).Scan(&rows).Error; err != nil {
		return report, err
	}
	countSQL := cte + fmt.Sprintf("SELECT COUNT(*) %s", fromSQL)
	if err := model.LOG_DB.WithContext(ctx).Raw(countSQL, args...).Scan(&report.Total).Error; err != nil {
		return report, err
	}

	// One batch for the visible requests gives ordered trails without N+1 queries.
	var requestKeys []string
	for _, row := range rows {
		requestKeys = append(requestKeys, row.ReqKey)
	}
	trails := make(map[string][]string)
	attemptsByRequest := make(map[string][]logRow)
	attemptNumbers := make(map[int]int)
	if len(requestKeys) > 0 {
		var attempts []logRow
		trailCTE := pageCTE
		trailArgs := append([]any{}, innerArgs...)
		var oldest, newest int64
		for _, row := range rows {
			if oldest == 0 || row.ReportCreatedAt < oldest {
				oldest = row.ReportCreatedAt
			}
			if row.ReportCreatedAt > newest {
				newest = row.ReportCreatedAt
			}
		}
		if oldest > 0 && newest > 0 {
			scopeWhere := strings.Join(innerWhere, " AND ")
			trailCTE = strings.Replace(trailCTE, "FROM logs WHERE "+scopeWhere, "FROM logs WHERE "+scopeWhere+" AND created_at BETWEEN ? AND ?", 1)
			trailArgs = append(trailArgs, oldest-300, newest+300)
		}
		trailArgs = append(trailArgs, requestKeys)
		if err := model.LOG_DB.WithContext(ctx).Raw(trailCTE+`
SELECT req_key, id, channel_id, type, created_at AS report_created_at,
 content, prompt_tokens, completion_tokens, token_name, token_id, model_name,
 quota, other, upstream_request_id, use_time, request_id, ip, is_stream
 FROM scoped WHERE req_key IN ? ORDER BY created_at ASC, id ASC`, trailArgs...).Scan(&attempts).Error; err != nil {
			return report, err
		}
		for _, attempt := range attempts {
			mark := "✗"
			if attempt.Type == model.LogTypeConsume {
				mark = "✓"
			}
			trails[attempt.ReqKey] = append(trails[attempt.ReqKey], channelName(attempt.ChannelID)+" "+mark)
			attemptNumbers[attempt.ID] = len(trails[attempt.ReqKey])
			attemptsByRequest[attempt.ReqKey] = append(attemptsByRequest[attempt.ReqKey], attempt)
		}
	}
	makeItem := func(row logRow) XPikiLogItem {
		status, errorCode := "success", ""
		if row.Type == model.LogTypeError {
			status, errorCode = "failed", row.Content
			if content := strings.ToLower(row.Content); strings.Contains(content, "cancel") || strings.Contains(content, "abort") {
				status = "canceled"
			}
		}
		var cacheUsage struct {
			Read        int    `json:"cache_tokens"`
			Write       int    `json:"cache_write_tokens"`
			Creation    int    `json:"cache_creation_tokens"`
			FiveMinutes int    `json:"cache_creation_tokens_5m"`
			OneHour     int    `json:"cache_creation_tokens_1h"`
			RequestPath string `json:"request_path"`
			AdminInfo   struct {
				UseChannel []string `json:"use_channel"`
			} `json:"admin_info"`
		}
		quotaSource := "unmatched"
		if row.Other != "" {
			if err := common.Unmarshal([]byte(row.Other), &cacheUsage); err != nil && row.UpstreamRequestID == "" {
				quotaSource = "unavailable"
				report.QuotaErrors = append(report.QuotaErrors, "Invalid local usage metadata for request "+row.RequestID)
			}
		}
		if cacheUsage.Write == 0 {
			cacheUsage.Write = cacheUsage.Creation
		}
		if cacheUsage.FiveMinutes > 0 || cacheUsage.OneHour > 0 {
			cacheUsage.Write = cacheUsage.FiveMinutes + cacheUsage.OneHour
		}
		tokenName := row.TokenName
		if tokenName == "" {
			tokenName = tokenNameByID[row.TokenID]
		}
		requestType := strings.TrimPrefix(cacheUsage.RequestPath, "/v1/")
		if requestType == cacheUsage.RequestPath {
			requestType = strings.TrimPrefix(requestType, "/")
		}
		if requestType == "" {
			requestType = "unknown"
		}
		item := XPikiLogItem{
			LogID: row.ID, CreatedAt: time.Unix(row.ReportCreatedAt, 0).Format(time.RFC3339),
			Type: requestType, RequestPath: cacheUsage.RequestPath, ModelSlug: row.ModelName, Status: status, ErrorCode: errorCode,
			InputTokens: row.PromptTokens, OutputTokens: row.CompletionTokens,
			CacheReadTokens: cacheUsage.Read, CacheWriteTokens: cacheUsage.Write,
			CreditCost: float64(row.Quota), UpstreamRequestID: row.UpstreamRequestID,
			QuotaSource: quotaSource,
			DurationMs:  row.UseTime * 1000, RequestID: row.RequestID, IP: row.IP,
			TokenName: tokenName, TokenID: row.TokenID, Username: row.Username, KeyLabel: tokenName, RequestOrigin: "newapi", LocalRequestID: row.RequestID,
			ChannelId: row.ChannelID, ChannelName: channelName(row.ChannelID), Destination: destination(row.ChannelID),
			TotalAttempts: row.TotalAttempts, FailedAttempts: row.FailedAttempts,
		}
		if len(cacheUsage.AdminInfo.UseChannel) > 1 {
			ids := make([]int, 0, len(cacheUsage.AdminInfo.UseChannel))
			for _, value := range cacheUsage.AdminInfo.UseChannel {
				id, err := strconv.Atoi(value)
				if err != nil || id <= 0 {
					ids = nil
					break
				}
				ids = append(ids, id)
			}
			// Only trust a complete trail ending at the recorded final channel.
			if len(ids) > 1 && ids[len(ids)-1] == row.ChannelID {
				item.TotalAttempts = max(item.TotalAttempts, len(ids))
				failed := len(ids)
				if row.Type == model.LogTypeConsume {
					failed--
				}
				item.FailedAttempts = max(item.FailedAttempts, failed)
				item.Retried = true
				item.InitialChannelId, item.InitialChannelName = ids[0], channelName(ids[0])
				item.InitialDestination = destination(ids[0])
				for i, id := range ids {
					mark := "✗"
					if i == len(ids)-1 && row.Type == model.LogTypeConsume {
						mark = "✓"
					}
					item.ChannelTrail = append(item.ChannelTrail, channelName(id)+" "+mark)
				}
			}
		}
		return item
	}
	for _, row := range rows {
		item := makeItem(row)
		if grouped {
			item.RequestOutcome = localRequestOutcome(row.Type, row.IsStream, row.Content, row.Other, item.FailedAttempts)
			if strings.TrimSpace(row.RequestID) == "" {
				item.RequestOutcome = "unknown"
			}
			if len(item.ChannelTrail) == 0 {
				item.ChannelTrail = trails[row.ReqKey]
			}
			item.Retried = item.Retried || row.TotalAttempts > 1
			if row.TotalAttempts > 1 {
				for _, attempt := range attemptsByRequest[row.ReqKey] {
					item.Attempts = append(item.Attempts, makeItem(attempt))
				}
			}
			if row.FailedAttempts > 0 && (item.InitialChannelId == 0 || item.InitialChannelId == row.InitialChannelID) {
				item.InitialChannelId, item.InitialChannelName = row.InitialChannelID, channelName(row.InitialChannelID)
				item.InitialDestination, item.InitialErrorCode = destination(row.InitialChannelID), row.InitialError
			}
		} else {
			item.AttemptNumber = attemptNumbers[row.ID]
		}
		report.Items = append(report.Items, item)
	}
	report.HasMore = page*pageSize < report.Total
	return report, nil
}

func fetchXPikiLogsReportAtURL(
	ctx context.Context,
	credentials []XPikiQuotaCredential,
	endpoint string,
	filter XPikiLogsFilter,
) (XPikiLogsReport, error) {
	report := XPikiLogsReport{Items: []XPikiLogItem{}, KeyOptions: []XPikiKeyOption{{Value: "all", Label: "Tất cả key XPiki"}}, Total: -1, Source: "xpiki"}
	page, pageSize := max(filter.Page, 1), filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 100)
	if page > (int(^uint(0)>>1)-1)/pageSize {
		return report, errors.New("invalid log page")
	}
	offset, needed := (page-1)*pageSize, page*pageSize+1
	var start, end int64
	bounds := []struct {
		value string
		upper bool
	}{
		{filter.StartDate, false}, {filter.EndDate, true},
	}
	if strings.TrimSpace(filter.Date) != "" {
		bounds[0].value, bounds[1].value = filter.Date, filter.Date
	}
	for _, bound := range bounds {
		if strings.TrimSpace(bound.value) == "" {
			continue
		}
		stamp, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(bound.value), time.Local)
		if err != nil {
			return report, errors.New("invalid log date: use YYYY-MM-DD")
		}
		if bound.upper {
			end = stamp.AddDate(0, 0, 1).Unix()
		} else {
			start = stamp.Unix()
		}
	}
	if start > 0 && end > 0 && start >= end {
		return report, errors.New("invalid log date range")
	}

	// One stream per distinct provider key; shared-channel aliases stay visible.
	var sources []XPikiQuotaCredential
	sourceIndex := make(map[string]int)
	for _, cred := range credentials {
		cred.APIKey = strings.TrimSpace(cred.APIKey)
		if cred.APIKey == "" || (cred.ChannelId > 0 && !IsXPikiBaseURL(cred.BaseURL)) {
			continue
		}
		aliases := append([]XPikiSourceChannel{}, cred.SourceChannels...)
		if cred.ChannelId > 0 && !slices.ContainsFunc(aliases, func(ch XPikiSourceChannel) bool { return ch.ID == cred.ChannelId }) {
			aliases = append(aliases, XPikiSourceChannel{ID: cred.ChannelId, Name: cred.ChannelName})
		}
		idx, exists := sourceIndex[cred.APIKey]
		if !exists {
			idx = len(sources)
			sourceIndex[cred.APIKey] = idx
			cred.SourceChannels = nil
			sources = append(sources, cred)
		}
		for _, alias := range aliases {
			if !slices.ContainsFunc(sources[idx].SourceChannels, func(ch XPikiSourceChannel) bool { return ch.ID == alias.ID }) {
				sources[idx].SourceChannels = append(sources[idx].SourceChannels, alias)
			}
		}
	}
	for i := range sources {
		slices.SortFunc(sources[i].SourceChannels, func(a, b XPikiSourceChannel) int { return a.ID - b.ID })
		var names []string
		for _, ch := range sources[i].SourceChannels {
			names = append(names, ch.Name)
		}
		if len(names) > 0 {
			sources[i].Label = strings.Join(names, " / ")
		}
		report.KeyOptions = append(report.KeyOptions, XPikiKeyOption{
			Value: xpikiSourceKeyID(sources[i].APIKey), Label: sources[i].Label,
			ChannelId: sources[i].ChannelId, ChannelName: sources[i].Label,
		})
	}
	keyFilter := strings.TrimSpace(filter.KeyFilter)
	tokenFilter := ""
	if strings.HasPrefix(keyFilter, "token:") {
		tokenFilter = strings.TrimPrefix(keyFilter, "token:")
	}
	if keyFilter != "" && keyFilter != "all" && tokenFilter == "" {
		sources = slices.DeleteFunc(sources, func(cred XPikiQuotaCredential) bool {
			if keyFilter == xpikiSourceKeyID(cred.APIKey) || keyFilter == MaskXPikiKey(cred.APIKey) || keyFilter == cred.Label {
				return false
			}
			return !slices.ContainsFunc(cred.SourceChannels, func(ch XPikiSourceChannel) bool {
				return keyFilter == fmt.Sprintf("channel:%d", ch.ID) || keyFilter == strconv.Itoa(ch.ID)
			})
		})
	}
	if len(sources) == 0 {
		report.Total = 0
		return report, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	type ledgerPage struct {
		items []XPikiLogItem
		more  bool
	}
	type stream struct {
		cred                      XPikiQuotaCredential
		cache                     map[int]ledgerPage
		reads, nextPage, position int
		items                     []XPikiLogItem
		more                      bool
		err                       error
		originErr                 error
		seenIDs                   map[string]bool
	}
	streams := make([]stream, len(sources))
	for i, cred := range sources {
		streams[i] = stream{cred: cred, cache: make(map[int]ledgerPage), seenIDs: make(map[string]bool), nextPage: 1, more: true}
	}
	loadPage := func(s *stream, number int) (ledgerPage, error) {
		if cached, ok := s.cache[number]; ok {
			return cached, nil
		}
		if s.reads >= 40 {
			return ledgerPage{}, errors.New("XPiki page budget reached; narrow the date/search filters")
		}
		s.reads++
		status := filter.Status
		if status == "canceled" {
			status = "all"
		}
		items, more, err := fetchXPikiLogsForCredential(ctx, s.cred, endpoint, number, 50, status, filter.Type)
		if err != nil {
			return ledgerPage{}, err
		}
		var previous int64
		for i, item := range items {
			stamp, err := time.Parse(time.RFC3339, item.CreatedAt)
			if err != nil {
				return ledgerPage{}, errors.New("invalid XPiki log timestamp")
			}
			if i > 0 && stamp.Unix() > previous {
				return ledgerPage{}, errors.New("XPiki logs are not ordered by time")
			}
			previous = stamp.Unix()
		}
		if len(items) == 0 && more {
			return ledgerPage{}, errors.New("XPiki returned an empty page with has_more=true")
		}
		value := ledgerPage{items, more}
		s.cache[number] = value
		return value, nil
	}
	load := func(s *stream) {
		p, err := loadPage(s, s.nextPage)
		if err != nil {
			s.err = err
			s.more = false
			s.items = nil
			return
		}
		s.items, s.position, s.more = p.items, 0, p.more
		s.nextPage++
		for _, item := range s.items {
			if item.RequestID == "" {
				continue
			}
			identity := item.RequestID + ":" + item.Type
			if s.seenIDs[identity] {
				s.err = errors.New("XPiki pagination changed while loading; refresh logs")
				s.more, s.items = false, nil
				return
			}
			s.seenIDs[identity] = true
		}
		sourceID := xpikiSourceKeyID(s.cred.APIKey)
		for i := range s.items {
			item := &s.items[i]
			item.SourceKeyID = sourceID
			item.SourceChannels = s.cred.SourceChannels
			item.RequestOrigin = "unmatched"
			item.TokenName = ""
			item.ChannelName = s.cred.Label
			item.ChannelId = 0
			if len(s.cred.SourceChannels) == 1 {
				item.ChannelId = s.cred.SourceChannels[0].ID
			}
			item.LogKey = fmt.Sprintf("%s:%s:%d:%d", sourceID, item.RequestID, s.nextPage-1, i)
		}
		if err := attachXPikiRequestOrigins(ctx, s.items); err != nil {
			// Do not discard provider rows when local attribution is unavailable.
			s.originErr = fmt.Errorf("local origin lookup failed: %w", err)
		}
	}
	// Seek older date windows rather than re-reading every newer provider page.
	prepare := func(s *stream) {
		if end > 0 {
			low, high := 1, 1
			for {
				p, err := loadPage(s, high)
				if err != nil {
					s.err = err
					s.more = false
					return
				}
				if len(p.items) == 0 {
					break
				}
				last, _ := time.Parse(time.RFC3339, p.items[len(p.items)-1].CreatedAt)
				if last.Unix() < end || !p.more {
					break
				}
				low, high = high+1, high*2
			}
			for low < high {
				mid := low + (high-low)/2
				p, err := loadPage(s, mid)
				if err != nil {
					s.err = err
					s.more = false
					return
				}
				if len(p.items) == 0 {
					high = mid
					continue
				}
				last, _ := time.Parse(time.RFC3339, p.items[len(p.items)-1].CreatedAt)
				if last.Unix() >= end {
					low = mid + 1
				} else {
					high = mid
				}
			}
			s.nextPage = low
		}
		load(s)
	}
	jobs := make(chan int, len(streams))
	for i := range streams {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for range min(xpikiMaxWorkers, len(streams)) {
		wg.Go(func() {
			for i := range jobs {
				prepare(&streams[i])
			}
		})
	}
	wg.Wait()
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	matches := 0
	for matches < needed {
		best := -1
		var newest int64
		for i := range streams {
			s := &streams[i]
			if s.position >= len(s.items) && s.more && s.err == nil {
				load(s)
			}
			if s.position >= len(s.items) {
				continue
			}
			stamp, _ := time.Parse(time.RFC3339, s.items[s.position].CreatedAt)
			if start > 0 && stamp.Unix() < start {
				s.position = len(s.items)
				s.more = false
				continue
			}
			if best < 0 || stamp.Unix() > newest {
				best, newest = i, stamp.Unix()
			}
		}
		if best < 0 {
			break
		}
		s := &streams[best]
		item := s.items[s.position]
		s.position++
		if end > 0 && newest >= end {
			continue
		}
		if tokenFilter != "" && item.TokenName != tokenFilter {
			continue
		}
		if filter.Status != "" && filter.Status != "all" && item.Status != filter.Status {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{item.RequestID, item.LocalRequestID, item.ModelSlug, item.IP, item.KeyLabel, item.ChannelName, item.TokenName, item.ErrorCode}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		if matches >= offset && len(report.Items) < pageSize {
			report.Items = append(report.Items, item)
		}
		matches++
	}
	for _, s := range streams {
		if s.originErr != nil {
			report.QuotaErrors = append(report.QuotaErrors, sanitizeXPikiText(s.cred.Label+": "+s.originErr.Error(), s.cred.APIKey, 240))
		}
		if s.err != nil {
			report.Incomplete = true
			report.QuotaErrors = append(report.QuotaErrors, sanitizeXPikiText(s.cred.Label+": "+s.err.Error(), s.cred.APIKey, 240))
		}
		if s.position < len(s.items) || s.more {
			report.HasMore = true
		}
	}
	if report.Incomplete {
		report.HasMore = true
	}
	if !report.HasMore {
		report.Total = matches
	}
	return report, nil
}

func splitLogFilterValues(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

type XPikiLogsReport struct {
	Source           string                 `json:"source,omitempty"`
	Incomplete       bool                   `json:"incomplete,omitempty"`
	Items            []XPikiLogItem         `json:"items"`
	Total            int                    `json:"total"`
	KeyOptions       []XPikiKeyOption       `json:"key_options"`
	ModelOptions     []string               `json:"model_options,omitempty"`
	UserOptions      []string               `json:"user_options,omitempty"`
	HasMore          bool                   `json:"has_more"`
	QuotaErrors      []string               `json:"quota_errors,omitempty"`
	TokenCostSummary *XPikiTokenCostSummary `json:"token_cost_summary,omitempty"`
}

type xpikiResponse struct {
	Code    *int            `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type xpikiConversion struct {
	GroupRatio json.RawMessage `json:"group_ratio"`
	Models     []struct {
		DisplayName      string          `json:"display_name"`
		Slug             string          `json:"slug"`
		InputPricePer1M  json.RawMessage `json:"input_price_per_1m"`
		OutputPricePer1M json.RawMessage `json:"output_price_per_1m"`
	} `json:"models"`
}

type xpikiStatusData struct {
	QuotaUsed   json.RawMessage `json:"quota_used"`
	QuotaLimit  json.RawMessage `json:"quota_limit"`
	Name        string          `json:"name"`
	KeyPrefix   string          `json:"key_prefix"`
	Enabled     *bool           `json:"enabled"`
	StatusFlags struct {
		Expired        bool `json:"expired"`
		QuotaExhausted bool `json:"quota_exhausted"`
	} `json:"status_flags"`
	Conversion xpikiConversion `json:"conversion"`
}

type xpikiPricedModel struct {
	name        string
	slug        string
	price       decimal.Decimal
	outputPrice decimal.Decimal
}

type xpikiCheckResult struct {
	key                 XPikiQuotaKeyResult
	profile             *XPikiQuotaModelProfile
	remainingAstraToken decimal.Decimal
	remainingVND        decimal.Decimal
	costKnown           bool
}
