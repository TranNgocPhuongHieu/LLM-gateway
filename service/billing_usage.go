package service

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
)

const (
	usageBillingPathLocal              = "local"
	usageBillingPathUpstream           = "upstream"
	usageBillingPathOpenAI             = "billing-usage-openai"
	usageBillingPathOpenAIEstimated    = "billing-usage-openai-estimated"
	usageBillingPathAnthropic          = "billing-usage-anthropic"
	usageBillingPathAnthropicEstimated = "billing-usage-anthropic-estimated"
	usageBillingPathGemini             = "billing-usage-gemini"
	usageBillingPathGeminiEstimated    = "billing-usage-gemini-estimated"
)

func effectiveBillingUsage(usage *dto.Usage) *dto.Usage {
	if billingUsage, ok := usageFromBillingUsage(usage); ok {
		return billingUsage
	}
	return usage
}

// Cache creation is ordinary input. Only add it when the provider reports it
// separately; OpenAI input already includes it. Preserve the upstream payload.
func inputOnlyCacheWriteUsage(usage *dto.Usage, separateInput bool) *dto.Usage {
	if usage == nil {
		return usage
	}
	normalized := *usage
	writeTokens := usage.PromptTokensDetails.CacheCreationTokensTotal()
	writeTokens = max(writeTokens, usage.ClaudeCacheCreation5mTokens+usage.ClaudeCacheCreation1hTokens)
	if separateInput {
		normalized.PromptTokens += writeTokens
		normalized.UsageSemantic = dto.BillingUsageSemanticAnthropic
	} else if normalized.UsageSemantic == dto.BillingUsageSemanticAnthropic {
		// Legacy OpenRouter usage carries inclusive input despite its label.
		normalized.UsageSemantic = dto.BillingUsageSemanticOpenAI
	}
	normalized.InputTokens = normalized.PromptTokens
	if separateInput {
		normalized.InputTokens += normalized.PromptTokensDetails.CachedTokens
	}
	normalized.TotalTokens = normalized.InputTokens + normalized.CompletionTokens
	normalized.PromptTokensDetails.CachedCreationTokens = 0
	normalized.PromptTokensDetails.CacheWriteTokens = 0
	normalized.ClaudeCacheCreation5mTokens = 0
	normalized.ClaudeCacheCreation1hTokens = 0
	if usage.InputTokensDetails != nil {
		details := *usage.InputTokensDetails
		details.CachedCreationTokens = 0
		details.CacheWriteTokens = 0
		normalized.InputTokensDetails = &details
	}
	normalized.BillingUsage = dto.CloneBillingUsage(usage.BillingUsage)
	if normalized.BillingUsage != nil && normalized.BillingUsage.ClaudeUsage != nil {
		claudeUsage := normalized.BillingUsage.ClaudeUsage
		claudeUsage.InputTokens = normalized.PromptTokens
		claudeUsage.CacheCreationInputTokens = 0
		claudeUsage.ClaudeCacheCreation5mTokens = 0
		claudeUsage.ClaudeCacheCreation1hTokens = 0
		claudeUsage.CacheCreation = nil
	}
	if normalized.BillingUsage != nil && normalized.BillingUsage.OpenAIUsage != nil {
		openAIUsage := normalized.BillingUsage.OpenAIUsage
		openAIUsage.PromptTokensDetails.CachedCreationTokens = 0
		openAIUsage.PromptTokensDetails.CacheWriteTokens = 0
		openAIUsage.ClaudeCacheCreation5mTokens = 0
		openAIUsage.ClaudeCacheCreation1hTokens = 0
		if openAIUsage.InputTokensDetails != nil {
			openAIUsage.InputTokensDetails.CachedCreationTokens = 0
			openAIUsage.InputTokensDetails.CacheWriteTokens = 0
		}
	}
	return &normalized
}

func usageBillingPathForLog(isLocalCountTokens bool, usage *dto.Usage) string {
	effectiveUsage, ok := usageFromBillingUsage(usage)
	if !ok {
		if isLocalCountTokens {
			return usageBillingPathLocal
		}
		return usageBillingPathUpstream
	}

	switch effectiveUsage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI:
		if usage.BillingUsage.Estimated {
			return usageBillingPathOpenAIEstimated
		}
		return usageBillingPathOpenAI
	case dto.BillingUsageSemanticAnthropic:
		if usage.BillingUsage.Estimated {
			return usageBillingPathAnthropicEstimated
		}
		return usageBillingPathAnthropic
	case dto.BillingUsageSemanticGemini:
		if usage.BillingUsage.Estimated {
			return usageBillingPathGeminiEstimated
		}
		return usageBillingPathGemini
	}

	return usageBillingPathUpstream
}

func appendUsageBillingPathForLog(other *model.LogOther, isLocalCountTokens bool, usage *dto.Usage) {
	if other == nil {
		return
	}
	other.SetAdmin("usage_billing_path", usageBillingPathForLog(isLocalCountTokens, usage))
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
