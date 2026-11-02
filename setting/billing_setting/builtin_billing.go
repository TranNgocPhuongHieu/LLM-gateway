package billing_setting

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
var builtinBillingExpr = map[string]string{
	// https://developers.openai.com/api/docs/pricing (Standard, 2026-09-09).
	// The Images API reports image output in output_tokens, normalized to c.
	"gpt-image-2":            `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-sunburst": `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	"gpt-image-2.5-flare":    `tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`,
	// https://developers.openai.com/api/docs/models/gpt-6-astra
	// Standard pricing; the long-context rates apply to the whole request.
	// Do not infer service-tier discounts from incoming request parameters:
	// channels filter service_tier by default, so it may not reach the upstream.
	"gpt-6-astra":     `len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)`,
	"claude-opus-4-6": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	"claude-opus-4-7": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	"claude-opus-4-8": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	"claude-opus-5":   `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	"claude-opus-5-5": `tier("base", p * 0.05769230769230769 + c * 0.28846153846153846 + cr * 0.028846153846153848 + cc * 0.013)`,
	"gpt-6.1-sol":     `tier("base", p * 0.026923076923076924 + c * 0.13461538461538462 + cr * 0.013461538461538462 + cc * 0.008)`,
}
