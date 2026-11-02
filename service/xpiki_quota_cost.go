package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const xpikiCostMaxPages = 12

// Polling reads local logs every 10s. Immutable reconciled receipts can be
// reused; incomplete results are retained for a minute rather than every poll
// making the same provider lookup again.
var localRequestCostCache = struct {
	sync.Mutex
	entries map[[32]byte]localRequestCostEntry
}{entries: make(map[[32]byte]localRequestCostEntry)}

type localRequestCostEntry struct {
	cost    model.XPikiLogCost
	errors  []string
	expires time.Time
}

func enrichCachedLocalAttemptCosts(ctx context.Context, report *XPikiLogsReport, credentials []XPikiQuotaCredential, endpoint string, statusEndpoint ...string) {
	// Configuration is part of the digest; key changes must not reuse old costs.
	configuration := endpoint + strings.Join(statusEndpoint, "|")
	for _, cred := range credentials {
		configuration += fmt.Sprintf("|%d:%s:%s:%v", cred.ChannelId, cred.BaseURL, cred.APIKey, cred.SourceChannels)
	}
	configurationDigest := sha256.Sum256([]byte(configuration))
	pending := XPikiLogsReport{}
	var indices []int
	var keys [][32]byte
	now := time.Now()
	for i := range report.Items {
		item := &report.Items[i]
		if item.Destination != "api.xpiki.com" {
			item.QuotaSource = "not_xpiki"
			continue
		}
		key := sha256.Sum256([]byte(fmt.Sprintf("%x|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%s", configurationDigest,
			item.LogID, item.ChannelId, item.UpstreamRequestID, item.CreatedAt, item.Status, item.ModelSlug,
			item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens, item.QuotaSource)))
		localRequestCostCache.Lock()
		cached, ok := localRequestCostCache.entries[key]
		localRequestCostCache.Unlock()
		if ok && now.Before(cached.expires) {
			item.QuotaSource, item.UpstreamQuota, item.CostVND = cached.cost.Source, cached.cost.Quota, cached.cost.CostVND
			item.CostReferenceModel, item.ChargedReferenceTokens, item.TokenCost = cached.cost.ReferenceModel, cached.cost.ReferenceTokens, cached.cost.TokenCost
			item.OriginMatch = cached.cost.Match
			report.QuotaErrors = append(report.QuotaErrors, cached.errors...)
			continue
		}
		pending.Items = append(pending.Items, *item)
		indices, keys = append(indices, i), append(keys, key)
	}
	if len(pending.Items) == 0 {
		return
	}
	enrichXPikiLogCosts(ctx, &pending, credentials, endpoint, statusEndpoint...)
	report.QuotaErrors = append(report.QuotaErrors, pending.QuotaErrors...)
	for i, item := range pending.Items {
		report.Items[indices[i]] = item
		if ctx.Err() != nil {
			continue
		}
		ttl := time.Minute
		if item.TokenCost != nil && (item.TokenCost.Status == "reconciled" || item.TokenCost.Status == "confirmed_zero") {
			ttl = 10 * time.Minute
		}
		entry := localRequestCostEntry{cost: model.XPikiLogCost{Source: item.QuotaSource, Quota: item.UpstreamQuota, CostVND: item.CostVND,
			ReferenceModel: item.CostReferenceModel, ReferenceTokens: item.ChargedReferenceTokens, TokenCost: item.TokenCost, Match: item.OriginMatch},
			errors: pending.QuotaErrors, expires: time.Now().Add(ttl)}
		localRequestCostCache.Lock()
		for key, value := range localRequestCostCache.entries {
			if now.After(value.expires) {
				delete(localRequestCostCache.entries, key)
			}
		}
		if len(localRequestCostCache.entries) >= 2048 {
			for key := range localRequestCostCache.entries {
				delete(localRequestCostCache.entries, key)
				break
			}
		}
		localRequestCostCache.entries[keys[i]] = entry
		localRequestCostCache.Unlock()
	}
	report.QuotaErrors = slices.Compact(report.QuotaErrors)
}

