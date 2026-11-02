package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

func tokenConversion(t *testing.T, fixture string) xpikiConversion {
	t.Helper()
	var result xpikiConversion
	if err := json.Unmarshal([]byte(fixture), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestXPikiBalanceOnlyCheckDoesNotFetchTariffHistory(t *testing.T) {
	var statusCalls, historyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/logs" {
			historyCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []any{}, "has_more": false}})
			return
		}
		statusCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"quota_used": 100, "quota_limit": 1000, "conversion": json.RawMessage(gptConversionFixture),
		}})
	}))
	defer server.Close()
	credentials := []XPikiQuotaCredential{{Label: "fixture", APIKey: "fixture"}}
	quick := checkXPikiQuotasAtURL(context.Background(), credentials, server.URL+"/status")
	if statusCalls.Load() != 1 || historyCalls.Load() != 0 || len(quick.Keys) != 1 || quick.Keys[0].Error != "" || len(quick.ModelProfiles) != 1 {
		t.Fatalf("balance refresh fetched history or lost balance: status=%d history=%d report=%+v", statusCalls.Load(), historyCalls.Load(), quick)
	}
	deep := checkXPikiQuotasAtURL(context.Background(), credentials, server.URL+"/status", server.URL+"/logs")
	if statusCalls.Load() != 2 || historyCalls.Load() != 1 || deep.Keys[0].RemainingVND != quick.Keys[0].RemainingVND {
		t.Fatalf("explicit reconciliation changed balance or skipped history: status=%d history=%d", statusCalls.Load(), historyCalls.Load())
	}
}

func TestXPikiTokenCostReconcilesActualInputOutputCacheAndRounding(t *testing.T) {
	for _, tc := range []struct {
		model, fixture, fee, total, inputPrice, outputPrice, cachePrice, multiplier string
		input, output, cache                                                        int
	}{
		{"gpt-6.1-sol", gptConversionFixture, "35631", "185.2812", "624", "2340", "312", "3", 9548, 76, 574208},
		{"claude-opus-5-5", claudeConversionFixture, "46688", "80.925866666667", "1040", "5200", "520", "4", 56371, 3316, 9728},
	} {
		item := XPikiLogItem{ModelSlug: tc.model, QuotaSource: "xpiki", UpstreamQuota: tc.fee,
			InputTokens: tc.input, OutputTokens: tc.output, CacheReadTokens: tc.cache}
		cost := calculateXPikiTokenCost(item, tokenConversion(t, tc.fixture))
		if cost.Status != "reconciled" || cost.TotalVND != tc.total || cost.Multiplier != tc.multiplier ||
			cost.InputVNDPer1M != tc.inputPrice || cost.OutputVNDPer1M != tc.outputPrice || cost.CacheReadVNDPer1M != tc.cachePrice {
			t.Fatalf("wrong token calculation for %s: %+v", tc.model, cost)
		}
		components := decimal.RequireFromString(cost.InputVND).Add(decimal.RequireFromString(cost.OutputVND)).
			Add(decimal.RequireFromString(cost.CacheReadVND)).Add(decimal.RequireFromString(cost.RoundingVND))
		if components.Sub(decimal.RequireFromString(cost.TotalVND)).Abs().GreaterThan(decimal.RequireFromString("0.000000000002")) {
			t.Fatal("token breakdown does not sum to the total")
		}
		if cost.InputTokens != item.InputTokens || cost.CacheReadTokens != item.CacheReadTokens {
			t.Fatal("provider uncached input was subtracted twice")
		}
	}
}

