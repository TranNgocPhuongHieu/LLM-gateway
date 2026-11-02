package controller

import (
	"fmt"
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/billing_setting"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func buildDifferences(localData map[string]any, successfulChannels []struct {
	name string
	data map[string]any
}) map[string]map[string]dto.DifferenceItem {
	differences := make(map[string]map[string]dto.DifferenceItem)
	localData = effectivePricingSyncData(localData)
	normalizedChannels := make([]struct {
		name string
		data map[string]any
	}, 0, len(successfulChannels))
	for _, channel := range successfulChannels {
		channel.data = effectivePricingSyncData(channel.data)
		normalizedChannels = append(normalizedChannels, channel)
	}
	successfulChannels = normalizedChannels

	allModels := make(map[string]struct{})

	for _, field := range pricingSyncFields {
		for modelName := range valueMap(localData[field]) {
			allModels[modelName] = struct{}{}
		}
	}

	for _, channel := range successfulChannels {
		for _, field := range pricingSyncFields {
			for modelName := range valueMap(channel.data[field]) {
				allModels[modelName] = struct{}{}
			}
		}
	}

	confidenceMap := make(map[string]map[string]bool)

	// 预处理阶段：检查pricing接口的可信度
	for _, channel := range successfulChannels {
		confidenceMap[channel.name] = make(map[string]bool)

		modelRatios := valueMap(channel.data["model_ratio"])
		completionRatios := valueMap(channel.data["completion_ratio"])

		if len(modelRatios) > 0 && len(completionRatios) > 0 {
			// 遍历所有模型，检查是否满足不可信条件
			for modelName := range allModels {
				// 默认为可信
				confidenceMap[channel.name][modelName] = true

				// 检查是否满足不可信条件：model_ratio为37.5且completion_ratio为1
				if modelRatioVal, ok := modelRatios[modelName]; ok {
					if completionRatioVal, ok := completionRatios[modelName]; ok {
						// 转换为float64进行比较
						modelRatioFloat, modelRatioOK := asFloat64(modelRatioVal)
						completionRatioFloat, completionRatioOK := asFloat64(completionRatioVal)
						if modelRatioOK && completionRatioOK && nearlyEqual(modelRatioFloat, 37.5) && nearlyEqual(completionRatioFloat, 1.0) {
							confidenceMap[channel.name][modelName] = false
						}
					}
				}
			}
		} else {
			// 如果不是从pricing接口获取的数据，则全部标记为可信
			for modelName := range allModels {
				confidenceMap[channel.name][modelName] = true
			}
		}
	}

	for modelName := range allModels {
		expressionPriority := valueMap(localData[billing_setting.BillingModeField])[modelName] == billing_setting.BillingModeTieredExpr
		for _, channel := range successfulChannels {
			if valueMap(channel.data[billing_setting.BillingModeField])[modelName] == billing_setting.BillingModeTieredExpr {
				expressionPriority = true
			}
		}
		for _, ratioType := range pricingSyncFields {
			if expressionPriority && numericPricingSyncFields[ratioType] {
				continue
			}
			var localValue any = nil
			if val, exists := valueMap(localData[ratioType])[modelName]; exists {
				localValue = normalizeSyncValue(ratioType, val)
			}

			upstreamValues := make(map[string]any)
			confidenceValues := make(map[string]bool)
			hasUpstreamValue := false
			hasDifference := false

			for _, channel := range successfulChannels {
				if expressionPriority && valueMap(channel.data[billing_setting.BillingModeField])[modelName] != billing_setting.BillingModeTieredExpr {
					continue
				}
				var upstreamValue any = nil

				if val, exists := valueMap(channel.data[ratioType])[modelName]; exists {
					upstreamValue = normalizeSyncValue(ratioType, val)
					hasUpstreamValue = true

					if localValue != nil && !valuesEqual(localValue, upstreamValue) {
						hasDifference = true
					} else if valuesEqual(localValue, upstreamValue) {
						upstreamValue = "same"
					}
				}
				if upstreamValue == nil && localValue == nil {
					upstreamValue = "same"
				}

				if localValue == nil && upstreamValue != nil && upstreamValue != "same" {
					hasDifference = true
				}

				upstreamValues[channel.name] = upstreamValue

				confidenceValues[channel.name] = confidenceMap[channel.name][modelName]
			}

			shouldInclude := false

			if localValue != nil {
				if hasDifference {
					shouldInclude = true
				}
			} else {
				if hasUpstreamValue {
					shouldInclude = true
				}
			}

			if shouldInclude {
				if differences[modelName] == nil {
					differences[modelName] = make(map[string]dto.DifferenceItem)
				}
				differences[modelName][ratioType] = dto.DifferenceItem{
					Current:    localValue,
					Upstreams:  upstreamValues,
					Confidence: confidenceValues,
				}
			}
		}
	}

	channelHasDiff := make(map[string]bool)
	for _, ratioMap := range differences {
		for _, item := range ratioMap {
			for chName, val := range item.Upstreams {
				if val != nil && val != "same" {
					channelHasDiff[chName] = true
				}
			}
		}
	}

	for modelName, ratioMap := range differences {
		for ratioType, item := range ratioMap {
			for chName := range item.Upstreams {
				if !channelHasDiff[chName] {
					delete(item.Upstreams, chName)
					delete(item.Confidence, chName)
				}
			}

			allSame := true
			for _, v := range item.Upstreams {
				if v != "same" {
					allSame = false
					break
				}
			}
			if len(item.Upstreams) == 0 || allSame {
				delete(ratioMap, ratioType)
			} else {
				differences[modelName][ratioType] = item
			}
		}

		if len(ratioMap) == 0 {
			delete(differences, modelName)
		}
	}

	return differences
}