// The customer request is the row identity. Reconcile all its attempts, not
// just the winner: an XPiki attempt can incur cost before fallback to 9router.
func enrichLocalRequestCosts(ctx context.Context, report *XPikiLogsReport, credentials []XPikiQuotaCredential, endpoint string, statusEndpoint ...string) {
	batch := XPikiLogsReport{}
	for _, item := range report.Items {
		if len(item.Attempts) > 0 {
			batch.Items = append(batch.Items, item.Attempts...)
		} else {
			batch.Items = append(batch.Items, item)
		}
	}
	enrichCachedLocalAttemptCosts(ctx, &batch, credentials, endpoint, statusEndpoint...)
	report.QuotaErrors = append(report.QuotaErrors, batch.QuotaErrors...)
	index := 0
	for i := range report.Items {
		item := &report.Items[i]
		if len(item.Attempts) == 0 {
			*item = batch.Items[index]
			index++
			continue
		}
		item.Attempts = batch.Items[index : index+len(item.Attempts)]
		index += len(item.Attempts)
		item.TokenCost, item.QuotaSource = aggregateRequestAttemptCosts(item.Attempts)
	}
}

func aggregateRequestAttemptCosts(attempts []XPikiLogItem) (*model.XPikiTokenCost, string) {
	var costs []*model.XPikiTokenCost
	for _, attempt := range attempts {
		if attempt.QuotaSource == "not_xpiki" {
			continue
		}
		cost := attempt.TokenCost
		if cost == nil || (cost.Status != "reconciled" && cost.Status != "confirmed_zero") {
			return &model.XPikiTokenCost{Status: "unverified", Warning: "Chưa đối soát đủ giá vốn các lượt XPiki của request; xem chi tiết từng lượt"}, "unmatched"
		}
		costs = append(costs, cost)
	}
	if len(costs) == 0 {
		return nil, "not_xpiki"
	}
	if len(costs) == 1 {
		return costs[0], "xpiki"
	}
	total := decimal.Zero
	for _, cost := range costs {
		amount, err := decimal.NewFromString(cost.TotalVND)
		if err != nil || amount.IsNegative() {
			return &model.XPikiTokenCost{Status: "unverified", Warning: "Giá vốn một lượt XPiki không hợp lệ"}, "unmatched"
		}
		total = total.Add(amount)
	}
	status := "reconciled"
	if total.IsZero() {
		status = "confirmed_zero"
	}
	return &model.XPikiTokenCost{Status: status, TotalVND: total.String(), CostVND: formatXPikiNumber(total) + " ₫"}, "xpiki"
}

func EnrichUsageLogXPikiCosts(ctx context.Context, logs []*model.Log, credentials []XPikiQuotaCredential) {
	enrichUsageLogXPikiCostsAtURL(ctx, logs, credentials, XPikiKeyLogsURL, XPikiKeyStatusURL)
}

func EnrichUsageLogXPikiCostsCachedOnly(logs []*model.Log, credentials []XPikiQuotaCredential) {
	enrichUsageLogXPikiCostsCachedOnlyAtURL(logs, credentials, XPikiKeyLogsURL, XPikiKeyStatusURL)
}

func enrichUsageLogXPikiCostsCachedOnlyAtURL(logs []*model.Log, credentials []XPikiQuotaCredential, endpoint string, statusEndpoint ...string) {
	allowed := make(map[int]bool)
	for _, cred := range credentials {
		if !IsXPikiBaseURL(cred.BaseURL) {
			continue
		}
		allowed[cred.ChannelId] = true
		for _, alias := range cred.SourceChannels {
			allowed[alias.ID] = true
		}
	}
	configuration := endpoint + strings.Join(statusEndpoint, "|")
	for _, cred := range credentials {
		configuration += fmt.Sprintf("|%d:%s:%s:%v", cred.ChannelId, cred.BaseURL, cred.APIKey, cred.SourceChannels)
	}
	configurationDigest := sha256.Sum256([]byte(configuration))
	now := time.Now()

	for _, log := range logs {
		log.XPikiCost = &model.XPikiLogCost{Source: "not_xpiki"}
		if log.Type != model.LogTypeConsume && log.Type != model.LogTypeError {
			log.XPikiCost.Source = "not_applicable"
			continue
		}
		if !allowed[log.ChannelId] {
			continue
		}
		var usage struct {
			Read        int `json:"cache_tokens"`
			Write       int `json:"cache_write_tokens"`
			Creation    int `json:"cache_creation_tokens"`
			FiveMinutes int `json:"cache_creation_tokens_5m"`
			OneHour     int `json:"cache_creation_tokens_1h"`
		}
		source := "unmatched"
		if log.Other != "" && common.Unmarshal([]byte(log.Other), &usage) != nil && log.UpstreamRequestId == "" {
			source = "unavailable"
		}
		write := usage.Write
		if write == 0 {
			write = usage.Creation
		}
		if usage.FiveMinutes > 0 || usage.OneHour > 0 {
			write = usage.FiveMinutes + usage.OneHour
		}
		status := "success"
		if log.Type == model.LogTypeError {
			status = "failed"
		}
		item := XPikiLogItem{
			LogID: log.Id, ChannelId: log.ChannelId, Destination: "api.xpiki.com",
			CreatedAt: time.Unix(log.CreatedAt, 0).Format(time.RFC3339), Status: status,
			ModelSlug: log.ModelName, InputTokens: log.PromptTokens, OutputTokens: log.CompletionTokens,
			CacheReadTokens: usage.Read, CacheWriteTokens: write, UpstreamRequestID: log.UpstreamRequestId,
			QuotaSource: source,
		}
		key := sha256.Sum256([]byte(fmt.Sprintf("%x|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%s", configurationDigest,
			item.LogID, item.ChannelId, item.UpstreamRequestID, item.CreatedAt, item.Status, item.ModelSlug,
			item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.CacheWriteTokens, item.QuotaSource)))

		localRequestCostCache.Lock()
		cached, ok := localRequestCostCache.entries[key]
		localRequestCostCache.Unlock()

		if ok && now.Before(cached.expires) {
			costCopy := cached.cost
			log.XPikiCost = &costCopy
		} else {
			log.XPikiCost = &model.XPikiLogCost{Source: "loading"}
		}
	}
}

