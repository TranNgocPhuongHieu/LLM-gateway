package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const claudeConversionFixture = `{"group_ratio":1,"models":[
{"slug":"claude-opus-5-5","input_price_per_1m":150000,"output_price_per_1m":750000},
{"slug":"claude-opus-4-6","input_price_per_1m":150000,"output_price_per_1m":750000},
{"slug":"claude-sonnet-5","input_price_per_1m":90000,"output_price_per_1m":450000},
{"slug":"claude-haiku-4-5","input_price_per_1m":40000,"output_price_per_1m":200000},
{"slug":"claude-fable-5-1","input_price_per_1m":300000,"output_price_per_1m":1500000}]}`

func TestXPikiSeparateGPTAndClaudePurchaseAnchors(t *testing.T) {
	for _, slug := range []string{"claude-opus-5-5", "claude-opus-4-6", "claude-sonnet-5", "claude-haiku-4-5", "claude-fable-5-1"} {
		if got := FormatXPikiCostVNDForModel(decimal.NewFromInt(15000000), slug); got != "26,000 ₫" {
			t.Fatalf("model discount applied twice for %s: %s", slug, got)
		}
		if got := FormatXPikiCostVNDForModel(decimal.NewFromInt(600000), slug); got != "1,040 ₫" {
			t.Fatalf("wrong purchase conversion for %s: %s", slug, got)
		}
	}
	for _, tc := range []struct {
		model string
		quota int64
		want  string
	}{
		{"gpt-6-astra", 200000, "1,040 ₫"},
		{"gpt-6.1-sol", 40000, "208 ₫"},
		{"tranhieu13102003/gpt-6.1-sol", 120000, "624 ₫"},
		{"claude-opus-5-5", 150000, "260 ₫"},
		{"tranhieu13102003/claude-opus-5.5", 150000, "260 ₫"},
		{"claude-fable-5-1", 300000, "520 ₫"},
		{"unknown-model", 150000, ""},
		{"gpt-6.1-sol", -1, ""},
		{"unknown-model", 0, "0 ₫"},
	} {
		if got := FormatXPikiCostVNDForModel(decimal.NewFromInt(tc.quota), tc.model); got != tc.want {
			t.Errorf("%s/%d: got %q want %q", tc.model, tc.quota, got, tc.want)
		}
	}
}

const gptConversionFixture = `{"group_ratio":1,"models":[
{"slug":"gpt-6-astra","input_price_per_1m":200000,"output_price_per_1m":750000},
{"slug":"gpt-6.1-sol","input_price_per_1m":40000,"output_price_per_1m":150000},
{"slug":"gpt-6-sol","input_price_per_1m":40000,"output_price_per_1m":150000},
{"slug":"gpt-5.6-sol","input_price_per_1m":80000,"output_price_per_1m":300000},
{"slug":"gpt-6-luna","input_price_per_1m":2000,"output_price_per_1m":7500}]}`

func TestXPikiGPTModelTableUsesAstraPurchaseAnchor(t *testing.T) {
	var conversion xpikiConversion
	if err := json.Unmarshal([]byte(gptConversionFixture), &conversion); err != nil {
		t.Fatal(err)
	}
	profile := buildXPikiModelProfile("GPT", conversion, "test-only")
	if profile == nil || len(profile.Models) != 5 || !strings.Contains(profile.PriceBasis, "Astra") || strings.Contains(profile.PriceBasis, "Opus") {
		t.Fatalf("wrong GPT purchase basis: %+v", profile)
	}
	want := map[string][3]string{
		"gpt-6-astra": {"1,040", "3,900", "25,000,000"},
		"gpt-6.1-sol": {"208", "780", "125,000,000"},
		"gpt-6-sol":   {"208", "780", "125,000,000"},
		"gpt-5.6-sol": {"416", "1,560", "62,500,000"},
		"gpt-6-luna":  {"10.4", "39", "2,500,000,000"},
	}
	for _, row := range profile.Models {
		if got := [3]string{row.PriceVNDPer1M, row.OutputPriceVNDPer1M, row.TokensForPackage}; got != want[row.Model] {
			t.Errorf("%s: got %v want %v", row.Model, got, want[row.Model])
		}
	}
}

func TestXPikiMixedBalancesSumFamilyCostsNotRawQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conversion := gptConversionFixture
		quota := 200000
		if r.Header.Get("Authorization") == "Bearer claude-test" {
			conversion = claudeConversionFixture
			quota = 150000
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"quota_used": 0, "quota_limit": quota, "conversion": json.RawMessage(conversion)}})
	}))
	defer server.Close()
	report := checkXPikiQuotasAtURL(context.Background(), []XPikiQuotaCredential{{APIKey: "gpt-test"}, {APIKey: "claude-test"}}, server.URL)
	if report.Keys[0].LimitVND != "1,040 ₫" || report.Keys[1].LimitVND != "260 ₫" || report.TotalRemainingVND != "1,300 ₫" {
		t.Fatalf("mixed pools were converted using a shared rate: %+v", report)
	}
}

