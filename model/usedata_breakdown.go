package model

import (
	"errors"
	"sort"
	"strconv"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
)

// UsageBreakdownTotal holds the summed quota_data counters for a user's range.
type UsageBreakdownTotal struct {
	Quota     int64 `json:"quota"`
	Count     int64 `json:"count"`
	TokenUsed int64 `json:"token_used"`
}

// UsageBreakdownItem is one group (model or token) of a user's quota_data.
// Key is the model name or the token id as a string.
type UsageBreakdownItem struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Quota     int64  `json:"quota"`
	Count     int64  `json:"count"`
	TokenUsed int64  `json:"token_used"`
}

type UsageBreakdown struct {
	Total UsageBreakdownTotal  `json:"total"`
	Items []UsageBreakdownItem `json:"items"`
}

type usageBreakdownRow struct {
	ItemName  string `gorm:"column:item_name"`
	ItemID    int    `gorm:"column:item_id"`
	SumQuota  int64  `gorm:"column:sum_quota"`
	SumCount  int64  `gorm:"column:sum_count"`
	SumTokens int64  `gorm:"column:sum_token_used"`
}

// GetUserUsageBreakdown aggregates a single user's quota_data rows in
// [startTimestamp, endTimestamp] grouped by model name or token id. Token
// labels are only resolved for tokens owned by userId and not soft-deleted;
// any other token gets an empty label.
func GetUserUsageBreakdown(userId int, startTimestamp int64, endTimestamp int64, groupBy string) (*UsageBreakdown, error) {
	var groupColumn, selectColumn string
	switch groupBy {
	case "model":
		groupColumn, selectColumn = "model_name", "model_name AS item_name, 0 AS item_id"
	case "token":
		groupColumn, selectColumn = "token_id", "0 AS item_name, token_id AS item_id"
	default:
		return nil, errors.New("invalid group_by")
	}

	var rows []usageBreakdownRow
	err := DB.Table("quota_data").
		Select(selectColumn+", COALESCE(SUM(quota), 0) AS sum_quota, COALESCE(SUM(count), 0) AS sum_count, COALESCE(SUM(token_used), 0) AS sum_token_used").
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTimestamp, endTimestamp).
		Group(groupColumn).
		Scan(&rows).Error
	if err != nil {
		common.SysError("failed to aggregate user usage breakdown: " + err.Error())
		return nil, errors.New("failed to query usage data")
	}

	result := &UsageBreakdown{Items: make([]UsageBreakdownItem, 0, len(rows))}
	tokenIDs := make([]int, 0)
	for _, row := range rows {
		item := UsageBreakdownItem{
			Quota:     row.SumQuota,
			Count:     row.SumCount,
			TokenUsed: row.SumTokens,
		}
		if groupBy == "model" {
			item.Key = row.ItemName
			item.Label = row.ItemName
		} else {
			item.Key = strconv.Itoa(row.ItemID)
			tokenIDs = append(tokenIDs, row.ItemID)
		}
		result.Total.Quota += row.SumQuota
		result.Total.Count += row.SumCount
		result.Total.TokenUsed += row.SumTokens
		result.Items = append(result.Items, item)
	}

	if groupBy == "token" && len(tokenIDs) > 0 {
		labels, err := ownedTokenNames(userId, tokenIDs)
		if err != nil {
			return nil, err
		}
		for i := range result.Items {
			result.Items[i].Label = labels[result.Items[i].Key]
		}
	}

	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].Quota != result.Items[j].Quota {
			return result.Items[i].Quota > result.Items[j].Quota
		}
		return result.Items[i].Key < result.Items[j].Key
	})
	return result, nil
}

// ownedTokenNames returns names keyed by token id (as string) for tokens owned
// by userId. The default GORM scope excludes soft-deleted tokens, so deleted
// tokens are absent from the result and get an empty label.
func ownedTokenNames(userId int, tokenIDs []int) (map[string]string, error) {
	type tokenNameRow struct {
		Id   int
		Name string
	}
	var tokens []tokenNameRow
	err := DB.Model(&Token{}).Select("id, name").
		Where("user_id = ? and id in ?", userId, tokenIDs).
		Scan(&tokens).Error
	if err != nil {
		common.SysError("failed to load token names for usage breakdown: " + err.Error())
		return nil, errors.New("failed to query usage data")
	}
	names := make(map[string]string, len(tokens))
	for _, token := range tokens {
		names[strconv.Itoa(token.Id)] = token.Name
	}
	return names, nil
}