func roundRatioValue(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}

func isModelsDevAPIEndpoint(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.ToLower(parsedURL.Hostname()) != modelsDevHost {
		return false
	}
	path := strings.TrimSuffix(parsedURL.Path, "/")
	if path == "" {
		path = "/"
	}
	return path == modelsDevPath
}

func convertOpenRouterToRatioData(reader io.Reader) (map[string]any, error) {
	var orResp struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt         string `json:"prompt"`
				Completion     string `json:"completion"`
				InputCacheRead string `json:"input_cache_read"`
			} `json:"pricing"`
		} `json:"data"`
	}

	if err := common.DecodeJson(reader, &orResp); err != nil {
		return nil, fmt.Errorf("failed to decode OpenRouter response: %w", err)
	}

	modelRatioMap := make(map[string]any)
	completionRatioMap := make(map[string]any)
	cacheRatioMap := make(map[string]any)

	for _, m := range orResp.Data {
		promptPrice, promptErr := strconv.ParseFloat(m.Pricing.Prompt, 64)
		completionPrice, compErr := strconv.ParseFloat(m.Pricing.Completion, 64)

		if promptErr != nil && compErr != nil {
			// Both unparseable — skip this model
			continue
		}

		// Treat parse errors as 0
		if promptErr != nil {
			promptPrice = 0
		}
		if compErr != nil {
			completionPrice = 0
		}

		// Negative values are sentinel values (e.g., -1 for dynamic/variable pricing) — skip
		if promptPrice < 0 || completionPrice < 0 {
			continue
		}

		if promptPrice == 0 && completionPrice == 0 {
			// Free model
			modelRatioMap[m.ID] = 0.0
			continue
		}
		if promptPrice <= 0 {
			// No meaningful prompt baseline, cannot derive ratios safely.
			continue
		}

		// Normal case: promptPrice > 0
		ratio := promptPrice * 1000 * ratio_setting.USD
		ratio = roundRatioValue(ratio)
		modelRatioMap[m.ID] = ratio

		compRatio := completionPrice / promptPrice
		compRatio = roundRatioValue(compRatio)
		completionRatioMap[m.ID] = compRatio

		// Convert input_cache_read to cache_ratio (= cache_read_price / prompt_price)
		if m.Pricing.InputCacheRead != "" {
			if cachePrice, err := strconv.ParseFloat(m.Pricing.InputCacheRead, 64); err == nil && cachePrice >= 0 {
				cacheRatio := cachePrice / promptPrice
				cacheRatio = roundRatioValue(cacheRatio)
				cacheRatioMap[m.ID] = cacheRatio
			}
		}
	}

	converted := make(map[string]any)
	if len(modelRatioMap) > 0 {
		converted["model_ratio"] = modelRatioMap
	}
	if len(completionRatioMap) > 0 {
		converted["completion_ratio"] = completionRatioMap
	}
	if len(cacheRatioMap) > 0 {
		converted["cache_ratio"] = cacheRatioMap
	}

	return converted, nil
}

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	Cost modelsDevCost `json:"cost"`
}

type modelsDevCost struct {
	Input     *float64 `json:"input"`
	Output    *float64 `json:"output"`
	CacheRead *float64 `json:"cache_read"`
}

type modelsDevCandidate struct {
	Provider  string
	Input     float64
	Output    *float64
	CacheRead *float64
}

func cloneFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func isValidNonNegativeCost(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	return v >= 0
}

