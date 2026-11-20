package model

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"gorm.io/gorm"
)

// signupGrantLogPrefix is the content prefix written by User.FinishInsert for
// the QuotaForNewUser grant (type LogTypeSystem).
const signupGrantLogPrefix = "新用户注册赠送 "

// adminMicroUSDTUnits converts binance_top_ups.amount_units to USDT.
const adminMicroUSDTUnits = 1000000.0

type AdminTopUpMethodSummary struct {
	Method   string  `json:"method"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
	Count    int64   `json:"count"`
	// PaidVND and BonusVND: see AdminTopUpSummary.
	PaidVND  int64 `json:"paid_vnd"`
	BonusVND int64 `json:"bonus_vnd"`
}

type AdminTopUpSummary struct {
	CreditedQuota int64 `json:"credited_quota"`
	// PaidVND is the money customers paid, in VND, before any bonus: the SePay
	// amount, or the Binance USDT base amount converted at the order's rate
	// (fee excluded). It is read from the order, not from quota_snapshot, so
	// Binance orders whose snapshot was wrong are still counted correctly.
	// Only SePay and Binance orders contribute.
	PaidVND int64 `json:"paid_vnd"`
	// BonusVND is the bonus credited on top of PaidVND, in VND
	// (order credits x 1,000 - PaidVND, never negative).
	BonusVND     int64                     `json:"bonus_vnd"`
	Count        int64                     `json:"count"`
	PendingCount int64                     `json:"pending_count"`
	ByMethod     []AdminTopUpMethodSummary `json:"by_method"`
}

type AdminUserSummary struct {
	NewUsers           int64 `json:"new_users"`
	NewGoogleUsers     int64 `json:"new_google_users"`
	SignupQuotaGranted int64 `json:"signup_quota_granted"`
	ActiveUsers        int64 `json:"active_users"`
}

type AdminRequestSummary struct {
	Total  int64 `json:"total"`
	Errors int64 `json:"errors"`
}

type AdminUsageSummary struct {
	// ConsumedQuota is the quota charged on successful (consume) requests.
	ConsumedQuota int64 `json:"consumed_quota"`
}

type AdminSummary struct {
	Topup    AdminTopUpSummary   `json:"topup"`
	Users    AdminUserSummary    `json:"users"`
	Requests AdminRequestSummary `json:"requests"`
	Usage    AdminUsageSummary   `json:"usage"`
}

// GetAdminSummary aggregates platform-wide top-up, signup and request figures
// for [startTimestamp, endTimestamp] (unix seconds, inclusive). PendingCount is
// the only figure that ignores the range: it is the sidebar review badge count.
//
// excludeInternal drops admin and root accounts (internalUserIDs) from the
// usage figures: active users, requests and consumed quota. Top-ups and
// signups are always platform-wide.
func GetAdminSummary(startTimestamp int64, endTimestamp int64, excludeInternal bool) (*AdminSummary, error) {
	var excludeIDs []int
	if excludeInternal {
		ids, err := internalUserIDs()
		if err != nil {
			return nil, err
		}
		excludeIDs = ids
	}
	topup, err := getAdminTopUpSummary(startTimestamp, endTimestamp)
	if err != nil {
		return nil, err
	}
	users, err := getAdminUserSummary(startTimestamp, endTimestamp, excludeIDs)
	if err != nil {
		return nil, err
	}
	requests, usage, err := getAdminRequestSummary(startTimestamp, endTimestamp, excludeIDs)
	if err != nil {
		return nil, err
	}
	return &AdminSummary{Topup: topup, Users: users, Requests: requests, Usage: usage}, nil
}

// internalUserIDs returns the ids of admin and root accounts, soft-deleted
// ones included, whose own usage the admin Overview can hide.
func internalUserIDs() ([]int, error) {
	ids := make([]int, 0)
	err := DB.Unscoped().Model(&User{}).Where("role >= ?", common.RoleAdminUser).Pluck("id", &ids).Error
	return ids, err
}

// excludeUserIDs drops rows owned by ids. An empty list leaves tx unchanged,
// because "NOT IN ()" is invalid SQL.
func excludeUserIDs(tx *gorm.DB, ids []int) *gorm.DB {
	if len(ids) == 0 {
		return tx
	}
	return tx.Where("user_id NOT IN ?", ids)
}

// adminTopUpGroupRow is one (method, currency) bucket of completed top-ups.
type adminTopUpGroupRow struct {
	MethodKey     string  `gorm:"column:method_key"`
	CurrencyKey   string  `gorm:"column:currency_key"`
	RowCount      int64   `gorm:"column:row_count"`
	Amount        float64 `gorm:"column:amount"`
	CreditedQuota int64   `gorm:"column:credited_quota"`
	PaidVND       int64   `gorm:"column:paid_vnd"`
	CreditedVND   int64   `gorm:"column:credited_vnd"`
}

// getAdminTopUpSummary groups successful top-ups by payment method. A top-up
// completes at complete_time, or at create_time when no completion time was
// stored. Each order is joined to at most one SePay and one Binance row (both
// tables have a unique top_up_id), so the joins never multiply rows.
//
// Method and currency come from the provider rows when they exist: SePay is
// "sepay" in VND, Binance is "binance" in USDT. Other providers (epay, stripe,
// ...) use payment_provider with an empty currency, because their display
// currency is a runtime setting that is not stored on the order.
//
// credited_quota uses the quota_snapshot that the settlement transaction
// credited. Only SePay and Binance persist that snapshot; other providers
// contribute 0 here, so credited_quota understates them.
func getAdminTopUpSummary(startTimestamp int64, endTimestamp int64) (AdminTopUpSummary, error) {
	summary := AdminTopUpSummary{ByMethod: make([]AdminTopUpMethodSummary, 0)}

	var rows []adminTopUpGroupRow
	err := DB.Model(&TopUp{}).
		Joins("LEFT JOIN binance_top_ups ON binance_top_ups.top_up_id = top_ups.id").
		Joins("LEFT JOIN sepay_payments ON sepay_payments.top_up_id = top_ups.id").
		Where("top_ups.status = ?", common.TopUpStatusSuccess).
		Where("COALESCE(NULLIF(top_ups.complete_time, 0), top_ups.create_time) >= ? AND COALESCE(NULLIF(top_ups.complete_time, 0), top_ups.create_time) <= ?",
			startTimestamp, endTimestamp).
		Select("CASE WHEN binance_top_ups.id IS NOT NULL THEN 'binance' " +
			"WHEN sepay_payments.id IS NOT NULL THEN 'sepay' " +
			"ELSE COALESCE(NULLIF(top_ups.payment_provider, ''), top_ups.payment_method, '') END AS method_key, " +
			"CASE WHEN binance_top_ups.id IS NOT NULL THEN binance_top_ups.coin " +
			"WHEN sepay_payments.id IS NOT NULL THEN sepay_payments.currency " +
			"ELSE '' END AS currency_key, " +
			"COUNT(*) AS row_count, " +
			"COALESCE(SUM(CASE WHEN binance_top_ups.id IS NOT NULL THEN binance_top_ups.amount_units / 1000000.0 " +
			"WHEN sepay_payments.id IS NOT NULL THEN sepay_payments.amount_vnd * 1.0 " +
			"ELSE top_ups.money END), 0) AS amount, " +
			"COALESCE(SUM(CASE WHEN binance_top_ups.id IS NOT NULL THEN binance_top_ups.quota_snapshot " +
			"WHEN sepay_payments.id IS NOT NULL THEN sepay_payments.quota_snapshot " +
			"ELSE 0 END), 0) AS credited_quota, " +
			"COALESCE(SUM(CASE WHEN binance_top_ups.id IS NOT NULL THEN binance_top_ups.amount_vnd " +
			"WHEN sepay_payments.id IS NOT NULL THEN sepay_payments.amount_vnd " +
			"ELSE 0 END), 0) AS paid_vnd, " +
			"COALESCE(SUM(CASE WHEN binance_top_ups.id IS NOT NULL THEN binance_top_ups.credits * 1000 " +
			"WHEN sepay_payments.id IS NOT NULL THEN sepay_payments.credits * 1000 " +
			"ELSE 0 END), 0) AS credited_vnd").
		Group("method_key, currency_key").
		Order("method_key, currency_key").
		Scan(&rows).Error
	if err != nil {
		return summary, err
	}
	for _, row := range rows {
		bonusVND := row.CreditedVND - row.PaidVND
		if bonusVND < 0 {
			bonusVND = 0
		}
		summary.Count += row.RowCount
		summary.CreditedQuota += row.CreditedQuota
		summary.PaidVND += row.PaidVND
		summary.BonusVND += bonusVND
		summary.ByMethod = append(summary.ByMethod, AdminTopUpMethodSummary{
			Method:   row.MethodKey,
			Currency: row.CurrencyKey,
			Amount:   row.Amount,
			Count:    row.RowCount,
			PaidVND:  row.PaidVND,
			BonusVND: bonusVND,
		})
	}

	// Same predicate as the admin top-up review badge (needs_review count).
	if err := scopeAdminNeedsReview(DB.Model(&TopUp{})).Count(&summary.PendingCount).Error; err != nil {
		return summary, err
	}
	return summary, nil
}

// getAdminUserSummary counts accounts created in the range, including
// soft-deleted ones, because they were real signups when created. The users
// table has no index on created_at, so this is a table scan.
func getAdminUserSummary(startTimestamp int64, endTimestamp int64, excludeIDs []int) (AdminUserSummary, error) {
	var summary AdminUserSummary
	newUsers := func() *gorm.DB {
		return DB.Unscoped().Model(&User{}).
			Where("created_at >= ? AND created_at <= ?", startTimestamp, endTimestamp)
	}
	if err := newUsers().Count(&summary.NewUsers).Error; err != nil {
		return summary, err
	}
	// A Google signup creates its claim in the same transaction as the user.
	// A later Google binding of an account created in range is also counted.
	if err := newUsers().
		Where("EXISTS (SELECT 1 FROM external_identity_claims eic WHERE eic.user_id = users.id AND eic.provider = ?)",
			ExternalIdentityProviderGoogle).
		Count(&summary.NewGoogleUsers).Error; err != nil {
		return summary, err
	}

	// The grant amount is not stored on the grant log, so the count of grant
	// logs is multiplied by the current QuotaForNewUser. This is exact only
	// while that setting has not changed inside the range.
	var grants int64
	if err := LOG_DB.Model(&Log{}).
		Where("type = ? AND created_at >= ? AND created_at <= ? AND content LIKE ?",
			LogTypeSystem, startTimestamp, endTimestamp, signupGrantLogPrefix+"%").
		Count(&grants).Error; err != nil {
		return summary, err
	}
	summary.SignupQuotaGranted = grants * int64(common.QuotaForNewUser)

	active := LOG_DB.Model(&Log{}).
		Where("type = ? AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTimestamp, endTimestamp)
	if err := excludeUserIDs(active, excludeIDs).
		Select("COUNT(DISTINCT user_id)").
		Scan(&summary.ActiveUsers).Error; err != nil {
		return summary, err
	}
	return summary, nil
}

// getAdminRequestSummary counts final customer-visible outcomes: successful
// consume logs (LogTypeConsume) and final error logs (LogTypeError). Retry
// attempts flagged request_final=false are excluded, as in the self summary.
// The usage summary is the quota charged on the same consume logs.
func getAdminRequestSummary(startTimestamp int64, endTimestamp int64, excludeIDs []int) (AdminRequestSummary, AdminUsageSummary, error) {
	var summary AdminRequestSummary
	var usage AdminUsageSummary
	var rows []struct {
		Type     int   `gorm:"column:type"`
		RowCount int64 `gorm:"column:row_count"`
		Quota    int64 `gorm:"column:quota_sum"`
	}
	tx := LOG_DB.Model(&Log{}).
		Select("type, COUNT(*) AS row_count, COALESCE(SUM(quota), 0) AS quota_sum").
		Where("type IN ? AND created_at >= ? AND created_at <= ?",
			[]int{LogTypeConsume, LogTypeError}, startTimestamp, endTimestamp)
	tx = hideIntermediateRelayErrors(excludeUserIDs(tx, excludeIDs), "")
	if err := tx.Group("type").Scan(&rows).Error; err != nil {
		return summary, usage, err
	}
	for _, row := range rows {
		switch row.Type {
		case LogTypeConsume:
			summary.Total += row.RowCount
			usage.ConsumedQuota += row.Quota
		case LogTypeError:
			summary.Total += row.RowCount
			summary.Errors += row.RowCount
		}
	}
	return summary, usage, nil
}
