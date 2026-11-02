package service

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
)

type xpikiTariffHistory struct {
	items     []XPikiLogItem
	warning   string
	checkedAt time.Time
}

// Deep pricing checks are admin-only, but repeated reloads must not exhaust
// XPiki's read limit. Bound memory and coalesce concurrent checks of one key.
var xpikiTariffCache = struct {
	sync.Mutex
	entries map[string]xpikiTariffHistory
}{entries: make(map[string]xpikiTariffHistory)}
var xpikiTariffLoads singleflight.Group

type XPikiTokenCostSummary struct {
	KnownRequests   int    `json:"known_requests"`
	UnknownRequests int    `json:"unknown_requests"`
	TotalVND        string `json:"total_vnd"`
	CostVND         string `json:"cost_vnd"`
}

func normalizedXPikiModel(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slash := strings.LastIndex(slug, "/"); slash >= 0 {
		slug = slug[slash+1:]
	}
	if slug == "claude-opus-5.5" {
		return xpikiOpusModelSlug
	}
	return slug
}

// Calculate from actual token components and the provider's model ratios.
// The receipt is used only to reconcile the request-specific multiplier and
// provider rounding, not as the monetary calculation's input. XPiki does not
// publish that multiplier or cache-write tariffs in its public conversion API.
func calculateXPikiTokenCost(item XPikiLogItem, conversion xpikiConversion) *model.XPikiTokenCost {
	result := &model.XPikiTokenCost{Status: "unverified", InputTokens: item.InputTokens,
		OutputTokens: item.OutputTokens, CacheReadTokens: item.CacheReadTokens, CacheWriteTokens: item.CacheWriteTokens}
	unknown := func(message string) *model.XPikiTokenCost {
		result.Warning = message
		return result
	}
	if item.QuotaSource != "xpiki" {
		return unknown("Chưa có biên lai XPiki để đối soát token")
	}
	fee, err := decimal.NewFromString(item.UpstreamQuota)
	if err != nil || fee.IsNegative() {
		return unknown("Phí XPiki không hợp lệ")
	}
	if fee.IsZero() {
		result.Status, result.TotalVND, result.CostVND = "confirmed_zero", "0", "0 ₫"
		result.Warning = "Biên lai XPiki xác nhận không thu phí; không suy ra đơn giá từ request này"
		return result
	}
	if item.InputTokens < 0 || item.OutputTokens < 0 || item.CacheReadTokens < 0 || item.CacheWriteTokens < 0 || item.mediaUsage {
		return unknown("Token không hợp lệ hoặc có phí ảnh/video chưa được xác minh")
	}
	if item.CacheWriteTokens > 0 {
		return unknown("Chưa có đơn giá cache-write được XPiki xác nhận; không coi là miễn phí")
	}
	if item.CacheReadTokens > 0 && !xpikiVerifiedCacheReadModel(item.ModelSlug) {
		return unknown("Chưa xác minh đơn giá cache-read của model này")
	}
	reference := xpikiCostReferenceModel(item.ModelSlug)
	var anchorUnits, anchorPrice int64
	switch reference {
	case xpikiAstraModelSlug:
		anchorUnits, anchorPrice = xpikiAstraQuotaPer1M, xpikiAstraPriceVNDPer1M
	case xpikiOpusModelSlug:
		anchorUnits, anchorPrice = xpikiOpusQuotaPer1M, xpikiOpusPriceVNDPer1M
	default:
		return unknown("Chưa xác định mốc mua Astra/Opus cho model này")
	}
	group, err := parseXPikiDecimal(conversion.GroupRatio)
	if err != nil || group.Sign() <= 0 {
		return unknown("Chưa tải được tỷ lệ giá model của key XPiki")
	}
	var inputRate, outputRate decimal.Decimal
	found := false
	for _, entry := range conversion.Models {
		if normalizedXPikiModel(entry.Slug) != normalizedXPikiModel(item.ModelSlug) {
			continue
		}
		var inputErr, outputErr error
		inputRate, inputErr = parseXPikiDecimal(entry.InputPricePer1M)
		outputRate, outputErr = parseXPikiDecimal(entry.OutputPricePer1M)
		found = inputErr == nil && outputErr == nil && inputRate.Sign() > 0 && !outputRate.IsNegative()
		break
	}
	if !found {
		return unknown("Chưa có tỷ lệ input/output hợp lệ cho model này")
	}
	inputRate, outputRate = inputRate.Mul(group), outputRate.Mul(group)
	cacheRate := inputRate.Div(decimal.NewFromInt(2))
	million := decimal.NewFromInt(1000000)
	base := inputRate.Mul(decimal.NewFromInt(int64(item.InputTokens))).
		Add(outputRate.Mul(decimal.NewFromInt(int64(item.OutputTokens)))).
		Add(cacheRate.Mul(decimal.NewFromInt(int64(item.CacheReadTokens)))).DivRound(million, 64)
	settledBase := base.Floor()
	if settledBase.Sign() <= 0 {
		return unknown("Token chưa đủ để xác định hệ số của khoản thu này")
	}
	multiplier := fee.DivRound(settledBase, 28)
	if !multiplier.Equal(multiplier.Floor()) || multiplier.Sign() <= 0 || multiplier.GreaterThan(decimal.NewFromInt(100)) || !settledBase.Mul(multiplier).Equal(fee) {
		return unknown("Phí XPiki không khớp công thức token; không dùng giá ước tính để tính lời/lỗ")
	}
	// Normalize each component against its own purchase anchor. Quantize once
	// before the multiplier, matching XPiki's per-request floor settlement.
	toPrice := func(rate decimal.Decimal) decimal.Decimal {
		return rate.DivRound(decimal.NewFromInt(anchorUnits), 64).Mul(decimal.NewFromInt(anchorPrice)).Mul(multiplier)
	}
	inputPrice, outputPrice, cachePrice := toPrice(inputRate), toPrice(outputRate), toPrice(cacheRate)
	inputCost := inputPrice.Mul(decimal.NewFromInt(int64(item.InputTokens))).DivRound(million, 64)
	outputCost := outputPrice.Mul(decimal.NewFromInt(int64(item.OutputTokens))).DivRound(million, 64)
	cacheCost := cachePrice.Mul(decimal.NewFromInt(int64(item.CacheReadTokens))).DivRound(million, 64)
	total := settledBase.Mul(multiplier).DivRound(decimal.NewFromInt(anchorUnits), 64).Mul(decimal.NewFromInt(anchorPrice))
	result.Status, result.ReferenceModel, result.Multiplier = "reconciled", reference, multiplier.String()
	result.InputVNDPer1M, result.OutputVNDPer1M = inputPrice.Round(12).String(), outputPrice.Round(12).String()
	if xpikiVerifiedCacheReadModel(item.ModelSlug) {
		result.CacheReadVNDPer1M = cachePrice.Round(12).String()
	}
	result.InputVND, result.OutputVND, result.CacheReadVND = inputCost.Round(12).String(), outputCost.Round(12).String(), cacheCost.Round(12).String()
	result.RoundingVND = total.Sub(inputCost.Add(outputCost).Add(cacheCost)).Round(12).String()
	result.TotalVND, result.CostVND = total.Round(12).String(), formatXPikiNumber(total.Round(4))+" ₫"
	return result
}