func TestXPikiTokenCostUnknownsNeverUseQuotaWalletOrFreeCacheWrite(t *testing.T) {
	base := XPikiLogItem{ModelSlug: "gpt-6.1-sol", QuotaSource: "xpiki", UpstreamQuota: "120", InputTokens: 1000}
	conversion := tokenConversion(t, gptConversionFixture)
	for name, mutate := range map[string]func(*XPikiLogItem){
		"cache_write":         func(i *XPikiLogItem) { i.CacheWriteTokens = 500 },
		"unknown_cache_model": func(i *XPikiLogItem) { i.ModelSlug = "gpt-6-astra"; i.CacheReadTokens = 1000 },
		"missing_receipt":     func(i *XPikiLogItem) { i.QuotaSource = "unmatched" },
		"invalid_fee":         func(i *XPikiLogItem) { i.UpstreamQuota = "invalid" },
		"negative_fee":        func(i *XPikiLogItem) { i.UpstreamQuota = "-1" },
		"negative_tokens":     func(i *XPikiLogItem) { i.InputTokens = -1 },
		"missing_usage":       func(i *XPikiLogItem) { i.InputTokens = 0 },
		"unknown_model":       func(i *XPikiLogItem) { i.ModelSlug = "other" },
		"media_charge":        func(i *XPikiLogItem) { i.mediaUsage = true },
		"mismatch":            func(i *XPikiLogItem) { i.UpstreamQuota = "121" },
	} {
		t.Run(name, func(t *testing.T) {
			item := base
			mutate(&item)
			cost := calculateXPikiTokenCost(item, conversion)
			if cost.Status != "unverified" || cost.CostVND != "" || cost.TotalVND != "" || cost.Warning == "" {
				t.Fatalf("unknown charge became a number: %+v", cost)
			}
		})
	}
	if cost := calculateXPikiTokenCost(base, xpikiConversion{}); cost.Status != "unverified" || cost.CostVND != "" {
		t.Fatal("missing conversion was guessed")
	}
}

func TestXPikiTokenCostConfirmedZeroDoesNotInferTariff(t *testing.T) {
	item := XPikiLogItem{QuotaSource: "xpiki", UpstreamQuota: "0", ModelSlug: "unknown", InputTokens: 100000, CacheWriteTokens: 123}
	cost := calculateXPikiTokenCost(item, xpikiConversion{})
	if cost.Status != "confirmed_zero" || cost.CostVND != "0 ₫" || cost.Multiplier != "" || cost.InputVNDPer1M != "" {
		t.Fatalf("zero receipt invented a tariff: %+v", cost)
	}
}

func TestXPikiTokenCostRespectsKeyGroupAndProviderModelAlias(t *testing.T) {
	conversion := tokenConversion(t, claudeConversionFixture)
	conversion.GroupRatio = json.RawMessage(`2`)
	item := XPikiLogItem{ModelSlug: "vendor/claude-opus-5.5", QuotaSource: "xpiki", UpstreamQuota: "1200", InputTokens: 1000}
	cost := calculateXPikiTokenCost(item, conversion)
	if cost.Status != "reconciled" || cost.Multiplier != "4" || cost.InputVNDPer1M != "2080" || cost.TotalVND != "2.08" {
		t.Fatalf("key ratio or Opus alias lost: %+v", cost)
	}
}

func TestXPikiObservedPricesAreKeySpecificAndRejectConflicts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fees := []int{120, 120, 120}
		switch r.Header.Get("Authorization") {
		case "Bearer key-five":
			fees = []int{200, 200, 200}
		case "Bearer conflict":
			fees = []int{120, 200, 120}
		case "Bearer insufficient":
			fees = []int{120, 120}
		}
		items := make([]map[string]any, 0, len(fees))
		for i, fee := range fees {
			items = append(items, map[string]any{"request_id": string(rune('a' + i)), "model_slug": "gpt-6.1-sol", "status": "success", "input_tokens": 1000, "credit_cost": fee})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": items}})
	}))
	defer server.Close()
	for key, price := range map[string]string{"key-three": "624", "key-five": "1040", "conflict": "", "insufficient": ""} {
		profile := buildXPikiModelProfile(key, tokenConversion(t, gptConversionFixture), key)
		verifyXPikiEffectiveModelPrices(context.Background(), XPikiQuotaCredential{APIKey: key}, server.URL, profile)
		for _, row := range profile.Models {
			if row.ModelSlug == "gpt-6.1-sol" && row.EffectiveInputVNDPer1M != price {
				t.Fatalf("%s: model multiplier was generalized or guessed: %+v", key, row)
			}
			if row.ModelSlug == "gpt-6-astra" && row.EffectiveInputVNDPer1M != "" {
				t.Fatal("unobserved Astra inherited Sol's tariff")
			}
		}
	}
}

