package service

import (
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/pkg/billingexpr"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	hosttypes "github.com/TranNgocPhuongHieu/LLM-gateway/types"
)

func TestLongContextPricingUsesTotalInputAndFrozenSnapshot(t *testing.T) {
	expression := `len <= 272000 ? tier("base", p * 1 + c * 2 + cr * 0.5 + cc * 1.25) : tier("long_context", p * (1 * 2) + c * (2 * 1.5) + cr * (0.5 * 2) + cc * (1.25 * 2))`
	used := map[string]bool{"p": true, "c": true, "cr": true, "cc": true, "len": true}
	for _, claude := range []bool{false, true} {
		for _, input := range []int{271999, 272000, 272001} {
			usage := &dto.Usage{PromptTokens: input, CompletionTokens: 300000}
			usage.PromptTokensDetails.CachedTokens = input - 1000
			if claude {
				usage.PromptTokens = 1000
			}
			params := BuildTieredTokenParams(usage, claude, used)
			if params.Len != float64(input) || params.P != 1000 {
				t.Fatalf("cache normalization wrong: %+v", params)
			}
			_, estimated, err := billingexpr.RunExpr(expression, billingexpr.TokenParams{P: float64(input), Len: float64(input), C: 8192})
			if err != nil {
				t.Fatal(err)
			}
			snap := &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 1000000, EstimatedTier: estimated.MatchedTier}
			info := &relaycommon.RelayInfo{TieredBillingSnapshot: snap, PriceData: hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			wantTier := "base"
			want := params.P + params.C*2 + params.CR*0.5
			if input > 272000 {
				wantTier = "long_context"
				want = params.P*2 + params.C*3 + params.CR
			}
			// Fallback attempts reuse the same frozen expression, not a new
			// current price or a repeatedly multiplied context factor.
			for attempt := 0; attempt < 3; attempt++ {
				frozen, err := refreshTieredBillingGroup(info)
				if err != nil {
					t.Fatal(err)
				}
				result, err := billingexpr.ComputeTieredQuota(frozen, params)
				if err != nil {
					t.Fatal(err)
				}
				if result.MatchedTier != wantTier || result.ActualQuotaBeforeGroup != want || frozen.ExprString != expression {
					t.Fatalf("wrong charge input=%d claude=%t attempt=%d got=%+v want=%v", input, claude, attempt, result, want)
				}
			}
		}
	}
}