func enrichXPikiCredentialTokenCosts(ctx context.Context, cred XPikiQuotaCredential, endpoint string, items []XPikiLogItem) {
	checked := checkXPikiCredential(ctx, cred, endpoint)
	conversion := xpikiConversion{}
	if checked.profile != nil {
		conversion = checked.profile.conversion
	}
	for i := range items {
		items[i].TokenCost = calculateXPikiTokenCost(items[i], conversion)
		if checked.key.Error != "" && items[i].TokenCost.Status == "unverified" {
			items[i].TokenCost.Warning = "Không tải được bảng tỷ lệ XPiki: " + checked.key.Error
		}
	}
}

func enrichXPikiReportTokenCosts(ctx context.Context, report *XPikiLogsReport, credentials []XPikiQuotaCredential, endpoint string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type group struct {
		cred    XPikiQuotaCredential
		items   []XPikiLogItem
		indices []int
	}
	groups := make([]group, 0, len(credentials))
	for _, cred := range credentials {
		g := group{cred: cred}
		for i, row := range report.Items {
			if row.SourceKeyID == xpikiSourceKeyID(cred.APIKey) {
				g.items, g.indices = append(g.items, row), append(g.indices, i)
			}
		}
		if len(g.items) > 0 {
			groups = append(groups, g)
		}
	}
	jobs := make(chan int, len(groups))
	for i := range groups {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(xpikiMaxWorkers, len(groups)) {
		workers.Go(func() {
			for index := range jobs {
				g := &groups[index]
				enrichXPikiCredentialTokenCosts(ctx, g.cred, endpoint, g.items)
			}
		})
	}
	workers.Wait()
	for _, g := range groups {
		for i, index := range g.indices {
			report.Items[index].TokenCost = g.items[i].TokenCost
		}
	}
	summarizeXPikiTokenCosts(report)
}

func summarizeXPikiTokenCosts(report *XPikiLogsReport) {
	summary := &XPikiTokenCostSummary{}
	total := decimal.Zero
	for _, row := range report.Items {
		if row.QuotaSource == "not_xpiki" {
			continue
		}
		cost := row.TokenCost
		if cost == nil || (cost.Status != "reconciled" && cost.Status != "confirmed_zero") {
			summary.UnknownRequests++
			continue
		}
		amount, err := decimal.NewFromString(cost.TotalVND)
		if err != nil || amount.IsNegative() {
			summary.UnknownRequests++
			continue
		}
		summary.KnownRequests++
		total = total.Add(amount)
	}
	summary.TotalVND, summary.CostVND = total.Round(12).String(), formatXPikiNumber(total.Round(4))+" ₫"
	report.TokenCostSummary = summary
}

// Observed rates belong to this key and this sample window, never to every
// model in the same family. Different settled multipliers are separate tariffs.
func loadXPikiTariffHistory(ctx context.Context, cred XPikiQuotaCredential, endpoint string) xpikiTariffHistory {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	history := xpikiTariffHistory{checkedAt: time.Now()}
	seen := make(map[string]bool)
	for page := 1; page <= 20; page++ {
		if endpoint == XPikiKeyLogsURL && page > 1 {
			timer := time.NewTimer(time.Second)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				history.warning = "Lịch sử đối soát chưa đầy đủ: hết thời gian tải."
				return history
			}
		}
		rows, more, err := fetchXPikiLogsForCredential(ctx, cred, endpoint, page, 50, "success", "all")
		if err != nil {
			history.warning = "Lịch sử đối soát chưa đầy đủ: " + sanitizeXPikiText(err.Error(), cred.APIKey, 160)
			break
		}
		for _, row := range rows {
			identity := row.RequestID + "|" + row.CreatedAt
			if seen[identity] {
				continue
			}
			seen[identity] = true
			history.items = append(history.items, row)
		}
		if !more || len(rows) == 0 {
			break
		}
		if page == 20 {
			history.warning = "Đã đối soát tối đa 1.000 biên lai gần nhất/key; chưa phải toàn bộ lịch sử."
		}
	}
	return history
}