func buildModelsDevCandidate(provider string, cost modelsDevCost) (modelsDevCandidate, bool) {
	if cost.Input == nil {
		return modelsDevCandidate{}, false
	}

	input := *cost.Input
	if !isValidNonNegativeCost(input) {
		return modelsDevCandidate{}, false
	}

	var output *float64
	if cost.Output != nil {
		if !isValidNonNegativeCost(*cost.Output) {
			return modelsDevCandidate{}, false
		}
		output = cloneFloatPtr(cost.Output)
	}

	// input=0/output>0 cannot be transformed into local ratio.
	if input == 0 && output != nil && *output > 0 {
		return modelsDevCandidate{}, false
	}

	var cacheRead *float64
	if cost.CacheRead != nil && isValidNonNegativeCost(*cost.CacheRead) {
		cacheRead = cloneFloatPtr(cost.CacheRead)
	}

	return modelsDevCandidate{
		Provider:  provider,
		Input:     input,
		Output:    output,
		CacheRead: cacheRead,
	}, true
}

func shouldReplaceModelsDevCandidate(current, next modelsDevCandidate) bool {
	currentNonZero := current.Input > 0
	nextNonZero := next.Input > 0
	if currentNonZero != nextNonZero {
		// Prefer non-zero pricing data; this matches "cheapest non-zero" conflict policy.
		return nextNonZero
	}
	if nextNonZero && !nearlyEqual(next.Input, current.Input) {
		return next.Input < current.Input
	}
	// Stable tie-breaker for deterministic result.
	return next.Provider < current.Provider
}

func convertModelsDevToRatioData(reader io.Reader) (map[string]any, error) {
	var upstreamData map[string]modelsDevProvider
	if err := common.DecodeJson(reader, &upstreamData); err != nil {
		return nil, fmt.Errorf("failed to decode models.dev response: %w", err)
	}
	if len(upstreamData) == 0 {
		return nil, fmt.Errorf("empty models.dev response")
	}

	providers := make([]string, 0, len(upstreamData))
	for provider := range upstreamData {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	selectedCandidates := make(map[string]modelsDevCandidate)
	for _, provider := range providers {
		providerData := upstreamData[provider]
		if len(providerData.Models) == 0 {
			continue
		}

		modelNames := make([]string, 0, len(providerData.Models))
		for modelName := range providerData.Models {
			modelNames = append(modelNames, modelName)
		}
		sort.Strings(modelNames)

		for _, modelName := range modelNames {
			candidate, ok := buildModelsDevCandidate(provider, providerData.Models[modelName].Cost)
			if !ok {
				continue
			}
			current, exists := selectedCandidates[modelName]
			if !exists || shouldReplaceModelsDevCandidate(current, candidate) {
				selectedCandidates[modelName] = candidate
			}
		}
	}

	if len(selectedCandidates) == 0 {
		return nil, fmt.Errorf("no valid models.dev pricing entries found")
	}

	modelRatioMap := make(map[string]any)
	completionRatioMap := make(map[string]any)
	cacheRatioMap := make(map[string]any)

	for modelName, candidate := range selectedCandidates {
		if candidate.Input == 0 {
			modelRatioMap[modelName] = 0.0
			continue
		}

		modelRatio := candidate.Input * float64(ratio_setting.USD) / modelsDevInputCostRatioBase
		modelRatioMap[modelName] = roundRatioValue(modelRatio)

		if candidate.Output != nil {
			completionRatio := *candidate.Output / candidate.Input
			completionRatioMap[modelName] = roundRatioValue(completionRatio)
		}

		if candidate.CacheRead != nil {
			cacheRatio := *candidate.CacheRead / candidate.Input
			cacheRatioMap[modelName] = roundRatioValue(cacheRatio)
		}
	}

	converted := make(map[string]any)
	if len(modelRatioMap) > 0 {
		converted["model_ratio"] = modelRatioMap
	}
	if len(completionRatioMap) > 0 {
		converted["completion_ratio"] = completionRatioMap
	}
	if len(cacheRatioMap) > 0 {
		converted["cache_ratio"] = cacheRatioMap
	}
	return converted, nil
}

func GetSyncableChannels(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var syncableChannels []dto.SyncableChannel
	for _, channel := range channels {
		if channel.GetBaseURL() != "" {
			syncableChannels = append(syncableChannels, dto.SyncableChannel{
				ID:      channel.Id,
				Name:    channel.Name,
				BaseURL: channel.GetBaseURL(),
				Status:  channel.Status,
				Type:    channel.Type,
			})
		}
	}

	syncableChannels = append(syncableChannels, dto.SyncableChannel{
		ID:      officialRatioPresetID,
		Name:    officialRatioPresetName,
		BaseURL: officialRatioPresetBaseURL,
		Status:  1,
	})

	syncableChannels = append(syncableChannels, dto.SyncableChannel{
		ID:      modelsDevPresetID,
		Name:    modelsDevPresetName,
		BaseURL: modelsDevPresetBaseURL,
		Status:  1,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    syncableChannels,
	})
}
