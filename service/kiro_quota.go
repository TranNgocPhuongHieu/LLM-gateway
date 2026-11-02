package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const KiroQuotaURL = "https://kiro.pix4k.com/api/pub/quota"

type KiroQuotaCredential struct {
	Label  string
	APIKey string
}

type KiroQuotaKeyResult struct {
	Label           string  `json:"label"`
	Name            string  `json:"name,omitempty"`
	UsedCredits     float64 `json:"used_credits"`
	LimitCredits    float64 `json:"limit_credits"`
	Remaining       float64 `json:"remaining_credits"`
	UsedPercent     float64 `json:"used_percent"`
	Unlimited       bool    `json:"unlimited"`
	Status          string  `json:"status"`
	Error           string  `json:"error,omitempty"`
	QuotaMode       string  `json:"quota_mode,omitempty"`
	UsedTokens      float64 `json:"used_tokens,omitempty"`
	LimitTokens     float64 `json:"limit_tokens,omitempty"`
	RemainingTokens float64 `json:"remaining_tokens,omitempty"`
}

type KiroQuotaReport struct {
	Keys           []KiroQuotaKeyResult `json:"keys"`
	TotalRemaining float64              `json:"total_remaining"`
	TotalLimit     float64              `json:"total_limit"`
	TotalUsed      float64              `json:"total_used"`
}

func IsKiroBaseURL(baseURL string) bool {
	baseURL = strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	return baseURL == "https://kiro.pix4k.com" || baseURL == "http://kiro.pix4k.com" ||
		strings.HasPrefix(baseURL, "https://kiro.pix4k.com/") || strings.HasPrefix(baseURL, "http://kiro.pix4k.com/")
}

func CheckKiroQuotas(ctx context.Context, credentials []KiroQuotaCredential) KiroQuotaReport {
	report := KiroQuotaReport{Keys: make([]KiroQuotaKeyResult, 0, len(credentials))}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, credential := range credentials {
		row := KiroQuotaKeyResult{Label: credential.Label, Status: "error"}
		var response map[string]any
		if strings.TrimSpace(credential.APIKey) == "" {
			row.Error = "Missing Kiro key"
			report.Keys = append(report.Keys, row)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, KiroQuotaURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(credential.APIKey))
			resp, requestErr := client.Do(req)
			if requestErr == nil {
				defer resp.Body.Close()
				if decodeErr := json.NewDecoder(resp.Body).Decode(&response); decodeErr != nil {
					err = decodeErr
				} else if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
					err = fmt.Errorf("Kiro returned HTTP %d", resp.StatusCode)
				}
			} else {
				err = requestErr
			}
		}
		if err != nil {
			row.Error = err.Error()
			report.Keys = append(report.Keys, row)
			continue
		}
		row = parseKiroQuotaResult(credential.Label, response)
		report.Keys = append(report.Keys, row)
		if row.Status == "ok" && row.QuotaMode != "token" {
			report.TotalUsed += row.UsedCredits
			if !row.Unlimited {
				report.TotalLimit += row.LimitCredits
				report.TotalRemaining += row.Remaining
			}
		}
	}
	return report
}

func parseKiroQuotaResult(label string, response map[string]any) KiroQuotaKeyResult {
	for _, key := range []string{"data", "quota"} {
		if nested, ok := response[key].(map[string]any); ok {
			response = nested
			break
		}
	}
	row := KiroQuotaKeyResult{Label: label, Status: "ok"}
	row.QuotaMode = strings.ToLower(kiroString(response, "quota_mode", "mode"))
	row.UsedTokens = kiroNumber(response, "token_used", "tokens_used")
	row.LimitTokens = kiroNumber(response, "token_limit", "tokens_limit")
	row.RemainingTokens = kiroNumber(response, "remaining_tokens", "tokens_remaining")
	row.Name = kiroString(response, "name", "key_name")
	if row.QuotaMode == "token" {
		limit, hasLimit := response["token_limit"].(float64)
		used, hasUsed := response["token_used"].(float64)
		remaining, hasRemaining := response["remaining"].(float64)
		if !hasLimit || !hasUsed || !hasRemaining || limit < 0 || used < 0 || remaining < 0 {
			row.Status = "error"
			row.Error = "Invalid Kiro token quota response"
			return row
		}
		row.LimitTokens, row.UsedTokens, row.RemainingTokens = limit, used, remaining
		row.Unlimited = limit == 0
		if limit > 0 {
			row.UsedPercent = used / limit * 100
		}
		return row
	}
	row.UsedCredits = kiroNumber(response, "used_credits", "credits_used", "credit_used", "used")
	row.LimitCredits = kiroNumber(response, "limit_credits", "credit_limit", "limit")
	row.Remaining = kiroNumber(response, "remaining_credits", "credits_remaining", "remaining", "credits")
	row.Unlimited = row.LimitCredits <= 0
	if row.LimitCredits > 0 {
		if row.UsedCredits == 0 && row.Remaining > 0 {
			row.UsedCredits = max(0, row.LimitCredits-row.Remaining)
		}
		if row.Remaining == 0 && row.UsedCredits > 0 {
			row.Remaining = max(0, row.LimitCredits-row.UsedCredits)
		}
		row.UsedPercent = row.UsedCredits / row.LimitCredits * 100
	} else {
		row.Unlimited = true
	}
	return row
}

func kiroNumber(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := values[key].(float64); ok {
			return value
		}
	}
	return 0
}

func kiroString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			return value
		}
	}
	return ""
}