func TestXPikiTokenReportUsesEachSourceOnceAndTotalsOnlyKnownAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fixture := gptConversionFixture
		if r.Header.Get("Authorization") == "Bearer claude" {
			fixture = claudeConversionFixture
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"quota_used": 0, "quota_limit": 1, "conversion": json.RawMessage(fixture),
		}})
	}))
	defer server.Close()
	report := XPikiLogsReport{Items: []XPikiLogItem{
		{SourceKeyID: xpikiSourceKeyID("gpt"), ModelSlug: "gpt-6.1-sol", QuotaSource: "xpiki", UpstreamQuota: "120", InputTokens: 1000},
		{SourceKeyID: xpikiSourceKeyID("claude"), ModelSlug: "claude-opus-5-5", QuotaSource: "xpiki", UpstreamQuota: "600", InputTokens: 1000},
		{SourceKeyID: xpikiSourceKeyID("gpt"), QuotaSource: "xpiki", UpstreamQuota: "0"},
		{SourceKeyID: xpikiSourceKeyID("gpt"), QuotaSource: "unavailable"},
		{QuotaSource: "not_xpiki"},
	}}
	enrichXPikiReportTokenCosts(context.Background(), &report, []XPikiQuotaCredential{{APIKey: "gpt"}, {APIKey: "claude"}, {APIKey: "unused"}}, server.URL)
	if calls.Load() != 2 || report.TokenCostSummary.TotalVND != "1.664" || report.TokenCostSummary.KnownRequests != 3 || report.TokenCostSummary.UnknownRequests != 1 {
		t.Fatalf("wrong source or summary: calls=%d summary=%+v", calls.Load(), report.TokenCostSummary)
	}
	if report.Items[0].TokenCost.InputVNDPer1M != "624" || report.Items[1].TokenCost.InputVNDPer1M != "1040" {
		t.Fatal("GPT and Claude purchase anchors were merged")
	}
}

func TestXPikiUsageCostKeepsMatchedProviderTokenBreakdown(t *testing.T) {
	local := []XPikiLogItem{{ChannelId: 13, UpstreamRequestID: "request", QuotaSource: "unmatched", InputTokens: 999999}}
	provider := XPikiLogItem{ChannelId: 13, RequestID: "request", QuotaSource: "xpiki", UpstreamQuota: "600", ModelSlug: "claude-opus-5-5", InputTokens: 1000}
	provider.TokenCost = calculateXPikiTokenCost(provider, tokenConversion(t, claudeConversionFixture))
	attachMatchedXPikiCosts(local, []XPikiLogItem{provider})
	if local[0].TokenCost == nil || local[0].TokenCost.InputTokens != 1000 || local[0].InputTokens != 999999 {
		t.Fatal("cost used local wallet usage or mutated local token counts")
	}
	// API model supports the same nested contract used by Quota Check.
	cost := model.XPikiLogCost{TokenCost: local[0].TokenCost}
	if cost.TokenCost.TotalVND != "1.04" {
		t.Fatal("matched token cost lost")
	}
}

func TestXPikiTariffHistoryKeepsMultipleLevelsAndPartialEvidence(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := r.URL.Query().Get("page")
		if page == "3" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var rows []map[string]any
		for i := 0; i < 4; i++ {
			fee := 600
			if page == "2" {
				fee = 1500
			}
			rows = append(rows, map[string]any{"request_id": page + string(rune('a'+i)), "model_slug": "claude-opus-5-5", "status": "success", "input_tokens": 1000, "credit_cost": fee, "created_at": "2026-10-10T01:00:00+07:00"})
		}
		rows = append(rows, rows[0]) // duplicate provider entry must not inflate samples
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": rows, "has_more": true}})
	}))
	defer server.Close()
	profile := buildXPikiModelProfile("test", tokenConversion(t, claudeConversionFixture), "test-key")
	verifyXPikiEffectiveModelPrices(context.Background(), XPikiQuotaCredential{APIKey: "test-key"}, server.URL, profile)
	if calls.Load() != 3 || profile.PricingWarning == "" {
		t.Fatal("partial lookup failure was hidden")
	}
	for _, row := range profile.Models {
		if row.ModelSlug != "claude-opus-5-5" {
			continue
		}
		if row.EffectiveInputVNDPer1M != "" || row.VerifiedSamples != 8 || len(row.ObservedTariffs) != 2 {
			t.Fatalf("different levels merged or dedup lost: %+v", row)
		}
		if row.ObservedTariffs[0].InputVNDPer1M != "1040" || row.ObservedTariffs[1].InputVNDPer1M != "2600" || row.ObservedTariffs[1].OutputVNDPer1M != "13000" {
			t.Fatal("mixed Opus settlements priced with one multiplier")
		}
		if row.ObservedTariffs[0].CacheReadVNDPer1M != "" {
			t.Fatal("non-cache receipts projected onto cache price")
		}
	}
}