func cachedXPikiTariffHistory(ctx context.Context, cred XPikiQuotaCredential, endpoint string) xpikiTariffHistory {
	if endpoint != XPikiKeyLogsURL {
		return loadXPikiTariffHistory(ctx, cred, endpoint)
	}
	key := xpikiSourceKeyID(cred.APIKey)
	result := xpikiTariffLoads.DoChan(key, func() (any, error) {
		xpikiTariffCache.Lock()
		saved, ok := xpikiTariffCache.entries[key]
		ttl := 10 * time.Minute
		if (saved.warning != "" && !strings.HasPrefix(saved.warning, "Đã đối soát")) || len(saved.items) == 0 {
			ttl = time.Minute
		}
		if ok && time.Since(saved.checkedAt) < ttl {
			xpikiTariffCache.Unlock()
			return saved, nil
		}
		xpikiTariffCache.Unlock()
		history := loadXPikiTariffHistory(ctx, cred, endpoint)
		xpikiTariffCache.Lock()
		if len(xpikiTariffCache.entries) >= 32 {
			oldestKey := ""
			var oldest time.Time
			for candidate, entry := range xpikiTariffCache.entries {
				if oldestKey == "" || entry.checkedAt.Before(oldest) {
					oldestKey, oldest = candidate, entry.checkedAt
				}
			}
			delete(xpikiTariffCache.entries, oldestKey)
		}
		xpikiTariffCache.entries[key] = history
		xpikiTariffCache.Unlock()
		return history, nil
	})
	select {
	case loaded := <-result:
		return loaded.Val.(xpikiTariffHistory)
	case <-ctx.Done():
		return xpikiTariffHistory{warning: "Lịch sử đối soát chưa đầy đủ: hết thời gian chờ.", checkedAt: time.Now()}
	}
}

