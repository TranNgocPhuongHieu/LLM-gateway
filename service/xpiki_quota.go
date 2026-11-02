package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/shopspring/decimal"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	XPikiKeyStatusURL   = "https://api.xpiki.com/api/public/key-status"
	XPikiKeyLogsURL     = "https://api.xpiki.com/api/public/key-status/logs"
	xpikiAstraModelSlug = "gpt-6-astra"
	xpikiOpusModelSlug  = "claude-opus-5-5"
	// Purchase anchors are displayed input-equivalent tokens, not request
	// capacity. Claude's reference is Opus 5.5, not Fable.
	xpikiPackagePriceVND      = int64(26000)
	xpikiAstraPriceVNDPer1M   = int64(1040)
	xpikiAstraQuotaPer1M      = int64(200000)
	xpikiOpusPriceVNDPer1M    = int64(260)
	xpikiOpusQuotaPer1M       = int64(150000)
	xpikiAstraTokenMultiple   = int64(5)
	xpikiMaxWorkers           = 8
	xpikiMaxResponseBytes     = 256 << 10
	xpikiMaxNumericChars      = 64
	xpikiMaxDecimalExponent   = 28
	xpikiMaxSignificantDigits = 48
)

type XPikiQuotaCredential struct {
	Label          string
	APIKey         string
	ChannelId      int
	ChannelName    string
	ChannelStatus  int
	BaseURL        string
	SourceChannels []XPikiSourceChannel
}

type XPikiSourceChannel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type XPikiQuotaKeyResult struct {
	Label           string   `json:"label"`
	Name            string   `json:"name,omitempty"`
	ChannelId       int      `json:"channel_id,omitempty"`
	ChannelName     string   `json:"channel_name,omitempty"`
	ChannelStatus   int      `json:"channel_status,omitempty"`
	UsedQuota       int64    `json:"used_quota"`
	LimitQuota      int64    `json:"limit_quota"`
	RemainingQuota  int64    `json:"remaining_quota"`
	UsedVND         string   `json:"used_vnd"`
	LimitVND        string   `json:"limit_vnd"`
	RemainingVND    string   `json:"remaining_vnd"`
	UsedAstra       string   `json:"used_astra"`
	LimitAstra      string   `json:"limit_astra"`
	RemainingAstra  string   `json:"remaining_astra"`
	UsedPercent     string   `json:"used_percent"`
	Unlimited       bool     `json:"unlimited"`
	Status          []string `json:"status"`
	Error           string   `json:"error,omitempty"`
	ReferenceModel  string   `json:"reference_model,omitempty"`
	UsedTokens      string   `json:"used_tokens,omitempty"`
	LimitTokens     string   `json:"limit_tokens,omitempty"`
	RemainingTokens string   `json:"remaining_tokens,omitempty"`
}

type XPikiQuotaModelRow struct {
	Model                      string                `json:"model"`
	ModelSlug                  string                `json:"model_slug"`
	PriceVNDPer1M              string                `json:"price_vnd_per_1m"`
	TokensPerQuota             string                `json:"tokens_per_quota"`
	OutputPriceVNDPer1M        string                `json:"output_price_vnd_per_1m"`
	CacheReadVNDPer1M          string                `json:"cache_read_vnd_per_1m"`
	CacheWriteVNDPer1M         string                `json:"cache_write_vnd_per_1m"`
	TokensForPackage           string                `json:"tokens_for_26000"`
	EffectiveInputVNDPer1M     string                `json:"effective_input_vnd_per_1m,omitempty"`
	EffectiveOutputVNDPer1M    string                `json:"effective_output_vnd_per_1m,omitempty"`
	EffectiveCacheReadVNDPer1M string                `json:"effective_cache_read_vnd_per_1m,omitempty"`
	ObservedMultiplier         string                `json:"observed_multiplier,omitempty"`
	VerifiedSamples            int                   `json:"verified_samples"`
	VerifiedAt                 string                `json:"verified_at,omitempty"`
	ObservedTariffs            []XPikiObservedTariff `json:"observed_tariffs,omitempty"`
	UnverifiedSamples          int                   `json:"unverified_samples"`
}

type XPikiObservedTariff struct {
	Multiplier        string `json:"multiplier"`
	InputVNDPer1M     string `json:"input_vnd_per_1m"`
	OutputVNDPer1M    string `json:"output_vnd_per_1m"`
	CacheReadVNDPer1M string `json:"cache_read_vnd_per_1m,omitempty"`
	Samples           int    `json:"samples"`
	CacheSamples      int    `json:"cache_samples"`
	LatestAt          string `json:"latest_at"`
}