func TestXPikiClaudeModelTableUsesProviderConversionNotRequestMultiplier(t *testing.T) {
	var conversion xpikiConversion
	if err := json.Unmarshal([]byte(claudeConversionFixture), &conversion); err != nil {
		t.Fatal(err)
	}
	profile := buildXPikiModelProfile("Claude", conversion, "test-key")
	if profile == nil || len(profile.Models) != 5 {
		t.Fatal("Claude profile incorrectly requires an Astra model")
	}
	if !strings.Contains(profile.PriceBasis, "Opus") || strings.Contains(profile.PriceBasis, "Astra") || strings.Contains(profile.PriceBasis, "Fable") {
		t.Fatalf("Claude uses the wrong purchase reference: %s", profile.PriceBasis)
	}
	want := map[string][3]string{
		"claude-opus-5-5":  {"260", "1,300", "100,000,000"},
		"claude-opus-4-6":  {"260", "1,300", "100,000,000"},
		"claude-sonnet-5":  {"156", "780", "166,666,667"},
		"claude-haiku-4-5": {"69.3333", "346.6667", "375,000,000"},
		"claude-fable-5-1": {"520", "2,600", "50,000,000"},
	}
	for _, row := range profile.Models {
		got := [3]string{row.PriceVNDPer1M, row.OutputPriceVNDPer1M, row.TokensForPackage}
		if got != want[row.Model] {
			t.Fatalf("%s: got %v want %v", row.Model, got, want[row.Model])
		}
	}
}

func TestXPikiBalanceMatches100MDisplayedOpusPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("quota status contract changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"quota_used": 0, "quota_limit": 15000000, "conversion": json.RawMessage(claudeConversionFixture),
		}})
	}))
	defer server.Close()
	report := checkXPikiQuotasAtURL(context.Background(), []XPikiQuotaCredential{{APIKey: "test-only"}}, server.URL)
	key := report.Keys[0]
	if key.ReferenceModel != "claude-opus-5-5" || key.LimitTokens != "100,000,000" ||
		key.LimitVND != "26,000 ₫" || report.TotalRemainingVND != "26,000 ₫" {
		t.Fatalf("displayed package and capital cost disagree: %+v", key)
	}
}

func TestXPikiModelTableGroupRatioAndUnknownRatio(t *testing.T) {
	var conversion xpikiConversion
	if err := json.Unmarshal([]byte(claudeConversionFixture), &conversion); err != nil {
		t.Fatal(err)
	}
	for _, ratio := range []string{"2", "null"} {
		conversion.GroupRatio = json.RawMessage(ratio)
		profile := buildXPikiModelProfile("Claude", conversion, "test-only")
		for _, row := range profile.Models {
			if row.Model != "claude-opus-5-5" {
				continue
			}
			want := [3]string{"520", "2,600", "50,000,000"}
			if ratio == "null" {
				want = [3]string{"—", "—", "—"}
			}
			if got := [3]string{row.PriceVNDPer1M, row.OutputPriceVNDPer1M, row.TokensForPackage}; got != want {
				t.Fatalf("ratio %s: got %v want %v", ratio, got, want)
			}
		}
	}
}

