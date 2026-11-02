package operation_setting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/config"
)

// PromotionSetting stores event configuration as JSON so administrators can
// change campaigns without changing the database schema. The public
// announcement endpoint only returns an announcement while its window is
// active; model discounts are applied at request pricing time.
type PromotionSetting struct {
	TopupPromotions string `json:"topup_promotions"`
	ModelPromotions string `json:"model_promotions"`
	Announcement    string `json:"announcement"`
}

var promotionSetting = PromotionSetting{}

func init() {
	config.GlobalConfig.Register("promotion_setting", &promotionSetting)
}

func GetPromotionSetting() *PromotionSetting {
	return &promotionSetting
}

type ModelPromotion struct {
	Enabled         bool     `json:"enabled"`
	Models          []string `json:"models"`
	DiscountPercent float64  `json:"discount_percent"`
	StartAt         string   `json:"start_at"`
	EndAt           string   `json:"end_at"`
}

type PromotionAnnouncement struct {
	Enabled bool   `json:"enabled"`
	Title   string `json:"title"`
	Message string `json:"message"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type VisiblePromotion struct {
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	MinAmount       int64    `json:"min_amount,omitempty"`
	BonusPercent    float64  `json:"bonus_percent,omitempty"`
	DiscountPercent float64  `json:"discount_percent,omitempty"`
	Models          []string `json:"models,omitempty"`
	StartAt         string   `json:"start_at"`
	EndAt           string   `json:"end_at"`
}

func GetVisiblePromotions(now time.Time) []VisiblePromotion {
	s := GetPromotionSetting()
	out := []VisiblePromotion{}
	status := func(enabled bool, start, end string) (string, bool) {
		if !enabled {
			return "", false
		}
		st, en := promotionTime(start, time.Unix(0, 0).UTC()), promotionTime(end, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC))
		if !now.Before(en) {
			return "", false
		}
		if now.Before(st) {
			return "upcoming", true
		}
		return "active", true
	}
	var tops []TopupPromotion
	if json.Unmarshal([]byte(s.TopupPromotions), &tops) == nil {
		for _, p := range tops {
			st, ok := status(p.Enabled, p.StartAt, p.EndAt)
			if ok {
				out = append(out, VisiblePromotion{Type: "topup", Status: st, Title: "Nạp tiền", Summary: "Nạp từ " + formatAmount(p.MinAmount) + " ₫ nhận thêm " + formatPercent(p.BonusPercent) + "% credit", MinAmount: p.MinAmount, BonusPercent: p.BonusPercent, StartAt: p.StartAt, EndAt: p.EndAt})
			}
		}
	}
	var mods []ModelPromotion
	if json.Unmarshal([]byte(s.ModelPromotions), &mods) == nil {
		for _, p := range mods {
			st, ok := status(p.Enabled, p.StartAt, p.EndAt)
			if ok {
				n := len(p.Models)
				target := fmt.Sprintf("%d mô hình", n)
				if len(p.Models) == 1 && p.Models[0] == "*" {
					target = "tất cả mô hình"
				}
				out = append(out, VisiblePromotion{Type: "model", Status: st, Title: "Giảm giá mô hình", Summary: "Giảm " + formatPercent(p.DiscountPercent) + "% cho " + target, DiscountPercent: p.DiscountPercent, Models: p.Models, StartAt: p.StartAt, EndAt: p.EndAt})
			}
		}
	}
	return out
}

func formatAmount(v int64) string {
	b := strconv.FormatInt(v, 10)
	for i := len(b) - 3; i > 0; i -= 3 {
		b = b[:i] + "." + b[i:]
	}
	return b
}
func formatPercent(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

type TopupPromotion struct {
	Enabled         bool    `json:"enabled"`
	MinAmount       int64   `json:"min_amount"`
	DiscountPercent float64 `json:"discount_percent"`
	BonusPercent    float64 `json:"bonus_percent"`
	StartAt         string  `json:"start_at"`
	EndAt           string  `json:"end_at"`
}

func promotionTime(value string, fallback time.Time) time.Time {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fallback
	}
	return parsed
}

func promotionActive(startAt, endAt string, now time.Time) bool {
	start := promotionTime(startAt, time.Unix(0, 0).UTC())
	end := promotionTime(endAt, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC))
	return !now.Before(start) && now.Before(end)
}

// GetActiveModelDiscount returns the highest matching active discount as a
// fraction in [0, 1]. Invalid campaigns are ignored rather than changing
// billing behavior.
func GetActiveModelDiscount(model string, now time.Time) float64 {
	settings := GetPromotionSetting()
	var promotions []ModelPromotion
	if json.Unmarshal([]byte(settings.ModelPromotions), &promotions) != nil {
		return 0
	}
	best := 0.0
	for _, promotion := range promotions {
		if !promotion.Enabled || !promotionActive(promotion.StartAt, promotion.EndAt, now) {
			continue
		}
		matched := false
		for _, name := range promotion.Models {
			if strings.TrimSpace(name) == "*" || strings.EqualFold(strings.TrimSpace(name), model) {
				matched = true
				break
			}
		}
		if !matched || promotion.DiscountPercent <= 0 || promotion.DiscountPercent >= 100 {
			continue
		}
		if promotion.DiscountPercent > best {
			best = promotion.DiscountPercent
		}
	}
	return best / 100
}

func GetActiveTopupBonus(amount int64, now time.Time) float64 {
	settings := GetPromotionSetting()
	var promotions []TopupPromotion
	if json.Unmarshal([]byte(settings.TopupPromotions), &promotions) != nil {
		return 0
	}
	best := 0.0
	for _, promotion := range promotions {
		if !promotion.Enabled || amount < promotion.MinAmount || !promotionActive(promotion.StartAt, promotion.EndAt, now) {
			continue
		}
		if promotion.BonusPercent > best && promotion.BonusPercent > 0 && promotion.BonusPercent < 100 {
			best = promotion.BonusPercent
		}
	}
	return best / 100
}

func GetActivePromotionAnnouncement(now time.Time) *PromotionAnnouncement {
	settings := GetPromotionSetting()
	var announcement PromotionAnnouncement
	if json.Unmarshal([]byte(settings.Announcement), &announcement) != nil ||
		!announcement.Enabled || strings.TrimSpace(announcement.Message) == "" ||
		!promotionActive(announcement.StartAt, announcement.EndAt, now) {
		return nil
	}
	return &announcement
}

// GetVisiblePromotionAnnouncement includes scheduled campaigns before start;
// the frontend renders the upcoming label and hides only after expiry.
func GetVisiblePromotionAnnouncement(now time.Time) *PromotionAnnouncement {
	settings := GetPromotionSetting()
	var announcement PromotionAnnouncement
	if json.Unmarshal([]byte(settings.Announcement), &announcement) != nil || !announcement.Enabled || strings.TrimSpace(announcement.Message) == "" {
		return nil
	}
	end := promotionTime(announcement.EndAt, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC))
	if !now.Before(end) {
		return nil
	}
	return &announcement
}