type XPikiQuotaModelProfile struct {
	Labels             []string             `json:"labels"`
	GroupRatio         string               `json:"group_ratio"`
	AstraPriceVNDPer1M string               `json:"astra_price_vnd_per_1m"`
	Models             []XPikiQuotaModelRow `json:"models"`
	PriceBasis         string               `json:"price_basis"`
	CachePriceBasis    string               `json:"cache_price_basis"`
	PricingWarning     string               `json:"pricing_warning,omitempty"`
	PricingCheckedAt   string               `json:"pricing_checked_at,omitempty"`
	conversion         xpikiConversion
}

type XPikiQuotaReport struct {
	Keys              []XPikiQuotaKeyResult    `json:"keys"`
	TotalRemaining    string                   `json:"total_remaining"`
	TotalRemainingVND string                   `json:"total_remaining_vnd"`
	TotalUnlimited    bool                     `json:"total_unlimited"`
	TotalIncomplete   bool                     `json:"total_incomplete"`
	ModelProfiles     []XPikiQuotaModelProfile `json:"model_profiles"`
	NoModelPriceKeys  []string                 `json:"no_model_price_keys"`
}

type XPikiLogItem struct {
	LogKey                 string                `json:"log_key,omitempty"`
	SourceKeyID            string                `json:"source_key_id,omitempty"`
	SourceChannels         []XPikiSourceChannel  `json:"source_channels,omitempty"`
	RequestOrigin          string                `json:"request_origin,omitempty"`
	OriginMatch            string                `json:"origin_match,omitempty"`
	LocalRequestID         string                `json:"local_request_id,omitempty"`
	LocalFinalStatus       string                `json:"local_final_status,omitempty"`
	FallbackChannelID      int                   `json:"fallback_channel_id,omitempty"`
	FallbackChannelName    string                `json:"fallback_channel_name,omitempty"`
	LogID                  int                   `json:"log_id,omitempty"`
	CreatedAt              string                `json:"created_at"`
	Type                   string                `json:"type"`
	ModelSlug              string                `json:"model_slug"`
	Status                 string                `json:"status"`
	RequestOutcome         string                `json:"request_outcome,omitempty"`
	ErrorCode              string                `json:"error_code,omitempty"`
	InputTokens            int                   `json:"input_tokens"`
	OutputTokens           int                   `json:"output_tokens"`
	CacheReadTokens        int                   `json:"cache_read_tokens"`
	CacheWriteTokens       int                   `json:"cache_write_tokens"`
	CreditCost             float64               `json:"credit_cost"`
	QuotaAstra             string                `json:"quota_astra,omitempty"`
	CostReferenceModel     string                `json:"cost_reference_model,omitempty"`
	ChargedReferenceTokens string                `json:"charged_reference_tokens,omitempty"`
	QuotaSource            string                `json:"quota_source,omitempty"`
	UpstreamQuota          string                `json:"upstream_quota,omitempty"`
	UpstreamRequestID      string                `json:"upstream_request_id,omitempty"`
	CostVND                string                `json:"cost_vnd"`
	TokenCost              *model.XPikiTokenCost `json:"token_cost,omitempty"`
	mediaUsage             bool
	DurationMs             int            `json:"duration_ms"`
	RequestID              string         `json:"request_id"`
	IP                     string         `json:"ip"`
	KeyLabel               string         `json:"key_label"`
	KeyMasked              string         `json:"key_masked,omitempty"`
	TokenName              string         `json:"token_name,omitempty"`
	Username               string         `json:"username,omitempty"`
	TokenID                int            `json:"token_id,omitempty"`
	RequestPath            string         `json:"request_path,omitempty"`
	ChannelId              int            `json:"channel_id,omitempty"`
	ChannelName            string         `json:"channel_name,omitempty"`
	Destination            string         `json:"destination,omitempty"`
	Retried                bool           `json:"retried,omitempty"`
	InitialChannelId       int            `json:"initial_channel_id,omitempty"`
	InitialChannelName     string         `json:"initial_channel_name,omitempty"`
	InitialDestination     string         `json:"initial_destination,omitempty"`
	InitialErrorCode       string         `json:"initial_error_code,omitempty"`
	ChannelTrail           []string       `json:"channel_trail,omitempty"`
	TotalAttempts          int            `json:"total_attempts,omitempty"`
	FailedAttempts         int            `json:"failed_attempts,omitempty"`
	AttemptNumber          int            `json:"attempt_number,omitempty"`
	Attempts               []XPikiLogItem `json:"attempts,omitempty"`
}

type XPikiKeyOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	ChannelId   int    `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	KeyMasked   string `json:"key_masked"`
}

type XPikiLogsFilter struct {
	Source        string `json:"source"`
	KeyFilter     string `json:"key_filter"`
	ChannelFilter string `json:"channel_filter"`
	Search        string `json:"search"`
	Model         string `json:"model"`
	User          string `json:"user"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	Date          string `json:"date"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	GroupMode     string `json:"group_mode"`
}

// No question marks: GORM treats them as placeholders even inside SQL literals.
const xpikiChannelTrailPattern = `"use_channel"\s*:\s*(\[\s*"[1-9][0-9]*"(\s*,\s*"[1-9][0-9]*")*\s*\])`

func CheckXPikiQuotas(ctx context.Context, credentials []XPikiQuotaCredential) XPikiQuotaReport {
	return CheckXPikiQuotasWithPricing(ctx, credentials, true)
}

// Balance refreshes need only key-status. Historical tariff reconciliation is
// an explicit, slower dashboard action, not part of every balance refresh.
func CheckXPikiQuotasWithPricing(ctx context.Context, credentials []XPikiQuotaCredential, includePricing bool) XPikiQuotaReport {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	if !includePricing {
		return checkXPikiQuotasAtURL(ctx, credentials, XPikiKeyStatusURL)
	}
	return checkXPikiQuotasAtURL(ctx, credentials, XPikiKeyStatusURL, XPikiKeyLogsURL)
}

func checkXPikiQuotasAtURL(
	ctx context.Context,
	credentials []XPikiQuotaCredential,
	endpoint string,
	logsEndpoint ...string,
) XPikiQuotaReport {
	report := XPikiQuotaReport{
		Keys:             make([]XPikiQuotaKeyResult, len(credentials)),
		ModelProfiles:    make([]XPikiQuotaModelProfile, 0),
		NoModelPriceKeys: make([]string, 0),
	}
	if len(credentials) == 0 {
		report.TotalRemaining = "—"
		report.TotalRemainingVND = "—"
		return report
	}

	jobs := make(chan int)
	results := make([]xpikiCheckResult, len(credentials))
	workerCount := min(xpikiMaxWorkers, len(credentials))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Go(func() {
			for index := range jobs {
				results[index] = checkXPikiCredential(ctx, credentials[index], endpoint)
				if len(logsEndpoint) > 0 && results[index].profile != nil {
					verifyXPikiEffectiveModelPrices(ctx, credentials[index], logsEndpoint[0], results[index].profile)
				}
			}
		})
	}
	for index := range credentials {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	remainingTotal := decimal.Zero
	remainingVNDTotal := decimal.Zero
	totalCostKnown := true
	hasSuccessfulKey := false
	for index, result := range results {
		report.Keys[index] = result.key
		if result.key.Error != "" {
			report.TotalIncomplete = true
			totalCostKnown = false
			continue
		}
		hasSuccessfulKey = true
		if !result.costKnown {
			report.TotalIncomplete = true
			totalCostKnown = false
		}
		if result.key.Unlimited {
			report.TotalUnlimited = true
		} else {
			remainingTotal = remainingTotal.Add(result.remainingAstraToken)
			remainingVNDTotal = remainingVNDTotal.Add(result.remainingVND)
		}
		if result.profile == nil {
			report.NoModelPriceKeys = append(report.NoModelPriceKeys, result.key.Label)
			continue
		}
		addXPikiModelProfile(&report.ModelProfiles, *result.profile)
	}

	switch {
	case !hasSuccessfulKey:
		report.TotalRemaining = "—"
		report.TotalRemainingVND = "—"
	case report.TotalUnlimited:
		report.TotalRemaining = ""
		report.TotalRemainingVND = ""
	default:
		report.TotalRemaining = formatXPikiNumber(remainingTotal)
		report.TotalRemainingVND = "—"
		if totalCostKnown {
			report.TotalRemainingVND = formatXPikiNumber(remainingVNDTotal.Round(0)) + " ₫"
		}
	}

	return report
}

func checkXPikiCredential(
	ctx context.Context,
	credential XPikiQuotaCredential,
	endpoint string,
) xpikiCheckResult {
	keyResult := XPikiQuotaKeyResult{
		Label:          credential.Label,
		ChannelId:      credential.ChannelId,
		ChannelName:    credential.ChannelName,
		ChannelStatus:  credential.ChannelStatus,
		UsedAstra:      "—",
		LimitAstra:     "—",
		RemainingAstra: "—",
		UsedPercent:    "—",
		Status:         []string{},
	}
	apiKey := strings.TrimSpace(credential.APIKey)
	if apiKey == "" {
		keyResult.Error = "Empty API key"
		return xpikiCheckResult{key: keyResult}
	}

	client, err := GetHttpClientWithProxy("")
	if err != nil || client == nil {
		keyResult.Error = "Could not configure the upstream connection"
		return xpikiCheckResult{key: keyResult}
	}
	clientCopy := *client
	clientCopy.Timeout = 15 * time.Second
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		keyResult.Error = "Could not create the upstream request"
		return xpikiCheckResult{key: keyResult}
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := clientCopy.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			keyResult.Error = "Upstream request timed out"
		} else {
			keyResult.Error = "Could not connect to the upstream API"
		}
		return xpikiCheckResult{key: keyResult}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, xpikiMaxResponseBytes+1))
	if err != nil {
		keyResult.Error = "Could not read the upstream response"
		return xpikiCheckResult{key: keyResult}
	}
	if len(body) > xpikiMaxResponseBytes {
		keyResult.Error = "Upstream response is too large"
		return xpikiCheckResult{key: keyResult}
	}
	if response.StatusCode != http.StatusOK {
		keyResult.Error = xpikiHTTPError(response.StatusCode, body, apiKey)
		return xpikiCheckResult{key: keyResult}
	}

	var envelope xpikiResponse
	if err := common.Unmarshal(body, &envelope); err != nil {
		keyResult.Error = "Upstream returned invalid JSON"
		return xpikiCheckResult{key: keyResult}
	}
	if envelope.Code == nil {
		keyResult.Error = "Upstream returned invalid status data"
		return xpikiCheckResult{key: keyResult}
	}
	if *envelope.Code != 0 {
		keyResult.Error = sanitizeXPikiText(envelope.Message, apiKey, 200)
		if keyResult.Error == "" {
			keyResult.Error = "Upstream rejected the request"
		}
		return xpikiCheckResult{key: keyResult}
	}

	var data xpikiStatusData
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" || common.Unmarshal(envelope.Data, &data) != nil {
		keyResult.Error = "Upstream returned invalid quota data"
		return xpikiCheckResult{key: keyResult}
	}
	used, err := parseXPikiDecimal(data.QuotaUsed)
	if err != nil || used.IsNegative() {
		keyResult.Error = "Upstream returned an invalid quota value"
		return xpikiCheckResult{key: keyResult}
	}
	limit, err := parseXPikiDecimal(data.QuotaLimit)
	if err != nil {
		keyResult.Error = "Upstream returned an invalid quota value"
		return xpikiCheckResult{key: keyResult}
	}

	keyResult.Name = sanitizeXPikiText(data.Name, apiKey, 60)
	if keyResult.Name == "" {
		keyResult.Name = sanitizeXPikiText(data.KeyPrefix, apiKey, 60)
	}

	costModel := xpikiPoolReferenceModel(data.Conversion)
	calcVND := func(q decimal.Decimal) string {
		vnd, known := xpikiQuotaToVND(q, costModel)
		if !known {
			return ""
		}
		return formatXPikiNumber(vnd.Round(0)) + " ₫"
	}

	keyResult.UsedAstra = formatXPikiNumber(used.Mul(decimal.NewFromInt(xpikiAstraTokenMultiple)))
	keyResult.UsedQuota = used.IntPart()
	keyResult.UsedVND = calcVND(used)

	remainingAstraToken := decimal.Zero
	remainingQuota := decimal.Zero
	if limit.Sign() <= 0 {
		keyResult.Unlimited = true
		keyResult.LimitVND = "Không giới hạn"
		keyResult.RemainingVND = "Không giới hạn"
	} else {
		remaining := limit.Sub(used)
		if remaining.IsNegative() {
			remaining = decimal.Zero
		}
		percent := used.DivRound(limit, 8).Mul(decimal.NewFromInt(100)).Round(1)
		keyResult.LimitAstra = formatXPikiNumber(limit.Mul(decimal.NewFromInt(xpikiAstraTokenMultiple)))
		keyResult.RemainingAstra = formatXPikiNumber(remaining.Mul(decimal.NewFromInt(xpikiAstraTokenMultiple)))
		keyResult.UsedPercent = formatXPikiNumber(percent) + "%"

		keyResult.LimitQuota = limit.IntPart()
		keyResult.RemainingQuota = remaining.IntPart()
		keyResult.LimitVND = calcVND(limit)
		keyResult.RemainingVND = calcVND(remaining)

		remainingAstraToken = remaining.Mul(decimal.NewFromInt(xpikiAstraTokenMultiple))
		remainingQuota = remaining
	}

	if data.Enabled != nil && !*data.Enabled {
		keyResult.Status = append(keyResult.Status, "disabled")
	}
	if data.StatusFlags.Expired {
		keyResult.Status = append(keyResult.Status, "expired")
	}
	if data.StatusFlags.QuotaExhausted {
		keyResult.Status = append(keyResult.Status, "quota_exhausted")
	}
	if len(keyResult.Status) == 0 {
		keyResult.Status = append(keyResult.Status, "ok")
	}

	profile := buildXPikiModelProfile(credential.Label, data.Conversion, apiKey)
	groupRatio, groupErr := parseXPikiDecimal(data.Conversion.GroupRatio)
	if groupErr == nil && groupRatio.Sign() > 0 && costModel != "" {
		for _, entry := range data.Conversion.Models {
			if entry.Slug != xpikiAstraModelSlug && entry.Slug != "claude-opus-5-5" {
				continue
			}
			price, err := parseXPikiDecimal(entry.InputPricePer1M)
			if err != nil || price.Sign() <= 0 {
				continue
			}
			keyResult.ReferenceModel = sanitizeXPikiText(entry.Slug, apiKey, 100)
			toTokens := func(q decimal.Decimal) string {
				return formatXPikiNumber(q.Mul(decimal.NewFromInt(1000000)).Div(price.Mul(groupRatio)).Round(0))
			}
			keyResult.UsedTokens = toTokens(used)
			keyResult.LimitTokens = toTokens(limit)
			keyResult.RemainingTokens = toTokens(remainingQuota)
			break
		}
	}
	remainingVND, costKnown := xpikiQuotaToVND(remainingQuota, costModel)
	return xpikiCheckResult{
		key:                 keyResult,
		profile:             profile,
		remainingVND:        remainingVND,
		costKnown:           costKnown,
		remainingAstraToken: remainingAstraToken,
	}
}

func xpikiHTTPError(statusCode int, body []byte, apiKey string) string {
	var payload struct {
		Message string `json:"message"`
	}
	if common.Unmarshal(body, &payload) == nil {
		if message := sanitizeXPikiText(payload.Message, apiKey, 200); message != "" {
			return fmt.Sprintf("HTTP %d: %s", statusCode, message)
		}
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}

func parseXPikiDecimal(raw json.RawMessage) (decimal.Decimal, error) {
	if len(raw) == 0 || common.GetJsonType(raw) == "null" {
		return decimal.Zero, errors.New("missing number")
	}
	value := string(raw)
	if common.GetJsonType(raw) == "string" {
		if err := common.Unmarshal(raw, &value); err != nil {
			return decimal.Zero, errors.New("invalid number")
		}
	}
	value = strings.TrimSpace(value)
	if len(value) > xpikiMaxNumericChars {
		return decimal.Zero, errors.New("invalid number")
	}
	number, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, errors.New("invalid number")
	}
	if number.Exponent() < -xpikiMaxDecimalExponent ||
		number.Exponent() > xpikiMaxDecimalExponent ||
		len(number.Coefficient().String()) > xpikiMaxSignificantDigits {
		return decimal.Zero, errors.New("invalid number")
	}
	return number, nil
}

func buildXPikiModelProfile(
	label string,
	conversion xpikiConversion,
	apiKey string,
) *XPikiQuotaModelProfile {
	groupRatio, err := parseXPikiDecimal(conversion.GroupRatio)
	hasGroupRatio := err == nil && groupRatio.Sign() > 0
	groupRatioText := "—"
	if hasGroupRatio {
		groupRatioText = formatXPikiNumber(groupRatio)
	}

	pricedModels := make([]xpikiPricedModel, 0, len(conversion.Models))
	var astraPrice decimal.Decimal
	for _, model := range conversion.Models {
		price, err := parseXPikiDecimal(model.InputPricePer1M)
		if err != nil || price.IsNegative() {
			price = decimal.Zero
		}
		name := sanitizeXPikiText(model.DisplayName, apiKey, 100)
		if name == "" {
			name = sanitizeXPikiText(model.Slug, apiKey, 100)
		}
		if name == "" {
			name = "Model"
		}
		outputPrice, _ := parseXPikiDecimal(model.OutputPricePer1M)
		pricedModels = append(pricedModels, xpikiPricedModel{name: name, slug: model.Slug, price: price, outputPrice: outputPrice})
		if model.Slug == xpikiAstraModelSlug {
			astraPrice = price
		}
	}
	rows := make([]XPikiQuotaModelRow, 0, len(pricedModels))
	for _, model := range pricedModels {
		priceVNDText := "—"
		tokensPerQuotaText := "—"
		tokensForPackage := "—"
		outputVND := "—"
		cacheReadVND := ""
		if hasGroupRatio && model.price.Sign() > 0 {
			priceVND, priceKnown := xpikiQuotaToVND(model.price.Mul(groupRatio), model.slug)
			if priceKnown && priceVND.Sign() > 0 {
				priceVNDText = formatXPikiNumber(priceVND.Round(4))
				tokensForPackage = formatXPikiNumber(decimal.NewFromInt(xpikiPackagePriceVND).Mul(decimal.NewFromInt(1000000)).Div(priceVND).Round(0))
				if xpikiVerifiedCacheReadModel(model.slug) {
					cacheReadVND = formatXPikiNumber(priceVND.Div(decimal.NewFromInt(2)).Round(4))
				}
			}
			tokensPerQuota := decimal.NewFromInt(1_000_000).
				DivRound(model.price.Mul(groupRatio), 28).
				Round(0)
			tokensPerQuotaText = formatXPikiNumber(tokensPerQuota)
			if model.outputPrice.Sign() > 0 {
				if price, known := xpikiQuotaToVND(model.outputPrice.Mul(groupRatio), model.slug); known {
					outputVND = formatXPikiNumber(price.Round(4))
				}
			}
		}
		rows = append(rows, XPikiQuotaModelRow{
			Model:               model.name,
			ModelSlug:           sanitizeXPikiText(model.slug, apiKey, 100),
			PriceVNDPer1M:       priceVNDText,
			TokensPerQuota:      tokensPerQuotaText,
			OutputPriceVNDPer1M: outputVND,
			CacheReadVNDPer1M:   cacheReadVND,
			TokensForPackage:    tokensForPackage,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	slices.SortFunc(rows, func(a, b XPikiQuotaModelRow) int {
		return strings.Compare(strings.ToLower(a.Model), strings.ToLower(b.Model))
	})

	priceBasis := "Chưa xác định được mốc giá vốn của key XPiki"
	switch xpikiPoolReferenceModel(conversion) {
	case xpikiAstraModelSlug:
		priceBasis = "Mốc GPT: 1.040 VND/1M token Astra; các model GPT quy đổi theo tỷ lệ giá với Astra"
	case xpikiOpusModelSlug:
		priceBasis = "Mốc Claude: 260 VND/1M token Opus 5.5; các model Claude quy đổi theo tỷ lệ giá với Opus"
	}
	astraVND, _ := xpikiQuotaToVND(astraPrice, xpikiAstraModelSlug)
	return &XPikiQuotaModelProfile{
		Labels:             []string{label},
		GroupRatio:         groupRatioText,
		AstraPriceVNDPer1M: formatXPikiNumber(astraVND.Round(4)),
		Models:             rows,
		PriceBasis:         priceBasis,
		CachePriceBasis:    "Cache-read đã đối chiếu ledger được tính bằng 50% input cơ bản. Model chưa đối chiếu và cache-write chưa có giá xác nhận để trống; không phải miễn phí.",
		conversion:         conversion,
	}
}

func addXPikiModelProfile(profiles *[]XPikiQuotaModelProfile, candidate XPikiQuotaModelProfile) {
	for index := range *profiles {
		current := &(*profiles)[index]
		if !sameXPikiModelProfile(*current, candidate) {
			continue
		}
		current.Labels = append(current.Labels, candidate.Labels...)
		return
	}
	*profiles = append(*profiles, candidate)
}

func sameXPikiModelProfile(a, b XPikiQuotaModelProfile) bool {
	if a.GroupRatio != b.GroupRatio || a.AstraPriceVNDPer1M != b.AstraPriceVNDPer1M || a.PriceBasis != b.PriceBasis || a.PricingWarning != b.PricingWarning || len(a.Models) != len(b.Models) {
		return false
	}
	for index := range a.Models {
		if !reflect.DeepEqual(a.Models[index], b.Models[index]) {
			return false
		}
	}
	return true
}

func sanitizeXPikiText(value, apiKey string, maxRunes int) string {
	if apiKey != "" {
		value = strings.ReplaceAll(value, apiKey, "[redacted]")
	}
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, value))
	if maxRunes > 0 {
		runes := []rune(value)
		if len(runes) > maxRunes {
			value = string(runes[:maxRunes])
		}
	}
	return value
}

func formatXPikiNumber(value decimal.Decimal) string {
	text := value.String()
	negative := strings.HasPrefix(text, "-")
	if negative {
		text = strings.TrimPrefix(text, "-")
	}
	whole, fraction, hasFraction := strings.Cut(text, ".")
	var grouped strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	whole = grouped.String()
	if hasFraction {
		fraction = strings.TrimRight(fraction, "0")
		if fraction != "" {
			whole += "." + fraction
		}
	}
	if negative {
		return "-" + whole
	}
	return whole
}

func MaskXPikiKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 12 {
		return "***"
	}
	return key[:7] + "..." + key[len(key)-4:]
}

func FormatXPikiCostVND(costDecimal decimal.Decimal) string {
	return FormatXPikiCostVNDForModel(costDecimal, "")
}

func FormatXPikiCostVNDForModel(costDecimal decimal.Decimal, modelSlug string) string {
	if costDecimal.IsNegative() {
		return ""
	}
	if costDecimal.IsZero() {
		return "0 ₫"
	}
	// The model's input/output/cache/multiplier is already in credit_cost.
	// Applying another model discount here would count it twice.
	tokens, reference := xpikiChargedReferenceTokens(costDecimal, modelSlug)
	vnd, known := xpikiTokenCost(tokens, reference)
	if !known {
		return ""
	}
	if vnd.GreaterThanOrEqual(decimal.NewFromInt(1000)) {
		return formatXPikiNumber(vnd.Round(2)) + " ₫"
	}
	return formatXPikiNumber(vnd.Round(4)) + " ₫"
}

func xpikiQuotaToVND(quota decimal.Decimal, modelSlug string) (decimal.Decimal, bool) {
	tokens, reference := xpikiChargedReferenceTokens(quota, modelSlug)
	return xpikiTokenCost(tokens, reference)
}

func xpikiTokenCost(tokens decimal.Decimal, modelSlug string) (decimal.Decimal, bool) {
	switch xpikiCostReferenceModel(modelSlug) {
	case xpikiAstraModelSlug:
		return tokens.Mul(decimal.NewFromInt(xpikiAstraPriceVNDPer1M)).DivRound(decimal.NewFromInt(1000000), 64), true
	case xpikiOpusModelSlug:
		return tokens.Mul(decimal.NewFromInt(xpikiOpusPriceVNDPer1M)).DivRound(decimal.NewFromInt(1000000), 64), true
	default:
		return decimal.Zero, false
	}
}

// Receipt fees include output/cache and provider multipliers. These are paid
// reference-token equivalents, not the request's raw input/output token count.
func xpikiChargedReferenceTokens(fee decimal.Decimal, modelSlug string) (decimal.Decimal, string) {
	reference := xpikiCostReferenceModel(modelSlug)
	var units int64
	switch reference {
	case xpikiAstraModelSlug:
		units = xpikiAstraQuotaPer1M
	case xpikiOpusModelSlug:
		units = xpikiOpusQuotaPer1M
	default:
		return decimal.Zero, ""
	}
	return fee.Mul(decimal.NewFromInt(1000000)).DivRound(decimal.NewFromInt(units), 64), reference
}

// Confirmed against current successful XPiki cache-read receipts. Do not
// project these tariffs onto other models or invent a cache-write price.
func xpikiVerifiedCacheReadModel(slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slash := strings.LastIndex(slug, "/"); slash >= 0 {
		slug = slug[slash+1:]
	}
	switch slug {
	case "gpt-6.1-sol", "gpt-6-sol", "gpt-5.6-sol", "gpt-6-astra", "gpt-5.6-luna", "claude-opus-5-5", "claude-opus-5.5":
		return true
	default:
		return false
	}
}

func xpikiCostReferenceModel(modelSlug string) string {
	slug := strings.ToLower(strings.TrimSpace(modelSlug))
	if slash := strings.LastIndex(slug, "/"); slash >= 0 {
		slug = slug[slash+1:]
	}
	switch {
	case strings.HasPrefix(slug, "gpt-"):
		return xpikiAstraModelSlug
	case strings.HasPrefix(slug, "claude-"):
		return xpikiOpusModelSlug
	default:
		return ""
	}
}

// A key balance needs one purchase pool. Mixed or unknown catalogs must not
// guess a currency conversion even when individual model fees are known.
func xpikiPoolReferenceModel(conversion xpikiConversion) string {
	reference := ""
	for _, entry := range conversion.Models {
		modelReference := xpikiCostReferenceModel(entry.Slug)
		if modelReference == "" || (reference != "" && reference != modelReference) {
			return ""
		}
		reference = modelReference
	}
	return reference
}

func FetchXPikiLogsReport(
	ctx context.Context,
	credentials []XPikiQuotaCredential,
	filter XPikiLogsFilter,
) (XPikiLogsReport, error) {
	if filter.Source == "newapi" {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if model.LOG_DB == nil {
			return XPikiLogsReport{}, errors.New("NewAPI request log database unavailable")
		}
		report, err := fetchLocalRequestLogsReport(ctx, credentials, filter)
		if err != nil {
			return report, err
		}
		enrichLocalRequestCosts(ctx, &report, credentials, XPikiKeyLogsURL, XPikiKeyStatusURL)
		summarizeXPikiTokenCosts(&report)
		return report, nil
	}
	if filter.Source != "" && filter.Source != "xpiki" {
		return XPikiLogsReport{}, errors.New("invalid log source")
	}
	report, err := fetchXPikiLogsReportAtURL(ctx, credentials, XPikiKeyLogsURL, filter)
	if err == nil {
		enrichXPikiReportTokenCosts(ctx, &report, credentials, XPikiKeyStatusURL)
	}
	return report, err
}

func extractDestination(baseURL string) string {
	u := strings.TrimSpace(baseURL)
	if u == "" {
		return "api.openai.com"
	}
	parsed, err := url.Parse(u)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	trimmed := strings.TrimPrefix(u, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if slashIdx := strings.Index(trimmed, "/"); slashIdx != -1 {
		trimmed = trimmed[:slashIdx]
	}
	if trimmed != "" {
		return trimmed
	}
	return u
}

func IsXPikiBaseURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && parsed.User == nil && strings.EqualFold(parsed.Scheme, "https") &&
		strings.EqualFold(parsed.Hostname(), "api.xpiki.com") && (parsed.Port() == "" || parsed.Port() == "443")
}

// A failed attempt is not a terminal request. Older rows without terminal
// evidence stay unknown; settlement alone cannot prove a stream completed.
func localRequestOutcome(logType int, stream bool, content, metadata string, failedAttempts int) string {
	var info struct {
		Final     bool  `json:"request_final"`
		Cancelled *bool `json:"request_cancelled"`
		Stream    *struct {
			Status           string `json:"status"`
			Reason           string `json:"end_reason"`
			TerminalEvidence string `json:"terminal_evidence"`
		} `json:"stream_status"`
	}
	if metadata != "" && json.Unmarshal([]byte(metadata), &info) != nil {
		return "unknown"
	}
	if logType == model.LogTypeError {
		if !info.Final {
			return "unknown"
		}
		lower := strings.ToLower(content)
		if info.Cancelled != nil && *info.Cancelled || info.Cancelled == nil && (strings.Contains(lower, "cancel") || strings.Contains(lower, "abort")) {
			return "cancelled"
		}
		return "failed_final"
	}
	if logType != model.LogTypeConsume {
		return "unknown"
	}
	if stream {
		if info.Stream == nil {
			return "unknown"
		}
		if info.Stream.Reason == "client_gone" {
			return "cancelled"
		}
		if info.Stream.Status == "error" {
			return "stream_failed"
		}
		evidence := strings.ToLower(info.Stream.TerminalEvidence)
		if strings.Contains(evidence, "incomplete") {
			return "stream_incomplete"
		}
		if strings.Contains(evidence, "failed") || strings.Contains(evidence, "error") {
			return "stream_failed"
		}
		hasTerminalEvidence := info.Stream.Reason == "done" ||
			(info.Stream.TerminalEvidence != "" && (info.Stream.Reason == "eof" || info.Stream.Reason == "handler_stop"))
		if info.Stream.Status != "ok" || !hasTerminalEvidence {
			return "unknown"
		}
	}
	if failedAttempts > 0 {
		return "retry_recovered"
	}
	return "success"
}

// RequestHealth is a bounded, local-only operational view, never a billing
// ledger. Unknown outcomes are deliberately excluded from final-failure rates.
type RequestHealth struct {
	StartAt  int64          `json:"start_at"`
	EndAt    int64          `json:"end_at"`
	Attempts int            `json:"attempts"`
	Requests int            `json:"requests"`
	Capped   bool           `json:"capped"`
	Outcomes map[string]int `json:"outcomes"`
	Failures map[string]int `json:"failures"`
	Alerts   []string       `json:"alerts"`
}