func TestXPikiUsageLogCostIsLedgerOnlyAndSharedKeyFetchedOnce(t *testing.T) {
	stamp := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"has_more": false, "items": []map[string]any{
				{"created_at": stamp.Format(time.RFC3339), "model_slug": "claude-opus-5-5", "status": "success", "credit_cost": 78840, "request_id": "charged"},
				{"created_at": stamp.Format(time.RFC3339), "model_slug": "claude-opus-5-5", "status": "canceled", "credit_cost": 0, "request_id": "canceled"},
			},
		}})
	}))
	defer server.Close()
	logs := []*model.Log{
		{ChannelId: 14, Type: model.LogTypeConsume, CreatedAt: stamp.Unix(), UpstreamRequestId: "charged", Quota: 999999, PromptTokens: 999999},
		{ChannelId: 13, Type: model.LogTypeError, CreatedAt: stamp.Unix(), UpstreamRequestId: "canceled", PromptTokens: 180033},
		{ChannelId: 10, Type: model.LogTypeConsume, CreatedAt: stamp.Unix(), UpstreamRequestId: "charged", ModelName: "claude-opus-5-5", Quota: 999999},
		{ChannelId: 13, Type: model.LogTypeConsume, CreatedAt: stamp.Unix(), UpstreamRequestId: "missing", Quota: 999999},
		{ChannelId: 13, Type: model.LogTypeTopup, Quota: 15000000},
	}
	creds := []XPikiQuotaCredential{{ChannelId: 13, APIKey: "shared-test-key", BaseURL: "https://api.xpiki.com", SourceChannels: []XPikiSourceChannel{{ID: 13}, {ID: 14}}}}
	enrichUsageLogXPikiCostsAtURL(context.Background(), logs, creds, server.URL)
	if calls.Load() != 1 {
		t.Fatalf("shared Claude key fetched %d times", calls.Load())
	}
	if got := logs[0].XPikiCost; got.Source != "xpiki" || got.CostVND != "136.656 ₫" || got.Quota != "78840" || got.Match != "request_id" {
		t.Fatalf("estimated fee replaced actual ledger charge: %+v", got)
	}
	if got := logs[1].XPikiCost; got.Source != "xpiki" || got.CostVND != "0 ₫" {
		t.Fatalf("canceled provider zero was estimated from tokens: %+v", got)
	}
	for i, source := range map[int]string{2: "not_xpiki", 3: "unmatched", 4: "not_applicable"} {
		if got := logs[i].XPikiCost; got.Source != source || got.CostVND != "" {
			t.Fatalf("row %d fell back to wallet/model fees: %+v", i, got)
		}
	}
	if logs[0].Quota != 999999 || logs[0].PromptTokens != 999999 {
		t.Fatal("provider enrichment changed user billing or tokens")
	}
	enrichUsageLogXPikiCostsAtURL(context.Background(), logs, creds, server.URL)
	if calls.Load() != 1 || logs[0].XPikiCost.CostVND != "136.656 ₫" || logs[1].XPikiCost.CostVND != "0 ₫" {
		t.Fatalf("reloading unchanged usage logs refetched receipts or lost costs: calls=%d", calls.Load())
	}
}

func TestXPikiUsageLogProviderFailureNeverBecomesZeroOrWalletFee(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer server.Close()
	logs := []*model.Log{{ChannelId: 13, Type: model.LogTypeConsume, CreatedAt: time.Now().Unix(), Quota: 999999}}
	enrichUsageLogXPikiCostsAtURL(context.Background(), logs, []XPikiQuotaCredential{{ChannelId: 13, APIKey: "test-only", BaseURL: "https://api.xpiki.com"}}, server.URL)
	if got := logs[0].XPikiCost; got.Source != "unavailable" || got.CostVND != "" || got.Warning == "" {
		t.Fatalf("provider failure hidden: %+v", got)
	}
}