func enrichUsageLogXPikiCostsAtURL(ctx context.Context, logs []*model.Log, credentials []XPikiQuotaCredential, endpoint string, statusEndpoint ...string) {
	allowed := make(map[int]bool)
	for _, cred := range credentials {
		if !IsXPikiBaseURL(cred.BaseURL) {
			continue
		}
		allowed[cred.ChannelId] = true
		for _, alias := range cred.SourceChannels {
			allowed[alias.ID] = true
		}
	}
	report := XPikiLogsReport{}
	indices := make([]int, 0, len(logs))
	for i, log := range logs {
		log.XPikiCost = &model.XPikiLogCost{Source: "not_xpiki"}
		if log.Type != model.LogTypeConsume && log.Type != model.LogTypeError {
			log.XPikiCost.Source = "not_applicable"
			continue
		}
		if !allowed[log.ChannelId] {
			continue
		}
		var usage struct {
			Read        int `json:"cache_tokens"`
			Write       int `json:"cache_write_tokens"`
			Creation    int `json:"cache_creation_tokens"`
			FiveMinutes int `json:"cache_creation_tokens_5m"`
			OneHour     int `json:"cache_creation_tokens_1h"`
		}
		source := "unmatched"
		if log.Other != "" && common.Unmarshal([]byte(log.Other), &usage) != nil && log.UpstreamRequestId == "" {
			source = "unavailable"
		}
		write := usage.Write
		if write == 0 {
			write = usage.Creation
		}
		if usage.FiveMinutes > 0 || usage.OneHour > 0 {
			write = usage.FiveMinutes + usage.OneHour
		}
		status := "success"
		if log.Type == model.LogTypeError {
			status = "failed"
		}
		report.Items = append(report.Items, XPikiLogItem{LogID: log.Id, ChannelId: log.ChannelId, Destination: "api.xpiki.com",
			CreatedAt: time.Unix(log.CreatedAt, 0).Format(time.RFC3339), Status: status,
			ModelSlug: log.ModelName, InputTokens: log.PromptTokens, OutputTokens: log.CompletionTokens,
			CacheReadTokens: usage.Read, CacheWriteTokens: write, UpstreamRequestID: log.UpstreamRequestId,
			QuotaSource: source})
		indices = append(indices, i)
	}
	enrichCachedLocalAttemptCosts(ctx, &report, credentials, endpoint, statusEndpoint...)
	for i, row := range report.Items {
		cost := &model.XPikiLogCost{Source: row.QuotaSource, Quota: row.UpstreamQuota, CostVND: row.CostVND, Match: row.OriginMatch,
			ReferenceModel: row.CostReferenceModel, ReferenceTokens: row.ChargedReferenceTokens, TokenCost: row.TokenCost}
		if len(report.QuotaErrors) > 0 && cost.Source != "xpiki" {
			cost.Source = "unavailable"
			cost.Warning = strings.Join(report.QuotaErrors, "; ")
		}
		logs[indices[i]].XPikiCost = cost
	}
}

