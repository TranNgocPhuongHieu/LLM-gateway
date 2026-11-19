package model

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/logger"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int           `json:"id"`
	UserId          int           `json:"user_id" gorm:"index"`
	Amount          int64         `json:"amount"`
	Money           float64       `json:"money"`
	TradeNo         string        `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string        `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string        `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64         `json:"create_time"`
	CompleteTime    int64         `json:"complete_time"`
	Status          string        `json:"status"`
	Sepay           *SepayPayment `json:"sepay,omitempty" gorm:"-"`
	Binance         *BinanceTopUp `json:"binance,omitempty" gorm:"-"`
	Username        string        `json:"username,omitempty" gorm:"-"`
}

type TopUpSummary struct {
	Total        int64   `json:"total"`
	Pending      int64   `json:"pending"`
	Success      int64   `json:"success"`
	Cancelled    int64   `json:"cancelled"`
	Processing   int64   `json:"processing"`
	NeedsReview  int64   `json:"needs_review"`
	CollectedVND float64 `json:"collected_vnd"`
	TodayVND     float64 `json:"today_vnd"`
}

// GetTopUpSummary returns all-time admin counts and VND totals. todayStart and
// todayEnd bound the caller's local day in unix seconds (end exclusive).
func GetTopUpSummary(todayStart, todayEnd int64) (TopUpSummary, error) {
	var summary TopUpSummary
	if err := DB.Model(&TopUp{}).Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	if err := DB.Model(&TopUp{}).Where("status = ?", common.TopUpStatusPending).Count(&summary.Pending).Error; err != nil {
		return summary, err
	}
	if err := DB.Model(&TopUp{}).Where("status = ?", common.TopUpStatusSuccess).Count(&summary.Success).Error; err != nil {
		return summary, err
	}
	if err := DB.Model(&TopUp{}).Where("status = ?", common.TopUpStatusExpired).Count(&summary.Cancelled).Error; err != nil {
		return summary, err
	}
	summary.Processing = summary.Pending
	if err := scopeAdminNeedsReview(DB.Model(&TopUp{})).Count(&summary.NeedsReview).Error; err != nil {
		return summary, err
	}
	collected, today, err := sumSuccessfulTopUpVND(todayStart, todayEnd)
	if err != nil {
		return summary, err
	}
	summary.CollectedVND = collected
	summary.TodayVND = today
	return summary, nil
}

// topUpVNDExpr is the VND amount the admin UI shows for a successful top-up:
// the SePay amount when a SePay order exists, otherwise the stored money.
const topUpVNDExpr = "COALESCE(sepay_payments.amount_vnd, top_ups.money)"

func sumSuccessfulTopUpVND(todayStart, todayEnd int64) (collected float64, today float64, err error) {
	var row struct {
		Collected float64
		Today     float64
	}
	err = DB.Model(&TopUp{}).
		Joins("LEFT JOIN sepay_payments ON sepay_payments.top_up_id = top_ups.id").
		Where("top_ups.status = ?", common.TopUpStatusSuccess).
		Select("COALESCE(SUM("+topUpVNDExpr+"), 0) AS collected, "+
			"COALESCE(SUM(CASE WHEN top_ups.create_time >= ? AND top_ups.create_time < ? THEN "+topUpVNDExpr+" ELSE 0 END), 0) AS today",
			todayStart, todayEnd).
		Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}
	return row.Collected, row.Today, nil
}

// EnrichTopUpUsernames fills usernames with one bounded query for admin lists.
func EnrichTopUpUsernames(topups []*TopUp) error {
	ids := make([]int, 0, len(topups))
	for _, topup := range topups {
		if topup != nil && topup.UserId > 0 {
			ids = append(ids, topup.UserId)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var users []User
	if err := DB.Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return err
	}
	byID := make(map[int]string, len(users))
	for _, user := range users {
		byID[user.Id] = user.Username
	}
	for _, topup := range topups {
		if topup != nil {
			topup.Username = byID[topup.UserId]
		}
	}
	return nil
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
	PaymentProviderSepay        = "sepay"
)

var (
	ErrPaymentMethodMismatch    = errors.New("payment method mismatch")
	ErrTopUpNotFound            = errors.New("topup not found")
	ErrTopUpStatusInvalid       = errors.New("topup status invalid")
	ErrInvalidTopUpQuota        = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded  = errors.New("top-up quota limit exceeded")
	ErrWalletQuotaLimitExceeded = errors.New("wallet quota limit exceeded")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota > common.MaxWalletQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxWalletQuota - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the wallet ceiling while adding quota.
// Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int, updates map[string]any) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	updateFields := make(map[string]any, len(updates)+1)
	maps.Copy(updateFields, updates)
	updateFields["quota"] = gorm.Expr("quota + ?", creditedQuota)

	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, maxCurrentQuota).
		Updates(updateFields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var count int64
	if err := tx.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrTopUpQuotaLimitExceeded
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		var quotaErr error
		quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		if bonus := operation_setting.GetActiveTopupBonus(topUp.Amount, time.Now()); bonus > 0 {
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(int64(quotaToAdd)).Mul(decimal.NewFromFloat(1 + bonus)))
			if quotaErr != nil || quotaToAdd <= 0 {
				return ErrInvalidTopUpQuota
			}
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		return creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil)
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) {
			common.SysError("epay topup failed: " + err.Error())
		}
		return false, err
	}
	if alreadyDone {
		return true, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "epay topup")

	common.SysLog(fmt.Sprintf("易支付充值成功 trade_no=%s user_id=%d quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota, err = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if bonus := operation_setting.GetActiveTopupBonus(topUp.Amount, time.Now()); bonus > 0 {
			quota, err = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(int64(quota)).Mul(decimal.NewFromFloat(1 + bonus)))
		}
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}
		return creditTopUpQuota(tx, topUp.UserId, quota, map[string]any{
			"stripe_customer": customerId,
		})
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, "stripe topup")

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

const (
	AdminTopUpStatusSuccess     = "success"
	AdminTopUpStatusExpired     = "expired"
	AdminTopUpStatusNeedsReview = "needs_review"
	AdminTopUpMethodSepay       = PaymentProviderSepay
	AdminTopUpMethodBinance     = "binance"
)

// adminTopUpBinanceReviewStatuses mirrors the admin UI rule (needsReview):
// USDT orders in these states need manual action.
var adminTopUpBinanceReviewStatuses = []string{"review", "partial", "matched", "expired"}

// scopeAdminNeedsReview keeps unsuccessful top-ups whose USDT order is in a
// review state or whose SePay payment is flagged for manual review.
func scopeAdminNeedsReview(db *gorm.DB) *gorm.DB {
	return db.Where(
		"top_ups.status <> ? AND (EXISTS (SELECT 1 FROM binance_top_ups WHERE binance_top_ups.top_up_id = top_ups.id AND binance_top_ups.status IN ?) OR EXISTS (SELECT 1 FROM sepay_payments sp_review WHERE sp_review.top_up_id = top_ups.id AND sp_review.verification_status = ?))",
		common.TopUpStatusSuccess, adminTopUpBinanceReviewStatuses, SepayVerificationReview,
	)
}

// escapeAdminLikeLiteral escapes LIKE wildcards so keyword text matches literally ('!' is the ESCAPE character).
func escapeAdminLikeLiteral(input string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(input)
}

// scopeAdminKeyword matches a case-insensitive substring of the order ID or of
// the matched Binance deposit TxID.
func scopeAdminKeyword(db *gorm.DB, keyword string) *gorm.DB {
	pattern := "%" + escapeAdminLikeLiteral(strings.ToLower(keyword)) + "%"
	txOrders := DB.Model(&BinanceTopUp{}).
		Select("binance_top_ups.top_up_id").
		Joins("JOIN binance_deposits ON binance_deposits.id = binance_top_ups.matched_deposit_id").
		Where("LOWER(binance_deposits.tx_id) LIKE ? ESCAPE '!'", pattern)
	return db.Where("(LOWER(top_ups.trade_no) LIKE ? ESCAPE '!' OR top_ups.id IN (?))", pattern, txOrders)
}

// AdminTopUpFilter holds the optional server-side filters of the admin top-up list.
type AdminTopUpFilter struct {
	Keyword       string
	Status        string // "", AdminTopUpStatusSuccess, AdminTopUpStatusExpired or AdminTopUpStatusNeedsReview
	PaymentMethod string // "", AdminTopUpMethodSepay or AdminTopUpMethodBinance
	StartTime     int64  // inclusive create_time lower bound; 0 = unbounded
	EndTime       int64  // exclusive create_time upper bound; 0 = unbounded
}

// ListAdminTopUps returns one page of platform-wide top-ups matching filter, newest first.
func ListAdminTopUps(filter AdminTopUpFilter, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	query := DB.Model(&TopUp{})
	if filter.Keyword != "" {
		query = scopeAdminKeyword(query, filter.Keyword)
	}
	switch filter.Status {
	case "":
	case AdminTopUpStatusSuccess:
		query = query.Where("top_ups.status = ?", common.TopUpStatusSuccess)
	case AdminTopUpStatusExpired:
		query = query.Where("top_ups.status = ?", common.TopUpStatusExpired)
	case AdminTopUpStatusNeedsReview:
		query = scopeAdminNeedsReview(query)
	default:
		return nil, 0, fmt.Errorf("unsupported top-up status filter: %q", filter.Status)
	}
	switch filter.PaymentMethod {
	case "":
	case AdminTopUpMethodSepay:
		query = query.Where("top_ups.payment_provider = ?", PaymentProviderSepay)
	case AdminTopUpMethodBinance:
		query = query.Where("top_ups.id IN (?)", DB.Model(&BinanceTopUp{}).Select("top_up_id"))
	default:
		return nil, 0, fmt.Errorf("unsupported top-up payment method filter: %q", filter.PaymentMethod)
	}
	if filter.StartTime > 0 {
		query = query.Where("top_ups.create_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("top_ups.create_time < ?", filter.EndTime)
	}

	pageQuery := query.Session(&gorm.Session{})
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = pageQuery.Order("top_ups.id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// GetAllTopUps returns an unfiltered page of platform-wide top-ups.
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return ListAdminTopUps(AdminTopUpFilter{}, pageInfo)
}

// SearchAllTopUps searches platform-wide top-ups by order ID or Binance TxID.
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	return ListAdminTopUps(AdminTopUpFilter{Keyword: keyword}, pageInfo)
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if order := GetTopUpByTradeNo(tradeNo); order != nil && order.PaymentProvider == PaymentProviderSepay {
		return errors.New("SePay orders must be reconciled with the provider")
	}
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.PaymentProvider == PaymentProviderSepay {
			return errors.New("SePay orders require provider reconciliation")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		var quotaErr error
		if topUp.PaymentProvider == PaymentProviderStripe {
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		} else {
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		if bonus := operation_setting.GetActiveTopupBonus(topUp.Amount, time.Now()); bonus > 0 {
			quotaToAdd, quotaErr = common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(int64(quotaToAdd)).Mul(decimal.NewFromFloat(1 + bonus)))
			if quotaErr != nil || quotaToAdd <= 0 {
				return ErrInvalidTopUpQuota
			}
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, nil); err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	syncCreditUserQuotaCache(userId, quotaToAdd, "manual topup")
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