func TestXPikiUnknownAndMixedKeyPoolsKeepMoneyUnknown(t *testing.T) {
	for _, catalog := range []string{
		`{"group_ratio":1,"models":[{"slug":"unknown-model","input_price_per_1m":200000}]}`,
		`{"group_ratio":1,"models":[{"slug":"gpt-6-astra","input_price_per_1m":200000},{"slug":"claude-opus-5-5","input_price_per_1m":150000}]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"quota_used": 1000, "quota_limit": 200000, "conversion": json.RawMessage(catalog),
			}})
		}))
		report := checkXPikiQuotasAtURL(context.Background(), []XPikiQuotaCredential{{APIKey: "test-only"}}, server.URL)
		server.Close()
		if !report.TotalIncomplete || report.TotalRemainingVND != "—" || report.Keys[0].RemainingVND != "" || report.Keys[0].RemainingQuota != 199000 || report.Keys[0].ReferenceModel != "" || report.Keys[0].RemainingTokens != "" {
			t.Fatalf("unknown purchase pool got a fabricated value or lost raw quota: %+v", report)
		}
	}
}

func TestXPikiLogFeesUseProviderFamilyNotLocalModelOrWallet(t *testing.T) {
	stamp := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"has_more": false, "items": []map[string]any{
			{"created_at": stamp.Format(time.RFC3339), "request_id": "gpt-request", "model_slug": "gpt-6.1-sol", "status": "success", "credit_cost": 78840},
			{"created_at": stamp.Format(time.RFC3339), "request_id": "claude-request", "model_slug": "claude-opus-5-5", "status": "success", "credit_cost": 78840},
		}}})
	}))
	defer server.Close()
	logs := []*model.Log{
		{ChannelId: 8, Type: model.LogTypeConsume, CreatedAt: stamp.Unix(), UpstreamRequestId: "gpt-request", ModelName: "local-alias", Quota: 999999},
		{ChannelId: 13, Type: model.LogTypeConsume, CreatedAt: stamp.Unix(), UpstreamRequestId: "claude-request", ModelName: "local-alias", Quota: 999999},
	}
	creds := []XPikiQuotaCredential{{ChannelId: 8, APIKey: "test-only", BaseURL: "https://api.xpiki.com"}, {ChannelId: 13, APIKey: "test-only", BaseURL: "https://api.xpiki.com"}}
	enrichUsageLogXPikiCostsAtURL(context.Background(), logs, creds, server.URL)
	if logs[0].XPikiCost.CostVND != "409.968 ₫" || logs[1].XPikiCost.CostVND != "136.656 ₫" {
		t.Fatalf("provider model pools were mixed: GPT=%+v Claude=%+v", logs[0].XPikiCost, logs[1].XPikiCost)
	}
	if logs[0].XPikiCost.ReferenceModel != xpikiAstraModelSlug || logs[0].XPikiCost.ReferenceTokens != "394,200" || logs[1].XPikiCost.ReferenceModel != xpikiOpusModelSlug || logs[1].XPikiCost.ReferenceTokens != "525,600" {
		t.Fatal("Logs page lost paid reference-token equivalents")
	}
	if logs[0].Quota != 999999 || logs[1].Quota != 999999 {
		t.Fatal("capital cost changed wallet deductions")
	}
}

func TestXPikiSmallModelPricesDoNotUnderflowPackageDivision(t *testing.T) {
	var conversion xpikiConversion
	if err := json.Unmarshal([]byte(`{"group_ratio":"1e-28","models":[{"slug":"gpt-6-astra","input_price_per_1m":"1e-28"}]}`), &conversion); err != nil {
		t.Fatal(err)
	}
	profile := buildXPikiModelProfile("small", conversion, "test-only")
	if profile == nil || len(profile.Models) != 1 || profile.Models[0].TokensForPackage == "—" {
		t.Fatalf("valid small price was lost: %+v", profile)
	}
}

func TestXPikiPaidReferenceTokensAndCacheReadTariffs(t *testing.T) {
	for _, tc := range []struct {
		model                   string
		fee                     int64
		reference, tokens, cost string
	}{
		{"gpt-6.1-sol", 2076, "gpt-6-astra", "10,380", "10.7952 ₫"},
		{"claude-opus-5-5", 51800, "claude-opus-5-5", "345,333.3333", "89.7867 ₫"},
	} {
		tokens, reference := xpikiChargedReferenceTokens(decimal.NewFromInt(tc.fee), tc.model)
		if reference != tc.reference || formatXPikiNumber(tokens.Round(4)) != tc.tokens || FormatXPikiCostVNDForModel(decimal.NewFromInt(tc.fee), tc.model) != tc.cost {
			t.Fatalf("wrong paid reference token conversion for %s", tc.model)
		}
	}
	for _, fixture := range []string{gptConversionFixture, claudeConversionFixture} {
		var conversion xpikiConversion
		if err := json.Unmarshal([]byte(fixture), &conversion); err != nil {
			t.Fatal(err)
		}
		profile := buildXPikiModelProfile("test", conversion, "test-only")
		for _, row := range profile.Models {
			if row.Model == "gpt-6.1-sol" && row.CacheReadVNDPer1M != "104" {
				t.Fatalf("wrong Sol cache-read price: %+v", row)
			}
			if row.Model == "claude-opus-5-5" && row.CacheReadVNDPer1M != "130" {
				t.Fatalf("wrong Opus cache-read price: %+v", row)
			}
			if row.Model == "gpt-6-astra" && row.CacheReadVNDPer1M != "520" {
				t.Fatal("Astra cache tariff does not match newly verified receipts")
			}
			if row.CacheWriteVNDPer1M != "" {
				t.Fatal("invented a cache-write tariff")
			}
		}
	}
}

func TestXPikiObservedCacheReadReceiptsKeepProviderRoundingAndMultipliers(t *testing.T) {
	// Current XPiki receipts, not official-vendor pricing assumptions.
	for _, tc := range []struct {
		input, output, cache, fee, multiplier int64
		inputRate, outputRate                 string
	}{
		{1969, 335, 28160, 2076, 3, ".04", ".15"},
		{920, 341, 167424, 51800, 4, ".15", ".75"},
	} {
		input := decimal.RequireFromString(tc.inputRate)
		output := decimal.RequireFromString(tc.outputRate)
		base := input.Mul(decimal.NewFromInt(tc.input)).Add(output.Mul(decimal.NewFromInt(tc.output))).Add(input.Div(decimal.NewFromInt(2)).Mul(decimal.NewFromInt(tc.cache)))
		if got := base.Floor().Mul(decimal.NewFromInt(tc.multiplier)); !got.Equal(decimal.NewFromInt(tc.fee)) {
			t.Fatalf("cache tariff does not reconcile to the provider receipt: %s", got)
		}
	}
}