// Local quota is the user's NewAPI bill, not the provider's deduction. Only
// attach an Astra-equivalent cost after matching an actual XPiki ledger item.
func enrichXPikiLogCosts(ctx context.Context, report *XPikiLogsReport, credentials []XPikiQuotaCredential, endpoint string, statusEndpoint ...string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	targets := make([]XPikiQuotaCredential, 0, len(credentials))
	var newest, oldest time.Time
	for i := range report.Items {
		item := &report.Items[i]
		item.QuotaAstra, item.UpstreamQuota, item.CostVND = "", "", ""
		item.CostReferenceModel, item.ChargedReferenceTokens = "", ""
		item.TokenCost = nil
		if item.Destination != "api.xpiki.com" {
			item.QuotaSource = "not_xpiki"
			continue
		}
		if item.QuotaSource == "unavailable" {
			continue
		}
		item.QuotaSource = "unmatched"
		stamp, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			continue
		}
		if newest.IsZero() || stamp.After(newest) {
			newest = stamp
		}
		if oldest.IsZero() || stamp.Before(oldest) {
			oldest = stamp
		}
	}
	if newest.IsZero() {
		return
	}
	for _, credential := range credentials {
		for _, item := range report.Items {
			if item.QuotaSource == "unmatched" && item.Destination == "api.xpiki.com" &&
				(item.ChannelId == credential.ChannelId || slices.ContainsFunc(credential.SourceChannels, func(ch XPikiSourceChannel) bool { return ch.ID == item.ChannelId })) {
				targets = append(targets, credential)
				break
			}
		}
	}
	type result struct {
		items []XPikiLogItem
		err   error
	}
	results := make([]result, len(targets))
	jobs := make(chan int, len(targets))
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for range min(xpikiMaxWorkers, len(targets)) {
		wg.Go(func() {
			for i := range jobs {
				results[i].items, results[i].err = fetchXPikiCostRange(ctx, targets[i], endpoint, newest, oldest)
				if len(statusEndpoint) > 0 && len(results[i].items) > 0 {
					enrichXPikiCredentialTokenCosts(ctx, targets[i], statusEndpoint[0], results[i].items)
				}
			}
		})
	}
	wg.Wait()
	var upstream []XPikiLogItem
	for i, result := range results {
		channelIDs := []int{targets[i].ChannelId}
		for _, alias := range targets[i].SourceChannels {
			if !slices.Contains(channelIDs, alias.ID) {
				channelIDs = append(channelIDs, alias.ID)
			}
		}
		for _, provider := range result.items {
			for _, id := range channelIDs {
				copy := provider
				copy.ChannelId = id
				upstream = append(upstream, copy)
			}
		}
		if result.err != nil {
			report.QuotaErrors = append(report.QuotaErrors, fmt.Sprintf("#%d: %s", targets[i].ChannelId,
				sanitizeXPikiText(result.err.Error(), targets[i].APIKey, 200)))
		}
	}
	attachMatchedXPikiCosts(report.Items, upstream)
}

// Seek by timestamp so historical searches do not scan every page. Each key
// has a fixed request budget and all keys share the report's deadline.
func fetchXPikiCostRange(ctx context.Context, credential XPikiQuotaCredential, endpoint string, newest, oldest time.Time) ([]XPikiLogItem, error) {
	type pageResult struct {
		items []XPikiLogItem
		more  bool
	}
	pages := map[int]pageResult{}
	load := func(page int) (pageResult, error) {
		if saved, ok := pages[page]; ok {
			return saved, nil
		}
		if len(pages) >= xpikiCostMaxPages {
			return pageResult{}, errors.New("upstream quota lookup page limit reached")
		}
		items, more, err := fetchXPikiLogsForCredential(ctx, credential, endpoint, page, 50, "all", "all")
		if err != nil {
			return pageResult{}, err
		}
		saved := pageResult{items, more}
		pages[page] = saved
		return saved, nil
	}
	upperTime, lowerTime := newest.Add(time.Second), oldest.Add(-time.Second)
	low, high := 1, 1
	for {
		page, err := load(high)
		if err != nil {
			return nil, err
		}
		if len(page.items) == 0 {
			break
		}
		last, err := time.Parse(time.RFC3339, page.items[len(page.items)-1].CreatedAt)
		if err != nil {
			return nil, errors.New("invalid upstream log timestamp")
		}
		if !last.After(upperTime) || !page.more {
			break
		}
		low, high = high+1, high*2
	}
	for low < high {
		mid := low + (high-low)/2
		page, err := load(mid)
		if err != nil {
			return nil, err
		}
		if len(page.items) == 0 {
			high = mid
			continue
		}
		last, err := time.Parse(time.RFC3339, page.items[len(page.items)-1].CreatedAt)
		if err != nil {
			return nil, errors.New("invalid upstream log timestamp")
		}
		if last.After(upperTime) {
			low = mid + 1
		} else {
			high = mid
		}
	}
	var matched []XPikiLogItem
	for pageNo := low; ; pageNo++ {
		page, err := load(pageNo)
		if err != nil {
			return matched, err
		}
		stop := !page.more || len(page.items) == 0
		for _, item := range page.items {
			stamp, err := time.Parse(time.RFC3339, item.CreatedAt)
			if err != nil {
				return matched, errors.New("invalid upstream log timestamp")
			}
			if stamp.Before(lowerTime) {
				stop = true
			} else if !stamp.After(upperTime) {
				matched = append(matched, item)
			}
		}
		if stop {
			return matched, nil
		}
	}
}