func TestXPikiImageCatalogRemainsUnknownNotFree(t *testing.T) {
	conversion := tokenConversion(t, `{"group_ratio":1,"models":[{"slug":"gpt-image-2","input_price_per_1m":0,"output_price_per_1m":0}]}`)
	profile := buildXPikiModelProfile("image", conversion, "test")
	if profile == nil || len(profile.Models) != 1 || profile.Models[0].ModelSlug != "gpt-image-2" || profile.Models[0].PriceVNDPer1M != "—" {
		t.Fatalf("image catalog hidden or zero conversion treated as free: %+v", profile)
	}
}

func TestXPikiTariffHistoryHasBoundedPagesAndCacheEvidence(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := r.URL.Query().Get("page")
		rows := []map[string]any{
			{"request_id": page + "cache", "model_slug": "gpt-6-astra", "status": "success", "input_tokens": 1000, "cache_read_tokens": 1000, "credit_cost": 900},
			{"request_id": page + "plain", "model_slug": "gpt-6.1-sol", "status": "success", "input_tokens": 1000, "credit_cost": 120},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": rows, "has_more": true}})
	}))
	defer server.Close()
	profile := buildXPikiModelProfile("test", tokenConversion(t, gptConversionFixture), "test-key")
	verifyXPikiEffectiveModelPrices(context.Background(), XPikiQuotaCredential{APIKey: "test-key"}, server.URL, profile)
	if calls.Load() != 20 || profile.PricingWarning == "" {
		t.Fatal("history scan is unbounded or silently claims completeness")
	}
	for _, row := range profile.Models {
		if row.ModelSlug == "gpt-6-astra" && row.EffectiveCacheReadVNDPer1M != "1560" {
			t.Fatal("newly verified Astra cached receipts lost")
		}
		if row.ModelSlug == "gpt-6.1-sol" && row.EffectiveCacheReadVNDPer1M != "" {
			t.Fatal("cache rate projected without cache samples")
		}
	}
}

func TestXPikiTariffCacheReusesDatedSnapshotWithoutUpstreamCalls(t *testing.T) {
	cred := XPikiQuotaCredential{APIKey: "test-tariff-cache-only"}
	key := xpikiSourceKeyID(cred.APIKey)
	snapshot := xpikiTariffHistory{checkedAt: time.Now(), warning: "Đã đối soát tối đa 1.000 biên lai gần nhất/key; chưa phải toàn bộ lịch sử.",
		items: []XPikiLogItem{{RequestID: "snapshot"}}}
	xpikiTariffCache.Lock()
	xpikiTariffCache.entries[key] = snapshot
	xpikiTariffCache.Unlock()
	defer func() { xpikiTariffCache.Lock(); delete(xpikiTariffCache.entries, key); xpikiTariffCache.Unlock() }()
	for range 3 {
		history := cachedXPikiTariffHistory(context.Background(), cred, XPikiKeyLogsURL)
		if !history.checkedAt.Equal(snapshot.checkedAt) || history.warning != snapshot.warning || len(history.items) != 1 || history.items[0].RequestID != "snapshot" {
			t.Fatal("cache lost provenance or re-fetched the provider")
		}
	}
}