func verifyXPikiEffectiveModelPrices(ctx context.Context, cred XPikiQuotaCredential, endpoint string, profile *XPikiQuotaModelProfile) {
	history := cachedXPikiTariffHistory(ctx, cred, endpoint)
	profile.PricingWarning = history.warning
	profile.PricingCheckedAt = history.checkedAt.Format(time.RFC3339)
	for i := range profile.Models {
		row := &profile.Models[i]
		row.ObservedTariffs = nil
		row.EffectiveInputVNDPer1M, row.EffectiveOutputVNDPer1M, row.EffectiveCacheReadVNDPer1M = "", "", ""
		row.ObservedMultiplier, row.VerifiedAt, row.VerifiedSamples, row.UnverifiedSamples = "", "", 0, 0
		tariffs := make(map[string]XPikiObservedTariff)
		for _, item := range history.items {
			if item.Status != "success" || normalizedXPikiModel(item.ModelSlug) != normalizedXPikiModel(row.ModelSlug) {
				continue
			}
			cost := calculateXPikiTokenCost(item, profile.conversion)
			if cost.Status == "confirmed_zero" {
				continue
			}
			if cost.Status != "reconciled" {
				row.UnverifiedSamples++
				continue
			}
			tariff := tariffs[cost.Multiplier]
			if tariff.Samples == 0 {
				tariff = XPikiObservedTariff{Multiplier: cost.Multiplier, InputVNDPer1M: cost.InputVNDPer1M,
					OutputVNDPer1M: cost.OutputVNDPer1M, LatestAt: item.CreatedAt}
			}
			tariff.Samples++
			if item.CacheReadTokens > 0 && cost.CacheReadVNDPer1M != "" {
				tariff.CacheSamples++
				if tariff.CacheSamples >= 3 {
					tariff.CacheReadVNDPer1M = cost.CacheReadVNDPer1M
				}
			}
			tariffs[cost.Multiplier] = tariff
		}
		for _, tariff := range tariffs {
			if tariff.Samples < 3 {
				row.UnverifiedSamples += tariff.Samples
				continue
			}
			row.ObservedTariffs = append(row.ObservedTariffs, tariff)
			row.VerifiedSamples += tariff.Samples
		}
		slices.SortFunc(row.ObservedTariffs, func(a, b XPikiObservedTariff) int {
			left, _ := strconv.Atoi(a.Multiplier)
			right, _ := strconv.Atoi(b.Multiplier)
			return left - right
		})
		if len(row.ObservedTariffs) == 1 {
			tariff := row.ObservedTariffs[0]
			row.EffectiveInputVNDPer1M, row.EffectiveOutputVNDPer1M = tariff.InputVNDPer1M, tariff.OutputVNDPer1M
			row.EffectiveCacheReadVNDPer1M = tariff.CacheReadVNDPer1M
			row.ObservedMultiplier, row.VerifiedAt = tariff.Multiplier, tariff.LatestAt
		}
	}
}