func attachMatchedXPikiCosts(local, upstream []XPikiLogItem) {
	matches := make(map[int]int)
	used := make(map[string]int)
	for i, item := range local {
		if item.QuotaSource != "unmatched" {
			continue
		}
		candidate := -1
		for j, provider := range upstream {
			if !xpikiCostMatches(item, provider) {
				continue
			}
			if candidate >= 0 {
				candidate = -1
				break
			}
			candidate = j
		}
		if candidate >= 0 {
			matches[i] = candidate
			used[upstream[candidate].RequestID]++
		}
	}
	for i, candidate := range matches {
		provider := upstream[candidate]
		if used[provider.RequestID] != 1 {
			continue
		}
		local[i].QuotaAstra = provider.QuotaAstra
		local[i].UpstreamQuota = provider.UpstreamQuota
		local[i].CostVND = provider.CostVND
		local[i].CostReferenceModel = provider.CostReferenceModel
		local[i].ChargedReferenceTokens = provider.ChargedReferenceTokens
		local[i].TokenCost = provider.TokenCost
		local[i].QuotaSource = "xpiki"
		local[i].OriginMatch = "usage_time"
		if local[i].UpstreamRequestID != "" {
			local[i].OriginMatch = "request_id"
		}
	}
}

func xpikiCostMatches(local, provider XPikiLogItem) bool {
	if local.ChannelId != provider.ChannelId || provider.QuotaSource != "xpiki" || provider.RequestID == "" {
		return false
	}
	if local.UpstreamRequestID != "" {
		return local.UpstreamRequestID == provider.RequestID
	}
	// Older local logs have no provider ID. Require a unique successful event
	// with matching usage and end time; never guess the cost of zero-usage errors.
	if local.Status != "success" || provider.Status != "success" || local.ModelSlug != provider.ModelSlug ||
		(local.InputTokens == 0 && local.OutputTokens == 0) ||
		local.OutputTokens != provider.OutputTokens || local.CacheReadTokens != provider.CacheReadTokens ||
		local.CacheWriteTokens != provider.CacheWriteTokens {
		return false
	}
	// NewAPI prompt tokens include cache; XPiki's ledger input_tokens is the
	// uncached portion. Normalize only for matching, never for display/billing.
	// Subtract with bounds checks to reject invalid metadata without overflow.
	uncachedInput := local.InputTokens
	if uncachedInput < 0 || provider.InputTokens < 0 || local.OutputTokens < 0 {
		return false
	}
	for _, cacheTokens := range []int{local.CacheReadTokens, local.CacheWriteTokens} {
		if cacheTokens < 0 || cacheTokens > uncachedInput {
			return false
		}
		uncachedInput -= cacheTokens
	}
	if uncachedInput != provider.InputTokens {
		// When upstream XPiki charges 0 input tokens (common for tiny test/admin prompts <= 50 tokens),
		// allow matching if model, completion tokens and timestamp match strictly.
		if !(provider.InputTokens == 0 && uncachedInput <= 50) {
			return false
		}
	}
	localTime, err := time.Parse(time.RFC3339, local.CreatedAt)
	if err != nil {
		return false
	}
	providerTime, err := time.Parse(time.RFC3339, provider.CreatedAt)
	if err != nil {
		return false
	}
	delta := localTime.Sub(providerTime)
	return delta >= -time.Second && delta <= time.Second
}
