package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Quantitative pricing tests required by user specifications:
// - Claude Opus (claude-opus-4-6, claude-opus-4-7, claude-opus-4-8, claude-opus-5, claude-opus-5-5):
//   Input: 1,500 VND / 1M
//   Cache-read: 750 VND / 1M
//   Output: 7,500 VND / 1M
//   1M input + 1M output = 9,000 VND
//   500k input + 500k cache-read + 1M output = 8,625 VND
//
// - GPT-6.1 Sol (gpt-6.1-sol):
//   Input: 700 VND / 1M
//   Cache-read: 350 VND / 1M
//   Output: 3,500 VND / 1M
//   1M input + 1M output = 4,200 VND
//   500k input + 500k cache-read + 1M output = 4,025 VND
func TestSellingPricingQuantitativeOpus(t *testing.T) {
	const exchangeRate = 26000.0
	const quotaPerUnit = 500000.0

	opusExprStr := `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`

	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		GroupRatio:   1.0,
		QuotaPerUnit: quotaPerUnit,
		ExprString:   opusExprStr,
		ExprHash:     billingexpr.ExprHashString(opusExprStr),
		ExprVersion:  1,
	}

	usedVars := map[string]bool{"p": true, "c": true, "cr": true, "cc": true}

	t.Run("Opus 1M input + 1M output = 9,000 VND (Claude Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     1000000,
			CompletionTokens: 1000000,
			UsageSemantic:    "anthropic",
		}
		params := BuildTieredTokenParams(usage, true, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-9000.0) > 0.5 {
			t.Errorf("Expected 9000 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 9000 {
			t.Errorf("Rounded VND expected 9000, got %d", int(math.Round(vnd)))
		}
	})

	t.Run("Opus 1M input + 1M output = 9,000 VND (OpenAI Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     1000000,
			CompletionTokens: 1000000,
		}
		params := BuildTieredTokenParams(usage, false, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-9000.0) > 0.5 {
			t.Errorf("Expected 9000 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 9000 {
			t.Errorf("Rounded VND expected 9000, got %d", int(math.Round(vnd)))
		}
	})

	t.Run("Opus 500k input + 500k cache-read + 1M output = 8,625 VND (Claude Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     500000, // In Claude, input_tokens excludes cache read
			CompletionTokens: 1000000,
			UsageSemantic:    "anthropic",
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 500000,
			},
		}
		params := BuildTieredTokenParams(usage, true, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-8625.0) > 0.5 {
			t.Errorf("Expected 8625 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 8625 {
			t.Errorf("Rounded VND expected 8625, got %d", int(math.Round(vnd)))
		}
	})

	t.Run("Opus 500k input + 500k cache-read + 1M output = 8,625 VND (OpenAI Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     1000000, // In OpenAI, prompt_tokens includes cached_tokens
			CompletionTokens: 1000000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 500000,
			},
		}
		params := BuildTieredTokenParams(usage, false, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-8625.0) > 0.5 {
			t.Errorf("Expected 8625 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 8625 {
			t.Errorf("Rounded VND expected 8625, got %d", int(math.Round(vnd)))
		}
	})
}

func TestSellingPricingQuantitativeSol(t *testing.T) {
	const exchangeRate = 26000.0
	const quotaPerUnit = 500000.0

	solExprStr := `tier("base", p * 0.026923076923076924 + c * 0.13461538461538462 + cr * 0.013461538461538462 + cc * 0.008)`

	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		GroupRatio:   1.0,
		QuotaPerUnit: quotaPerUnit,
		ExprString:   solExprStr,
		ExprHash:     billingexpr.ExprHashString(solExprStr),
		ExprVersion:  1,
	}

	usedVars := map[string]bool{"p": true, "c": true, "cr": true, "cc": true}

	t.Run("Sol 1M input + 1M output = 4,200 VND (OpenAI Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     1000000,
			CompletionTokens: 1000000,
		}
		params := BuildTieredTokenParams(usage, false, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-4200.0) > 0.5 {
			t.Errorf("Expected 4200 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 4200 {
			t.Errorf("Rounded VND expected 4200, got %d", int(math.Round(vnd)))
		}
	})

	t.Run("Sol 500k input + 500k cache-read + 1M output = 4,025 VND (OpenAI Semantics)", func(t *testing.T) {
		usage := &dto.Usage{
			PromptTokens:     1000000,
			CompletionTokens: 1000000,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 500000,
			},
		}
		params := BuildTieredTokenParams(usage, false, usedVars)
		result, err := billingexpr.ComputeTieredQuota(snap, params)
		if err != nil {
			t.Fatalf("ComputeTieredQuota failed: %v", err)
		}

		vnd := float64(result.ActualQuotaAfterGroup) / quotaPerUnit * exchangeRate
		if math.Abs(vnd-4025.0) > 0.5 {
			t.Errorf("Expected 4025 VND, got %f", vnd)
		}
		if int(math.Round(vnd)) != 4025 {
			t.Errorf("Rounded VND expected 4025, got %d", int(math.Round(vnd)))
		}
	})
}
